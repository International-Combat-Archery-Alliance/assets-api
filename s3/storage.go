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
	MaxUploadSize = 25 * 1024 * 1024 // 25MB
)

var _ assets.StorageRepository = &Storage{}

type Storage struct {
	client     *s3.Client
	presigner  *s3.PresignClient
	bucketName string
	cdnBaseURL string
}

func NewStorage(client *s3.Client, bucketName, cdnBaseURL string) *Storage {
	return &Storage{
		client:     client,
		presigner:  s3.NewPresignClient(client),
		bucketName: bucketName,
		cdnBaseURL: cdnBaseURL,
	}
}

func (s *Storage) GenerateObjectKey(assetID uuid.UUID) string {
	return assetID.String()
}

func (s *Storage) GeneratePresignedUploadURL(ctx context.Context, assetID uuid.UUID, contentType string, ttl time.Duration, maxFileSize int) (assets.PresignedUploadResult, error) {
	expiresAt := time.Now().Add(ttl)
	objectKey := s.GenerateObjectKey(assetID)

	// Build the POST policy conditions
	conditions := []interface{}{
		// Exact bucket match
		map[string]string{"bucket": s.bucketName},
		// Exact key match
		[]interface{}{"eq", "$key", objectKey},
		// Exact content-type match
		[]interface{}{"eq", "$Content-Type", contentType},
		// File size range: 1 byte to maxFileSize
		[]interface{}{"content-length-range", 1, maxFileSize},
	}

	presignedReq, err := s.presigner.PresignPostObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucketName),
		Key:         aws.String(objectKey),
		ContentType: aws.String(contentType),
	}, func(opts *s3.PresignPostOptions) {
		opts.Expires = ttl
		opts.Conditions = conditions
	})
	if err != nil {
		return assets.PresignedUploadResult{}, fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	// Build form fields that the client must include
	formFields := make(map[string]string)
	for k, v := range presignedReq.Values {
		formFields[k] = v
	}
	// Add required fields
	formFields["key"] = objectKey
	formFields["Content-Type"] = contentType

	return assets.PresignedUploadResult{
		UploadURL:  presignedReq.URL,
		FormFields: formFields,
		ExpiresAt:  expiresAt,
	}, nil
}

func (s *Storage) HeadObject(ctx context.Context, assetID uuid.UUID) (assets.HeadObjectResult, error) {
	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(s.GenerateObjectKey(assetID)),
	})
	if err != nil {
		// Check if the error is a "not found" error
		// The AWS SDK v2 returns a specific error type for this
		return assets.HeadObjectResult{Exists: false}, nil
	}

	var size int64
	if result.ContentLength != nil {
		size = *result.ContentLength
	}

	var contentType string
	if result.ContentType != nil {
		contentType = *result.ContentType
	}

	return assets.HeadObjectResult{
		Size:        size,
		ContentType: contentType,
		Exists:      true,
	}, nil
}

func (s *Storage) DeleteObject(ctx context.Context, assetID uuid.UUID) error {
	objectKey := s.GenerateObjectKey(assetID)

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return assets.NewFailedToDeleteError(fmt.Sprintf("failed to delete S3 object %s", objectKey), err)
	}
	return nil
}
