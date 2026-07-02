package httpapi

import (
	"net/http"
	"time"

	userapp "github.com/mariadb-cp/db-manager/backend/internal/app/user"
	userdom "github.com/mariadb-cp/db-manager/backend/internal/domain/user"
	"github.com/mariadb-cp/db-manager/backend/internal/platform/apperr"
)

// UserHandler exposes user administration and self-service profile endpoints.
type UserHandler struct {
	svc *userapp.Service
}

// NewUserHandler builds the user handler.
func NewUserHandler(svc *userapp.Service) *UserHandler { return &UserHandler{svc: svc} }

type userResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toUserResponse(u userdom.WithRoles) userResponse {
	return userResponse{
		ID:        u.ID.String(),
		Email:     u.Email,
		Name:      u.Name,
		Status:    u.Status,
		Roles:     u.Roles,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

type roleResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IsSystem    bool     `json:"is_system"`
	Permissions []string `json:"permissions"`
}

// ---- Administration ----

// List handles GET /v1/users.
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]userResponse, 0, len(users))
	for _, u := range users {
		items = append(items, toUserResponse(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type createUserRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// Create handles POST /v1/users.
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	u, err := h.svc.Create(r.Context(), userapp.CreateInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
		Role:     req.Role,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toUserResponse(*u))
}

type updateUserRequest struct {
	Name   *string `json:"name"`
	Email  *string `json:"email"`
	Status *string `json:"status"`
	Role   *string `json:"role"`
}

// Update handles PATCH /v1/users/{id}.
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	var req updateUserRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	u, err := h.svc.Update(r.Context(), p.UserID, r.PathValue("id"), userapp.UpdateInput{
		Name:   req.Name,
		Email:  req.Email,
		Status: req.Status,
		Role:   req.Role,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(*u))
}

type resetPasswordRequest struct {
	Password string `json:"password"`
}

// ResetPassword handles POST /v1/users/{id}/password.
func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.ResetPassword(r.Context(), r.PathValue("id"), req.Password); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Delete handles DELETE /v1/users/{id}.
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	if err := h.svc.Delete(r.Context(), p.UserID, r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ListRoles handles GET /v1/roles.
func (h *UserHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.ListRoles(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]roleResponse, 0, len(roles))
	for _, ro := range roles {
		items = append(items, roleResponse{
			ID:          ro.ID.String(),
			Name:        ro.Name,
			Description: ro.Description,
			IsSystem:    ro.IsSystem,
			Permissions: ro.Permissions,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// ---- Role management ----

type roleInputRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Permissions []string `json:"permissions"`
}

// CreateRole handles POST /v1/roles.
func (h *UserHandler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req roleInputRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	ro, err := h.svc.CreateRole(r.Context(), userapp.RoleInput{
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, roleResponse{
		ID: ro.ID.String(), Name: ro.Name, Description: ro.Description,
		IsSystem: ro.IsSystem, Permissions: ro.Permissions,
	})
}

// UpdateRole handles PATCH /v1/roles/{id}.
func (h *UserHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	var req roleInputRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	ro, err := h.svc.UpdateRole(r.Context(), r.PathValue("id"), userapp.RoleInput{
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, roleResponse{
		ID: ro.ID.String(), Name: ro.Name, Description: ro.Description,
		IsSystem: ro.IsSystem, Permissions: ro.Permissions,
	})
}

// DeleteRole handles DELETE /v1/roles/{id}.
func (h *UserHandler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteRole(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ListPermissions handles GET /v1/permissions.
func (h *UserHandler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": h.svc.PermissionCatalog()})
}

// ---- Self-service profile ----

// Profile handles GET /v1/profile.
func (h *UserHandler) Profile(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	u, err := h.svc.Profile(r.Context(), p.UserID)
	if err != nil {
		writeError(w, err)
		return
	}
	resp := struct {
		userResponse
		Permissions []string `json:"permissions"`
	}{toUserResponse(*u), p.Permissions()}
	writeJSON(w, http.StatusOK, resp)
}

type updateProfileRequest struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
}

// UpdateProfile handles PATCH /v1/profile.
func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	var req updateProfileRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	u, err := h.svc.UpdateProfile(r.Context(), p.UserID, req.Name, req.Email)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(*u))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword handles POST /v1/profile/password.
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	var req changePasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.CurrentPassword == "" {
		writeError(w, apperr.Invalid("current_password", "current_password is required"))
		return
	}
	if err := h.svc.ChangePassword(r.Context(), p.UserID, req.CurrentPassword, req.NewPassword); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
