package content

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

var (
	// sqlIdentifierRegex validates SQL identifiers for safe interpolation
	// in contexts where parameterization is not supported (table names, column names).
	sqlIdentifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

func isValidSortColumn(name string) bool {
	return sqlIdentifierRegex.MatchString(name)
}

func validateTableName(name string) error {
	if !sqlIdentifierRegex.MatchString(name) {
		return fmt.Errorf("invalid table name: %q", name)
	}
	return nil
}

func validateIdentifier(name string) error {
	if !sqlIdentifierRegex.MatchString(name) {
		return fmt.Errorf("invalid identifier: %q", name)
	}
	return nil
}

type Store struct {
	db       interfaces.DatabaseService
	driver   string
}

func NewStore(db interfaces.DatabaseService, driver string) *Store {
	return &Store{db: db, driver: driver}
}

func (s *Store) MigratePartialUniqueIndex(ctx context.Context, tableName string) {
	if err := validateTableName(tableName); err != nil {
		return
	}
	idxName := fmt.Sprintf("idx_%s_slug_unique", tableName)
	_, _ = s.db.Exec(ctx, fmt.Sprintf(
		"CREATE UNIQUE INDEX IF NOT EXISTS \"%s\" ON \"%s\" (\"slug\") WHERE \"deleted_at\" IS NULL",
		idxName, tableName))
}

func (s *Store) CreateTables(ctx context.Context) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS _content_types (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			slug TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			fields_json TEXT NOT NULL,
			table_name TEXT NOT NULL,
			is_builtin INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
			updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_content_types_slug ON _content_types(slug)`,
		`CREATE TABLE IF NOT EXISTS _content_versions (
			id TEXT PRIMARY KEY,
			content_type TEXT NOT NULL,
			content_id TEXT NOT NULL,
			version_number INTEGER NOT NULL,
			data_json TEXT NOT NULL,
			modified_by TEXT,
			modified_at TEXT,
			created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_content_versions_ct_id ON _content_versions(content_type, content_id)`,
		`CREATE INDEX IF NOT EXISTS idx_content_versions_ct_id_ver ON _content_versions(content_type, content_id, version_number)`,
	}

	for _, table := range tables {
		if _, err := s.db.Exec(ctx, table); err != nil {
			return fmt.Errorf("create content table: %w", err)
		}
	}
	return nil
}

func newUUID() string {
	return uuid.New().String()
}

func nilTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

func (s *Store) CreateContentType(ctx context.Context, ct *interfaces.ContentType) error {
	fieldsJSON, err := json.Marshal(ct.Fields)
	if err != nil {
		return fmt.Errorf("marshal fields: %w", err)
	}

	if ct.ID == "" {
		ct.ID = newUUID()
	}

	now := time.Now().UTC().Format(time.RFC3339)

	slug := ct.Slug
	if slug == "" {
		slug = ct.Name
	}

	_, err = s.db.Exec(ctx,
		`INSERT INTO _content_types (id, name, slug, display_name, description, fields_json, table_name, is_builtin, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ct.ID, ct.Name, slug, ct.DisplayName, ct.Description, string(fieldsJSON),
		ct.TableName, 0, now, now,
	)
	if err != nil {
		return fmt.Errorf("insert content type: %w", err)
	}
	return nil
}

func (s *Store) GetContentType(ctx context.Context, name string) (*interfaces.ContentType, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, name, slug, display_name, description, fields_json, table_name, is_builtin, created_at, updated_at
		 FROM _content_types WHERE name = ?`, name,
	)
	return s.scanContentType(row)
}

func (s *Store) ListContentTypes(ctx context.Context) ([]*interfaces.ContentType, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, name, slug, display_name, description, fields_json, table_name, is_builtin, created_at, updated_at
		 FROM _content_types ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("list content types: %w", err)
	}
	defer rows.Close()

	var types []*interfaces.ContentType
	for rows.Next() {
		ct, err := s.scanContentTypeRow(rows)
		if err != nil {
			return nil, err
		}
		types = append(types, ct)
	}
	return types, rows.Err()
}

func (s *Store) UpdateContentType(ctx context.Context, ct *interfaces.ContentType) error {
	fieldsJSON, err := json.Marshal(ct.Fields)
	if err != nil {
		return fmt.Errorf("marshal fields: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	slug := ct.Slug
	if slug == "" {
		slug = ct.Name
	}

	_, err = s.db.Exec(ctx,
		`UPDATE _content_types SET display_name = ?, description = ?, fields_json = ?, table_name = ?, slug = ?, updated_at = ?
		 WHERE name = ?`,
		ct.DisplayName, ct.Description, string(fieldsJSON), ct.TableName, slug, now, ct.Name,
	)
	if err != nil {
		return fmt.Errorf("update content type: %w", err)
	}
	return nil
}

func (s *Store) DeleteContentType(ctx context.Context, name string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM _content_types WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete content type: %w", err)
	}
	return nil
}

func (s *Store) ContentTypeExists(ctx context.Context, name string) (bool, error) {
	row := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM _content_types WHERE name = ?`, name)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, fmt.Errorf("check content type exists: %w", err)
	}
	return count > 0, nil
}

