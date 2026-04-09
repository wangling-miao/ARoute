package ddl

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type MigrationRecord struct {
	ID            int64     `json:"id"`
	ContentType   string    `json:"content_type"`
	Operation     string    `json:"operation"`
	SQLExecuted   string    `json:"sql_executed"`
	SchemaVersion int       `json:"schema_version"`
	ExecutedAt    time.Time `json:"executed_at"`
	Success       bool      `json:"success"`
	ErrorMessage  string    `json:"error_message,omitempty"`
}

type MigrationTracker struct {
	db interfaces.DatabaseService
}

func NewMigrationTracker(db interfaces.DatabaseService) *MigrationTracker {
	return &MigrationTracker{db: db}
}

func (t *MigrationTracker) Init(ctx context.Context) error {
	_, err := t.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS _schema_migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content_type VARCHAR(255) NOT NULL,
			operation VARCHAR(50) NOT NULL,
			sql_executed TEXT NOT NULL,
			schema_version INTEGER NOT NULL,
			executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			success BOOLEAN NOT NULL DEFAULT true,
			error_message TEXT
		)
	`)
	if err != nil {
		return fmt.Errorf("creating _schema_migrations table: %w", err)
	}

	_, err = t.db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_schema_migrations_content_type 
		ON _schema_migrations(content_type)
	`)
	if err != nil {
		return fmt.Errorf("creating migration index: %w", err)
	}

	return nil
}

func (t *MigrationTracker) Record(ctx context.Context, contentType, operation, sqlExecuted string, success bool, errMsg string) error {
	version, err := t.GetNextVersion(ctx, contentType)
	if err != nil {
		return fmt.Errorf("getting next version: %w", err)
	}

	_, err = t.db.Exec(ctx, `
		INSERT INTO _schema_migrations 
		(content_type, operation, sql_executed, schema_version, executed_at, success, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, contentType, operation, sqlExecuted, version, time.Now(), success, errMsg)

	if err != nil {
		return fmt.Errorf("recording migration: %w", err)
	}

	return nil
}

func (t *MigrationTracker) GetNextVersion(ctx context.Context, contentType string) (int, error) {
	var maxVersion sql.NullInt64
	err := t.db.QueryRow(ctx, `
		SELECT MAX(schema_version) FROM _schema_migrations 
		WHERE content_type = ? AND success = true
	`, contentType).Scan(&maxVersion)

	if err != nil {
		return 0, fmt.Errorf("querying max version: %w", err)
	}

	if !maxVersion.Valid {
		return 1, nil
	}

	return int(maxVersion.Int64) + 1, nil
}

func (t *MigrationTracker) GetCurrentVersion(ctx context.Context, contentType string) (int, error) {
	var maxVersion sql.NullInt64
	err := t.db.QueryRow(ctx, `
		SELECT MAX(schema_version) FROM _schema_migrations 
		WHERE content_type = ? AND success = true
	`, contentType).Scan(&maxVersion)

	if err != nil {
		return 0, fmt.Errorf("querying current version: %w", err)
	}

	if !maxVersion.Valid {
		return 0, nil
	}

	return int(maxVersion.Int64), nil
}

func (t *MigrationTracker) GetMigrationHistory(ctx context.Context, contentType string) ([]MigrationRecord, error) {
	rows, err := t.db.Query(ctx, `
		SELECT id, content_type, operation, sql_executed, schema_version, executed_at, success, error_message
		FROM _schema_migrations
		WHERE content_type = ?
		ORDER BY executed_at ASC
	`, contentType)

	if err != nil {
		return nil, fmt.Errorf("querying migration history: %w", err)
	}
	defer rows.Close()

	var records []MigrationRecord
	for rows.Next() {
		var r MigrationRecord
		var errMsg sql.NullString

		err := rows.Scan(
			&r.ID, &r.ContentType, &r.Operation, &r.SQLExecuted,
			&r.SchemaVersion, &r.ExecutedAt, &r.Success, &errMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning migration record: %w", err)
		}

		if errMsg.Valid {
			r.ErrorMessage = errMsg.String
		}

		records = append(records, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating migration records: %w", err)
	}

	return records, nil
}

func (t *MigrationTracker) GetAllMigrations(ctx context.Context) ([]MigrationRecord, error) {
	rows, err := t.db.Query(ctx, `
		SELECT id, content_type, operation, sql_executed, schema_version, executed_at, success, error_message
		FROM _schema_migrations
		ORDER BY executed_at ASC
	`)

	if err != nil {
		return nil, fmt.Errorf("querying all migrations: %w", err)
	}
	defer rows.Close()

	var records []MigrationRecord
	for rows.Next() {
		var r MigrationRecord
		var errMsg sql.NullString

		err := rows.Scan(
			&r.ID, &r.ContentType, &r.Operation, &r.SQLExecuted,
			&r.SchemaVersion, &r.ExecutedAt, &r.Success, &errMsg,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning migration record: %w", err)
		}

		if errMsg.Valid {
			r.ErrorMessage = errMsg.String
		}

		records = append(records, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating migration records: %w", err)
	}

	return records, nil
}

func (t *MigrationTracker) ToJSON(records []MigrationRecord) ([]byte, error) {
	return json.MarshalIndent(records, "", "  ")
}
