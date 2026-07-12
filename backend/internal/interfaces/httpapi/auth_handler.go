package httpapi

import (
	"net/http"

	authapp "github.com/Fleetdock/fleetdock/backend/internal/app/auth"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
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

type grantDTO struct {
	Permission string `json:"permission"`
	ScopeType  string `json:"scope_type"`
	ScopeID    string `json:"scope_id,omitempty"`
}

type meResponse struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Permissions []string   `json:"permissions"`
	Grants      []grantDTO `json:"grants"`
}

func toGrantDTOs(grants []authz.Grant) []grantDTO {
	out := make([]grantDTO, 0, len(grants))
	for _, g := range grants {
		d := grantDTO{Permission: g.Permission, ScopeType: string(g.Scope.Type)}
		if g.Scope.Type != authz.ScopeGlobal {
			d.ScopeID = g.Scope.ID.String()
		}
		out = append(out, d)
	}
	return out
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
		Grants:      toGrantDTOs(p.Grants()),
	})
}
