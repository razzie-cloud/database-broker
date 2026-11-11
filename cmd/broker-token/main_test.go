package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommands(t *testing.T) {
	policy := "version: 1\ndeployment_id: test\nkey_id: k1\nclients:\n  app:\n    generation: 1\n    allow: []\n"
	var out bytes.Buffer
	if e := run([]string{"validate", "--policy", "-", "--deployment-id", "test"}, strings.NewReader(policy), &out); e != nil || !strings.Contains(out.String(), `"deployment_id":"test"`) {
		t.Fatal("validate")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "policy")
	k := filepath.Join(dir, "key")
	os.WriteFile(p, []byte(policy), 0600)
	os.WriteFile(k, make([]byte, 32), 0600)
	out.Reset()
	if e := run([]string{"generate", "--policy", p, "--deployment-id", "test", "--key-file", k, "--client", "app"}, nil, &out); e != nil || !strings.HasPrefix(out.String(), "v1.k1.1.") || strings.ContainsAny(out.String(), "\r\n ") {
		t.Fatal("generate")
	}
	for _, args := range [][]string{{"generate", "--key", "secret"}, {"generate", "--policy", p, "--deployment-id", "test", "--key-file", "/missing", "--client", "app"}, {"hold", "--max-lifetime", "0s"}} {
		out.Reset()
		if run(args, nil, &out) == nil || out.Len() != 0 {
			t.Fatal("failed CLI emitted output")
		}
	}
	if run([]string{"hold", "--max-lifetime", "1ms"}, nil, &out) != nil {
		t.Fatal("hold")
	}
}
