package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lalith/urlshortener/internal/handler"
	"github.com/lalith/urlshortener/internal/model"
	"github.com/lalith/urlshortener/internal/repository"
	"github.com/lalith/urlshortener/internal/service"
	"golang.org/x/crypto/bcrypt"
)

type pingOK struct{}

func (pingOK) Ping(context.Context) error { return nil }

type limitOK struct{}

func (limitOK) Increment(context.Context, string, time.Duration) (int64, error) {
	return 1, nil
}

type noopStore struct{}

func (noopStore) CreateLink(_ context.Context, link model.Link) (model.Link, error) {
	return link, nil
}
func (noopStore) GetByCode(context.Context, string) (model.Link, error) {
	return model.Link{}, repository.ErrNotFound
}
func (noopStore) ListByUser(context.Context, uuid.UUID, int, int) ([]model.Link, int, error) {
	return []model.Link{}, 0, nil
}
func (noopStore) DeleteByCode(context.Context, uuid.UUID, string) error {
	return repository.ErrNotFound
}
func (noopStore) AddClicks(context.Context, []model.ClickDelta) error { return nil }

type noopCache struct{}

func (noopCache) GetLink(context.Context, string) (model.CachedLink, error) {
	return model.CachedLink{}, repository.ErrNotFound
}
func (noopCache) SetLink(context.Context, string, model.CachedLink, time.Duration) error {
	return nil
}
func (noopCache) DeleteLink(context.Context, string) error { return nil }

func TestRouterHealthAndAuthz(t *testing.T) {
	authSvc := &service.Auth{Secret: []byte("s"), Expiry: time.Hour, Cost: bcrypt.MinCost}
	h := newRouter(
		time.Second,
		60,
		pingOK{},
		limitOK{},
		pingOK{},
		authSvc,
		handler.Auth{Auth: authSvc},
		handler.Links{Links: &service.Links{Store: noopStore{}, Cache: noopCache{}, TTL: time.Hour}},
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("readyz %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/shorten", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("shorten without jwt %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("redirect missing %d", rec.Code)
	}
}
