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
	"github.com/lalith/urlshortener/internal/middleware"
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
func (s *linkStub) ListByUser(_ context.Context, userID uuid.UUID, _, _ int) ([]model.Link, int, error) {
	var all []model.Link
	for _, l := range s.byCode {
		if l.UserID == userID {
			all = append(all, l)
		}
	}
	return all, len(all), nil
}
func (s *linkStub) DeleteByCode(_ context.Context, userID uuid.UUID, code string) error {
	l, ok := s.byCode[code]
	if !ok || l.UserID != userID {
		return repository.ErrNotFound
	}
	delete(s.byCode, code)
	return nil
}
func (s *linkStub) AddClicks(context.Context, []model.ClickDelta) error { return nil }

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

func TestListAndStatsAndDelete(t *testing.T) {
	owner := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h := Links{Links: &service.Links{
		Store: &linkStub{byCode: map[string]model.Link{
			"abc": {UserID: owner, Code: "abc", OriginalURL: "https://example.com", CreatedAt: now, ClickCount: 4},
		}},
		Cache: &cacheStub{data: map[string]model.CachedLink{}},
		TTL:   time.Hour,
	}}

	r := chi.NewRouter()
	r.Get("/api/v1/links", h.List)
	r.Get("/api/v1/links/{code}/stats", h.Stats)
	r.Delete("/api/v1/links/{code}", h.Delete)

	withUser := func(req *http.Request) *http.Request {
		return req.WithContext(middleware.WithUserID(req.Context(), owner))
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/links/abc/stats", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withUser(req))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"clicks":4`) {
		t.Fatalf("stats status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/links?page=1&page_size=20", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, withUser(req))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/links/abc", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, withUser(req))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", rec.Code)
	}
}
