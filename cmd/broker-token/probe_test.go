package main

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbePerReplicaWithVerifiedTLS(t *testing.T) {
	calls := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/readyz" {
			w.WriteHeader(200)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Query().Get("token") != "" {
			w.WriteHeader(403)
		} else {
			w.WriteHeader(401)
		}
	}))
	defer s.Close()
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca")
	token := filepath.Join(dir, "token")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0600)
	os.WriteFile(token, []byte("v1.k1.1."+strings.Repeat("A", 43)), 0600)
	u, _ := url.Parse(s.URL)
	args := []string{"--origin", s.URL, "--ca-file", ca, "--addresses", u.Host + "," + u.Host, "--token-file", token, "--client", "app"}
	if e := probe(args); e != nil || calls != 10 {
		t.Fatal("replica probe failed")
	}
	args[1] = "https://invalid.example:" + u.Port()
	if e := probe(args); e == nil {
		t.Fatal("accepted invalid TLS hostname")
	}
}
