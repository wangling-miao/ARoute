package ddl

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/wangling-miao/aroute/sdk/interfaces"
	_ "modernc.org/sqlite"
)

// migrationTestDB implements interfaces.DatabaseService backed by an in-memory SQLite database.
// Named differently from testDB in registry_test.go to avoid conflict within the same package.
type migrationTestDB struct {
	db *sql.DB
}

func newMigrationTestDB(t *testing.T) *migrationTestDB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	return &migrationTestDB{db: db}
}

func (tdb *migrationTestDB) Close() error {
	return tdb.db.Close()
}

func (tdb *migrationTestDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return tdb.db.ExecContext(ctx, query, args...)
}

func (tdb *migrationTestDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return tdb.db.QueryContext(ctx, query, args...)
}

func (tdb *migrationTestDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return tdb.db.QueryRowContext(ctx, query, args...)
}

func (tdb *migrationTestDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return tdb.db.BeginTx(ctx, opts)
}

func (tdb *migrationTestDB) Ping(ctx context.Context) error {
	return tdb.db.PingContext(ctx)
}

func (tdb *migrationTestDB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return tdb.db.PrepareContext(ctx, query)
}

func (tdb *migrationTestDB) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	return nil, fmt.Errorf("not implemented")
}

func setupTracker(t *testing.T) (*MigrationTracker, *migrationTestDB) {
	t.Helper()
	tdb := newMigrationTestDB(t)
	tracker := NewMigrationTracker(tdb)
	if err := tracker.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	return tracker, tdb
}

