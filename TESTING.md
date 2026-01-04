# Testing Guide

This repository contains comprehensive unit and integration tests for all packages.

## Test Structure

### Unit Tests
- **ptr**: Tests for pointer utility functions
- **assets**: Tests for domain models, error types, and the AssetsManager business logic
- **api**: Tests for HTTP handlers and request/response transformations

### Integration Tests
- **s3**: Integration tests using LocalStack testcontainer to test S3 operations
- **dynamo**: Integration tests using DynamoDB Local testcontainer to test database operations

## Running Tests

### Prerequisites
- Go 1.25.1 or later
- Docker (required for integration tests with testcontainers)

### Run All Tests
```bash
go test ./... -v
```

### Run Unit Tests Only
```bash
# Run tests for specific packages
go test ./ptr/... -v
go test ./assets/... -v
go test ./api/... -v
```

### Run Integration Tests
```bash
# S3 integration tests (requires Docker)
go test ./s3/... -v

# DynamoDB integration tests (requires Docker)
go test ./dynamo/... -v
```

### Run Tests with Coverage
```bash
go test ./... -cover -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

## Test Dependencies

The integration tests use [Testcontainers for Go](https://golang.testcontainers.org/) to spin up:
- **LocalStack** for S3 integration tests
- **DynamoDB Local** for DynamoDB integration tests

These containers are automatically started and stopped during test execution.

## Package Test Coverage

### ptr Package
- Tests all pointer utility functions (Int, Int64, String, Duration, Time)
- Validates correct pointer creation and value preservation

### assets Package
- **errors_test.go**: Tests all error types and error checking functions
- **assets_test.go**: Tests domain models (File, Folder) and their methods
- **assetsmanager_test.go**: Tests business logic using mocks for repositories

### s3 Package
- Tests S3 storage operations against a real LocalStack S3 instance
- Tests presigned URL generation, object head, and object deletion
- Validates error handling for non-existent objects

### dynamo Package
- **assets_test.go**: Integration tests for CRUD operations on assets
- **db_test.go**: Unit tests for helper functions (cursor encoding/decoding)
- Tests pagination, optimistic locking, and transactional content count updates
- Validates all error scenarios (not found, version conflicts, folder not empty, etc.)

### api Package
- Tests HTTP handler logic using mocks for AssetsManager
- Tests request validation and error response formatting
- Tests conversion between domain models and API DTOs

## Test Patterns

### Mocking
Unit tests use simple mock implementations defined within the test files. See:
- `assets/assetsmanager_test.go` for repository mocks
- `api/api_test.go` for AssetsManager mocks

### Integration Testing
Integration tests use testcontainers to provide real AWS service instances:
```go
// Example from s3/storage_test.go
localstackContainer, err := localstack.Run(ctx, "localstack/localstack:3.0")
```

### Table-Driven Tests
Many tests use table-driven patterns for comprehensive coverage:
```go
tests := []struct {
    name string
    input string
    want string
}{
    {"case1", "input1", "expected1"},
    {"case2", "input2", "expected2"},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // test implementation
    })
}
```

## Continuous Integration

Tests can be run in CI/CD pipelines. Ensure Docker is available for integration tests.

Example GitHub Actions workflow:
```yaml
- name: Run tests
  run: |
    go test ./... -v -race -coverprofile=coverage.txt
```

## Troubleshooting

### Docker Not Available
If Docker is not available, integration tests will fail. You can skip them:
```bash
go test ./... -short
```
(Note: Integration tests should be marked with `testing.Short()` check)

### Testcontainer Timeouts
If testcontainers fail to start, increase the timeout or check Docker daemon status:
```bash
docker ps
```

### Network Issues
Integration tests require network access to pull Docker images (LocalStack, DynamoDB Local). Ensure your environment has internet access or pre-pull images:
```bash
docker pull localstack/localstack:3.0
docker pull amazon/dynamodb-local:latest
```
