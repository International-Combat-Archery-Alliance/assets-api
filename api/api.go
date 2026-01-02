//go:generate go tool oapi-codegen --config openapi-codegen-config.yaml ../spec/api.yaml
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/International-Combat-Archery-Alliance/assets-api/assets"
	s3storage "github.com/International-Combat-Archery-Alliance/assets-api/s3"
	"github.com/International-Combat-Archery-Alliance/auth"
	"github.com/International-Combat-Archery-Alliance/middleware"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type Environment int

const (
	LOCAL Environment = iota
	PROD
)

type API struct {
	db            assets.MetadataRepository
	storage       assets.StorageRepository
	logger        *slog.Logger
	env           Environment
	cdnBaseURL    string
	authValidator auth.Validator
}

var _ StrictServerInterface = (*API)(nil)

func NewAPI(
	db assets.MetadataRepository,
	storage *s3storage.Storage,
	logger *slog.Logger,
	env Environment,
	cdnBaseURL string,
	authValidator auth.Validator,
) *API {
	return &API{
		db:            db,
		storage:       storage,
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

// getAdminEmailFromCtx gets the admin email from the JWT in context
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

	limit := int32(20)
	if request.Params.Limit != nil {
		limit = int32(*request.Params.Limit)
	}

	result, err := a.db.GetAssets(ctx, request.Params.Path, limit, request.Params.Cursor)
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
		apiAssets[i] = assetToAPI(asset, a.cdnBaseURL)
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

	// Get admin email from JWT
	userEmail, err := a.getAdminEmailFromCtx(ctx)
	if err != nil {
		return PostAssetsV1Folders401JSONResponse{
			Code:    AuthError,
			Message: "Unauthorized",
		}, nil
	}

	folderID := uuid.New()
	now := time.Now().UTC()

	folder := &assets.Folder{
		ID:           folderID,
		Path:         request.Body.Path,
		Name:         request.Body.Name,
		Description:  request.Body.Description,
		ContentCount: 0,
		CreatedAt:    now,
		CreatedBy:    userEmail,
	}

	if err := a.db.CreateAsset(ctx, folder); err != nil {
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

	fileID := uuid.New()

	objectKey := a.storage.GenerateObjectKey(fileID)

	presignResult, err := a.storage.GeneratePresignedUploadURL(ctx, fileID, request.Body.ContentType)
	if err != nil {
		logger.Error("failed to generate presigned URL", slog.String("error", err.Error()))
		return PostAssetsV1UploadUrl500JSONResponse{
			Code:    InternalError,
			Message: "Failed to generate upload URL",
		}, nil
	}

	// Create pending file record
	now := time.Now().UTC()
	file := &assets.File{
		ID:          fileID,
		Path:        request.Body.Path,
		Name:        request.Body.FileName,
		Description: request.Body.Description,
		ContentType: request.Body.ContentType,
		Size:        0, // Will be updated on confirm
		ObjectKey:   objectKey,
		Status:      assets.StatusPending,
		CreatedAt:   now,
		CreatedBy:   userEmail,
	}

	if err := a.db.CreateAsset(ctx, file); err != nil {
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
		FileId:    fileID,
		UploadUrl: presignResult.UploadURL,
		ExpiresAt: presignResult.ExpiresAt,
	}, nil
}

// GetAssetsV1Id returns a single asset by ID
func (a *API) GetAssetsV1Id(ctx context.Context, request GetAssetsV1IdRequestObject) (GetAssetsV1IdResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	asset, err := a.db.GetAsset(ctx, uuid.UUID(request.Id))
	if err != nil {
		if assets.IsNotFoundError(err) {
			return GetAssetsV1Id404JSONResponse{
				Code:    NotFound,
				Message: fmt.Sprintf("Asset %s not found", request.Id),
			}, nil
		}
		logger.Error("failed to get asset", slog.String("error", err.Error()))
		return GetAssetsV1Id500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get asset",
		}, nil
	}

	return GetAssetsV1Id200JSONResponse{
		Asset: assetToAPI(asset, a.cdnBaseURL),
	}, nil
}

// DeleteAssetsV1Id deletes an asset by ID
func (a *API) DeleteAssetsV1Id(ctx context.Context, request DeleteAssetsV1IdRequestObject) (DeleteAssetsV1IdResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	// Validate admin auth
	_, err := a.getAdminEmailFromCtx(ctx)
	if err != nil {
		return DeleteAssetsV1Id401JSONResponse{
			Code:    AuthError,
			Message: "Unauthorized",
		}, nil
	}

	// Get asset first to determine type and get S3 key if it's a file
	asset, err := a.db.GetAsset(ctx, uuid.UUID(request.Id))
	if err != nil {
		if assets.IsNotFoundError(err) {
			return DeleteAssetsV1Id404JSONResponse{
				Code:    NotFound,
				Message: fmt.Sprintf("Asset %s not found", request.Id),
			}, nil
		}
		logger.Error("failed to get asset", slog.String("error", err.Error()))
		return DeleteAssetsV1Id500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get asset",
		}, nil
	}

	// If it's a file, delete from S3 first
	if file, ok := asset.(*assets.File); ok {
		if err := a.storage.DeleteObject(ctx, file.ID); err != nil {
			logger.Error("failed to delete from S3", slog.String("error", err.Error()))
			return DeleteAssetsV1Id500JSONResponse{
				Code:    InternalError,
				Message: "Failed to delete file from storage",
			}, nil
		}
	}

	// Delete from DynamoDB
	if err := a.db.DeleteAsset(ctx, uuid.UUID(request.Id)); err != nil {
		if assets.IsFolderNotEmptyError(err) {
			return DeleteAssetsV1Id400JSONResponse{
				Code:    FolderNotEmpty,
				Message: "Cannot delete folder: folder is not empty",
			}, nil
		}
		logger.Error("failed to delete asset from DB", slog.String("error", err.Error()))
		return DeleteAssetsV1Id500JSONResponse{
			Code:    InternalError,
			Message: "Failed to delete asset record",
		}, nil
	}

	return DeleteAssetsV1Id204Response{}, nil
}

