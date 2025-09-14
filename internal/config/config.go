package config

import (
	"log"
	"net/url"
	"strconv"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	ServicePort  int    `envconfig:"SERVICE_PORT" default:"8080"`
	PostgresURI  string `envconfig:"POSTGRES_URI" required:"true"`
	PostgresHost string `envconfig:"-"`
	PostgresPort int    `envconfig:"-"`
}

func Load() *Config {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatal("Failed to load config: ", err)
	}
	uri, err := url.Parse(cfg.PostgresURI)
	if err != nil {
		log.Fatal("Failed to parse POSTGRES_URI: ", err)
	}
	if strings.Contains(uri.Host, ":") {
		parts := strings.Split(uri.Host, ":")
		cfg.PostgresHost = parts[0]
		cfg.PostgresPort, err = strconv.Atoi(parts[1])
		if err != nil {
			log.Fatal("Failed to parse port from POSTGRES_URI: ", err)
		}
	} else {
		cfg.PostgresHost = uri.Host
		cfg.PostgresPort = 5432
	}
	return &cfg
}
