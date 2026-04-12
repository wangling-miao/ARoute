package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Migration Edge Case Tests
// ============================================================================

func TestMigrationRunner_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations from empty directory: %v", err)
	}

	if runner.TotalCount() != 0 {
		t.Errorf("Expected 0 migrations in empty directory, got %d", runner.TotalCount())
	}

	if runner.PendingCount() != 0 {
		t.Errorf("Expected 0 pending in empty directory, got %d", runner.PendingCount())
	}

	appliedCount, err := runner.Apply(ctx)
	if err != nil {
		t.Fatalf("Apply failed on empty migrations: %v", err)
	}
	if appliedCount != 0 {
		t.Errorf("Expected 0 migrations applied, got %d", appliedCount)
	}

	revertedCount, err := runner.Revert(ctx, 1)
	if err != nil {
		t.Fatalf("Revert failed on empty migrations: %v", err)
	}
	if revertedCount != 0 {
		t.Errorf("Expected 0 migrations reverted, got %d", revertedCount)
	}
}

func TestMigrationRunner_NonexistentDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nonexistentDir := filepath.Join(tmpDir, "does_not_exist")

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, nonexistentDir)
	err = runner.Load(ctx)
	if err != nil {
		t.Fatalf("Load should succeed with nonexistent directory: %v", err)
	}

	if runner.TotalCount() != 0 {
		t.Errorf("Expected 0 migrations for nonexistent directory, got %d", runner.TotalCount())
	}
}

