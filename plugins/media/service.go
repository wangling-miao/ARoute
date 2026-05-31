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

var extMIME = map[string]string{
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".doc":  "application/msword",
	".xls":  "application/vnd.ms-excel",
	".ppt":  "application/vnd.ms-powerpoint",
}

func mimeByExt(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	return extMIME[ext]
}

var blockedTypes = map[string]bool{
	"application/x-executable":    true,
	"application/x-msdos-program": true,
	"application/x-sh":            true,
	"application/x-bat":           true,
}

var allowedTypes = map[string]bool{
	"application/pdf": true,
}

func init() {
	for _, mimeType := range extMIME {
		allowedTypes[mimeType] = true
	}
}

func isAllowedUploadMIME(mimeType string) bool {
	if blockedTypes[mimeType] {
		return false
	}
	if isImageMIME(mimeType) {
		return true
	}
	if strings.HasPrefix(mimeType, "video/") || strings.HasPrefix(mimeType, "audio/") {
		return true
	}
	return allowedTypes[mimeType]
}

type Service struct {
	store     *Store
	storage   StorageBackend
	events    core.EventBus
	logger    *slog.Logger
	stopClean chan struct{}
}

func NewService(store *Store, storage StorageBackend, ev core.EventBus, logger *slog.Logger) *Service {
	return &Service{
		store:   store,
		storage: storage,
		events:  ev,
		logger:  logger,
	}
}

// StartCleanup launches a background goroutine that periodically removes
// database records whose physical files no longer exist on disk.
func (s *Service) StartCleanup(interval time.Duration) {
	if s.stopClean != nil {
		return
	}
	s.stopClean = make(chan struct{})
	go s.cleanupLoop(interval)
}

// StopCleanup signals the background cleanup goroutine to exit.
func (s *Service) StopCleanup() {
	if s.stopClean != nil {
		close(s.stopClean)
		s.stopClean = nil
	}
}

func (s *Service) cleanupLoop(interval time.Duration) {
	// Run once immediately on start, then on interval.
	s.cleanMissingFiles()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanMissingFiles()
		case <-s.stopClean:
			return
		}
	}
}

func (s *Service) cleanMissingFiles() {
	ctx := context.Background()

	paths, err := s.store.ListAllStoragePaths(ctx)
	if err != nil {
		s.logger.Error("cleanup: failed to list storage paths", "error", err)
		return
	}

	if len(paths) == 0 {
		return
	}

	removed := 0
	for _, p := range paths {
		if _, err := s.storage.Get(ctx, p); err != nil {
			s.logger.Info("cleanup: removing orphaned media record", "path", p)
			if delErr := s.store.DeleteByStoragePath(ctx, p); delErr != nil {
				s.logger.Error("cleanup: failed to delete record", "path", p, "error", delErr)
			} else {
				removed++
			}
		}
	}

	if removed > 0 {
		s.logger.Info("cleanup: removed orphaned media records", "count", removed)
	}
}

func (s *Service) Upload(ctx context.Context, reader io.Reader, filename string, contentType string, size int64, uploaderID string) (*interfaces.MediaFile, error) {
	lr := io.LimitReader(reader, maxFileSize+1)
	data, err := io.ReadAll(lr)
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
	mimeType, _, _ = strings.Cut(mimeType, ";")

	// http.DetectContentType returns application/zip for Office files (docx, xlsx, pptx).
	// Correct based on the file extension.
	if mimeType == "application/zip" || mimeType == "application/octet-stream" {
		if corrected := mimeByExt(filename); corrected != "" {
			mimeType = corrected
		}
	}

	if !isAllowedUploadMIME(mimeType) {
		return nil, fmt.Errorf("mime type %q is not allowed: %w", mimeType, interfaces.ErrValidation)
	}

	now := time.Now().UTC()
	ext := filepath.Ext(filename)
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
		Filename:      filename,
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

// UploadMultipart handles file upload from HTTP multipart forms.
// It extracts the file from the multipart header and delegates to Upload.
func (s *Service) UploadMultipart(ctx context.Context, file multipart.File, header *multipart.FileHeader, uploaderID string) (*interfaces.MediaFile, error) {
	return s.Upload(ctx, file, header.Filename, header.Header.Get("Content-Type"), header.Size, uploaderID)
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

// UploadFromReader stores a file from an io.Reader, avoiding HTTP multipart coupling.
func (s *Service) UploadFromReader(ctx context.Context, reader io.Reader, filename string, contentType string, uploaderID string) (*interfaces.MediaFile, error) {
	lr := io.LimitReader(reader, maxFileSize+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("read file data: %w", err)
	}

	if int64(len(data)) > maxFileSize {
		return nil, fmt.Errorf("file size exceeds maximum allowed size %d: %w", maxFileSize, interfaces.ErrValidation)
	}

	mimeType := contentType
	if mimeType == "" {
		sniffLen := len(data)
		if sniffLen > 512 {
			sniffLen = 512
		}
		mimeType = http.DetectContentType(data[:sniffLen])
	}
	mimeType, _, _ = strings.Cut(mimeType, ";")

	if mimeType == "application/zip" || mimeType == "application/octet-stream" {
		if corrected := mimeByExt(filename); corrected != "" {
			mimeType = corrected
		}
	}

	if !isAllowedUploadMIME(mimeType) {
		return nil, fmt.Errorf("mime type %q is not allowed: %w", mimeType, interfaces.ErrValidation)
	}

	now := time.Now().UTC()
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".bin"
	}
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
		Filename:      filename,
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

func (s *Service) emitEvent(ctx context.Context, topic string, data map[string]interface{}) {
	if s.events == nil {
		return
	}
	s.events.Emit(ctx, events.Event{
		Topic: topic,
		Data:  data,
	})
}
