package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

const (
	requestIDKey    ctxKey = 2
	requestIDHeader        = "X-Request-ID"
)

func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// RequestID echoes an incoming X-Request-ID or generates a UUID.
// Downstream logs and clients can correlate with the same value.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
