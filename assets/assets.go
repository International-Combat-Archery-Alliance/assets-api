package assets

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Status represents the status of an asset
type Status string

const (
	StatusPending   Status = "pending"
	StatusConfirmed Status = "confirmed"
)

// Asset represents an uploaded file (image, document, etc.)
type Asset struct {
	ID          uuid.UUID
	Folder      string
	Name        string
	Description *string
	ContentType string
	Size        int64
	S3Key       string
	Status      Status
	CreatedAt   time.Time
	CreatedBy   string
}

// URL returns the CDN URL for the asset
func (a Asset) URL(cdnBaseURL string) string {
	return cdnBaseURL + "/" + a.S3Key
}

// GetAssetsResponse is the response for listing assets
type GetAssetsResponse struct {
	Data        []Asset
	Cursor      *string
	HasNextPage bool
}

// Repository defines the interface for asset persistence
type Repository interface {
	// GetAsset retrieves a single asset by ID
	GetAsset(ctx context.Context, id uuid.UUID) (Asset, error)

	// GetAssets retrieves assets with optional folder filter and pagination
	GetAssets(ctx context.Context, folder *string, limit int32, cursor *string) (GetAssetsResponse, error)

	// GetFolders retrieves all distinct folder names
	GetFolders(ctx context.Context) ([]string, error)

	// CreateAsset creates a new asset record
	CreateAsset(ctx context.Context, asset Asset) error

	// UpdateAsset updates an existing asset
	UpdateAsset(ctx context.Context, asset Asset) error

	// DeleteAsset deletes an asset by ID
	DeleteAsset(ctx context.Context, id uuid.UUID) error

	// AddFolder adds a folder to the folder index (called when first asset in folder is created)
	AddFolder(ctx context.Context, folder string) error

	// RemoveFolder removes a folder from the index (called when last asset in folder is deleted)
	RemoveFolder(ctx context.Context, folder string) error

	// CountAssetsInFolder returns the number of assets in a folder
	CountAssetsInFolder(ctx context.Context, folder string) (int, error)
}
