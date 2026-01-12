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
	"github.com/google/uuid"
)

// mockStorageRepository implements assets.StorageRepository for testing
type mockStorageRepository struct {
	generateObjectKeyFunc          func(assetID uuid.UUID) string
	generatePresignedUploadURLFunc func(ctx context.Context, assetID uuid.UUID, contentType string, ttl time.Duration, maxFileSize int) (assets.PresignedUploadResult, error)
	headObjectFunc                 func(ctx context.Context, assetID uuid.UUID) (assets.HeadObjectResult, error)
	deleteObjectFunc               func(ctx context.Context, assetID uuid.UUID) error
}

func (m *mockStorageRepository) GenerateObjectKey(assetID uuid.UUID) string {
	if m.generateObjectKeyFunc != nil {
		return m.generateObjectKeyFunc(assetID)
	}
	return assetID.String()
}

func (m *mockStorageRepository) GeneratePresignedUploadURL(ctx context.Context, assetID uuid.UUID, contentType string, ttl time.Duration, maxFileSize int) (assets.PresignedUploadResult, error) {
	if m.generatePresignedUploadURLFunc != nil {
		return m.generatePresignedUploadURLFunc(ctx, assetID, contentType, ttl, maxFileSize)
	}
	return assets.PresignedUploadResult{
		UploadURL:  "https://example.com/upload",
		FormFields: map[string]string{"key": "value"},
		ExpiresAt:  time.Now().Add(ttl),
	}, nil
}

func (m *mockStorageRepository) HeadObject(ctx context.Context, assetID uuid.UUID) (assets.HeadObjectResult, error) {
	if m.headObjectFunc != nil {
		return m.headObjectFunc(ctx, assetID)
	}
	return assets.HeadObjectResult{
		Size:        1024,
		ContentType: "text/plain",
		Exists:      true,
	}, nil
}

func (m *mockStorageRepository) DeleteObject(ctx context.Context, assetID uuid.UUID) error {
	if m.deleteObjectFunc != nil {
		return m.deleteObjectFunc(ctx, assetID)
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

	fileAsset, err := successResponse.Asset.AsFile()
	if err != nil {
		t.Fatalf("Asset.AsFile() error = %v", err)
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

func TestFileToAPI(t *testing.T) {
	fileID := uuid.New()
	createdAt := time.Now().UTC()
	desc := "Test file"

	file := &assets.File{
		ID:          fileID,
		Name:        "test.txt",
		Path:        "/",
		Description: &desc,
		ContentType: "text/plain",
		Size:        1024,
		ObjectKey:   fileID.String(),
		Status:      assets.StatusConfirmed,
		CreatedAt:   createdAt,
		CreatedBy:   "user@example.com",
	}

	cdnBaseURL := "https://cdn.example.com"
	apiFile, err := fileToAPI(file, cdnBaseURL)
	if err != nil {
		t.Fatalf("fileToAPI() error = %v", err)
	}

	if apiFile.Name != "test.txt" {
		t.Errorf("Name = %q, want %q", apiFile.Name, "test.txt")
	}
	if apiFile.Path != "/" {
		t.Errorf("Path = %q, want %q", apiFile.Path, "/")
	}
	if apiFile.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want %q", apiFile.ContentType, "text/plain")
	}
	if apiFile.Size != 1024 {
		t.Errorf("Size = %d, want 1024", apiFile.Size)
	}
	if apiFile.Status != Confirmed {
		t.Errorf("Status = %v, want %v", apiFile.Status, Confirmed)
	}
	if apiFile.Url == "" {
		t.Error("Url should not be empty")
	}
	expectedURL := fmt.Sprintf("%s/%s", cdnBaseURL, fileID.String())
	if apiFile.Url != expectedURL {
		t.Errorf("Url = %q, want %q", apiFile.Url, expectedURL)
	}
}

func TestFolderToAPI(t *testing.T) {
	folderID := uuid.New()
	createdAt := time.Now().UTC()
	desc := "Test folder"

	folder := &assets.Folder{
		ID:           folderID,
		Name:         "test-folder",
		Path:         "/",
		Description:  &desc,
		ContentCount: 5,
		CreatedAt:    createdAt,
		CreatedBy:    "user@example.com",
	}

	apiFolder := folderToAPI(folder)

	if apiFolder.Name != "test-folder" {
		t.Errorf("Name = %q, want %q", apiFolder.Name, "test-folder")
	}
	if apiFolder.Path != "/" {
		t.Errorf("Path = %q, want %q", apiFolder.Path, "/")
	}
	if apiFolder.ContentCount != 5 {
		t.Errorf("ContentCount = %d, want 5", apiFolder.ContentCount)
	}
	if apiFolder.Description == nil || *apiFolder.Description != desc {
		t.Errorf("Description = %v, want %q", apiFolder.Description, desc)
	}
}

func TestAssetToAPI_File(t *testing.T) {
	fileID := uuid.New()
	file := &assets.File{
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

	cdnBaseURL := "https://cdn.example.com"
	apiAsset, err := assetToAPI(file, cdnBaseURL)
	if err != nil {
		t.Fatalf("assetToAPI() error = %v", err)
	}

	fileAsset, err := apiAsset.AsFile()
	if err != nil {
		t.Fatalf("AsFile() error = %v", err)
	}

	if fileAsset.Name != "test.txt" {
		t.Errorf("Name = %q, want %q", fileAsset.Name, "test.txt")
	}
}

func TestAssetToAPI_Folder(t *testing.T) {
	folderID := uuid.New()
	folder := &assets.Folder{
		ID:           folderID,
		Name:         "test-folder",
		Path:         "/",
		ContentCount: 3,
		CreatedAt:    time.Now().UTC(),
		CreatedBy:    "user@example.com",
	}

	cdnBaseURL := "https://cdn.example.com"
	apiAsset, err := assetToAPI(folder, cdnBaseURL)
	if err != nil {
		t.Fatalf("assetToAPI() error = %v", err)
	}

	folderAsset, err := apiAsset.AsFolder()
	if err != nil {
		t.Fatalf("AsFolder() error = %v", err)
	}

	if folderAsset.Name != "test-folder" {
		t.Errorf("Name = %q, want %q", folderAsset.Name, "test-folder")
	}
}

func TestAPI_DeleteAssetsV1ByPath_Success(t *testing.T) {
	storageRepo := &mockStorageRepository{
		deleteObjectFunc: func(ctx context.Context, assetID uuid.UUID) error {
			return nil
		},
	}
	metadataRepo := &mockMetadataRepository{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			// Return a mock file for deletion
			return &assets.File{
				ID:   uuid.New(),
				Path: fullPath,
				Name: "test.txt",
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
