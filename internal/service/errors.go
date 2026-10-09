package service

import "errors"

var (
	ErrInvalidEmail       = errors.New("invalid email")
	ErrInvalidPassword    = errors.New("password must be 8-72 characters")
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrInvalidURL         = errors.New("url must be http or https and at most 2048 characters")
	ErrInvalidAlias       = errors.New("custom_alias must be 1-64 chars of [A-Za-z0-9_-]")
	ErrAliasTaken         = errors.New("custom_alias already in use")
	ErrExpiryInPast       = errors.New("expires_at must be in the future")
	ErrLinkExpired        = errors.New("link expired")
	ErrLinkNotFound       = errors.New("link not found")
	ErrCodeCollision      = errors.New("could not allocate a unique short code")
)
