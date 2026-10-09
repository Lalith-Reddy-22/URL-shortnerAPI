package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lalith/urlshortener/internal/service"
	"golang.org/x/crypto/bcrypt"
)

func TestJWTAuth(t *testing.T) {
	auth := &service.Auth{Secret: []byte("test-secret"), Expiry: time.Hour, Cost: bcrypt.MinCost}
	id := uuid.New()
	tok, err := auth.IssueToken(id)
	if err != nil {
		t.Fatal(err)
	}

	protected := JWTAuth(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := UserIDFromContext(r.Context())
		if !ok || got != id {
			t.Errorf("context user = %v ok=%v", got, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	t.Run("missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
	})
}