func TestMigrationTracker_Init(t *testing.T) {
	tdb := newMigrationTestDB(t)
	defer tdb.Close()

	tracker := NewMigrationTracker(tdb)
	ctx := context.Background()

	if err := tracker.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	var tableCount int
	err := tdb.QueryRow(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='_schema_migrations'").Scan(&tableCount)
	if err != nil {
		t.Fatalf("failed to check table existence: %v", err)
	}
	if tableCount != 1 {
		t.Errorf("_schema_migrations table not created, count = %d, want 1", tableCount)
	}

	var indexCount int
	err = tdb.QueryRow(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_schema_migrations_content_type'").Scan(&indexCount)
	if err != nil {
		t.Fatalf("failed to check index existence: %v", err)
	}
	if indexCount != 1 {
		t.Errorf("migration index not created, count = %d, want 1", indexCount)
	}
}

func TestMigrationTracker_Init_Idempotent(t *testing.T) {
	tdb := newMigrationTestDB(t)
	defer tdb.Close()

	tracker := NewMigrationTracker(tdb)
	ctx := context.Background()

	if err := tracker.Init(ctx); err != nil {
		t.Fatalf("first Init() error = %v", err)
	}
	// CREATE TABLE IF NOT EXISTS makes Init idempotent
	if err := tracker.Init(ctx); err != nil {
		t.Fatalf("second Init() error = %v", err)
	}
}

func TestMigrationTracker_Record(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	err := tracker.Record(ctx, "posts", "create", "CREATE TABLE posts (id INTEGER PRIMARY KEY)", true, "")
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	var contentType, operation string
	var schemaVersion int
	var success bool
	err = tdb.QueryRow(ctx,
		"SELECT content_type, operation, schema_version, success FROM _schema_migrations WHERE id = 1").Scan(
		&contentType, &operation, &schemaVersion, &success)
	if err != nil {
		t.Fatalf("failed to query record: %v", err)
	}

	if contentType != "posts" {
		t.Errorf("content_type = %q, want %q", contentType, "posts")
	}
	if operation != "create" {
		t.Errorf("operation = %q, want %q", operation, "create")
	}
	if schemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", schemaVersion)
	}
	if !success {
		t.Errorf("success = false, want true")
	}
}

func TestMigrationTracker_Record_Failed(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	err := tracker.Record(ctx, "posts", "create", "CREATE TABLE posts (id INTEGER PRIMARY KEY)", false, "table already exists")
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	var success bool
	var errMsg string
	err = tdb.QueryRow(ctx,
		"SELECT success, error_message FROM _schema_migrations WHERE id = 1").Scan(&success, &errMsg)
	if err != nil {
		t.Fatalf("failed to query record: %v", err)
	}

	if success {
		t.Errorf("success = true, want false for failed migration")
	}
	if errMsg != "table already exists" {
		t.Errorf("error_message = %q, want %q", errMsg, "table already exists")
	}
}

func TestMigrationTracker_GetNextVersion_NewContentType(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	version, err := tracker.GetNextVersion(ctx, "posts")
	if err != nil {
		t.Fatalf("GetNextVersion() error = %v", err)
	}

	if version != 1 {
		t.Errorf("GetNextVersion() for new content type = %d, want 1", version)
	}
}

func TestMigrationTracker_GetNextVersion_Increments(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	if err := tracker.Record(ctx, "posts", "create", "CREATE TABLE posts (...)", true, ""); err != nil {
		t.Fatalf("first Record() error = %v", err)
	}

	version, err := tracker.GetNextVersion(ctx, "posts")
	if err != nil {
		t.Fatalf("GetNextVersion() error = %v", err)
	}
	if version != 2 {
		t.Errorf("GetNextVersion() after one successful record = %d, want 2", version)
	}

	if err := tracker.Record(ctx, "posts", "alter", "ALTER TABLE posts ADD COLUMN title TEXT", true, ""); err != nil {
		t.Fatalf("second Record() error = %v", err)
	}

	version, err = tracker.GetNextVersion(ctx, "posts")
	if err != nil {
		t.Fatalf("GetNextVersion() error = %v", err)
	}
	if version != 3 {
		t.Errorf("GetNextVersion() after two successful records = %d, want 3", version)
	}
}

func TestMigrationTracker_GetCurrentVersion_NewContentType(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	version, err := tracker.GetCurrentVersion(ctx, "posts")
	if err != nil {
		t.Fatalf("GetCurrentVersion() error = %v", err)
	}

	if version != 0 {
		t.Errorf("GetCurrentVersion() for new content type = %d, want 0", version)
	}
}

func TestMigrationTracker_GetCurrentVersion_ExistingContentType(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	if err := tracker.Record(ctx, "posts", "create", "CREATE TABLE posts (...)", true, ""); err != nil {
		t.Fatalf("first Record() error = %v", err)
	}
	if err := tracker.Record(ctx, "posts", "alter", "ALTER TABLE posts ADD COLUMN title TEXT", true, ""); err != nil {
		t.Fatalf("second Record() error = %v", err)
	}

	version, err := tracker.GetCurrentVersion(ctx, "posts")
	if err != nil {
		t.Fatalf("GetCurrentVersion() error = %v", err)
	}
	if version != 2 {
		t.Errorf("GetCurrentVersion() = %d, want 2", version)
	}
}

func TestMigrationTracker_GetMigrationHistory_ChronologicalOrder(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	if err := tracker.Record(ctx, "posts", "create", "CREATE TABLE posts (...)", true, ""); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if err := tracker.Record(ctx, "posts", "alter", "ALTER TABLE posts ADD COLUMN title TEXT", true, ""); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if err := tracker.Record(ctx, "posts", "alter", "ALTER TABLE posts ADD COLUMN body TEXT", true, ""); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	records, err := tracker.GetMigrationHistory(ctx, "posts")
	if err != nil {
		t.Fatalf("GetMigrationHistory() error = %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("len(records) = %d, want 3", len(records))
	}

	for i, want := range []int{1, 2, 3} {
		if records[i].SchemaVersion != want {
			t.Errorf("records[%d].SchemaVersion = %d, want %d", i, records[i].SchemaVersion, want)
		}
	}

	for _, r := range records {
		if r.ContentType != "posts" {
			t.Errorf("ContentType = %q, want %q", r.ContentType, "posts")
		}
	}
}

func TestMigrationTracker_GetMigrationHistory_NonExistentContentType(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	records, err := tracker.GetMigrationHistory(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetMigrationHistory() error = %v", err)
	}

	if len(records) != 0 {
		t.Errorf("len(records) = %d, want 0 for non-existent content type", len(records))
	}
}

func TestMigrationTracker_GetAllMigrations(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	if err := tracker.Record(ctx, "posts", "create", "CREATE TABLE posts (...)", true, ""); err != nil {
		t.Fatalf("Record(posts) error = %v", err)
	}
	if err := tracker.Record(ctx, "users", "create", "CREATE TABLE users (...)", true, ""); err != nil {
		t.Fatalf("Record(users) error = %v", err)
	}
	if err := tracker.Record(ctx, "posts", "alter", "ALTER TABLE posts ADD COLUMN title TEXT", true, ""); err != nil {
		t.Fatalf("Record(posts alter) error = %v", err)
	}

	records, err := tracker.GetAllMigrations(ctx)
	if err != nil {
		t.Fatalf("GetAllMigrations() error = %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("len(records) = %d, want 3", len(records))
	}

	contentTypes := make(map[string]int)
	for _, r := range records {
		contentTypes[r.ContentType]++
	}
	if contentTypes["posts"] != 2 {
		t.Errorf("posts count = %d, want 2", contentTypes["posts"])
	}
	if contentTypes["users"] != 1 {
		t.Errorf("users count = %d, want 1", contentTypes["users"])
	}
}

func TestMigrationTracker_ToJSON(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	if err := tracker.Record(ctx, "posts", "create", "CREATE TABLE posts (...)", true, ""); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	records, err := tracker.GetMigrationHistory(ctx, "posts")
	if err != nil {
		t.Fatalf("GetMigrationHistory() error = %v", err)
	}

	data, err := tracker.ToJSON(records)
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	var parsed []MigrationRecord
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("ToJSON() produced invalid JSON: %v", err)
	}

	if len(parsed) != 1 {
		t.Errorf("len(parsed) = %d, want 1", len(parsed))
	}
	if parsed[0].ContentType != "posts" {
		t.Errorf("parsed[0].ContentType = %q, want %q", parsed[0].ContentType, "posts")
	}
	if parsed[0].SchemaVersion != 1 {
		t.Errorf("parsed[0].SchemaVersion = %d, want 1", parsed[0].SchemaVersion)
	}
	if !parsed[0].Success {
		t.Errorf("parsed[0].Success = false, want true")
	}
}

func TestMigrationTracker_MultipleRecords_VersionIncrement(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	for i := 0; i < 5; i++ {
		ops := []string{"create", "alter", "alter", "alter", "alter"}
		if err := tracker.Record(ctx, "posts", ops[i], fmt.Sprintf("sql_%d", i), true, ""); err != nil {
			t.Fatalf("Record(%d) error = %v", i, err)
		}
	}

	records, err := tracker.GetMigrationHistory(ctx, "posts")
	if err != nil {
		t.Fatalf("GetMigrationHistory() error = %v", err)
	}

	if len(records) != 5 {
		t.Fatalf("len(records) = %d, want 5", len(records))
	}

	for i, r := range records {
		wantVersion := i + 1
		if r.SchemaVersion != wantVersion {
			t.Errorf("records[%d].SchemaVersion = %d, want %d", i, r.SchemaVersion, wantVersion)
		}
	}

	current, err := tracker.GetCurrentVersion(ctx, "posts")
	if err != nil {
		t.Fatalf("GetCurrentVersion() error = %v", err)
	}
	if current != 5 {
		t.Errorf("GetCurrentVersion() = %d, want 5", current)
	}

	next, err := tracker.GetNextVersion(ctx, "posts")
	if err != nil {
		t.Fatalf("GetNextVersion() error = %v", err)
	}
	if next != 6 {
		t.Errorf("GetNextVersion() = %d, want 6", next)
	}
}

func TestMigrationTracker_FailedRecord_DoesNotIncrementVersion(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	if err := tracker.Record(ctx, "posts", "create", "CREATE TABLE posts (...)", true, ""); err != nil {
		t.Fatalf("Record(success) error = %v", err)
	}

	if err := tracker.Record(ctx, "posts", "alter", "ALTER TABLE posts ADD COLUMN bad TEXT", false, "column already exists"); err != nil {
		t.Fatalf("Record(failed) error = %v", err)
	}

	current, err := tracker.GetCurrentVersion(ctx, "posts")
	if err != nil {
		t.Fatalf("GetCurrentVersion() error = %v", err)
	}
	if current != 1 {
		t.Errorf("GetCurrentVersion() after failed record = %d, want 1", current)
	}

	next, err := tracker.GetNextVersion(ctx, "posts")
	if err != nil {
		t.Fatalf("GetNextVersion() error = %v", err)
	}
	if next != 2 {
		t.Errorf("GetNextVersion() after failed record = %d, want 2", next)
	}

	if err := tracker.Record(ctx, "posts", "alter", "ALTER TABLE posts ADD COLUMN title TEXT", true, ""); err != nil {
		t.Fatalf("Record(success after fail) error = %v", err)
	}

	current, err = tracker.GetCurrentVersion(ctx, "posts")
	if err != nil {
		t.Fatalf("GetCurrentVersion() error = %v", err)
	}
	if current != 2 {
		t.Errorf("GetCurrentVersion() after recovery = %d, want 2", current)
	}

	records, err := tracker.GetMigrationHistory(ctx, "posts")
	if err != nil {
		t.Fatalf("GetMigrationHistory() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("len(records) = %d, want 3", len(records))
	}

	failedFound := false
	for _, r := range records {
		if !r.Success {
			failedFound = true
			if r.ErrorMessage != "column already exists" {
				t.Errorf("failed record ErrorMessage = %q, want %q", r.ErrorMessage, "column already exists")
			}
		}
	}
	if !failedFound {
		t.Error("expected to find a failed record in migration history")
	}
}

func TestMigrationTracker_ContentTypeIsolation(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	if err := tracker.Record(ctx, "posts", "create", "CREATE TABLE posts (...)", true, ""); err != nil {
		t.Fatalf("Record(posts) error = %v", err)
	}
	if err := tracker.Record(ctx, "users", "create", "CREATE TABLE users (...)", true, ""); err != nil {
		t.Fatalf("Record(users) error = %v", err)
	}
	if err := tracker.Record(ctx, "posts", "alter", "ALTER TABLE posts ADD COLUMN title TEXT", true, ""); err != nil {
		t.Fatalf("Record(posts alter) error = %v", err)
	}

	postsVersion, err := tracker.GetCurrentVersion(ctx, "posts")
	if err != nil {
		t.Fatalf("GetCurrentVersion(posts) error = %v", err)
	}
	if postsVersion != 2 {
		t.Errorf("GetCurrentVersion(posts) = %d, want 2", postsVersion)
	}

	usersVersion, err := tracker.GetCurrentVersion(ctx, "users")
	if err != nil {
		t.Fatalf("GetCurrentVersion(users) error = %v", err)
	}
	if usersVersion != 1 {
		t.Errorf("GetCurrentVersion(users) = %d, want 1", usersVersion)
	}

	postsHistory, err := tracker.GetMigrationHistory(ctx, "posts")
	if err != nil {
		t.Fatalf("GetMigrationHistory(posts) error = %v", err)
	}
	if len(postsHistory) != 2 {
		t.Errorf("len(postsHistory) = %d, want 2", len(postsHistory))
	}
	for _, r := range postsHistory {
		if r.ContentType != "posts" {
			t.Errorf("found ContentType = %q in posts history, want %q", r.ContentType, "posts")
		}
	}

	usersHistory, err := tracker.GetMigrationHistory(ctx, "users")
	if err != nil {
		t.Fatalf("GetMigrationHistory(users) error = %v", err)
	}
	if len(usersHistory) != 1 {
		t.Errorf("len(usersHistory) = %d, want 1", len(usersHistory))
	}
	for _, r := range usersHistory {
		if r.ContentType != "users" {
			t.Errorf("found ContentType = %q in users history, want %q", r.ContentType, "users")
		}
	}
}

func TestMigrationTracker_Record_Fields(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	if err := tracker.Record(ctx, "articles", "create", "CREATE TABLE articles (id INT)", true, ""); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	records, err := tracker.GetMigrationHistory(ctx, "articles")
	if err != nil {
		t.Fatalf("GetMigrationHistory() error = %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}

	r := records[0]

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"content_type", r.ContentType, "articles"},
		{"operation", r.Operation, "create"},
		{"sql_executed", r.SQLExecuted, "CREATE TABLE articles (id INT)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}

	if r.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", r.SchemaVersion)
	}
	if !r.Success {
		t.Errorf("Success = false, want true")
	}
	if r.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty string for successful record", r.ErrorMessage)
	}
	if r.ID != 1 {
		t.Errorf("ID = %d, want 1", r.ID)
	}
}

func TestMigrationTracker_ToJSON_EmptyRecords(t *testing.T) {
	tracker, _ := setupTracker(t)

	data, err := tracker.ToJSON([]MigrationRecord{})
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	var parsed []MigrationRecord
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("ToJSON() produced invalid JSON: %v", err)
	}

	if len(parsed) != 0 {
		t.Errorf("len(parsed) = %d, want 0 for empty records", len(parsed))
	}
}

func TestMigrationTracker_GetAllMigrations_Empty(t *testing.T) {
	tracker, tdb := setupTracker(t)
	defer tdb.Close()

	ctx := context.Background()

	records, err := tracker.GetAllMigrations(ctx)
	if err != nil {
		t.Fatalf("GetAllMigrations() error = %v", err)
	}

	if len(records) != 0 {
		t.Errorf("len(records) = %d, want 0 for empty database", len(records))
	}
}

type migrationFailDB struct {
	*sql.DB
	failAfterExec int
	execCount     int
	queryRowErr   error
}

func (f *migrationFailDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	f.execCount++
	if f.execCount > f.failAfterExec {
		return nil, fmt.Errorf("injected exec failure")
	}
	return f.DB.ExecContext(ctx, query, args...)
}

func (f *migrationFailDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return f.DB.QueryContext(ctx, query, args...)
}

func (f *migrationFailDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	if f.queryRowErr != nil {
		return nil
	}
	return f.DB.QueryRowContext(ctx, query, args...)
}

func (f *migrationFailDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return f.DB.BeginTx(ctx, opts)
}

func (f *migrationFailDB) Ping(ctx context.Context) error {
	return f.DB.PingContext(ctx)
}

func (f *migrationFailDB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return f.DB.PrepareContext(ctx, query)
}

func (f *migrationFailDB) Close() error {
	return f.DB.Close()
}

func (f *migrationFailDB) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	return nil, fmt.Errorf("not implemented")
}

type migrationQueryRowFailDB struct {
	*sql.DB
	failQueryRow bool
}

func (f *migrationQueryRowFailDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return f.DB.ExecContext(ctx, query, args...)
}

func (f *migrationQueryRowFailDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return f.DB.QueryContext(ctx, query, args...)
}

func (f *migrationQueryRowFailDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	if f.failQueryRow {
		return f.DB.QueryRowContext(ctx, "SELECT MAX(schema_version) FROM _nonexistent_table WHERE content_type = ?", args...)
	}
	return f.DB.QueryRowContext(ctx, query, args...)
}

func (f *migrationQueryRowFailDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return f.DB.BeginTx(ctx, opts)
}

func (f *migrationQueryRowFailDB) Ping(ctx context.Context) error {
	return f.DB.PingContext(ctx)
}

func (f *migrationQueryRowFailDB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return f.DB.PrepareContext(ctx, query)
}

func (f *migrationQueryRowFailDB) Close() error {
	return f.DB.Close()
}

func (f *migrationQueryRowFailDB) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	return nil, fmt.Errorf("not implemented")
}

