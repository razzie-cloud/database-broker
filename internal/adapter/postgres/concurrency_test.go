package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/razzie-cloud/database-broker/internal/adapter"
	"github.com/stretchr/testify/require"
)

func TestConcurrentStartupAndProvisioning(t *testing.T) {
	stop, uri, _ := startPostgresContainer(t)
	defer stop()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	const replicas = 8
	type startup struct {
		peer adapter.Interface
		err  error
	}
	start := make(chan struct{})
	started := make(chan startup, replicas)
	for range replicas {
		go func() {
			<-start
			peer, err := New(uri)
			started <- startup{peer, err}
		}()
	}
	close(start)
	var peers []adapter.Interface
	for range replicas {
		result := <-started
		require.NoError(t, result.err)
		defer result.peer.Close()
		peers = append(peers, result.peer)
	}
	type provisioned struct {
		instance adapter.Instance
		err      error
	}
	start = make(chan struct{})
	results := make(chan provisioned, replicas)
	for _, peer := range peers {
		go func() {
			<-start
			instance, err := peer.GetOrCreateInstance(ctx, "concurrent")
			results <- provisioned{instance, err}
		}()
	}
	close(start)
	var canonical *Instance
	for range replicas {
		result := <-results
		require.NoError(t, result.err)
		if canonical == nil {
			canonical = result.instance.(*Instance)
		}
		require.Equal(t, canonical.GetURI(), result.instance.GetURI())
	}
	app, err := sql.Open("pgx", canonical.GetURI())
	require.NoError(t, err)
	defer app.Close()
	require.NoError(t, app.PingContext(ctx), "the single published credential must authenticate")
	pg := peers[0].(*postgresAdapter)
	var count int
	require.NoError(t, pg.db.QueryRowContext(ctx, `SELECT count(*) FROM instances WHERE instance_name='concurrent'`).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, pg.db.QueryRowContext(ctx, `SELECT count(*) FROM pg_roles WHERE starts_with(rolname,'user_concurrent_')`).Scan(&count))
	require.Equal(t, 1, count, "competing requests must not leave extra roles")
	require.NoError(t, pg.db.QueryRowContext(ctx, `SELECT count(*) FROM rel_schema_versions WHERE version=1`).Scan(&count))
	require.Equal(t, 1, count, "migration must run only once")
}

func TestProvisioningLockCancellation(t *testing.T) {
	stop, uri, _ := startPostgresContainer(t)
	defer stop()
	peer, err := New(uri)
	require.NoError(t, err)
	defer peer.Close()
	pg := peer.(*postgresAdapter)
	pg.db.SetMaxOpenConns(1)
	blocker, err := sql.Open("pgx", uri)
	require.NoError(t, err)
	defer blocker.Close()
	_, unlock, err := lockSession(t.Context(), blocker, "database-broker:postgres:cancelled")
	require.NoError(t, err)
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err = pg.GetOrCreateInstance(ctx, "cancelled")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	unlock()
	unlock = nil
	ctx, cancelRetry := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelRetry()
	_, err = pg.GetOrCreateInstance(ctx, "cancelled")
	require.NoError(t, err, "canceled lock acquisition must not poison the pool or leak a lock")
}

func TestMigrationFailureRollsBackAndReleasesLock(t *testing.T) {
	stop, uri, _ := startPostgresContainer(t)
	defer stop()
	db, err := sql.Open("pgx", uri)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.ExecContext(t.Context(), `CREATE TABLE instances(dummy int)`)
	require.NoError(t, err)
	_, err = New(uri)
	require.Error(t, err)
	var versionsExist bool
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT to_regclass('rel_schema_versions') IS NOT NULL`).Scan(&versionsExist))
	require.False(t, versionsExist, "failed migration must roll back version-table creation too")
	_, err = db.ExecContext(t.Context(), `DROP TABLE instances`)
	require.NoError(t, err)
	peer, err := New(uri)
	require.NoError(t, err)
	defer peer.Close()
}

func TestProvisioningFailureRollsBackAndReleasesLock(t *testing.T) {
	stop, uri, _ := startPostgresContainer(t)
	defer stop()
	peer, err := New(uri)
	require.NoError(t, err)
	defer peer.Close()
	pg := peer.(*postgresAdapter)
	_, err = pg.db.ExecContext(t.Context(), `ALTER TABLE instances ADD CONSTRAINT reject_instance CHECK (instance_name <> 'recover')`)
	require.NoError(t, err)
	_, err = peer.GetOrCreateInstance(t.Context(), "recover")
	require.Error(t, err)
	var count int
	require.NoError(t, pg.db.QueryRowContext(t.Context(), `SELECT count(*) FROM pg_roles WHERE starts_with(rolname,'user_recover_')`).Scan(&count))
	require.Zero(t, count, "failed metadata publication must roll back its role")
	_, err = pg.db.ExecContext(t.Context(), `ALTER TABLE instances DROP CONSTRAINT reject_instance`)
	require.NoError(t, err)
	other, err := New(uri)
	require.NoError(t, err)
	defer other.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	instance, err := other.GetOrCreateInstance(ctx, "recover")
	require.NoError(t, err, "another replica must acquire the released lock and reuse the database")
	app, err := sql.Open("pgx", instance.GetURI())
	require.NoError(t, err)
	defer app.Close()
	require.NoError(t, app.PingContext(ctx))
}
