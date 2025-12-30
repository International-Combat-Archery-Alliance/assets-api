package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/International-Combat-Archery-Alliance/assets-api/assets"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

const (
	// MaxUploadSize is the maximum file size allowed (25MB)
	MaxUploadSize = 25 * 1024 * 1024

	// PresignedURLExpiration is how long presigned URLs are valid
	PresignedURLExpiration = 1 * time.Hour
)

// Storage handles S3 operations for asset files
type Storage struct {
	client     *s3.Client
	presigner  *s3.PresignClient
	bucketName string
	cdnBaseURL string
}

// NewStorage creates a new S3 storage instance
func NewStorage(client *s3.Client, bucketName, cdnBaseURL string) *Storage {
	return &Storage{
		client:     client,
		presigner:  s3.NewPresignClient(client),
		bucketName: bucketName,
		cdnBaseURL: cdnBaseURL,
	}
}

// GenerateS3Key generates the S3 key for an asset
// Format: {folder}/{assetId}/{fileName}
func GenerateS3Key(folder string, assetID uuid.UUID, fileName string) string {
	return fmt.Sprintf("%s/%s/%s", folder, assetID.String(), fileName)
}

// PresignedUploadResult contains the result of generating a presigned upload URL
type PresignedUploadResult struct {
	UploadURL string
	ExpiresAt time.Time
}

// GeneratePresignedUploadURL generates a presigned URL for uploading an asset
func (s *Storage) GeneratePresignedUploadURL(ctx context.Context, s3Key, contentType string) (PresignedUploadResult, error) {
	expiresAt := time.Now().Add(PresignedURLExpiration)

	presignedReq, err := s.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucketName),
		Key:           aws.String(s3Key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(MaxUploadSize), // This is the max allowed
	}, func(opts *s3.PresignOptions) {
		opts.Expires = PresignedURLExpiration
	})
	if err != nil {
		return PresignedUploadResult{}, fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return PresignedUploadResult{
		UploadURL: presignedReq.URL,
		ExpiresAt: expiresAt,
	}, nil
}

// HeadObjectResult contains metadata about an S3 object
type HeadObjectResult struct {
	Size        int64
	ContentType string
	Exists      bool
}

// HeadObject checks if an object exists and returns its metadata
func (s *Storage) HeadObject(ctx context.Context, s3Key string) (HeadObjectResult, error) {
	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		// Check if the error is a "not found" error
		// The AWS SDK v2 returns a specific error type for this
		return HeadObjectResult{Exists: false}, nil
	}

	var size int64
	if result.ContentLength != nil {
		size = *result.ContentLength
	}

	var contentType string
	if result.ContentType != nil {
		contentType = *result.ContentType
	}

	return HeadObjectResult{
		Size:        size,
		ContentType: contentType,
		Exists:      true,
	}, nil
}

// DeleteObject deletes an object from S3
func (s *Storage) DeleteObject(ctx context.Context, s3Key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		return assets.NewFailedToDeleteError(fmt.Sprintf("failed to delete S3 object %s", s3Key), err)
	}
	return nil
}

// GetCDNURL returns the CDN URL for an asset
func (s *Storage) GetCDNURL(s3Key string) string {
	return s.cdnBaseURL + "/" + s3Key
}

// BucketName returns the bucket name
func (s *Storage) BucketName() string {
	return s.bucketName
}
