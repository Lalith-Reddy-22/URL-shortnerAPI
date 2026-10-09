package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lalith/urlshortener/internal/model"
	"github.com/lalith/urlshortener/internal/repository"
)

type linkMem struct {
	byCode map[string]model.Link
	fail   error
}

func (m *linkMem) CreateLink(_ context.Context, link model.Link) (model.Link, error) {
	if m.fail != nil {
		return model.Link{}, m.fail
	}
	if _, exists := m.byCode[link.Code]; exists {
		return model.Link{}, repository.ErrDuplicate
	}
	link.ID = uuid.New()
	m.byCode[link.Code] = link
	return link, nil
}
func (m *linkMem) GetByCode(_ context.Context, code string) (model.Link, error) {
	l, ok := m.byCode[code]
	if !ok {
		return model.Link{}, repository.ErrNotFound
	}
	return l, nil
}
func (m *linkMem) ListByUser(context.Context, uuid.UUID, int, int) ([]model.Link, int, error) {
	return nil, 0, nil
}
func (m *linkMem) DeleteByCode(context.Context, uuid.UUID, string) error { return nil }
func (m *linkMem) AddClicks(context.Context, []model.ClickDelta) error   { return nil }

type cacheMem struct {
	data map[string]model.CachedLink
}

func (c *cacheMem) GetLink(_ context.Context, code string) (model.CachedLink, error) {
	v, ok := c.data[code]
	if !ok {
		return model.CachedLink{}, repository.ErrNotFound
	}
	return v, nil
}
func (c *cacheMem) SetLink(_ context.Context, code string, link model.CachedLink, _ time.Duration) error {
	c.data[code] = link
	return nil
}
func (c *cacheMem) DeleteLink(_ context.Context, code string) error {
	delete(c.data, code)
	return nil
}

func TestNormalizeURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		wantErr error
	}{
		{in: "https://example.com/x", wantErr: nil},
		{in: "http://example.com", wantErr: nil},
		{in: "ftp://example.com", wantErr: ErrInvalidURL},
		{in: "javascript:alert(1)", wantErr: ErrInvalidURL},
		{in: "not a url", wantErr: ErrInvalidURL},
		{in: "", wantErr: ErrInvalidURL},
	}
	for _, tt := range tests {
		_, err := normalizeURL(tt.in)
		if tt.wantErr == nil && err != nil {
			t.Fatalf("normalizeURL(%q) unexpected err %v", tt.in, err)
		}
		if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
			t.Fatalf("normalizeURL(%q) err = %v, want %v", tt.in, err, tt.wantErr)
		}
	}
}

func TestShortenCustomAliasAndCollisionRetry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &linkMem{byCode: map[string]model.Link{}}
	svc := &Links{
		Store: store,
		Cache: &cacheMem{data: map[string]model.CachedLink{}},
		TTL:   time.Hour,
		Now:   func() time.Time { return now },
	}

	user := uuid.New()
	got, err := svc.Shorten(context.Background(), ShortenInput{
		UserID: user, URL: "https://example.com", CustomAlias: "my-link",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != "my-link" {
		t.Fatalf("code = %s", got.Code)
	}

	_, err = svc.Shorten(context.Background(), ShortenInput{
		UserID: user, URL: "https://example.com/2", CustomAlias: "my-link",
	})
	if !errors.Is(err, ErrAliasTaken) {
		t.Fatalf("err = %v, want alias taken", err)
	}

	calls := 0
	svc.GenerateCode = func() (string, error) {
		calls++
		if calls == 1 {
			return "my-link", nil // already taken
		}
		return "abcdefg", nil
	}
	got, err = svc.Shorten(context.Background(), ShortenInput{UserID: user, URL: "https://example.com/3"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != "abcdefg" || calls != 2 {
		t.Fatalf("code=%s calls=%d", got.Code, calls)
	}
}

func TestResolveCacheAsideAndExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	store := &linkMem{byCode: map[string]model.Link{
		"live": {Code: "live", OriginalURL: "https://example.com/live"},
		"dead": {Code: "dead", OriginalURL: "https://example.com/dead", ExpiresAt: &past},
	}}
	cache := &cacheMem{data: map[string]model.CachedLink{}}
	svc := &Links{Store: store, Cache: cache, TTL: time.Hour, Now: func() time.Time { return now }}

	dest, err := svc.Resolve(context.Background(), "live")
	if err != nil || dest != "https://example.com/live" {
		t.Fatalf("miss path: dest=%s err=%v", dest, err)
	}
	if _, ok := cache.data["live"]; !ok {
		t.Fatal("expected cache fill after DB miss")
	}

	dest, err = svc.Resolve(context.Background(), "live")
	if err != nil || dest != "https://example.com/live" {
		t.Fatalf("hit path: dest=%s err=%v", dest, err)
	}

	_, err = svc.Resolve(context.Background(), "dead")
	if !errors.Is(err, ErrLinkExpired) {
		t.Fatalf("expired err = %v", err)
	}

	_, err = svc.Resolve(context.Background(), "missing")
	if !errors.Is(err, ErrLinkNotFound) {
		t.Fatalf("missing err = %v", err)
	}
}

func TestRandomCode(t *testing.T) {
	t.Parallel()
	code, err := RandomCode(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 7 {
		t.Fatalf("len = %d", len(code))
	}
	for _, c := range code {
		if !aliasRe.MatchString(string(c)) {
			t.Fatalf("unexpected rune %q", c)
		}
	}
}
