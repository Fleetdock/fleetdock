package httpapi

import (
	"net/http"

	authapp "github.com/mariadb-cp/db-manager/backend/internal/app/auth"
)

// AuthHandler exposes login and identity endpoints.
type AuthHandler struct {
	svc *authapp.Service
}

// NewAuthHandler builds the auth handler.
func NewAuthHandler(svc *authapp.Service) *AuthHandler { return &AuthHandler{svc: svc} }

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userSummary struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type loginResponse struct {
	Token string      `json:"token"`
	User  userSummary `json:"user"`
}

type meResponse struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	Permissions []string `json:"permissions"`
}

// Login handles POST /v1/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	res, err := h.svc.Authenticate(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{
		Token: res.Token,
		User:  userSummary{ID: res.User.ID.String(), Email: res.User.Email},
	})
}

// Me handles GET /v1/auth/me.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	writeJSON(w, http.StatusOK, meResponse{
		ID:          p.UserID.String(),
		Email:       p.Email,
		Permissions: p.Permissions(),
	})
}
