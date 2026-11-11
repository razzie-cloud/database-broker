package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRejectOverridesAndPrograms(t *testing.T) {
	for _, args := range [][]string{{"--program", "/bin/sh"}, {"--program", "/razchess", "--", "--redis", "redis://invalid"}, {"--program", "/geoip-server", "--", "--redis-from=unsafe"}} {
		if run(args) == nil {
			t.Fatal("unsafe startup override")
		}
	}
}

func TestDiscoveryFailureNeverStartsApplication(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(401) }))
	defer server.Close()
	for _, program := range []string{"/razchess", "/geoip-server"} {
		if run([]string{"--program", program, "--url", server.URL}) == nil {
			t.Fatal("authentication failure must stop startup")
		}
	}
	if calls != 2 {
		t.Fatal("failure was retried without authentication")
	}
}