func (s *Store) ContentTypeSlugExists(ctx context.Context, slug string) (bool, error) {
	if slug == "" {
		return false, nil
	}
	row := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM _content_types WHERE slug = ?`, slug)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, fmt.Errorf("check content type slug exists: %w", err)
	}
	return count > 0, nil
}

func (s *Store) CreateContent(ctx context.Context, tableName string, data map[string]interface{}) (string, error) {
	if err := validateTableName(tableName); err != nil {
		return "", fmt.Errorf("create content: %w", err)
	}
	id := newUUID()
	data["id"] = id

	cols, vals := s.columnsAndValues(data)
	placeholders := make([]string, len(cols))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	query := fmt.Sprintf("INSERT INTO \"%s\" (%s) VALUES (%s)",
		tableName,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	_, err := s.db.Exec(ctx, query, vals...)
	if err != nil {
		return "", fmt.Errorf("insert content into %s: %w", tableName, err)
	}
	return id, nil
}

func (s *Store) GetContent(ctx context.Context, tableName, id string) (map[string]interface{}, error) {
	if err := validateTableName(tableName); err != nil {
		return nil, fmt.Errorf("get content: %w", err)
	}
	query := fmt.Sprintf("SELECT * FROM \"%s\" WHERE id = ? AND deleted_at IS NULL", tableName)
	rows, err := s.db.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("get content: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, interfaces.ErrNotFound
	}

	result, err := s.scanMap(rows)
	if err != nil {
		return nil, err
	}
	return result, rows.Err()
}

func (s *Store) UpdateContent(ctx context.Context, tableName, id string, data map[string]interface{}) error {
	if err := validateTableName(tableName); err != nil {
		return fmt.Errorf("update content: %w", err)
	}
	delete(data, "id")
	delete(data, "created_at")
	delete(data, "content_type")

	cols, vals := s.columnsAndValues(data)
	if len(cols) == 0 {
		return nil
	}

	setClauses := make([]string, len(cols))
	for i, col := range cols {
		setClauses[i] = fmt.Sprintf("\"%s\" = ?", col)
	}

	vals = append(vals, id)
	query := fmt.Sprintf("UPDATE \"%s\" SET %s WHERE id = ? AND deleted_at IS NULL",
		tableName,
		strings.Join(setClauses, ", "),
	)

	res, err := s.db.Exec(ctx, query, vals...)
	if err != nil {
		return fmt.Errorf("update content: %w", err)
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		return interfaces.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteContent(ctx context.Context, tableName, id string) error {
	if err := validateTableName(tableName); err != nil {
		return fmt.Errorf("delete content: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(ctx,
		fmt.Sprintf("UPDATE \"%s\" SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL", tableName),
		now, id,
	)
	if err != nil {
		return fmt.Errorf("delete content: %w", err)
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		return interfaces.ErrNotFound
	}
	return nil
}

func (s *Store) HardDeleteContent(ctx context.Context, tableName, id string) error {
	if err := validateTableName(tableName); err != nil {
		return fmt.Errorf("hard delete content: %w", err)
	}
	res, err := s.db.Exec(ctx,
		fmt.Sprintf("DELETE FROM \"%s\" WHERE id = ?", tableName),
		id,
	)
	if err != nil {
		return fmt.Errorf("hard delete content: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return interfaces.ErrNotFound
	}
	return nil
}

func (s *Store) RestoreContent(ctx context.Context, tableName, id string) error {
	if err := validateTableName(tableName); err != nil {
		return fmt.Errorf("restore content: %w", err)
	}
	res, err := s.db.Exec(ctx,
		fmt.Sprintf("UPDATE \"%s\" SET deleted_at = NULL WHERE id = ?", tableName),
		id,
	)
	if err != nil {
		return fmt.Errorf("restore content: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return interfaces.ErrNotFound
	}
	return nil
}

func (s *Store) ListContent(ctx context.Context, ct *interfaces.ContentType, query *interfaces.ListQuery) ([]map[string]interface{}, int64, error) {
	var whereClauses []string
	var args []interface{}

	includeDeleted := false
	if query != nil && len(query.Filters) > 0 {
		if _, ok := query.Filters["_include_deleted"]; ok {
			includeDeleted = true
			delete(query.Filters, "_include_deleted")
		}
	}

	if !includeDeleted {
		whereClauses = append(whereClauses, "deleted_at IS NULL")
	}

	if query != nil && len(query.Filters) > 0 {
		for field, value := range query.Filters {
			clause, filterArgs := parseFilter(field, value)
			whereClauses = append(whereClauses, clause)
			args = append(args, filterArgs...)
		}
	}

	// Future-date filtering for published status
	if query != nil && len(query.Filters) > 0 {
		if statusVal, ok := query.Filters["status"]; ok {
			if sv, _ := statusVal.(string); sv == "published" {
				nowFn := "datetime('now')"
				if s.driver == "postgres" {
					nowFn = "NOW()"
				}
				whereClauses = append(whereClauses, fmt.Sprintf("(published_at IS NULL OR published_at <= %s)", nowFn))
			}
		}
	}

	whereStr := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM \"%s\" WHERE %s", ct.TableName, whereStr)
	var total int64
	row := s.db.QueryRow(ctx, countQuery, args...)
	if err := row.Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count content: %w", err)
	}

	sortCol := "created_at"
	sortOrder := "DESC"
	if query != nil {
		if query.Sort != "" && isValidSortColumn(query.Sort) {
			sortCol = query.Sort
		}
		if strings.EqualFold(query.Order, "asc") {
			sortOrder = "ASC"
		}
	}

	page := 1
	perPage := 20
	if query != nil {
		if query.Page > 0 {
			page = query.Page
		}
		if query.PerPage > 0 && query.PerPage <= 100 {
			perPage = query.PerPage
		}
		if query.PerPage > 100 {
			perPage = 100
		}
	}
	offset := (page - 1) * perPage

	selectQuery := fmt.Sprintf("SELECT * FROM \"%s\" WHERE %s ORDER BY \"%s\" %s LIMIT ? OFFSET ?",
		ct.TableName, whereStr, sortCol, sortOrder,
	)
	args = append(args, perPage, offset)

	rows, err := s.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list content: %w", err)
	}
	defer rows.Close()

	var items []map[string]interface{}
	for rows.Next() {
		item, err := s.scanMap(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}

	return items, total, rows.Err()
}

func parseFilter(field string, value interface{}) (string, []interface{}) {
	var op string
	baseField := field

	switch {
	case strings.HasSuffix(field, "_gte"):
		op = ">="
		baseField = strings.TrimSuffix(field, "_gte")
	case strings.HasSuffix(field, "_lte"):
		op = "<="
		baseField = strings.TrimSuffix(field, "_lte")
	case strings.HasSuffix(field, "_gt"):
		op = ">"
		baseField = strings.TrimSuffix(field, "_gt")
	case strings.HasSuffix(field, "_lt"):
		op = "<"
		baseField = strings.TrimSuffix(field, "_lt")
	case strings.HasSuffix(field, "_contains"):
		op = "LIKE"
		baseField = strings.TrimSuffix(field, "_contains")
		return fmt.Sprintf("\"%s\" LIKE ?", baseField), []interface{}{fmt.Sprintf("%%%v%%", value)}
	case strings.HasSuffix(field, "_ne"):
		op = "!="
		baseField = strings.TrimSuffix(field, "_ne")
	default:
		op = "="
	}

	return fmt.Sprintf("\"%s\" %s ?", baseField, op), []interface{}{value}
}

func (s *Store) CreateVersion(ctx context.Context, contentType, contentID string, versionNumber int, data map[string]interface{}, modifiedBy string) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal version data: %w", err)
	}

	id := newUUID()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = s.db.Exec(ctx,
		`INSERT INTO _content_versions (id, content_type, content_id, version_number, data_json, modified_by, modified_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, contentType, contentID, versionNumber, string(dataJSON), modifiedBy, now, now,
	)
	if err != nil {
		return fmt.Errorf("create version: %w", err)
	}
	return nil
}

