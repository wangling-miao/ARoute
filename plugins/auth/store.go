package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// Store handles all database operations for the auth plugin.
type Store struct {
	db interfaces.DatabaseService
}

// NewStore creates a new Store with the given DatabaseService.
func NewStore(db interfaces.DatabaseService) *Store {
	return &Store{db: db}
}

// CreateTables creates all auth-related tables if they do not exist.
func (s *Store) CreateTables(ctx context.Context) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
			updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
			last_login_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS roles (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			is_builtin INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
			updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
		)`,
		`CREATE TABLE IF NOT EXISTS permissions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			resource TEXT NOT NULL,
			action TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS role_permissions (
			role_id TEXT NOT NULL REFERENCES roles(id),
			permission_id TEXT NOT NULL REFERENCES permissions(id),
			PRIMARY KEY (role_id, permission_id)
		)`,
		`CREATE TABLE IF NOT EXISTS user_roles (
			user_id TEXT NOT NULL REFERENCES users(id),
			role_id TEXT NOT NULL REFERENCES roles(id),
			PRIMARY KEY (user_id, role_id)
		)`,
		`CREATE TABLE IF NOT EXISTS api_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id),
			name TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
			expires_at TEXT,
			last_used_at TEXT,
			revoked INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS token_blacklist (
			jti TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
		)`,
		`CREATE TABLE IF NOT EXISTS user_token_revocations (
			user_id TEXT NOT NULL PRIMARY KEY,
			revoked_before TEXT NOT NULL
		)`,
	}

	for _, table := range tables {
		if _, err := s.db.Exec(ctx, table); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}
	return nil
}

// --- User operations ---

// CreateUser inserts a new user record.
func (s *Store) CreateUser(ctx context.Context, user *interfaces.User) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO users (id, email, username, password_hash, status, created_at, updated_at, last_login_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.Email, user.Username, user.PasswordHash, user.Status,
		user.CreatedAt.Format(time.RFC3339), user.UpdatedAt.Format(time.RFC3339), nilTime(user.LastLoginAt),
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// GetUserByID retrieves a user by their unique ID.
func (s *Store) GetUserByID(ctx context.Context, id string) (*interfaces.User, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, email, username, password_hash, status, created_at, updated_at, last_login_at
		 FROM users WHERE id = ?`, id,
	)
	return s.scanUser(row)
}

// GetUserByEmail retrieves a user by their email address.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*interfaces.User, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, email, username, password_hash, status, created_at, updated_at, last_login_at
		 FROM users WHERE email = ?`, email,
	)
	return s.scanUser(row)
}

// GetUserByUsername retrieves a user by their username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*interfaces.User, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, email, username, password_hash, status, created_at, updated_at, last_login_at
		 FROM users WHERE username = ?`, username,
	)
	return s.scanUser(row)
}

// UpdateUser updates an existing user record.
func (s *Store) UpdateUser(ctx context.Context, user *interfaces.User) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET email = ?, username = ?, password_hash = ?, status = ?, updated_at = ?
		 WHERE id = ?`,
		user.Email, user.Username, user.PasswordHash, user.Status,
		user.UpdatedAt.Format(time.RFC3339), user.ID,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

// UpdateLastLogin sets the last_login_at timestamp for a user.
func (s *Store) UpdateLastLogin(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE users SET last_login_at = ?, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	return nil
}

// CountUsers returns the total number of users.
func (s *Store) CountUsers(ctx context.Context) (int64, error) {
	row := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM users`)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

// --- Role operations ---

// CreateRole inserts a new role record.
func (s *Store) CreateRole(ctx context.Context, role *interfaces.Role) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO roles (id, name, display_name, description, is_builtin, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		role.ID, role.Name, role.DisplayName, role.Description, 0,
		role.CreatedAt.Format(time.RFC3339), role.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert role: %w", err)
	}
	return nil
}

