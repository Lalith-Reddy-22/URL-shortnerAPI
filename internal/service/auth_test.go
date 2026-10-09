package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lalith/urlshortener/internal/model"
	"github.com/lalith/urlshortener/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type userStub struct {
	createFn func(ctx context.Context, email, hash string) (model.User, error)
	getFn    func(ctx context.Context, email string) (model.User, error)
}

func (s userStub) CreateUser(ctx context.Context, email, passwordHash string) (model.User, error) {
	return s.createFn(ctx, email, passwordHash)
}
func (s userStub) GetByEmail(ctx context.Context, email string) (model.User, error) {
	return s.getFn(ctx, email)
}
func (s userStub) GetByID(context.Context, uuid.UUID) (model.User, error) {
	return model.User{}, errors.New("unused")
}

func newTestAuth(users repository.UserStore) *Auth {
	return &Auth{
		Users:  users,
		Secret: []byte("test-secret"),
		Expiry: time.Hour,
		Cost:   bcrypt.MinCost,
	}
}

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    string
		wantErr error
	}{
		{in: "  A@B.com ", want: "a@b.com"},
		{in: "not-an-email", wantErr: ErrInvalidEmail},
		{in: "", wantErr: ErrInvalidEmail},
		{in: "Name <a@b.com>", wantErr: ErrInvalidEmail},
	}
	for _, tt := range tests {
		got, err := normalizeEmail(tt.in)
		if tt.wantErr != nil {
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("normalizeEmail(%q) err = %v, want %v", tt.in, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Fatalf("normalizeEmail(%q) = %q, %v; want %q, nil", tt.in, got, err, tt.want)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()
	if err := validatePassword("short"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("short password: %v", err)
	}
	if err := validatePassword("long-enough"); err != nil {
		t.Fatalf("ok password: %v", err)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	auth := newTestAuth(userStub{
		createFn: func(context.Context, string, string) (model.User, error) {
			return model.User{}, repository.ErrDuplicate
		},
	})
	_, err := auth.Register(context.Background(), "a@b.com", "password1")
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
}

func TestLoginUnknownUser(t *testing.T) {
	auth := newTestAuth(userStub{
		getFn: func(context.Context, string) (model.User, error) {
			return model.User{}, repository.ErrNotFound
		},
	})
	_, err := auth.Login(context.Background(), "missing@b.com", "password1")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password1"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	auth := newTestAuth(userStub{
		getFn: func(context.Context, string) (model.User, error) {
			return model.User{ID: id, Email: "a@b.com", PasswordHash: string(hash)}, nil
		},
	})
	_, err = auth.Login(context.Background(), "a@b.com", "wrongpass")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestIssueAndParseToken(t *testing.T) {
	auth := newTestAuth(userStub{})
	id := uuid.New()
	tok, err := auth.IssueToken(id)
	if err != nil {
		t.Fatal(err)
	}
	got, err := auth.ParseToken(tok)
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Fatalf("id = %s, want %s", got, id)
	}
}
