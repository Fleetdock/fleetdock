package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	auditapp "github.com/Fleetdock/fleetdock/backend/internal/app/audit"
	authapp "github.com/Fleetdock/fleetdock/backend/internal/app/auth"
	authz "github.com/Fleetdock/fleetdock/backend/internal/domain/authz"
	"github.com/Fleetdock/fleetdock/backend/internal/platform/apperr"
)

// AuthHandler exposes login and identity endpoints.
type AuthHandler struct {
	svc    *authapp.Service
	cookie SessionCookieConfig
	guard  *accountGuard
}

// NewAuthHandler builds the auth handler. Login sets the dashboard's session
// cookie as configured by cookie.
func NewAuthHandler(svc *authapp.Service, cookie SessionCookieConfig) *AuthHandler {
	// 10 failed sign-ins to one account within 15 minutes lock it for the
	// rest of those 15 minutes, whatever addresses they came from.
	return &AuthHandler{svc: svc, cookie: cookie, guard: newAccountGuard(10, 15*time.Minute)}
}

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
	Name        string     `json:"name"`
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
	if locked, wait := h.guard.Locked(req.Email); locked {
		auditapp.SetActor(r.Context(), nil, accountKey(req.Email))
		auditapp.AddMeta(r.Context(), "reason", "account locked after repeated failures")
		auditapp.Fail(r.Context())
		minutes := int(wait.Minutes()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeJSON(w, http.StatusTooManyRequests, errorBody{Error: errorDetail{
			Code:    "rate_limited",
			Message: fmt.Sprintf("Too many failed sign-ins for this account. Try again in %d minute%s.", minutes, plural(minutes)),
		}})
		return
	}
	res, err := h.svc.Authenticate(r.Context(), req.Email, req.Password)
	if err != nil {
		// Logged as a failed login with the email that was tried.
		auditapp.SetActor(r.Context(), nil, accountKey(req.Email))
		auditapp.Fail(r.Context())
		if apperr.KindOf(err) == apperr.KindUnauthorized {
			h.guard.Failed(req.Email)
		}
		writeError(w, err)
		return
	}
	h.guard.Succeeded(req.Email)
	auditapp.SetActor(r.Context(), &res.User.ID, res.User.Email)
	h.cookie.set(w, r, res.Token)
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
		Name:        p.Name,
		Permissions: p.Permissions(),
		Grants:      toGrantDTOs(p.Grants()),
	})
}

// Logout handles POST /v1/auth/logout: it invalidates every browser session
// (JWT) of the caller and clears the session cookie.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	if err := h.svc.Logout(r.Context(), p.UserID); err != nil {
		writeError(w, err)
		return
	}
	h.cookie.clear(w, r)
	writeJSON(w, http.StatusNoContent, nil)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
