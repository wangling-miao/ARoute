package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// RBACManager handles role-based access control operations.
type RBACManager struct {
	store  *Store
	logger *slog.Logger
}

// NewRBACManager creates a new RBACManager.
func NewRBACManager(store *Store, logger *slog.Logger) *RBACManager {
	return &RBACManager{
		store:  store,
		logger: logger,
	}
}

// defaultRoleDef defines a builtin role with its permissions.
type defaultRoleDef struct {
	Name        string
	DisplayName string
	Description string
	Permissions []defaultPermDef
}

// defaultPermDef defines a permission for builtin roles.
type defaultPermDef struct {
	Name        string
	Resource    string
	Action      string
	DisplayName string
	Description string
}

var defaultRoles = []defaultRoleDef{
	{
		Name:        "admin",
		DisplayName: "Administrator",
		Description: "Full access to all system resources",
		Permissions: []defaultPermDef{
			{Name: "wildcard.all", Resource: "*", Action: "*", DisplayName: "Full Access", Description: "Access to all resources and actions"},
		},
	},
	{
		Name:        "editor",
		DisplayName: "Editor",
		Description: "Can manage content and upload media",
		Permissions: []defaultPermDef{
			{Name: "content.create", Resource: "content", Action: "create", DisplayName: "Create Content", Description: "Create new content items"},
			{Name: "content.read", Resource: "content", Action: "read", DisplayName: "Read Content", Description: "View content items"},
			{Name: "content.update", Resource: "content", Action: "update", DisplayName: "Update Content", Description: "Edit any content items"},
			{Name: "content.delete", Resource: "content", Action: "delete", DisplayName: "Delete Content", Description: "Delete content items"},
			{Name: "media.read", Resource: "media", Action: "read", DisplayName: "Read Media", Description: "View media files"},
			{Name: "media.upload", Resource: "media", Action: "upload", DisplayName: "Upload Media", Description: "Upload new media files"},
		},
	},
	{
		Name:        "author",
		DisplayName: "Author",
		Description: "Can create and manage own content, upload media",
		Permissions: []defaultPermDef{
			{Name: "content.create", Resource: "content", Action: "create", DisplayName: "Create Content", Description: "Create new content items"},
			{Name: "content.read", Resource: "content", Action: "read", DisplayName: "Read Content", Description: "View content items"},
			{Name: "content.update_own", Resource: "content", Action: "update_own", DisplayName: "Update Own Content", Description: "Edit own content items"},
			{Name: "media.upload", Resource: "media", Action: "upload", DisplayName: "Upload Media", Description: "Upload new media files"},
		},
	},
	{
		Name:        "viewer",
		DisplayName: "Viewer",
		Description: "Read-only access to content and media",
		Permissions: []defaultPermDef{
			{Name: "content.read", Resource: "content", Action: "read", DisplayName: "Read Content", Description: "View content items"},
			{Name: "media.read", Resource: "media", Action: "read", DisplayName: "Read Media", Description: "View media files"},
		},
	},
}

