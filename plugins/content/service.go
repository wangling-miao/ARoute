package content

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/ddl"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

const defaultMaxVersions = 50

var validFieldNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

type Service struct {
	store       *Store
	validator   *FieldValidator
	events      core.EventBus
	logger      *slog.Logger
	maxVersions int
}

func NewService(store *Store, ev core.EventBus, logger *slog.Logger) *Service {
	return &Service{
		store:       store,
		validator:   NewFieldValidator(store),
		events:      ev,
		logger:      logger,
		maxVersions: defaultMaxVersions,
	}
}

func (s *Service) Create(ctx context.Context, contentType string, data map[string]interface{}) (*interfaces.Content, error) {
	ct, err := s.store.GetContentType(ctx, contentType)
	if err != nil {
		return nil, fmt.Errorf("get content type: %w", err)
	}

	s.autoGenerateSlug(ctx, ct, data)

	if err := s.validator.Validate(ctx, ct, data); err != nil {
		return nil, err
	}

	if err := s.checkUniqueFields(ctx, ct, data, ""); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	data["content_type"] = ct.Name
	data["created_at"] = now
	data["updated_at"] = now
	data["deleted_at"] = nil
	data["version"] = 1
	data["created_by"] = ""
	data["updated_by"] = ""

	if _, ok := data["status"]; !ok {
		data["status"] = "draft"
	}

	m2mData := s.extractManyToManyData(ct, data)

	id, err := s.store.CreateContent(ctx, ct.TableName, data)
	if err != nil {
		return nil, fmt.Errorf("create content: %w", err)
	}

	s.insertManyToManyRows(ctx, ct, id, m2mData)

	s.emitEvent(ctx, "content.created", map[string]interface{}{
		"content_type": ct.Name,
		"id":           id,
		"fields":       data,
	})

	return s.buildContent(id, ct.Name, data)
}

func (s *Service) GetByID(ctx context.Context, id string) (*interfaces.Content, error) {
	ct, err := s.findContentTypeForID(ctx, id)
	if err != nil {
		return nil, err
	}

	data, err := s.store.GetContent(ctx, ct.TableName, id)
	if err != nil {
		return nil, err
	}
	return s.mapToContent(data), nil
}

