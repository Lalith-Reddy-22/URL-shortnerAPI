package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/lalith/urlshortener/internal/model"
)

// UserStore is the persistence API for users.
// Handlers/services depend on this interface so tests can mock it.
type UserStore interface {
	CreateUser(ctx context.Context, email, passwordHash string) (model.User, error)
	GetByEmail(ctx context.Context, email string) (model.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (model.User, error)
}

// LinkStore is the persistence API for shortened links.
type LinkStore interface {
	CreateLink(ctx context.Context, link model.Link) (model.Link, error)
	GetByCode(ctx context.Context, code string) (model.Link, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) (links []model.Link, total int, err error)
	DeleteByCode(ctx context.Context, userID uuid.UUID, code string) error
	AddClicks(ctx context.Context, deltas []model.ClickDelta) error
}

// CacheStore is the Redis cache-aside API for the redirect hot path.
type CacheStore interface {
	GetLink(ctx context.Context, code string) (model.CachedLink, error)
	SetLink(ctx context.Context, code string, link model.CachedLink, ttl time.Duration) error
	DeleteLink(ctx context.Context, code string) error
}

// Pinger is implemented by both Postgres and Redis so /readyz stays generic.
type Pinger interface {
	Ping(ctx context.Context) error
}
