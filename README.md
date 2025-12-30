# ICAA Assets API

An API for managing assets (images, documents) for the ICAA website. Built with Go and deployed as an AWS Lambda function using AWS SAM.

## Tech Stack

- **Language**: Go 1.25+
- **Infrastructure**: AWS Lambda, API Gateway, DynamoDB, S3
- **Deployment**: AWS SAM (Serverless Application Model)
- **API Spec**: OpenAPI 3.0 with code generation via [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)
- **Authentication**: Google OAuth (cookie and bearer token)

## API Endpoints

| Method | Endpoint | Description | Auth Required |
|--------|----------|-------------|---------------|
| GET | `/assets/v1` | List all assets (with optional folder filter and pagination) | No |
| GET | `/assets/v1/folders` | List all distinct folders | No |
| GET | `/assets/v1/{id}` | Get a single asset by ID | No |
| POST | `/assets/v1/upload-url` | Get a presigned S3 upload URL | Admin |
| POST | `/assets/v1/{id}/confirm` | Confirm an asset upload | Admin |
| DELETE | `/assets/v1/{id}` | Delete an asset | Admin |

## Prerequisites

- Go 1.25+
- [AWS SAM CLI](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/install-sam-cli.html)
- Docker
- AWS CLI (configured with appropriate credentials for deployment)

## Local Development

1. **Start local infrastructure** (DynamoDB Local and LocalStack for S3):

   ```bash
   docker-compose up -d
   ```

2. **Build and run the API locally**:

   ```bash
   make local
   ```

   This will:
   - Generate API code from the OpenAPI spec
   - Build the SAM application
   - Start the local API server with hot reloading

3. **Create an `env.json` file** for local environment variables:

   ```json
   {
     "ICAAAssets": {
       "DYNAMO_TABLE_NAME": "assets-api",
       "S3_BUCKET_NAME": "icaa-assets",
       "ASSETS_CDN_BASE_URL": "http://localhost:4566/icaa-assets"
     }
   }
   ```

The local API will be available at `http://localhost:3000`.

## Building

```bash
make build
```

This generates the API code from the OpenAPI spec and builds the SAM application.

## Project Structure

```
├── api/              # API handlers and server setup
│   ├── api.go        # Main API implementation
│   ├── gen.go        # Generated OpenAPI code
│   └── middleware.go # HTTP middleware (auth, CORS, validation)
├── assets/           # Asset domain logic
├── cmd/
│   └── main.go       # Application entrypoint
├── dynamo/           # DynamoDB client and operations
├── s3/               # S3 storage client
├── ptr/              # Pointer utility helpers
├── spec/
│   └── api.yaml      # OpenAPI specification
├── template.yml      # AWS SAM template
├── docker-compose.yml # Local development services
├── Dockerfile        # Container image for Lambda
└── Makefile          # Build commands
```

## Deployment

The API is deployed via AWS SAM. The CI/CD pipeline is configured in `.github/workflows/go.yml`.

For manual deployment:

```bash
sam deploy --guided
```

## Configuration

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `DYNAMO_TABLE_NAME` | DynamoDB table name | `assets-api` |
| `S3_BUCKET_NAME` | S3 bucket for asset storage | `icaa-assets` |
| `ASSETS_CDN_BASE_URL` | Base URL for CDN asset URLs | `https://assets.icaa.world` |
| `HOST` | Server host | `0.0.0.0` |
| `PORT` | Server port | `8080` |

## License

See [LICENSE](LICENSE) for details.
