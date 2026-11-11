package router

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/razzie-cloud/database-broker/internal/adapter"
	"github.com/razzie-cloud/database-broker/internal/auth"
	"github.com/razzie-cloud/database-broker/internal/broker"
)

type countedBroker struct {
	broker.Interface
	calls int
}

func (b *countedBroker) GetOrCreateInstance(context.Context, string, string) (adapter.Instance, error) {
	b.calls++
	return testInstance{}, nil
}
func (b *countedBroker) GetInstances(context.Context, string) ([]string, error) {
	b.calls++
	return []string{}, nil
}

type testInstance struct{}

func (testInstance) GetJSON() any   { return map[string]string{"uri": "redis://secret"} }
func (testInstance) GetURI() string { return "redis://secret" }
func TestScopedRoutes(t *testing.T) {
	p, e := auth.Parse(strings.NewReader("version: 1\ndeployment_id: test\nkey_id: k1\nclients:\n  app:\n    generation: 1\n    allow:\n      - adapter: redis\n        instance: app\n"), "test")
	if e != nil {
		t.Fatal(e)
	}
	k := bytes.Repeat([]byte{1}, 32)
	token, _ := auth.Generate(p, k, "app")
	verifier, _ := auth.NewVerifier(p, map[string][]byte{"k1": k})
	for _, suffix := range []string{"", "/uri"} {
		for _, tt := range []struct {
			path, query   string
			status, calls int
		}{
			{"redis/app", "", 401, 0}, {"redis/app", "?client=app&token=bad", 401, 0},
			{"redis/app", "?client=app&client=app&token=" + token, 401, 0},
			{"redis/app", "?client=app&token=" + token + "&token=" + token, 401, 0},
			{"redis/app", "?client=app&token=" + token + "&bad=%zz", 401, 0},
			{"redis/other", "?client=app&token=" + token, 403, 0},
			{"valkey/app", "?client=app&token=" + token, 403, 0},
			{"ReDiS/APP", "?client=app&token=" + token, 200, 1},
		} {
			b := &countedBroker{}
			w := httptest.NewRecorder()
			WithAuth(b, nil, verifier).ServeHTTP(w, httptest.NewRequest("GET", "/v1/instances/"+tt.path+suffix+tt.query, nil))
			if w.Code != tt.status || b.calls != tt.calls || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status/calls/cache: %d/%d", w.Code, b.calls)
			}
		}
	}
	for _, path := range []string{"/v1/instances/redis?client=app&token=" + token, "/healthz", "/readyz"} {
		b := &countedBroker{}
		w := httptest.NewRecorder()
		WithAuth(b, nil, verifier).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if b.calls != 0 {
			t.Fatal("probe/list provisioned")
		}
		if strings.HasPrefix(path, "/v1") && w.Code != 403 {
			t.Fatal("listing allowed")
		}
	}
	b := &countedBroker{}
	w := httptest.NewRecorder()
	WithAuth(b, nil, nil).ServeHTTP(w, httptest.NewRequest("GET", "/v1/instances/redis/app/uri?client=bad&token=bad", nil))
	if w.Code != 200 || b.calls != 1 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("legacy mode")
	}
}
