package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lalith/urlshortener/internal/model"
)

// pgxPool is the subset of *pgxpool.Pool the repo uses.
// Tests inject a mock; production uses a real pool.
type pgxPool interface {
	Ping(ctx context.Context) error
	Close()
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Postgres implements UserStore and LinkStore with pgxpool.
type Postgres struct {
	pool pgxPool
}

func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *Postgres) Close() {
	p.pool.Close()
}

func (p *Postgres) CreateUser(ctx context.Context, email, passwordHash string) (model.User, error) {
	const q = `
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email, password_hash, created_at`

	var u model.User
	err := p.pool.QueryRow(ctx, q, email, passwordHash).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return model.User{}, mapPostgresError(err)
	}
	return u, nil
}

func (p *Postgres) GetByEmail(ctx context.Context, email string) (model.User, error) {
	const q = `SELECT id, email, password_hash, created_at FROM users WHERE email = $1`
	return scanUser(p.pool.QueryRow(ctx, q, email))
}

func (p *Postgres) GetByID(ctx context.Context, id uuid.UUID) (model.User, error) {
	const q = `SELECT id, email, password_hash, created_at FROM users WHERE id = $1`
	return scanUser(p.pool.QueryRow(ctx, q, id))
}

func (p *Postgres) CreateLink(ctx context.Context, link model.Link) (model.Link, error) {
	const q = `
		INSERT INTO links (user_id, code, original_url, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, code, original_url, expires_at, click_count, last_clicked_at, created_at`

	out, err := scanLink(p.pool.QueryRow(ctx, q, link.UserID, link.Code, link.OriginalURL, link.ExpiresAt))
	if err != nil {
		return model.Link{}, mapPostgresError(err)
	}
	return out, nil
}

func (p *Postgres) GetByCode(ctx context.Context, code string) (model.Link, error) {
	const q = `
		SELECT id, user_id, code, original_url, expires_at, click_count, last_clicked_at, created_at
		FROM links WHERE code = $1`
	link, err := scanLink(p.pool.QueryRow(ctx, q, code))
	if err != nil {
		return model.Link{}, mapPostgresError(err)
	}
	return link, nil
}

func (p *Postgres) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]model.Link, int, error) {
	const countQ = `SELECT COUNT(*) FROM links WHERE user_id = $1`
	var total int
	if err := p.pool.QueryRow(ctx, countQ, userID).Scan(&total); err != nil {
		return nil, 0, mapPostgresError(err)
	}

	const q = `
		SELECT id, user_id, code, original_url, expires_at, click_count, last_clicked_at, created_at
		FROM links
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := p.pool.Query(ctx, q, userID, limit, offset)
	if err != nil {
		return nil, 0, mapPostgresError(err)
	}
	defer rows.Close()

	links := make([]model.Link, 0)
	for rows.Next() {
		link, err := scanLink(rows)
		if err != nil {
			return nil, 0, mapPostgresError(err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, mapPostgresError(err)
	}
	return links, total, nil
}

func (p *Postgres) DeleteByCode(ctx context.Context, userID uuid.UUID, code string) error {
	const q = `DELETE FROM links WHERE user_id = $1 AND code = $2`
	tag, err := p.pool.Exec(ctx, q, userID, code)
	if err != nil {
		return mapPostgresError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AddClicks applies batched increments in one statement.
// unnest lets the worker flush many codes without a round-trip per click.
func (p *Postgres) AddClicks(ctx context.Context, deltas []model.ClickDelta) error {
	if len(deltas) == 0 {
		return nil
	}
	codes := make([]string, len(deltas))
	counts := make([]int64, len(deltas))
	lastClicks := make([]time.Time, len(deltas))
	for i, d := range deltas {
		codes[i] = d.Code
		counts[i] = d.Count
		lastClicks[i] = d.LastClick
	}

	const q = `
		UPDATE links AS l
		SET click_count = l.click_count + d.count,
		    last_clicked_at = GREATEST(COALESCE(l.last_clicked_at, d.last_click), d.last_click)
		FROM unnest($1::text[], $2::bigint[], $3::timestamptz[]) AS d(code, count, last_click)
		WHERE l.code = d.code`

	_, err := p.pool.Exec(ctx, q, codes, counts, lastClicks)
	return mapPostgresError(err)
}

func scanUser(row pgx.Row) (model.User, error) {
	var u model.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return model.User{}, mapPostgresError(err)
	}
	return u, nil
}

func scanLink(row interface {
	Scan(dest ...any) error
}) (model.Link, error) {
	var l model.Link
	err := row.Scan(
		&l.ID,
		&l.UserID,
		&l.Code,
		&l.OriginalURL,
		&l.ExpiresAt,
		&l.ClickCount,
		&l.LastClickedAt,
		&l.CreatedAt,
	)
	return l, err
}

func mapPostgresError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicate
	}
	return err
}

var _ UserStore = (*Postgres)(nil)
var _ LinkStore = (*Postgres)(nil)
var _ Pinger = (*Postgres)(nil)
