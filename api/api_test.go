package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/assets-api/assets"
	"github.com/google/uuid"
)

type mockAssetsManager struct {
	getAssetFunc           func(ctx context.Context, fullPath string) (assets.Asset, error)
	getAssetsFunc          func(ctx context.Context, path string, limit int32, cursor *string) (assets.GetAssetsResponse, error)
	createFolderFunc       func(ctx context.Context, path string, name string, description *string, createdBy string) (*assets.Folder, error)
	createFileUploadFunc   func(ctx context.Context, path string, name string, description *string, contentType string, createdBy string) (*assets.PresignedUploadResult, *assets.File, error)
	confirmFileUploadFunc  func(ctx context.Context, fullPath string) (*assets.File, error)
	deleteAssetFunc        func(ctx context.Context, fullPath string) error
}

func (m *mockAssetsManager) GetAsset(ctx context.Context, fullPath string) (assets.Asset, error) {
	if m.getAssetFunc != nil {
		return m.getAssetFunc(ctx, fullPath)
	}
	return nil, nil
}

func (m *mockAssetsManager) GetAssets(ctx context.Context, path string, limit int32, cursor *string) (assets.GetAssetsResponse, error) {
	if m.getAssetsFunc != nil {
		return m.getAssetsFunc(ctx, path, limit, cursor)
	}
	return assets.GetAssetsResponse{}, nil
}

func (m *mockAssetsManager) CreateFolder(ctx context.Context, path string, name string, description *string, createdBy string) (*assets.Folder, error) {
	if m.createFolderFunc != nil {
		return m.createFolderFunc(ctx, path, name, description, createdBy)
	}
	return nil, nil
}

func (m *mockAssetsManager) CreateFileUpload(ctx context.Context, path string, name string, description *string, contentType string, createdBy string) (*assets.PresignedUploadResult, *assets.File, error) {
	if m.createFileUploadFunc != nil {
		return m.createFileUploadFunc(ctx, path, name, description, contentType, createdBy)
	}
	return nil, nil, nil
}

func (m *mockAssetsManager) ConfirmFileUpload(ctx context.Context, fullPath string) (*assets.File, error) {
	if m.confirmFileUploadFunc != nil {
		return m.confirmFileUploadFunc(ctx, fullPath)
	}
	return nil, nil
}

func (m *mockAssetsManager) DeleteAsset(ctx context.Context, fullPath string) error {
	if m.deleteAssetFunc != nil {
		return m.deleteAssetFunc(ctx, fullPath)
	}
	return nil
}

func TestNewAPI(t *testing.T) {
	manager := &mockAssetsManager{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cdnBaseURL := "https://cdn.example.com"

	api := NewAPI(manager, logger, LOCAL, cdnBaseURL, nil)

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

	manager := &mockAssetsManager{
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

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", nil)

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
	manager := &mockAssetsManager{
		getAssetsFunc: func(ctx context.Context, path string, limit int32, cursor *string) (assets.GetAssetsResponse, error) {
			return assets.GetAssetsResponse{}, assets.NewInvalidCursorError("invalid cursor", nil)
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", nil)

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

	manager := &mockAssetsManager{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			if fullPath != "/test.txt" {
				t.Errorf("GetAsset called with path %q, want %q", fullPath, "/test.txt")
			}
			return expectedFile, nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", nil)

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
	manager := &mockAssetsManager{
		getAssetFunc: func(ctx context.Context, fullPath string) (assets.Asset, error) {
			return nil, assets.NewNotFoundError("not found", nil)
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", nil)

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
	if apiFile.Status != FileStatusConfirmed {
		t.Errorf("Status = %v, want %v", apiFile.Status, FileStatusConfirmed)
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
	manager := &mockAssetsManager{
		deleteAssetFunc: func(ctx context.Context, fullPath string) error {
			if fullPath != "/test.txt" {
				t.Errorf("DeleteAsset called with path %q, want %q", fullPath, "/test.txt")
			}
			return nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", nil)

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
	manager := &mockAssetsManager{
		deleteAssetFunc: func(ctx context.Context, fullPath string) error {
			return assets.NewNotFoundError("not found", nil)
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", nil)

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
	manager := &mockAssetsManager{
		deleteAssetFunc: func(ctx context.Context, fullPath string) error {
			return assets.NewFolderNotEmptyError("folder not empty")
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := NewAPI(manager, logger, LOCAL, "https://cdn.example.com", nil)

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
