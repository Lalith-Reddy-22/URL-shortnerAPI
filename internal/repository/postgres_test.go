package repository

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lalith/urlshortener/internal/model"
)

type stubRow struct {
	vals []any
	err  error
}

func (r stubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return assign(dest, r.vals)
}

type stubRows struct {
	data [][]any
	i    int
	err  error
}

func (r *stubRows) Close()                                       {}
func (r *stubRows) Err() error                                   { return r.err }
func (r *stubRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *stubRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *stubRows) Next() bool {
	if r.i >= len(r.data) {
		return false
	}
	r.i++
	return true
}
func (r *stubRows) Scan(dest ...any) error { return assign(dest, r.data[r.i-1]) }
func (r *stubRows) Values() ([]any, error) { return r.data[r.i-1], nil }
func (r *stubRows) RawValues() [][]byte    { return nil }
func (r *stubRows) Conn() *pgx.Conn        { return nil }
func (r *stubRows) TypeMap() *pgtype.Map   { return nil }

type step struct {
	row  stubRow
	rows *stubRows
	tag  pgconn.CommandTag
	err  error
}

type scriptedPool struct {
	steps []step
}

func (p *scriptedPool) next() step {
	s := p.steps[0]
	p.steps = p.steps[1:]
	return s
}
func (p *scriptedPool) Ping(context.Context) error { return nil }
func (p *scriptedPool) Close()                     {}
func (p *scriptedPool) QueryRow(context.Context, string, ...any) pgx.Row {
	s := p.next()
	if s.err != nil {
		return stubRow{err: s.err}
	}
	return s.row
}
func (p *scriptedPool) Query(context.Context, string, ...any) (pgx.Rows, error) {
	s := p.next()
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}
func (p *scriptedPool) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	s := p.next()
	return s.tag, s.err
}

func assign(dest, src []any) error {
	for i := range dest {
		if dest[i] == nil {
			continue
		}
		dv := reflect.ValueOf(dest[i]).Elem()
		if src[i] == nil {
			dv.Set(reflect.Zero(dv.Type()))
			continue
		}
		sv := reflect.ValueOf(src[i])
		if sv.Type().AssignableTo(dv.Type()) {
			dv.Set(sv)
			continue
		}
		if dv.Kind() == reflect.Ptr {
			ptr := reflect.New(dv.Type().Elem())
			ptr.Elem().Set(sv)
			dv.Set(ptr)
		}
	}
	return nil
}

func TestPostgresCRUD(t *testing.T) {
	id := uuid.New()
	now := time.Now().UTC()
	userVals := []any{id, "a@b.com", "hash", now}
	linkVals := []any{id, id, "abc", "https://example.com", nil, int64(2), nil, now}

	pool := &scriptedPool{steps: []step{
		{row: stubRow{vals: userVals}},
		{row: stubRow{vals: userVals}},
		{row: stubRow{vals: userVals}},
		{row: stubRow{vals: linkVals}},
		{row: stubRow{vals: linkVals}},
		{row: stubRow{vals: []any{1}}},
		{rows: &stubRows{data: [][]any{linkVals}}},
		{tag: pgconn.NewCommandTag("DELETE 1")},
		{tag: pgconn.NewCommandTag("DELETE 0")},
		{tag: pgconn.NewCommandTag("UPDATE 1")},
		{row: stubRow{err: &pgconn.PgError{Code: "23505"}}},
		{row: stubRow{err: pgx.ErrNoRows}},
	}}
	p := &Postgres{pool: pool}
	ctx := context.Background()

	if err := p.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	p.Close()

	if _, err := p.CreateUser(ctx, "a@b.com", "hash"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.GetByEmail(ctx, "a@b.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.GetByID(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := p.CreateLink(ctx, model.Link{UserID: id, Code: "abc", OriginalURL: "https://example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.GetByCode(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	items, total, err := p.ListByUser(ctx, id, 20, 0)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("list total=%d n=%d err=%v", total, len(items), err)
	}
	if err := p.DeleteByCode(ctx, id, "abc"); err != nil {
		t.Fatal(err)
	}
	if err := p.DeleteByCode(ctx, id, "missing"); err != ErrNotFound {
		t.Fatalf("delete missing %v", err)
	}
	if err := p.AddClicks(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := p.AddClicks(ctx, []model.ClickDelta{{Code: "abc", Count: 1, LastClick: now}}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.CreateUser(ctx, "a@b.com", "hash"); err != ErrDuplicate {
		t.Fatalf("dup %v", err)
	}
	if _, err := p.GetByEmail(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("missing %v", err)
	}
}
