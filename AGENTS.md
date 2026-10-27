# Database Broker development

## Scope and repository workflow

This directory is the `github.com/razzie-cloud/database-broker` Go 1.27 module
and a Git submodule of razcloud. Run Go commands here; the parent has no Go
module. Keep broker code, tests, Docker files, and documentation in this
repository. Follow the parent `AGENTS.md` for submodule workflow and preserve
unrelated changes in sibling components.

Inspect status and existing diffs before editing. If committing, use a branch
in this repository and commit component changes before updating the parent's
submodule pointer. Do not push, publish images, or deploy without authorization.
The infra stack uses a published broker image; local source and parent pointer
changes alone do not update that image.

## Source map

- `cmd/database-broker/main.go` loads configuration, constructs enabled
  adapters, registers them with the broker, and starts the HTTP server.
- `internal/config/` parses CLI flags and environment variables and injects
  passwords read from files into backend URIs.
- `internal/broker/` owns adapter registration, name normalization, instance
  name validation, and errors with HTTP status codes.
- `internal/router/` owns chi routes, request context propagation, response
  encoding, and error-to-HTTP translation. Tests use mocked broker interfaces.
- `internal/adapter/interface.go` defines backend operations and the instance
  methods `GetJSON` and `GetURI`. Backend implementations live in `postgres/`
  and `dragonfly/`, with response models and URI construction in `instance.go`.
- `internal/adapter/postgres/migrations.go` contains versioned go-rel metadata
  migrations, applied during PostgreSQL adapter construction.
- `internal/util/` provides URI host/port parsing and cryptographic random
  credentials. `Dockerfile` builds the binary; `docker-compose.yml` supplies
  local PostgreSQL, Dragonfly, and broker services.

## Configuration and local running

Configuration uses `internal/config.Load`; there is no configuration-file
loader. Keep CLI flags, environment variables, README examples, and container
configuration consistent when changing settings.

| Environment variable | CLI flag | Default |
| --- | --- | --- |
| `SERVICE_PORT` | `--port` | `8080` |
| `POSTGRES_URI` | `--postgres-uri` | unset |
| `POSTGRES_PASSWORD_FILE` | `--postgres-password-file` | unset |
| `DRAGONFLY_URI` | `--dragonfly-uri` | unset |
| `DRAGONFLY_PASSWORD_FILE` | `--dragonfly-password-file` | unset |

Each backend is enabled only when its URI is supplied. Dragonfly registers
both `dragonfly` and `redis` against the same adapter. With neither URI, the
HTTP server still starts but no adapters are registered.

Password files replace the password in the corresponding URI only when that
URI is set. Their bytes are read verbatim, including trailing newlines. Use
local secret files and keep credentials, generated connection URIs, and local
environment files out of Git and diagnostic output.

Run with `go run ./cmd/database-broker` after setting local backend URIs, or
use `docker compose up --build` against a disposable local Docker daemon.
Compose publishes ports 5432, 6379, and 8080; check for conflicts with other
local stacks. Backend hosts in returned URIs come from the configured backend
URI, so they must be reachable by the intended consumer.

## HTTP contract

The broker currently has no authentication or authorization middleware. Keep
it on the internal network and use local backends for development requests.
Both instance lookup endpoints can create databases/users and return secrets.
Do not probe a deployed instance as a routine health or validation check.

- `GET /v1/instances/{adapter_name}` lists provisioned instance names as JSON.
- `GET /v1/instances/{adapter_name}/{instance_name}` gets or creates an
  instance and returns JSON including credentials and its connection URI.
- `GET /v1/instances/{adapter_name}/{instance_name}/uri` gets or creates an
  instance and returns the URI with content type `text/uri-list`.
- Successful responses use HTTP 200. Errors use JSON `{"error":"..."}`;
  invalid instance names return 422, missing adapters return 404, and other
  errors default to 500 through `StatusCode()` handling.

