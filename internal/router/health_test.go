package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func TestHealthProbesBypassAccessLoggingAndBroker(t *testing.T) {
	calls := 0
	previous := middleware.DefaultLogger
	middleware.DefaultLogger = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			next.ServeHTTP(w, r)
		})
	}
	defer func() { middleware.DefaultLogger = previous }()
	h := New(nil)
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", "/healthz", 200}, {"HEAD", "/healthz", 200}, {"GET", "/readyz", 503}, {"POST", "/readyz", 405}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
	if calls != 0 {
		t.Fatal("probes reached request logging")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/missing", nil))
	if calls != 1 || w.Code != 404 {
		t.Fatal("ordinary request logging or routing changed")
	}
}
