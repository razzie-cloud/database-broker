package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/razzie-cloud/database-broker/internal/adapter/postgres"
	"github.com/razzie-cloud/database-broker/internal/broker"
	"github.com/razzie-cloud/database-broker/internal/config"
	"github.com/razzie-cloud/database-broker/internal/router"
)

func main() {
	cfg := config.Load()

	a, err := postgres.New(cfg.PostgresURI)
	if err != nil {
		log.Fatal("Failed to connect to database: ", err)
	}
	defer a.Close()

	b := broker.New()
	b.RegisterAdapter("postgres", a)
	r := router.New(b)
	addr := fmt.Sprintf(":%d", cfg.ServicePort)
	log.Print("Listening on ", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
