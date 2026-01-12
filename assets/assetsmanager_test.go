package assets

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Mock implementations for testing

type mockMetadataRepo struct {
	getAssetFunc              func(ctx context.Context, fullPath string) (Asset, error)
	getAssetsFunc             func(ctx context.Context, path string, limit int32, cursor *string) (GetAssetsResponse, error)
	createAssetFunc           func(ctx context.Context, asset Asset) error
	updateAssetFunc           func(ctx context.Context, asset Asset) error
	deleteAssetFunc           func(ctx context.Context, fullPath string) error
	ensureRootFolderExitsFunc func(ctx context.Context, createdBy string) error
}

func (m *mockMetadataRepo) GetAsset(ctx context.Context, fullPath string) (Asset, error) {
	if m.getAssetFunc != nil {
		return m.getAssetFunc(ctx, fullPath)
	}
	return nil, nil
}

func (m *mockMetadataRepo) GetAssets(ctx context.Context, path string, limit int32, cursor *string) (GetAssetsResponse, error) {
	if m.getAssetsFunc != nil {
		return m.getAssetsFunc(ctx, path, limit, cursor)
	}
	return GetAssetsResponse{}, nil
}

func (m *mockMetadataRepo) CreateAsset(ctx context.Context, asset Asset) error {
	if m.createAssetFunc != nil {
		return m.createAssetFunc(ctx, asset)
	}
	return nil
}

func (m *mockMetadataRepo) UpdateAsset(ctx context.Context, asset Asset) error {
	if m.updateAssetFunc != nil {
		return m.updateAssetFunc(ctx, asset)
	}
	return nil
}

func (m *mockMetadataRepo) DeleteAsset(ctx context.Context, fullPath string) error {
	if m.deleteAssetFunc != nil {
		return m.deleteAssetFunc(ctx, fullPath)
	}
	return nil
}

func (m *mockMetadataRepo) EnsureRootFolderExists(ctx context.Context, createdBy string) error {
	if m.ensureRootFolderExitsFunc != nil {
		return m.ensureRootFolderExitsFunc(ctx, createdBy)
	}
	return nil
}

type mockStorageRepo struct {
	generateObjectKeyFunc          func(assetID uuid.UUID, filename string) string
	generatePresignedUploadURLFunc func(ctx context.Context, assetID uuid.UUID, filename string, contentType string, ttl time.Duration, maxFileSize int) (PresignedUploadResult, error)
	headObjectFunc                 func(ctx context.Context, objectKey string) (HeadObjectResult, error)
	deleteObjectFunc               func(ctx context.Context, objectKey string) error
}

func (m *mockStorageRepo) GenerateObjectKey(assetID uuid.UUID, filename string) string {
	if m.generateObjectKeyFunc != nil {
		return m.generateObjectKeyFunc(assetID, filename)
	}
	return assetID.String()
}

func (m *mockStorageRepo) GeneratePresignedUploadURL(ctx context.Context, assetID uuid.UUID, filename string, contentType string, ttl time.Duration, maxFileSize int) (PresignedUploadResult, error) {
	if m.generatePresignedUploadURLFunc != nil {
		return m.generatePresignedUploadURLFunc(ctx, assetID, filename, contentType, ttl, maxFileSize)
	}
	return PresignedUploadResult{}, nil
}

func (m *mockStorageRepo) HeadObject(ctx context.Context, objectKey string) (HeadObjectResult, error) {
	if m.headObjectFunc != nil {
		return m.headObjectFunc(ctx, objectKey)
	}
	return HeadObjectResult{}, nil
}

func (m *mockStorageRepo) DeleteObject(ctx context.Context, objectKey string) error {
	if m.deleteObjectFunc != nil {
		return m.deleteObjectFunc(ctx, objectKey)
	}
	return nil
}

func TestNewAssetsManager(t *testing.T) {
	metadataRepo := &mockMetadataRepo{}
	storageRepo := &mockStorageRepo{}

	am := NewAssetsManager(storageRepo, metadataRepo)

	if am == nil {
		t.Fatal("NewAssetsManager() returned nil")
	}
	if am.storageRepo != storageRepo {
		t.Error("storageRepo not set correctly")
	}
	if am.metadataRepo != metadataRepo {
		t.Error("metadataRepo not set correctly")
	}
}

