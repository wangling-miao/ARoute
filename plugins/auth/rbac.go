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
