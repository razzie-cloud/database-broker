package config

import (
	"crypto/tls"
	"errors"
	"os"
	"strings"

	"github.com/razzie-cloud/database-broker/internal/auth"
)

// Authentication is evaluated before constructing any provisioning backend.
func (c *Config) Authentication() (*auth.Verifier, error) {
	if c.AuthMode == "none" {
		return nil, nil
	}
	if c.AuthMode != "token" {
		return nil, errors.New("AUTH_MODE must be token or none")
	}
	p, err := auth.LoadPolicy(c.AuthPolicyFile, c.AuthDeploymentID)
	if err != nil {
		return nil, err
	}
	keys := map[string][]byte{}
	for _, item := range strings.Split(c.AuthKeyFiles, ",") {
		id, path, ok := strings.Cut(item, "=")
		if !ok || !auth.ValidID(id) || path == "" || keys[id] != nil {
			return nil, errors.New("AUTH_KEY_FILES must contain unique key_id=file pairs")
		}
		k, err := auth.ReadKey(path)
		if err != nil {
			return nil, err
		}
		keys[id] = k
	}
	return auth.NewVerifier(p, keys)
}
func (c *Config) ValidateTLS() error {
	if c.ServicePort < 1 || c.ServicePort > 65535 || c.HealthPort < 1 || c.HealthPort > 65535 || c.LegacyHTTPPort < 0 || c.LegacyHTTPPort > 65535 {
		return errors.New("invalid broker listener port")
	}
	if c.LegacyHTTPPort != 0 && (c.AuthMode != "none" || c.TLSCertFile == "" || c.LegacyHTTPPort == c.ServicePort || c.LegacyHTTPPort == c.HealthPort) {
		return errors.New("legacy HTTP listener requires explicit none mode and separate TLS listener")
	}
	if c.TLSCertFile != "" && c.ServicePort == c.HealthPort {
		return errors.New("TLS and health listeners require distinct ports")
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return errors.New("TLS certificate and key files must be configured together")
	}
	if c.TLSCertFile != "" {
		if _, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile); err != nil {
			return errors.New("cannot load broker TLS certificate and key")
		}
	}
	return nil
}
func explicitEmptyMode() bool {
	if v, ok := os.LookupEnv("AUTH_MODE"); ok && v == "" {
		// A CLI mode overrides an explicitly empty environment setting.
		for i, a := range os.Args[1:] {
			if strings.HasPrefix(a, "--auth-mode=") {
				return a == "--auth-mode="
			}
			if a == "--auth-mode" && i+2 < len(os.Args) {
				return os.Args[i+2] == ""
			}
		}
		return true
	}
	return false
}
