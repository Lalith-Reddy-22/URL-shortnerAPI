package service

import (
	"net/url"
	"strings"
	"unicode/utf8"
)

const maxURLLen = 2048

func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || utf8.RuneCountInString(raw) > maxURLLen {
		return "", ErrInvalidURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", ErrInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", ErrInvalidURL
	}
	return u.String(), nil
}
