package httpapi

import (
	"errors"
	"net/http"
)

type magicLinkResponse struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	DisplayName string  `json:"display_name"`
	CreatedAt   string  `json:"created_at"`
	ExpiresAt   string  `json:"expires_at"`
	UsedAt      *string `json:"used_at,omitempty"`
}

func (s *Server) requestMagicLink(w http.ResponseWriter, r *http.Request) {
	if s.disableDevAuth {
		writeError(w, http.StatusNotImplemented, errors.New("magic-link delivery is not configured"))
		return
	}
	var body struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	link, err := s.store.CreateMagicLink(r.Context(), body.Email, body.DisplayName)
	response := map[string]any{"magic_link": magicLinkResponse{
		ID:          link.ID,
		Email:       link.Email,
		DisplayName: link.DisplayName,
		CreatedAt:   link.CreatedAt,
		ExpiresAt:   link.ExpiresAt,
		UsedAt:      link.UsedAt,
	}}
	response["token"] = link.Token
	writeResultStatus(w, http.StatusCreated, response, err)
}

func (s *Server) consumeMagicLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	user, session, err := s.store.ConsumeMagicLink(r.Context(), body.Token)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.setSessionCookie(w, r, session)
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "session": session, "token": session.Token})
}
