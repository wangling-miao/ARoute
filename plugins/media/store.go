package media

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

var mediaSortColRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

type Store struct {
	db interfaces.DatabaseService
}

func NewStore(db interfaces.DatabaseService) *Store {
	return &Store{db: db}
}

func (s *Store) CreateTables(ctx context.Context) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS _media (
			id TEXT PRIMARY KEY,
			filename TEXT NOT NULL,
			mime_type TEXT NOT NULL,
			size INTEGER NOT NULL,
			width INTEGER NOT NULL DEFAULT 0,
			height INTEGER NOT NULL DEFAULT 0,
			storage_path TEXT NOT NULL,
			storage_type TEXT NOT NULL DEFAULT 'local',
			uploader_id TEXT NOT NULL DEFAULT '',
			thumbnail_path TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_media_mime_type ON _media(mime_type)`,
		`CREATE INDEX IF NOT EXISTS idx_media_uploader_id ON _media(uploader_id)`,
		`CREATE INDEX IF NOT EXISTS idx_media_created_at ON _media(created_at)`,
	}
	for _, table := range tables {
		if _, err := s.db.Exec(ctx, table); err != nil {
			return fmt.Errorf("create media table: %w", err)
		}
	}
	return nil
}

func (s *Store) Create(ctx context.Context, mf *interfaces.MediaFile) error {
	if mf.ID == "" {
		mf.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(ctx,
		`INSERT INTO _media (id, filename, mime_type, size, width, height, storage_path, storage_type, uploader_id, thumbnail_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		mf.ID, mf.Filename, mf.MIMEType, mf.Size, mf.Width, mf.Height,
		mf.StoragePath, mf.StorageType, mf.UploaderID, mf.ThumbnailPath, now,
	)
	if err != nil {
		return fmt.Errorf("insert media file: %w", err)
	}
	return nil
}

func (s *Store) GetByID(ctx context.Context, id string) (*interfaces.MediaFile, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, filename, mime_type, size, width, height, storage_path, storage_type, uploader_id, thumbnail_path, created_at
		 FROM _media WHERE id = ?`, id,
	)
	return s.scanMediaFile(row)
}

func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.Exec(ctx, `DELETE FROM _media WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete media file: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return interfaces.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateThumbnail(ctx context.Context, id string, thumbnailPath string) error {
	res, err := s.db.Exec(ctx,
		`UPDATE _media SET thumbnail_path = ? WHERE id = ?`,
		thumbnailPath, id,
	)
	if err != nil {
		return fmt.Errorf("update media thumbnail: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return interfaces.ErrNotFound
	}
	return nil
}

func (s *Store) List(ctx context.Context, query *interfaces.ListQuery) ([]*interfaces.MediaFile, int64, error) {
	var whereClauses []string
	var args []interface{}

	if query != nil && len(query.Filters) > 0 {
		for field, value := range query.Filters {
			switch field {
			case "mime_type":
				whereClauses = append(whereClauses, "mime_type = ?")
				args = append(args, value)
			case "uploader_id":
				whereClauses = append(whereClauses, "uploader_id = ?")
				args = append(args, value)
			}
		}
	}

	whereStr := "1=1"
	if len(whereClauses) > 0 {
		whereStr = strings.Join(whereClauses, " AND ")
	}

	countQuery := `SELECT COUNT(*) FROM _media WHERE ` + whereStr
	var total int64
	row := s.db.QueryRow(ctx, countQuery, args...)
	if err := row.Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count media files: %w", err)
	}

	sortCol := "created_at"
	sortOrder := "DESC"
	if query != nil {
		if query.Sort != "" && mediaSortColRegex.MatchString(query.Sort) {
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

	selectQuery := fmt.Sprintf(
		`SELECT id, filename, mime_type, size, width, height, storage_path, storage_type, uploader_id, thumbnail_path, created_at
		 FROM _media WHERE %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		whereStr, sortCol, sortOrder,
	)
	args = append(args, perPage, offset)

	rows, err := s.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list media files: %w", err)
	}
	defer rows.Close()

	var items []*interfaces.MediaFile
	for rows.Next() {
		mf, err := s.scanMediaFileRow(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, mf)
	}

	return items, total, rows.Err()
}

func (s *Store) scanMediaFile(row *sql.Row) (*interfaces.MediaFile, error) {
	var mf interfaces.MediaFile
	var createdAt string

	err := row.Scan(
		&mf.ID, &mf.Filename, &mf.MIMEType, &mf.Size,
		&mf.Width, &mf.Height, &mf.StoragePath, &mf.StorageType,
		&mf.UploaderID, &mf.ThumbnailPath, &createdAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, interfaces.ErrNotFound
		}
		return nil, fmt.Errorf("scan media file: %w", err)
	}

	mf.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &mf, nil
}

func (s *Store) scanMediaFileRow(rows *sql.Rows) (*interfaces.MediaFile, error) {
	var mf interfaces.MediaFile
	var createdAt string

	err := rows.Scan(
		&mf.ID, &mf.Filename, &mf.MIMEType, &mf.Size,
		&mf.Width, &mf.Height, &mf.StoragePath, &mf.StorageType,
		&mf.UploaderID, &mf.ThumbnailPath, &createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan media file row: %w", err)
	}

	mf.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &mf, nil
}