func (s *Store) GetVersions(ctx context.Context, contentType, contentID string, limit, offset int) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, content_type, content_id, version_number, modified_by, modified_at, created_at
		 FROM _content_versions WHERE content_type = ? AND content_id = ?
		 ORDER BY version_number DESC LIMIT ? OFFSET ?`,
		contentType, contentID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get versions: %w", err)
	}
	defer rows.Close()

	var versions []map[string]interface{}
	for rows.Next() {
		m, err := s.scanMap(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, m)
	}
	return versions, rows.Err()
}

func (s *Store) GetVersion(ctx context.Context, contentType, contentID string, versionNumber int) (map[string]interface{}, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, content_type, content_id, version_number, data_json, modified_by, modified_at, created_at
		 FROM _content_versions WHERE content_type = ? AND content_id = ? AND version_number = ?`,
		contentType, contentID, versionNumber,
	)

	var id, ct, cid, dataJSON string
	var verNum int
	var modifiedBy, modifiedAt, createdAt sql.NullString

	err := row.Scan(&id, &ct, &cid, &verNum, &dataJSON, &modifiedBy, &modifiedAt, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, interfaces.ErrNotFound
		}
		return nil, fmt.Errorf("get version: %w", err)
	}

	var data interface{}
	if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
		return nil, fmt.Errorf("unmarshal version data: %w", err)
	}

	result := map[string]interface{}{
		"id":             id,
		"content_type":   ct,
		"content_id":     cid,
		"version_number": verNum,
		"data":           data,
	}
	if modifiedBy.Valid {
		result["modified_by"] = modifiedBy.String
	}
	if modifiedAt.Valid {
		result["modified_at"] = modifiedAt.String
	}
	if createdAt.Valid {
		result["created_at"] = createdAt.String
	}
	return result, nil
}

