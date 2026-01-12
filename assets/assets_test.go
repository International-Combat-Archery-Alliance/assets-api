package assets

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestFile_Type(t *testing.T) {
	file := &File{}
	if file.Type() != AssetTypeFile {
		t.Errorf("Type() = %v, want %v", file.Type(), AssetTypeFile)
	}
}

func TestFile_AsFile(t *testing.T) {
	id := uuid.New()
	file := &File{
		ID:   id,
		Name: "test.txt",
		Path: "/",
	}

	result := file.AsFile()
	if result.ID != id {
		t.Errorf("AsFile() returned different file")
	}
}

func TestFile_AsFolder_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("AsFolder() should panic")
		}
	}()

	file := &File{}
	file.AsFolder()
}

func TestFile_URL(t *testing.T) {
	file := &File{
		ObjectKey: "test-key-123",
	}

	baseURL := "https://cdn.example.com"
	url, err := file.URL(baseURL)
	if err != nil {
		t.Fatalf("URL() error = %v", err)
	}

	expected := "https://cdn.example.com/test-key-123"
	if url != expected {
		t.Errorf("URL() = %q, want %q", url, expected)
	}
}

func TestFolder_Type(t *testing.T) {
	folder := &Folder{}
	if folder.Type() != AssetTypeFolder {
		t.Errorf("Type() = %v, want %v", folder.Type(), AssetTypeFolder)
	}
}

func TestFolder_AsFolder(t *testing.T) {
	id := uuid.New()
	folder := &Folder{
		ID:   id,
		Name: "test-folder",
		Path: "/",
	}

	result := folder.AsFolder()
	if result.ID != id {
		t.Errorf("AsFolder() returned different folder")
	}
}

func TestFolder_AsFile_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("AsFile() should panic")
		}
	}()

	folder := &Folder{}
	folder.AsFile()
}

func TestFolder_IsEmpty(t *testing.T) {
	tests := []struct {
		name         string
		contentCount int
		want         bool
	}{
		{"empty folder", 0, true},
		{"folder with one item", 1, false},
		{"folder with multiple items", 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			folder := &Folder{ContentCount: tt.contentCount}
			got := folder.IsEmpty()
			if got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatus_Constants(t *testing.T) {
	if StatusPending != "pending" {
		t.Errorf("StatusPending = %q, want %q", StatusPending, "pending")
	}
	if StatusConfirmed != "confirmed" {
		t.Errorf("StatusConfirmed = %q, want %q", StatusConfirmed, "confirmed")
	}
}

func TestRootPath_Constant(t *testing.T) {
	if RootPath != "/" {
		t.Errorf("RootPath = %q, want %q", RootPath, "/")
	}
}

func TestRootFolderID_Constant(t *testing.T) {
	expected := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	if RootFolderID != expected {
		t.Errorf("RootFolderID = %v, want %v", RootFolderID, expected)
	}
}

func TestAssetType_Constants(t *testing.T) {
	if AssetTypeFile != 0 {
		t.Errorf("AssetTypeFile = %d, want 0", AssetTypeFile)
	}
	if AssetTypeFolder != 1 {
		t.Errorf("AssetTypeFolder = %d, want 1", AssetTypeFolder)
	}
}

func TestAsset_Interface(t *testing.T) {
	t.Run("File implements Asset", func(t *testing.T) {
		var _ Asset = &File{}
	})

	t.Run("Folder implements Asset", func(t *testing.T) {
		var _ Asset = &Folder{}
	})
}

func TestGetAssetsResponse(t *testing.T) {
	cursor := "test-cursor"
	response := GetAssetsResponse{
		Data: []Asset{
			&File{ID: uuid.New(), Name: "file.txt"},
			&Folder{ID: uuid.New(), Name: "folder"},
		},
		Cursor:      &cursor,
		HasNextPage: true,
	}

	if len(response.Data) != 2 {
		t.Errorf("Data length = %d, want 2", len(response.Data))
	}
	if response.Cursor == nil || *response.Cursor != cursor {
		t.Errorf("Cursor = %v, want %v", response.Cursor, cursor)
	}
	if !response.HasNextPage {
		t.Error("HasNextPage should be true")
	}
}

func TestPresignedUploadResult(t *testing.T) {
	expiresAt := time.Now().Add(1 * time.Hour)
	result := PresignedUploadResult{
		UploadURL: "https://s3.amazonaws.com/bucket",
		FormFields: map[string]string{
			"key":    "test-key",
			"policy": "encoded-policy",
		},
		ExpiresAt: expiresAt,
	}

	if result.UploadURL == "" {
		t.Error("UploadURL should not be empty")
	}
	if len(result.FormFields) != 2 {
		t.Errorf("FormFields length = %d, want 2", len(result.FormFields))
	}
	if !result.ExpiresAt.Equal(expiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", result.ExpiresAt, expiresAt)
	}
}

func TestHeadObjectResult(t *testing.T) {
	result := HeadObjectResult{
		Size:        1024,
		ContentType: "text/plain",
		Exists:      true,
	}

	if result.Size != 1024 {
		t.Errorf("Size = %d, want 1024", result.Size)
	}
	if result.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want %q", result.ContentType, "text/plain")
	}
	if !result.Exists {
		t.Error("Exists should be true")
	}
}
