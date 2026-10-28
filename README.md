# Database Broker

Database Broker is a Go-based service for provisioning instances of dedicated databases/namespaces from a shared database server. It supports multiple database backends and provides a unified interface for database operations, making it easy to integrate with cloud-native environments and automation workflows.

## Features
- Provision database instances
- Support for multiple backends: PostgreSQL, Redis (via Dragonfly)
- RESTful API for database operations
- Configuration via environment variables and config files
- Docker support for easy deployment

## Project Structure
```
cmd/database-broker/      # Main entry point for the broker service
internal/adapter/         # Database adapters: PostgreSQL, Dragonfly (Redis with namespaces)
internal/broker/          # Core broker logic
internal/config/          # Configuration management
internal/router/          # HTTP routing and controllers
internal/util/            # Utility functions
Dockerfile                # Container build file
docker-compose.yml        # Multi-container orchestration for testing
```

## Getting Started

### Prerequisites
- Go 1.27+
- Docker (required for tests and containerized deployment)

### Build and Run Locally
```bash
git clone https://github.com/razzie-cloud/database-broker.git
cd database-broker
go build -o broker ./cmd/database-broker
./broker
```

### Run with Docker
```bash
docker compose up --build
```

## Configuration
The service can be configured via the following optional environment variables:
- `SERVICE_PORT`: Port for the broker API server (default: `8080`)
- `POSTGRES_URI`: Connection URI for PostgreSQL (e.g. `postgres://admin:adminpass@postgres:5432/broker?sslmode=disable`)
- `POSTGRES_PASSWORD_FILE`: Read PostgreSQL password from file
- `DRAGONFLY_URI`: Connection URI for DragonflyDB (e.g. `redis://:adminpass@dragonfly:6379/0`)
- `DRAGONFLY_PASSWORD_FILE`: Read DragonflyDB password from file

Or via the matching command line flags:
- `--port`
- `--postgres-uri`
- `--postgres-password-file`
- `--dragonfly-uri`
- `--dragonfly-password-file`

## API Usage

The broker exposes a RESTful API for managing database instances:

* **GET** `/v1/instances/{adapter_name}`

  Returns a list of database instances for the specified adapter (e.g., `postgres`, `redis`, `dragonfly`) in a JSON string array.

* **GET** `/v1/instances/{adapter_name}/{instance_name}`

  Returns details for a specific database instance in JSON format. It creates the instance if it did not exist before.

* **GET** `/v1/instances/{adapter_name}/{instance_name}/uri`

  Returns the connection URI for a specific database instance. It creates the instance if it did not exist before.

## Testing
Run unit tests with:
```bash
go test ./...
```

## Publish an image

The `Publish image` workflow builds the tagged commit's Dockerfile for
`linux/amd64` and pushes `ghcr.io/razzie-cloud/database-broker:<git-tag>` when a
new tag is pushed to this service repository. For example, from this directory:

```sh
git tag v1.2.3
git push origin v1.2.3
```

The tagged commit must include `.github/workflows/publish-image.yml`. Creating
a new tag through GitHub's release UI also triggers the workflow. Tags in the
parent razcloud repository do not trigger service builds.

The image tag exactly matches the Git tag, including any `v` prefix; no `latest`
alias is added. Use Docker-compatible tags: 1–128 ASCII letters, digits,
underscores, dots, or hyphens, starting with a letter, digit, or underscore.
Incompatible tags fail before publication.

Authentication uses the built-in `GITHUB_TOKEN` with `packages: write`; no custom
registry secret is needed. Organization policy must allow package creation;
existing packages must grant this repository Actions write access. Set package
visibility to public for unauthenticated pulls, or authenticate deployment hosts.

## License
This project is licensed under the MIT License.
