package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/International-Combat-Archery-Alliance/assets-api/api"
	"github.com/International-Combat-Archery-Alliance/assets-api/dynamo"
	s3storage "github.com/International-Combat-Archery-Alliance/assets-api/s3"
	"github.com/International-Combat-Archery-Alliance/auth/google"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	db, err := makeDB(ctx)
	if err != nil {
		logger.Error("Error creating db client", "error", err)
		os.Exit(1)
	}

	_, err = db.EnsureRootFolderExists(ctx, "machine")
	if err != nil {
		logger.Error("Error making root folder", "error", err)
		os.Exit(1)
	}

	storage, err := makeStorage(ctx)
	if err != nil {
		logger.Error("Error creating storage client", "error", err)
		os.Exit(1)
	}

	googleAuthValidator, err := google.NewValidator(ctx)
	if err != nil {
		logger.Error("failed to create google auth validator", slog.String("error", err.Error()))
		os.Exit(1)
	}

	env := getApiEnvironment()
	cdnBaseURL := getEnvOrDefault("ASSETS_CDN_BASE_URL", "https://assets.icaa.world")

	assetsAPI := api.NewAPI(db, storage, logger, env, cdnBaseURL, googleAuthValidator)

	serverSettings := getServerSettingsFromEnv()
	err = assetsAPI.ListenAndServe(serverSettings.Host, serverSettings.Port)
	if err != nil && err != http.ErrServerClosed {
		logger.Error("error running server", "error", err)
		os.Exit(1)
	}
	logger.Info("shutting down")
}

type ServerSettings struct {
	Host string
	Port string
}

func getServerSettingsFromEnv() ServerSettings {
	return ServerSettings{
		Host: getEnvOrDefault("HOST", "0.0.0.0"),
		Port: getEnvOrDefault("PORT", "8080"),
	}
}

func getEnvOrDefault(key string, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultVal
}

func makeDB(ctx context.Context) (*dynamo.DB, error) {
	var dynamoClient *dynamodb.Client
	var err error
	if isLocal() {
		dynamoClient, err = createLocalDynamoClient(ctx)
	} else {
		dynamoClient, err = createProdDynamoClient(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamo client: %w", err)
	}

	database := dynamo.NewDB(dynamoClient, os.Getenv("DYNAMO_TABLE_NAME"))
	return database, nil
}

func makeStorage(ctx context.Context) (*s3storage.Storage, error) {
	var s3Client *s3.Client
	var err error
	if isLocal() {
		s3Client, err = createLocalS3Client(ctx)
	} else {
		s3Client, err = createProdS3Client(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create s3 client: %w", err)
	}

	bucketName := os.Getenv("S3_BUCKET_NAME")
	cdnBaseURL := getEnvOrDefault("ASSETS_CDN_BASE_URL", "https://assets.icaa.world")

	return s3storage.NewStorage(s3Client, bucketName, cdnBaseURL), nil
}

func isLocal() bool {
	return getEnvOrDefault("AWS_SAM_LOCAL", "false") == "true"
}

func getApiEnvironment() api.Environment {
	if isLocal() {
		return api.LOCAL
	}
	return api.PROD
}

func createLocalDynamoClient(ctx context.Context) (*dynamodb.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("localhost"),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID: "local", SecretAccessKey: "local", SessionToken: "",
				Source: "Mock credentials used above for local instance",
			},
		}),
	)
	if err != nil {
		return nil, err
	}

	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://dynamodb:8000")
	}), nil
}

func createProdDynamoClient(ctx context.Context) (*dynamodb.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return dynamodb.NewFromConfig(cfg), nil
}

func createLocalS3Client(ctx context.Context) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID: "local", SecretAccessKey: "local", SessionToken: "",
				Source: "Mock credentials used for local instance",
			},
		}),
	)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://localstack:4566")
		o.UsePathStyle = true
	}), nil
}

func createProdS3Client(ctx context.Context) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg), nil
}
