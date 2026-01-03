//go:generate go tool oapi-codegen --config openapi-codegen-config.yaml ../spec/api.yaml
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/International-Combat-Archery-Alliance/assets-api/assets"
	"github.com/International-Combat-Archery-Alliance/auth"
	"github.com/International-Combat-Archery-Alliance/middleware"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type Environment int

const (
	LOCAL Environment = iota
	PROD
)

type API struct {
	assetManager  *assets.AssetsManager
	logger        *slog.Logger
	env           Environment
	cdnBaseURL    string
	authValidator auth.Validator
}

var _ StrictServerInterface = (*API)(nil)

func NewAPI(
	assetsManager *assets.AssetsManager,
	logger *slog.Logger,
	env Environment,
	cdnBaseURL string,
	authValidator auth.Validator,
) *API {
	return &API{
		assetManager:  assetsManager,
		logger:        logger,
		env:           env,
		cdnBaseURL:    cdnBaseURL,
		authValidator: authValidator,
	}
}

func (a *API) ListenAndServe(host string, port string) error {
	swagger, err := GetSwagger()
	if err != nil {
		return fmt.Errorf("error loading swagger spec: %w", err)
	}

	swagger.Servers = nil

	strictHandler := NewStrictHandler(a, []StrictMiddlewareFunc{})

	r := http.NewServeMux()

	HandlerFromMux(strictHandler, r)

	swaggerUIMiddleware, err := middleware.HostSwaggerUI("/assets", swagger)
	if err != nil {
		return fmt.Errorf("failed to create swagger ui middleware: %w", err)
	}

	middlewares := []middleware.MiddlewareFunc{
		// Executes from the bottom up
		a.openapiValidateMiddleware(swagger),
		a.corsMiddleware(),
		swaggerUIMiddleware,
		middleware.AccessLogging(a.logger),
	}

	if a.env == PROD {
		middlewares = append(middlewares, middleware.BaseNamePrefix(a.logger, "/assets"))
	}

	h := middleware.UseMiddlewares(r, middlewares...)

	s := &http.Server{
		Handler: h,
		Addr:    net.JoinHostPort(host, port),
	}

	return s.ListenAndServe()
}

func (a *API) getLoggerOrBaseLogger(ctx context.Context) *slog.Logger {
	logger, ok := middleware.GetLoggerFromCtx(ctx)
	if !ok {
		a.logger.Error("tried to get logger and it wasn't in the context")
		return a.logger
	}
	return logger
}

func (a *API) getAdminEmailFromCtx(ctx context.Context) (string, error) {
	jwt, ok := middleware.GetJWTFromCtx(ctx)
	if !ok {
		return "", fmt.Errorf("no JWT in context")
	}
	if !jwt.IsAdmin() {
		return "", fmt.Errorf("user is not admin")
	}
	return jwt.UserEmail(), nil
}

// GetAssetsV1 returns all assets at a path
func (a *API) GetAssetsV1(ctx context.Context, request GetAssetsV1RequestObject) (GetAssetsV1ResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	// guaranteed to be non-nil from openapi doc
	limit := int32(*request.Params.Limit)

	result, err := a.assetManager.GetAssets(ctx, request.Params.Path, limit, request.Params.Cursor)
	if err != nil {
		if assets.IsInvalidCursorError(err) {
			return GetAssetsV1400JSONResponse{
				Code:    InvalidCursor,
				Message: "Invalid cursor",
			}, nil
		}
		logger.Error("failed to get assets", slog.String("error", err.Error()))
		return GetAssetsV1500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get assets",
		}, nil
	}

	apiAssets := make([]Asset, len(result.Data))
	for i, asset := range result.Data {
		apiAsset, err := assetToAPI(asset, a.cdnBaseURL)
		if err != nil {
			return GetAssetsV1500JSONResponse{
				Code:    InternalError,
				Message: "Failed to get assets",
			}, nil
		}

		apiAssets[i] = apiAsset
	}

	return GetAssetsV1200JSONResponse{
		Data:        apiAssets,
		Cursor:      result.Cursor,
		HasNextPage: result.HasNextPage,
	}, nil
}

