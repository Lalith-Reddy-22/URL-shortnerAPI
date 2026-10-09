package model

import (
	"time"

	"github.com/google/uuid"
)

// Link is a shortened URL owned by a user.
type Link struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Code          string
	OriginalURL   string
	ExpiresAt     *time.Time
	ClickCount    int64
	LastClickedAt *time.Time
	CreatedAt     time.Time
}

// Expired reports whether the link is past expires_at.
// A nil ExpiresAt means it never expires.
func (l Link) Expired(now time.Time) bool {
	return l.ExpiresAt != nil && !l.ExpiresAt.After(now)
}

// CachedLink is the Redis value for cache-aside on the redirect path.
// Keep it small: the hot path only needs the destination and expiry.
type CachedLink struct {
	OriginalURL string     `json:"original_url"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

func (c CachedLink) Expired(now time.Time) bool {
	return c.ExpiresAt != nil && !c.ExpiresAt.After(now)
}

// ClickDelta is one batched increment flushed by the async click worker.
type ClickDelta struct {
	Code      string
	Count     int64
	LastClick time.Time
}
