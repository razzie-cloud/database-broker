package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/razzie-cloud/database-broker/internal/adapter"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startPostgresContainer(t *testing.T) (func(), string, int) {
	t.Helper()
	if uri, port, stop := localPostgres(t); uri != "" {
		return stop, uri, port
	}
	req := testcontainers.ContainerRequest{
		Image:        "postgres:latest",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_PASSWORD": "adminpass",
			"POSTGRES_USER":     "admin",
			"POSTGRES_DB":       "postgres",
		},
		// Initialization starts a temporary server before restarting PostgreSQL.
		// Wait for both startups before checking the published port.
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		),
	}
	container, err := testcontainers.GenericContainer(context.Background(), testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	host, err := container.Host(context.Background())
	require.NoError(t, err)
	portStr, err := container.MappedPort(context.Background(), "5432")
	require.NoError(t, err)
	port := portStr.Int()
	uri := fmt.Sprintf("postgres://admin:adminpass@%s:%d/postgres?sslmode=disable", host, port)
	return func() { _ = container.Terminate(context.Background()) }, uri, port
}

func TestPostgresAdapterIntegration(t *testing.T) {
	ctx := context.Background()
	stop, uri, _ := startPostgresContainer(t)
	defer stop()

	adapter, err := New(uri)
	require.NoError(t, err)
	defer adapter.Close()
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	require.NoError(t, adapter.Ping(ctx))
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.Error(t, adapter.Ping(cancelled))

	foo, err := adapter.GetOrCreateInstance(ctx, "foo")
	require.NoError(t, err)
	bar, err := adapter.GetOrCreateInstance(ctx, "bar")
	require.NoError(t, err)
	require.NotContains(t, output.String(), foo.(*Instance).Password)
	require.NotContains(t, output.String(), bar.(*Instance).Password)
	require.NotContains(t, output.String(), "CREATE ROLE")
	require.NotContains(t, output.String(), "SELECT")
	require.Contains(t, output.String(), `"event":"instance.provisioned"`)
	require.Contains(t, output.String(), `"adapter":"postgres"`)
	output.Reset()
	_, err = adapter.GetOrCreateInstance(ctx, "foo")
	require.NoError(t, err)
	// The old error-only instrumenter leaked this statement's password.
	err = createUser(ctx, adapter.(*postgresAdapter).db, foo.(*Instance).Username, "logging-sentinel-secret")
	require.Error(t, err)
	require.Empty(t, output.String(), "existing lookups and low-level SQL failures must not log")

	instances, err := adapter.GetInstances(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"foo", "bar"}, instances)

	fooDB, err := sql.Open("pgx", foo.GetURI())
	require.NoError(t, err)
	defer fooDB.Close()
	barDB, err := sql.Open("pgx", bar.GetURI())
	require.NoError(t, err)
	defer barDB.Close()

	// Create table and insert data in foo
	_, err = fooDB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS test (id SERIAL PRIMARY KEY, value TEXT)`)
	require.NoError(t, err)
	_, err = fooDB.ExecContext(ctx, `INSERT INTO test (value) VALUES ('foo-value')`)
	require.NoError(t, err)

	// Check data in foo
	var fooVal string
	err = fooDB.QueryRowContext(ctx, `SELECT value FROM test WHERE value = 'foo-value'`).Scan(&fooVal)
	require.NoError(t, err)
	require.Equal(t, "foo-value", fooVal)

	// Check data not in bar
	var barVal string
	err = barDB.QueryRowContext(ctx, `SELECT value FROM test WHERE value = 'foo-value'`).Scan(&barVal)
	require.Error(t, err)

	// Create table and insert data in bar
	_, err = barDB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS test (id SERIAL PRIMARY KEY, value TEXT)`)
	require.NoError(t, err)
	_, err = barDB.ExecContext(ctx, `INSERT INTO test (value) VALUES ('bar-value')`)
	require.NoError(t, err)

	// Check data in bar
	err = barDB.QueryRowContext(ctx, `SELECT value FROM test WHERE value = 'bar-value'`).Scan(&barVal)
	require.NoError(t, err)
	require.Equal(t, "bar-value", barVal)

	// Check data not in foo
	err = fooDB.QueryRowContext(ctx, `SELECT value FROM test WHERE value = 'bar-value'`).Scan(&fooVal)
	require.Error(t, err)
}

func TestPostgresInstanceNameLimit(t *testing.T) {
	stop, uri, _ := startPostgresContainer(t)
	defer stop()
	peer, err := New(uri)
	require.NoError(t, err)
	defer peer.Close()
	pg := peer.(*postgresAdapter)
	ctx := t.Context()

	t.Run("50-byte names retain complete identifiers and authenticate", func(t *testing.T) {
		for _, name := range []string{strings.Repeat("a", 49) + "a", strings.Repeat("a", 49) + "b"} {
			created, err := pg.GetOrCreateInstance(ctx, name)
			require.NoError(t, err)
			instance := created.(*Instance)
			require.Len(t, instance.Username, 63)
			require.Len(t, instance.Database, 53)
			saved, err := pg.GetInstance(ctx, name)
			require.NoError(t, err)
			require.Equal(t, instance.GetURI(), saved.GetURI())
			app, err := sql.Open("pgx", instance.GetURI())
			require.NoError(t, err)
			require.NoError(t, app.PingContext(ctx))
			require.NoError(t, app.Close())
		}
	})

	t.Run("overlong new names create no resources", func(t *testing.T) {
		snapshot := func() [3]string {
			var state [3]string
			require.NoError(t, pg.db.QueryRowContext(ctx, `SELECT
 (SELECT json_agg(datname ORDER BY datname)::text FROM pg_database),
 (SELECT json_agg(rolname ORDER BY rolname)::text FROM pg_roles),
 (SELECT coalesce(json_agg(instances ORDER BY instance_name)::text, '[]') FROM instances)`).Scan(&state[0], &state[1], &state[2]))
			return state
		}
		before := snapshot()
		for _, name := range []string{
			strings.Repeat("b", 51),
			strings.Repeat("a", 58) + "a",
			strings.Repeat("a", 58) + "b",
			strings.Repeat("c", 128),
		} {
			instance, err := pg.GetOrCreateInstance(ctx, name)
			require.Nil(t, instance)
			require.ErrorIs(t, err, adapter.ErrInstanceNameTooLong)
			require.Equal(t, before, snapshot(), "rejection must leave databases, roles and metadata unchanged")
		}
	})

	t.Run("existing long lookup keys retain stored credentials", func(t *testing.T) {
		created, err := pg.GetOrCreateInstance(ctx, "legacy")
		require.NoError(t, err)
		saved, err := pg.GetInstance(ctx, "legacy")
		require.NoError(t, err)
		name := strings.Repeat("d", 59)
		_, err = pg.db.ExecContext(ctx, `UPDATE instances SET instance_name=$1 WHERE instance_name='legacy'`, name)
		require.NoError(t, err)
		expected := *saved.(*Instance)
		expected.InstanceName = name
		for _, lookup := range []func(context.Context, string) (adapter.Instance, error){pg.GetInstance, pg.GetOrCreateInstance} {
			found, err := lookup(ctx, name)
			require.NoError(t, err)
			require.Equal(t, &expected, found)
			require.Equal(t, created.GetURI(), found.GetURI())
		}
	})
}
