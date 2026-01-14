package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/assets-api/assets"
	"github.com/International-Combat-Archery-Alliance/auth"
	"github.com/International-Combat-Archery-Alliance/middleware"
	"github.com/google/uuid"
)

// mockStorageRepository implements assets.StorageRepository for testing
type mockStorageRepository struct {
	generateObjectKeyFunc          func(assetID uuid.UUID, filename string) string
	generatePresignedUploadURLFunc func(ctx context.Context, assetID uuid.UUID, filename string, contentType string, ttl time.Duration, maxFileSize int) (assets.PresignedUploadResult, error)
	headObjectFunc                 func(ctx context.Context, objectKey string) (assets.HeadObjectResult, error)
	deleteObjectFunc               func(ctx context.Context, objectKey string) error
}

func (m *mockStorageRepository) GenerateObjectKey(assetID uuid.UUID, filename string) string {
	if m.generateObjectKeyFunc != nil {
		return m.generateObjectKeyFunc(assetID, filename)
	}
	return assetID.String()
}

func (m *mockStorageRepository) GeneratePresignedUploadURL(ctx context.Context, assetID uuid.UUID, filename string, contentType string, ttl time.Duration, maxFileSize int) (assets.PresignedUploadResult, error) {
	if m.generatePresignedUploadURLFunc != nil {
		return m.generatePresignedUploadURLFunc(ctx, assetID, filename, contentType, ttl, maxFileSize)
	}
	return assets.PresignedUploadResult{
		UploadURL:  "https://example.com/upload",
		FormFields: map[string]string{"key": "value"},
		ExpiresAt:  time.Now().Add(ttl),
	}, nil
}

func (m *mockStorageRepository) HeadObject(ctx context.Context, objectKey string) (assets.HeadObjectResult, error) {
	if m.headObjectFunc != nil {
		return m.headObjectFunc(ctx, objectKey)
	}
	return assets.HeadObjectResult{
		Size:        1024,
		ContentType: "text/plain",
		Exists:      true,
	}, nil
}

func (m *mockStorageRepository) DeleteObject(ctx context.Context, objectKey string) error {
	if m.deleteObjectFunc != nil {
		return m.deleteObjectFunc(ctx, objectKey)
	}
	return nil
}

// mockMetadataRepository implements assets.MetadataRepository for testing
type mockMetadataRepository struct {
	getAssetFunc               func(ctx context.Context, fullPath string) (assets.Asset, error)
	getAssetsFunc              func(ctx context.Context, path string, limit int32, cursor *string) (assets.GetAssetsResponse, error)
	createAssetFunc            func(ctx context.Context, asset assets.Asset) error
	updateAssetFunc            func(ctx context.Context, asset assets.Asset) error
	deleteAssetFunc            func(ctx context.Context, fullPath string) error
	ensureRootFolderExistsFunc func(ctx context.Context, createdBy string) error
}

func (m *mockMetadataRepository) GetAsset(ctx context.Context, fullPath string) (assets.Asset, error) {
	if m.getAssetFunc != nil {
		return m.getAssetFunc(ctx, fullPath)
	}
	return nil, nil
}

func (m *mockMetadataRepository) GetAssets(ctx context.Context, path string, limit int32, cursor *string) (assets.GetAssetsResponse, error) {
	if m.getAssetsFunc != nil {
		return m.getAssetsFunc(ctx, path, limit, cursor)
	}
	return assets.GetAssetsResponse{}, nil
}

func (m *mockMetadataRepository) CreateAsset(ctx context.Context, asset assets.Asset) error {
	if m.createAssetFunc != nil {
		return m.createAssetFunc(ctx, asset)
	}
	return nil
}

func (m *mockMetadataRepository) UpdateAsset(ctx context.Context, asset assets.Asset) error {
	if m.updateAssetFunc != nil {
		return m.updateAssetFunc(ctx, asset)
	}
	return nil
}

func (m *mockMetadataRepository) DeleteAsset(ctx context.Context, fullPath string) error {
	if m.deleteAssetFunc != nil {
		return m.deleteAssetFunc(ctx, fullPath)
	}
	return nil
}

func (m *mockMetadataRepository) EnsureRootFolderExists(ctx context.Context, createdBy string) error {
	if m.ensureRootFolderExistsFunc != nil {
		return m.ensureRootFolderExistsFunc(ctx, createdBy)
	}
	return nil
}

