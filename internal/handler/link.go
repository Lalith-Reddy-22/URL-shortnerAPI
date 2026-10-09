package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lalith/urlshortener/internal/middleware"
	"github.com/lalith/urlshortener/internal/service"
)

type Links struct {
	Links *service.Links
}

type shortenRequest struct {
	URL         string     `json:"url"`
	CustomAlias string     `json:"custom_alias"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

func (h Links) Shorten(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req shortenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	link, err := h.Links.Shorten(r.Context(), service.ShortenInput{
		UserID:      userID,
		URL:         req.URL,
		CustomAlias: req.CustomAlias,
		ExpiresAt:   req.ExpiresAt,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidURL),
			errors.Is(err, service.ErrInvalidAlias),
			errors.Is(err, service.ErrExpiryInPast):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrAliasTaken):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"code":         link.Code,
		"original_url": link.OriginalURL,
		"expires_at":   link.ExpiresAt,
	})
}

func (h Links) Redirect(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	dest, err := h.Links.Resolve(r.Context(), code)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrLinkNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, service.ErrLinkExpired):
			writeError(w, http.StatusGone, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	http.Redirect(w, r, dest, http.StatusFound)
}