type migrationQueryFailDB struct {
	*sql.DB
	failQuery bool
}

func (f *migrationQueryFailDB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return f.DB.ExecContext(ctx, query, args...)
}

func (f *migrationQueryFailDB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if f.failQuery {
		return nil, fmt.Errorf("injected query failure")
	}
	return f.DB.QueryContext(ctx, query, args...)
}

func (f *migrationQueryFailDB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return f.DB.QueryRowContext(ctx, query, args...)
}

func (f *migrationQueryFailDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return f.DB.BeginTx(ctx, opts)
}

func (f *migrationQueryFailDB) Ping(ctx context.Context) error {
	return f.DB.PingContext(ctx)
}

func (f *migrationQueryFailDB) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return f.DB.PrepareContext(ctx, query)
}

func (f *migrationQueryFailDB) Close() error {
	return f.DB.Close()
}

func (f *migrationQueryFailDB) SchemaIntrospect(ctx context.Context) (*interfaces.DatabaseSchema, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestMigrationTracker_Init_ExecFail_TableCreate(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	failDB := &migrationFailDB{DB: db, failAfterExec: 0}
	tracker := NewMigrationTracker(failDB)

	err = tracker.Init(context.Background())
	if err == nil {
		t.Fatal("expected error when Exec fails for table creation, got nil")
	}
	if !strings.Contains(err.Error(), "creating _schema_migrations table") {
		t.Errorf("error should mention table creation, got: %v", err)
	}
}

func TestMigrationTracker_Init_ExecFail_IndexCreate(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	failDB := &migrationFailDB{DB: db, failAfterExec: 1}
	tracker := NewMigrationTracker(failDB)

	err = tracker.Init(context.Background())
	if err == nil {
		t.Fatal("expected error when Exec fails for index creation, got nil")
	}
	if !strings.Contains(err.Error(), "creating migration index") {
		t.Errorf("error should mention index creation, got: %v", err)
	}
}

func TestMigrationTracker_Record_GetNextVersionFail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	realDB := &migrationTestDB{db: db}
	tracker := NewMigrationTracker(realDB)
	if err := tracker.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	failDB := &migrationQueryRowFailDB{DB: db, failQueryRow: true}
	failTracker := NewMigrationTracker(failDB)

	err = failTracker.Record(context.Background(), "posts", "create", "SQL", true, "")
	if err == nil {
		t.Fatal("expected error when GetNextVersion fails, got nil")
	}
	if !strings.Contains(err.Error(), "getting next version") {
		t.Errorf("error should mention getting next version, got: %v", err)
	}
}