// mockAuthToken implements auth.AuthToken for testing
type mockAuthToken struct {
	email   string
	isAdmin bool
}

func (m *mockAuthToken) ExpiresAt() time.Time {
	return time.Now().Add(time.Hour)
}

func (m *mockAuthToken) ProfilePicURL() string {
	return "https://example.com/profile.jpg"
}

func (m *mockAuthToken) IsAdmin() bool {
	return m.isAdmin
}

func (m *mockAuthToken) UserEmail() string {
	return m.email
}

// mockAuthValidator implements auth.Validator for testing
type mockAuthValidator struct{}

func (m *mockAuthValidator) Validate(ctx context.Context, token string, audience string) (auth.AuthToken, error) {
	return &mockAuthToken{
		email:   "test@example.com",
		isAdmin: true,
	}, nil
}

func TestNewAPI(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cdnBaseURL := "https://cdn.example.com"
	authValidator := &mockAuthValidator{}

	api := NewAPI(manager, logger, LOCAL, cdnBaseURL, authValidator)

	if api == nil {
		t.Fatal("NewAPI() returned nil")
	}
	if api.assetManager != manager {
		t.Error("assetManager not set correctly")
	}
	if api.logger != logger {
		t.Error("logger not set correctly")
	}
	if api.env != LOCAL {
		t.Errorf("env = %v, want %v", api.env, LOCAL)
	}
	if api.cdnBaseURL != cdnBaseURL {
		t.Errorf("cdnBaseURL = %q, want %q", api.cdnBaseURL, cdnBaseURL)
	}
}

