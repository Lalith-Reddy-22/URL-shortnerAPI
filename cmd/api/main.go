// Command api is the URL shortener HTTP server.
// This file is wiring only: load config, open pools, build the router, listen, shut down.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/lalith/urlshortener/internal/config"
	"github.com/lalith/urlshortener/internal/handler"
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
	clicks := service.NewClickCounter(db, 4096, 100, time.Second, cfg.RequestTimeout)
	workerCtx, stopWorker := context.WithCancel(context.Background())
	var workerWG sync.WaitGroup
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		clicks.Run(workerCtx)
	}()

	linkSvc := &service.Links{
		Store:  db,
		Cache:  cache,
		Clicks: clicks,
		TTL:    cfg.CacheTTL,
	}
	linkH := handler.Links{Links: linkSvc}

	r := newRouter(cfg.RequestTimeout, cfg.RateLimitPerMinute, db, cache, cache, authSvc, authH, linkH)

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
	stopWorker()
	workerWG.Wait()
	slog.Info("stopped")
}
