package postgres

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/razzie-cloud/database-broker/internal/adapter"
	"github.com/razzie-cloud/database-broker/internal/util"

	"github.com/go-rel/postgres"
	"github.com/go-rel/rel"
	"github.com/lib/pq"
)

type PostgresAdapter interface {
	adapter.Interface
	Close() error
}

type postgresAdapter struct {
	adapter rel.Adapter
	repo    rel.Repository
	host    string
	port    int
}

func New(postgresUri string) (PostgresAdapter, error) {
	host, port, err := util.GetURIHostPort(postgresUri, 5432)
	if err != nil {
		return nil, fmt.Errorf("parse postgres uri: %w", err)
	}
	adapter, err := postgres.Open(postgresUri)
	if err != nil {
		return nil, err
	}
	repo := rel.New(adapter)
	repo.Instrumentation(rel.Instrumenter(func(ctx context.Context, op, message string, args ...any) func(err error) {
		return func(err error) {
			if err != nil && err != rel.ErrNotFound {
				log.Printf("[op: %s] %s - %v", op, fmt.Sprintf(message, args...), err)
			}
		}
	}))
	migrate(repo)
	return &postgresAdapter{
		adapter: adapter,
		repo:    repo,
		host:    host,
		port:    port,
	}, nil
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

func (pg *postgresAdapter) GetOrCreateInstance(ctx context.Context, instanceName string) (adapter.Instance, error) {
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
	if err := createDatabase(ctx, pg.repo, dbName); err != nil {
		return nil, err
	}
	err = pg.repo.Transaction(ctx, func(txCtx context.Context) error {
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
		if err := pg.repo.Insert(txCtx, instance); err != nil {
			return fmt.Errorf("insert instance: %w", err)
		}
		if err := transferDatabaseOwnership(txCtx, pg.repo, dbName, dbUser); err != nil {
			return fmt.Errorf("transfer db ownership: %w", err)
		}
		if err := revokePublicDatabaseAccess(txCtx, pg.repo, dbName); err != nil {
			return fmt.Errorf("revoke db public access: %w", err)
		}
		if err := grantDatabaseAccess(txCtx, pg.repo, dbName, dbUser); err != nil {
			return fmt.Errorf("grant db connect access: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return instance, nil
}

func createDatabase(ctx context.Context, repo rel.Repository, name string) error {
	sql := fmt.Sprintf("CREATE DATABASE %s", pq.QuoteIdentifier(name))
	_, _, err := repo.Exec(ctx, sql)
	if err == nil {
		return nil
	}
	const pgErrDuplicateDB = "42P04" // https://www.postgresql.org/docs/current/errcodes-appendix.html
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && string(pqErr.Code) == pgErrDuplicateDB {
		return nil
	}
	return err
}

func createUser(ctx context.Context, repo rel.Repository, name, password string) error {
	sql := fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD %s NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;",
		pq.QuoteIdentifier(name), pq.QuoteLiteral(password))
	_, _, err := repo.Exec(ctx, sql)
	return err
}

func transferDatabaseOwnership(ctx context.Context, repo rel.Repository, dbName, dbUser string) error {
	sql := fmt.Sprintf("ALTER DATABASE %s OWNER TO %s;",
		pq.QuoteIdentifier(dbName), pq.QuoteIdentifier(dbUser))
	_, _, err := repo.Exec(ctx, sql)
	return err
}

func revokePublicDatabaseAccess(ctx context.Context, repo rel.Repository, dbName string) error {
	sql := fmt.Sprintf("REVOKE CONNECT ON DATABASE %s FROM PUBLIC;",
		pq.QuoteIdentifier(dbName))
	_, _, err := repo.Exec(ctx, sql)
	return err
}

func grantDatabaseAccess(ctx context.Context, repo rel.Repository, dbName, dbUser string) error {
	sql := fmt.Sprintf("GRANT CONNECT ON DATABASE %s TO %s;",
		pq.QuoteIdentifier(dbName), pq.QuoteIdentifier(dbUser))
	_, _, err := repo.Exec(ctx, sql)
	return err
}
