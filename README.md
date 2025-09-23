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
- Go 1.25+
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
Configuration options can be set via the following environment variables:

- `POSTGRES_URI`: Connection URI for PostgreSQL (e.g. `postgres://admin:adminpass@postgres:5432/broker?sslmode=disable`)
- `DRAGONFLY_URI`: Connection URI for Dragonfly (e.g. `redis://:adminpass@dragonfly:6379/0`)
- `SERVER_PORT`: Port for the broker API server (default: `8080`)

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

## License
This project is licensed under the MIT License.
