//go:generate go tool oapi-codegen --config openapi-codegen-config.yaml ../spec/api.yaml
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/International-Combat-Archery-Alliance/assets-api/assets"
	"github.com/International-Combat-Archery-Alliance/auth/token"
	"github.com/International-Combat-Archery-Alliance/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type Environment int

const (
	LOCAL Environment = iota
	PROD
)

type API struct {
	assetManager *assets.AssetsManager
	logger       *slog.Logger
	env          Environment
	tracer       trace.Tracer
	cdnBaseURL   string
	tokenService *token.TokenService
}

var _ StrictServerInterface = (*API)(nil)

func NewAPI(
	assetsManagers *assets.AssetsManager,
	logger *slog.Logger,
	env Environment,
	cdnBaseURL string,
	tokenService *token.TokenService,
) *API {
	return &API{
		assetManager: assetsManagers,
		logger:       logger,
		env:          env,
		tracer:       otel.Tracer("github.com/International-Combat-Archery-Alliance/assets-api/api"),
		cdnBaseURL:   cdnBaseURL,
		tokenService: tokenService,
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

	// Setup CORS middleware
	corsConfig := middleware.DefaultCorsConfig()
	corsConfig.IsProduction = a.env == PROD
	corsMiddleware := middleware.CorsMiddleware(corsConfig)

	middlewares := []middleware.MiddlewareFunc{
		// Executes from the bottom up
		a.openapiValidateMiddleware(swagger),
		corsMiddleware,
		swaggerUIMiddleware,
		middleware.AccessLogging(a.logger),
		middleware.OTELHandler,
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

// isAdmin checks if the user in the context is an admin
func (a *API) isAdmin(ctx context.Context) bool {
	jwt, ok := middleware.GetJWTFromCtx(ctx)
	if !ok {
		return false
	}
	return jwt.IsAdmin()
}

// GetAssetsV1 returns all assets at a path
func (a *API) GetAssetsV1(ctx context.Context, request GetAssetsV1RequestObject) (GetAssetsV1ResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "GetAssetsV1")
	defer span.End()

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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("failed to get assets", slog.String("error", err.Error()))
		return GetAssetsV1500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get assets",
		}, nil
	}

	// Return admin response with full details if user is admin, otherwise return public response
	assets := make([]Asset, len(result.Data))
	for i, asset := range result.Data {
		item, err := assetToAPI(asset, a.cdnBaseURL, a.isAdmin(ctx))
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			logger.Error("failed to convert asset to API model", slog.String("filepath", request.Params.Path), slog.String("error", err.Error()))
			return GetAssetsV1500JSONResponse{
				Code:    InternalError,
				Message: "Failed to get asset",
			}, nil
		}

		assets[i] = *item
	}

	return GetAssetsV1200JSONResponse{
		Data:         assets,
		Cursor:       result.Cursor,
		ContentCount: result.ContentCount,
		HasNextPage:  result.HasNextPage,
	}, nil
}

// PostAssetsV1Folders creates a new folder
func (a *API) PostAssetsV1Folders(ctx context.Context, request PostAssetsV1FoldersRequestObject) (PostAssetsV1FoldersResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PostAssetsV1Folders")
	defer span.End()

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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("failed to create folder", slog.String("error", err.Error()))
		return PostAssetsV1Folders500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create folder",
		}, nil
	}

	return PostAssetsV1Folders201JSONResponse{
		Folder: folderToAdminAPI(folder),
	}, nil
}

