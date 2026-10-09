package repository

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapPostgresError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   error
		want error
	}{
		{name: "nil", in: nil, want: nil},
		{name: "no rows", in: pgx.ErrNoRows, want: ErrNotFound},
		{name: "unique violation", in: &pgconn.PgError{Code: "23505"}, want: ErrDuplicate},
		{name: "other", in: errors.New("boom"), want: errors.New("boom")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mapPostgresError(tt.in)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			if tt.name == "other" {
				if got.Error() != tt.want.Error() {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
				return
			}
			if !errors.Is(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
