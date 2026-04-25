# ICAA Assets API

An API for managing assets (images, documents) for the ICAA website. Built with Go and deployed as an AWS Lambda function using AWS SAM.

## Tech Stack

- **Language**: Go 1.25+
- **Infrastructure**: AWS Lambda, API Gateway, DynamoDB, S3
- **Deployment**: AWS SAM (Serverless Application Model)
- **API Spec**: OpenAPI 3.0 with code generation via [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)
- **Authentication**: Google OAuth (cookie and bearer token)

## Prerequisites

- Go 1.25+
- [AWS SAM CLI](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/install-sam-cli.html)
- Docker
- AWS CLI (configured with appropriate credentials for deployment)

## Local Development

1. **Create an `env.json` file** for local environment variables:

   ```json
   {
     "ICAAAssets": {
       "DYNAMO_TABLE_NAME": "assets-api",
       "S3_BUCKET_NAME": "icaa-assets",
       "ASSETS_CDN_BASE_URL": "http://localhost:9000/icaa-assets"
     }
   }
   ```
2. **Start shared infrastructure**:
    Shared infrastructure (DynamoDB, Jaeger, MinIO) is managed in `icaa.world/docker-compose.yml`.
   ```bash
   cd ../icaa.world && docker compose up -d
   ```

3. **Build and run the API locally**:

   ```bash
   make local
   ```

   This will:
   - Generate API code from the OpenAPI spec
   - Build the SAM application
   - Start the local API server with hot reloading

The local API will be available at `http://localhost:3002`.

## Building

```bash
make build
```

This generates the API code from the OpenAPI spec and builds the SAM application.

## Deployment

The API is deployed via AWS SAM. The CI/CD pipeline is configured in `.github/workflows/go.yml`.

For manual deployment:

```bash
sam deploy --guided
```

## Configuration

| Environment Variable  | Description                 | Default                     |
| --------------------- | --------------------------- | --------------------------- |
| `DYNAMO_TABLE_NAME`   | DynamoDB table name         | `assets-api`                |
| `S3_BUCKET_NAME`      | S3 bucket for asset storage | `icaa-assets`               |
| `ASSETS_CDN_BASE_URL` | Base URL for CDN asset URLs | `https://assets.icaa.world` |
| `HOST`                | Server host                 | `0.0.0.0`                   |
| `PORT`                | Server port                 | `8080`                      |

## License

See [LICENSE](LICENSE) for details.
