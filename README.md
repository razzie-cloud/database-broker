# Database Broker

An internal HTTP service that creates per-application PostgreSQL databases,
Dragonfly namespaces, and Valkey numbered databases on demand. An application
requests a named instance and receives credentials and a connection URI;
repeated requests return the saved credentials. Applications then connect
directly to the database server.

The `postgres` adapter provisions PostgreSQL. The `dragonfly` and `redis` names
both use Dragonfly's Redis-compatible namespaces; ordinary Redis cannot replace
Dragonfly as the provisioning backend. The `valkey` adapter requires Valkey 9.1.0
or newer and restricts each application's user to one database. Pub/sub is
disabled for every provisioned Valkey user; the administrator retains access.

## Try it locally

You need Git, Go 1.27, Python 3 and Docker with the Compose plugin. The included stack starts the
broker, PostgreSQL, Dragonfly, and Valkey with local development credentials.
Ports 5432, 6379, 6380, and 8080 must be free.

```sh
git clone https://github.com/razzie-cloud/database-broker.git
cd database-broker
python3 scripts/prepare-dev-auth.py
docker compose up --build -d
# Save the returned database URI privately; never log it.
umask 077
curl --fail --get --data-urlencode client=example \
  --data-urlencode token@.local/broker-auth/example.token \
  -o .local/broker-auth/example.uri \
  http://localhost:8080/v1/instances/postgres/example/uri
```

The request creates `example` if needed and returns its PostgreSQL connection URI.
Use `redis` in place of `postgres` to provision a Dragonfly namespace, or `valkey`
to provision a Valkey database. Returned URIs use the Compose backend hostnames,
so consumers must share its Docker
network; see [configuration](docs/reference.md#development-and-checks) for host-based clients.

## Use it with your backends

Set `POSTGRES_URI`, `DRAGONFLY_URI`, and/or `VALKEY_URI` to enable those backends,
then run the container or, with Go 1.27, `go run ./cmd/database-broker`. PostgreSQL needs an
existing metadata database and an account with database/role creation privileges;
Dragonfly needs administrative ACL permissions. Valkey needs a dedicated standalone
server, an administrative account, and at least two configured databases (one is
reserved for broker metadata). See [Valkey configuration](docs/reference.md#valkey).
`SERVICE_PORT` defaults to `8080`.

| GET endpoint | Result |
| --- | --- |
| `/v1/instances/{adapter}` | List provisioned instance names |
| `/v1/instances/{adapter}/{name}` | Get or create an instance; return JSON credentials |
| `/v1/instances/{adapter}/{name}/uri` | Get or create an instance; return its connection URI |

Names accept letters, digits, and underscores and are normalized to lowercase.
The API defaults to scoped token authentication: exact policy grants authorize
get-or-create requests, and listing is denied in token mode. Keep the broker
private and use verified TLS in production. Explicit AUTH_MODE=none preserves
legacy tokenless operation; local Compose can select it using
`-f docker-compose.yml -f docker-compose.none.yml` without preparing auth files.

See the [reference guide](docs/reference.md) for all settings, checks, operations,
and image publishing, or [AGENTS.md](AGENTS.md) for provisioning internals and
contributor guidance. Licensed under the [MIT License](LICENSE).

See [scoped authentication and TLS](docs/authentication.md) before starting the broker.
Authentication defaults to token mode; explicitly select AUTH_MODE=none only for legacy operation.