func (s *Service) Update(ctx context.Context, id string, data map[string]interface{}) (*interfaces.Content, error) {
	ct, err := s.findContentTypeForID(ctx, id)
	if err != nil {
		return nil, err
	}

	current, err := s.store.GetContent(ctx, ct.TableName, id)
	if err != nil {
		return nil, err
	}

	merged := s.mergeForUpdate(current, data)

	if err := s.validator.Validate(ctx, ct, merged); err != nil {
		return nil, err
	}

	if err := s.checkUniqueFields(ctx, ct, data, id); err != nil {
		return nil, err
	}

	s.snapshotVersion(ctx, ct.Name, id, current)

	currentVersion := toIntValue(current["version"], 1)
	data["version"] = currentVersion + 1
	data["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	data["content_type"] = ct.Name
	data["updated_by"] = ""

	if status, ok := data["status"]; ok && status == "published" {
		if _, hasPub := current["published_at"]; !hasPub || current["published_at"] == nil {
			data["published_at"] = time.Now().UTC().Format(time.RFC3339)
		}
	}

	m2mData := s.extractManyToManyData(ct, data)

	if err := s.store.UpdateContent(ctx, ct.TableName, id, data); err != nil {
		return nil, fmt.Errorf("update content: %w", err)
	}

	s.updateManyToManyRows(ctx, ct, id, m2mData)

	changedFields := s.computeChangedFields(current, data)

	s.emitEvent(ctx, "content.updated", map[string]interface{}{
		"content_type":   ct.Name,
		"id":             id,
		"fields":         data,
		"changed_fields": changedFields,
	})
	s.emitEvent(ctx, "content.cache.invalidate", map[string]interface{}{
		"content_type": ct.Name,
		"id":           id,
	})

	updated, err := s.store.GetContent(ctx, ct.TableName, id)
	if err != nil {
		return nil, err
	}
	return s.mapToContent(updated), nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	ct, err := s.findContentTypeForID(ctx, id)
	if err != nil {
		return err
	}

	if err := s.store.DeleteContent(ctx, ct.TableName, id); err != nil {
		return err
	}

	s.emitEvent(ctx, "content.deleted", map[string]interface{}{
		"content_type": ct.Name,
		"id":           id,
	})
	s.emitEvent(ctx, "content.cache.invalidate", map[string]interface{}{
		"content_type": ct.Name,
		"id":           id,
	})

	return nil
}

func (s *Service) HardDelete(ctx context.Context, id string) error {
	ct, err := s.findContentTypeForID(ctx, id)
	if err != nil {
		return err
	}

	s.deleteManyToManyRows(ctx, ct, id)

	if err := s.store.DeleteVersionsByContentID(ctx, ct.Name, id); err != nil {
		s.logger.Warn("failed to delete versions", "error", err)
	}

	if err := s.store.HardDeleteContent(ctx, ct.TableName, id); err != nil {
		return err
	}

	s.emitEvent(ctx, "content.deleted", map[string]interface{}{
		"content_type": ct.Name,
		"id":           id,
	})
	s.emitEvent(ctx, "content.cache.invalidate", map[string]interface{}{
		"content_type": ct.Name,
		"id":           id,
	})

	return nil
}

func (s *Service) Restore(ctx context.Context, id string) error {
	ct, err := s.findContentTypeForID(ctx, id)
	if err != nil {
		return err
	}

	return s.store.RestoreContent(ctx, ct.TableName, id)
}

func (s *Service) List(ctx context.Context, contentType string, query *interfaces.ListQuery) (*interfaces.Page, error) {
	ct, err := s.store.GetContentType(ctx, contentType)
	if err != nil {
		return nil, fmt.Errorf("get content type: %w", err)
	}

	if query == nil {
		query = &interfaces.ListQuery{}
	}

	items, total, err := s.store.ListContent(ctx, ct, query)
	if err != nil {
		return nil, err
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}
	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	return &interfaces.Page{
		Data: items,
		Meta: interfaces.PageMeta{
			Total:      total,
			Page:       page,
			PerPage:    perPage,
			TotalPages: totalPages,
			HasPrev:    page > 1,
			HasNext:    page < totalPages,
		},
	}, nil
}

func (s *Service) GetContentType(ctx context.Context, name string) (*interfaces.ContentType, error) {
	return s.store.GetContentType(ctx, name)
}

func (s *Service) CreateContentType(ctx context.Context, ct *interfaces.ContentType) (*interfaces.ContentType, error) {
	if ct.Name == "" {
		return nil, fmt.Errorf("content type name is required: %w", interfaces.ErrValidation)
	}

	if ct.Slug == "" {
		ct.Slug = generateTypeSlug(ct.Name)
	}

	slugExists, err := s.store.ContentTypeSlugExists(ctx, ct.Slug)
	if err != nil {
		return nil, err
	}
	if slugExists {
		return nil, fmt.Errorf("content type slug '%s' already exists: %w", ct.Slug, interfaces.ErrConflict)
	}

	exists, err := s.store.ContentTypeExists(ctx, ct.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("content type '%s' already exists: %w", ct.Name, interfaces.ErrConflict)
	}

	if ct.TableName == "" {
		ct.TableName = "content_" + ct.Name + "s"
	}

	if err := s.store.CreateContentType(ctx, ct); err != nil {
		return nil, err
	}

	if err := s.createContentTable(ctx, ct); err != nil {
		return nil, err
	}

	s.emitEvent(ctx, "content.cache.invalidate_all", map[string]interface{}{
		"content_type": ct.Name,
	})

	return ct, nil
}

func (s *Service) UpdateContentType(ctx context.Context, name string, ct *interfaces.ContentType) (*interfaces.ContentType, error) {
	existing, err := s.store.GetContentType(ctx, name)
	if err != nil {
		return nil, err
	}

	if err := s.protectBuiltInFields(existing, ct); err != nil {
		return nil, err
	}

	ct.Name = name
	if ct.TableName == "" {
		ct.TableName = existing.TableName
	}

	if err := s.alterTableForFieldChanges(ctx, existing, ct); err != nil {
		s.logger.Warn("alter table failed", "error", err)
	}

	s.createJunctionTablesForNewFields(ctx, existing, ct)

	if err := s.store.UpdateContentType(ctx, ct); err != nil {
		return nil, err
	}

	s.emitEvent(ctx, "content.cache.invalidate_all", map[string]interface{}{
		"content_type": name,
	})

	return s.store.GetContentType(ctx, name)
}

func (s *Service) DeleteContentType(ctx context.Context, name string) error {
	ct, err := s.store.GetContentType(ctx, name)
	if err != nil {
		return err
	}

	executor := ddl.NewSQLiteExecutor(s.store.db)
	if err := executor.Execute(ctx, []ddl.DiffOperation{{
		Type:      ddl.OpTableDrop,
		TableName: ct.TableName,
	}}, true); err != nil {
		s.logger.Warn("failed to drop content table", "table", ct.TableName, "error", err)
	}

	if err := s.store.DeleteContentType(ctx, name); err != nil {
		return err
	}

	s.emitEvent(ctx, "content.cache.invalidate_all", map[string]interface{}{
		"content_type": name,
	})

	return nil
}

func (s *Service) createContentTable(ctx context.Context, ct *interfaces.ContentType) error {
	fields := s.buildDDLFields(ct)

	schema := &ddl.Schema{
		Name:      ct.Name,
		TableName: ct.TableName,
		Fields:    fields,
	}

	ops := []ddl.DiffOperation{{
		Type:      ddl.OpTableCreate,
		TableName: ct.TableName,
		Schema:    schema,
	}}

	executor := ddl.NewSQLiteExecutor(s.store.db)
	if err := executor.Execute(ctx, ops, false); err != nil {
		return err
	}

	for _, f := range ct.Fields {
		if f.Type == "relation" && f.RelationConfig != nil &&
			f.RelationConfig.RelationType == "many-to-many" {
			junctionTable := s.junctionTableName(ct.TableName, f)
			if err := s.store.CreateJunctionTable(ctx, junctionTable, ct.Name, f.RelationConfig.TargetContentType); err != nil {
				s.logger.Warn("failed to create junction table", "table", junctionTable, "error", err)
			}
		}
	}

	return nil
}

func (s *Service) buildDDLFields(ct *interfaces.ContentType) []ddl.FieldDefinition {
	fields := []ddl.FieldDefinition{
		{Name: "id", Type: ddl.FieldTypeText, Constraints: &ddl.Constraints{Nullable: false}},
		{Name: "content_type", Type: ddl.FieldTypeText, Constraints: &ddl.Constraints{Nullable: false}},
		{Name: "title", Type: ddl.FieldTypeText},
		{Name: "slug", Type: ddl.FieldTypeText, Constraints: &ddl.Constraints{Unique: true}},
		{Name: "created_by", Type: ddl.FieldTypeText},
		{Name: "updated_by", Type: ddl.FieldTypeText},
		{Name: "status", Type: ddl.FieldTypeText},
		{Name: "published_at", Type: ddl.FieldTypeDatetime},
		{Name: "created_at", Type: ddl.FieldTypeText},
		{Name: "updated_at", Type: ddl.FieldTypeText},
		{Name: "deleted_at", Type: ddl.FieldTypeText},
		{Name: "version", Type: ddl.FieldTypeNumber},
	}

	for _, f := range ct.Fields {
		if s.isSystemField(f.Name) {
			continue
		}
		if f.Type == "relation" && f.RelationConfig != nil &&
			f.RelationConfig.RelationType == "many-to-many" {
			continue
		}
		ddlType := mapFieldTypeToDDL(f.Type)
		fd := ddl.FieldDefinition{
			Name: f.Name,
			Type: ddlType,
		}
		if f.Unique {
			fd.Constraints = &ddl.Constraints{Unique: true}
		}
		if f.Index {
			fd.Index = true
		}
		if f.Type == "relation" && f.RelationConfig != nil {
			targetTable := resolveTargetTable(s.store, f.RelationConfig.TargetContentType)
			if targetTable != ct.TableName {
				fd.ForeignKey = &ddl.ForeignKeyReference{
					Table:    targetTable,
					Column:   "id",
					OnDelete: "SET NULL",
				}
			}
		}
		fields = append(fields, fd)
	}

	return fields
}

func (s *Service) isSystemField(name string) bool {
	systemFields := map[string]bool{
		"id": true, "content_type": true, "title": true, "slug": true,
		"created_by": true, "updated_by": true, "status": true, "published_at": true,
		"created_at": true, "updated_at": true, "deleted_at": true, "version": true,
	}
	return systemFields[name]
}

func resolveTargetTable(store *Store, targetContentType string) string {
	ct, err := store.GetContentType(context.Background(), targetContentType)
	if err == nil && ct.TableName != "" {
		return ct.TableName
	}
	return "content_" + targetContentType + "s"
}

func mapFieldTypeToDDL(fieldType string) ddl.FieldType {
	switch fieldType {
	case "number":
		return ddl.FieldTypeNumber
	case "boolean":
		return ddl.FieldTypeBoolean
	case "date", "datetime":
		return ddl.FieldTypeDatetime
	case "json":
		return ddl.FieldTypeJSON
	case "relation":
		return ddl.FieldTypeRelation
	default:
		return ddl.FieldTypeText
	}
}

func (s *Service) autoGenerateSlug(ctx context.Context, ct *interfaces.ContentType, data map[string]interface{}) {
	if _, hasSlug := data["slug"]; hasSlug {
		return
	}

	var title string
	if t, ok := data["title"]; ok {
		title, _ = t.(string)
	}
	if title == "" {
		if t, ok := data["name"]; ok {
			title, _ = t.(string)
		}
	}
	if title == "" {
		return
	}

	slug, err := s.GenerateUniqueSlug(ctx, ct, title)
	if err != nil {
		s.logger.Warn("failed to generate unique slug", "error", err)
		slug = GenerateSlug(title)
	}
	data["slug"] = slug

	if _, hasTitle := data["title"]; !hasTitle {
		data["title"] = title
	}
}

func (s *Service) checkUniqueFields(ctx context.Context, ct *interfaces.ContentType, data map[string]interface{}, excludeID string) error {
	verrs := interfaces.NewValidationErrors()

	for _, field := range ct.Fields {
		if !field.Unique {
			continue
		}
		val, ok := data[field.Name]
		if !ok || val == nil {
			continue
		}
		strVal, ok := val.(string)
		if !ok || strVal == "" {
			continue
		}

		isUnique, err := s.store.CheckUnique(ctx, ct.TableName, field.Name, strVal, excludeID)
		if err != nil {
			s.logger.Warn("failed to check unique", "field", field.Name, "error", err)
			continue
		}
		if !isUnique {
			verrs.Add(field.Name, fmt.Sprintf("field '%s' value must be unique", field.Name), "unique")
		}
	}

	if verrs.HasErrors() {
		return verrs
	}
	return nil
}

func (s *Service) snapshotVersion(ctx context.Context, contentType, contentID string, current map[string]interface{}) {
	ver := toIntValue(current["version"], 1)
	if err := s.store.CreateVersion(ctx, contentType, contentID, ver, current, ""); err != nil {
		s.logger.Warn("failed to create version snapshot", "error", err)
	}
	if err := s.store.PruneVersions(ctx, contentType, contentID, s.maxVersions); err != nil {
		s.logger.Warn("failed to prune versions", "error", err)
	}
}

func (s *Service) findContentTypeForID(ctx context.Context, id string) (*interfaces.ContentType, error) {
	types, err := s.store.ListContentTypes(ctx)
	if err != nil {
		return nil, err
	}

	for _, ct := range types {
		_, err := s.store.GetContent(ctx, ct.TableName, id)
		if err == nil {
			return ct, nil
		}
	}

	return nil, interfaces.ErrNotFound
}

func (s *Service) buildContent(id, contentType string, data map[string]interface{}) (*interfaces.Content, error) {
	c := &interfaces.Content{
		ID:          id,
		ContentType: contentType,
		Data:        data,
		Version:     toIntValue(data["version"], 1),
	}

	if t, ok := data["title"]; ok {
		c.Title, _ = t.(string)
	}
	if t, ok := data["slug"]; ok {
		c.Slug, _ = t.(string)
	}
	if t, ok := data["created_by"]; ok {
		c.AuthorID, _ = t.(string)
	}
	if t, ok := data["status"]; ok {
		c.Status, _ = t.(string)
	}

	if t, ok := data["published_at"]; ok && t != nil {
		if sv, ok := t.(string); ok && sv != "" {
			pt, err := time.Parse(time.RFC3339, sv)
			if err == nil {
				c.PublishedAt = &pt
			}
		}
	}

	if t, ok := data["created_at"]; ok {
		if sv, ok := t.(string); ok {
			c.CreatedAt, _ = time.Parse(time.RFC3339, sv)
		}
	}
	if t, ok := data["updated_at"]; ok {
		if sv, ok := t.(string); ok {
			c.UpdatedAt, _ = time.Parse(time.RFC3339, sv)
		}
	}

	return c, nil
}

func (s *Service) mapToContent(data map[string]interface{}) *interfaces.Content {
	c := &interfaces.Content{
		Data:    data,
		Version: toIntValue(data["version"], 1),
	}

	c.ID, _ = data["id"].(string)
	c.ContentType, _ = data["content_type"].(string)
	c.Title, _ = data["title"].(string)
	c.Slug, _ = data["slug"].(string)
	c.AuthorID, _ = data["created_by"].(string)
	c.Status, _ = data["status"].(string)

	if t, ok := data["published_at"]; ok && t != nil {
		if sv, ok := t.(string); ok && sv != "" {
			pt, err := time.Parse(time.RFC3339, sv)
			if err == nil {
				c.PublishedAt = &pt
			}
		}
	}

	if t, ok := data["created_at"]; ok {
		if sv, ok := t.(string); ok {
			c.CreatedAt, _ = time.Parse(time.RFC3339, sv)
		}
	}
	if t, ok := data["updated_at"]; ok {
		if sv, ok := t.(string); ok {
			c.UpdatedAt, _ = time.Parse(time.RFC3339, sv)
		}
	}

	return c
}

func (s *Service) emitEvent(ctx context.Context, topic string, data map[string]interface{}) {
	if s.events == nil {
		return
	}
	s.events.Emit(ctx, events.Event{
		Topic: topic,
		Data:  data,
	})
}

func (s *Service) mergeForUpdate(current, updates map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{}, len(current))
	for k, v := range current {
		merged[k] = v
	}
	for k, v := range updates {
		merged[k] = v
	}
	return merged
}