func TestMigrationRunner_InvalidFilenameFormat(t *testing.T) {
	tmpDir := t.TempDir()

	invalidFiles := []string{
		"invalid.sql",
		"no_version.sql",
		"short.sql",
		"abc123_test.sql",
		"202_test.sql",
	}

	for _, filename := range invalidFiles {
		filePath := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(filePath, []byte("SELECT 1;"), 0644); err != nil {
			t.Fatalf("Failed to write invalid file %s: %v", filename, err)
		}
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	err = runner.Load(ctx)
	if err == nil {
		t.Fatal("Expected error for invalid migration filename format")
	}
	if !strings.Contains(err.Error(), "invalid migration filename format") {
		t.Errorf("Expected 'invalid migration filename format' error, got: %v", err)
	}
}

func TestMigrationRunner_MixedValidInvalidFiles(t *testing.T) {
	tmpDir := t.TempDir()

	validFile := filepath.Join(tmpDir, "2026041301_valid.sql")
	if err := os.WriteFile(validFile, []byte("CREATE TABLE valid (id INTEGER);"), 0644); err != nil {
		t.Fatalf("Failed to write valid migration: %v", err)
	}

	invalidFile := filepath.Join(tmpDir, "invalid.sql")
	if err := os.WriteFile(invalidFile, []byte("SELECT 1;"), 0644); err != nil {
		t.Fatalf("Failed to write invalid migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	err = runner.Load(ctx)
	if err == nil {
		t.Fatal("Expected error due to invalid filename in directory")
	}
}

func TestMigrationRunner_SkipNonSQLFiles(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_test.sql")
	if err := os.WriteFile(migrationFile, []byte("CREATE TABLE test (id INTEGER);"), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	otherFiles := []string{
		"readme.txt",
		"data.json",
		"script.sh",
		"config.yaml",
	}

	for _, filename := range otherFiles {
		filePath := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", filename, err)
		}
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if runner.TotalCount() != 1 {
		t.Errorf("Expected 1 migration (SQL files only), got %d", runner.TotalCount())
	}
}

func TestMigrationRunner_VersionSorting(t *testing.T) {
	tmpDir := t.TempDir()

	migrations := []struct {
		filename string
		content  string
	}{
		{"2026041305_fifth.sql", "CREATE TABLE fifth (id INTEGER);"},
		{"2026041301_first.sql", "CREATE TABLE first (id INTEGER);"},
		{"2026041303_third.sql", "CREATE TABLE third (id INTEGER);"},
		{"2026041302_second.sql", "CREATE TABLE second (id INTEGER);"},
		{"2026041304_fourth.sql", "CREATE TABLE fourth (id INTEGER);"},
	}

	for _, m := range migrations {
		filePath := filepath.Join(tmpDir, m.filename)
		if err := os.WriteFile(filePath, []byte(m.content), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", m.filename, err)
		}
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if runner.TotalCount() != 5 {
		t.Fatalf("Expected 5 migrations, got %d", runner.TotalCount())
	}

	expectedOrder := []int64{2026041301, 2026041302, 2026041303, 2026041304, 2026041305}
	for i, expected := range expectedOrder {
		if runner.migrations[i].Version != expected {
			t.Errorf("Migration at index %d: expected version %d, got %d", i, expected, runner.migrations[i].Version)
		}
	}
}

func TestMigrationRunner_RevertMoreThanApplied(t *testing.T) {
	tmpDir := t.TempDir()

	migration1 := filepath.Join(tmpDir, "2026041301_init.sql")
	content1 := `
CREATE TABLE revert_limit (id INTEGER PRIMARY KEY);
-- @down
DROP TABLE revert_limit;
`
	if err := os.WriteFile(migration1, []byte(content1), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if _, err := runner.Apply(ctx); err != nil {
		t.Fatalf("Failed to apply: %v", err)
	}

	revertedCount, err := runner.Revert(ctx, 10)
	if err != nil {
		t.Fatalf("Failed to revert: %v", err)
	}

	if revertedCount != 1 {
		t.Errorf("Expected 1 reverted (limited to available), got %d", revertedCount)
	}

	if runner.PendingCount() != 1 {
		t.Errorf("Expected 1 pending after revert, got %d", runner.PendingCount())
	}
}

func TestMigrationRunner_RevertWithoutDownMigration(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_no_down.sql")
	content := "CREATE TABLE no_down_test (id INTEGER PRIMARY KEY);"
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if _, err := runner.Apply(ctx); err != nil {
		t.Fatalf("Failed to apply: %v", err)
	}

	_, err = runner.Revert(ctx, 1)
	if err == nil {
		t.Fatal("Expected error when reverting migration without down section")
	}
	if !strings.Contains(err.Error(), "no down migration found") {
		t.Errorf("Expected 'no down migration found' error, got: %v", err)
	}
}

func TestMigrationRunner_RevertZero(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_test.sql")
	content := `
CREATE TABLE revert_zero (id INTEGER);
-- @down
DROP TABLE revert_zero;
`
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if _, err := runner.Apply(ctx); err != nil {
		t.Fatalf("Failed to apply: %v", err)
	}

	revertedCount, err := runner.Revert(ctx, 0)
	if err != nil {
		t.Fatalf("Failed to revert 0: %v", err)
	}
	if revertedCount != 0 {
		t.Errorf("Expected 0 reverted, got %d", revertedCount)
	}

	if runner.PendingCount() != 0 {
		t.Errorf("Expected 0 pending (all still applied), got %d", runner.PendingCount())
	}
}

func TestMigrationRunner_ChecksumCalculation(t *testing.T) {
	tmpDir := t.TempDir()

	content := "CREATE TABLE checksum_test (id INTEGER PRIMARY KEY);"
	expectedSum := sha256.Sum256([]byte(content))
	expectedChecksum := hex.EncodeToString(expectedSum[:])

	migrationFile := filepath.Join(tmpDir, "2026041301_checksum.sql")
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if runner.TotalCount() != 1 {
		t.Fatalf("Expected 1 migration, got %d", runner.TotalCount())
	}

	if runner.migrations[0].Checksum != expectedChecksum {
		t.Errorf("Checksum mismatch: got %s, expected %s", runner.migrations[0].Checksum, expectedChecksum)
	}
}

func TestMigrationRunner_VerifyChecksum(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_verify.sql")
	content := `
CREATE TABLE verify_checksum (id INTEGER);
-- @down
DROP TABLE verify_checksum;
`
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if _, err := runner.Apply(ctx); err != nil {
		t.Fatalf("Failed to apply: %v", err)
	}

	tampered, err := runner.VerifyChecksum(ctx)
	if err != nil {
		t.Fatalf("VerifyChecksum failed: %v", err)
	}

	if len(tampered) != 0 {
		t.Errorf("Expected no tampered migrations, got %d", len(tampered))
	}
}

func TestMigrationRunner_MultipleReverts(t *testing.T) {
	tmpDir := t.TempDir()

	for i := 1; i <= 5; i++ {
		filename := filepath.Join(tmpDir, fmt.Sprintf("202604130%d_multi.sql", i))
		content := fmt.Sprintf(`
CREATE TABLE multi_%d (id INTEGER);
-- @down
DROP TABLE multi_%d;
`, i, i)
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write migration %d: %v", i, err)
		}
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	appliedCount, err := runner.Apply(ctx)
	if err != nil {
		t.Fatalf("Failed to apply: %v", err)
	}
	if appliedCount != 5 {
		t.Fatalf("Expected 5 applied, got %d", appliedCount)
	}

	revertedCount, err := runner.Revert(ctx, 3)
	if err != nil {
		t.Fatalf("Failed to revert 3: %v", err)
	}
	if revertedCount != 3 {
		t.Errorf("Expected 3 reverted, got %d", revertedCount)
	}

	if runner.AppliedCount() != 2 {
		t.Errorf("Expected 2 still applied, got %d", runner.AppliedCount())
	}
	if runner.PendingCount() != 3 {
		t.Errorf("Expected 3 pending, got %d", runner.PendingCount())
	}
}

func TestMigrationRunner_ReApplyAfterRevert(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_reapply.sql")
	content := `
CREATE TABLE reapply_test (id INTEGER PRIMARY KEY);
-- @down
DROP TABLE reapply_test;
`
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if _, err := runner.Apply(ctx); err != nil {
		t.Fatalf("First apply failed: %v", err)
	}

	if _, err := runner.Revert(ctx, 1); err != nil {
		t.Fatalf("Revert failed: %v", err)
	}

	appliedCount, err := runner.Apply(ctx)
	if err != nil {
		t.Fatalf("Re-apply failed: %v", err)
	}
	if appliedCount != 1 {
		t.Errorf("Expected 1 re-applied, got %d", appliedCount)
	}

	if runner.AppliedCount() != 1 {
		t.Errorf("Expected 1 applied, got %d", runner.AppliedCount())
	}
}

func TestMigrationRunner_SQLStatementSplitting(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_split.sql")
	content := `
CREATE TABLE split_test1 (id INTEGER PRIMARY KEY);
CREATE TABLE split_test2 (id INTEGER PRIMARY KEY, name TEXT);
CREATE INDEX idx_split_test2_name ON split_test2(name);
-- @down
DROP INDEX idx_split_test2_name;
DROP TABLE split_test2;
DROP TABLE split_test1;
`
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if _, err := runner.Apply(ctx); err != nil {
		t.Fatalf("Failed to apply multi-statement migration: %v", err)
	}

	rows, err := service.Query(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'split_test%' ORDER BY name")
	if err != nil {
		t.Fatalf("Failed to query tables: %v", err)
	}
	defer rows.Close()

	tableCount := 0
	for rows.Next() {
		tableCount++
	}
	if tableCount != 2 {
		t.Errorf("Expected 2 tables created, found %d", tableCount)
	}

	if _, err := runner.Revert(ctx, 1); err != nil {
		t.Fatalf("Failed to revert multi-statement migration: %v", err)
	}

	rows2, err := service.Query(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'split_test%'")
	if err != nil {
		t.Fatalf("Failed to query tables after revert: %v", err)
	}
	defer rows2.Close()

	tableCountAfter := 0
	for rows2.Next() {
		tableCountAfter++
	}
	if tableCountAfter != 0 {
		t.Errorf("Expected 0 tables after revert, found %d", tableCountAfter)
	}
}

func TestMigrationRunner_EmptyStatements(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_empty.sql")
	content := `

   
CREATE TABLE empty_stmt_test (id INTEGER);



-- @down
DROP TABLE empty_stmt_test;
`
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	appliedCount, err := runner.Apply(ctx)
	if err != nil {
		t.Fatalf("Failed to apply migration with empty statements: %v", err)
	}
	if appliedCount != 1 {
		t.Errorf("Expected 1 applied, got %d", appliedCount)
	}

	rows, err := service.Query(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='empty_stmt_test'")
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Error("Expected table to be created despite empty statements")
	}
}

func TestMigrationRunner_ExtractDownMigration(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "standard down section",
			content: `CREATE TABLE test (id INTEGER);
-- @down
DROP TABLE test;`,
			expected: "DROP TABLE test;",
		},
		{
			name:     "no down section",
			content:  `CREATE TABLE test (id INTEGER);`,
			expected: "",
		},
		{
			name: "down with multiple statements",
			content: `CREATE TABLE test (id INTEGER);
CREATE INDEX idx_test ON test(id);
-- @down
DROP INDEX idx_test;
DROP TABLE test;`,
			expected: "DROP INDEX idx_test;\nDROP TABLE test;",
		},
		{
			name: "down section with extra whitespace",
			content: `CREATE TABLE test (id INTEGER);
-- @down   
  DROP TABLE test;`,
			expected: "  DROP TABLE test;",
		},
		{
			name: "down section only",
			content: `-- @down
DROP TABLE test;`,
			expected: "DROP TABLE test;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDownMigration(tt.content)
			if strings.TrimSpace(result) != strings.TrimSpace(tt.expected) {
				t.Errorf("extractDownMigration() = '%s', want '%s'", result, tt.expected)
			}
		})
	}
}

func TestMigrationRunner_ExtractUpMigration(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "standard up/down",
			content: `CREATE TABLE test (id INTEGER);
-- @down
DROP TABLE test;`,
			expected: "CREATE TABLE test (id INTEGER);",
		},
		{
			name:     "no down section",
			content:  `CREATE TABLE test (id INTEGER);`,
			expected: "CREATE TABLE test (id INTEGER);",
		},
		{
			name: "empty up section",
			content: `-- @down
DROP TABLE test;`,
			expected: "",
		},
		{
			name: "multi-line up",
			content: `CREATE TABLE test (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL
);
-- @down
DROP TABLE test;`,
			expected: "CREATE TABLE test (\n\tid INTEGER PRIMARY KEY,\n\tname TEXT NOT NULL\n);",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractUpMigration(tt.content)
			if strings.TrimSpace(result) != strings.TrimSpace(tt.expected) {
				t.Errorf("extractUpMigration() = '%s', want '%s'", result, tt.expected)
			}
		})
	}
}

func TestMigrationRunner_StatusPending(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_status_pending.sql")
	content := `CREATE TABLE status_pending (id INTEGER);`
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	status, err := runner.Status(ctx)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if len(status) != 1 {
		t.Fatalf("Expected 1 status entry, got %d", len(status))
	}

	if status[0].Status != "pending" {
		t.Errorf("Expected status 'pending', got '%s'", status[0].Status)
	}

	if !status[0].AppliedAt.IsZero() {
		t.Errorf("Expected AppliedAt to be zero for pending migration")
	}
}

func TestMigrationRunner_StatusApplied(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_status_applied.sql")
	content := `
CREATE TABLE status_applied (id INTEGER);
-- @down
DROP TABLE status_applied;
`
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if _, err := runner.Apply(ctx); err != nil {
		t.Fatalf("Failed to apply: %v", err)
	}

	status, err := runner.Status(ctx)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if len(status) != 1 {
		t.Fatalf("Expected 1 status entry, got %d", len(status))
	}

	if status[0].Status != "applied" {
		t.Errorf("Expected status 'applied', got '%s'", status[0].Status)
	}

	if status[0].AppliedAt.IsZero() {
		t.Errorf("Expected AppliedAt to be non-zero for applied migration")
	}
}

func TestMigrationRunner_DirectoryWithSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()

	subDir := filepath.Join(tmpDir, "subdirectory")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	migrationFile := filepath.Join(tmpDir, "2026041301_main.sql")
	if err := os.WriteFile(migrationFile, []byte("CREATE TABLE main (id INTEGER);"), 0644); err != nil {
		t.Fatalf("Failed to write main migration: %v", err)
	}

	subMigrationFile := filepath.Join(subDir, "2026041302_sub.sql")
	if err := os.WriteFile(subMigrationFile, []byte("CREATE TABLE sub (id INTEGER);"), 0644); err != nil {
		t.Fatalf("Failed to write sub migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if runner.TotalCount() != 1 {
		t.Errorf("Expected 1 migration (subdirectory files should be skipped), got %d", runner.TotalCount())
	}
}

func TestMigrationRunner_ReadOnlyFile(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_readonly.sql")
	content := "CREATE TABLE readonly_test (id INTEGER);"
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	if err := os.Chmod(migrationFile, 0444); err != nil {
		t.Fatalf("Failed to set read-only mode: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	err = runner.Load(ctx)
	if err != nil {
		t.Fatalf("Load should succeed for readable file: %v", err)
	}

	if runner.TotalCount() != 1 {
		t.Errorf("Expected 1 migration, got %d", runner.TotalCount())
	}
}

// ============================================================================
// PostgreSQL Migration Tests
// ============================================================================

func TestMigrationRunner_PostgreSQL_Apply(t *testing.T) {
	t.Skip("pgx stdlib wrapper encoding issue with int64 parameters")
}

func TestMigrationRunner_PostgreSQL_MultipleMigrations(t *testing.T) {
	t.Skip("pgx stdlib wrapper encoding issue with int64 parameters")
}

func TestMigrationRunner_PostgreSQL_Status(t *testing.T) {
	t.Skip("pgx stdlib wrapper encoding issue with int64 parameters")
}

func TestMigrationRunner_PostgreSQL_ChecksumVerification(t *testing.T) {
	t.Skip("pgx stdlib wrapper encoding issue with int64 parameters")
}

// ============================================================================
// Transaction Rollback on Migration Failure
// ============================================================================

func TestMigrationRunner_TransactionRollbackOnFailure(t *testing.T) {
	tmpDir := t.TempDir()

	migrationFile := filepath.Join(tmpDir, "2026041301_tx_rollback.sql")
	content := `
CREATE TABLE tx_rollback_before (id INTEGER PRIMARY KEY);
INVALID SQL HERE;
CREATE TABLE tx_rollback_after (id INTEGER PRIMARY KEY);
-- @down
DROP TABLE tx_rollback_before;
`
	if err := os.WriteFile(migrationFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write migration: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	_, err = runner.Apply(ctx)
	if err == nil {
		t.Fatal("Expected error for invalid SQL")
	}

	rows, err := service.Query(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='tx_rollback_before'")
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		t.Error("Expected tx_rollback_before table to NOT exist due to transaction rollback")
	}

	rows2, err := service.Query(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='tx_rollback_after'")
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	defer rows2.Close()

	if rows2.Next() {
		t.Error("Expected tx_rollback_after table to NOT exist due to transaction rollback")
	}

	var count int
	err = service.QueryRow(ctx, "SELECT COUNT(*) FROM _migrations WHERE version = 2026041301").Scan(&count)
	if err == nil && count > 0 {
		t.Error("Migration should not be recorded in _migrations table after failure")
	}
}

func TestMigrationRunner_ExistingAppliedMigrations(t *testing.T) {
	tmpDir := t.TempDir()

	migration1 := filepath.Join(tmpDir, "2026041301_existing.sql")
	content1 := `
CREATE TABLE existing_test (id INTEGER PRIMARY KEY);
-- @down
DROP TABLE existing_test;
`
	if err := os.WriteFile(migration1, []byte(content1), 0644); err != nil {
		t.Fatalf("Failed to write migration 1: %v", err)
	}

	migration2 := filepath.Join(tmpDir, "2026041302_new.sql")
	content2 := `
CREATE TABLE new_test (id INTEGER PRIMARY KEY);
-- @down
DROP TABLE new_test;
`
	if err := os.WriteFile(migration2, []byte(content2), 0644); err != nil {
		t.Fatalf("Failed to write migration 2: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	service := NewService(db, DriverSQLite)
	ctx := context.Background()

	_, err = service.Exec(ctx, `
CREATE TABLE IF NOT EXISTS _migrations (
	version INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	applied_at TEXT NOT NULL
)
`)
	if err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	appliedAt := time.Now().Format(time.RFC3339)
	_, err = service.Exec(ctx, "INSERT INTO _migrations (version, name, applied_at) VALUES (?, ?, ?)",
		2026041301, "existing", appliedAt)
	if err != nil {
		t.Fatalf("Failed to record existing migration: %v", err)
	}

	runner := NewMigrationRunner(service, tmpDir)
	if err := runner.Load(ctx); err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if runner.TotalCount() != 2 {
		t.Errorf("Expected 2 total migrations, got %d", runner.TotalCount())
	}

	if runner.AppliedCount() != 1 {
		t.Errorf("Expected 1 applied (from existing record), got %d", runner.AppliedCount())
	}

	if runner.PendingCount() != 1 {
		t.Errorf("Expected 1 pending, got %d", runner.PendingCount())
	}

	appliedCount, err := runner.Apply(ctx)
	if err != nil {
		t.Fatalf("Failed to apply: %v", err)
	}
	if appliedCount != 1 {
		t.Errorf("Expected only 1 new migration applied, got %d", appliedCount)
	}
}