// PostAssetsV1UploadUrl generates a presigned upload URL
func (a *API) PostAssetsV1UploadUrl(ctx context.Context, request PostAssetsV1UploadUrlRequestObject) (PostAssetsV1UploadUrlResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PostAssetsV1UploadUrl")
	defer span.End()

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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("failed to create file record", slog.String("error", err.Error()))
		return PostAssetsV1UploadUrl500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create file record",
		}, nil
	}

	// This is mega hacky and fragile, but this makes the upload url better when running locally
	if a.env == LOCAL {
		presignResult.UploadURL = strings.Replace(presignResult.UploadURL, "minio:9000", "localhost:9000", 1)
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
	ctx, span := a.tracer.Start(ctx, "GetAssetsV1ByPath")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	asset, err := a.assetManager.GetAsset(ctx, request.Params.Path)
	if err != nil {
		if assets.IsNotFoundError(err) {
			return GetAssetsV1ByPath404JSONResponse{
				Code:    NotFound,
				Message: fmt.Sprintf("Asset at path %q not found", request.Params.Path),
			}, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("failed to get asset", slog.String("filepath", request.Params.Path), slog.String("error", err.Error()))
		return GetAssetsV1ByPath500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get asset",
		}, nil
	}

	item, err := assetToAPI(asset, a.cdnBaseURL, a.isAdmin(ctx))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("failed to convert asset to API model", slog.String("filepath", request.Params.Path), slog.String("error", err.Error()))
		return GetAssetsV1ByPath500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get asset",
		}, nil

	}

	return GetAssetsV1ByPath200JSONResponse{
		Asset: *item,
	}, nil
}

// DeleteAssetsV1ByPath deletes an asset by full path
func (a *API) DeleteAssetsV1ByPath(ctx context.Context, request DeleteAssetsV1ByPathRequestObject) (DeleteAssetsV1ByPathResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "DeleteAssetsV1ByPath")
	defer span.End()

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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("failed to delete asset", slog.String("filepath", request.Params.Path), slog.String("error", err.Error()))
		return DeleteAssetsV1ByPath500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get asset",
		}, nil
	}

	return DeleteAssetsV1ByPath204Response{}, nil
}

// PostAssetsV1ByPathReplaceUrl generates a presigned URL to replace a file's contents
func (a *API) PostAssetsV1ByPathReplaceUrl(ctx context.Context, request PostAssetsV1ByPathReplaceUrlRequestObject) (PostAssetsV1ByPathReplaceUrlResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PostAssetsV1ByPathReplaceUrl")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	presignResult, file, err := a.assetManager.CreateReplaceUpload(ctx, request.Params.Path)
	if err != nil {
		if assets.IsNotFoundError(err) {
			return PostAssetsV1ByPathReplaceUrl404JSONResponse{
				Code:    NotFound,
				Message: fmt.Sprintf("File at path %q not found", request.Params.Path),
			}, nil
		}
		if assets.IsNotAFileError(err) {
			return PostAssetsV1ByPathReplaceUrl400JSONResponse{
				Code:    NotAFile,
				Message: "Asset is not a file",
			}, nil
		}
		if assets.IsFileNotConfirmedError(err) {
			return PostAssetsV1ByPathReplaceUrl400JSONResponse{
				Code:    FileNotConfirmed,
				Message: "File is not confirmed, cannot replace",
			}, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("failed to create replace upload", slog.String("filepath", request.Params.Path), slog.String("error", err.Error()))
		return PostAssetsV1ByPathReplaceUrl500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create replace upload",
		}, nil
	}

	// This is mega hacky and fragile, but this makes the upload url better when running locally
	if a.env == LOCAL {
		presignResult.UploadURL = strings.Replace(presignResult.UploadURL, "minio:9000", "localhost:9000", 1)
	}

	return PostAssetsV1ByPathReplaceUrl200JSONResponse{
		FileId:     file.ID,
		UploadUrl:  presignResult.UploadURL,
		FormFields: presignResult.FormFields,
		ExpiresAt:  presignResult.ExpiresAt,
	}, nil
}

