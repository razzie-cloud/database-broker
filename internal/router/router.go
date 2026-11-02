package router

import (
	"net/http"

	"github.com/razzie-cloud/database-broker/internal/broker"
	"github.com/razzie-cloud/database-broker/internal/health"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func New(broker broker.Interface) http.Handler {
	return WithHealth(broker, nil)
}

// WithHealth keeps probes outside access logging and API middleware.
func WithHealth(broker broker.Interface, monitor *health.Monitor) http.Handler {
	ctrl := &controller{broker: broker}
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Route("/v1", func(r chi.Router) {
		r.Get("/instances/{adapter_name}", ctrl.listInstances)
		r.Get("/instances/{adapter_name}/{instance_name}", ctrl.getInstance)
		r.Get("/instances/{adapter_name}/{instance_name}/uri", ctrl.getInstanceURI)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !monitor.Handle(w, req) {
			r.ServeHTTP(w, req)
		}
	})
}
