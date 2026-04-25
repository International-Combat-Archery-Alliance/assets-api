package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/International-Combat-Archery-Alliance/assets-api/api"
	"github.com/International-Combat-Archery-Alliance/auth/token"
	"github.com/International-Combat-Archery-Alliance/telemetry"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"go.opentelemetry.io/otel/codes"
)

const (
	newRelicLicenseEnvVar  = "NEW_RELIC_LICENSE_KEY"
	newRelicLicenseSSMPath = "/newrelic-license-key"
)

var (
	awsCfg     aws.Config
	awsCfgErr  error
	awsCfgOnce sync.Once
)

func loadAWSConfig(ctx context.Context) (aws.Config, error) {
	awsCfgOnce.Do(func() {
		ctx, span := tracer.Start(ctx, "load-aws-config")
		defer span.End()

		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			awsCfgErr = fmt.Errorf("unable to load AWS SDK config: %w", err)
			return
		}
		telemetry.InstrumentAWSConfig(&cfg)
		awsCfg = cfg
	})
	return awsCfg, awsCfgErr
}

func getSSMParameter(ctx context.Context, name string) (string, error) {
	cfg, err := loadAWSConfig(ctx)
	if err != nil {
		return "", err
	}

	client := ssm.NewFromConfig(cfg)
	result, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get parameter %q: %w", name, err)
	}

	return aws.ToString(result.Parameter.Value), nil
}

type AppConfig struct {
	JWTSigningKeys  map[string]token.SigningKey
	JWTCurrentKeyID string
}

func fetchAppConfig(ctx context.Context, env api.Environment) (*AppConfig, error) {
	if env == api.LOCAL {
		return localAppConfig()
	}
	return fetchProdAppConfig(ctx)
}

func localAppConfig() (*AppConfig, error) {
	key := os.Getenv("JWT_SIGNING_KEY")
	if key == "" {
		key = "local-development-signing-key-minimum-32-characters-long"
	}

	return &AppConfig{
		JWTSigningKeys: map[string]token.SigningKey{
			"local": {ID: "local", Key: []byte(key)},
		},
		JWTCurrentKeyID: "local",
	}, nil
}

func fetchProdAppConfig(ctx context.Context) (*AppConfig, error) {
	signingKeysJSON, err := getSSMParameter(ctx, "/jwtSigningKeys")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWT signing keys from SSM: %w", err)
	}

	signingKeys, currentKeyID, err := parseJWTSigningKeysJSON(signingKeysJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT signing keys: %w", err)
	}

	return &AppConfig{
		JWTSigningKeys:  signingKeys,
		JWTCurrentKeyID: currentKeyID,
	}, nil
}

type jwtSigningKeysData struct {
	CurrentKey string            `json:"currentKey"`
	Keys       map[string]string `json:"keys"`
}

func parseJWTSigningKeysJSON(raw string) (map[string]token.SigningKey, string, error) {
	var data jwtSigningKeysData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, "", fmt.Errorf("failed to parse JWT signing keys JSON: %w", err)
	}

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

	if _, ok := signingKeys[data.CurrentKey]; !ok {
		return nil, "", fmt.Errorf("current key ID %q not found in keys", data.CurrentKey)
	}

	return signingKeys, data.CurrentKey, nil
}

func getNewRelicLicenseKey(ctx context.Context, env api.Environment) (string, error) {
	if env == api.LOCAL {
		return os.Getenv(newRelicLicenseEnvVar), nil
	}
	return getSSMParameter(ctx, newRelicLicenseSSMPath)
}

func getApiEnvironment() api.Environment {
	if isLocal() {
		return api.LOCAL
	}
	return api.PROD
}

func isLocal() bool {
	return getEnvOrDefault("AWS_SAM_LOCAL", "false") == "true"
}

func getEnvOrDefault(key, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultVal
}