// InitializeDefaultRoles creates the default roles and permissions if they don't exist.
func (m *RBACManager) InitializeDefaultRoles(ctx context.Context) error {
	for _, roleDef := range defaultRoles {
		// Check if role already exists.
		existing, err := m.store.GetRoleByName(ctx, roleDef.Name)
		if err != nil && err != interfaces.ErrNotFound {
			return fmt.Errorf("check role %q: %w", roleDef.Name, err)
		}
		if existing != nil {
			m.logger.Debug("role already exists, skipping", "role", roleDef.Name)
			continue
		}

		now := time.Now().UTC()
		role := &interfaces.Role{
			ID:          newUUID(),
			Name:        roleDef.Name,
			DisplayName: roleDef.DisplayName,
			Description: roleDef.Description,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if err := m.store.CreateBuiltinRole(ctx, role); err != nil {
			return fmt.Errorf("create builtin role %q: %w", roleDef.Name, err)
		}
		m.logger.Info("created builtin role", "role", roleDef.Name)

		// Create permissions and assign to role.
		for _, permDef := range roleDef.Permissions {
			perm, err := m.store.GetPermissionByResourceAction(ctx, permDef.Resource, permDef.Action)
			if err != nil && err != interfaces.ErrNotFound {
				return fmt.Errorf("check permission %q: %w", permDef.Name, err)
			}
			if perm == nil {
				perm = &interfaces.Permission{
					ID:          newUUID(),
					Name:        permDef.Name,
					Resource:    permDef.Resource,
					Action:      permDef.Action,
					DisplayName: permDef.DisplayName,
					Description: permDef.Description,
				}
				if err := m.store.CreatePermission(ctx, perm); err != nil {
					return fmt.Errorf("create permission %q: %w", permDef.Name, err)
				}
			}

			if err := m.store.AddRolePermission(ctx, role.ID, perm.ID); err != nil {
				return fmt.Errorf("assign permission %q to role %q: %w", permDef.Name, roleDef.Name, err)
			}
		}
		m.logger.Debug("assigned permissions to role",
			"role", roleDef.Name,
			"permission_count", len(roleDef.Permissions),
		)
	}

	// Seed all known permissions so they appear in the role management UI.
	if err := m.seedAllPermissions(ctx); err != nil {
		m.logger.Warn("failed to seed permission entries", "error", err)
	}

	return nil
}

// seedAllPermissions ensures all known resource.action permissions exist in the database,
// even if not assigned to any default role. This is needed for the role management UI.
func (m *RBACManager) seedAllPermissions(ctx context.Context) error {
	allPerms := []defaultPermDef{
		// Users
		{Name: "users.create", Resource: "users", Action: "create", DisplayName: "Create Users", Description: "Create new user accounts"},
		{Name: "users.read", Resource: "users", Action: "read", DisplayName: "Read Users", Description: "View user accounts"},
		{Name: "users.update", Resource: "users", Action: "update", DisplayName: "Update Users", Description: "Edit user accounts"},
		{Name: "users.delete", Resource: "users", Action: "delete", DisplayName: "Delete Users", Description: "Delete user accounts"},
		// Roles
		{Name: "roles.read", Resource: "roles", Action: "read", DisplayName: "Read Roles", Description: "View roles"},
		{Name: "roles.update", Resource: "roles", Action: "update", DisplayName: "Update Roles", Description: "Edit role permissions"},
		// Content types
		{Name: "content_types.create", Resource: "content_types", Action: "create", DisplayName: "Create Content Types", Description: "Create new content types"},
		{Name: "content_types.read", Resource: "content_types", Action: "read", DisplayName: "Read Content Types", Description: "View content types"},
		{Name: "content_types.update", Resource: "content_types", Action: "update", DisplayName: "Update Content Types", Description: "Edit content types"},
		{Name: "content_types.delete", Resource: "content_types", Action: "delete", DisplayName: "Delete Content Types", Description: "Delete content types"},
		// Content
		{Name: "content.create", Resource: "content", Action: "create", DisplayName: "Create Content", Description: "Create content items"},
		{Name: "content.read", Resource: "content", Action: "read", DisplayName: "Read Content", Description: "View content items"},
		{Name: "content.update", Resource: "content", Action: "update", DisplayName: "Update Content", Description: "Edit any content items"},
		{Name: "content.delete", Resource: "content", Action: "delete", DisplayName: "Delete Content", Description: "Delete content items"},
		// Media
		{Name: "media.read", Resource: "media", Action: "read", DisplayName: "Read Media", Description: "View media files"},
		{Name: "media.upload", Resource: "media", Action: "upload", DisplayName: "Upload Media", Description: "Upload new media files"},
		{Name: "media.delete", Resource: "media", Action: "delete", DisplayName: "Delete Media", Description: "Delete media files"},
		// Settings
		{Name: "settings.read", Resource: "settings", Action: "read", DisplayName: "Read Settings", Description: "View settings"},
		{Name: "settings.update", Resource: "settings", Action: "update", DisplayName: "Update Settings", Description: "Edit settings"},
		// Plugins
		{Name: "plugins.read", Resource: "plugins", Action: "read", DisplayName: "Read Plugins", Description: "View plugins"},
		{Name: "plugins.enable", Resource: "plugins", Action: "enable", DisplayName: "Enable Plugins", Description: "Enable plugins"},
		{Name: "plugins.disable", Resource: "plugins", Action: "disable", DisplayName: "Disable Plugins", Description: "Disable plugins"},
		// API Tokens
		{Name: "api_tokens.create", Resource: "api_tokens", Action: "create", DisplayName: "Create API Tokens", Description: "Create API tokens"},
		{Name: "api_tokens.read", Resource: "api_tokens", Action: "read", DisplayName: "Read API Tokens", Description: "View API tokens"},
		{Name: "api_tokens.delete", Resource: "api_tokens", Action: "delete", DisplayName: "Delete API Tokens", Description: "Revoke API tokens"},
		// Webhooks
		{Name: "webhooks.create", Resource: "webhooks", Action: "create", DisplayName: "Create Webhooks", Description: "Create webhook endpoints"},
		{Name: "webhooks.read", Resource: "webhooks", Action: "read", DisplayName: "Read Webhooks", Description: "View webhook endpoints"},
		{Name: "webhooks.update", Resource: "webhooks", Action: "update", DisplayName: "Update Webhooks", Description: "Edit webhook endpoints"},
		{Name: "webhooks.delete", Resource: "webhooks", Action: "delete", DisplayName: "Delete Webhooks", Description: "Delete webhook endpoints"},
		{Name: "webhooks.test", Resource: "webhooks", Action: "test", DisplayName: "Test Webhooks", Description: "Send webhook test deliveries"},
		// Menus
		{Name: "menus.read", Resource: "menus", Action: "read", DisplayName: "Read Menus", Description: "View menus"},
		{Name: "menus.update", Resource: "menus", Action: "update", DisplayName: "Update Menus", Description: "Edit menus"},
	}

	for _, permDef := range allPerms {
		existing, err := m.store.GetPermissionByResourceAction(ctx, permDef.Resource, permDef.Action)
		if err != nil && err != interfaces.ErrNotFound {
			return fmt.Errorf("check permission %q: %w", permDef.Name, err)
		}
		if existing != nil {
			continue
		}
		perm := &interfaces.Permission{
			ID:          newUUID(),
			Name:        permDef.Name,
			Resource:    permDef.Resource,
			Action:      permDef.Action,
			DisplayName: permDef.DisplayName,
			Description: permDef.Description,
		}
		if err := m.store.CreatePermission(ctx, perm); err != nil {
			return fmt.Errorf("create permission %q: %w", permDef.Name, err)
		}
	}

	return nil
}

// AssignRole assigns a role to a user by role name.
func (m *RBACManager) AssignRole(ctx context.Context, userID, roleName string) error {
	role, err := m.store.GetRoleByName(ctx, roleName)
	if err != nil {
		return fmt.Errorf("get role %q: %w", roleName, err)
	}
	if err := m.store.AddUserRole(ctx, userID, role.ID); err != nil {
		return fmt.Errorf("assign role %q to user %q: %w", roleName, userID, err)
	}
	return nil
}

// RemoveRole removes a role from a user by role name.
func (m *RBACManager) RemoveRole(ctx context.Context, userID, roleName string) error {
	role, err := m.store.GetRoleByName(ctx, roleName)
	if err != nil {
		return fmt.Errorf("get role %q: %w", roleName, err)
	}
	if err := m.store.RemoveUserRole(ctx, userID, role.ID); err != nil {
		return fmt.Errorf("remove role %q from user %q: %w", roleName, userID, err)
	}
	return nil
}

// HasPermission checks if a user has a specific permission on a resource/action.
// It supports wildcard matching: if the user has a wildcard permission (*, *),
// all permission checks pass.
func (m *RBACManager) HasPermission(ctx context.Context, userID, resource, action string) (bool, error) {
	perms, err := m.store.GetUserPermissions(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("get user permissions: %w", err)
	}

	for _, perm := range perms {
		// Check for full wildcard (*, *).
		if perm.Resource == "*" && perm.Action == "*" {
			return true, nil
		}
		// Check for resource wildcard (*, action).
		if perm.Resource == "*" && perm.Action == action {
			return true, nil
		}
		// Check for action wildcard (resource, *).
		if perm.Resource == resource && perm.Action == "*" {
			return true, nil
		}
		// Check for exact match.
		if perm.Resource == resource && perm.Action == action {
			return true, nil
		}
	}

	return false, nil
}

// CreateRole creates a new custom role.
func (m *RBACManager) CreateRole(ctx context.Context, name, displayName, description string) (*interfaces.Role, error) {
	now := time.Now().UTC()
	role := &interfaces.Role{
		ID:          newUUID(),
		Name:        name,
		DisplayName: displayName,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := m.store.CreateRole(ctx, role); err != nil {
		return nil, fmt.Errorf("create role %q: %w", name, err)
	}
	return role, nil
}

// DeleteRole deletes a role by name. Builtin roles cannot be deleted.
func (m *RBACManager) DeleteRole(ctx context.Context, name string) error {
	role, err := m.store.GetRoleByName(ctx, name)
	if err != nil {
		return fmt.Errorf("get role %q: %w", name, err)
	}

	isBuiltin, err := m.store.IsBuiltinRole(ctx, role.ID)
	if err != nil {
		return fmt.Errorf("check builtin role: %w", err)
	}
	if isBuiltin {
		return fmt.Errorf("cannot delete builtin role %q", name)
	}

	assigned, err := m.store.IsRoleAssigned(ctx, role.ID)
	if err != nil {
		return fmt.Errorf("check role assignment: %w", err)
	}
	if assigned {
		return fmt.Errorf("cannot delete role %q: still assigned to users", name)
	}

	if err := m.store.DeleteRole(ctx, role.ID); err != nil {
		return fmt.Errorf("delete role %q: %w", name, err)
	}
	return nil
}

// AssignPermission assigns a permission (resource:action) to a role by name.
func (m *RBACManager) AssignPermission(ctx context.Context, roleName, resource, action string) error {
	role, err := m.store.GetRoleByName(ctx, roleName)
	if err != nil {
		return fmt.Errorf("get role %q: %w", roleName, err)
	}

	// Find or create the permission.
	perm, err := m.store.GetPermissionByResourceAction(ctx, resource, action)
	if err != nil && err != interfaces.ErrNotFound {
		return fmt.Errorf("get permission: %w", err)
	}
	if perm == nil {
		permName := resource + "." + action
		perm = &interfaces.Permission{
			ID:          newUUID(),
			Name:        permName,
			Resource:    resource,
			Action:      action,
			DisplayName: permName,
		}
		if err := m.store.CreatePermission(ctx, perm); err != nil {
			return fmt.Errorf("create permission: %w", err)
		}
	}

	if err := m.store.AddRolePermission(ctx, role.ID, perm.ID); err != nil {
		return fmt.Errorf("assign permission to role %q: %w", roleName, err)
	}
	return nil
}

// RevokePermission removes a permission from a role by name.
func (m *RBACManager) RevokePermission(ctx context.Context, roleName, resource, action string) error {
	role, err := m.store.GetRoleByName(ctx, roleName)
	if err != nil {
		return fmt.Errorf("get role %q: %w", roleName, err)
	}

	perm, err := m.store.GetPermissionByResourceAction(ctx, resource, action)
	if err != nil {
		if err == interfaces.ErrNotFound {
			return nil // Permission doesn't exist, nothing to revoke.
		}
		return fmt.Errorf("get permission: %w", err)
	}

	if err := m.store.RemoveRolePermission(ctx, role.ID, perm.ID); err != nil {
		return fmt.Errorf("revoke permission from role %q: %w", roleName, err)
	}
	return nil
}

// ListPermissionsForRole returns all permission names for a given role.
func (m *RBACManager) ListPermissionsForRole(ctx context.Context, roleName string) ([]string, error) {
	role, err := m.store.GetRoleByName(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("get role %q: %w", roleName, err)
	}

	perms, err := m.store.GetPermissionsByRoleID(ctx, role.ID)
	if err != nil {
		return nil, fmt.Errorf("get permissions for role %q: %w", roleName, err)
	}

	names := make([]string, 0, len(perms))
	for _, p := range perms {
		// Use resource.action format so groupPermissions can reconstruct the
		// structured entry correctly (e.g. "*.*" for the admin wildcard).
		names = append(names, p.Resource+"."+p.Action)
	}
	return names, nil
}

// ListAllRoles returns all roles in the system.
func (m *RBACManager) ListAllRoles(ctx context.Context) ([]*interfaces.Role, error) {
	return m.store.ListRoles(ctx)
}
