package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Migration struct {
	Version   int64
	Name      string
	Checksum  string
	FilePath  string
	Content   string
	Applied   bool
	AppliedAt time.Time
}

type MigrationRunner struct {
	service       *Service
	migrations    []*Migration
	migrationsDir string
}

func NewMigrationRunner(service *Service, migrationsDir string) *MigrationRunner {
	return &MigrationRunner{
		service:       service,
		migrationsDir: migrationsDir,
		migrations:    []*Migration{},
	}
}

func (m *MigrationRunner) Load(ctx context.Context) error {
	if _, err := os.Stat(m.migrationsDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(m.migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		migration, err := m.parseMigrationFile(m.migrationsDir, entry.Name())
		if err != nil {
			return fmt.Errorf("failed to parse migration %s: %w", entry.Name(), err)
		}

		m.migrations = append(m.migrations, migration)
	}

	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version < m.migrations[j].Version
	})

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to query applied migrations: %w", err)
	}

	for _, migration := range m.migrations {
		appliedAt, exists := applied[migration.Version]
		if exists {
			migration.Applied = true
			migration.AppliedAt = appliedAt
		}
	}

	return nil
}

func (m *MigrationRunner) parseMigrationFile(dir, filename string) (*Migration, error) {
	version, name := extractVersionAndName(filename)
	if version == 0 {
		return nil, fmt.Errorf("invalid migration filename format: %s (expected YYYYMMDDNN_description.sql)", filename)
	}

	filePath := filepath.Join(dir, filename)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read migration file: %w", err)
	}

	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:])

	return &Migration{
		Version:  version,
		Name:     name,
		Checksum: checksum,
		FilePath: filePath,
		Content:  string(content),
		Applied:  false,
	}, nil
}

func extractVersionAndName(filename string) (int64, string) {
	re := regexp.MustCompile(`^(\d{4,12})_(.+)\.sql$`)
	matches := re.FindStringSubmatch(filename)
	if len(matches) < 3 {
		return 0, ""
	}
	versionStr := matches[1]
	version, err := strconv.ParseInt(versionStr, 10, 64)
	if err != nil {
		return 0, ""
	}
	return version, matches[2]
}

func (m *MigrationRunner) getAppliedMigrations(ctx context.Context) (map[int64]time.Time, error) {
	rows, err := m.service.Query(ctx, "SELECT version, applied_at FROM _migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[int64]time.Time{}
	for rows.Next() {
		var version int64
		var appliedAtStr string
		if err := rows.Scan(&version, &appliedAtStr); err != nil {
			return nil, err
		}

		appliedAt, err := time.Parse(time.RFC3339, appliedAtStr)
		if err != nil {
			appliedAt = time.Now()
		}

		applied[version] = appliedAt
	}

	return applied, rows.Err()
}

func (m *MigrationRunner) Apply(ctx context.Context) (int, error) {
	appliedCount := 0

	for _, migration := range m.migrations {
		if migration.Applied {
			continue
		}

		err := m.applyMigration(ctx, migration)
		if err != nil {
			return appliedCount, fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}

		migration.Applied = true
		migration.AppliedAt = time.Now()
		appliedCount++
	}

	return appliedCount, nil
}

func (m *MigrationRunner) applyMigration(ctx context.Context, migration *Migration) error {
	upContent := extractUpMigration(migration.Content)
	if upContent == "" {
		upContent = migration.Content
	}

	tx, err := m.service.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	statements := splitSQLStatements(upContent)
	for i, stmt := range statements {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		_, err = tx.ExecContext(ctx, stmt)
		if err != nil {
			return fmt.Errorf("statement %d failed: %w", i+1, err)
		}
	}

	appliedAt := time.Now().Format(time.RFC3339)
	var insertSQL string
	if m.service.Driver() == DriverSQLite {
		insertSQL = "INSERT INTO _migrations (version, name, applied_at) VALUES (?, ?, ?)"
	} else {
		insertSQL = "INSERT INTO _migrations (version, name, applied_at) VALUES ($1, $2, $3)"
	}
	_, err = tx.ExecContext(ctx, insertSQL, migration.Version, migration.Name, appliedAt)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return tx.Commit()
}

func extractUpMigration(content string) string {
	idx := strings.Index(content, "-- @down")
	if idx == -1 {
		return content
	}
	return strings.TrimSpace(content[:idx])
}

func splitSQLStatements(content string) []string {
	statements := strings.Split(content, ";")
	result := []string{}
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt != "" {
			result = append(result, stmt)
		}
	}
	return result
}

