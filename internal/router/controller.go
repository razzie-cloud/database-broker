package router

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/razzie-cloud/database-broker/internal/broker"
	"github.com/razzie-cloud/database-broker/internal/logging"

	"github.com/go-chi/chi/v5"
)

type controller struct {
	broker broker.Interface
}

func (ctrl *controller) listInstances(w http.ResponseWriter, r *http.Request) {
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
	resp.Error = err.Error()
	writeJSON(w, code, resp)
}
