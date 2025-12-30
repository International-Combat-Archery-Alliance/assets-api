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
	"github.com/International-Combat-Archery-Alliance/assets-api/ptr"
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
	db            assets.Repository
	storage       *s3storage.Storage
	logger        *slog.Logger
	env           Environment
	cdnBaseURL    string
	authValidator auth.Validator
}

var _ StrictServerInterface = (*API)(nil)

func NewAPI(
	db assets.Repository,
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

// GetAssetsV1 returns all assets, optionally filtered by folder
func (a *API) GetAssetsV1(ctx context.Context, request GetAssetsV1RequestObject) (GetAssetsV1ResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	limit := int32(20)
	if request.Params.Limit != nil {
		limit = int32(*request.Params.Limit)
	}

	result, err := a.db.GetAssets(ctx, request.Params.Folder, limit, request.Params.Cursor)
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

// GetAssetsV1Folders returns all distinct folder names
func (a *API) GetAssetsV1Folders(ctx context.Context, _ GetAssetsV1FoldersRequestObject) (GetAssetsV1FoldersResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	folders, err := a.db.GetFolders(ctx)
	if err != nil {
		logger.Error("failed to get folders", slog.String("error", err.Error()))
		return GetAssetsV1Folders500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get folders",
		}, nil
	}

	return GetAssetsV1Folders200JSONResponse{
		Folders: folders,
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

	// Generate asset ID
	assetID := uuid.New()

	// Generate S3 key
	s3Key := s3storage.GenerateS3Key(request.Body.Folder, assetID, request.Body.FileName)

	// Generate presigned URL
	presignResult, err := a.storage.GeneratePresignedUploadURL(ctx, s3Key, request.Body.ContentType)
	if err != nil {
		logger.Error("failed to generate presigned URL", slog.String("error", err.Error()))
		return PostAssetsV1UploadUrl500JSONResponse{
			Code:    InternalError,
			Message: "Failed to generate upload URL",
		}, nil
	}

	// Create pending asset record
	now := time.Now().UTC()
	asset := assets.Asset{
		ID:          assetID,
		Folder:      request.Body.Folder,
		Name:        request.Body.FileName,
		Description: request.Body.Description,
		ContentType: request.Body.ContentType,
		Size:        0, // Will be updated on confirm
		S3Key:       s3Key,
		Status:      assets.StatusPending,
		CreatedAt:   now,
		CreatedBy:   userEmail,
	}

	if err := a.db.CreateAsset(ctx, asset); err != nil {
		logger.Error("failed to create asset record", slog.String("error", err.Error()))
		return PostAssetsV1UploadUrl500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create asset record",
		}, nil
	}

	// Check if this is the first asset in the folder
	count, err := a.db.CountAssetsInFolder(ctx, request.Body.Folder)
	if err != nil {
		logger.Warn("failed to count assets in folder", slog.String("error", err.Error()))
	} else if count == 1 {
		// This is the first asset in the folder, add to folder index
		if err := a.db.AddFolder(ctx, request.Body.Folder); err != nil {
			logger.Warn("failed to add folder to index", slog.String("error", err.Error()))
		}
	}

	return PostAssetsV1UploadUrl200JSONResponse{
		AssetId:   assetID,
		UploadUrl: presignResult.UploadURL,
		ExpiresAt: presignResult.ExpiresAt,
	}, nil
}

// GetAssetsV1Id returns a single asset by ID
func (a *API) GetAssetsV1Id(ctx context.Context, request GetAssetsV1IdRequestObject) (GetAssetsV1IdResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	asset, err := a.db.GetAsset(ctx, request.Id)
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

	// Validate admin auth (already done by middleware, but double check)
	_, err := a.getAdminEmailFromCtx(ctx)
	if err != nil {
		return DeleteAssetsV1Id401JSONResponse{
			Code:    AuthError,
			Message: "Unauthorized",
		}, nil
	}

	// Get asset first to get folder and S3 key
	asset, err := a.db.GetAsset(ctx, request.Id)
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

	// Delete from S3
	if err := a.storage.DeleteObject(ctx, asset.S3Key); err != nil {
		logger.Error("failed to delete from S3", slog.String("error", err.Error()))
		return DeleteAssetsV1Id500JSONResponse{
			Code:    InternalError,
			Message: "Failed to delete asset from storage",
		}, nil
	}

	// Delete from DynamoDB
	if err := a.db.DeleteAsset(ctx, request.Id); err != nil {
		logger.Error("failed to delete asset from DB", slog.String("error", err.Error()))
		return DeleteAssetsV1Id500JSONResponse{
			Code:    InternalError,
			Message: "Failed to delete asset record",
		}, nil
	}

	// Check if this was the last asset in the folder
	count, err := a.db.CountAssetsInFolder(ctx, asset.Folder)
	if err != nil {
		logger.Warn("failed to count assets in folder", slog.String("error", err.Error()))
	} else if count == 0 {
		// This was the last asset in the folder, remove from folder index
		if err := a.db.RemoveFolder(ctx, asset.Folder); err != nil {
			logger.Warn("failed to remove folder from index", slog.String("error", err.Error()))
		}
	}

	return DeleteAssetsV1Id204Response{}, nil
}

// PostAssetsV1IdConfirm confirms an asset upload
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
	asset, err := a.db.GetAsset(ctx, request.Id)
	if err != nil {
		if assets.IsNotFoundError(err) {
			return PostAssetsV1IdConfirm404JSONResponse{
				Code:    NotFound,
				Message: fmt.Sprintf("Asset %s not found", request.Id),
			}, nil
		}
		logger.Error("failed to get asset", slog.String("error", err.Error()))
		return PostAssetsV1IdConfirm500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get asset",
		}, nil
	}

	if asset.Status == assets.StatusConfirmed {
		// Already confirmed, just return it
		return PostAssetsV1IdConfirm200JSONResponse{
			Asset: assetToAPI(asset, a.cdnBaseURL),
		}, nil
	}

	// Verify the file exists in S3
	headResult, err := a.storage.HeadObject(ctx, asset.S3Key)
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
			Message: "Asset has not been uploaded yet",
		}, nil
	}

	// Check file size
	if headResult.Size > s3storage.MaxUploadSize {
		// Delete the oversized file
		_ = a.storage.DeleteObject(ctx, asset.S3Key)
		_ = a.db.DeleteAsset(ctx, asset.ID)
		return PostAssetsV1IdConfirm400JSONResponse{
			Code:    FileTooLarge,
			Message: fmt.Sprintf("File size %d exceeds maximum allowed size of %d bytes", headResult.Size, s3storage.MaxUploadSize),
		}, nil
	}

	// Update asset with file size and status
	asset.Size = headResult.Size
	asset.Status = assets.StatusConfirmed

	if err := a.db.UpdateAsset(ctx, asset); err != nil {
		logger.Error("failed to update asset", slog.String("error", err.Error()))
		return PostAssetsV1IdConfirm500JSONResponse{
			Code:    InternalError,
			Message: "Failed to confirm asset",
		}, nil
	}

	return PostAssetsV1IdConfirm200JSONResponse{
		Asset: assetToAPI(asset, a.cdnBaseURL),
	}, nil
}

// Helper to convert domain asset to API asset
func assetToAPI(asset assets.Asset, cdnBaseURL string) Asset {
	id := openapi_types.UUID(asset.ID)
	createdAt := asset.CreatedAt
	return Asset{
		Id:          &id,
		Folder:      asset.Folder,
		Name:        asset.Name,
		Description: asset.Description,
		ContentType: asset.ContentType,
		Size:        asset.Size,
		Url:         asset.URL(cdnBaseURL),
		Status:      AssetStatus(asset.Status),
		CreatedAt:   &createdAt,
		CreatedBy:   ptr.String(asset.CreatedBy),
	}
}
