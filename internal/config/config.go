package config

import (
	"log"

	"github.com/alexflint/go-arg"
)

type Config struct {
	ServicePort  int    `arg:"--port,env:SERVICE_PORT" default:"8080"`
	PostgresURI  string `arg:"--postgres-uri,env:POSTGRES_URI"`
	DragonflyURI string `arg:"--dragonfly-uri,env:DRAGONFLY_URI"`
}

func Load() *Config {
	var cfg Config
	if err := arg.Parse(&cfg); err != nil {
		log.Fatal("Failed to load config: ", err)
	}
	return &cfg
}
