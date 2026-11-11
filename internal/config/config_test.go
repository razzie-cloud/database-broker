package config

import (
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad_FromEnv(t *testing.T) {
	t.Setenv("AUTH_MODE", "none")
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"cmd"}
	t.Setenv("SERVICE_PORT", "12345")
	t.Setenv("POSTGRES_URI", "postgres://user:pass@localhost:5432/testdb")
	t.Setenv("DRAGONFLY_URI", "dragon://localhost:3000")
	t.Setenv("VALKEY_URI", "redis://localhost:6379/0")

	cfg, err := Load()
	assert.NoError(t, err, "Load should not return an error")
	assert.Equal(t, 12345, cfg.ServicePort, "ServicePort should be loaded from env")
	assert.Equal(t, "postgres://user:pass@localhost:5432/testdb", cfg.PostgresURI)
	assert.Equal(t, "dragon://localhost:3000", cfg.DragonflyURI)
	assert.Equal(t, "redis://localhost:6379/0", cfg.ValkeyURI)
}

func TestLoad_FromArgs(t *testing.T) {
	t.Setenv("AUTH_MODE", "none")
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// ensure relevant env vars are unset for this test; register cleanup to restore them
	keys := []string{"SERVICE_PORT", "POSTGRES_URI", "DRAGONFLY_URI", "VALKEY_URI"}
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
		"--valkey-uri", "rediss://cli:pass@localhost:6379/3",
	}

	cfg, err := Load()
	assert.NoError(t, err, "Load should not return an error")
	assert.Equal(t, 54321, cfg.ServicePort, "ServicePort should be loaded from CLI args")
	assert.Equal(t, "postgres://cli:pass@localhost:5432/clidb", cfg.PostgresURI)
	assert.Equal(t, "dragon://cli:4000", cfg.DragonflyURI)
	assert.Equal(t, "rediss://cli:pass@localhost:6379/3", cfg.ValkeyURI)
}

func TestLoad_ValkeyPasswordFile(t *testing.T) {
	t.Setenv("AUTH_MODE", "none")
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"cmd"}
	pwdPath := t.TempDir() + "/valkey-password"
	pwd := "secret:@/with-newline\n"
	assert.NoError(t, os.WriteFile(pwdPath, []byte(pwd), 0600))
	t.Setenv("VALKEY_URI", "rediss://admin:old@localhost:6379/2")
	t.Setenv("VALKEY_PASSWORD_FILE", pwdPath)
	cfg, err := Load()
	if !assert.NoError(t, err) {
		return
	}
	u, err := url.Parse(cfg.ValkeyURI)
	assert.NoError(t, err)
	got, _ := u.User.Password()
	assert.Equal(t, pwd, got)
	assert.Equal(t, "admin", u.User.Username())
	assert.Equal(t, "rediss", u.Scheme)
	assert.Equal(t, "/2", u.Path)

	t.Setenv("VALKEY_PASSWORD_FILE", pwdPath+"-missing")
	_, err = Load()
	assert.ErrorContains(t, err, "failed to read Valkey password file")
	t.Setenv("VALKEY_URI", "")
	_, err = Load()
	assert.NoError(t, err, "password file is unused without URI")
}

func TestLoad_PostgresPasswordFile(t *testing.T) {
	t.Setenv("AUTH_MODE", "none")
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"cmd"}

	// create a temp password file
	dir := t.TempDir()
	pwdPath := dir + "/pgpwd"
	pwd := "pg-secret"
	if err := os.WriteFile(pwdPath, []byte(pwd), 0600); err != nil {
		t.Fatalf("failed to write temp password file: %v", err)
	}

	t.Setenv("POSTGRES_URI", "postgres://user@localhost:5432/testdb")
	t.Setenv("POSTGRES_PASSWORD_FILE", pwdPath)

	cfg, err := Load()
	assert.NoError(t, err, "Load should not return an error when using postgres password file")
	// parse the resulting URI and ensure the password was injected
	u, err := url.Parse(cfg.PostgresURI)
	assert.NoError(t, err, "resulting Postgres URI should parse")
	gotPwd, _ := u.User.Password()
	assert.Equal(t, pwd, gotPwd, "password from file should be set in Postgres URI")
}

func TestLoad_DragonflyPasswordFile(t *testing.T) {
	t.Setenv("AUTH_MODE", "none")
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"cmd"}

	dir := t.TempDir()
	pwdPath := dir + "/dfpwd"
	pwd := "df-secret"
	if err := os.WriteFile(pwdPath, []byte(pwd), 0600); err != nil {
		t.Fatalf("failed to write temp password file: %v", err)
	}

	t.Setenv("DRAGONFLY_URI", "redis://user@localhost:3000/0")
	t.Setenv("DRAGONFLY_PASSWORD_FILE", pwdPath)

	cfg, err := Load()
	assert.NoError(t, err, "Load should not return an error when using dragonfly password file")
	u, err := url.Parse(cfg.DragonflyURI)
	assert.NoError(t, err, "resulting Dragonfly URI should parse")
	gotPwd, _ := u.User.Password()
	assert.Equal(t, pwd, gotPwd, "password from file should be set in Dragonfly URI")
}