// CreateBuiltinRole inserts a builtin role record.
func (s *Store) CreateBuiltinRole(ctx context.Context, role *interfaces.Role) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO roles (id, name, display_name, description, is_builtin, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 1, ?, ?)`,
		role.ID, role.Name, role.DisplayName, role.Description,
		role.CreatedAt.Format(time.RFC3339), role.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert builtin role: %w", err)
	}
	return nil
}

// GetRoleByID retrieves a role by its unique ID.
func (s *Store) GetRoleByID(ctx context.Context, id string) (*interfaces.Role, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, name, display_name, description, created_at, updated_at
		 FROM roles WHERE id = ?`, id,
	)
	return s.scanRole(row)
}

// GetRoleByName retrieves a role by its name.
func (s *Store) GetRoleByName(ctx context.Context, name string) (*interfaces.Role, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, name, display_name, description, created_at, updated_at
		 FROM roles WHERE name = ?`, name,
	)
	return s.scanRole(row)
}

// ListRoles returns all roles.
func (s *Store) ListRoles(ctx context.Context) ([]*interfaces.Role, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, name, display_name, description, created_at, updated_at
		 FROM roles ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	roles := []*interfaces.Role{}
	for rows.Next() {
		role, err := s.scanRoleRow(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

// UpdateRole updates an existing role.
func (s *Store) UpdateRole(ctx context.Context, role *interfaces.Role) error {
	_, err := s.db.Exec(ctx,
		`UPDATE roles SET display_name = ?, description = ?, updated_at = ? WHERE id = ?`,
		role.DisplayName, role.Description, role.UpdatedAt.Format(time.RFC3339), role.ID,
	)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	return nil
}

// DeleteRole removes a role by ID.
func (s *Store) DeleteRole(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM roles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	return nil
}

// IsRoleAssigned checks if a role is assigned to any user.
func (s *Store) IsRoleAssigned(ctx context.Context, id string) (bool, error) {
	row := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM user_roles WHERE role_id = ?`, id)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, fmt.Errorf("check role assignment: %w", err)
	}
	return count > 0, nil
}

// IsBuiltinRole checks if a role is a builtin role.
func (s *Store) IsBuiltinRole(ctx context.Context, id string) (bool, error) {
	row := s.db.QueryRow(ctx, `SELECT is_builtin FROM roles WHERE id = ?`, id)
	var isBuiltin int
	if err := row.Scan(&isBuiltin); err != nil {
		return false, fmt.Errorf("check builtin role: %w", err)
	}
	return isBuiltin == 1, nil
}

// --- Permission operations ---

// CreatePermission inserts a new permission.
func (s *Store) CreatePermission(ctx context.Context, perm *interfaces.Permission) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO permissions (id, name, resource, action, display_name, description)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		perm.ID, perm.Name, perm.Resource, perm.Action, perm.DisplayName, perm.Description,
	)
	if err != nil {
		return fmt.Errorf("insert permission: %w", err)
	}
	return nil
}

// GetPermissionByID retrieves a permission by ID.
func (s *Store) GetPermissionByID(ctx context.Context, id string) (*interfaces.Permission, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, name, resource, action, display_name, description
		 FROM permissions WHERE id = ?`, id,
	)
	return s.scanPermission(row)
}

// GetPermissionByResourceAction retrieves a permission by resource and action.
func (s *Store) GetPermissionByResourceAction(ctx context.Context, resource, action string) (*interfaces.Permission, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, name, resource, action, display_name, description
		 FROM permissions WHERE resource = ? AND action = ?`, resource, action,
	)
	return s.scanPermission(row)
}

// GetPermissionsByRoleID retrieves all permissions for a given role.
func (s *Store) GetPermissionsByRoleID(ctx context.Context, roleID string) ([]*interfaces.Permission, error) {
	rows, err := s.db.Query(ctx,
		`SELECT p.id, p.name, p.resource, p.action, p.display_name, p.description
		 FROM permissions p
		 JOIN role_permissions rp ON rp.permission_id = p.id
		 WHERE rp.role_id = ?`, roleID,
	)
	if err != nil {
		return nil, fmt.Errorf("get permissions by role: %w", err)
	}
	defer rows.Close()

	perms := []*interfaces.Permission{}
	for rows.Next() {
		perm, err := s.scanPermissionRow(rows)
		if err != nil {
			return nil, err
		}
		perms = append(perms, perm)
	}
	return perms, rows.Err()
}

