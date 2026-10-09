package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lalith/urlshortener/internal/handler"
	mw "github.com/lalith/urlshortener/internal/middleware"
	"github.com/lalith/urlshortener/internal/repository"
	"github.com/lalith/urlshortener/internal/service"
)

func newRouter(
	timeout time.Duration,
	perMinute int,
	db repository.Pinger,
	cache mw.Incrementer,
	cachePing repository.Pinger,
	authSvc *service.Auth,
	authH handler.Auth,
	linkH handler.Links,
) http.Handler {
	r := chi.NewRouter()
	r.Use(mw.RequestID)
	r.Use(mw.Logger)
	r.Use(mw.Recoverer)
	r.Use(mw.Timeout(timeout))
	r.Use(mw.RateLimit(cache, perMinute))

	r.Get("/healthz", handler.Health)
	r.Method(http.MethodGet, "/readyz", handler.Ready{
		DB:      db,
		Cache:   cachePing,
		Timeout: timeout,
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", authH.Register)
		r.Post("/auth/login", authH.Login)

		r.Group(func(r chi.Router) {
			r.Use(mw.JWTAuth(authSvc))
			r.Post("/shorten", linkH.Shorten)
			r.Get("/links", linkH.List)
			r.Get("/links/{code}/stats", linkH.Stats)
			r.Delete("/links/{code}", linkH.Delete)
		})
	})

	r.Get("/{code}", linkH.Redirect)
	return r
}
