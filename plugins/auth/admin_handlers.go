package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

func (p *Plugin) handleListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	result, err := p.service.ListUsers(r.Context(), &interfaces.UserQuery{
		Page:    page,
		PerPage: perPage,
		Status:  q.Get("status"),
		Role:    q.Get("role"),
		Search:  q.Get("search"),
		Sort:    q.Get("sort"),
		Order:   q.Get("order"),
	})
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list users")
		return
	}

	meta := map[string]interface{}{
		"total":       result.Meta.Total,
		"page":        result.Meta.Page,
		"per_page":    result.Meta.PerPage,
		"total_pages": result.Meta.TotalPages,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(authResponse{Data: result.Data, Meta: meta})
}

func (p *Plugin) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req interfaces.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}

	user, err := p.service.CreateUser(r.Context(), &req)
	if err != nil {
		if errors.Is(err, interfaces.ErrValidation) {
			writeAuthError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		if errors.Is(err, interfaces.ErrConflict) {
			writeAuthError(w, http.StatusConflict, "CONFLICT", err.Error())
			return
		}
		writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create user")
		return
	}

	writeAuthJSON(w, http.StatusCreated, user)
}

func (p *Plugin) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeAuthError(w, http.StatusBadRequest, "BAD_REQUEST", "missing user id")
		return
	}

	var req interfaces.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}

	user, err := p.service.UpdateUser(r.Context(), id, &req)
	if err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		if errors.Is(err, interfaces.ErrValidation) {
			writeAuthError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update user")
		return
	}

	writeAuthJSON(w, http.StatusOK, user)
}

func (p *Plugin) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeAuthError(w, http.StatusBadRequest, "BAD_REQUEST", "missing user id")
		return
	}

	if err := p.service.DeleteUser(r.Context(), id); err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete user")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type permissionEntry struct {
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}

type roleResponse struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Description string            `json:"description"`
	Permissions []permissionEntry `json:"permissions"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

func groupPermissions(flat []string) []permissionEntry {
	grouped := make(map[string][]string)
	for _, p := range flat {
		parts := strings.SplitN(p, ".", 2)
		if len(parts) == 2 {
			grouped[parts[0]] = append(grouped[parts[0]], parts[1])
		} else {
			grouped[p] = []string{"all"}
		}
	}
	result := make([]permissionEntry, 0, len(grouped))
	for res, actions := range grouped {
		result = append(result, permissionEntry{Resource: res, Actions: actions})
	}
	return result
}

func toRoleResponse(role *interfaces.Role) roleResponse {
	return roleResponse{
		ID:          role.ID,
		Name:        role.Name,
		DisplayName: role.DisplayName,
		Description: role.Description,
		Permissions: groupPermissions(role.Permissions),
		CreatedAt:   role.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   role.UpdatedAt.Format(time.RFC3339),
	}
}

func (p *Plugin) handleListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := p.service.ListRoles(r.Context())
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list roles")
		return
	}

	if roles == nil {
		roles = []*interfaces.Role{}
	}

	result := make([]roleResponse, 0, len(roles))
	for _, role := range roles {
		perms, perr := p.service.rbac.ListPermissionsForRole(r.Context(), role.Name)
		if perr != nil {
			p.service.logger.Warn("failed to get permissions for role", "role", role.Name, "error", perr)
			role.Permissions = []string{}
		} else {
			role.Permissions = perms
		}
		result = append(result, toRoleResponse(role))
	}

	writeAuthJSON(w, http.StatusOK, result)
}

type updateRoleRequest struct {
	Description string            `json:"description,omitempty"`
	Permissions []permissionEntry `json:"permissions,omitempty"`
}

func flattenPermissions(structured []permissionEntry) []string {
	var flat []string
	for _, p := range structured {
		for _, a := range p.Actions {
			flat = append(flat, p.Resource+"."+a)
		}
	}
	return flat
}

func (p *Plugin) handleUpdateRole(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeAuthError(w, http.StatusBadRequest, "BAD_REQUEST", "missing role id")
		return
	}

	var req updateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}

	flatPerms := flattenPermissions(req.Permissions)
	role, err := p.service.UpdateRole(r.Context(), id, req.Description, flatPerms)
	if err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "NOT_FOUND", "role not found")
			return
		}
		writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update role")
		return
	}

	writeAuthJSON(w, http.StatusOK, toRoleResponse(role))
}

func (p *Plugin) handleListAPITokens(w http.ResponseWriter, r *http.Request) {
	claims, err := authenticateRequest(r, p.service)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid authorization header")
		return
	}

	tokens, err := p.service.ListAPITokens(r.Context(), claims.UserID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list api tokens")
		return
	}

	if tokens == nil {
		tokens = []*interfaces.APIToken{}
	}

	writeAuthJSON(w, http.StatusOK, tokens)
}

type createTokenRequest struct {
	Name      string  `json:"name"`
	ExpiresAt *string `json:"expires_at,omitempty"`
}

type createTokenResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Token     string     `json:"token"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (p *Plugin) handleCreateAPIToken(w http.ResponseWriter, r *http.Request) {
	claims, err := authenticateRequest(r, p.service)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid authorization header")
		return
	}

	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}

	if req.Name == "" {
		writeAuthError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, perr := time.Parse(time.RFC3339, *req.ExpiresAt)
		if perr != nil {
			writeAuthError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid expires_at format, use RFC3339")
			return
		}
		expiresAt = &t
	}

	apiToken, err := p.service.CreateAPIToken(r.Context(), claims.UserID, req.Name, expiresAt)
	if err != nil {
		if errors.Is(err, interfaces.ErrValidation) {
			writeAuthError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create api token")
		return
	}

	writeAuthJSON(w, http.StatusCreated, createTokenResponse{
		ID:        apiToken.ID,
		Name:      apiToken.Name,
		Token:     apiToken.TokenHash,
		CreatedAt: apiToken.CreatedAt,
		ExpiresAt: apiToken.ExpiresAt,
	})
}

func (p *Plugin) handleRevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeAuthError(w, http.StatusBadRequest, "BAD_REQUEST", "missing token id")
		return
	}

	if err := p.service.RevokeAPIToken(r.Context(), id); err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "NOT_FOUND", "api token not found")
			return
		}
		writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to revoke api token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