func TestMigrationTracker_Record_ExecFail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	realDB := &migrationTestDB{db: db}
	tracker := NewMigrationTracker(realDB)
	if err := tracker.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	_, _ = realDB.Exec(context.Background(), "INSERT INTO _schema_migrations (content_type, operation, sql_executed, schema_version, executed_at, success, error_message) VALUES (?, ?, ?, ?, ?, ?, ?)", "posts", "create", "SQL", 1, "2024-01-01", true, "")

	failDB := &migrationFailDB{DB: db, failAfterExec: 0}
	failTracker := NewMigrationTracker(failDB)

	err = failTracker.Record(context.Background(), "posts", "alter", "SQL2", true, "")
	if err == nil {
		t.Fatal("expected error when INSERT fails, got nil")
	}
	if !strings.Contains(err.Error(), "recording migration") {
		t.Errorf("error should mention recording migration, got: %v", err)
	}
}

func TestMigrationTracker_GetMigrationHistory_QueryFail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	failDB := &migrationQueryFailDB{DB: db, failQuery: true}
	tracker := NewMigrationTracker(failDB)

	_, err = tracker.GetMigrationHistory(context.Background(), "posts")
	if err == nil {
		t.Fatal("expected error when Query fails, got nil")
	}
	if !strings.Contains(err.Error(), "querying migration history") {
		t.Errorf("error should mention querying migration history, got: %v", err)
	}
}

func TestMigrationTracker_GetAllMigrations_QueryFail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	failDB := &migrationQueryFailDB{DB: db, failQuery: true}
	tracker := NewMigrationTracker(failDB)

	_, err = tracker.GetAllMigrations(context.Background())
	if err == nil {
		t.Fatal("expected error when Query fails, got nil")
	}
	if !strings.Contains(err.Error(), "querying all migrations") {
		t.Errorf("error should mention querying all migrations, got: %v", err)
	}
}