// PostAssetsV1Folders creates a new folder
func (a *API) PostAssetsV1Folders(ctx context.Context, request PostAssetsV1FoldersRequestObject) (PostAssetsV1FoldersResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	userEmail, err := a.getAdminEmailFromCtx(ctx)
	if err != nil {
		return PostAssetsV1Folders401JSONResponse{
			Code:    AuthError,
			Message: "Unauthorized",
		}, nil
	}

	folder, err := a.assetManager.CreateFolder(
		ctx,
		request.Body.Path,
		request.Body.Name,
		request.Body.Description,
		userEmail,
	)

	if err != nil {
		if assets.IsParentFolderNotFoundError(err) {
			return PostAssetsV1Folders404JSONResponse{
				Code:    ParentFolderNotFound,
				Message: fmt.Sprintf("Parent folder at path %q not found", request.Body.Path),
			}, nil
		}
		if assets.IsAlreadyExistsError(err) {
			return PostAssetsV1Folders409JSONResponse{
				Code:    AlreadyExists,
				Message: "Folder already exists",
			}, nil
		}
		logger.Error("failed to create folder", slog.String("error", err.Error()))
		return PostAssetsV1Folders500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create folder",
		}, nil
	}

	return PostAssetsV1Folders201JSONResponse{
		Folder: folderToAPI(folder),
	}, nil
}

// PostAssetsV1UploadUrl generates a presigned upload URL
func (a *API) PostAssetsV1UploadUrl(ctx context.Context, request PostAssetsV1UploadUrlRequestObject) (PostAssetsV1UploadUrlResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	// Get admin email from JWT (auth already validated by middleware)
	userEmail, err := a.getAdminEmailFromCtx(ctx)
	if err != nil {
		return PostAssetsV1UploadUrl401JSONResponse{
			Code:    AuthError,
			Message: "Unauthorized",
		}, nil
	}

	presignResult, file, err := a.assetManager.CreateFileUpload(
		ctx,
		request.Body.Path,
		request.Body.FileName,
		request.Body.Description,
		request.Body.ContentType,
		userEmail,
	)

	if err != nil {
		if assets.IsParentFolderNotFoundError(err) {
			return PostAssetsV1UploadUrl404JSONResponse{
				Code:    ParentFolderNotFound,
				Message: fmt.Sprintf("Parent folder at path %q not found", request.Body.Path),
			}, nil
		}
		logger.Error("failed to create file record", slog.String("error", err.Error()))
		return PostAssetsV1UploadUrl500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create file record",
		}, nil
	}

	return PostAssetsV1UploadUrl200JSONResponse{
		FileId:     file.ID,
		UploadUrl:  presignResult.UploadURL,
		FormFields: presignResult.FormFields,
		ExpiresAt:  presignResult.ExpiresAt,
	}, nil
}

// GetAssetsV1ByPath returns a single asset by full path
func (a *API) GetAssetsV1ByPath(ctx context.Context, request GetAssetsV1ByPathRequestObject) (GetAssetsV1ByPathResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	asset, err := a.assetManager.GetAsset(ctx, request.Params.Path)
	if err != nil {
		if assets.IsNotFoundError(err) {
			return GetAssetsV1ByPath404JSONResponse{
				Code:    NotFound,
				Message: fmt.Sprintf("Asset at path %q not found", request.Params.Path),
			}, nil
		}
		logger.Error("failed to get asset", slog.String("filepath", request.Params.Path), slog.String("error", err.Error()))
		return GetAssetsV1ByPath500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get asset",
		}, nil
	}

	apiAsset, err := assetToAPI(asset, a.cdnBaseURL)
	if err != nil {
		return GetAssetsV1ByPath500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get asset",
		}, nil
	}

	return GetAssetsV1ByPath200JSONResponse{
		Asset: apiAsset,
	}, nil
}

