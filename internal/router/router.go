package router

import (
	"net/http"

	"github.com/razzie-cloud/database-broker/internal/broker"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func New(broker broker.Interface) http.Handler {
	ctrl := &controller{broker: broker}
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Route("/v1", func(r chi.Router) {
		r.Get("/instances/{adapter_name}", ctrl.listInstances)
		r.Get("/instances/{adapter_name}/{instance_name}", ctrl.getInstance)
		r.Get("/instances/{adapter_name}/{instance_name}/uri", ctrl.getInstanceURI)
	})
	return r
}
