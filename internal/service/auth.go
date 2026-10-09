package service

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/lalith/urlshortener/internal/model"
	"github.com/lalith/urlshortener/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

// Auth handles registration, login, and JWT issue/parse.
type Auth struct {
	Users  repository.UserStore
	Secret []byte
	Expiry time.Duration
	Cost   int
}

func (a *Auth) Register(ctx context.Context, email, password string) (model.User, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return model.User{}, err
	}
	if err := validatePassword(password); err != nil {
		return model.User{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), a.Cost)
	if err != nil {
		return model.User{}, err
	}

	user, err := a.Users.CreateUser(ctx, email, string(hash))
	if err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return model.User{}, ErrEmailTaken
		}
		return model.User{}, err
	}
	return user, nil
}

func (a *Auth) Login(ctx context.Context, email, password string) (string, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return "", ErrInvalidCredentials
	}
	if err := validatePassword(password); err != nil {
		return "", ErrInvalidCredentials
	}

	user, err := a.Users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", ErrInvalidCredentials
		}
		return "", err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	return a.IssueToken(user.ID)
}

func (a *Auth) IssueToken(userID uuid.UUID) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(a.Expiry)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(a.Secret)
}

// ParseToken returns the user id from a signed JWT.
// jwt.WithValidMethods blocks the "alg: none" attack.
func (a *Auth) ParseToken(token string) (uuid.UUID, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	parsed, err := parser.ParseWithClaims(token, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		return a.Secret, nil
	})
	if err != nil || !parsed.Valid {
		return uuid.Nil, ErrUnauthorized
	}
	claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
	if !ok || claims.Subject == "" {
		return uuid.Nil, ErrUnauthorized
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, ErrUnauthorized
	}
	return id, nil
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || utf8.RuneCountInString(email) > 255 {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", ErrInvalidEmail
	}
	return email, nil
}

func validatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	// bcrypt silently truncates after 72 bytes; reject instead of hashing a prefix.
	if n < 8 || n > 72 || len(password) > 72 {
		return ErrInvalidPassword
	}
	return nil
}