// DeleteAssetsV1ByPath deletes an asset by full path
func (a *API) DeleteAssetsV1ByPath(ctx context.Context, request DeleteAssetsV1ByPathRequestObject) (DeleteAssetsV1ByPathResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	err := a.assetManager.DeleteAsset(ctx, request.Params.Path)
	if err != nil {
		if assets.IsNotFoundError(err) {
			return DeleteAssetsV1ByPath404JSONResponse{
				Code:    NotFound,
				Message: fmt.Sprintf("Asset at path %q not found", request.Params.Path),
			}, nil
		}
		if assets.IsFolderNotEmptyError(err) {
			return DeleteAssetsV1ByPath400JSONResponse{
				Code:    FolderNotEmpty,
				Message: "Cannot delete folder: folder is not empty",
			}, nil
		}
		logger.Error("failed to delete asset", slog.String("filepath", request.Params.Path), slog.String("error", err.Error()))
		return DeleteAssetsV1ByPath500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get asset",
		}, nil
	}

	return DeleteAssetsV1ByPath204Response{}, nil
}

// PostAssetsV1ByPathConfirm confirms a file upload
func (a *API) PostAssetsV1ByPathConfirm(ctx context.Context, request PostAssetsV1ByPathConfirmRequestObject) (PostAssetsV1ByPathConfirmResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	file, err := a.assetManager.ConfirmFileUpload(ctx, request.Params.Path)
	if err != nil {
		if assets.IsNotFoundError(err) {
			return PostAssetsV1ByPathConfirm404JSONResponse{
				Code:    NotFound,
				Message: fmt.Sprintf("File at path %q not found", request.Params.Path),
			}, nil
		}
		if assets.IsNotAFileError(err) {
			return PostAssetsV1ByPathConfirm400JSONResponse{
				Code:    NotAFile,
				Message: "Asset is not a file",
			}, nil
		}
		if assets.IsAssetNotUploadedError(err) {
			return PostAssetsV1ByPathConfirm400JSONResponse{
				Code:    AssetNotUploaded,
				Message: "File has not been uploaded yet",
			}, nil
		}
		if assets.IsVersionConflictError(err) {
			return PostAssetsV1ByPathConfirm409JSONResponse{
				Code:    VersionConflict,
				Message: "File was modified by another request, please retry",
			}, nil
		}
		logger.Error("failed to confirm upload", slog.String("filepath", request.Params.Path), slog.String("error", err.Error()))
		return PostAssetsV1ByPathConfirm500JSONResponse{
			Code:    InternalError,
			Message: "Failed to confirm upload",
		}, nil
	}

	apiFile, err := fileToAPI(file, a.cdnBaseURL)
	if err != nil {
		return PostAssetsV1ByPathConfirm500JSONResponse{
			Code:    InternalError,
			Message: "Failed to confirm upload",
		}, nil
	}

	return PostAssetsV1ByPathConfirm200JSONResponse{
		File: apiFile,
	}, nil
}

// assetToAPI converts a domain asset to an API asset union type
func assetToAPI(asset assets.Asset, cdnBaseURL string) (Asset, error) {
	var apiAsset Asset

	switch a := asset.(type) {
	case *assets.File:
		file, err := fileToAPI(a, cdnBaseURL)
		if err != nil {
			return Asset{}, err
		}
		_ = apiAsset.FromFile(file)
	case *assets.Folder:
		_ = apiAsset.FromFolder(folderToAPI(a))
	}

	return apiAsset, nil
}

// fileToAPI converts a domain file to an API file
func fileToAPI(file *assets.File, cdnBaseURL string) (File, error) {
	id := openapi_types.UUID(file.ID)
	createdAt := file.CreatedAt
	url, err := file.URL(cdnBaseURL)
	if err != nil {
		return File{}, fmt.Errorf("failed to create URL for file: %w", err)
	}

	return File{
		Type:        AssetTypeFile,
		Id:          &id,
		Path:        file.Path,
		Name:        file.Name,
		Description: file.Description,
		ContentType: file.ContentType,
		Size:        file.Size,
		Url:         url,
		Status:      FileStatus(file.Status),
		CreatedAt:   &createdAt,
		CreatedBy:   &file.CreatedBy,
	}, nil
}

// folderToAPI converts a domain folder to an API folder
func folderToAPI(folder *assets.Folder) Folder {
	id := openapi_types.UUID(folder.ID)
	createdAt := folder.CreatedAt
	return Folder{
		Type:         AssetTypeFolder,
		Id:           &id,
		Path:         folder.Path,
		Name:         folder.Name,
		Description:  folder.Description,
		ContentCount: folder.ContentCount,
		CreatedAt:    &createdAt,
		CreatedBy:    &folder.CreatedBy,
	}
}
