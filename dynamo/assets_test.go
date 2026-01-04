package dynamo

import (
	"context"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/assets-api/assets"
	"github.com/International-Combat-Archery-Alliance/assets-api/ptr"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupDynamoDB(t *testing.T) (*dynamodb.Client, string, func()) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "amazon/dynamodb-local:latest",
		ExposedPorts: []string{"8000/tcp"},
		WaitingFor:   wait.ForListeningPort("8000/tcp"),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start DynamoDB container: %v", err)
	}

	cleanup := func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}

	mappedPort, err := container.MappedPort(ctx, "8000/tcp")
	if err != nil {
		cleanup()
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	hostIP, err := container.Host(ctx)
	if err != nil {
		cleanup()
		t.Fatalf("Failed to get host: %v", err)
	}

	endpoint := "http://" + hostIP + ":" + mappedPort.Port()

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		cleanup()
		t.Fatalf("Failed to load AWS config: %v", err)
	}

	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	tableName := "test-assets-table"
	_, err = client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("PK"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("SK"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("PK"),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String("SK"),
				KeyType:       types.KeyTypeRange,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		cleanup()
		t.Fatalf("Failed to create table: %v", err)
	}

	return client, tableName, cleanup
}

func TestNewDB(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)

	if db == nil {
		t.Fatal("NewDB() returned nil")
	}
	if db.dynamoClient != client {
		t.Error("dynamoClient not set correctly")
	}
	if db.tableName != tableName {
		t.Errorf("tableName = %q, want %q", db.tableName, tableName)
	}
}

func TestDB_EnsureRootFolderExists(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	err := db.EnsureRootFolderExists(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("EnsureRootFolderExists() error = %v", err)
	}

	asset, err := db.GetAsset(ctx, "/")
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}

	folder, ok := asset.(*assets.Folder)
	if !ok {
		t.Fatal("Root asset is not a folder")
	}

	if folder.ID != assets.RootFolderID {
		t.Errorf("Root folder ID = %v, want %v", folder.ID, assets.RootFolderID)
	}
	if folder.Name != "/" {
		t.Errorf("Root folder Name = %q, want %q", folder.Name, "/")
	}
	if folder.Path != "" {
		t.Errorf("Root folder Path = %q, want empty string", folder.Path)
	}
	if folder.CreatedBy != "admin@example.com" {
		t.Errorf("Root folder CreatedBy = %q, want %q", folder.CreatedBy, "admin@example.com")
	}

	err = db.EnsureRootFolderExists(ctx, "admin@example.com")
	if err != nil {
		t.Errorf("EnsureRootFolderExists() should not error when called twice, got: %v", err)
	}
}

func TestDB_CreateAsset_Folder(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	err := db.EnsureRootFolderExists(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("EnsureRootFolderExists() error = %v", err)
	}

	folderID := uuid.New()
	desc := "Test folder"
	folder := &assets.Folder{
		ID:           folderID,
		Path:         "/",
		Name:         "test-folder",
		Description:  &desc,
		ContentCount: 0,
		CreatedAt:    time.Now().UTC(),
		CreatedBy:    "user@example.com",
		Version:      0,
	}

	err = db.CreateAsset(ctx, folder)
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}

	asset, err := db.GetAsset(ctx, "/test-folder")
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}

	retrievedFolder, ok := asset.(*assets.Folder)
	if !ok {
		t.Fatal("Retrieved asset is not a folder")
	}

	if retrievedFolder.ID != folderID {
		t.Errorf("Folder ID = %v, want %v", retrievedFolder.ID, folderID)
	}
	if retrievedFolder.Name != "test-folder" {
		t.Errorf("Folder Name = %q, want %q", retrievedFolder.Name, "test-folder")
	}
	if retrievedFolder.Description == nil || *retrievedFolder.Description != desc {
		t.Errorf("Folder Description = %v, want %q", retrievedFolder.Description, desc)
	}
}

