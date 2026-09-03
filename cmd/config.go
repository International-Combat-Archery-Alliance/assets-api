package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/International-Combat-Archery-Alliance/assets-api/api"
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
	// JWKSURL is the login JWKS endpoint used to verify user tokens.
	JWKSURL string
}

func fetchAppConfig(ctx context.Context, env api.Environment) (*AppConfig, error) {
	if env == api.LOCAL {
		return localAppConfig()
	}
	return fetchProdAppConfig(ctx)
}

func localAppConfig() (*AppConfig, error) {
	return &AppConfig{
		JWKSURL: jwksURLForEnv(api.LOCAL),
	}, nil
}

func fetchProdAppConfig(_ context.Context) (*AppConfig, error) {
	return &AppConfig{
		JWKSURL: jwksURLForEnv(api.PROD),
	}, nil
}

// jwksURLForEnv returns the login JWKS endpoint used to verify user tokens.
// LOGIN_JWKS_URL overrides both environments.
func jwksURLForEnv(env api.Environment) string {
	if u := os.Getenv("LOGIN_JWKS_URL"); u != "" {
		return u
	}
	if env == api.LOCAL {
		return "http://localhost:3001/login/.well-known/jwks.json"
	}
	return "https://api.icaa.world/login/.well-known/jwks.json"
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