func (s *Service) computeChangedFields(oldData, newData map[string]interface{}) []string {
	var changed []string
	for k, v := range newData {
		if oldVal, ok := oldData[k]; !ok || fmt.Sprintf("%v", oldVal) != fmt.Sprintf("%v", v) {
			changed = append(changed, k)
		}
	}
	return changed
}

func toIntValue(v interface{}, defaultVal int) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	case string:
		return defaultVal
	}
	return defaultVal
}

func (s *Service) alterTableForFieldChanges(ctx context.Context, oldCT, newCT *interfaces.ContentType) error {
	oldFields := fieldMap(oldCT.Fields)
	newFields := fieldMap(newCT.Fields)

	tableName := oldCT.TableName
	if tableName == "" {
		tableName = newCT.TableName
	}

	for name, field := range newFields {
		if _, exists := oldFields[name]; !exists {
			ddlType := mapFieldTypeToDDL(field.Type)
			sqlType := ddlTypeToSQL(ddlType)
			alterSQL := fmt.Sprintf("ALTER TABLE \"%s\" ADD COLUMN \"%s\" %s", tableName, name, sqlType)
			if _, err := s.store.db.Exec(ctx, alterSQL); err != nil {
				s.logger.Warn("failed to add column", "table", tableName, "column", name, "error", err)
			}
		}
	}

	for name := range oldFields {
		if _, exists := newFields[name]; !exists {
			alterSQL := fmt.Sprintf("ALTER TABLE \"%s\" DROP COLUMN \"%s\"", tableName, name)
			if _, err := s.store.db.Exec(ctx, alterSQL); err != nil {
				s.logger.Warn("failed to drop column", "table", tableName, "column", name, "error", err)
			}
		}
	}

	return nil
}

