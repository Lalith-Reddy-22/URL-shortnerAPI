package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lalith/urlshortener/internal/middleware"
	"github.com/lalith/urlshortener/internal/model"
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

func (h Links) Stats(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	link, err := h.Links.Stats(r.Context(), userID, chi.URLParam(r, "code"))
	if err != nil {
		writeLinkErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code":         link.Code,
		"original_url": link.OriginalURL,
		"clicks":       link.ClickCount,
		"created_at":   link.CreatedAt,
		"last_clicked": link.LastClickedAt,
		"expires_at":   link.ExpiresAt,
	})
}

func (h Links) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	page := queryInt(r, "page", 1)
	pageSize := queryInt(r, "page_size", 20)
	out, err := h.Links.List(r.Context(), userID, page, pageSize)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	items := make([]map[string]any, 0, len(out.Items))
	for _, link := range out.Items {
		items = append(items, linkJSON(link))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"page":      out.Page,
		"page_size": out.PageSize,
		"total":     out.Total,
	})
}

func (h Links) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.Links.Delete(r.Context(), userID, chi.URLParam(r, "code")); err != nil {
		writeLinkErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeLinkErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrLinkNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func linkJSON(link model.Link) map[string]any {
	return map[string]any{
		"code":         link.Code,
		"original_url": link.OriginalURL,
		"clicks":       link.ClickCount,
		"created_at":   link.CreatedAt,
		"last_clicked": link.LastClickedAt,
		"expires_at":   link.ExpiresAt,
	}
}
