package config

import (
	"fmt"
	"net/url"
	"os"

	"github.com/alexflint/go-arg"
)

type Config struct {
	ServicePort           int    `arg:"--port,env:SERVICE_PORT" default:"8080"`
	PostgresURI           string `arg:"--postgres-uri,env:POSTGRES_URI"`
	PostgresPasswordFile  string `arg:"--postgres-password-file,env:POSTGRES_PASSWORD_FILE"`
	DragonflyURI          string `arg:"--dragonfly-uri,env:DRAGONFLY_URI"`
	DragonflyPasswordFile string `arg:"--dragonfly-password-file,env:DRAGONFLY_PASSWORD_FILE"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := arg.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.PostgresURI != "" && cfg.PostgresPasswordFile != "" {
		pwd, err := os.ReadFile(cfg.PostgresPasswordFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read Postgres password file: %w", err)
		}
		uri, err := url.Parse(cfg.PostgresURI)
		if err != nil {
			return nil, fmt.Errorf("invalid Postgres URI: %w", err)
		}
		uri.User = url.UserPassword(uri.User.Username(), string(pwd))
		cfg.PostgresURI = uri.String()
	}

	if cfg.DragonflyURI != "" && cfg.DragonflyPasswordFile != "" {
		pwd, err := os.ReadFile(cfg.DragonflyPasswordFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read Dragonfly password file: %w", err)
		}
		uri, err := url.Parse(cfg.DragonflyURI)
		if err != nil {
			return nil, fmt.Errorf("invalid Dragonfly URI: %w", err)
		}
		uri.User = url.UserPassword(uri.User.Username(), string(pwd))
		cfg.DragonflyURI = uri.String()
	}

	return &cfg, nil
}