func TestDB_CreateAsset_File(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	err := db.EnsureRootFolderExists(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("EnsureRootFolderExists() error = %v", err)
	}

	fileID := uuid.New()
	desc := "Test file"
	expiresAt := time.Now().UTC().Add(1 * time.Hour)
	file := &assets.File{
		ID:          fileID,
		Path:        "/",
		Name:        "test.txt",
		Description: &desc,
		ContentType: "text/plain",
		Size:        1024,
		ObjectKey:   fileID.String(),
		Status:      assets.StatusPending,
		ExpiresAt:   &expiresAt,
		CreatedAt:   time.Now().UTC(),
		CreatedBy:   "user@example.com",
		Version:     0,
	}

	err = db.CreateAsset(ctx, file)
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}

	asset, err := db.GetAsset(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}

	retrievedFile, ok := asset.(*assets.File)
	if !ok {
		t.Fatal("Retrieved asset is not a file")
	}

	if retrievedFile.ID != fileID {
		t.Errorf("File ID = %v, want %v", retrievedFile.ID, fileID)
	}
	if retrievedFile.Name != "test.txt" {
		t.Errorf("File Name = %q, want %q", retrievedFile.Name, "test.txt")
	}
	if retrievedFile.ContentType != "text/plain" {
		t.Errorf("File ContentType = %q, want %q", retrievedFile.ContentType, "text/plain")
	}
	if retrievedFile.Size != 1024 {
		t.Errorf("File Size = %d, want 1024", retrievedFile.Size)
	}
	if retrievedFile.Status != assets.StatusPending {
		t.Errorf("File Status = %q, want %q", retrievedFile.Status, assets.StatusPending)
	}
}

func TestDB_CreateAsset_AlreadyExists(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	err := db.EnsureRootFolderExists(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("EnsureRootFolderExists() error = %v", err)
	}

	folder := &assets.Folder{
		ID:          uuid.New(),
		Path:        "/",
		Name:        "test-folder",
		CreatedAt:   time.Now().UTC(),
		CreatedBy:   "user@example.com",
		Version:     0,
	}

	err = db.CreateAsset(ctx, folder)
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}

	err = db.CreateAsset(ctx, folder)
	if err == nil {
		t.Fatal("CreateAsset() expected error for duplicate, got nil")
	}
	if !assets.IsAlreadyExistsError(err) {
		t.Errorf("CreateAsset() error type = %T, want AlreadyExistsError", err)
	}
}

func TestDB_CreateAsset_ParentNotFound(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	folder := &assets.Folder{
		ID:        uuid.New(),
		Path:      "/nonexistent",
		Name:      "test-folder",
		CreatedAt: time.Now().UTC(),
		CreatedBy: "user@example.com",
		Version:   0,
	}

	err := db.CreateAsset(ctx, folder)
	if err == nil {
		t.Fatal("CreateAsset() expected error for missing parent, got nil")
	}
	if !assets.IsParentFolderNotFoundError(err) {
		t.Errorf("CreateAsset() error type = %T, want ParentFolderNotFoundError", err)
	}
}

func TestDB_GetAssets(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	err := db.EnsureRootFolderExists(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("EnsureRootFolderExists() error = %v", err)
	}

	for i := 0; i < 3; i++ {
		folder := &assets.Folder{
			ID:        uuid.New(),
			Path:      "/",
			Name:      string(rune('a'+i)) + "-folder",
			CreatedAt: time.Now().UTC(),
			CreatedBy: "user@example.com",
			Version:   0,
		}
		err = db.CreateAsset(ctx, folder)
		if err != nil {
			t.Fatalf("CreateAsset() error = %v", err)
		}
	}

	result, err := db.GetAssets(ctx, "/", 10, nil)
	if err != nil {
		t.Fatalf("GetAssets() error = %v", err)
	}

	if len(result.Data) != 3 {
		t.Errorf("GetAssets() returned %d items, want 3", len(result.Data))
	}

	if result.HasNextPage {
		t.Error("GetAssets() HasNextPage = true, want false")
	}

	if result.Cursor != nil {
		t.Error("GetAssets() Cursor should be nil when no next page")
	}
}

func TestDB_GetAssets_Pagination(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	err := db.EnsureRootFolderExists(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("EnsureRootFolderExists() error = %v", err)
	}

	for i := 0; i < 5; i++ {
		folder := &assets.Folder{
			ID:        uuid.New(),
			Path:      "/",
			Name:      string(rune('a'+i)) + "-folder",
			CreatedAt: time.Now().UTC(),
			CreatedBy: "user@example.com",
			Version:   0,
		}
		err = db.CreateAsset(ctx, folder)
		if err != nil {
			t.Fatalf("CreateAsset() error = %v", err)
		}
	}

	result, err := db.GetAssets(ctx, "/", 2, nil)
	if err != nil {
		t.Fatalf("GetAssets() error = %v", err)
	}

	if len(result.Data) != 2 {
		t.Errorf("GetAssets() returned %d items, want 2", len(result.Data))
	}

	if !result.HasNextPage {
		t.Error("GetAssets() HasNextPage = false, want true")
	}

	if result.Cursor == nil {
		t.Fatal("GetAssets() Cursor should not be nil when has next page")
	}

	result2, err := db.GetAssets(ctx, "/", 2, result.Cursor)
	if err != nil {
		t.Fatalf("GetAssets() error = %v", err)
	}

	if len(result2.Data) != 2 {
		t.Errorf("GetAssets() page 2 returned %d items, want 2", len(result2.Data))
	}
}

