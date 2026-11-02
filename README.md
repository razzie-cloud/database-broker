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
new tag is pushed to this service repository. Set `RELEASE_TAG` to your chosen
release tag, then run from this directory:

```sh
git tag "${RELEASE_TAG:?Set RELEASE_TAG}"
git push origin "$RELEASE_TAG"
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

## Health probes

Internal `GET /healthz` reports process liveness; `GET /readyz` reports readiness
of all configured backends (PostgreSQL and/or Dragonfly). The redis alias is
checked only once.
Both support HEAD and return plain text: 200 on success, 503 when not ready,
and 405 for other methods. Responses use `Cache-Control: no-store` and never
include backend addresses, credentials, or error details.

Readiness checks run immediately in a background worker and repeat ten seconds
after each cycle, with a two-second deadline per dependency. Until the first
successful cycle, after a failed cycle, or once the last result is thirty seconds
old, readiness returns 503. Probes only read cached state; they never query a
backend, provision an instance, or generate request logs. Failed background
checks log dependency failure and recovery transitions without raw backend errors.
With no configured backends, liveness succeeds and readiness returns 503.

Use these endpoints directly on the private backend port. Keep public ingress
and authentication rules in place. Container health checks use `/readyz` on
`SERVICE_PORT` (default 8080).

## Logging

Logs are JSON records written to stderr using Go's `log/slog`. Each record has
`time`, `level`, `msg`, `service`, and a stable `event` name. `LOG_LEVEL` controls
verbosity independently of service configuration: `DEBUG`, `INFO` (default),
`WARN`, or `ERROR`, case-insensitive. Invalid values fail startup.

Operational failures include a generated `request_id` (also returned as
`X-Request-ID`), method, static route template, status, duration in milliseconds,
operation, and a safe `error_kind`. Known database errors include their SQLSTATE;
raw backend error messages and SQL are excluded. Request URLs, query strings,
cookies, bodies, credentials, and panic values are not logged. Panic diagnostics
include code locations only.

Routine requests are quiet at INFO. DEBUG enables request summaries; static
assets and health probes remain quiet. Dependency outages emit one
`dependency.unavailable` WARN and a `dependency.recovered` INFO with outage
duration, rather than repeating a warning every probe cycle. Normal canceled
requests and interrupted client downloads are DEBUG events.

`server.listening` lists enabled adapters; `broker.no_backends` warns once if
none are configured. New instances emit `instance.provisioned` only after their
backend mutations succeed. Existing-instance lookups and lists are quiet at
INFO. Failures identify the provisioning stage and validated instance name
(bounded to 128 bytes), never generated credentials or connection URIs. The
`redis` alias is attributed to the underlying `dragonfly` adapter for provisioning.
