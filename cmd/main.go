package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/International-Combat-Archery-Alliance/assets-api/api"
	"github.com/International-Combat-Archery-Alliance/assets-api/assets"
	"github.com/International-Combat-Archery-Alliance/assets-api/dynamo"
	s3storage "github.com/International-Combat-Archery-Alliance/assets-api/s3"
	"github.com/International-Combat-Archery-Alliance/assets-api/telemetry"
	"github.com/International-Combat-Archery-Alliance/auth/token"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	traceShutdown, err := telemetry.Init(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize telemetry: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := traceShutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "failed to shutdown telemetry: %v\n", err)
		}
	}()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	db, err := makeDB(ctx)
	if err != nil {
		logger.Error("Error creating db client", "error", err)
		os.Exit(1)
	}

	err = db.EnsureRootFolderExists(ctx, "machine")
	if err != nil {
		logger.Error("Error making root folder", "error", err)
		os.Exit(1)
	}

	storage, err := makeStorage(ctx)
	if err != nil {
		logger.Error("Error creating storage client", "error", err)
		os.Exit(1)
	}

	assetsManager := assets.NewAssetsManager(storage, db)

	env := getApiEnvironment()

	signingKeys, currentKeyID, err := getJWTSigningKeys(ctx, env)
	if err != nil {
		logger.Error("failed to get JWT signing keys", slog.String("error", err.Error()))
		os.Exit(1)
	}

	tokenService := token.NewTokenService(
		signingKeys[currentKeyID],
		token.WithSigningKeys(signingKeys, currentKeyID),
	)

	cdnBaseURL := getEnvOrDefault("ASSETS_CDN_BASE_URL", "https://assets.icaa.world")

	assetsAPI := api.NewAPI(assetsManager, logger, env, cdnBaseURL, tokenService)

	serverSettings := getServerSettingsFromEnv()

	sigCtx, sigStop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer sigStop()

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- assetsAPI.ListenAndServe(serverSettings.Host, serverSettings.Port)
	}()

	select {
	case <-sigCtx.Done():
		logger.Info("shutting down gracefully")
	case err := <-serverErrCh:
		logger.Error("error running server", "error", err)
		os.Exit(1)
	}
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

	otelaws.AppendMiddlewares(&cfg.APIOptions)

	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://dynamodb:8000")
	}), nil
}

func createProdDynamoClient(ctx context.Context) (*dynamodb.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	otelaws.AppendMiddlewares(&cfg.APIOptions)

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

	otelaws.AppendMiddlewares(&cfg.APIOptions)

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://minio:9000")
		o.UsePathStyle = true
	}), nil
}

func createProdS3Client(ctx context.Context) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	otelaws.AppendMiddlewares(&cfg.APIOptions)

	return s3.NewFromConfig(cfg), nil
}

// jwtSigningKeysData represents the JSON structure for signing keys
type jwtSigningKeysData struct {
	CurrentKey string            `json:"currentKey"`
	Keys       map[string]string `json:"keys"`
}

// getJWTSigningKeys retrieves the JWT signing keys from environment variable (local)
// or AWS Parameter Store (production)
func getJWTSigningKeys(ctx context.Context, env api.Environment) (map[string]token.SigningKey, string, error) {
	if env == api.LOCAL {
		// Local development: use environment variable
		key := os.Getenv("JWT_SIGNING_KEY")
		if key == "" {
			key = "local-development-signing-key-minimum-32-characters-long"
		}
		return map[string]token.SigningKey{
			"local": {ID: "local", Key: []byte(key)},
		}, "local", nil
	}

	// Production: retrieve from AWS Parameter Store
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	client := ssm.NewFromConfig(cfg)

	result, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String("/jwtSigningKeys"),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get JWT signing keys from Parameter Store: %w", err)
	}

	// Parse JSON response
	var data jwtSigningKeysData
	if err := json.Unmarshal([]byte(*result.Parameter.Value), &data); err != nil {
		return nil, "", fmt.Errorf("failed to parse JWT signing keys JSON: %w", err)
	}

	// Convert to map of SigningKey (keys are base64 encoded)
	signingKeys := make(map[string]token.SigningKey)
	for keyID, keyValue := range data.Keys {
		decodedKey, err := base64.StdEncoding.DecodeString(keyValue)
		if err != nil {
			return nil, "", fmt.Errorf("failed to decode base64 key %q: %w", keyID, err)
		}
		signingKeys[keyID] = token.SigningKey{
			ID:  keyID,
			Key: decodedKey,
		}
	}

	// Validate that current key exists
	if _, ok := signingKeys[data.CurrentKey]; !ok {
		return nil, "", fmt.Errorf("current key ID %q not found in keys", data.CurrentKey)
	}

	return signingKeys, data.CurrentKey, nil
}