func TestDB_UpdateAsset(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	err := db.EnsureRootFolderExists(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("EnsureRootFolderExists() error = %v", err)
	}

	fileID := uuid.New()
	file := &assets.File{
		ID:          fileID,
		Path:        "/",
		Name:        "test.txt",
		ContentType: "text/plain",
		Size:        0,
		ObjectKey:   fileID.String(),
		Status:      assets.StatusPending,
		CreatedAt:   time.Now().UTC(),
		CreatedBy:   "user@example.com",
		Version:     0,
	}

	err = db.CreateAsset(ctx, file)
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}

	file.Size = 1024
	file.Status = assets.StatusConfirmed
	file.ExpiresAt = nil
	file.Version = 1

	err = db.UpdateAsset(ctx, file)
	if err != nil {
		t.Fatalf("UpdateAsset() error = %v", err)
	}

	asset, err := db.GetAsset(ctx, "/test.txt")
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}

	updatedFile, ok := asset.(*assets.File)
	if !ok {
		t.Fatal("Retrieved asset is not a file")
	}

	if updatedFile.Size != 1024 {
		t.Errorf("File Size = %d, want 1024", updatedFile.Size)
	}
	if updatedFile.Status != assets.StatusConfirmed {
		t.Errorf("File Status = %q, want %q", updatedFile.Status, assets.StatusConfirmed)
	}
	if updatedFile.Version != 1 {
		t.Errorf("File Version = %d, want 1", updatedFile.Version)
	}
}

func TestDB_UpdateAsset_VersionConflict(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	err := db.EnsureRootFolderExists(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("EnsureRootFolderExists() error = %v", err)
	}

	fileID := uuid.New()
	file := &assets.File{
		ID:          fileID,
		Path:        "/",
		Name:        "test.txt",
		ContentType: "text/plain",
		Size:        0,
		ObjectKey:   fileID.String(),
		Status:      assets.StatusPending,
		CreatedAt:   time.Now().UTC(),
		CreatedBy:   "user@example.com",
		Version:     0,
	}

	err = db.CreateAsset(ctx, file)
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}

	file.Size = 1024
	file.Version = 5

	err = db.UpdateAsset(ctx, file)
	if err == nil {
		t.Fatal("UpdateAsset() expected version conflict error, got nil")
	}
	if !assets.IsVersionConflictError(err) {
		t.Errorf("UpdateAsset() error type = %T, want VersionConflictError", err)
	}
}

func TestDB_DeleteAsset_Folder(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	err := db.EnsureRootFolderExists(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("EnsureRootFolderExists() error = %v", err)
	}

	folder := &assets.Folder{
		ID:           uuid.New(),
		Path:         "/",
		Name:         "test-folder",
		ContentCount: 0,
		CreatedAt:    time.Now().UTC(),
		CreatedBy:    "user@example.com",
		Version:      0,
	}

	err = db.CreateAsset(ctx, folder)
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}

	err = db.DeleteAsset(ctx, "/test-folder")
	if err != nil {
		t.Fatalf("DeleteAsset() error = %v", err)
	}

	_, err = db.GetAsset(ctx, "/test-folder")
	if err == nil {
		t.Fatal("GetAsset() should return error for deleted asset")
	}
	if !assets.IsNotFoundError(err) {
		t.Errorf("GetAsset() error type = %T, want NotFoundError", err)
	}
}

