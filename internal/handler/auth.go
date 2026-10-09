package handler

import (
	"errors"
	"net/http"

	"github.com/lalith/urlshortener/internal/service"
)

type Auth struct {
	Auth *service.Auth
}

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h Auth) Register(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	user, err := h.Auth.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidEmail), errors.Is(err, service.ErrInvalidPassword):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, service.ErrEmailTaken):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":    user.ID.String(),
		"email": user.Email,
	})
}

func (h Auth) Login(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	token, err := h.Auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}
