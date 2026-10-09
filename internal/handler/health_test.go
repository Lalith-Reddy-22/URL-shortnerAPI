package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

type stubPinger struct {
	err error
}

func (s stubPinger) Ping(context.Context) error { return s.err }

func TestReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		dbErr      error
		cacheErr   error
		wantStatus int
	}{
		{name: "ok", wantStatus: http.StatusOK},
		{name: "db down", dbErr: errors.New("db"), wantStatus: http.StatusServiceUnavailable},
		{name: "cache down", cacheErr: errors.New("cache"), wantStatus: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := Ready{
				DB:      stubPinger{err: tt.dbErr},
				Cache:   stubPinger{err: tt.cacheErr},
				Timeout: time.Second,
			}
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
