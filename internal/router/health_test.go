package router

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthProbesBypassAccessLoggingAndBroker(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)
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
	if output.Len() != 0 {
		t.Fatal("probes reached request logging")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/missing", nil))
	if !strings.Contains(output.String(), `"event":"request.completed"`) || w.Code != 404 {
		t.Fatal("ordinary request logging or routing changed")
	}
}
