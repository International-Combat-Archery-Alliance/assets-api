package s3

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupMinIO(t *testing.T) (*s3.Client, string, func()) {
	ctx := context.Background()
	bucketName := "test-bucket-" + uuid.New().String()

	if _, ok := os.LookupEnv("TEST_IN_CI"); ok {
		return setupMinIOInCI(t, ctx, bucketName)
	}
	return setupMinIOTestContainers(t, ctx, bucketName)
}

func setupMinIOInCI(t *testing.T, ctx context.Context, bucketName string) (*s3.Client, string, func()) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     "minioadmin",
				SecretAccessKey: "minioadmin",
				SessionToken:    "",
				Source:          "Mock credentials used for local instance",
			},
		}),
	)
	if err != nil {
		t.Fatalf("Failed to load AWS config: %v", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://localhost:9000")
		o.UsePathStyle = true
	})

	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}

	cleanup := func() {
		_, _ = client.DeleteBucket(ctx, &s3.DeleteBucketInput{
			Bucket: aws.String(bucketName),
		})
	}

	return client, bucketName, cleanup
}

func setupMinIOTestContainers(t *testing.T, ctx context.Context, bucketName string) (*s3.Client, string, func()) {
	req := testcontainers.ContainerRequest{
		Image:        "minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		Cmd:          []string{"server", "/data"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minioadmin",
			"MINIO_ROOT_PASSWORD": "minioadmin",
		},
		WaitingFor: wait.ForHTTP("/minio/health/live").WithPort("9000/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start MinIO container: %v", err)
	}

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}

	mappedPort, err := container.MappedPort(ctx, "9000/tcp")
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
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("minioadmin", "minioadmin", "")),
	)
	if err != nil {
		cleanup()
		t.Fatalf("Failed to load AWS config: %v", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

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
	client, bucketName, cleanup := setupMinIO(t)
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
	client, bucketName, cleanup := setupMinIO(t)
	defer cleanup()

	storage := NewStorage(client, bucketName, "https://cdn.example.com")

	assetID := uuid.New()
	filename := "test.txt"
	objectKey := storage.GenerateObjectKey(assetID, filename)

	expected := assetID.String() + ".txt"
	if objectKey != expected {
		t.Errorf("GenerateObjectKey() = %q, want %q", objectKey, expected)
	}
}

func TestStorage_GeneratePresignedUploadURL(t *testing.T) {
	client, bucketName, cleanup := setupMinIO(t)
	defer cleanup()

	storage := NewStorage(client, bucketName, "https://cdn.example.com")
	ctx := context.Background()

	assetID := uuid.New()
	filename := "test.txt"
	contentType := "text/plain"
	ttl := 1 * time.Hour
	maxFileSize := 10 * 1024 * 1024

	result, err := storage.GeneratePresignedUploadURL(ctx, assetID, filename, contentType, ttl, maxFileSize)
	if err != nil {
		t.Fatalf("GeneratePresignedUploadURL() error = %v", err)
	}

	if result.UploadURL == "" {
		t.Error("UploadURL is empty")
	}

	if len(result.FormFields) == 0 {
		t.Error("FormFields is empty")
	}

	expectedKey := assetID.String() + ".txt"
	if result.FormFields["key"] != expectedKey {
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
	client, bucketName, cleanup := setupMinIO(t)
	defer cleanup()

	storage := NewStorage(client, bucketName, "https://cdn.example.com")
	ctx := context.Background()

	assetID := uuid.New()
	filename := "test.txt"
	objectKey := storage.GenerateObjectKey(assetID, filename)
	contentType := "text/plain"
	content := "Hello, World!"

	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(objectKey),
		Body:        bytes.NewReader([]byte(content)),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		t.Fatalf("Failed to put object: %v", err)
	}

	result, err := storage.HeadObject(ctx, objectKey)
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
	client, bucketName, cleanup := setupMinIO(t)
	defer cleanup()

	storage := NewStorage(client, bucketName, "https://cdn.example.com")
	ctx := context.Background()

	assetID := uuid.New()
	filename := "test.txt"
	objectKey := storage.GenerateObjectKey(assetID, filename)

	result, err := storage.HeadObject(ctx, objectKey)
	if err != nil {
		t.Fatalf("HeadObject() error = %v", err)
	}

	if result.Exists {
		t.Error("HeadObject() Exists = true, want false for non-existent object")
	}
}

func TestStorage_DeleteObject(t *testing.T) {
	client, bucketName, cleanup := setupMinIO(t)
	defer cleanup()

	storage := NewStorage(client, bucketName, "https://cdn.example.com")
	ctx := context.Background()

	assetID := uuid.New()
	filename := "test.txt"
	objectKey := storage.GenerateObjectKey(assetID, filename)

	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectKey),
		Body:   bytes.NewReader([]byte("test content")),
	})
	if err != nil {
		t.Fatalf("Failed to put object: %v", err)
	}

	result, err := storage.HeadObject(ctx, objectKey)
	if err != nil {
		t.Fatalf("HeadObject() error = %v", err)
	}
	if !result.Exists {
		t.Fatal("Object should exist before deletion")
	}

	err = storage.DeleteObject(ctx, objectKey)
	if err != nil {
		t.Fatalf("DeleteObject() error = %v", err)
	}

	result, err = storage.HeadObject(ctx, objectKey)
	if err != nil {
		t.Fatalf("HeadObject() error = %v", err)
	}
	if result.Exists {
		t.Error("Object should not exist after deletion")
	}
}

func TestStorage_DeleteObject_NotExists(t *testing.T) {
	client, bucketName, cleanup := setupMinIO(t)
	defer cleanup()

	storage := NewStorage(client, bucketName, "https://cdn.example.com")
	ctx := context.Background()

	assetID := uuid.New()
	filename := "test.txt"
	objectKey := storage.GenerateObjectKey(assetID, filename)

	err := storage.DeleteObject(ctx, objectKey)
	if err != nil {
		t.Fatalf("DeleteObject() should not error for non-existent object, got: %v", err)
	}
}
