package auth

import (
	"net/http"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// checkPermission authenticates the request and checks the specified permission.
// Returns the UserClaims on success, or writes an error response and returns nil.
func (p *Plugin) checkPermission(w http.ResponseWriter, r *http.Request, resource, action string) *interfaces.UserClaims {
	claims, err := authenticateRequest(r, p.service)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid authorization header")
		return nil
	}

	allowed, err := p.service.HasPermission(r.Context(), claims.UserID, resource, action)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "permission check failed")
		return nil
	}
	if !allowed {
		writeAuthError(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
		return nil
	}

	return claims
}

// checkNotModifyingAdmin ensures a non-admin actor cannot modify (update/delete) a user with the admin role.
// Returns true if the operation is allowed, false if forbidden (writes 403 response).
func (p *Plugin) checkNotModifyingAdmin(w http.ResponseWriter, r *http.Request, claims *interfaces.UserClaims, targetUserID string) bool {
	// Admins (wildcard permission) can modify anyone.
	isAdmin, err := p.service.HasPermission(r.Context(), claims.UserID, "*", "*")
	if err != nil || isAdmin {
		return true
	}

	// Look up the target user's roles.
	targetRoles, err := p.service.GetUserRoleNames(r.Context(), targetUserID)
	if err != nil {
		// If user not found, the handler will catch it later.
		return true
	}

	for _, role := range targetRoles {
		if role == "admin" {
			writeAuthError(w, http.StatusForbidden, "FORBIDDEN", "cannot modify admin user")
			return false
		}
	}

	return true
}

// checkNotModifyingAdminRole ensures a non-admin cannot modify the admin role itself.
// Returns true if the operation is allowed, false if forbidden.
func (p *Plugin) checkNotModifyingAdminRole(w http.ResponseWriter, r *http.Request, claims *interfaces.UserClaims, roleID string) bool {
	isAdmin, err := p.service.HasPermission(r.Context(), claims.UserID, "*", "*")
	if err != nil || isAdmin {
		return true
	}

	role, err := p.service.GetRoleByID(r.Context(), roleID)
	if err != nil {
		return true
	}

	if role.Name == "admin" {
		writeAuthError(w, http.StatusForbidden, "FORBIDDEN", "cannot modify admin role")
		return false
	}

	return true
}
