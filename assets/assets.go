package assets

import (
	"context"
	"net/url"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusConfirmed Status = "confirmed"
)

const RootPath = "/"

// RootFolderID is the well-known UUID for the root folder.
// The root folder has Path="" (no parent) and Name="/".
var RootFolderID = uuid.MustParse("00000000-0000-0000-0000-000000000000")

type AssetType int

const (
	AssetTypeFile AssetType = iota
	AssetTypeFolder
)

type Asset interface {
	Type() AssetType
	AsFolder() Folder
	AsFile() File
}

var _ Asset = &File{}

type File struct {
	ID          uuid.UUID
	Path        string
	Name        string
	Description *string
	ContentType string
	Size        int64
	ObjectKey   string
	Status      Status
	ExpiresAt   *time.Time
	CreatedAt   time.Time
	CreatedBy   string
	Version     int
}

func (f *File) Type() AssetType {
	return AssetTypeFile
}

func (f *File) AsFolder() Folder {
	panic("not a folder")
}

func (f *File) AsFile() File {
	return *f
}

func (f *File) URL(baseURL string) (string, error) {
	return url.JoinPath(baseURL, f.ObjectKey)
}

var _ Asset = &Folder{}

type Folder struct {
	ID           uuid.UUID
	Path         string
	Name         string
	ContentCount int
	Description  *string
	CreatedAt    time.Time
	CreatedBy    string
	Version      int
}

func (f *Folder) AsFile() File {
	panic("not a file")
}

func (f *Folder) AsFolder() Folder {
	return *f
}

func (f *Folder) Type() AssetType {
	return AssetTypeFolder
}

func (f *Folder) IsEmpty() bool {
	return f.ContentCount == 0
}

type GetAssetsResponse struct {
	Data        []Asset
	Cursor      *string
	HasNextPage bool
}

type MetadataRepository interface {
	GetAsset(ctx context.Context, fullPath string) (Asset, error)
	GetAssets(ctx context.Context, path string, limit int32, cursor *string) (GetAssetsResponse, error)
	CreateAsset(ctx context.Context, asset Asset) error
	UpdateAsset(ctx context.Context, asset Asset) error
	DeleteAsset(ctx context.Context, fullPath string) error
	EnsureRootFolderExists(ctx context.Context, createdBy string) error
}

type PresignedUploadResult struct {
	UploadURL  string
	FormFields map[string]string
	ExpiresAt  time.Time
}

type HeadObjectResult struct {
	Size        int64
	ContentType string
	Exists      bool
}

type StorageRepository interface {
	GenerateObjectKey(assetID uuid.UUID) string
	GeneratePresignedUploadURL(ctx context.Context, assetID uuid.UUID, contentType string, ttl time.Duration, maxFileSize int) (PresignedUploadResult, error)
	HeadObject(ctx context.Context, assetID uuid.UUID) (HeadObjectResult, error)
	DeleteObject(ctx context.Context, assetID uuid.UUID) error
}
