package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/lalith/urlshortener/internal/model"
	"github.com/lalith/urlshortener/internal/repository"
)

type clickEvent struct {
	Code string
	At   time.Time
}

// ClickCounter records redirects without blocking the HTTP handler.
// Handlers send on a buffered channel; a single worker batches writes to Postgres.
type ClickCounter struct {
	events       chan clickEvent
	store        repository.LinkStore
	batchSize    int
	flushEvery   time.Duration
	writeTimeout time.Duration
	now          func() time.Time
}

func NewClickCounter(store repository.LinkStore, buffer, batch int, flushEvery, writeTimeout time.Duration) *ClickCounter {
	if buffer < 1 {
		buffer = 1
	}
	if batch < 1 {
		batch = 1
	}
	return &ClickCounter{
		events:       make(chan clickEvent, buffer),
		store:        store,
		batchSize:    batch,
		flushEvery:   flushEvery,
		writeTimeout: writeTimeout,
		now:          func() time.Time { return time.Now().UTC() },
	}
}

// Record never blocks. If the buffer is full we drop the event so a slow
// database cannot stall redirects.
func (c *ClickCounter) Record(code string) {
	if c == nil || code == "" {
		return
	}
	ev := clickEvent{Code: code, At: c.now()}
	select {
	case c.events <- ev:
	default:
	}
}

// Run flushes when the batch is full, on a ticker, and once more on shutdown.
func (c *ClickCounter) Run(ctx context.Context) {
	ticker := time.NewTicker(c.flushEvery)
	defer ticker.Stop()

	batch := make([]clickEvent, 0, c.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		c.write(batch)
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			drain := true
			for drain {
				select {
				case ev := <-c.events:
					batch = append(batch, ev)
				default:
					drain = false
				}
			}
			flush()
			return
		case ev := <-c.events:
			batch = append(batch, ev)
			if len(batch) >= c.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (c *ClickCounter) write(events []clickEvent) {
	byCode := make(map[string]model.ClickDelta, len(events))
	for _, ev := range events {
		d := byCode[ev.Code]
		d.Code = ev.Code
		d.Count++
		if ev.At.After(d.LastClick) {
			d.LastClick = ev.At
		}
		byCode[ev.Code] = d
	}
	deltas := make([]model.ClickDelta, 0, len(byCode))
	for _, d := range byCode {
		deltas = append(deltas, d)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.writeTimeout)
	defer cancel()
	if err := c.store.AddClicks(ctx, deltas); err != nil {
		slog.Error("click flush", "err", err, "codes", len(deltas))
	}
}