func (s *Store) DeleteVersionsByContentID(ctx context.Context, contentType, contentID string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM _content_versions WHERE content_type = ? AND content_id = ?`,
		contentType, contentID,
	)
	if err != nil {
		return fmt.Errorf("delete versions: %w", err)
	}
	return nil
}

func (s *Store) PruneVersions(ctx context.Context, contentType, contentID string, maxVersions int) error {
	row := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM _content_versions WHERE content_type = ? AND content_id = ?`,
		contentType, contentID,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		return fmt.Errorf("count versions: %w", err)
	}

	if count <= maxVersions {
		return nil
	}

	deleteCount := count - maxVersions
	_, err := s.db.Exec(ctx,
		`DELETE FROM _content_versions WHERE id IN (
			SELECT id FROM _content_versions
			WHERE content_type = ? AND content_id = ?
			ORDER BY version_number ASC LIMIT ?
		)`,
		contentType, contentID, deleteCount,
	)
	if err != nil {
		return fmt.Errorf("prune versions: %w", err)
	}
	return nil
}

func (s *Store) CheckUnique(ctx context.Context, tableName, fieldName, value, excludeID string) (bool, error) {
	if err := validateTableName(tableName); err != nil {
		return false, fmt.Errorf("check unique: %w", err)
	}
	if err := validateIdentifier(fieldName); err != nil {
		return false, fmt.Errorf("check unique: %w", err)
	}
	query := fmt.Sprintf("SELECT COUNT(*) FROM \"%s\" WHERE \"%s\" = ? AND deleted_at IS NULL", tableName, fieldName)
	args := []interface{}{value}

	if excludeID != "" {
		query += " AND id != ?"
		args = append(args, excludeID)
	}

	row := s.db.QueryRow(ctx, query, args...)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, fmt.Errorf("check unique: %w", err)
	}
	return count == 0, nil
}