func TestDB_DeleteAsset_FolderNotEmpty(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	err := db.EnsureRootFolderExists(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("EnsureRootFolderExists() error = %v", err)
	}

	parentFolder := &assets.Folder{
		ID:           uuid.New(),
		Path:         "/",
		Name:         "parent",
		ContentCount: 0,
		CreatedAt:    time.Now().UTC(),
		CreatedBy:    "user@example.com",
		Version:      0,
	}

	err = db.CreateAsset(ctx, parentFolder)
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}

	childFolder := &assets.Folder{
		ID:           uuid.New(),
		Path:         "/parent",
		Name:         "child",
		ContentCount: 0,
		CreatedAt:    time.Now().UTC(),
		CreatedBy:    "user@example.com",
		Version:      0,
	}

	err = db.CreateAsset(ctx, childFolder)
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}

	err = db.DeleteAsset(ctx, "/parent")
	if err == nil {
		t.Fatal("DeleteAsset() expected error for non-empty folder, got nil")
	}
	if !assets.IsFolderNotEmptyError(err) {
		t.Errorf("DeleteAsset() error type = %T, want FolderNotEmptyError", err)
	}
}

func TestDB_DeleteAsset_RootFolder(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	err := db.EnsureRootFolderExists(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("EnsureRootFolderExists() error = %v", err)
	}

	err = db.DeleteAsset(ctx, "/")
	if err == nil {
		t.Fatal("DeleteAsset() expected error for root folder, got nil")
	}
	if !assets.IsNotAllowedToDeleteRootError(err) {
		t.Errorf("DeleteAsset() error type = %T, want NotAllowedToDeleteRootError", err)
	}
}

func TestDB_GetAsset_NotFound(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	_, err := db.GetAsset(ctx, "/nonexistent")
	if err == nil {
		t.Fatal("GetAsset() expected error, got nil")
	}
	if !assets.IsNotFoundError(err) {
		t.Errorf("GetAsset() error type = %T, want NotFoundError", err)
	}
}

func TestSplitFullPath(t *testing.T) {
	tests := []struct {
		name           string
		fullPath       string
		wantParentPath string
		wantName       string
	}{
		{
			name:           "root folder",
			fullPath:       "/",
			wantParentPath: "",
			wantName:       "/",
		},
		{
			name:           "file in root",
			fullPath:       "/test.txt",
			wantParentPath: "/",
			wantName:       "test.txt",
		},
		{
			name:           "nested file",
			fullPath:       "/foo/bar.txt",
			wantParentPath: "/foo",
			wantName:       "bar.txt",
		},
		{
			name:           "nested folder",
			fullPath:       "/foo/bar/baz",
			wantParentPath: "/foo/bar",
			wantName:       "baz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotParentPath, gotName := splitFullPath(tt.fullPath)
			if gotParentPath != tt.wantParentPath {
				t.Errorf("splitFullPath(%q) parentPath = %q, want %q", tt.fullPath, gotParentPath, tt.wantParentPath)
			}
			if gotName != tt.wantName {
				t.Errorf("splitFullPath(%q) name = %q, want %q", tt.fullPath, gotName, tt.wantName)
			}
		})
	}
}

func TestDB_ContentCountUpdates(t *testing.T) {
	client, tableName, cleanup := setupDynamoDB(t)
	defer cleanup()

	db := NewDB(client, tableName)
	ctx := context.Background()

	err := db.EnsureRootFolderExists(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("EnsureRootFolderExists() error = %v", err)
	}

	rootAsset, err := db.GetAsset(ctx, "/")
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}
	rootFolder := rootAsset.(*assets.Folder)
	if rootFolder.ContentCount != 0 {
		t.Errorf("Initial ContentCount = %d, want 0", rootFolder.ContentCount)
	}

	folder := &assets.Folder{
		ID:        uuid.New(),
		Path:      "/",
		Name:      "test-folder",
		CreatedAt: time.Now().UTC(),
		CreatedBy: "user@example.com",
		Version:   0,
	}
	err = db.CreateAsset(ctx, folder)
	if err != nil {
		t.Fatalf("CreateAsset() error = %v", err)
	}

	rootAsset, err = db.GetAsset(ctx, "/")
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}
	rootFolder = rootAsset.(*assets.Folder)
	if rootFolder.ContentCount != 1 {
		t.Errorf("ContentCount after add = %d, want 1", rootFolder.ContentCount)
	}

	err = db.DeleteAsset(ctx, "/test-folder")
	if err != nil {
		t.Fatalf("DeleteAsset() error = %v", err)
	}

	rootAsset, err = db.GetAsset(ctx, "/")
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}
	rootFolder = rootAsset.(*assets.Folder)
	if rootFolder.ContentCount != 0 {
		t.Errorf("ContentCount after delete = %d, want 0", rootFolder.ContentCount)
	}
}