Adapter names are case insensitive. The broker lowercases instance names and
accepts only nonempty ASCII letters, digits, and underscores (`^[A-Za-z0-9_]+$`).
Keep validation in the broker so every caller follows the same rules. Preserve
request contexts through adapter operations. Existing instances must retain
their credentials on repeated lookups. Coordinate API changes with consumers
in access-gateway and infra, especially callers of the `/uri` endpoint.

## Adapter and persistence rules

- Implement `adapter.Interface` and `adapter.Instance` for new backends and
  register them in `main.go`. Keep HTTP encoding out of backend operations.
- PostgreSQL stores instance metadata, including passwords, in the configured
  broker database's `instances` table. Add new versioned registrations in
  `migrations.go`; do not rewrite migrations that may already have run. These
  use go-rel/migration, separate from access-gateway's Goose SQL migrations.
- Keep passwords cryptographically random via `crypto/rand` helpers. Build
  URIs with URL userinfo escaping and `net.JoinHostPort` for IPv6 support.
- Preserve database/namespace isolation and restricted role/ACL permissions.
  The broker's mutex protects the adapter registry, not provisioning operations.

Current behavior to recheck before extending URI handling: the Dragonfly
adapter passes 5432 as the fallback port to `GetURIHostPort`, so use an explicit
6379 port in local Dragonfly URIs. PostgreSQL response URIs use `sslmode=prefer`;
they do not copy all options from the configured administrative URI.

### PostgreSQL provisioning sequence

`internal/adapter/postgres/postgres.go` uses the administrative connection from
`POSTGRES_URI` for every provisioning statement. Its account must be able to
create databases and roles, transfer database ownership, change access grants,
and read/write broker metadata. The broker does not create this administrative
account or its metadata database; they must already exist. Adapter startup
applies the metadata migrations in that database.

For a request for `Example`, the broker normalizes the name to `example` before
calling `GetOrCreateInstance`:

1. Look up `instances.instance_name = 'example'` in the metadata database.
   If found, return the stored database, username, password, and creation time,
   with host/port from the current adapter configuration. Do not recreate or
   check the existence of the actual database/role. Only `rel.ErrNotFound`
   enters creation; other lookup errors stop the request.
2. Choose database `db_example`, role `user_example_<suffix>`, and a password.
   The suffix is seven lowercase unpadded base32 characters from four random
   bytes. Passwords are 26 lowercase unpadded base32 characters from 16 random
   bytes. Both use `crypto/rand` through `internal/util/rand.go`.
3. Execute `CREATE DATABASE` outside the metadata transaction. A PostgreSQL
   `DuplicateDatabase` error is accepted, allowing reuse of an existing
   `db_example`; other errors stop provisioning.
4. In one go-rel transaction, create the login role, insert the metadata row,
   transfer database ownership, revoke PUBLIC CONNECT, and grant the new role
   CONNECT. The SQL below shows the order; angle-bracket values are illustrative
   placeholders, and actual SQL quotes identifiers/values with `postgres.Quote`:

   ```sql
   CREATE DATABASE "db_example"; -- outside the transaction
   -- Inside the transaction:
   CREATE ROLE "user_example_<suffix>" LOGIN PASSWORD '<generated-password>'
     NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
   -- go-rel inserts the instances row here.
   ALTER DATABASE "db_example" OWNER TO "user_example_<suffix>";
   REVOKE CONNECT ON DATABASE "db_example" FROM PUBLIC;
   GRANT CONNECT ON DATABASE "db_example" TO "user_example_<suffix>";
   ```

5. Return the instance only after the transaction succeeds. The metadata row
   contains `instance_name` (primary key), `db_name`, `db_user`, `db_password`
   (stored as plaintext), and `created_at` (set to UTC by the adapter). Host
   and port are not persisted. The generated URI is
   `postgres://<user>:<password>@<host>:<port>/db_example?sslmode=prefer` with
   escaped userinfo. No application tables, schemas, or application migrations
   are created by this provisioning path.