// PostAssetsV1IdConfirm confirms a file upload
func (a *API) PostAssetsV1IdConfirm(ctx context.Context, request PostAssetsV1IdConfirmRequestObject) (PostAssetsV1IdConfirmResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	// Validate admin auth
	_, err := a.getAdminEmailFromCtx(ctx)
	if err != nil {
		return PostAssetsV1IdConfirm401JSONResponse{
			Code:    AuthError,
			Message: "Unauthorized",
		}, nil
	}

	// Get the pending asset
	asset, err := a.db.GetAsset(ctx, uuid.UUID(request.Id))
	if err != nil {
		if assets.IsNotFoundError(err) {
			return PostAssetsV1IdConfirm404JSONResponse{
				Code:    NotFound,
				Message: fmt.Sprintf("File %s not found", request.Id),
			}, nil
		}
		logger.Error("failed to get asset", slog.String("error", err.Error()))
		return PostAssetsV1IdConfirm500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get file",
		}, nil
	}

	// Ensure it's a file
	file, ok := asset.(*assets.File)
	if !ok {
		return PostAssetsV1IdConfirm400JSONResponse{
			Code:    NotAFile,
			Message: "Asset is not a file",
		}, nil
	}

	if file.Status == assets.StatusConfirmed {
		// Already confirmed, just return it
		return PostAssetsV1IdConfirm200JSONResponse{
			File: fileToAPI(file, a.cdnBaseURL),
		}, nil
	}

	// Verify the file exists in S3
	headResult, err := a.storage.HeadObject(ctx, file.ID)
	if err != nil {
		logger.Error("failed to head S3 object", slog.String("error", err.Error()))
		return PostAssetsV1IdConfirm500JSONResponse{
			Code:    InternalError,
			Message: "Failed to verify upload",
		}, nil
	}

	if !headResult.Exists {
		return PostAssetsV1IdConfirm400JSONResponse{
			Code:    AssetNotUploaded,
			Message: "File has not been uploaded yet",
		}, nil
	}

	// Check file size
	if headResult.Size > s3storage.MaxUploadSize {
		// Delete the oversized file
		_ = a.storage.DeleteObject(ctx, file.ID)
		_ = a.db.DeleteAsset(ctx, file.ID)
		return PostAssetsV1IdConfirm400JSONResponse{
			Code:    FileTooLarge,
			Message: fmt.Sprintf("File size %d exceeds maximum allowed size of %d bytes", headResult.Size, s3storage.MaxUploadSize),
		}, nil
	}

	// Update file with size, status, and incremented version
	updatedFile := &assets.File{
		ID:          file.ID,
		Path:        file.Path,
		Name:        file.Name,
		Description: file.Description,
		ContentType: file.ContentType,
		Size:        headResult.Size,
		ObjectKey:   file.ObjectKey,
		Status:      assets.StatusConfirmed,
		CreatedAt:   file.CreatedAt,
		CreatedBy:   file.CreatedBy,
		Version:     file.Version + 1, // Increment version for optimistic locking
	}

	if err := a.db.UpdateAsset(ctx, updatedFile); err != nil {
		if assets.IsVersionConflictError(err) {
			return PostAssetsV1IdConfirm409JSONResponse{
				Code:    VersionConflict,
				Message: "File was modified by another request, please retry",
			}, nil
		}
		logger.Error("failed to update file", slog.String("error", err.Error()))
		return PostAssetsV1IdConfirm500JSONResponse{
			Code:    InternalError,
			Message: "Failed to confirm file",
		}, nil
	}

	return PostAssetsV1IdConfirm200JSONResponse{
		File: fileToAPI(updatedFile, a.cdnBaseURL),
	}, nil
}

// assetToAPI converts a domain asset to an API asset union type
func assetToAPI(asset assets.Asset, cdnBaseURL string) Asset {
	var apiAsset Asset

	switch a := asset.(type) {
	case *assets.File:
		_ = apiAsset.FromFile(fileToAPI(a, cdnBaseURL))
	case *assets.Folder:
		_ = apiAsset.FromFolder(folderToAPI(a))
	}

	return apiAsset
}

// fileToAPI converts a domain file to an API file
func fileToAPI(file *assets.File, cdnBaseURL string) File {
	id := openapi_types.UUID(file.ID)
	createdAt := file.CreatedAt
	return File{
		Type:        AssetTypeFile,
		Id:          &id,
		Path:        file.Path,
		Name:        file.Name,
		Description: file.Description,
		ContentType: file.ContentType,
		Size:        file.Size,
		Url:         file.URL(cdnBaseURL),
		Status:      FileStatus(file.Status),
		CreatedAt:   &createdAt,
		CreatedBy:   &file.CreatedBy,
	}
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
