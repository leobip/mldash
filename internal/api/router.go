package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter creates and return a configured chi router
func NewRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", h.HealthCheck)
	r.Get("/", h.Index)

	r.Route("/api", func(r chi.Router) {
		r.Get("/gpus", h.GetGPUs)
		r.Get("/nodes", h.GetNodes)
		r.Get("/notebooks", h.GetNotebooks)
		r.Get("/workloads", h.GetGPUWorkloads)
		r.Get("/quotas", h.GetNamespaceQuotas)
	})

	return r
}