func TestAssetsManager_GetAsset(t *testing.T) {
	expectedAsset := &File{
		ID:   uuid.New(),
		Name: "test.txt",
		Path: "/",
	}

	metadataRepo := &mockMetadataRepo{
		getAssetFunc: func(ctx context.Context, fullPath string) (Asset, error) {
			if fullPath != "/test.txt" {
				t.Errorf("GetAsset called with path %q, want %q", fullPath, "/test.txt")
			}
			return expectedAsset, nil
		},
	}

	am := NewAssetsManager(&mockStorageRepo{}, metadataRepo)
	ctx := context.Background()

	asset, err := am.GetAsset(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}
	if asset != expectedAsset {
		t.Error("GetAsset() returned different asset")
	}
}

func TestAssetsManager_GetAssets(t *testing.T) {
	cursor := "test-cursor"
	expectedResponse := GetAssetsResponse{
		Data: []Asset{
			&File{ID: uuid.New(), Name: "file.txt"},
		},
		Cursor:      &cursor,
		HasNextPage: true,
	}

	metadataRepo := &mockMetadataRepo{
		getAssetsFunc: func(ctx context.Context, path string, limit int32, c *string) (GetAssetsResponse, error) {
			if path != "/" {
				t.Errorf("GetAssets called with path %q, want %q", path, "/")
			}
			if limit != 10 {
				t.Errorf("GetAssets called with limit %d, want 10", limit)
			}
			return expectedResponse, nil
		},
	}

	am := NewAssetsManager(&mockStorageRepo{}, metadataRepo)
	ctx := context.Background()

	response, err := am.GetAssets(ctx, "/", 10, nil)
	if err != nil {
		t.Fatalf("GetAssets() error = %v", err)
	}
	if len(response.Data) != len(expectedResponse.Data) {
		t.Errorf("GetAssets() returned %d items, want %d", len(response.Data), len(expectedResponse.Data))
	}
	if !response.HasNextPage {
		t.Error("GetAssets() HasNextPage = false, want true")
	}
}

func TestAssetsManager_CreateFolder(t *testing.T) {
	var createdAsset Asset

	metadataRepo := &mockMetadataRepo{
		createAssetFunc: func(ctx context.Context, asset Asset) error {
			createdAsset = asset
			return nil
		},
	}

	am := NewAssetsManager(&mockStorageRepo{}, metadataRepo)
	ctx := context.Background()

	desc := "Test folder"
	folder, err := am.CreateFolder(ctx, "/", "test-folder", &desc, "user@example.com")
	if err != nil {
		t.Fatalf("CreateFolder() error = %v", err)
	}

	if folder.Name != "test-folder" {
		t.Errorf("CreateFolder() name = %q, want %q", folder.Name, "test-folder")
	}
	if folder.Path != "/" {
		t.Errorf("CreateFolder() path = %q, want %q", folder.Path, "/")
	}
	if folder.Description == nil || *folder.Description != desc {
		t.Errorf("CreateFolder() description = %v, want %q", folder.Description, desc)
	}
	if folder.CreatedBy != "user@example.com" {
		t.Errorf("CreateFolder() createdBy = %q, want %q", folder.CreatedBy, "user@example.com")
	}
	if folder.ContentCount != 0 {
		t.Errorf("CreateFolder() contentCount = %d, want 0", folder.ContentCount)
	}
	if createdAsset == nil {
		t.Fatal("CreateAsset not called")
	}
}

func TestAssetsManager_CreateFileUpload(t *testing.T) {
	var createdAsset Asset
	expectedObjectKey := "test-object-key"
	expectedUploadURL := "https://s3.amazonaws.com/bucket"

	metadataRepo := &mockMetadataRepo{
		createAssetFunc: func(ctx context.Context, asset Asset) error {
			createdAsset = asset
			return nil
		},
	}

	storageRepo := &mockStorageRepo{
		generateObjectKeyFunc: func(assetID uuid.UUID, filename string) string {
			return expectedObjectKey
		},
		generatePresignedUploadURLFunc: func(ctx context.Context, assetID uuid.UUID, filename string, contentType string, ttl time.Duration, maxFileSize int) (PresignedUploadResult, error) {
			return PresignedUploadResult{
				UploadURL:  expectedUploadURL,
				FormFields: map[string]string{"key": "value"},
				ExpiresAt:  time.Now().Add(ttl),
			}, nil
		},
	}

	am := NewAssetsManager(storageRepo, metadataRepo)
	ctx := context.Background()

	desc := "Test file"
	presignResult, file, err := am.CreateFileUpload(ctx, "/", "test.txt", &desc, "text/plain", "user@example.com")
	if err != nil {
		t.Fatalf("CreateFileUpload() error = %v", err)
	}

	if presignResult.UploadURL != expectedUploadURL {
		t.Errorf("CreateFileUpload() uploadURL = %q, want %q", presignResult.UploadURL, expectedUploadURL)
	}

	if file.Name != "test.txt" {
		t.Errorf("CreateFileUpload() name = %q, want %q", file.Name, "test.txt")
	}
	if file.Path != "/" {
		t.Errorf("CreateFileUpload() path = %q, want %q", file.Path, "/")
	}
	if file.ContentType != "text/plain" {
		t.Errorf("CreateFileUpload() contentType = %q, want %q", file.ContentType, "text/plain")
	}
	if file.Status != StatusPending {
		t.Errorf("CreateFileUpload() status = %q, want %q", file.Status, StatusPending)
	}
	if file.ObjectKey != expectedObjectKey {
		t.Errorf("CreateFileUpload() objectKey = %q, want %q", file.ObjectKey, expectedObjectKey)
	}
	if file.Size != 0 {
		t.Errorf("CreateFileUpload() size = %d, want 0", file.Size)
	}
	if createdAsset == nil {
		t.Fatal("CreateAsset not called")
	}
}

