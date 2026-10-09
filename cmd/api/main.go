// Command api is the URL shortener HTTP server.
// This file is wiring only: load config, open pools, build the router, listen, shut down.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lalith/urlshortener/internal/config"
	"github.com/lalith/urlshortener/internal/handler"
	mw "github.com/lalith/urlshortener/internal/middleware"
	"github.com/lalith/urlshortener/internal/repository"
	"github.com/lalith/urlshortener/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.RequestTimeout)
	db, err := repository.NewPostgres(ctx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		slog.Error("postgres", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	cache := repository.NewRedis(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	defer func() { _ = cache.Close() }()

	pingCtx, pingCancel := context.WithTimeout(context.Background(), cfg.RequestTimeout)
	if err := cache.Ping(pingCtx); err != nil {
		pingCancel()
		slog.Error("redis", "err", err)
		os.Exit(1)
	}
	pingCancel()

	authSvc := &service.Auth{
		Users:  db,
		Secret: []byte(cfg.JWTSecret),
		Expiry: cfg.JWTExpiry,
		Cost:   cfg.BcryptCost,
	}
	authH := handler.Auth{Auth: authSvc}
	linkSvc := &service.Links{
		Store: db,
		Cache: cache,
		TTL:   cfg.CacheTTL,
	}
	linkH := handler.Links{Links: linkSvc}

	r := chi.NewRouter()
	r.Get("/healthz", handler.Health)
	r.Method(http.MethodGet, "/readyz", handler.Ready{
		DB:      db,
		Cache:   cache,
		Timeout: cfg.RequestTimeout,
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", authH.Register)
		r.Post("/auth/login", authH.Login)

		r.Group(func(r chi.Router) {
			r.Use(mw.JWTAuth(authSvc))
			r.Post("/shorten", linkH.Shorten)
		})
	})

	// Registered last so /healthz, /readyz, and /api/v1 are not captured as codes.
	r.Get("/{code}", linkH.Redirect)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server", "err", err)
			os.Exit(1)
		}
	case sig := <-stop:
		slog.Info("shutdown signal", "signal", sig.String())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown", "err", err)
		os.Exit(1)
	}
	slog.Info("stopped")
}
