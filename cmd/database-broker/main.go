package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/razzie-cloud/database-broker/internal/adapter/dragonfly"
	"github.com/razzie-cloud/database-broker/internal/adapter/postgres"
	"github.com/razzie-cloud/database-broker/internal/adapter/valkey"
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
	verifier, err := cfg.Authentication()
	if err != nil {
		return logging.Wrap("auth_config", err)
	}
	if cfg.AuthMode == "none" {
		slog.Warn("Broker authentication disabled by explicit mode", "event", "broker.auth_mode", "mode", "none")
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

	if cfg.ValkeyURI != "" {
		v, err := valkey.New(cfg.ValkeyURI)
		if err != nil {
			return logging.Wrap("valkey_init", err)
		}
		defer v.Close()
		b.RegisterAdapter("valkey", v)
		checks = append(checks, health.Check{Name: "valkey", Ping: v.Ping})
	}

	monitor := health.New(checks...)
	stopHealth := monitor.Start(context.Background())
	defer stopHealth()
	r := router.WithAuth(b, monitor, verifier)
	addr := fmt.Sprintf(":%d", cfg.ServicePort)
	var adapters []string
	if cfg.PostgresURI != "" {
		adapters = append(adapters, "postgres")
	}
	if cfg.DragonflyURI != "" {
		adapters = append(adapters, "dragonfly", "redis")
	}
	if cfg.ValkeyURI != "" {
		adapters = append(adapters, "valkey")
	}
	if len(adapters) == 0 {
		slog.Warn("No backends configured; readiness will fail", "event", "broker.no_backends")
	}
	slog.Info("Server listening", "event", "server.listening", "address", addr, "adapters", adapters)
	server := &http.Server{Addr: addr, Handler: r, ErrorLog: logging.ServerLogger(), ReadHeaderTimeout: 5 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	if cfg.LegacyHTTPPort != 0 {
		legacy := &http.Server{Addr: fmt.Sprintf(":%d", cfg.LegacyHTTPPort), Handler: r, ReadHeaderTimeout: 5 * time.Second, ErrorLog: logging.ServerLogger()}
		defer legacy.Close()
		go func() {
			if err := legacy.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("Legacy listener failed", "event", "server.legacy_listener_failed")
				server.Close()
			}
		}()
	}
	if cfg.TLSCertFile != "" {
		probe := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", cfg.HealthPort), ReadHeaderTimeout: 5 * time.Second, ErrorLog: logging.ServerLogger(), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !monitor.Handle(w, r) {
				http.NotFound(w, r)
			}
		})}
		defer probe.Close()
		go func() {
			if err := probe.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("Health listener failed", "event", "health.listener_failed")
				server.Close()
			}
		}()
		return logging.Wrap("serve", server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile))
	}
	return logging.Wrap("serve", server.ListenAndServe())
}
