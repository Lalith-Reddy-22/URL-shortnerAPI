package repository

import "errors"

var (
	// ErrNotFound is returned when a row (or cache key) does not exist.
	ErrNotFound = errors.New("not found")
	// ErrDuplicate is returned on unique-constraint violations (email or code).
	ErrDuplicate = errors.New("duplicate")
)
