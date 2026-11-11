package router

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/razzie-cloud/database-broker/internal/auth"
	"github.com/razzie-cloud/database-broker/internal/broker"
	"github.com/razzie-cloud/database-broker/internal/logging"

	"github.com/go-chi/chi/v5"
)

type controller struct {
	broker broker.Interface
	auth   *auth.Verifier
}

func (ctrl *controller) listInstances(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !ctrl.authorize(w, r) {
		return
	}
	ctx := r.Context()
	adapterName := chi.URLParam(r, "adapter_name")
	instances, err := ctrl.broker.GetInstances(ctx, adapterName)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, instances)
}

func (ctrl *controller) getInstance(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !ctrl.authorize(w, r) {
		return
	}
	ctx := r.Context()
	adapterName := chi.URLParam(r, "adapter_name")
	instanceName := chi.URLParam(r, "instance_name")
	instance, err := ctrl.broker.GetOrCreateInstance(ctx, adapterName, instanceName)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, instance.GetJSON())
}

func (ctrl *controller) getInstanceURI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !ctrl.authorize(w, r) {
		return
	}
	ctx := r.Context()
	adapterName := chi.URLParam(r, "adapter_name")
	instanceName := chi.URLParam(r, "instance_name")
	instance, err := ctrl.broker.GetOrCreateInstance(ctx, adapterName, instanceName)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/uri-list")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(instance.GetURI()))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	code := http.StatusInternalServerError
	if errWithCode, ok := err.(interface{ StatusCode() int }); ok {
		code = errWithCode.StatusCode()
	}
	if code >= 500 {
		logging.Fail(r.Context(), slog.LevelError, "broker", err)
	}
	var resp struct {
		Error string `json:"error"`
	}
	resp.Error = http.StatusText(code)
	if code < 500 {
		resp.Error = err.Error()
	}
	writeJSON(w, code, resp)
}

func (ctrl *controller) authorize(w http.ResponseWriter, r *http.Request) bool {
	if ctrl.auth == nil {
		return true
	}
	// ParseQuery rejects malformed escapes and semicolons instead of ignoring them.
	if len(r.URL.RawQuery) > 2048 {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return false
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(q["client"]) != 1 || len(q["token"]) != 1 || !ctrl.auth.Authenticate(q.Get("client"), q.Get("token")) {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return false
	}
	client := q.Get("client")
	logging.Fields(r.Context(), "client", client)
	if !ctrl.auth.Allows(client, chi.URLParam(r, "adapter_name"), chi.URLParam(r, "instance_name")) || chi.URLParam(r, "instance_name") == "" {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return false
	}
	return true
}
