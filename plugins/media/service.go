package media

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

const maxFileSize = 50 * 1024 * 1024

var allowedTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/gif":       true,
	"image/webp":      true,
	"image/bmp":       true,
	"image/svg+xml":   true,
	"video/mp4":       true,
	"video/webm":      true,
	"audio/mpeg":      true,
	"audio/ogg":       true,
	"audio/wav":       true,
	"application/pdf": true,
}

type Service struct {
	store   *Store
	storage StorageBackend
	events  core.EventBus
	logger  *slog.Logger
}

func NewService(store *Store, storage StorageBackend, ev core.EventBus, logger *slog.Logger) *Service {
	return &Service{
		store:   store,
		storage: storage,
		events:  ev,
		logger:  logger,
	}
}

func (s *Service) Upload(ctx context.Context, file multipart.File, header *multipart.FileHeader, uploaderID string) (*interfaces.MediaFile, error) {
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read file data: %w", err)
	}

	if int64(len(data)) > maxFileSize {
		return nil, fmt.Errorf("file size %d exceeds maximum allowed size %d: %w",
			len(data), maxFileSize, interfaces.ErrValidation)
	}

	sniffLen := len(data)
	if sniffLen > 512 {
		sniffLen = 512
	}
	mimeType := http.DetectContentType(data[:sniffLen])

	if !allowedTypes[mimeType] {
		return nil, fmt.Errorf("mime type %q is not allowed: %w", mimeType, interfaces.ErrValidation)
	}

	now := time.Now().UTC()
	ext := filepath.Ext(header.Filename)
	storagePath := filepath.Join(
		now.Format("2006"),
		now.Format("01"),
		now.Format("02"),
		uuid.New().String()+ext,
	)

	if err := s.storage.Save(ctx, data, storagePath); err != nil {
		return nil, fmt.Errorf("save file to storage: %w", err)
	}

	var width, height int
	if isImageMIME(mimeType) {
		w, h, err := decodeImageConfig(data)
		if err == nil {
			width = w
			height = h
		}
	}

	mf := &interfaces.MediaFile{
		Filename:      header.Filename,
		MIMEType:      mimeType,
		Size:          int64(len(data)),
		Width:         width,
		Height:        height,
		StoragePath:   storagePath,
		StorageType:   s.storage.Type(),
		UploaderID:    uploaderID,
		ThumbnailPath: "",
	}

	if err := s.store.Create(ctx, mf); err != nil {
		return nil, fmt.Errorf("save media metadata: %w", err)
	}

	s.emitEvent(ctx, "media.uploaded", map[string]interface{}{
		"id":          mf.ID,
		"filename":    mf.Filename,
		"mime_type":   mf.MIMEType,
		"size":        mf.Size,
		"uploader_id": mf.UploaderID,
	})

	return mf, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*interfaces.MediaFile, error) {
	return s.store.GetByID(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	mf, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := s.storage.Delete(ctx, mf.StoragePath); err != nil {
		s.logger.Warn("failed to delete file from storage", "path", mf.StoragePath, "error", err)
	}

	if mf.ThumbnailPath != "" {
		if err := s.storage.Delete(ctx, mf.ThumbnailPath); err != nil {
			s.logger.Warn("failed to delete thumbnail from storage", "path", mf.ThumbnailPath, "error", err)
		}
	}

	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}

	s.emitEvent(ctx, "media.deleted", map[string]interface{}{
		"id":           id,
		"filename":     mf.Filename,
		"storage_path": mf.StoragePath,
	})

	return nil
}

func (s *Service) List(ctx context.Context, query *interfaces.ListQuery) (*interfaces.Page, error) {
	if query == nil {
		query = &interfaces.ListQuery{}
	}

	items, total, err := s.store.List(ctx, query)
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

func (s *Service) GetURL(ctx context.Context, id string) (string, error) {
	mf, err := s.store.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	return s.storage.GetURL(ctx, mf.StoragePath)
}

func (s *Service) GenerateThumbnail(ctx context.Context, id string, width, height int) (string, error) {
	mf, err := s.store.GetByID(ctx, id)
	if err != nil {
		return "", err
	}

	if !isImageMIME(mf.MIMEType) {
		return "", fmt.Errorf("thumbnail generation not supported for mime type %q: %w",
			mf.MIMEType, interfaces.ErrValidation)
	}

	data, err := s.storage.Get(ctx, mf.StoragePath)
	if err != nil {
		return "", fmt.Errorf("get original file: %w", err)
	}

	thumbnailData, err := generateThumbnail(data, width, height)
	if err != nil {
		return "", fmt.Errorf("generate thumbnail: %w", err)
	}

	ext := filepath.Ext(mf.StoragePath)
	thumbFilename := strings.TrimSuffix(filepath.Base(mf.StoragePath), ext) + ".jpg"
	thumbPath := filepath.Join(
		"thumbnails",
		filepath.Dir(mf.StoragePath),
		thumbFilename,
	)

	if err := s.storage.Save(ctx, thumbnailData, thumbPath); err != nil {
		return "", fmt.Errorf("save thumbnail: %w", err)
	}

	if err := s.store.UpdateThumbnail(ctx, id, thumbPath); err != nil {
		return "", fmt.Errorf("update thumbnail path: %w", err)
	}

	s.emitEvent(ctx, "media.thumbnail_generated", map[string]interface{}{
		"id":             id,
		"thumbnail_path": thumbPath,
		"width":          width,
		"height":         height,
	})

	return thumbPath, nil
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