func (s *Store) GetNextSlug(ctx context.Context, tableName, baseSlug string) (string, error) {
	if err := validateTableName(tableName); err != nil {
		return "", fmt.Errorf("get next slug: %w", err)
	}
	likePattern := baseSlug + "%"
	query := fmt.Sprintf(
		"SELECT slug FROM \"%s\" WHERE slug LIKE ? AND deleted_at IS NULL ORDER BY slug",
		tableName,
	)
	rows, err := s.db.Query(ctx, query, likePattern)
	if err != nil {
		return "", fmt.Errorf("get next slug: %w", err)
	}
	defer rows.Close()

	existing := make(map[string]bool)
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return "", fmt.Errorf("scan slug: %w", err)
		}
		existing[slug] = true
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	if !existing[baseSlug] {
		return baseSlug, nil
	}

	for i := 2; i <= 1000; i++ {
		candidate := fmt.Sprintf("%s-%d", baseSlug, i)
		if !existing[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find unique slug for %s", baseSlug)
}

func (s *Store) CreateJunctionTable(ctx context.Context, tableName, sourceSingular, targetSingular string) error {
	if err := validateTableName(tableName); err != nil {
		return fmt.Errorf("create junction table: %w", err)
	}
	if err := validateIdentifier(sourceSingular); err != nil {
		return fmt.Errorf("create junction table: invalid source: %w", err)
	}
	if err := validateIdentifier(targetSingular); err != nil {
		return fmt.Errorf("create junction table: invalid target: %w", err)
	}
	sql := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (
		id TEXT PRIMARY KEY,
		%s_id TEXT NOT NULL,
		%s_id TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
	)`, tableName, sourceSingular, targetSingular)

	if _, err := s.db.Exec(ctx, sql); err != nil {
		return fmt.Errorf("create junction table %s: %w", tableName, err)
	}
	return nil
}

func (s *Store) InsertJunctionRows(ctx context.Context, tableName, sourceSingular, targetSingular, sourceID string, targetIDs []string) error {
	if err := validateTableName(tableName); err != nil {
		return fmt.Errorf("insert junction rows: %w", err)
	}
	if err := validateIdentifier(sourceSingular); err != nil {
		return fmt.Errorf("insert junction rows: %w", err)
	}
	if err := validateIdentifier(targetSingular); err != nil {
		return fmt.Errorf("insert junction rows: %w", err)
	}
	for _, targetID := range targetIDs {
		id := newUUID()
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := s.db.Exec(ctx,
			fmt.Sprintf(`INSERT INTO "%s" (id, %s_id, %s_id, created_at) VALUES (?, ?, ?, ?)`,
				tableName, sourceSingular, targetSingular),
			id, sourceID, targetID, now,
		)
		if err != nil {
			return fmt.Errorf("insert junction row: %w", err)
		}
	}
	return nil
}

func (s *Store) DeleteJunctionRows(ctx context.Context, tableName, sourceSingular, sourceID string) error {
	if err := validateTableName(tableName); err != nil {
		return fmt.Errorf("delete junction rows: %w", err)
	}
	if err := validateIdentifier(sourceSingular); err != nil {
		return fmt.Errorf("delete junction rows: %w", err)
	}
	_, err := s.db.Exec(ctx,
		fmt.Sprintf(`DELETE FROM "%s" WHERE %s_id = ?`, tableName, sourceSingular),
		sourceID,
	)
	if err != nil {
		return fmt.Errorf("delete junction rows: %w", err)
	}
	return nil
}

func (s *Store) GetJunctionIDs(ctx context.Context, tableName, sourceSingular, targetSingular, sourceID string) ([]string, error) {
	if err := validateTableName(tableName); err != nil {
		return nil, fmt.Errorf("get junction ids: %w", err)
	}
	if err := validateIdentifier(sourceSingular); err != nil {
		return nil, fmt.Errorf("get junction ids: %w", err)
	}
	if err := validateIdentifier(targetSingular); err != nil {
		return nil, fmt.Errorf("get junction ids: %w", err)
	}
	rows, err := s.db.Query(ctx,
		fmt.Sprintf(`SELECT %s_id FROM "%s" WHERE %s_id = ?`, targetSingular, tableName, sourceSingular),
		sourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("get junction ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) scanContentType(row *sql.Row) (*interfaces.ContentType, error) {
	var ct interfaces.ContentType
	var fieldsJSON string
	var isBuiltin int
	var createdAt, updatedAt string

	err := row.Scan(
		&ct.ID, &ct.Name, &ct.Slug, &ct.DisplayName, &ct.Description,
		&fieldsJSON, &ct.TableName, &isBuiltin, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, interfaces.ErrNotFound
		}
		return nil, fmt.Errorf("scan content type: %w", err)
	}

	if err := json.Unmarshal([]byte(fieldsJSON), &ct.Fields); err != nil {
		return nil, fmt.Errorf("unmarshal fields: %w", err)
	}

	ct.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	ct.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &ct, nil
}

func (s *Store) scanContentTypeRow(rows *sql.Rows) (*interfaces.ContentType, error) {
	var ct interfaces.ContentType
	var fieldsJSON string
	var isBuiltin int
	var createdAt, updatedAt string

	err := rows.Scan(
		&ct.ID, &ct.Name, &ct.Slug, &ct.DisplayName, &ct.Description,
		&fieldsJSON, &ct.TableName, &isBuiltin, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan content type row: %w", err)
	}

	if err := json.Unmarshal([]byte(fieldsJSON), &ct.Fields); err != nil {
		return nil, fmt.Errorf("unmarshal fields: %w", err)
	}

	ct.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	ct.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &ct, nil
}

func (s *Store) scanMap(rows *sql.Rows) (map[string]interface{}, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("get columns: %w", err)
	}

	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, fmt.Errorf("scan row: %w", err)
	}

	result := make(map[string]interface{}, len(cols))
	for i, col := range cols {
		val := values[i]
		switch v := val.(type) {
		case []byte:
			result[col] = string(v)
		default:
			result[col] = v
		}
	}
	return result, nil
}

func (s *Store) columnsAndValues(data map[string]interface{}) ([]string, []interface{}) {
	cols := make([]string, 0, len(data))
	vals := make([]interface{}, 0, len(data))

	for col := range data {
		cols = append(cols, col)
	}
	sort.Strings(cols)
	for _, col := range cols {
		vals = append(vals, data[col])
	}
	return cols, vals
}
