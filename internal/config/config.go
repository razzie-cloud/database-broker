package config

import (
	"log"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	ServicePort  int    `envconfig:"SERVICE_PORT" default:"8080"`
	PostgresURI  string `envconfig:"POSTGRES_URI" required:"true"`
	DragonflyURI string `envconfig:"DRAGONFLY_URI" required:"true"`
}

func Load() *Config {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatal("Failed to load config: ", err)
	}
	return &cfg
}