func TestAssetsManager_ConfirmFileUpload_Success(t *testing.T) {
	fileID := uuid.New()
	file := &File{
		ID:          fileID,
		Name:        "test.txt",
		Path:        "/",
		Status:      StatusPending,
		ContentType: "text/plain",
		Size:        0,
		ObjectKey:   fileID.String(),
		Version:     0,
	}

	var updatedAsset Asset

	metadataRepo := &mockMetadataRepo{
		getAssetFunc: func(ctx context.Context, fullPath string) (Asset, error) {
			return file, nil
		},
		updateAssetFunc: func(ctx context.Context, asset Asset) error {
			updatedAsset = asset
			return nil
		},
	}

	storageRepo := &mockStorageRepo{
		headObjectFunc: func(ctx context.Context, objectKey string) (HeadObjectResult, error) {
			expectedKey := fileID.String()
			if objectKey != expectedKey {
				t.Errorf("HeadObject called with key %v, want %v", objectKey, expectedKey)
			}
			return HeadObjectResult{
				Size:        1024,
				ContentType: "text/plain",
				Exists:      true,
			}, nil
		},
	}

	am := NewAssetsManager(storageRepo, metadataRepo)
	ctx := context.Background()

	confirmedFile, err := am.ConfirmFileUpload(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("ConfirmFileUpload() error = %v", err)
	}

	if confirmedFile.Status != StatusConfirmed {
		t.Errorf("ConfirmFileUpload() status = %q, want %q", confirmedFile.Status, StatusConfirmed)
	}
	if confirmedFile.Size != 1024 {
		t.Errorf("ConfirmFileUpload() size = %d, want 1024", confirmedFile.Size)
	}
	if confirmedFile.ExpiresAt != nil {
		t.Error("ConfirmFileUpload() expiresAt should be nil")
	}
	if confirmedFile.Version != 1 {
		t.Errorf("ConfirmFileUpload() version = %d, want 1", confirmedFile.Version)
	}
	if updatedAsset == nil {
		t.Fatal("UpdateAsset not called")
	}
}

func TestAssetsManager_ConfirmFileUpload_AlreadyConfirmed(t *testing.T) {
	file := &File{
		ID:     uuid.New(),
		Name:   "test.txt",
		Path:   "/",
		Status: StatusConfirmed,
		Size:   1024,
	}

	metadataRepo := &mockMetadataRepo{
		getAssetFunc: func(ctx context.Context, fullPath string) (Asset, error) {
			return file, nil
		},
	}

	am := NewAssetsManager(&mockStorageRepo{}, metadataRepo)
	ctx := context.Background()

	confirmedFile, err := am.ConfirmFileUpload(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("ConfirmFileUpload() error = %v", err)
	}

	if confirmedFile.Status != StatusConfirmed {
		t.Errorf("ConfirmFileUpload() status = %q, want %q", confirmedFile.Status, StatusConfirmed)
	}
}

func TestAssetsManager_ConfirmFileUpload_NotAFile(t *testing.T) {
	folder := &Folder{
		ID:   uuid.New(),
		Name: "test-folder",
		Path: "/",
	}

	metadataRepo := &mockMetadataRepo{
		getAssetFunc: func(ctx context.Context, fullPath string) (Asset, error) {
			return folder, nil
		},
	}

	am := NewAssetsManager(&mockStorageRepo{}, metadataRepo)
	ctx := context.Background()

	_, err := am.ConfirmFileUpload(ctx, "/test-folder")
	if err == nil {
		t.Fatal("ConfirmFileUpload() expected error, got nil")
	}
	if !IsNotAFileError(err) {
		t.Errorf("ConfirmFileUpload() error type = %T, want NotAFileError", err)
	}
}