// PostAssetsV1ByPathConfirm confirms a file upload
func (a *API) PostAssetsV1ByPathConfirm(ctx context.Context, request PostAssetsV1ByPathConfirmRequestObject) (PostAssetsV1ByPathConfirmResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PostAssetsV1ByPathConfirm")
	defer span.End()

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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("failed to confirm upload", slog.String("filepath", request.Params.Path), slog.String("error", err.Error()))
		return PostAssetsV1ByPathConfirm500JSONResponse{
			Code:    InternalError,
			Message: "Failed to confirm upload",
		}, nil
	}

	apiFile, err := fileToAdminAPI(file, a.cdnBaseURL)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("failed to convert file to admin API", slog.String("filepath", request.Params.Path), slog.String("error", err.Error()))
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
func assetToAPI(asset assets.Asset, cdnBaseURL string, isAdmin bool) (*Asset, error) {
	var apiAsset Asset

	if isAdmin {
		adminAsset, err := assetToAdminAPI(asset, cdnBaseURL)
		if err != nil {
			return nil, err
		}
		err = apiAsset.FromAdminAsset(adminAsset)
		if err != nil {
			return nil, err
		}
	} else {
		publicAsset, err := assetToPublicAPI(asset, cdnBaseURL)
		if err != nil {
			return nil, err
		}
		err = apiAsset.FromPublicAsset(publicAsset)
		if err != nil {
			return nil, err
		}

	}

	return &apiAsset, nil
}

// assetToAdminAPI converts a domain asset to an admin API asset (full details)
func assetToAdminAPI(asset assets.Asset, cdnBaseURL string) (AdminAsset, error) {
	var apiAsset AdminAsset

	switch a := asset.(type) {
	case *assets.File:
		file, err := fileToAdminAPI(a, cdnBaseURL)
		if err != nil {
			return AdminAsset{}, err
		}
		_ = apiAsset.FromAdminFile(file)
	case *assets.Folder:
		_ = apiAsset.FromAdminFolder(folderToAdminAPI(a))
	}

	return apiAsset, nil
}

// fileToAdminAPI converts a domain file to an admin API file
func fileToAdminAPI(file *assets.File, cdnBaseURL string) (AdminFile, error) {
	id := openapi_types.UUID(file.ID)
	createdAt := file.CreatedAt
	url, err := file.URL(cdnBaseURL)
	if err != nil {
		return AdminFile{}, fmt.Errorf("failed to create URL for file: %w", err)
	}

	return AdminFile{
		Type:        File,
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
		Version:     &file.Version,
	}, nil
}

// folderToAdminAPI converts a domain folder to an admin API folder
func folderToAdminAPI(folder *assets.Folder) AdminFolder {
	id := openapi_types.UUID(folder.ID)
	createdAt := folder.CreatedAt
	return AdminFolder{
		Type:         Folder,
		Id:           &id,
		Path:         folder.Path,
		Name:         folder.Name,
		Description:  folder.Description,
		ContentCount: folder.ContentCount,
		CreatedAt:    &createdAt,
		CreatedBy:    &folder.CreatedBy,
		Version:      &folder.Version,
	}
}

// assetToPublicAPI converts a domain asset to a public API asset (limited details)
func assetToPublicAPI(asset assets.Asset, cdnBaseURL string) (PublicAsset, error) {
	var apiAsset PublicAsset

	switch a := asset.(type) {
	case *assets.File:
		file, err := fileToPublicAPI(a, cdnBaseURL)
		if err != nil {
			return PublicAsset{}, err
		}
		_ = apiAsset.FromPublicFile(file)
	case *assets.Folder:
		_ = apiAsset.FromPublicFolder(folderToPublicAPI(a))
	}

	return apiAsset, nil
}

// fileToPublicAPI converts a domain file to a public API file
func fileToPublicAPI(file *assets.File, cdnBaseURL string) (PublicFile, error) {
	url, err := file.URL(cdnBaseURL)
	if err != nil {
		return PublicFile{}, fmt.Errorf("failed to create URL for file: %w", err)
	}

	return PublicFile{
		Type:        File,
		Path:        file.Path,
		Name:        file.Name,
		Description: file.Description,
		ContentType: file.ContentType,
		Size:        file.Size,
		Url:         url,
	}, nil
}

// folderToPublicAPI converts a domain folder to a public API folder
func folderToPublicAPI(folder *assets.Folder) PublicFolder {
	return PublicFolder{
		Type:         Folder,
		Path:         folder.Path,
		Name:         folder.Name,
		Description:  folder.Description,
		ContentCount: folder.ContentCount,
	}
}
