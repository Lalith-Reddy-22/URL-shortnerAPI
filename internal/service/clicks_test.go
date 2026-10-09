package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lalith/urlshortener/internal/model"
)

type clickSink struct {
	mu     sync.Mutex
	deltas []model.ClickDelta
	got    chan struct{}
}

func (s *clickSink) CreateLink(context.Context, model.Link) (model.Link, error) {
	return model.Link{}, nil
}
func (s *clickSink) GetByCode(context.Context, string) (model.Link, error) {
	return model.Link{}, nil
}
func (s *clickSink) ListByUser(context.Context, uuid.UUID, int, int) ([]model.Link, int, error) {
	return nil, 0, nil
}
func (s *clickSink) DeleteByCode(context.Context, uuid.UUID, string) error { return nil }
func (s *clickSink) AddClicks(_ context.Context, deltas []model.ClickDelta) error {
	s.mu.Lock()
	s.deltas = append(s.deltas, deltas...)
	s.mu.Unlock()
	select {
	case s.got <- struct{}{}:
	default:
	}
	return nil
}

func TestClickCounterBatches(t *testing.T) {
	sink := &clickSink{got: make(chan struct{}, 1)}
	c := NewClickCounter(sink, 8, 2, time.Hour, time.Second)
	c.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()

	c.Record("abc")
	c.Record("abc")

	select {
	case <-sink.got:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for flush")
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.deltas) != 1 || sink.deltas[0].Code != "abc" || sink.deltas[0].Count != 2 {
		t.Fatalf("deltas = %+v", sink.deltas)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop")
	}
}

func TestClickCounterRecordDoesNotBlock(t *testing.T) {
	sink := &clickSink{got: make(chan struct{}, 1)}
	c := NewClickCounter(sink, 1, 10, time.Hour, time.Second)
	c.events <- clickEvent{Code: "x", At: time.Now()}
	start := time.Now()
	c.Record("y")
	if time.Since(start) > 50*time.Millisecond {
		t.Fatal("Record blocked")
	}
}