// ListPermissions returns all permissions.
func (s *Store) ListPermissions(ctx context.Context) ([]*interfaces.Permission, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, name, resource, action, display_name, description
		 FROM permissions ORDER BY resource, action`,
	)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()

	perms := []*interfaces.Permission{}
	for rows.Next() {
		perm, err := s.scanPermissionRow(rows)
		if err != nil {
			return nil, err
		}
		perms = append(perms, perm)
	}
	return perms, rows.Err()
}

// DeletePermission removes a permission by ID.
func (s *Store) DeletePermission(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM permissions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete permission: %w", err)
	}
	return nil
}

// --- Role-Permission mapping ---

// AddRolePermission adds a permission to a role.
func (s *Store) AddRolePermission(ctx context.Context, roleID, permID string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?) ON CONFLICT (role_id, permission_id) DO NOTHING`,
		roleID, permID,
	)
	if err != nil {
		return fmt.Errorf("add role permission: %w", err)
	}
	return nil
}

// RemoveRolePermission removes a permission from a role.
func (s *Store) RemoveRolePermission(ctx context.Context, roleID, permID string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM role_permissions WHERE role_id = ? AND permission_id = ?`,
		roleID, permID,
	)
	if err != nil {
		return fmt.Errorf("remove role permission: %w", err)
	}
	return nil
}

// --- User-Role mapping ---

// AddUserRole assigns a role to a user.
func (s *Store) AddUserRole(ctx context.Context, userID, roleID string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES (?, ?) ON CONFLICT (user_id, role_id) DO NOTHING`,
		userID, roleID,
	)
	if err != nil {
		return fmt.Errorf("add user role: %w", err)
	}
	return nil
}

