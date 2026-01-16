package assets

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	maxUploadSize = 25 * 1024 * 1024 // 25MB
	uploadTTL     = 1 * time.Hour
)

type AssetsManager struct {
	storageRepo  StorageRepository
	metadataRepo MetadataRepository
}

func NewAssetsManager(storageRepo StorageRepository, metadataRepo MetadataRepository) *AssetsManager {
	return &AssetsManager{
		storageRepo:  storageRepo,
		metadataRepo: metadataRepo,
	}
}

func (am *AssetsManager) GetAsset(ctx context.Context, fullPath string) (Asset, error) {
	return am.metadataRepo.GetAsset(ctx, fullPath)
}

func (am *AssetsManager) GetAssets(ctx context.Context, path string, limit int32, cursor *string) (GetAssetsResponse, error) {
	return am.metadataRepo.GetAssets(ctx, path, limit, cursor)
}

func (am *AssetsManager) CreateFolder(
	ctx context.Context,
	path string,
	name string,
	description *string,
	createdBy string,
) (*Folder, error) {
	id := uuid.New()
	now := time.Now().UTC()

	folder := &Folder{
		ID:           id,
		Path:         path,
		Name:         name,
		Description:  description,
		ContentCount: 0,
		CreatedAt:    now,
		CreatedBy:    createdBy,
		Version:      1,
	}

	err := am.metadataRepo.CreateAsset(ctx, folder)
	if err != nil {
		return nil, err
	}

	return folder, nil
}

func (am *AssetsManager) CreateFileUpload(
	ctx context.Context,
	path string,
	name string,
	description *string,
	contentType string,
	createdBy string,
) (*PresignedUploadResult, *File, error) {
	id := uuid.New()
	now := time.Now().UTC()

	result, err := am.storageRepo.GeneratePresignedUploadURL(ctx, id, name, contentType, uploadTTL, maxUploadSize)
	if err != nil {
		return nil, nil, err
	}

	file := &File{
		ID:          id,
		Path:        path,
		Name:        name,
		Description: description,
		ContentType: contentType,
		Size:        0, // updated on confirm
		ObjectKey:   am.storageRepo.GenerateObjectKey(id, name),
		Status:      StatusPending,
		CreatedAt:   now,
		CreatedBy:   createdBy,
		ExpiresAt:   &result.ExpiresAt,
		Version:     1,
	}
	err = am.metadataRepo.CreateAsset(ctx, file)
	if err != nil {
		return nil, nil, err
	}

	return &result, file, nil
}

func (am *AssetsManager) ConfirmFileUpload(
	ctx context.Context,
	fullPath string,
) (*File, error) {
	asset, err := am.metadataRepo.GetAsset(ctx, fullPath)
	if err != nil {
		return nil, err
	}

	file, ok := asset.(*File)
	if !ok {
		return nil, NewNotAFileError(fmt.Sprintf("%q is not a file", fullPath))
	}

	headResult, err := am.storageRepo.HeadObject(ctx, file.ObjectKey)
	if err != nil {
		return nil, NewFailedToFetchError(fmt.Sprintf("%q failed to fetch from storage", fullPath), err)
	}

	if !headResult.Exists {
		return nil, NewAssetNotUploadedError(fmt.Sprintf("%q does not exist in storage", fullPath))
	}

	// Check if anything changed - if already confirmed with same size, no update needed
	if file.Status == StatusConfirmed && file.Size == headResult.Size {
		return file, nil
	}

	file.ExpiresAt = nil
	file.Size = headResult.Size
	file.Status = StatusConfirmed
	file.Version += 1

	err = am.metadataRepo.UpdateAsset(ctx, file)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (am *AssetsManager) CreateReplaceUpload(
	ctx context.Context,
	fullPath string,
) (*PresignedUploadResult, *File, error) {
	asset, err := am.metadataRepo.GetAsset(ctx, fullPath)
	if err != nil {
		return nil, nil, err
	}

	if asset.Type() != AssetTypeFile {
		return nil, nil, NewNotAFileError(fmt.Sprintf("%q is not a file", fullPath))
	}

	file := asset.AsFile()

	if file.Status != StatusConfirmed {
		return nil, nil, NewFileNotConfirmedError(fmt.Sprintf("%q is not confirmed, cannot replace", fullPath))
	}

	// Generate presigned URL for the existing object key (overwrites the file)
	result, err := am.storageRepo.GeneratePresignedUploadURL(ctx, file.ID, file.Name, file.ContentType, uploadTTL, maxUploadSize)
	if err != nil {
		return nil, nil, err
	}

	return &result, &file, nil
}

func (am *AssetsManager) DeleteAsset(ctx context.Context, fullPath string) error {
	// Get asset first to determine type and get S3 key if it's a file
	asset, err := am.metadataRepo.GetAsset(ctx, fullPath)
	if err != nil {
		return err
	}

	// If it's a file, delete from storage first
	if file, ok := asset.(*File); ok {
		if err := am.storageRepo.DeleteObject(ctx, file.ObjectKey); err != nil {
			return err
		}
	}

	// Delete from metadata
	if err := am.metadataRepo.DeleteAsset(ctx, fullPath); err != nil {
		return err
	}

	return nil
}
