package router

import (
	"net/http"

	"github.com/razzie-cloud/database-broker/internal/auth"
	"github.com/razzie-cloud/database-broker/internal/broker"
	"github.com/razzie-cloud/database-broker/internal/health"
	"github.com/razzie-cloud/database-broker/internal/logging"

	"github.com/go-chi/chi/v5"
)

func New(broker broker.Interface) http.Handler {
	return WithHealth(broker, nil)
}

// WithHealth keeps probes outside access logging and API middleware.
func WithHealth(broker broker.Interface, monitor *health.Monitor) http.Handler {
	return WithAuth(broker, monitor, nil)
}

// WithAuth uses nil only for explicitly selected legacy mode.
func WithAuth(broker broker.Interface, monitor *health.Monitor, verifier *auth.Verifier) http.Handler {
	ctrl := &controller{broker: broker, auth: verifier}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			defer func() { logging.SetRoute(req.Context(), chi.RouteContext(req.Context()).RoutePattern()) }()
			next.ServeHTTP(w, req)
		})
	})
	r.Route("/v1", func(r chi.Router) {
		r.Get("/instances/{adapter_name}", ctrl.listInstances)
		r.Get("/instances/{adapter_name}/{instance_name}", ctrl.getInstance)
		r.Get("/instances/{adapter_name}/{instance_name}/uri", ctrl.getInstanceURI)
	})
	return logging.Middleware(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !monitor.Handle(w, req) {
			r.ServeHTTP(w, req)
		}
	}))
}
