package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/lalith/urlshortener/internal/model"
	"github.com/lalith/urlshortener/internal/repository"
)

const createRetries = 5

var aliasRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var reservedAliases = map[string]struct{}{
	"healthz": {},
	"readyz":  {},
	"api":     {},
}

// Links is the shorten + redirect use case.
type Links struct {
	Store        repository.LinkStore
	Cache        repository.CacheStore
	Clicks       *ClickCounter
	TTL          time.Duration
	Now          func() time.Time
	GenerateCode func() (string, error)
}

func (s *Links) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *Links) nextCode() (string, error) {
	if s.GenerateCode != nil {
		return s.GenerateCode()
	}
	return RandomCode(shortCodeLen)
}

type ShortenInput struct {
	UserID      uuid.UUID
	URL         string
	CustomAlias string
	ExpiresAt   *time.Time
}

func (s *Links) Shorten(ctx context.Context, in ShortenInput) (model.Link, error) {
	dest, err := normalizeURL(in.URL)
	if err != nil {
		return model.Link{}, err
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(s.now()) {
		return model.Link{}, ErrExpiryInPast
	}

	alias := strings.TrimSpace(in.CustomAlias)
	if alias != "" {
		if err := validateAlias(alias); err != nil {
			return model.Link{}, err
		}
		link, err := s.Store.CreateLink(ctx, model.Link{
			UserID:      in.UserID,
			Code:        alias,
			OriginalURL: dest,
			ExpiresAt:   in.ExpiresAt,
		})
		if errors.Is(err, repository.ErrDuplicate) {
			return model.Link{}, ErrAliasTaken
		}
		return link, err
	}

	// Generated codes: retry on unique_violation instead of a pre-check.
	// A pre-check TOCTOU-races; the unique index is the real source of truth.
	for i := 0; i < createRetries; i++ {
		code, err := s.nextCode()
		if err != nil {
			return model.Link{}, err
		}
		link, err := s.Store.CreateLink(ctx, model.Link{
			UserID:      in.UserID,
			Code:        code,
			OriginalURL: dest,
			ExpiresAt:   in.ExpiresAt,
		})
		if errors.Is(err, repository.ErrDuplicate) {
			continue
		}
		return link, err
	}
	return model.Link{}, ErrCodeCollision
}

// Resolve looks up the destination for GET /{code}.
// Cache-aside: Redis first, Postgres on miss, then fill the cache.
// Redis errors are treated as a miss so a cache outage does not take down redirects.
func (s *Links) Resolve(ctx context.Context, code string) (string, error) {
	if code == "" {
		return "", ErrLinkNotFound
	}

	cached, err := s.Cache.GetLink(ctx, code)
	if err == nil {
		if cached.Expired(s.now()) {
			_ = s.Cache.DeleteLink(ctx, code)
			return "", ErrLinkExpired
		}
		s.recordClick(code)
		return cached.OriginalURL, nil
	}

	link, err := s.Store.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", ErrLinkNotFound
		}
		return "", err
	}
	if link.Expired(s.now()) {
		return "", ErrLinkExpired
	}

	ttl := s.cacheTTL(link.ExpiresAt)
	_ = s.Cache.SetLink(ctx, code, model.CachedLink{
		OriginalURL: link.OriginalURL,
		ExpiresAt:   link.ExpiresAt,
	}, ttl)

	s.recordClick(code)
	return link.OriginalURL, nil
}

func (s *Links) recordClick(code string) {
	if s.Clicks != nil {
		s.Clicks.Record(code)
	}
}

func (s *Links) Stats(ctx context.Context, userID uuid.UUID, code string) (model.Link, error) {
	return s.ownedLink(ctx, userID, code)
}

type ListResult struct {
	Items    []model.Link
	Total    int
	Page     int
	PageSize int
}

func (s *Links) List(ctx context.Context, userID uuid.UUID, page, pageSize int) (ListResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	items, total, err := s.Store.ListByUser(ctx, userID, pageSize, offset)
	if err != nil {
		return ListResult{}, err
	}
	if items == nil {
		items = []model.Link{}
	}
	return ListResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *Links) Delete(ctx context.Context, userID uuid.UUID, code string) error {
	if _, err := s.ownedLink(ctx, userID, code); err != nil {
		return err
	}
	if err := s.Store.DeleteByCode(ctx, userID, code); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrLinkNotFound
		}
		return err
	}
	_ = s.Cache.DeleteLink(ctx, code)
	return nil
}

func (s *Links) ownedLink(ctx context.Context, userID uuid.UUID, code string) (model.Link, error) {
	link, err := s.Store.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return model.Link{}, ErrLinkNotFound
		}
		return model.Link{}, err
	}
	if link.UserID != userID {
		return model.Link{}, ErrForbidden
	}
	return link, nil
}

func (s *Links) cacheTTL(expiresAt *time.Time) time.Duration {
	ttl := s.TTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	if expiresAt == nil {
		return ttl
	}
	until := expiresAt.Sub(s.now())
	if until < ttl {
		return until
	}
	return ttl
}

func validateAlias(alias string) error {
	n := utf8.RuneCountInString(alias)
	if n < 1 || n > 64 || !aliasRe.MatchString(alias) {
		return ErrInvalidAlias
	}
	if _, reserved := reservedAliases[strings.ToLower(alias)]; reserved {
		return ErrInvalidAlias
	}
	return nil
}