func (m *MigrationRunner) Revert(ctx context.Context, n int) (int, error) {
	appliedMigrations := []*Migration{}
	for _, migration := range m.migrations {
		if migration.Applied {
			appliedMigrations = append(appliedMigrations, migration)
		}
	}

	if len(appliedMigrations) == 0 {
		return 0, nil
	}

	sort.Slice(appliedMigrations, func(i, j int) bool {
		return appliedMigrations[i].Version > appliedMigrations[j].Version
	})

	if n > len(appliedMigrations) {
		n = len(appliedMigrations)
	}

	revertedCount := 0
	for i := 0; i < n; i++ {
		migration := appliedMigrations[i]

		err := m.revertMigration(ctx, migration)
		if err != nil {
			return revertedCount, fmt.Errorf("failed to revert migration %d: %w", migration.Version, err)
		}

		migration.Applied = false
		revertedCount++
	}

	return revertedCount, nil
}

func (m *MigrationRunner) revertMigration(ctx context.Context, migration *Migration) error {
	downContent := extractDownMigration(migration.Content)
	if downContent == "" {
		return fmt.Errorf("no down migration found for %d", migration.Version)
	}

	tx, err := m.service.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	statements := splitSQLStatements(downContent)
	for i, stmt := range statements {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		_, err = tx.ExecContext(ctx, stmt)
		if err != nil {
			return fmt.Errorf("down statement %d failed: %w", i+1, err)
		}
	}

	var deleteSQL string
	if m.service.Driver() == DriverSQLite {
		deleteSQL = "DELETE FROM _migrations WHERE version = ?"
	} else {
		deleteSQL = "DELETE FROM _migrations WHERE version = $1"
	}
	_, err = tx.ExecContext(ctx, deleteSQL, migration.Version)
	if err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	return tx.Commit()
}

func extractDownMigration(content string) string {
	re := regexp.MustCompile(`(?s)--\s*@down\s*\n(.+)`)
	matches := re.FindStringSubmatch(content)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func (m *MigrationRunner) Status(ctx context.Context) ([]MigrationStatus, error) {
	status := []MigrationStatus{}

	for _, migration := range m.migrations {
		statusStr := "pending"
		if migration.Applied {
			statusStr = "applied"
		}
		status = append(status, MigrationStatus{
			Version:   migration.Version,
			Name:      migration.Name,
			Checksum:  migration.Checksum,
			Status:    statusStr,
			AppliedAt: migration.AppliedAt,
		})
	}

	return status, nil
}

type MigrationStatus struct {
	Version   int64
	Name      string
	Checksum  string
	Status    string
	AppliedAt time.Time
}

func (m *MigrationRunner) PendingCount() int {
	count := 0
	for _, migration := range m.migrations {
		if !migration.Applied {
			count++
		}
	}
	return count
}

func (m *MigrationRunner) AppliedCount() int {
	count := 0
	for _, migration := range m.migrations {
		if migration.Applied {
			count++
		}
	}
	return count
}

func (m *MigrationRunner) TotalCount() int {
	return len(m.migrations)
}

func (m *MigrationRunner) VerifyChecksum(ctx context.Context) ([]string, error) {
	rows, err := m.service.Query(ctx, "SELECT version, name FROM _migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tampered := []string{}
	for rows.Next() {
		var version int64
		var name string
		if err := rows.Scan(&version, &name); err != nil {
			return nil, err
		}

		for _, migration := range m.migrations {
			if migration.Version == version && migration.Name != name {
				tampered = append(tampered, strconv.FormatInt(version, 10))
			}
		}
	}

	return tampered, rows.Err()
}
