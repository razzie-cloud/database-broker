package main

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if e := probeOnce(context.Background(), args); e == nil {
		t.Fatal("accepted invalid TLS hostname")
	}
}

func TestProbeWaitsForReadinessAndRespectsDeadline(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	ca := filepath.Join(t.TempDir(), "ca")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	origin, _ := url.Parse(server.URL)
	args := []string{"--origin", server.URL, "--ca-file", ca, "--addresses", origin.Host, "--readiness-only", "--max-wait", "2s"}
	if err := probe(args); err != nil || calls < 2 {
		t.Fatal("did not wait for ready worker")
	}
	args[len(args)-1] = "1ns"
	before := time.Now()
	if err := probe(args); err == nil || time.Since(before) > time.Second {
		t.Fatal("probe deadline not respected")
	}
}

func TestValidatePublicPKIBeforeSwarmMutation(t *testing.T) {
	s := httptest.NewTLSServer(http.NotFoundHandler())
	defer s.Close()
	cert := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}))
	payload, _ := json.Marshal(map[string]string{"ca": cert, "cert": cert})
	if validateTLS([]string{"--hostname", "127.0.0.1"}, strings.NewReader(string(payload))) != nil {
		t.Fatal("valid public PKI")
	}
	for _, host := range []string{"wrong.example", ""} {
		if validateTLS([]string{"--hostname", host}, strings.NewReader(string(payload))) == nil {
			t.Fatal("invalid certificate hostname")
		}
	}
	if validateTLS([]string{"--hostname", "127.0.0.1"}, strings.NewReader(`{"ca":"invalid","cert":"invalid"}`)) == nil {
		t.Fatal("invalid PEM")
	}
}
