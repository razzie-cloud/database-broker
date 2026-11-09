package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"
)

// A session lock spans CREATE DATABASE (which cannot run in a transaction)
// and the metadata transaction. All work must use this same pinned connection:
// losing the lock's connection then also prevents further provisioning writes.
func lockSession(ctx context.Context, db *sql.DB, key string) (*sql.Conn, func(), error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, nil, err
	}
	discard := func() {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		_ = conn.Close()
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1,0))`, key); err != nil {
		// A canceled query may have acquired the lock just before cancellation.
		discard()
		return nil, nil, err
	}
	return conn, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, key); err != nil {
			discard()
			return
		}
		_ = conn.Close()
	}, nil
}
