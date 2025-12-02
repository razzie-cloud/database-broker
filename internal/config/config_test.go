package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad_FromEnv(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"cmd"}
	t.Setenv("SERVICE_PORT", "12345")
	t.Setenv("POSTGRES_URI", "postgres://user:pass@localhost:5432/testdb")
	t.Setenv("DRAGONFLY_URI", "dragon://localhost:3000")

	cfg := Load()

	assert.Equal(t, 12345, cfg.ServicePort, "ServicePort should be loaded from env")
	assert.Equal(t, "postgres://user:pass@localhost:5432/testdb", cfg.PostgresURI)
	assert.Equal(t, "dragon://localhost:3000", cfg.DragonflyURI)
}

func TestLoad_FromArgs(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// ensure relevant env vars are unset for this test; register cleanup to restore them
	keys := []string{"SERVICE_PORT", "POSTGRES_URI", "DRAGONFLY_URI"}
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			// restore original value at cleanup
			val := v
			t.Cleanup(func() { os.Setenv(k, val) })
		} else {
			// ensure unset at cleanup
			t.Cleanup(func() { os.Unsetenv(k) })
		}
		os.Unsetenv(k)
	}

	os.Args = []string{
		"cmd",
		"--port", "54321",
		"--postgres-uri", "postgres://cli:pass@localhost:5432/clidb",
		"--dragonfly-uri", "dragon://cli:4000",
	}

	cfg := Load()

	assert.Equal(t, 54321, cfg.ServicePort, "ServicePort should be loaded from CLI args")
	assert.Equal(t, "postgres://cli:pass@localhost:5432/clidb", cfg.PostgresURI)
	assert.Equal(t, "dragon://cli:4000", cfg.DragonflyURI)
}