func TestAPI_GetAssetsV1(t *testing.T) {
	fileID := uuid.New()
	folderID := uuid.New()
	expectedResponse := assets.GetAssetsResponse{
		Data: []assets.Asset{
			&assets.File{
				ID:          fileID,
				Name:        "test.txt",
				Path:        "/",
				ContentType: "text/plain",
				Size:        1024,
				ObjectKey:   fileID.String(),
				Status:      assets.StatusConfirmed,
				CreatedAt:   time.Now().UTC(),
				CreatedBy:   "user@example.com",
			},
			&assets.Folder{
				ID:           folderID,
				Name:         "folder",
				Path:         "/",
				ContentCount: 0,
				CreatedAt:    time.Now().UTC(),
				CreatedBy:    "user@example.com",
			},
		},
		Cursor:      nil,
		HasNextPage: false,
	}

	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		getAssetsFunc: func(ctx context.Context, path string, limit int32, cursor *string) (assets.GetAssetsResponse, error) {
			if path != "/" {
				t.Errorf("GetAssets called with path %q, want %q", path, "/")
			}
			if limit != 10 {
				t.Errorf("GetAssets called with limit %d, want 10", limit)
			}
			return expectedResponse, nil
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	limit := 10
	request := GetAssetsV1RequestObject{
		Params: GetAssetsV1Params{
			Path:   "/",
			Limit:  &limit,
			Cursor: nil,
		},
	}

	response, err := api.GetAssetsV1(context.Background(), request)
	if err != nil {
		t.Fatalf("GetAssetsV1() error = %v", err)
	}

	successResponse, ok := response.(GetAssetsV1200JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want GetAssetsV1200JSONResponse", response)
	}

	if len(successResponse.Data) != 2 {
		t.Errorf("response data length = %d, want 2", len(successResponse.Data))
	}

	if successResponse.HasNextPage {
		t.Error("HasNextPage should be false")
	}
}

func TestAPI_GetAssetsV1_InvalidCursor(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		getAssetsFunc: func(ctx context.Context, path string, limit int32, cursor *string) (assets.GetAssetsResponse, error) {
			return assets.GetAssetsResponse{}, assets.NewInvalidCursorError("invalid cursor", nil)
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	limit := 10
	cursor := "invalid"
	request := GetAssetsV1RequestObject{
		Params: GetAssetsV1Params{
			Path:   "/",
			Limit:  &limit,
			Cursor: &cursor,
		},
	}

	response, err := api.GetAssetsV1(context.Background(), request)
	if err != nil {
		t.Fatalf("GetAssetsV1() error = %v", err)
	}

	errorResponse, ok := response.(GetAssetsV1400JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want GetAssetsV1400JSONResponse", response)
	}

	if errorResponse.Code != InvalidCursor {
		t.Errorf("error code = %v, want %v", errorResponse.Code, InvalidCursor)
	}
}

func TestAPI_GetAssetsV1ByPath_Success(t *testing.T) {
	fileID := uuid.New()
	expectedFile := &assets.File{
		ID:          fileID,
		Name:        "test.txt",
		Path:        "/",
		ContentType: "text/plain",
		Size:        1024,
		ObjectKey:   fileID.String(),
		Status:      assets.StatusConfirmed,
		CreatedAt:   time.Now().UTC(),
		CreatedBy:   "user@example.com",
	}

	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			if fullPath != "/test.txt" {
				t.Errorf("GetAsset called with path %q, want %q", fullPath, "/test.txt")
			}
			return expectedFile, nil
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	request := GetAssetsV1ByPathRequestObject{
		Params: GetAssetsV1ByPathParams{
			Path: "/test.txt",
		},
	}

	response, err := api.GetAssetsV1ByPath(context.Background(), request)
	if err != nil {
		t.Fatalf("GetAssetsV1ByPath() error = %v", err)
	}

	successResponse, ok := response.(GetAssetsV1ByPath200JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want GetAssetsV1ByPath200JSONResponse", response)
	}

	adminAsset, err := successResponse.Asset.AsAdminAsset()
	if err != nil {
		t.Fatalf("Asset.AsAdminAsset() error = %v", err)
	}
	fileAsset, err := adminAsset.AsAdminFile()
	if err != nil {
		t.Fatalf("Asset.AsAdminFile() error = %v", err)
	}

	if fileAsset.Name != "test.txt" {
		t.Errorf("file name = %q, want %q", fileAsset.Name, "test.txt")
	}
}

func TestAPI_GetAssetsV1ByPath_NotFound(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			return nil, assets.NewNotFoundError("not found", nil)
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	request := GetAssetsV1ByPathRequestObject{
		Params: GetAssetsV1ByPathParams{
			Path: "/nonexistent.txt",
		},
	}

	response, err := api.GetAssetsV1ByPath(context.Background(), request)
	if err != nil {
		t.Fatalf("GetAssetsV1ByPath() error = %v", err)
	}

	errorResponse, ok := response.(GetAssetsV1ByPath404JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want GetAssetsV1ByPath404JSONResponse", response)
	}

	if errorResponse.Code != NotFound {
		t.Errorf("error code = %v, want %v", errorResponse.Code, NotFound)
	}
}

func TestAPI_DeleteAssetsV1ByPath_Success(t *testing.T) {
	fileID := uuid.New()
	storageRepo := &mockStorageRepository{
		deleteObjectFunc: func(ctx context.Context, objectKey string) error {
			return nil
		},
	}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			// Return a mock file for deletion
			return &assets.File{
				ID:        fileID,
				Path:      fullPath,
				Name:      "test.txt",
				ObjectKey: fileID.String(),
			}, nil
		},
		deleteAssetFunc: func(ctx context.Context, fullPath string) error {
			if fullPath != "/test.txt" {
				t.Errorf("DeleteAsset called with path %q, want %q", fullPath, "/test.txt")
			}
			return nil
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	request := DeleteAssetsV1ByPathRequestObject{
		Params: DeleteAssetsV1ByPathParams{
			Path: "/test.txt",
		},
	}

	response, err := api.DeleteAssetsV1ByPath(context.Background(), request)
	if err != nil {
		t.Fatalf("DeleteAssetsV1ByPath() error = %v", err)
	}

	_, ok := response.(DeleteAssetsV1ByPath204Response)
	if !ok {
		t.Fatalf("response type = %T, want DeleteAssetsV1ByPath204Response", response)
	}
}

func TestAPI_DeleteAssetsV1ByPath_NotFound(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			return nil, assets.NewNotFoundError("not found", nil)
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	request := DeleteAssetsV1ByPathRequestObject{
		Params: DeleteAssetsV1ByPathParams{
			Path: "/nonexistent.txt",
		},
	}

	response, err := api.DeleteAssetsV1ByPath(context.Background(), request)
	if err != nil {
		t.Fatalf("DeleteAssetsV1ByPath() error = %v", err)
	}

	errorResponse, ok := response.(DeleteAssetsV1ByPath404JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want DeleteAssetsV1ByPath404JSONResponse", response)
	}

	if errorResponse.Code != NotFound {
		t.Errorf("error code = %v, want %v", errorResponse.Code, NotFound)
	}
}

func TestAPI_DeleteAssetsV1ByPath_FolderNotEmpty(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			// Return a mock folder
			return &assets.Folder{
				ID:           uuid.New(),
				Path:         "/",
				Name:         "folder",
				ContentCount: 1, // Not empty
			}, nil
		},
		deleteAssetFunc: func(ctx context.Context, fullPath string) error {
			return assets.NewFolderNotEmptyError("folder not empty")
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	request := DeleteAssetsV1ByPathRequestObject{
		Params: DeleteAssetsV1ByPathParams{
			Path: "/folder",
		},
	}

	response, err := api.DeleteAssetsV1ByPath(context.Background(), request)
	if err != nil {
		t.Fatalf("DeleteAssetsV1ByPath() error = %v", err)
	}

	errorResponse, ok := response.(DeleteAssetsV1ByPath400JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want DeleteAssetsV1ByPath400JSONResponse", response)
	}

	if errorResponse.Code != FolderNotEmpty {
		t.Errorf("error code = %v, want %v", errorResponse.Code, FolderNotEmpty)
	}
}

func TestAPI_PostAssetsV1Folders_Success(t *testing.T) {
	folderID := uuid.New()
	createdAt := time.Now().UTC()
	desc := "Test folder"
	expectedFolder := &assets.Folder{
		ID:           folderID,
		Name:         "new-folder",
		Path:         "/",
		Description:  &desc,
		ContentCount: 0,
		CreatedAt:    createdAt,
		CreatedBy:    "admin@example.com",
		Version:      1,
	}

	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		createAssetFunc: func(ctx context.Context, asset assets.Asset) error {
			return nil
		},
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			if fullPath == "/" {
				return &assets.Folder{ID: uuid.New(), Path: "/", Name: "/", ContentCount: 0}, nil
			}
			return nil, assets.NewNotFoundError("not found", nil)
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	// Create context with admin JWT using middleware helper
	ctx := middleware.CtxWithJWT(context.Background(), &mockAuthToken{
		email:   "admin@example.com",
		isAdmin: true,
	})

	request := PostAssetsV1FoldersRequestObject{
		Body: &PostAssetsV1FoldersJSONRequestBody{
			Path:        "/",
			Name:        "new-folder",
			Description: &desc,
		},
	}

	// Mock the createAssetFunc to return the expected folder
	metadataRepo.createAssetFunc = func(ctx context.Context, asset assets.Asset) error {
		folder := asset.(*assets.Folder)
		folder.ID = expectedFolder.ID
		folder.CreatedAt = expectedFolder.CreatedAt
		return nil
	}

	response, err := api.PostAssetsV1Folders(ctx, request)
	if err != nil {
		t.Fatalf("PostAssetsV1Folders() error = %v", err)
	}

	successResponse, ok := response.(PostAssetsV1Folders201JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want PostAssetsV1Folders201JSONResponse", response)
	}

	if successResponse.Folder.Name != "new-folder" {
		t.Errorf("folder name = %q, want %q", successResponse.Folder.Name, "new-folder")
	}
}

func TestAPI_PostAssetsV1Folders_Unauthorized(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	// Context without JWT
	ctx := context.Background()

	desc := "Test folder"
	request := PostAssetsV1FoldersRequestObject{
		Body: &PostAssetsV1FoldersJSONRequestBody{
			Path:        "/",
			Name:        "new-folder",
			Description: &desc,
		},
	}

	response, err := api.PostAssetsV1Folders(ctx, request)
	if err != nil {
		t.Fatalf("PostAssetsV1Folders() error = %v", err)
	}

	errorResponse, ok := response.(PostAssetsV1Folders401JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want PostAssetsV1Folders401JSONResponse", response)
	}

	if errorResponse.Code != AuthError {
		t.Errorf("error code = %v, want %v", errorResponse.Code, AuthError)
	}
}

func TestAPI_PostAssetsV1Folders_ParentNotFound(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		createAssetFunc: func(ctx context.Context, asset assets.Asset) error {
			return assets.NewParentFolderNotFoundError("/nonexistent")
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	ctx := middleware.CtxWithJWT(context.Background(), &mockAuthToken{
		email:   "admin@example.com",
		isAdmin: true,
	})

	desc := "Test folder"
	request := PostAssetsV1FoldersRequestObject{
		Body: &PostAssetsV1FoldersJSONRequestBody{
			Path:        "/nonexistent",
			Name:        "new-folder",
			Description: &desc,
		},
	}

	response, err := api.PostAssetsV1Folders(ctx, request)
	if err != nil {
		t.Fatalf("PostAssetsV1Folders() error = %v", err)
	}

	errorResponse, ok := response.(PostAssetsV1Folders404JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want PostAssetsV1Folders404JSONResponse", response)
	}

	if errorResponse.Code != ParentFolderNotFound {
		t.Errorf("error code = %v, want %v", errorResponse.Code, ParentFolderNotFound)
	}
}

func TestAPI_PostAssetsV1Folders_AlreadyExists(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			if fullPath == "/" {
				return &assets.Folder{ID: uuid.New(), Path: "/", Name: "/"}, nil
			}
			return nil, assets.NewNotFoundError("not found", nil)
		},
		createAssetFunc: func(ctx context.Context, asset assets.Asset) error {
			return assets.NewAlreadyExistsError("already exists", nil)
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	ctx := middleware.CtxWithJWT(context.Background(), &mockAuthToken{
		email:   "admin@example.com",
		isAdmin: true,
	})

	desc := "Test folder"
	request := PostAssetsV1FoldersRequestObject{
		Body: &PostAssetsV1FoldersJSONRequestBody{
			Path:        "/",
			Name:        "existing-folder",
			Description: &desc,
		},
	}

	response, err := api.PostAssetsV1Folders(ctx, request)
	if err != nil {
		t.Fatalf("PostAssetsV1Folders() error = %v", err)
	}

	errorResponse, ok := response.(PostAssetsV1Folders409JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want PostAssetsV1Folders409JSONResponse", response)
	}

	if errorResponse.Code != AlreadyExists {
		t.Errorf("error code = %v, want %v", errorResponse.Code, AlreadyExists)
	}
}

func TestAPI_PostAssetsV1UploadUrl_Success(t *testing.T) {
	fileID := uuid.New()
	uploadURL := "https://s3.example.com/upload"
	formFields := map[string]string{"key": "value"}
	expiresAt := time.Now().Add(time.Hour)

	storageRepo := &mockStorageRepository{
		generatePresignedUploadURLFunc: func(ctx context.Context, assetID uuid.UUID, filename string, contentType string, ttl time.Duration, maxFileSize int) (assets.PresignedUploadResult, error) {
			return assets.PresignedUploadResult{
				UploadURL:  uploadURL,
				FormFields: formFields,
				ExpiresAt:  expiresAt,
			}, nil
		},
	}
	metadataRepo := &mockMetadataRepository{
		createAssetFunc: func(ctx context.Context, asset assets.Asset) error {
			file := asset.(*assets.File)
			file.ID = fileID
			return nil
		},
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			if fullPath == "/" {
				return &assets.Folder{ID: uuid.New(), Path: "/", Name: "/"}, nil
			}
			return nil, assets.NewNotFoundError("not found", nil)
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	ctx := middleware.CtxWithJWT(context.Background(), &mockAuthToken{
		email:   "admin@example.com",
		isAdmin: true,
	})

	desc := "Test file"
	request := PostAssetsV1UploadUrlRequestObject{
		Body: &PostAssetsV1UploadUrlJSONRequestBody{
			Path:        "/",
			FileName:    "test.txt",
			Description: &desc,
			ContentType: "text/plain",
		},
	}

	response, err := api.PostAssetsV1UploadUrl(ctx, request)
	if err != nil {
		t.Fatalf("PostAssetsV1UploadUrl() error = %v", err)
	}

	successResponse, ok := response.(PostAssetsV1UploadUrl200JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want PostAssetsV1UploadUrl200JSONResponse", response)
	}

	if successResponse.FileId != fileID {
		t.Errorf("file ID = %v, want %v", successResponse.FileId, fileID)
	}
	if successResponse.UploadUrl != uploadURL {
		t.Errorf("upload URL = %q, want %q", successResponse.UploadUrl, uploadURL)
	}
	if successResponse.ExpiresAt != expiresAt {
		t.Errorf("expires at = %v, want %v", successResponse.ExpiresAt, expiresAt)
	}
}

func TestAPI_PostAssetsV1UploadUrl_Unauthorized(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	ctx := context.Background()

	desc := "Test file"
	request := PostAssetsV1UploadUrlRequestObject{
		Body: &PostAssetsV1UploadUrlJSONRequestBody{
			Path:        "/",
			FileName:    "test.txt",
			Description: &desc,
			ContentType: "text/plain",
		},
	}

	response, err := api.PostAssetsV1UploadUrl(ctx, request)
	if err != nil {
		t.Fatalf("PostAssetsV1UploadUrl() error = %v", err)
	}

	errorResponse, ok := response.(PostAssetsV1UploadUrl401JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want PostAssetsV1UploadUrl401JSONResponse", response)
	}

	if errorResponse.Code != AuthError {
		t.Errorf("error code = %v, want %v", errorResponse.Code, AuthError)
	}
}

func TestAPI_PostAssetsV1UploadUrl_ParentNotFound(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		createAssetFunc: func(ctx context.Context, asset assets.Asset) error {
			return assets.NewParentFolderNotFoundError("/nonexistent")
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	ctx := middleware.CtxWithJWT(context.Background(), &mockAuthToken{
		email:   "admin@example.com",
		isAdmin: true,
	})

	desc := "Test file"
	request := PostAssetsV1UploadUrlRequestObject{
		Body: &PostAssetsV1UploadUrlJSONRequestBody{
			Path:        "/nonexistent",
			FileName:    "test.txt",
			Description: &desc,
			ContentType: "text/plain",
		},
	}

	response, err := api.PostAssetsV1UploadUrl(ctx, request)
	if err != nil {
		t.Fatalf("PostAssetsV1UploadUrl() error = %v", err)
	}

	errorResponse, ok := response.(PostAssetsV1UploadUrl404JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want PostAssetsV1UploadUrl404JSONResponse", response)
	}

	if errorResponse.Code != ParentFolderNotFound {
		t.Errorf("error code = %v, want %v", errorResponse.Code, ParentFolderNotFound)
	}
}

func TestAPI_PostAssetsV1ByPathConfirm_Success(t *testing.T) {
	fileID := uuid.New()
	createdAt := time.Now().UTC()
	desc := "Test file"

	storageRepo := &mockStorageRepository{
		headObjectFunc: func(ctx context.Context, objectKey string) (assets.HeadObjectResult, error) {
			return assets.HeadObjectResult{
				Size:        1024,
				ContentType: "text/plain",
				Exists:      true,
			}, nil
		},
	}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			return &assets.File{
				ID:          fileID,
				Name:        "test.txt",
				Path:        "/",
				Description: &desc,
				ContentType: "text/plain",
				Size:        0,
				ObjectKey:   fileID.String(),
				Status:      assets.StatusPending,
				CreatedAt:   createdAt,
				CreatedBy:   "admin@example.com",
				Version:     0,
			}, nil
		},
		updateAssetFunc: func(ctx context.Context, asset assets.Asset) error {
			return nil
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	request := PostAssetsV1ByPathConfirmRequestObject{
		Params: PostAssetsV1ByPathConfirmParams{
			Path: "/test.txt",
		},
	}

	response, err := api.PostAssetsV1ByPathConfirm(context.Background(), request)
	if err != nil {
		t.Fatalf("PostAssetsV1ByPathConfirm() error = %v", err)
	}

	successResponse, ok := response.(PostAssetsV1ByPathConfirm200JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want PostAssetsV1ByPathConfirm200JSONResponse", response)
	}

	if successResponse.File.Name != "test.txt" {
		t.Errorf("file name = %q, want %q", successResponse.File.Name, "test.txt")
	}
	if successResponse.File.Status != Confirmed {
		t.Errorf("file status = %v, want %v", successResponse.File.Status, Confirmed)
	}
}

func TestAPI_PostAssetsV1ByPathConfirm_NotFound(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			return nil, assets.NewNotFoundError("not found", nil)
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	request := PostAssetsV1ByPathConfirmRequestObject{
		Params: PostAssetsV1ByPathConfirmParams{
			Path: "/nonexistent.txt",
		},
	}

	response, err := api.PostAssetsV1ByPathConfirm(context.Background(), request)
	if err != nil {
		t.Fatalf("PostAssetsV1ByPathConfirm() error = %v", err)
	}

	errorResponse, ok := response.(PostAssetsV1ByPathConfirm404JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want PostAssetsV1ByPathConfirm404JSONResponse", response)
	}

	if errorResponse.Code != NotFound {
		t.Errorf("error code = %v, want %v", errorResponse.Code, NotFound)
	}
}

func TestAPI_PostAssetsV1ByPathConfirm_NotAFile(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			return &assets.Folder{
				ID:   uuid.New(),
				Name: "folder",
				Path: "/",
			}, nil
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	request := PostAssetsV1ByPathConfirmRequestObject{
		Params: PostAssetsV1ByPathConfirmParams{
			Path: "/folder",
		},
	}

	response, err := api.PostAssetsV1ByPathConfirm(context.Background(), request)
	if err != nil {
		t.Fatalf("PostAssetsV1ByPathConfirm() error = %v", err)
	}

	errorResponse, ok := response.(PostAssetsV1ByPathConfirm400JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want PostAssetsV1ByPathConfirm400JSONResponse", response)
	}

	if errorResponse.Code != NotAFile {
		t.Errorf("error code = %v, want %v", errorResponse.Code, NotAFile)
	}
}

func TestAPI_PostAssetsV1ByPathConfirm_AssetNotUploaded(t *testing.T) {
	fileID := uuid.New()
	storageRepo := &mockStorageRepository{
		headObjectFunc: func(ctx context.Context, objectKey string) (assets.HeadObjectResult, error) {
			return assets.HeadObjectResult{
				Exists: false,
			}, nil
		},
	}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			return &assets.File{
				ID:          fileID,
				Name:        "test.txt",
				Path:        "/",
				ContentType: "text/plain",
				Size:        0,
				ObjectKey:   fileID.String(),
				Status:      assets.StatusPending,
				CreatedAt:   time.Now().UTC(),
				CreatedBy:   "admin@example.com",
			}, nil
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	request := PostAssetsV1ByPathConfirmRequestObject{
		Params: PostAssetsV1ByPathConfirmParams{
			Path: "/test.txt",
		},
	}

	response, err := api.PostAssetsV1ByPathConfirm(context.Background(), request)
	if err != nil {
		t.Fatalf("PostAssetsV1ByPathConfirm() error = %v", err)
	}

	errorResponse, ok := response.(PostAssetsV1ByPathConfirm400JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want PostAssetsV1ByPathConfirm400JSONResponse", response)
	}

	if errorResponse.Code != AssetNotUploaded {
		t.Errorf("error code = %v, want %v", errorResponse.Code, AssetNotUploaded)
	}
}

func TestAPI_PostAssetsV1ByPathConfirm_VersionConflict(t *testing.T) {
	fileID := uuid.New()
	storageRepo := &mockStorageRepository{
		headObjectFunc: func(ctx context.Context, objectKey string) (assets.HeadObjectResult, error) {
			return assets.HeadObjectResult{
				Size:        1024,
				ContentType: "text/plain",
				Exists:      true,
			}, nil
		},
	}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			return &assets.File{
				ID:          fileID,
				Name:        "test.txt",
				Path:        "/",
				ContentType: "text/plain",
				Size:        0,
				ObjectKey:   fileID.String(),
				Status:      assets.StatusPending,
				CreatedAt:   time.Now().UTC(),
				CreatedBy:   "admin@example.com",
			}, nil
		},
		updateAssetFunc: func(ctx context.Context, asset assets.Asset) error {
			return assets.NewVersionConflictError("version conflict")
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	request := PostAssetsV1ByPathConfirmRequestObject{
		Params: PostAssetsV1ByPathConfirmParams{
			Path: "/test.txt",
		},
	}

	response, err := api.PostAssetsV1ByPathConfirm(context.Background(), request)
	if err != nil {
		t.Fatalf("PostAssetsV1ByPathConfirm() error = %v", err)
	}

	errorResponse, ok := response.(PostAssetsV1ByPathConfirm409JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want PostAssetsV1ByPathConfirm409JSONResponse", response)
	}

	if errorResponse.Code != VersionConflict {
		t.Errorf("error code = %v, want %v", errorResponse.Code, VersionConflict)
	}
}

func TestAPI_GetAdminEmailFromCtx_Success(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	ctx := middleware.CtxWithJWT(context.Background(), &mockAuthToken{
		email:   "admin@example.com",
		isAdmin: true,
	})

	email, err := api.getAdminEmailFromCtx(ctx)
	if err != nil {
		t.Fatalf("getAdminEmailFromCtx() error = %v", err)
	}

	if email != "admin@example.com" {
		t.Errorf("email = %q, want %q", email, "admin@example.com")
	}
}

func TestAPI_GetAdminEmailFromCtx_NoJWT(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	ctx := context.Background()

	_, err := api.getAdminEmailFromCtx(ctx)
	if err == nil {
		t.Fatal("getAdminEmailFromCtx() should return error when no JWT in context")
	}
}

func TestAPI_GetAdminEmailFromCtx_NotAdmin(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	ctx := middleware.CtxWithJWT(context.Background(), &mockAuthToken{
		email:   "user@example.com",
		isAdmin: false,
	})

	_, err := api.getAdminEmailFromCtx(ctx)
	if err == nil {
		t.Fatal("getAdminEmailFromCtx() should return error when user is not admin")
	}
}

func TestAPI_GetAssetsV1_InternalError(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		getAssetsFunc: func(ctx context.Context, path string, limit int32, cursor *string) (assets.GetAssetsResponse, error) {
			return assets.GetAssetsResponse{}, fmt.Errorf("database error")
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	limit := 10
	request := GetAssetsV1RequestObject{
		Params: GetAssetsV1Params{
			Path:   "/",
			Limit:  &limit,
			Cursor: nil,
		},
	}

	response, err := api.GetAssetsV1(context.Background(), request)
	if err != nil {
		t.Fatalf("GetAssetsV1() error = %v", err)
	}

	errorResponse, ok := response.(GetAssetsV1500JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want GetAssetsV1500JSONResponse", response)
	}

	if errorResponse.Code != InternalError {
		t.Errorf("error code = %v, want %v", errorResponse.Code, InternalError)
	}
}

func TestAPI_GetAssetsV1ByPath_InternalError(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	request := GetAssetsV1ByPathRequestObject{
		Params: GetAssetsV1ByPathParams{
			Path: "/test.txt",
		},
	}

	response, err := api.GetAssetsV1ByPath(context.Background(), request)
	if err != nil {
		t.Fatalf("GetAssetsV1ByPath() error = %v", err)
	}

	errorResponse, ok := response.(GetAssetsV1ByPath500JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want GetAssetsV1ByPath500JSONResponse", response)
	}

	if errorResponse.Code != InternalError {
		t.Errorf("error code = %v, want %v", errorResponse.Code, InternalError)
	}
}

func TestAPI_DeleteAssetsV1ByPath_InternalError(t *testing.T) {
	storageRepo := &mockStorageRepository{}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			return &assets.File{
				ID:   uuid.New(),
				Path: fullPath,
				Name: "test.txt",
			}, nil
		},
		deleteAssetFunc: func(ctx context.Context, fullPath string) error {
			return fmt.Errorf("database error")
		},
	}
	manager := assets.NewAssetsManager(storageRepo, metadataRepo)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", &mockAuthValidator{})

	request := DeleteAssetsV1ByPathRequestObject{
		Params: DeleteAssetsV1ByPathParams{
			Path: "/test.txt",
		},
	}

	response, err := api.DeleteAssetsV1ByPath(context.Background(), request)
	if err != nil {
		t.Fatalf("DeleteAssetsV1ByPath() error = %v", err)
	}

	errorResponse, ok := response.(DeleteAssetsV1ByPath500JSONResponse)
	if !ok {
		t.Fatalf("response type = %T, want DeleteAssetsV1ByPath500JSONResponse", response)
	}

	if errorResponse.Code != InternalError {
		t.Errorf("error code = %v, want %v", errorResponse.Code, InternalError)
	}
}
