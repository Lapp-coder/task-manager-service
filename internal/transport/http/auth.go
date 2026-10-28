package http

import (
	"net/http"

	"github.com/lapp-coder/task-manager-service/internal/service"
)

func (h *handler) register(w http.ResponseWriter, r *http.Request) error {
	var req registerRequest
	if err := decode(w, r, &req); err != nil {
		return err
	}
	u, err := h.auth.Register(r.Context(), service.Registration{Email: req.Email, Password: req.Password, Name: req.Name})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, userResponseOf(u))
	return nil
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) error {
	var req loginRequest
	if err := decode(w, r, &req); err != nil {
		return err
	}
	token, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, tokenResponse{AccessToken: token.Value, TokenType: "Bearer", ExpiresAt: token.ExpiresAt})
	return nil
}
