package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuthenticationModes(t *testing.T) {
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"broker"}
	for _, k := range []string{"AUTH_MODE", "AUTH_POLICY_FILE", "AUTH_KEY_FILES", "AUTH_DEPLOYMENT_ID", "POSTGRES_URI", "VALKEY_URI", "DRAGONFLY_URI", "TLS_CERT_FILE", "TLS_KEY_FILE"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	if _, e := Load(); e == nil {
		t.Fatal("default token mode must require configuration")
	}
	for _, mode := range []string{"", "invalid"} {
		t.Setenv("AUTH_MODE", mode)
		if _, e := Load(); e == nil {
			t.Fatal("invalid mode")
		}
	}
	t.Setenv("AUTH_MODE", "none")
	t.Setenv("AUTH_POLICY_FILE", "/does-not-exist")
	t.Setenv("AUTH_KEY_FILES", "missing=/missing")
	c, e := Load()
	if e != nil || c.AuthMode != "none" {
		t.Fatal("explicit none must not read authentication files")
	}
	dir := t.TempDir()
	policy := filepath.Join(dir, "policy")
	key := filepath.Join(dir, "key")
	os.WriteFile(policy, []byte("version: 1\ndeployment_id: test\nkey_id: k1\nclients:\n  app:\n    generation: 1\n    allow: []\n"), 0600)
	os.WriteFile(key, make([]byte, 32), 0600)
	t.Setenv("AUTH_MODE", "token")
	t.Setenv("AUTH_POLICY_FILE", policy)
	t.Setenv("AUTH_KEY_FILES", "k1="+key)
	t.Setenv("AUTH_DEPLOYMENT_ID", "test")
	if _, e := Load(); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(key, make([]byte, 31), 0600)
	if _, e := Load(); e == nil {
		t.Fatal("bad key")
	}
}
