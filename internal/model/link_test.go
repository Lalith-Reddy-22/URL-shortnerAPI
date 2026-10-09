package model

import (
	"testing"
	"time"
)

func TestLinkExpired(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name string
		exp  *time.Time
		want bool
	}{
		{name: "never", exp: nil, want: false},
		{name: "past", exp: &past, want: true},
		{name: "future", exp: &future, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			link := Link{ExpiresAt: tt.exp}
			if got := link.Expired(now); got != tt.want {
				t.Fatalf("Link.Expired() = %v, want %v", got, tt.want)
			}
			cached := CachedLink{ExpiresAt: tt.exp}
			if got := cached.Expired(now); got != tt.want {
				t.Fatalf("CachedLink.Expired() = %v, want %v", got, tt.want)
			}
		})
	}
}
