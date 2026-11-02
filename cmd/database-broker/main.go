package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/razzie-cloud/database-broker/internal/adapter/dragonfly"
	"github.com/razzie-cloud/database-broker/internal/adapter/postgres"
	"github.com/razzie-cloud/database-broker/internal/broker"
	"github.com/razzie-cloud/database-broker/internal/config"
	"github.com/razzie-cloud/database-broker/internal/health"
	"github.com/razzie-cloud/database-broker/internal/logging"
	"github.com/razzie-cloud/database-broker/internal/router"
)

func main() {
	if err := logging.Setup("database-broker"); err != nil {
		slog.Error(err.Error(), "event", "config.invalid", "field", "LOG_LEVEL")
		os.Exit(1)
	}
	if err := run(); err != nil {
		slog.Error("Service failed", append([]any{"event", "server.failed"}, logging.ErrorAttrs(err)...)...)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return logging.Wrap("config", err)
	}
	b := broker.New()
	var checks []health.Check

	if cfg.PostgresURI != "" {
		p, err := postgres.New(cfg.PostgresURI)
		if err != nil {
			return logging.Wrap("postgres_init", err)
		}
		defer p.Close()
		b.RegisterAdapter("postgres", p)
		checks = append(checks, health.Check{Name: "postgres", Ping: p.Ping})
	}

	if cfg.DragonflyURI != "" {
		d, err := dragonfly.New(cfg.DragonflyURI)
		if err != nil {
			return logging.Wrap("dragonfly_init", err)
		}
		defer d.Close()
		b.RegisterAdapter("dragonfly", d)
		b.RegisterAdapter("redis", d)
		checks = append(checks, health.Check{Name: "dragonfly", Ping: d.Ping})
	}

	monitor := health.New(checks...)
	stopHealth := monitor.Start(context.Background())
	defer stopHealth()
	r := router.WithHealth(b, monitor)
	addr := fmt.Sprintf(":%d", cfg.ServicePort)
	var adapters []string
	if cfg.PostgresURI != "" {
		adapters = append(adapters, "postgres")
	}
	if cfg.DragonflyURI != "" {
		adapters = append(adapters, "dragonfly", "redis")
	}
	if len(adapters) == 0 {
		slog.Warn("No backends configured; readiness will fail", "event", "broker.no_backends")
	}
	slog.Info("Server listening", "event", "server.listening", "address", addr, "adapters", adapters)
	server := &http.Server{Addr: addr, Handler: r, ErrorLog: logging.ServerLogger()}
	return logging.Wrap("serve", server.ListenAndServe())
}
