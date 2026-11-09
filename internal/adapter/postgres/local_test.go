package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	pg "github.com/go-rel/postgres"
	"github.com/stretchr/testify/require"
)

// An explicitly supplied disposable loopback backend is useful without Docker.
// Each test owns a separate metadata database and cleans up its provisioned roles/DBs.
func localPostgres(t *testing.T) (string, int, func()) {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		return "", 0, nil
	}
	uri, err := url.Parse(raw)
	require.NoError(t, err)
	ip := net.ParseIP(uri.Hostname())
	require.True(t, ip != nil && ip.IsLoopback(), "TEST_DATABASE_URL must be a disposable loopback backend")
	port, err := strconv.Atoi(uri.Port())
	require.NoError(t, err)
	admin, err := sql.Open("pgx", raw)
	require.NoError(t, err)
	name := "broker_tests_" + strings.ToLower(rand.Text())
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE "+name)
	require.NoError(t, err)
	uri.Path = "/" + name
	db, err := sql.Open("pgx", uri.String())
	require.NoError(t, err)
	return uri.String(), port, func() {
		ctx := context.Background()
		rows, err := db.QueryContext(ctx, "SELECT db_name,db_user FROM instances")
		if err == nil {
			var instances [][2]string
			for rows.Next() {
				var item [2]string
				if rows.Scan(&item[0], &item[1]) == nil {
					instances = append(instances, item)
				}
			}
			_ = rows.Close()
			for _, item := range instances {
				_, _ = admin.ExecContext(ctx, "DROP DATABASE "+(pg.Quote{}).ID(item[0])+" WITH (FORCE)")
				_, _ = admin.ExecContext(ctx, "DROP ROLE "+(pg.Quote{}).ID(item[1]))
			}
		}
		_ = db.Close()
		_, _ = admin.ExecContext(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
		_ = admin.Close()
	}
}