func fieldMap(fields []interfaces.Field) map[string]interfaces.Field {
	m := make(map[string]interfaces.Field, len(fields))
	for _, f := range fields {
		m[f.Name] = f
	}
	return m
}

func ddlTypeToSQL(ft ddl.FieldType) string {
	switch ft {
	case ddl.FieldTypeNumber:
		return "REAL"
	case ddl.FieldTypeBoolean:
		return "INTEGER"
	case ddl.FieldTypeDatetime:
		return "TEXT"
	case ddl.FieldTypeJSON:
		return "TEXT"
	default:
		return "TEXT"
	}
}

func (s *Service) protectBuiltInFields(existing, updated *interfaces.ContentType) error {
	if existing.Name != "page" && existing.Name != "post" &&
		existing.Name != "category" && existing.Name != "tag" {
		return nil
	}

	oldFields := fieldMap(existing.Fields)
	newFields := fieldMap(updated.Fields)

	for name := range oldFields {
		if _, exists := newFields[name]; !exists {
			return fmt.Errorf("cannot remove core fields from built-in content types: %w", interfaces.ErrValidation)
		}
	}
	return nil
}

func validateFieldName(name string) error {
	if !validFieldNameRegex.MatchString(name) {
		return fmt.Errorf("field name '%s' contains invalid characters: only alphanumeric and underscore allowed: %w",
			name, interfaces.ErrValidation)
	}
	return nil
}

