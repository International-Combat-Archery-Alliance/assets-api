package s3

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
)

func setupLocalStack(t *testing.T) (*s3.Client, string, func()) {
	ctx := context.Background()

	localstackContainer, err := localstack.Run(ctx, "localstack/localstack:3.0")
	if err != nil {
		t.Fatalf("Failed to start LocalStack container: %v", err)
	}

	cleanup := func() {
		if err := testcontainers.TerminateContainer(localstackContainer); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}

	mappedPort, err := localstackContainer.MappedPort(ctx, "4566/tcp")
	if err != nil {
		cleanup()
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	hostIP, err := localstackContainer.Host(ctx)
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

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	bucketName := "test-bucket"
	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		cleanup()
		t.Fatalf("Failed to create bucket: %v", err)
	}

	return client, bucketName, cleanup
}

func TestNewStorage(t *testing.T) {
	client, bucketName, cleanup := setupLocalStack(t)
	defer cleanup()

	cdnBaseURL := "https://cdn.example.com"
	storage := NewStorage(client, bucketName, cdnBaseURL)

	if storage == nil {
		t.Fatal("NewStorage() returned nil")
	}
	if storage.client != client {
		t.Error("client not set correctly")
	}
	if storage.bucketName != bucketName {
		t.Errorf("bucketName = %q, want %q", storage.bucketName, bucketName)
	}
	if storage.cdnBaseURL != cdnBaseURL {
		t.Errorf("cdnBaseURL = %q, want %q", storage.cdnBaseURL, cdnBaseURL)
	}
}

func TestStorage_GenerateObjectKey(t *testing.T) {
	client, bucketName, cleanup := setupLocalStack(t)
	defer cleanup()

	storage := NewStorage(client, bucketName, "https://cdn.example.com")

	assetID := uuid.New()
	objectKey := storage.GenerateObjectKey(assetID)

	if objectKey != assetID.String() {
		t.Errorf("GenerateObjectKey() = %q, want %q", objectKey, assetID.String())
	}
}

func TestStorage_GeneratePresignedUploadURL(t *testing.T) {
	client, bucketName, cleanup := setupLocalStack(t)
	defer cleanup()

	storage := NewStorage(client, bucketName, "https://cdn.example.com")
	ctx := context.Background()

	assetID := uuid.New()
	contentType := "text/plain"
	ttl := 1 * time.Hour
	maxFileSize := 10 * 1024 * 1024

	result, err := storage.GeneratePresignedUploadURL(ctx, assetID, contentType, ttl, maxFileSize)
	if err != nil {
		t.Fatalf("GeneratePresignedUploadURL() error = %v", err)
	}

	if result.UploadURL == "" {
		t.Error("UploadURL is empty")
	}

	if len(result.FormFields) == 0 {
		t.Error("FormFields is empty")
	}

	if result.FormFields["key"] != assetID.String() {
		t.Errorf("FormFields[key] = %q, want %q", result.FormFields["key"], assetID.String())
	}

	if result.FormFields["Content-Type"] != contentType {
		t.Errorf("FormFields[Content-Type] = %q, want %q", result.FormFields["Content-Type"], contentType)
	}

	if result.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt is in the past")
	}

	expectedExpiresAt := time.Now().Add(ttl)
	if result.ExpiresAt.After(expectedExpiresAt.Add(1 * time.Minute)) {
		t.Error("ExpiresAt is too far in the future")
	}
}

func TestStorage_HeadObject_Exists(t *testing.T) {
	client, bucketName, cleanup := setupLocalStack(t)
	defer cleanup()

	storage := NewStorage(client, bucketName, "https://cdn.example.com")
	ctx := context.Background()

	assetID := uuid.New()
	objectKey := storage.GenerateObjectKey(assetID)
	contentType := "text/plain"
	content := "Hello, World!"

	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(objectKey),
		Body:        aws.ReadSeekCloser(aws.NewReadCloser([]byte(content))),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		t.Fatalf("Failed to put object: %v", err)
	}

	result, err := storage.HeadObject(ctx, assetID)
	if err != nil {
		t.Fatalf("HeadObject() error = %v", err)
	}

	if !result.Exists {
		t.Error("HeadObject() Exists = false, want true")
	}

	if result.Size != int64(len(content)) {
		t.Errorf("HeadObject() Size = %d, want %d", result.Size, len(content))
	}

	if result.ContentType != contentType {
		t.Errorf("HeadObject() ContentType = %q, want %q", result.ContentType, contentType)
	}
}

func TestStorage_HeadObject_NotExists(t *testing.T) {
	client, bucketName, cleanup := setupLocalStack(t)
	defer cleanup()

	storage := NewStorage(client, bucketName, "https://cdn.example.com")
	ctx := context.Background()

	assetID := uuid.New()

	result, err := storage.HeadObject(ctx, assetID)
	if err != nil {
		t.Fatalf("HeadObject() error = %v", err)
	}

	if result.Exists {
		t.Error("HeadObject() Exists = true, want false for non-existent object")
	}
}

func TestStorage_DeleteObject(t *testing.T) {
	client, bucketName, cleanup := setupLocalStack(t)
	defer cleanup()

	storage := NewStorage(client, bucketName, "https://cdn.example.com")
	ctx := context.Background()

	assetID := uuid.New()
	objectKey := storage.GenerateObjectKey(assetID)

	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
		Body:   aws.ReadSeekCloser(aws.NewReadCloser([]byte("test content"))),
	})
	if err != nil {
		t.Fatalf("Failed to put object: %v", err)
	}

	result, err := storage.HeadObject(ctx, assetID)
	if err != nil {
		t.Fatalf("HeadObject() error = %v", err)
	}
	if !result.Exists {
		t.Fatal("Object should exist before deletion")
	}

	err = storage.DeleteObject(ctx, assetID)
	if err != nil {
		t.Fatalf("DeleteObject() error = %v", err)
	}

	result, err = storage.HeadObject(ctx, assetID)
	if err != nil {
		t.Fatalf("HeadObject() error = %v", err)
	}
	if result.Exists {
		t.Error("Object should not exist after deletion")
	}
}

func TestStorage_DeleteObject_NotExists(t *testing.T) {
	client, bucketName, cleanup := setupLocalStack(t)
	defer cleanup()

	storage := NewStorage(client, bucketName, "https://cdn.example.com")
	ctx := context.Background()

	assetID := uuid.New()

	err := storage.DeleteObject(ctx, assetID)
	if err != nil {
		t.Fatalf("DeleteObject() should not error for non-existent object, got: %v", err)
	}
}
