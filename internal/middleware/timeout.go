package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Timeout cancels the request context after d and returns 504 JSON if the
// handler has not written a response yet.
// The handler runs in a goroutine so we can return 504 without waiting for it.
// All writes are mutex-guarded because ResponseWriter is not concurrent-safe.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()

			tw := &timeoutWriter{ResponseWriter: w}
			done := make(chan struct{})
			go func() {
				next.ServeHTTP(tw, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
			case <-ctx.Done():
				tw.writeTimeout()
				// Do not wait for the handler: the client has already timed out.
				// Handlers that honor ctx (pgx, redis) return quickly after this.
			}
		})
	}
}

type timeoutWriter struct {
	http.ResponseWriter
	mu       sync.Mutex
	wrote    bool
	timedOut bool
}

func (w *timeoutWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timedOut || w.wrote {
		return
	}
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *timeoutWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timedOut {
		return 0, http.ErrHandlerTimeout
	}
	if !w.wrote {
		w.wrote = true
	}
	return w.ResponseWriter.Write(b)
}

func (w *timeoutWriter) Header() http.Header {
	return w.ResponseWriter.Header()
}

func (w *timeoutWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *timeoutWriter) writeTimeout() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.wrote {
		w.timedOut = true
		return
	}
	w.timedOut = true
	w.wrote = true
	w.Header().Set("Content-Type", "application/json")
	w.ResponseWriter.WriteHeader(http.StatusGatewayTimeout)
	_, _ = w.ResponseWriter.Write([]byte(`{"error":"request timeout"}` + "\n"))
}
