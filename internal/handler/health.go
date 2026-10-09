package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/lalith/urlshortener/internal/repository"
)

// Health reports process liveness. It does not check dependencies.
func Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready checks Postgres and Redis. Orchestrators use this to decide
// whether the instance should receive traffic.
type Ready struct {
	DB      repository.Pinger
	Cache   repository.Pinger
	Timeout time.Duration
}

func (h Ready) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.Timeout)
	defer cancel()

	if err := h.DB.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "postgres unavailable"})
		return
	}
	if err := h.Cache.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "redis unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