func (s *Service) junctionTableName(sourceTable string, field interfaces.Field) string {
	if field.RelationConfig != nil && field.RelationConfig.ThroughTable != "" {
		return field.RelationConfig.ThroughTable
	}
	target := field.RelationConfig.TargetContentType
	return fmt.Sprintf("%s_%s", sourceTable, target)
}

func (s *Service) extractManyToManyData(ct *interfaces.ContentType, data map[string]interface{}) map[string]interface{} {
	m2mData := make(map[string]interface{})
	for _, f := range ct.Fields {
		if f.Type == "relation" && f.RelationConfig != nil &&
			f.RelationConfig.RelationType == "many-to-many" {
			if val, ok := data[f.Name]; ok {
				m2mData[f.Name] = val
				delete(data, f.Name)
			}
		}
	}
	return m2mData
}

func (s *Service) insertManyToManyRows(ctx context.Context, ct *interfaces.ContentType, contentID string, m2mData map[string]interface{}) {
	for _, f := range ct.Fields {
		if f.Type != "relation" || f.RelationConfig == nil ||
			f.RelationConfig.RelationType != "many-to-many" {
			continue
		}
		val, ok := m2mData[f.Name]
		if !ok {
			continue
		}
		ids := toSliceOfStrings(val)
		if len(ids) == 0 {
			continue
		}
		junctionTable := s.junctionTableName(ct.TableName, f)
		if err := s.store.InsertJunctionRows(ctx, junctionTable, ct.Name, f.RelationConfig.TargetContentType, contentID, ids); err != nil {
			s.logger.Warn("failed to insert junction rows", "table", junctionTable, "error", err)
		}
	}
}