func TestAssetsManager_ConfirmFileUpload_ObjectNotExists(t *testing.T) {
	file := &File{
		ID:     uuid.New(),
		Name:   "test.txt",
		Path:   "/",
		Status: StatusPending,
	}

	metadataRepo := &mockMetadataRepo{
		getAssetFunc: func(ctx context.Context, fullPath string) (Asset, error) {
			return file, nil
		},
	}

	storageRepo := &mockStorageRepo{
		headObjectFunc: func(ctx context.Context, objectKey string) (HeadObjectResult, error) {
			return HeadObjectResult{
				Exists: false,
			}, nil
		},
	}

	am := NewAssetsManager(storageRepo, metadataRepo)
	ctx := context.Background()

	_, err := am.ConfirmFileUpload(ctx, "/test.txt")
	if err == nil {
		t.Fatal("ConfirmFileUpload() expected error, got nil")
	}
	if !IsAssetNotUploadedError(err) {
		t.Errorf("ConfirmFileUpload() error type = %T, want AssetNotUploadedError", err)
	}
}

func TestAssetsManager_DeleteAsset_File(t *testing.T) {
	fileID := uuid.New()
	file := &File{
		ID:        fileID,
		Name:      "test.txt",
		Path:      "/",
		ObjectKey: fileID.String(),
	}

	var deletedObjectKey string
	var deletedPath string

	metadataRepo := &mockMetadataRepo{
		getAssetFunc: func(ctx context.Context, fullPath string) (Asset, error) {
			return file, nil
		},
		deleteAssetFunc: func(ctx context.Context, fullPath string) error {
			deletedPath = fullPath
			return nil
		},
	}

	storageRepo := &mockStorageRepo{
		deleteObjectFunc: func(ctx context.Context, objectKey string) error {
			deletedObjectKey = objectKey
			return nil
		},
	}

	am := NewAssetsManager(storageRepo, metadataRepo)
	ctx := context.Background()

	err := am.DeleteAsset(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("DeleteAsset() error = %v", err)
	}

	if deletedObjectKey != fileID.String() {
		t.Errorf("DeleteObject called with key %v, want %v", deletedObjectKey, fileID.String())
	}
	if deletedPath != "/test.txt" {
		t.Errorf("DeleteAsset called with path %q, want %q", deletedPath, "/test.txt")
	}
}

func TestAssetsManager_DeleteAsset_Folder(t *testing.T) {
	folder := &Folder{
		ID:   uuid.New(),
		Name: "test-folder",
		Path: "/",
	}

	var deletedPath string
	deleteObjectCalled := false

	metadataRepo := &mockMetadataRepo{
		getAssetFunc: func(ctx context.Context, fullPath string) (Asset, error) {
			return folder, nil
		},
		deleteAssetFunc: func(ctx context.Context, fullPath string) error {
			deletedPath = fullPath
			return nil
		},
	}

	storageRepo := &mockStorageRepo{
		deleteObjectFunc: func(ctx context.Context, objectKey string) error {
			deleteObjectCalled = true
			return nil
		},
	}

	am := NewAssetsManager(storageRepo, metadataRepo)
	ctx := context.Background()

	err := am.DeleteAsset(ctx, "/test-folder")
	if err != nil {
		t.Fatalf("DeleteAsset() error = %v", err)
	}

	if deleteObjectCalled {
		t.Error("DeleteObject should not be called for folders")
	}
	if deletedPath != "/test-folder" {
		t.Errorf("DeleteAsset called with path %q, want %q", deletedPath, "/test-folder")
	}
}

func TestAssetsManager_DeleteAsset_StorageError(t *testing.T) {
	fileID := uuid.New()
	file := &File{
		ID:        fileID,
		Name:      "test.txt",
		Path:      "/",
		ObjectKey: fileID.String(),
	}

	metadataRepo := &mockMetadataRepo{
		getAssetFunc: func(ctx context.Context, fullPath string) (Asset, error) {
			return file, nil
		},
	}

	storageRepo := &mockStorageRepo{
		deleteObjectFunc: func(ctx context.Context, objectKey string) error {
			return fmt.Errorf("storage error")
		},
	}

	am := NewAssetsManager(storageRepo, metadataRepo)
	ctx := context.Background()

	err := am.DeleteAsset(ctx, "/test.txt")
	if err == nil {
		t.Fatal("DeleteAsset() expected error, got nil")
	}
}
