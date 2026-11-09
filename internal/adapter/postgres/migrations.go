package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/go-rel/migration"
	"github.com/go-rel/rel"
	"github.com/razzie-cloud/database-broker/internal/logging"
)

const migrationLockID int64 = 0x62726f6b65726d69

func MigrateCreateInstances(schema *rel.Schema) {
	schema.CreateTable("instances", func(t *rel.Table) {
		t.Text("instance_name")
		t.PrimaryKey("instance_name")
		t.Text("db_name")
		t.Text("db_user")
		t.Text("db_password")
		t.DateTime("created_at", rel.Default("NOW()"))
	})
}

func RollbackCreateInstances(schema *rel.Schema) {
	schema.DropTable("instances")
}

func migrate(repo rel.Repository) (err error) {
	// go-rel/migration reports failures by panic. Return them to the startup
	// logger so raw database errors cannot escape through a panic traceback.
	defer func() {
		if value := recover(); value != nil {
			var ok bool
			err, ok = value.(error)
			if !ok {
				err = errors.New("metadata migration panicked")
			}
		}
	}()
	m := migration.New(repo)
	m.Instrumentation(func(ctx context.Context, op, message string, args ...any) func(error) {
		start := time.Now()
		versionText, _, _ := strings.Cut(message, " ")
		version, _ := strconv.Atoi(versionText)
		return func(err error) {
			if err == nil {
				logging.For(ctx).Info("Database migration applied", "event", "migrations.applied", "component", "postgres", "version", version, "duration_ms", time.Since(start).Milliseconds())
			}
		}
	})
	m.Register(1, MigrateCreateInstances, RollbackCreateInstances)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// The outer transaction pins the advisory lock to the same session used by
	// migration discovery, version-table creation, and nested migration savepoints.
	return repo.Transaction(ctx, func(ctx context.Context) error {
		if _, _, err := repo.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrationLockID); err != nil {
			return err
		}
		m.Migrate(ctx)
		return nil
	})
}