func (s *Service) updateManyToManyRows(ctx context.Context, ct *interfaces.ContentType, contentID string, m2mData map[string]interface{}) {
	for _, f := range ct.Fields {
		if f.Type != "relation" || f.RelationConfig == nil ||
			f.RelationConfig.RelationType != "many-to-many" {
			continue
		}
		junctionTable := s.junctionTableName(ct.TableName, f)
		if err := s.store.DeleteJunctionRows(ctx, junctionTable, ct.Name, contentID); err != nil {
			s.logger.Warn("failed to delete old junction rows", "error", err)
		}
		val, ok := m2mData[f.Name]
		if !ok {
			continue
		}
		ids := toSliceOfStrings(val)
		if len(ids) == 0 {
			continue
		}
		if err := s.store.InsertJunctionRows(ctx, junctionTable, ct.Name, f.RelationConfig.TargetContentType, contentID, ids); err != nil {
			s.logger.Warn("failed to insert new junction rows", "error", err)
		}
	}
}

func (s *Service) deleteManyToManyRows(ctx context.Context, ct *interfaces.ContentType, contentID string) {
	for _, f := range ct.Fields {
		if f.Type != "relation" || f.RelationConfig == nil ||
			f.RelationConfig.RelationType != "many-to-many" {
			continue
		}
		junctionTable := s.junctionTableName(ct.TableName, f)
		if err := s.store.DeleteJunctionRows(ctx, junctionTable, ct.Name, contentID); err != nil {
			s.logger.Warn("failed to delete junction rows", "error", err)
		}
	}
}

