package config

import (
	"fmt"
	"net/url"
	"os"

	"github.com/alexflint/go-arg"
)

type Config struct {
	LegacyHTTPPort        int    `arg:"--legacy-http-port,env:LEGACY_HTTP_PORT"`
	HealthPort            int    `arg:"--health-port,env:HEALTH_PORT" default:"8081"`
	AuthMode              string `arg:"--auth-mode,env:AUTH_MODE" default:"token"`
	AuthPolicyFile        string `arg:"--auth-policy-file,env:AUTH_POLICY_FILE"`
	AuthDeploymentID      string `arg:"--auth-deployment-id,env:AUTH_DEPLOYMENT_ID"`
	AuthKeyFiles          string `arg:"--auth-key-files,env:AUTH_KEY_FILES"`
	TLSCertFile           string `arg:"--tls-cert-file,env:TLS_CERT_FILE"`
	TLSKeyFile            string `arg:"--tls-key-file,env:TLS_KEY_FILE"`
	ServicePort           int    `arg:"--port,env:SERVICE_PORT" default:"8080"`
	PostgresURI           string `arg:"--postgres-uri,env:POSTGRES_URI"`
	PostgresPasswordFile  string `arg:"--postgres-password-file,env:POSTGRES_PASSWORD_FILE"`
	DragonflyURI          string `arg:"--dragonfly-uri,env:DRAGONFLY_URI"`
	DragonflyPasswordFile string `arg:"--dragonfly-password-file,env:DRAGONFLY_PASSWORD_FILE"`
	ValkeyURI             string `arg:"--valkey-uri,env:VALKEY_URI"`
	ValkeyPasswordFile    string `arg:"--valkey-password-file,env:VALKEY_PASSWORD_FILE"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := arg.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if explicitEmptyMode() || (cfg.AuthMode != "token" && cfg.AuthMode != "none") {
		return nil, fmt.Errorf("AUTH_MODE must be token or none")
	}
	if err := cfg.ValidateTLS(); err != nil {
		return nil, err
	}
	if _, err := cfg.Authentication(); err != nil {
		return nil, err
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

	if cfg.ValkeyURI != "" && cfg.ValkeyPasswordFile != "" {
		pwd, err := os.ReadFile(cfg.ValkeyPasswordFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read Valkey password file: %w", err)
		}
		uri, err := url.Parse(cfg.ValkeyURI)
		if err != nil {
			return nil, fmt.Errorf("invalid Valkey URI: %w", err)
		}
		uri.User = url.UserPassword(uri.User.Username(), string(pwd))
		cfg.ValkeyURI = uri.String()
	}

	return &cfg, nil
}
