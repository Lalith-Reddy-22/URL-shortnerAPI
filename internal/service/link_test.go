package service

import (
	"context"
	"errors"
	"sort"
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
func (m *linkMem) ListByUser(_ context.Context, userID uuid.UUID, limit, offset int) ([]model.Link, int, error) {
	var all []model.Link
	for _, l := range m.byCode {
		if l.UserID == userID {
			all = append(all, l)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt.After(all[j].CreatedAt) })
	total := len(all)
	if offset > total {
		return []model.Link{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}
func (m *linkMem) DeleteByCode(_ context.Context, userID uuid.UUID, code string) error {
	l, ok := m.byCode[code]
	if !ok || l.UserID != userID {
		return repository.ErrNotFound
	}
	delete(m.byCode, code)
	return nil
}
func (m *linkMem) AddClicks(_ context.Context, deltas []model.ClickDelta) error {
	for _, d := range deltas {
		l, ok := m.byCode[d.Code]
		if !ok {
			continue
		}
		l.ClickCount += d.Count
		at := d.LastClick
		l.LastClickedAt = &at
		m.byCode[d.Code] = l
	}
	return nil
}

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

func TestStatsListDeleteOwner(t *testing.T) {
	owner := uuid.New()
	other := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := &linkMem{byCode: map[string]model.Link{
		"a": {UserID: owner, Code: "a", OriginalURL: "https://a.example", CreatedAt: now.Add(time.Minute), ClickCount: 3},
		"b": {UserID: owner, Code: "b", OriginalURL: "https://b.example", CreatedAt: now},
		"c": {UserID: other, Code: "c", OriginalURL: "https://c.example", CreatedAt: now},
	}}
	cache := &cacheMem{data: map[string]model.CachedLink{"a": {OriginalURL: "https://a.example"}}}
	svc := &Links{Store: store, Cache: cache, TTL: time.Hour}

	got, err := svc.Stats(context.Background(), owner, "a")
	if err != nil || got.ClickCount != 3 {
		t.Fatalf("stats = %+v err=%v", got, err)
	}
	if _, err := svc.Stats(context.Background(), other, "a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other stats err = %v", err)
	}

	list, err := svc.List(context.Background(), owner, 1, 1)
	if err != nil || list.Total != 2 || len(list.Items) != 1 || list.Items[0].Code != "a" {
		t.Fatalf("list = %+v err=%v", list, err)
	}

	if err := svc.Delete(context.Background(), owner, "a"); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.data["a"]; ok {
		t.Fatal("expected cache key removed")
	}
	if _, err := svc.Stats(context.Background(), owner, "a"); !errors.Is(err, ErrLinkNotFound) {
		t.Fatalf("deleted stats err = %v", err)
	}
}

func TestValidateAliasAndCacheTTLAndListClamp(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(30 * time.Minute)
	svc := &Links{
		Store: &linkMem{byCode: map[string]model.Link{}},
		Cache: &cacheMem{data: map[string]model.CachedLink{}},
		TTL:   time.Hour,
		Now:   func() time.Time { return now },
	}

	if err := validateAlias("healthz"); !errors.Is(err, ErrInvalidAlias) {
		t.Fatalf("reserved err=%v", err)
	}
	if err := validateAlias("ok"); err != nil {
		t.Fatal(err)
	}
	if got := svc.cacheTTL(&future); got != 30*time.Minute {
		t.Fatalf("ttl=%s", got)
	}
	if got := svc.cacheTTL(nil); got != time.Hour {
		t.Fatalf("default ttl=%s", got)
	}

	list, err := svc.List(context.Background(), uuid.New(), 0, 0)
	if err != nil || list.Page != 1 || list.PageSize != 20 {
		t.Fatalf("clamp %+v err=%v", list, err)
	}
	list, err = svc.List(context.Background(), uuid.New(), 1, 500)
	if err != nil || list.PageSize != 100 {
		t.Fatalf("max page size %+v", list)
	}

	clicks := NewClickCounter(svc.Store, 0, 0, time.Hour, time.Second)
	svc.Clicks = clicks
	svc.recordClick("x")

	_, err = svc.Shorten(context.Background(), ShortenInput{UserID: uuid.New(), URL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
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