func (s *Service) createJunctionTablesForNewFields(ctx context.Context, oldCT, newCT *interfaces.ContentType) {
	oldFields := fieldMap(oldCT.Fields)
	for _, f := range newCT.Fields {
		if f.Type == "relation" && f.RelationConfig != nil &&
			f.RelationConfig.RelationType == "many-to-many" {
			if _, existed := oldFields[f.Name]; !existed {
				junctionTable := s.junctionTableName(newCT.TableName, f)
				if err := s.store.CreateJunctionTable(ctx, junctionTable, newCT.Name, f.RelationConfig.TargetContentType); err != nil {
					s.logger.Warn("failed to create junction table", "error", err)
				}
			}
		}
	}
}

func toSliceOfStrings(val interface{}) []string {
	switch v := val.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if sv, ok := item.(string); ok {
				result = append(result, sv)
			} else {
				result = append(result, fmt.Sprintf("%v", item))
			}
		}
		return result
	default:
		return nil
	}
}

func generateTypeSlug(name string) string {
	s := strings.ToLower(name)
	if !strings.HasSuffix(s, "s") {
		s = s + "s"
	}
	return GenerateSlug(s)
}

var multiDashRegex = regexp.MustCompile(`-{2,}`)

func GenerateSlugUnicode(title string) string {
	s := strings.ToLower(title)
	s = strings.ReplaceAll(s, " ", "-")

	var b strings.Builder
	for _, r := range s {
		if r < 128 {
			b.WriteRune(r)
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if unicode.IsSpace(r) {
			b.WriteRune('-')
		}
	}
	s = b.String()

	s = multiDashRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	return s
}
