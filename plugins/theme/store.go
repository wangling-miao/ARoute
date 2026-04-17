package theme

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type ThemeRecord struct {
	ID          string
	Name        string
	Slug        string
	Version     string
	Engine      string
	Active      bool
	InstalledAt time.Time
	Settings    string
}

type Store struct {
	db interfaces.DatabaseService
}

func NewStore(db interfaces.DatabaseService) *Store {
	return &Store{db: db}
}

func (s *Store) CreateTables(ctx context.Context) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS _themes (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			version TEXT NOT NULL,
			engine TEXT NOT NULL,
			active INTEGER NOT NULL DEFAULT 0,
			installed_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
			settings TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_themes_slug ON _themes(slug)`,
		`CREATE INDEX IF NOT EXISTS idx_themes_active ON _themes(active)`,
	}
	for _, table := range tables {
		if _, err := s.db.Exec(ctx, table); err != nil {
			return fmt.Errorf("create themes table: %w", err)
		}
	}
	return nil
}

func (s *Store) Create(ctx context.Context, theme *ThemeRecord) error {
	if theme.ID == "" {
		theme.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(ctx,
		`INSERT INTO _themes (id, name, slug, version, engine, active, installed_at, settings)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		theme.ID, theme.Name, theme.Slug, theme.Version, theme.Engine, theme.Active, now, theme.Settings,
	)
	if err != nil {
		return fmt.Errorf("insert theme: %w", err)
	}
	return nil
}

func (s *Store) GetBySlug(ctx context.Context, slug string) (*ThemeRecord, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, name, slug, version, engine, active, installed_at, settings
		 FROM _themes WHERE slug = ?`, slug,
	)
	return s.scanTheme(row)
}

func (s *Store) GetActive(ctx context.Context) (*ThemeRecord, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, name, slug, version, engine, active, installed_at, settings
		 FROM _themes WHERE active = 1 LIMIT 1`,
	)
	return s.scanTheme(row)
}

func (s *Store) List(ctx context.Context) ([]*ThemeRecord, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, name, slug, version, engine, active, installed_at, settings
		 FROM _themes ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list themes: %w", err)
	}
	defer rows.Close()

	var items []*ThemeRecord
	for rows.Next() {
		rec, err := s.scanThemeRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, rec)
	}

	return items, rows.Err()
}

func (s *Store) SetActive(ctx context.Context, slug string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE _themes SET active = 0`); err != nil {
		return fmt.Errorf("deactivate all themes: %w", err)
	}

	res, err := tx.ExecContext(ctx, `UPDATE _themes SET active = 1 WHERE slug = ?`, slug)
	if err != nil {
		return fmt.Errorf("activate theme %q: %w", slug, err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return interfaces.ErrNotFound
	}

	return tx.Commit()
}

func (s *Store) Delete(ctx context.Context, slug string) error {
	res, err := s.db.Exec(ctx, `DELETE FROM _themes WHERE slug = ?`, slug)
	if err != nil {
		return fmt.Errorf("delete theme: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return interfaces.ErrNotFound
	}
	return nil
}

func (s *Store) scanTheme(row *sql.Row) (*ThemeRecord, error) {
	var rec ThemeRecord
	var installedAt string
	var active int

	err := row.Scan(
		&rec.ID, &rec.Name, &rec.Slug, &rec.Version,
		&rec.Engine, &active, &installedAt, &rec.Settings,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, interfaces.ErrNotFound
		}
		return nil, fmt.Errorf("scan theme: %w", err)
	}

	rec.Active = active == 1
	rec.InstalledAt, _ = time.Parse(time.RFC3339, installedAt)
	return &rec, nil
}

func (s *Store) scanThemeRow(rows *sql.Rows) (*ThemeRecord, error) {
	var rec ThemeRecord
	var installedAt string
	var active int

	err := rows.Scan(
		&rec.ID, &rec.Name, &rec.Slug, &rec.Version,
		&rec.Engine, &active, &installedAt, &rec.Settings,
	)
	if err != nil {
		return nil, fmt.Errorf("scan theme row: %w", err)
	}

	rec.Active = active == 1
	rec.InstalledAt, _ = time.Parse(time.RFC3339, installedAt)
	return &rec, nil
}
