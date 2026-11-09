# Database Broker reference

See the [README](../README.md) for the local quickstart.

- [Configuration](#configuration) and [API usage](#api-usage)
- [Native development and checks](#development-and-checks)
- [Image publishing](#publish-an-image), [health probes](#health-probes), and [logging](#logging)

## Configuration
Configuration uses environment variables or CLI flags; there is no configuration-file loader.
At least one backend URI is needed to provision instances. Set:
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

## Development and checks

Use Go 1.27. For a native run, set `POSTGRES_URI` and/or `DRAGONFLY_URI` to
backends reachable from the host, then run `go run ./cmd/database-broker`. The
PostgreSQL account needs database/role creation privileges and an existing
metadata database. Dragonfly needs administrative ACL permissions; ordinary
Redis does not support its namespace provisioning. Use an explicit `6379` port.

Returned URI hosts come from these backend settings. Compose uses the Docker
service names `postgres` and `dragonfly`; clients must be on that network to
resolve them. Use reachable hostnames when configuring another deployment.

Unit tests without Docker:

```sh
go build ./...
go vet ./...
go test ./internal/config ./internal/broker ./internal/router ./internal/util
```

The full `go test ./...` suite starts disposable PostgreSQL and Dragonfly
containers through testcontainers by default. Alternatively, set
`TEST_DATABASE_URL` and `TEST_DRAGONFLY_URL` to disposable loopback native backends.
PostgreSQL tests create/drop databases and roles; Dragonfly tests require an
empty broker registry and clean up their instance metadata, reservations, and ACL
users. They do not skip adapter tests when Docker is unavailable. See [AGENTS.md](../AGENTS.md)
for provisioning internals, backend constraints, and verification guidance.

## Multiple replicas

Replicas must share the same backend URIs, metadata database, and credentials.
PostgreSQL migrations take a transaction advisory lock before version discovery.
First-time PostgreSQL provisioning takes a per-instance session advisory lock,
rechecks metadata, and uses the same pinned connection for database creation and
the credentials/ownership transaction. Concurrent callers receive the saved
credentials; cancellation releases or discards the locked connection.

Dragonfly reserves one set of random credentials atomically under
`instance-pending:<name>` before creating its ACL user, then publishes the reserved
record under `instance:<name>`. Concurrent requests and retries apply the same
credentials, including after a crash between ACL creation and publication. The
reservation is deliberately retained without expiry, is excluded from listings,
and must be backed up alongside published metadata. Existing published instances
retain their credentials and are read without ACL changes.

During an upgrade from older broker images, quiesce first-time provisioning until
all replicas use these coordination protocols. Schema changes still need to be
compatible with the code running during a rolling upgrade. Backend availability
and durability depend on PostgreSQL/Dragonfly themselves; this does not add
backend replication or persistence.

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
