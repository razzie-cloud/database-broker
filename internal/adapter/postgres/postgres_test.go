package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startPostgresContainer(t *testing.T) (testcontainers.Container, string, int) {
	req := testcontainers.ContainerRequest{
		Image:        "postgres:latest",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_PASSWORD": "adminpass",
			"POSTGRES_USER":     "admin",
			"POSTGRES_DB":       "postgres",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp"),
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
	return container, uri, port
}

func TestPostgresAdapterIntegration(t *testing.T) {
	ctx := context.Background()
	container, uri, _ := startPostgresContainer(t)
	defer container.Terminate(ctx)

	adapter, err := New(uri)
	require.NoError(t, err)

	foo, err := adapter.GetOrCreateInstance(ctx, "foo")
	require.NoError(t, err)
	bar, err := adapter.GetOrCreateInstance(ctx, "bar")
	require.NoError(t, err)

	instances, err := adapter.GetInstances(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"foo", "bar"}, instances)

	fooDB, err := sql.Open("postgres", strings.Replace(foo.GetURI(), "sslmode=prefer", "sslmode=disable", 1))
	require.NoError(t, err)
	defer fooDB.Close()
	barDB, err := sql.Open("postgres", strings.Replace(bar.GetURI(), "sslmode=prefer", "sslmode=disable", 1))
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