// RemoveUserRole removes a role from a user.
func (s *Store) RemoveUserRole(ctx context.Context, userID, roleID string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM user_roles WHERE user_id = ? AND role_id = ?`,
		userID, roleID,
	)
	if err != nil {
		return fmt.Errorf("remove user role: %w", err)
	}
	return nil
}

// GetUserRoles returns all roles assigned to a user.
func (s *Store) GetUserRoles(ctx context.Context, userID string) ([]*interfaces.Role, error) {
	rows, err := s.db.Query(ctx,
		`SELECT r.id, r.name, r.display_name, r.description, r.created_at, r.updated_at
		 FROM roles r
		 JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get user roles: %w", err)
	}
	defer rows.Close()

	roles := []*interfaces.Role{}
	for rows.Next() {
		role, err := s.scanRoleRow(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

// GetUserPermissions returns all permissions for a user through their roles.
func (s *Store) GetUserPermissions(ctx context.Context, userID string) ([]*interfaces.Permission, error) {
	rows, err := s.db.Query(ctx,
		`SELECT DISTINCT p.id, p.name, p.resource, p.action, p.display_name, p.description
		 FROM permissions p
		 JOIN role_permissions rp ON rp.permission_id = p.id
		 JOIN user_roles ur ON ur.role_id = rp.role_id
		 WHERE ur.user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get user permissions: %w", err)
	}
	defer rows.Close()

	perms := []*interfaces.Permission{}
	for rows.Next() {
		perm, err := s.scanPermissionRow(rows)
		if err != nil {
			return nil, err
		}
		perms = append(perms, perm)
	}
	return perms, rows.Err()
}

// GetUserRoleNames returns role names for a user.
func (s *Store) GetUserRoleNames(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.Query(ctx,
		`SELECT r.name FROM roles r
		 JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get user role names: %w", err)
	}
	defer rows.Close()

	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan role name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// --- API Token operations ---

// CreateAPIToken inserts a new API token record.
func (s *Store) CreateAPIToken(ctx context.Context, token *interfaces.APIToken) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO api_tokens (id, user_id, name, token_hash, created_at, expires_at, last_used_at, revoked)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
		token.ID, token.UserID, token.Name, token.TokenHash,
		token.CreatedAt.Format(time.RFC3339), nilTime(token.ExpiresAt), nilTime(token.LastUsedAt),
	)
	if err != nil {
		return fmt.Errorf("insert api token: %w", err)
	}
	return nil
}

// GetAPITokenByHash retrieves an API token by its hash.
func (s *Store) GetAPITokenByHash(ctx context.Context, hash string) (*interfaces.APIToken, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, user_id, name, token_hash, created_at, expires_at, last_used_at, revoked
		 FROM api_tokens WHERE token_hash = ? AND revoked = 0`, hash,
	)
	return s.scanAPIToken(row)
}

// ListAPITokensByUser returns all API tokens for a user.
func (s *Store) ListAPITokensByUser(ctx context.Context, userID string) ([]*interfaces.APIToken, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, user_id, name, token_hash, created_at, expires_at, last_used_at, revoked
		 FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close()

	tokens := []*interfaces.APIToken{}
	for rows.Next() {
		token, err := s.scanAPITokenRow(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

// RevokeAPIToken marks an API token as revoked.
func (s *Store) RevokeAPIToken(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE api_tokens SET revoked = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("revoke api token: %w", err)
	}
	return nil
}

// UpdateAPITokenLastUsed updates the last_used_at timestamp.
func (s *Store) UpdateAPITokenLastUsed(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE api_tokens SET last_used_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("update api token last used: %w", err)
	}
	return nil
}

// --- Token blacklist ---

// BlacklistToken adds a token JTI to the blacklist.
func (s *Store) BlacklistToken(ctx context.Context, jti, userID string, expiresAt time.Time) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO token_blacklist (jti, user_id, expires_at) VALUES (?, ?, ?) ON CONFLICT (jti) DO NOTHING`,
		jti, userID, expiresAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("blacklist token: %w", err)
	}
	return nil
}

// IsTokenBlacklisted checks if a token JTI has been blacklisted.
func (s *Store) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	row := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM token_blacklist WHERE jti = ?`, jti,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, fmt.Errorf("check token blacklist: %w", err)
	}
	return count > 0, nil
}

// CleanupBlacklist removes expired blacklist entries.
func (s *Store) CleanupBlacklist(ctx context.Context, now time.Time) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM token_blacklist WHERE expires_at < ?`,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("cleanup blacklist: %w", err)
	}
	return nil
}

// --- User token revocations ---

