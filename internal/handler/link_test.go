package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lalith/urlshortener/internal/model"
	"github.com/lalith/urlshortener/internal/repository"
	"github.com/lalith/urlshortener/internal/service"
)

type linkStub struct {
	byCode map[string]model.Link
}

func (s *linkStub) CreateLink(_ context.Context, link model.Link) (model.Link, error) {
	if _, ok := s.byCode[link.Code]; ok {
		return model.Link{}, repository.ErrDuplicate
	}
	link.ID = uuid.New()
	s.byCode[link.Code] = link
	return link, nil
}
func (s *linkStub) GetByCode(_ context.Context, code string) (model.Link, error) {
	l, ok := s.byCode[code]
	if !ok {
		return model.Link{}, repository.ErrNotFound
	}
	return l, nil
}
func (s *linkStub) ListByUser(context.Context, uuid.UUID, int, int) ([]model.Link, int, error) {
	return nil, 0, nil
}
func (s *linkStub) DeleteByCode(context.Context, uuid.UUID, string) error { return nil }
func (s *linkStub) AddClicks(context.Context, []model.ClickDelta) error   { return nil }

type cacheStub struct {
	data map[string]model.CachedLink
}

func (c *cacheStub) GetLink(_ context.Context, code string) (model.CachedLink, error) {
	v, ok := c.data[code]
	if !ok {
		return model.CachedLink{}, repository.ErrNotFound
	}
	return v, nil
}
func (c *cacheStub) SetLink(_ context.Context, code string, link model.CachedLink, _ time.Duration) error {
	c.data[code] = link
	return nil
}
func (c *cacheStub) DeleteLink(_ context.Context, code string) error {
	delete(c.data, code)
	return nil
}

func TestShortenRequiresUser(t *testing.T) {
	h := Links{Links: &service.Links{
		Store: &linkStub{byCode: map[string]model.Link{}},
		Cache: &cacheStub{data: map[string]model.CachedLink{}},
		TTL:   time.Hour,
		GenerateCode: func() (string, error) {
			return "abcdefg", nil
		},
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", strings.NewReader(`{"url":"https://example.com"}`))
	rec := httptest.NewRecorder()
	h.Shorten(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRedirectNotFoundAndFound(t *testing.T) {
	h := Links{Links: &service.Links{
		Store: &linkStub{byCode: map[string]model.Link{
			"abcdefg": {Code: "abcdefg", OriginalURL: "https://example.com/x"},
		}},
		Cache: &cacheStub{data: map[string]model.CachedLink{}},
		TTL:   time.Hour,
	}}

	r := chi.NewRouter()
	r.Get("/{code}", h.Redirect)

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/abcdefg", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("found status = %d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "https://example.com/x" {
		t.Fatalf("Location = %s", loc)
	}
}
