package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/lalith/urlshortener/internal/model"
)

func TestRedisCacheAndIncrement(t *testing.T) {
	mr := miniredis.RunT(t)
	r := NewRedis(mr.Addr(), "", 0)
	t.Cleanup(func() { _ = r.Close() })

	ctx := context.Background()
	if err := r.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := r.GetLink(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("missing err = %v", err)
	}

	exp := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	want := model.CachedLink{OriginalURL: "https://example.com", ExpiresAt: &exp}
	if err := r.SetLink(ctx, "abc", want, time.Minute); err != nil {
		t.Fatal(err)
	}
	got, err := r.GetLink(ctx, "abc")
	if err != nil || got.OriginalURL != want.OriginalURL {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if err := r.DeleteLink(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetLink(ctx, "abc"); err != ErrNotFound {
		t.Fatalf("after delete err = %v", err)
	}

	n, err := r.Increment(ctx, "rl:1.2.3.4:1", time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("incr 1: n=%d err=%v", n, err)
	}
	n, err = r.Increment(ctx, "rl:1.2.3.4:1", time.Minute)
	if err != nil || n != 2 {
		t.Fatalf("incr 2: n=%d err=%v", n, err)
	}

	mr.Set("link:bad", "{")
	if _, err := r.GetLink(ctx, "bad"); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestNewPostgresInvalid(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := NewPostgres(ctx, "postgres://user:pass@127.0.0.1:1/db?connect_timeout=1")
	if err == nil {
		t.Fatal("expected error")
	}
}
