package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/razzie-cloud/database-broker/internal/adapter/dragonfly"
	"github.com/razzie-cloud/database-broker/internal/adapter/postgres"
	"github.com/razzie-cloud/database-broker/internal/broker"
	"github.com/razzie-cloud/database-broker/internal/config"
	"github.com/razzie-cloud/database-broker/internal/health"
	"github.com/razzie-cloud/database-broker/internal/router"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	b := broker.New()
	var checks []health.Check

	if cfg.PostgresURI != "" {
		log.Println("Registering Postgres adapter")
		p, err := postgres.New(cfg.PostgresURI)
		if err != nil {
			log.Fatal("Failed to connect to Postgres: ", err)
		}
		defer p.Close()
		b.RegisterAdapter("postgres", p)
		checks = append(checks, health.Check{Name: "postgres", Ping: p.Ping})
	}

	if cfg.DragonflyURI != "" {
		log.Println("Registering Dragonfly adapter")
		d, err := dragonfly.New(cfg.DragonflyURI)
		if err != nil {
			log.Fatal("Failed to connect to Dragonfly: ", err)
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
	log.Print("Listening on ", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
