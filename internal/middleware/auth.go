package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lalith/urlshortener/internal/service"
)

type ctxKey int

const userIDKey ctxKey = 1

// UserIDFromContext is how handlers read the authenticated user.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey).(uuid.UUID)
	return id, ok && id != uuid.Nil
}

// JWTAuth rejects requests without a valid Bearer token.
// Protected routes call this via chi's r.Use(...).
func JWTAuth(auth *service.Auth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			scheme, token, ok := strings.Cut(header, " ")
			if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			id, err := auth.ParseToken(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