// SetUserRevocation sets a revocation timestamp for all user tokens.
func (s *Store) SetUserRevocation(ctx context.Context, userID string, revokedBefore time.Time) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO user_token_revocations (user_id, revoked_before) VALUES (?, ?) ON CONFLICT (user_id) DO UPDATE SET revoked_before = EXCLUDED.revoked_before`,
		userID, revokedBefore.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("set user revocation: %w", err)
	}
	return nil
}

// GetUserRevocation returns the revocation timestamp for a user, if any.
func (s *Store) GetUserRevocation(ctx context.Context, userID string) (*time.Time, error) {
	row := s.db.QueryRow(ctx,
		`SELECT revoked_before FROM user_token_revocations WHERE user_id = ?`, userID,
	)
	var revokedBeforeStr string
	if err := row.Scan(&revokedBeforeStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get user revocation: %w", err)
	}
	t, err := time.Parse(time.RFC3339, revokedBeforeStr)
	if err != nil {
		return nil, fmt.Errorf("parse revocation timestamp: %w", err)
	}
	return &t, nil
}

func (s *Store) ListUsers(ctx context.Context, query *interfaces.UserQuery) (*interfaces.Page, error) {
	where := "WHERE 1=1"
	args := []interface{}{}

	if query.Status != "" {
		where += " AND status = ?"
		args = append(args, query.Status)
	}
	if query.Search != "" {
		where += " AND (email LIKE ? OR username LIKE ?)"
		pattern := "%" + query.Search + "%"
		args = append(args, pattern, pattern)
	}
	if query.Role != "" {
		where += " AND id IN (SELECT ur.user_id FROM user_roles ur JOIN roles r ON ur.role_id = r.id WHERE r.name = ?)"
		args = append(args, query.Role)
	}

	sortCol := "created_at"
	sortOrder := "DESC"
	if query.Sort != "" {
		if isValidSortColumn(query.Sort) {
			sortCol = query.Sort
		}
	}
	if strings.EqualFold(query.Order, "asc") {
		sortOrder = "ASC"
	}

	countRow := s.db.QueryRow(ctx, "SELECT COUNT(*) FROM users "+where, args...)
	var total int64
	if err := countRow.Scan(&total); err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}

	offset := (query.Page - 1) * query.PerPage
	rows, err := s.db.Query(ctx,
		"SELECT id, email, username, status, created_at, updated_at FROM users "+where+
			" ORDER BY "+sortCol+" "+sortOrder+" LIMIT ? OFFSET ?",
		append(args, query.PerPage, offset)...,
	)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	users := make([]*interfaces.User, 0)
	for rows.Next() {
		var u interfaces.User
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.Status, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan user row: %w", err)
		}
		if t, err := time.Parse(time.RFC3339, createdAt.String); err == nil {
			u.CreatedAt = t
		}
		if t, err := time.Parse(time.RFC3339, updatedAt.String); err == nil {
			u.UpdatedAt = t
		}
		roleNames, _ := s.GetUserRoleNames(ctx, u.ID)
		if roleNames == nil {
			roleNames = []string{}
		}
		u.Roles = roleNames
		users = append(users, &u)
	}

	return &interfaces.Page{
		Data: users,
		Meta: interfaces.PageMeta{
			Page:    query.Page,
			PerPage: query.PerPage,
			Total:   total,
		},
	}, nil
}

var sortColRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func isValidSortColumn(name string) bool {
	return sortColRegex.MatchString(name)
}

// --- Helpers ---

// newUUID generates a new UUID string.
func newUUID() string {
	return uuid.New().String()
}

// nilTime returns a RFC3339 string or nil for a nil time pointer.
func nilTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

func (s *Store) scanUser(row *sql.Row) (*interfaces.User, error) {
	var user interfaces.User
	var createdAt, updatedAt string
	var lastLoginAt sql.NullString

	err := row.Scan(
		&user.ID, &user.Email, &user.Username, &user.PasswordHash, &user.Status,
		&createdAt, &updatedAt, &lastLoginAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, interfaces.ErrNotFound
		}
		return nil, fmt.Errorf("scan user: %w", err)
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		user.CreatedAt = t
	} else {
		slog.Default().Warn("failed to parse created_at timestamp", "value", createdAt, "error", err)
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		user.UpdatedAt = t
	} else {
		slog.Default().Warn("failed to parse updated_at timestamp", "value", updatedAt, "error", err)
	}
	if lastLoginAt.Valid {
		if t, err := time.Parse(time.RFC3339, lastLoginAt.String); err == nil {
			user.LastLoginAt = &t
		} else {
			slog.Default().Warn("failed to parse last_login_at timestamp", "value", lastLoginAt.String, "error", err)
		}
	}

	return &user, nil
}

func (s *Store) scanRole(row *sql.Row) (*interfaces.Role, error) {
	var role interfaces.Role
	var createdAt, updatedAt string

	err := row.Scan(
		&role.ID, &role.Name, &role.DisplayName, &role.Description,
		&createdAt, &updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, interfaces.ErrNotFound
		}
		return nil, fmt.Errorf("scan role: %w", err)
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		role.CreatedAt = t
	} else {
		slog.Default().Warn("failed to parse created_at timestamp", "value", createdAt, "error", err)
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		role.UpdatedAt = t
	} else {
		slog.Default().Warn("failed to parse updated_at timestamp", "value", updatedAt, "error", err)
	}
	return &role, nil
}

func (s *Store) scanRoleRow(rows *sql.Rows) (*interfaces.Role, error) {
	var role interfaces.Role
	var createdAt, updatedAt string

	err := rows.Scan(
		&role.ID, &role.Name, &role.DisplayName, &role.Description,
		&createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan role row: %w", err)
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		role.CreatedAt = t
	} else {
		slog.Default().Warn("failed to parse created_at timestamp", "value", createdAt, "error", err)
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		role.UpdatedAt = t
	} else {
		slog.Default().Warn("failed to parse updated_at timestamp", "value", updatedAt, "error", err)
	}
	return &role, nil
}

func (s *Store) scanPermission(row *sql.Row) (*interfaces.Permission, error) {
	var perm interfaces.Permission

	err := row.Scan(
		&perm.ID, &perm.Name, &perm.Resource, &perm.Action,
		&perm.DisplayName, &perm.Description,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, interfaces.ErrNotFound
		}
		return nil, fmt.Errorf("scan permission: %w", err)
	}
	return &perm, nil
}

func (s *Store) scanPermissionRow(rows *sql.Rows) (*interfaces.Permission, error) {
	var perm interfaces.Permission

	err := rows.Scan(
		&perm.ID, &perm.Name, &perm.Resource, &perm.Action,
		&perm.DisplayName, &perm.Description,
	)
	if err != nil {
		return nil, fmt.Errorf("scan permission row: %w", err)
	}
	return &perm, nil
}

func (s *Store) scanAPIToken(row *sql.Row) (*interfaces.APIToken, error) {
	var token interfaces.APIToken
	var createdAt string
	var expiresAt, lastUsedAt sql.NullString
	var revoked int

	err := row.Scan(
		&token.ID, &token.UserID, &token.Name, &token.TokenHash,
		&createdAt, &expiresAt, &lastUsedAt, &revoked,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, interfaces.ErrNotFound
		}
		return nil, fmt.Errorf("scan api token: %w", err)
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		token.CreatedAt = t
	} else {
		slog.Default().Warn("failed to parse created_at timestamp", "value", createdAt, "error", err)
	}
	if expiresAt.Valid {
		if t, err := time.Parse(time.RFC3339, expiresAt.String); err == nil {
			token.ExpiresAt = &t
		} else {
			slog.Default().Warn("failed to parse expires_at timestamp", "value", expiresAt.String, "error", err)
		}
	}
	if lastUsedAt.Valid {
		if t, err := time.Parse(time.RFC3339, lastUsedAt.String); err == nil {
			token.LastUsedAt = &t
		} else {
			slog.Default().Warn("failed to parse last_used_at timestamp", "value", lastUsedAt.String, "error", err)
		}
	}
	return &token, nil
}

func (s *Store) scanAPITokenRow(rows *sql.Rows) (*interfaces.APIToken, error) {
	var token interfaces.APIToken
	var createdAt string
	var expiresAt, lastUsedAt sql.NullString
	var revoked int

	err := rows.Scan(
		&token.ID, &token.UserID, &token.Name, &token.TokenHash,
		&createdAt, &expiresAt, &lastUsedAt, &revoked,
	)
	if err != nil {
		return nil, fmt.Errorf("scan api token row: %w", err)
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		token.CreatedAt = t
	} else {
		slog.Default().Warn("failed to parse created_at timestamp", "value", createdAt, "error", err)
	}
	if expiresAt.Valid {
		if t, err := time.Parse(time.RFC3339, expiresAt.String); err == nil {
			token.ExpiresAt = &t
		} else {
			slog.Default().Warn("failed to parse expires_at timestamp", "value", expiresAt.String, "error", err)
		}
	}
	if lastUsedAt.Valid {
		if t, err := time.Parse(time.RFC3339, lastUsedAt.String); err == nil {
			token.LastUsedAt = &t
		} else {
			slog.Default().Warn("failed to parse last_used_at timestamp", "value", lastUsedAt.String, "error", err)
		}
	}
	return &token, nil
}
