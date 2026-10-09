package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lalith/urlshortener/internal/model"
	"github.com/lalith/urlshortener/internal/repository"
	"github.com/lalith/urlshortener/internal/service"
	"golang.org/x/crypto/bcrypt"
)

type userMem struct {
	users map[string]model.User
}

func (m *userMem) CreateUser(_ context.Context, email, passwordHash string) (model.User, error) {
	u := model.User{ID: uuid.New(), Email: email, PasswordHash: passwordHash, CreatedAt: time.Now()}
	m.users[email] = u
	return u, nil
}
func (m *userMem) GetByEmail(_ context.Context, email string) (model.User, error) {
	u, ok := m.users[email]
	if !ok {
		return model.User{}, repository.ErrNotFound
	}
	return u, nil
}
func (m *userMem) GetByID(context.Context, uuid.UUID) (model.User, error) {
	return model.User{}, nil
}

func testAuthHandler() Auth {
	return Auth{Auth: &service.Auth{
		Users:  &userMem{users: map[string]model.User{}},
		Secret: []byte("test-secret"),
		Expiry: time.Hour,
		Cost:   bcrypt.MinCost,
	}}
}

func TestRegisterAndLogin(t *testing.T) {
	h := testAuthHandler()

	reg := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"a@b.com","password":"password1"}`))
	reg.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Register(rec, reg)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status = %d body = %s", rec.Code, rec.Body.String())
	}

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"a@b.com","password":"password1"}`))
	login.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.Login(rec, login)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"token"`) {
		t.Fatalf("login body = %s", rec.Body.String())
	}
}

func TestRegisterBadJSON(t *testing.T) {
	h := testAuthHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{`))
	rec := httptest.NewRecorder()
	h.Register(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}
