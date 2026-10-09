package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Incrementer is the Redis INCR+EXPIRE primitive used by RateLimit.
type Incrementer interface {
	Increment(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

// RateLimit is a fixed 1-minute window per client IP, stored in Redis so
// multiple API processes share the same budget.
// Redis errors fail open: we would rather serve than 429 the whole site.
func RateLimit(counter Incrementer, perMinute int) func(http.Handler) http.Handler {
	if perMinute < 1 {
		perMinute = 60
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}

			ip := clientIP(r)
			key := fmt.Sprintf("rl:%s:%d", ip, time.Now().Unix()/60)
			n, err := counter.Increment(r.Context(), key, 70*time.Second)
			if err != nil {
				slog.Error("rate limit", "err", err, "request_id", RequestIDFromContext(r.Context()))
				next.ServeHTTP(w, r)
				return
			}
			if n > int64(perMinute) {
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
