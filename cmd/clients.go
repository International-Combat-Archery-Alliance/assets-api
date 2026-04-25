package main

import (
	"context"
	"fmt"
	"os"

	"github.com/International-Combat-Archery-Alliance/assets-api/assets"
	"github.com/International-Combat-Archery-Alliance/assets-api/dynamo"
	s3storage "github.com/International-Combat-Archery-Alliance/assets-api/s3"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

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

	return dynamo.NewDB(dynamoClient, os.Getenv("DYNAMO_TABLE_NAME")), nil
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

func makeAssetsManager(storage *s3storage.Storage, db *dynamo.DB) *assets.AssetsManager {
	return assets.NewAssetsManager(storage, db)
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
	cfg, err := loadAWSConfig(ctx)
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
				AccessKeyID: "local", SecretAccessKey: "locallocal", SessionToken: "",
				Source: "Mock credentials used for local instance",
			},
		}),
	)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://minio:9000")
		o.UsePathStyle = true
	}), nil
}

func createProdS3Client(ctx context.Context) (*s3.Client, error) {
	cfg, err := loadAWSConfig(ctx)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg), nil
}
