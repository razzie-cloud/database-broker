package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/razzie-cloud/database-broker/internal/adapter"
	"github.com/razzie-cloud/database-broker/internal/logging"
	"github.com/razzie-cloud/database-broker/internal/util"

	"github.com/go-rel/postgres"
	"github.com/go-rel/rel"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type postgresAdapter struct {
	db      *sql.DB
	adapter rel.Adapter
	repo    rel.Repository
	host    string
	port    int
}

func New(postgresUri string) (adapter.Interface, error) {
	host, port, err := util.GetURIHostPort(postgresUri, 5432)
	if err != nil {
		return nil, fmt.Errorf("parse postgres uri: %w", err)
	}
	db, err := sql.Open("pgx", postgresUri)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	adapter := postgres.New(db)
	repo := rel.New(adapter)
	repo.Instrumentation(nil)
	if err := migrate(repo); err != nil {
		_ = db.Close()
		return nil, logging.Wrap("migrate_metadata", err)
	}
	return &postgresAdapter{
		db:      db,
		adapter: adapter,
		repo:    repo,
		host:    host,
		port:    port,
	}, nil
}

func (pg *postgresAdapter) Ping(ctx context.Context) error {
	return pg.db.PingContext(ctx)
}

func (pg *postgresAdapter) Close() error {
	return pg.adapter.Close()
}

func (pg *postgresAdapter) GetInstances(ctx context.Context) ([]string, error) {
	var instances []Instance
	err := pg.repo.FindAll(ctx, &instances, rel.Select("instance_name"), rel.SortAsc("instance_name"))
	if err != nil {
		return nil, err
	}
	names := make([]string, len(instances))
	for i, inst := range instances {
		names[i] = inst.InstanceName
	}
	return names, nil
}

func (pg *postgresAdapter) GetInstance(ctx context.Context, instanceName string) (adapter.Instance, error) {
	instance := Instance{
		Host: pg.host,
		Port: pg.port,
	}
	err := pg.repo.Find(ctx, &instance, rel.Eq("instance_name", instanceName))
	if err != nil {
		return nil, err
	}
	return &instance, nil
}

func (pg *postgresAdapter) GetOrCreateInstance(ctx context.Context, instanceName string) (result adapter.Instance, err error) {
	logging.Fields(ctx, "adapter", "postgres", "instance_name", instanceName[:min(len(instanceName), 128)])
	start := time.Now()
	stage := "lookup_instance"
	defer func() {
		if err != nil {
			err = logging.Wrap(stage, err)
		}
	}()
	instance, err := pg.GetInstance(ctx, instanceName)
	if err == nil {
		return instance, nil
	}
	if err != rel.ErrNotFound {
		return nil, err
	}
	dbName := "db_" + instanceName
	dbUser := "user_" + instanceName + "_" + strings.ToLower(util.RandToken(4))
	dbPass := util.RandPassword()
	stage = "create_database"
	if err := createDatabase(ctx, pg.repo, dbName); err != nil {
		return nil, err
	}
	stage = "begin_transaction"
	err = pg.repo.Transaction(ctx, func(txCtx context.Context) error {
		stage = "create_role"
		if err := createUser(txCtx, pg.repo, dbUser, dbPass); err != nil {
			return fmt.Errorf("create role: %w", err)
		}
		instance = &Instance{
			InstanceName: instanceName,
			Host:         pg.host,
			Port:         pg.port,
			Database:     dbName,
			Username:     dbUser,
			Password:     dbPass,
			CreatedAt:    time.Now().UTC(),
		}
		stage = "save_metadata"
		if err := pg.repo.Insert(txCtx, instance); err != nil {
			return fmt.Errorf("insert instance: %w", err)
		}
		stage = "transfer_ownership"
		if err := transferDatabaseOwnership(txCtx, pg.repo, dbName, dbUser); err != nil {
			return fmt.Errorf("transfer db ownership: %w", err)
		}
		stage = "revoke_public_access"
		if err := revokePublicDatabaseAccess(txCtx, pg.repo, dbName); err != nil {
			return fmt.Errorf("revoke db public access: %w", err)
		}
		stage = "grant_access"
		if err := grantDatabaseAccess(txCtx, pg.repo, dbName, dbUser); err != nil {
			return fmt.Errorf("grant db connect access: %w", err)
		}
		stage = "commit_transaction"
		return nil
	})
	if err != nil {
		return nil, err
	}
	logging.For(ctx, "adapter", "postgres", "instance_name", instanceName[:min(len(instanceName), 128)]).Info("Instance provisioned", "event", "instance.provisioned", "duration_ms", time.Since(start).Milliseconds())
	return instance, nil
}

func createDatabase(ctx context.Context, repo rel.Repository, name string) error {
	sql := fmt.Sprintf("CREATE DATABASE %s", postgres.Quote{}.ID(name))
	_, _, err := repo.Exec(ctx, sql)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.DuplicateDatabase {
		return nil
	}
	return err
}

func createUser(ctx context.Context, repo rel.Repository, name, password string) error {
	sql := fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD %s NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;",
		postgres.Quote{}.ID(name), postgres.Quote{}.Value(password))
	_, _, err := repo.Exec(ctx, sql)
	return err
}

func transferDatabaseOwnership(ctx context.Context, repo rel.Repository, dbName, dbUser string) error {
	sql := fmt.Sprintf("ALTER DATABASE %s OWNER TO %s;",
		postgres.Quote{}.ID(dbName), postgres.Quote{}.ID(dbUser))
	_, _, err := repo.Exec(ctx, sql)
	return err
}

func revokePublicDatabaseAccess(ctx context.Context, repo rel.Repository, dbName string) error {
	sql := fmt.Sprintf("REVOKE CONNECT ON DATABASE %s FROM PUBLIC;",
		postgres.Quote{}.ID(dbName))
	_, _, err := repo.Exec(ctx, sql)
	return err
}

func grantDatabaseAccess(ctx context.Context, repo rel.Repository, dbName, dbUser string) error {
	sql := fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s;",
		postgres.Quote{}.ID(dbName), postgres.Quote{}.ID(dbUser))
	_, _, err := repo.Exec(ctx, sql)
	return err
}