Failures inside the transaction roll back its role/metadata/permission changes,
but leave the separately created database behind. There is no compensating
DROP DATABASE. Retries can reuse that database and generate a new role/password.
Concurrent first requests can both miss metadata; a duplicate metadata insert
can fail one transaction rather than automatically returning the winning row.
Do not describe get-or-create as fully atomic or assume an existing database
is empty before ownership is transferred.

### Dragonfly provisioning sequence

`internal/adapter/dragonfly/dragonfly.go` uses the configured administrative
Redis client. Its credentials need access to metadata keys and `ACL SETUSER`
with Dragonfly's `NAMESPACE:` extension. Both adapter names, `dragonfly` and
`redis`, use this implementation; ordinary Redis is not a drop-in backend.

For the same normalized instance name `example`:

1. `GET instance:example` in the administrative client's configured database.
   If metadata exists, decode it and return the stored credentials/namespace;
   fill instance name and host/port from the request/current configuration.
   Only a missing key (`redis.Nil`) enters creation. No ACL existence or
   credential validity check is made for an existing metadata record.
2. Choose username `user_example`, namespace `ns_example`, a fresh password
   using the same 16-random-byte helper as PostgreSQL, and a UTC creation time.
   Marshal the metadata JSON before issuing backend mutations.
3. Send the following command as separate Redis protocol arguments:

   ```text
   ACL SETUSER user_example NAMESPACE:ns_example ON ><generated-password> +@all -@admin -ACL -CONFIG -MODULE -CLUSTER ~*
   ```

   This enables the user, supplies a password, associates its namespace, allows
   all command categories before subtracting the listed administrative
   permissions, and allows all key patterns within that namespace. The broker
   issues no separate CREATE NAMESPACE command or numbered-database allocation;
   namespace assignment is part of the Dragonfly ACL operation.
4. Save JSON using `SETNX instance:example <json>` with no expiry. It includes
   `namespace`, `username`, plaintext `password`, `created_at`, and an empty
   `uri` field; instance name, host, and port are omitted by JSON tags. If the
   key already exists, reload and return that record instead of overwriting it.
5. Build the response URI dynamically as
   `redis://<user>:<password>@<host>:<port>/0`. Clients authenticate as the new
   user; the ACL namespace supplies isolation, and the URI always selects
   database 0. The metadata client's database still comes from `DRAGONFLY_URI`.

ACL creation and metadata storage have no shared transaction or cleanup step.
A failed metadata write can leave an ACL user behind. Concurrent requests can
both mutate the same deterministic ACL username with different generated
passwords before one wins `SETNX`; the command sends no `resetpass` option.
Do not assume `SETNX` serializes user creation or keeps ACL credentials and
metadata consistent under every failure/race.

Both adapters list their metadata registry rather than discovering all backend
databases/users/namespaces. PostgreSQL sorts stored instance names; Dragonfly
uses `SCAN instance:*` with count 100. There is no deletion, credential rotation,
or reconciliation API to repair resources changed outside the broker.

## Verification

For Go changes, format changed files, build, vet, and run relevant tests here:

```sh
gofmt -w <changed-go-files>
go build ./...
go vet ./...
go test ./internal/config ./internal/broker ./internal/router ./internal/util
```

The command above tests packages that do not require Docker. For adapter
changes, also run the full suite against a disposable local Docker daemon:

```sh
go test ./...
```

Adapter integration tests use testcontainers to start `postgres:latest` and
`docker.dragonflydb.io/dragonflydb/dragonfly:latest`, potentially pulling images.
They exercise provisioning and data isolation between instances and do not
skip when Docker is unavailable. Verify the effective testcontainers Docker
endpoint is local/disposable; never target the production daemon or change
Docker contexts as part of validation.

For Compose edits, run `docker compose -f docker-compose.yml config --quiet`.
Do not invoke infra deployment scripts as validation. Report exactly which
checks ran, whether adapter integration tests ran, and any toolchain, Docker,
or network limitations. Documentation-only changes need a diff/whitespace
review rather than Go builds or database tests.
