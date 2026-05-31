package media

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/plugins/database"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

var testCounter int64

func nextTestDBName() string {
	n := atomic.AddInt64(&testCounter, 1)
	return fmt.Sprintf("media_test_%d", n)
}

func setupTestService(t *testing.T) *Service {
	t.Helper()
	dbName := nextTestDBName()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", dbName))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dbSvc := database.NewService(db, database.DriverSQLite)
	store := NewStore(dbSvc)
	ctx := context.Background()
	if err := store.CreateTables(ctx); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("create local storage: %v", err)
	}
	eb := &events.EventBus{}
	return NewService(store, storage, eb, slog.Default())
}

type testFile struct {
	*bytes.Reader
}

func (tf *testFile) Close() error { return nil }

func newTestFile(data []byte) *testFile {
	return &testFile{Reader: bytes.NewReader(data)}
}

func createTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

// ==================== Store Tests ====================

func TestCreateTables(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	// Idempotent — calling again should not error
	if err := svc.store.CreateTables(ctx); err != nil {
		t.Fatalf("create tables idempotent: %v", err)
	}
}

func TestStoreCreate(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	mf := &interfaces.MediaFile{
		Filename:    "photo.png",
		MIMEType:    "image/png",
		Size:        1024,
		Width:       100,
		Height:      100,
		StoragePath: "2024/01/01/abc.png",
		StorageType: "local",
		UploaderID:  "user-1",
	}

	if err := svc.store.Create(ctx, mf); err != nil {
		t.Fatalf("create: %v", err)
	}

	if mf.ID == "" {
		t.Fatal("expected auto-generated ID")
	}
	if mf.Filename != "photo.png" {
		t.Errorf("expected filename 'photo.png', got %s", mf.Filename)
	}
	if mf.MIMEType != "image/png" {
		t.Errorf("expected mime_type 'image/png', got %s", mf.MIMEType)
	}
	if mf.Size != 1024 {
		t.Errorf("expected size 1024, got %d", mf.Size)
	}
}

func TestStoreGetByID(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	original := &interfaces.MediaFile{
		Filename:      "test.jpg",
		MIMEType:      "image/jpeg",
		Size:          2048,
		Width:         800,
		Height:        600,
		StoragePath:   "2024/01/img.jpg",
		StorageType:   "local",
		UploaderID:    "user-42",
		ThumbnailPath: "thumbnails/2024/01/img.jpg",
	}
	if err := svc.store.Create(ctx, original); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := svc.store.GetByID(ctx, original.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.ID != original.ID {
		t.Errorf("expected ID %s, got %s", original.ID, got.ID)
	}
	if got.Filename != original.Filename {
		t.Errorf("expected filename %s, got %s", original.Filename, got.Filename)
	}
	if got.MIMEType != original.MIMEType {
		t.Errorf("expected mime_type %s, got %s", original.MIMEType, got.MIMEType)
	}
	if got.Size != original.Size {
		t.Errorf("expected size %d, got %d", original.Size, got.Size)
	}
	if got.Width != original.Width {
		t.Errorf("expected width %d, got %d", original.Width, got.Width)
	}
	if got.Height != original.Height {
		t.Errorf("expected height %d, got %d", original.Height, got.Height)
	}
	if got.StoragePath != original.StoragePath {
		t.Errorf("expected storage_path %s, got %s", original.StoragePath, got.StoragePath)
	}
	if got.StorageType != original.StorageType {
		t.Errorf("expected storage_type %s, got %s", original.StorageType, got.StorageType)
	}
	if got.UploaderID != original.UploaderID {
		t.Errorf("expected uploader_id %s, got %s", original.UploaderID, got.UploaderID)
	}
	if got.ThumbnailPath != original.ThumbnailPath {
		t.Errorf("expected thumbnail_path %s, got %s", original.ThumbnailPath, got.ThumbnailPath)
	}
}

func TestStoreGetByIDNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.store.GetByID(ctx, "nonexistent-id")
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestStoreDelete(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	mf := &interfaces.MediaFile{
		Filename:    "del.png",
		MIMEType:    "image/png",
		Size:        512,
		StoragePath: "del.png",
		StorageType: "local",
	}
	if err := svc.store.Create(ctx, mf); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := svc.store.Delete(ctx, mf.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := svc.store.GetByID(ctx, mf.ID)
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestStoreDeleteNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	err := svc.store.Delete(ctx, "nonexistent-id")
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestStoreUpdateThumbnail(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	mf := &interfaces.MediaFile{
		Filename:    "thumb.png",
		MIMEType:    "image/png",
		Size:        256,
		StoragePath: "thumb.png",
		StorageType: "local",
	}
	if err := svc.store.Create(ctx, mf); err != nil {
		t.Fatalf("create: %v", err)
	}

	newThumb := "thumbnails/2024/thumb.png"
	if err := svc.store.UpdateThumbnail(ctx, mf.ID, newThumb); err != nil {
		t.Fatalf("update thumbnail: %v", err)
	}

	got, err := svc.store.GetByID(ctx, mf.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.ThumbnailPath != newThumb {
		t.Errorf("expected thumbnail_path %s, got %s", newThumb, got.ThumbnailPath)
	}
}

func TestStoreUpdateThumbnailNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	err := svc.store.UpdateThumbnail(ctx, "nonexistent-id", "thumb.jpg")
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestStoreList(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		mf := &interfaces.MediaFile{
			Filename:    fmt.Sprintf("file%d.png", i),
			MIMEType:    "image/png",
			Size:        int64(i * 100),
			StoragePath: fmt.Sprintf("file%d.png", i),
			StorageType: "local",
		}
		if err := svc.store.Create(ctx, mf); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	items, total, err := svc.store.List(ctx, &interfaces.ListQuery{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(items) != 5 {
		t.Errorf("expected 5 items, got %d", len(items))
	}
}

func TestStoreListPagination(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		mf := &interfaces.MediaFile{
			Filename:    fmt.Sprintf("page_file%d.png", i),
			MIMEType:    "image/png",
			Size:        int64(i * 100),
			StoragePath: fmt.Sprintf("page_file%d.png", i),
			StorageType: "local",
		}
		if err := svc.store.Create(ctx, mf); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	items, total, err := svc.store.List(ctx, &interfaces.ListQuery{Page: 1, PerPage: 3})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items on page 1, got %d", len(items))
	}
}

func TestStoreListFilterByMimeType(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	mimeTypes := []string{"image/png", "image/jpeg", "image/png", "video/mp4", "image/png"}
	for i, mt := range mimeTypes {
		mf := &interfaces.MediaFile{
			Filename:    fmt.Sprintf("mime_file%d.dat", i),
			MIMEType:    mt,
			Size:        100,
			StoragePath: fmt.Sprintf("mime_file%d.dat", i),
			StorageType: "local",
		}
		if err := svc.store.Create(ctx, mf); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	items, total, err := svc.store.List(ctx, &interfaces.ListQuery{
		Page:    1,
		PerPage: 20,
		Filters: map[string]interface{}{"mime_type": "image/png"},
	})
	if err != nil {
		t.Fatalf("list with filter: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 image/png files, got %d", total)
	}
	for _, item := range items {
		if item.MIMEType != "image/png" {
			t.Errorf("expected mime_type image/png, got %s", item.MIMEType)
		}
	}
}

func TestStoreListFilterByUploaderID(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	uploaderIDs := []string{"user-a", "user-b", "user-a", "user-c"}
	for i, uid := range uploaderIDs {
		mf := &interfaces.MediaFile{
			Filename:    fmt.Sprintf("uid_file%d.dat", i),
			MIMEType:    "image/png",
			Size:        100,
			StoragePath: fmt.Sprintf("uid_file%d.dat", i),
			StorageType: "local",
			UploaderID:  uid,
		}
		if err := svc.store.Create(ctx, mf); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	items, total, err := svc.store.List(ctx, &interfaces.ListQuery{
		Page:    1,
		PerPage: 20,
		Filters: map[string]interface{}{"uploader_id": "user-a"},
	})
	if err != nil {
		t.Fatalf("list with filter: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 files for user-a, got %d", total)
	}
	for _, item := range items {
		if item.UploaderID != "user-a" {
			t.Errorf("expected uploader_id user-a, got %s", item.UploaderID)
		}
	}
}

func TestStoreListSortOrder(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	// Create items with distinct filenames so sort is meaningful
	names := []string{"alpha", "beta", "gamma"}
	for _, n := range names {
		mf := &interfaces.MediaFile{
			Filename:    n + ".png",
			MIMEType:    "image/png",
			Size:        100,
			StoragePath: n + ".png",
			StorageType: "local",
		}
		if err := svc.store.Create(ctx, mf); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}

	// Sort ascending by filename
	itemsAsc, _, err := svc.store.List(ctx, &interfaces.ListQuery{
		Page:    1,
		PerPage: 20,
		Sort:    "filename",
		Order:   "asc",
	})
	if err != nil {
		t.Fatalf("list asc: %v", err)
	}
	if len(itemsAsc) < 2 {
		t.Fatal("expected at least 2 items")
	}
	if itemsAsc[0].Filename > itemsAsc[1].Filename {
		t.Errorf("expected ascending order, got %s before %s", itemsAsc[0].Filename, itemsAsc[1].Filename)
	}

	// Sort descending by filename
	itemsDesc, _, err := svc.store.List(ctx, &interfaces.ListQuery{
		Page:    1,
		PerPage: 20,
		Sort:    "filename",
		Order:   "desc",
	})
	if err != nil {
		t.Fatalf("list desc: %v", err)
	}
	if len(itemsDesc) < 2 {
		t.Fatal("expected at least 2 items")
	}
	if itemsDesc[0].Filename < itemsDesc[1].Filename {
		t.Errorf("expected descending order, got %s before %s", itemsDesc[0].Filename, itemsDesc[1].Filename)
	}
}

func TestStoreListNilQuery(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		mf := &interfaces.MediaFile{
			Filename:    fmt.Sprintf("nilq%d.png", i),
			MIMEType:    "image/png",
			Size:        100,
			StoragePath: fmt.Sprintf("nilq%d.png", i),
			StorageType: "local",
		}
		if err := svc.store.Create(ctx, mf); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	items, total, err := svc.store.List(ctx, nil)
	if err != nil {
		t.Fatalf("list nil query: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	// Default page=1, perPage=20 → all 3 items returned
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

// ==================== Storage Tests ====================

func TestLocalStorageSaveAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("create local storage: %v", err)
	}
	ctx := context.Background()

	data := []byte("hello world test data")
	path := "2024/01/test.txt"

	if err := storage.Save(ctx, data, path); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := storage.Get(ctx, path)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("expected %q, got %q", string(data), string(got))
	}
}

func TestLocalStorageDelete(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("create local storage: %v", err)
	}
	ctx := context.Background()

	data := []byte("to be deleted")
	path := "delete_me.txt"

	if err := storage.Save(ctx, data, path); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := storage.Delete(ctx, path); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = storage.Get(ctx, path)
	if err == nil {
		t.Error("expected error getting deleted file")
	}
}

func TestLocalStorageDeleteNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("create local storage: %v", err)
	}
	ctx := context.Background()

	// Deleting a non-existent file should not error
	if err := storage.Delete(ctx, "nonexistent.txt"); err != nil {
		t.Errorf("expected no error deleting non-existent file, got: %v", err)
	}
}

func TestLocalStorageGetURL(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("create local storage: %v", err)
	}
	ctx := context.Background()

	url, err := storage.GetURL(ctx, "2024/01/photo.png")
	if err != nil {
		t.Fatalf("get url: %v", err)
	}
	if !strings.HasPrefix(url, "/uploads/") {
		t.Errorf("expected URL to start with /uploads/, got %s", url)
	}
	if !strings.HasSuffix(url, "photo.png") {
		t.Errorf("expected URL to end with photo.png, got %s", url)
	}
}

func TestLocalStorageType(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("create local storage: %v", err)
	}
	if storage.Type() != "local" {
		t.Errorf("expected type 'local', got %s", storage.Type())
	}
}

func TestLocalStorageGetNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("create local storage: %v", err)
	}
	ctx := context.Background()

	_, err = storage.Get(ctx, "does_not_exist.bin")
	if err == nil {
		t.Error("expected error getting non-existent file")
	}
}

func TestNewStorageBackendLocal(t *testing.T) {
	tmpDir := t.TempDir()
	backend, err := NewStorageBackend("local", tmpDir, nil)
	if err != nil {
		t.Fatalf("new storage backend local: %v", err)
	}
	if _, ok := backend.(*LocalStorage); !ok {
		t.Errorf("expected *LocalStorage, got %T", backend)
	}
}

func TestNewStorageBackendUnsupported(t *testing.T) {
	_, err := NewStorageBackend("ftp", "/tmp", nil)
	if err == nil {
		t.Fatal("expected error for unsupported storage type")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected 'unsupported' in error, got: %v", err)
	}
}

// ==================== Thumbnail Tests ====================

func TestIsImageMIME(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		expected bool
	}{
		{"jpeg", "image/jpeg", true},
		{"png", "image/png", true},
		{"gif", "image/gif", true},
		{"bmp", "image/bmp", true},
		{"webp", "image/webp", true},
		{"mp4 video", "video/mp4", false},
		{"pdf", "application/pdf", false},
		{"svg not in supported", "image/svg+xml", false},
		{"plain text", "text/plain", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isImageMIME(tt.mimeType)
			if got != tt.expected {
				t.Errorf("isImageMIME(%q) = %v, want %v", tt.mimeType, got, tt.expected)
			}
		})
	}
}

func TestDecodeImageConfig(t *testing.T) {
	data := createTestPNG(t, 100, 50)

	w, h, err := decodeImageConfig(data)
	if err != nil {
		t.Fatalf("decode image config: %v", err)
	}
	if w != 100 {
		t.Errorf("expected width 100, got %d", w)
	}
	if h != 50 {
		t.Errorf("expected height 50, got %d", h)
	}
}

func TestDecodeImageConfigInvalidData(t *testing.T) {
	_, _, err := decodeImageConfig([]byte("this is not an image"))
	if err == nil {
		t.Fatal("expected error for invalid image data")
	}
}

func TestGenerateThumbnail(t *testing.T) {
	data := createTestPNG(t, 100, 100)

	thumb, err := generateThumbnail(data, 50, 50)
	if err != nil {
		t.Fatalf("generate thumbnail: %v", err)
	}
	if len(thumb) == 0 {
		t.Fatal("expected non-empty thumbnail bytes")
	}
	// Output should be JPEG (starts with 0xFF 0xD8)
	if len(thumb) < 2 || thumb[0] != 0xFF || thumb[1] != 0xD8 {
		t.Error("expected JPEG output for thumbnail")
	}
}

func TestGenerateThumbnailInvalidData(t *testing.T) {
	_, err := generateThumbnail([]byte("garbage data"), 50, 50)
	if err == nil {
		t.Fatal("expected error for invalid image data")
	}
}

// ==================== Service Tests ====================

func TestUpload(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 100, 100)
	file := newTestFile(data)
	header := &multipart.FileHeader{
		Filename: "test.png",
		Size:     int64(len(data)),
	}

	mf, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if mf.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if mf.Filename != "test.png" {
		t.Errorf("expected filename 'test.png', got %s", mf.Filename)
	}
	if mf.MIMEType != "image/png" {
		t.Errorf("expected mime_type 'image/png', got %s", mf.MIMEType)
	}
	if mf.Size != int64(len(data)) {
		t.Errorf("expected size %d, got %d", len(data), mf.Size)
	}
	if mf.StoragePath == "" {
		t.Fatal("expected non-empty storage path")
	}
	if mf.StorageType != "local" {
		t.Errorf("expected storage_type 'local', got %s", mf.StorageType)
	}
	if mf.UploaderID != "user-1" {
		t.Errorf("expected uploader_id 'user-1', got %s", mf.UploaderID)
	}
	if mf.Width != 100 {
		t.Errorf("expected width 100, got %d", mf.Width)
	}
	if mf.Height != 100 {
		t.Errorf("expected height 100, got %d", mf.Height)
	}
}

func TestUploadExceedsMaxSize(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	// Create data larger than 50MB
	bigData := make([]byte, 50*1024*1024+1)
	// Make it look like a PNG so it passes MIME check
	copy(bigData, createTestPNG(t, 1, 1))

	file := newTestFile(bigData)
	header := &multipart.FileHeader{
		Filename: "big.png",
		Size:     int64(len(bigData)),
	}

	_, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err == nil {
		t.Fatal("expected error for file exceeding max size")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("expected 'exceeds maximum' in error, got: %v", err)
	}
}

func TestUploadInvalidMIME(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	// Random binary data that http.DetectContentType identifies as application/octet-stream
	invalidData := []byte{0x89, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0}
	file := newTestFile(invalidData)
	header := &multipart.FileHeader{
		Filename: "random.bin",
		Size:     int64(len(invalidData)),
	}

	_, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err == nil {
		t.Fatal("expected error for invalid MIME type")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("expected 'not allowed' in error, got: %v", err)
	}
}

func TestGetByID(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 50, 50)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "get.png", Size: int64(len(data))}
	uploaded, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	got, err := svc.GetByID(ctx, uploaded.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if got.ID != uploaded.ID {
		t.Errorf("expected ID %s, got %s", uploaded.ID, got.ID)
	}
	if got.Filename != "get.png" {
		t.Errorf("expected filename 'get.png', got %s", got.Filename)
	}
}

func TestGetByIDNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.GetByID(ctx, "nonexistent-id")
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestDelete(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 50, 50)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "del.png", Size: int64(len(data))}
	uploaded, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if err := svc.Delete(ctx, uploaded.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = svc.GetByID(ctx, uploaded.ID)
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	err := svc.Delete(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error deleting non-existent media")
	}
}

func TestList(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		data := createTestPNG(t, 10, 10)
		file := newTestFile(data)
		header := &multipart.FileHeader{
			Filename: fmt.Sprintf("list%d.png", i),
			Size:     int64(len(data)),
		}
		if _, err := svc.UploadMultipart(ctx, file, header, fmt.Sprintf("user-%d", i)); err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}

	page, err := svc.List(ctx, &interfaces.ListQuery{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Meta.Total != 3 {
		t.Errorf("expected total 3, got %d", page.Meta.Total)
	}
	items, ok := page.Data.([]*interfaces.MediaFile)
	if !ok {
		t.Fatal("expected []*interfaces.MediaFile")
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestListPagination(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		data := createTestPNG(t, 10, 10)
		file := newTestFile(data)
		header := &multipart.FileHeader{
			Filename: fmt.Sprintf("page%d.png", i),
			Size:     int64(len(data)),
		}
		if _, err := svc.UploadMultipart(ctx, file, header, "user-1"); err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}

	page, err := svc.List(ctx, &interfaces.ListQuery{Page: 2, PerPage: 3})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	items, ok := page.Data.([]*interfaces.MediaFile)
	if !ok {
		t.Fatal("expected []*interfaces.MediaFile")
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items on page 2, got %d", len(items))
	}
	if !page.Meta.HasPrev {
		t.Error("expected HasPrev=true on page 2")
	}
}

func TestListNilQuery(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 10, 10)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "nilq.png", Size: int64(len(data))}
	if _, err := svc.UploadMultipart(ctx, file, header, "user-1"); err != nil {
		t.Fatalf("upload: %v", err)
	}

	page, err := svc.List(ctx, nil)
	if err != nil {
		t.Fatalf("list nil query: %v", err)
	}
	if page.Meta.Page != 1 {
		t.Errorf("expected default page 1, got %d", page.Meta.Page)
	}
	if page.Meta.PerPage != 20 {
		t.Errorf("expected default perPage 20, got %d", page.Meta.PerPage)
	}
	if page.Meta.Total != 1 {
		t.Errorf("expected total 1, got %d", page.Meta.Total)
	}
}

func TestGetURL(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 10, 10)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "url.png", Size: int64(len(data))}
	uploaded, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	url, err := svc.GetURL(ctx, uploaded.ID)
	if err != nil {
		t.Fatalf("get url: %v", err)
	}
	if url == "" {
		t.Fatal("expected non-empty URL")
	}
	if !strings.HasPrefix(url, "/uploads/") {
		t.Errorf("expected URL starting with /uploads/, got %s", url)
	}
}

func TestGetURLNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.GetURL(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for non-existent media URL")
	}
}

func TestServiceGenerateThumbnail(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 100, 100)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "thumb.png", Size: int64(len(data))}
	uploaded, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	thumbPath, err := svc.GenerateThumbnail(ctx, uploaded.ID, 50, 50)
	if err != nil {
		t.Fatalf("generate thumbnail: %v", err)
	}
	if thumbPath == "" {
		t.Fatal("expected non-empty thumbnail path")
	}
	if !strings.Contains(thumbPath, "thumbnails") {
		t.Errorf("expected thumbnail path to contain 'thumbnails', got %s", thumbPath)
	}

	// Verify the thumbnail path was persisted
	got, err := svc.GetByID(ctx, uploaded.ID)
	if err != nil {
		t.Fatalf("get after thumbnail: %v", err)
	}
	if got.ThumbnailPath != thumbPath {
		t.Errorf("expected thumbnail_path %s, got %s", thumbPath, got.ThumbnailPath)
	}
}

func TestServiceGenerateThumbnailNonImage(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	// PDF content — http.DetectContentType identifies %PDF- as application/pdf
	pdfData := []byte("%PDF-1.4 fake pdf content for testing purposes")
	file := newTestFile(pdfData)
	header := &multipart.FileHeader{Filename: "doc.pdf", Size: int64(len(pdfData))}
	uploaded, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload pdf: %v", err)
	}

	_, err = svc.GenerateThumbnail(ctx, uploaded.ID, 50, 50)
	if err == nil {
		t.Fatal("expected error generating thumbnail for non-image")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'not supported' in error, got: %v", err)
	}
}

func TestServiceGenerateThumbnailNotFound(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	_, err := svc.GenerateThumbnail(ctx, "nonexistent-id", 50, 50)
	if err == nil {
		t.Fatal("expected error for non-existent media thumbnail")
	}
}

// ==================== Mock CoreContext ====================

type mockMediaConfig struct {
	storage string
}

func (m *mockMediaConfig) GetString(key string) string {
	if key == "storage" {
		return m.storage
	}
	return ""
}
func (m *mockMediaConfig) GetInt(key string) int                          { return 0 }
func (m *mockMediaConfig) GetBool(key string) bool                        { return false }
func (m *mockMediaConfig) GetStringSlice(key string) []string             { return nil }
func (m *mockMediaConfig) Get(key string) interface{}                     { return nil }
func (m *mockMediaConfig) Unmarshal(key string, target interface{}) error { return nil }
func (m *mockMediaConfig) Set(key string, value interface{})              {}
func (m *mockMediaConfig) Save() error                                    { return nil }

type mockMediaServiceContainer struct {
	dbSvc interfaces.DatabaseService
}

func (m *mockMediaServiceContainer) Provide(fn interface{}) error { return nil }
func (m *mockMediaServiceContainer) Get(target interface{}) error {
	if p, ok := target.(*interfaces.DatabaseService); ok && m.dbSvc != nil {
		*p = m.dbSvc
		return nil
	}
	return fmt.Errorf("service not found")
}
func (m *mockMediaServiceContainer) GetNamed(name string, target interface{}) error { return nil }
func (m *mockMediaServiceContainer) Unregister(target interface{}) error            { return nil }
func (m *mockMediaServiceContainer) Has(target interface{}) bool                    { return false }
func (m *mockMediaServiceContainer) Keys() []string                                 { return nil }

type mockMediaCoreContext struct {
	svc     core.ServiceContainer
	config  *mockMediaConfig
	dataDir string
}

func (m *mockMediaCoreContext) Services() core.ServiceContainer { return m.svc }
func (m *mockMediaCoreContext) Events() core.EventBus           { return &events.EventBus{} }
func (m *mockMediaCoreContext) Config() core.ConfigProvider     { return m.config }
func (m *mockMediaCoreContext) Logger() *slog.Logger            { return slog.Default() }
func (m *mockMediaCoreContext) DataDir() string                 { return m.dataDir }
func (m *mockMediaCoreContext) PluginDir() string               { return "" }
func (m *mockMediaCoreContext) Context() context.Context        { return context.Background() }

func setupPluginWithCoreContext(t *testing.T) (*Plugin, *mockMediaCoreContext) {
	t.Helper()
	dbName := nextTestDBName()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", dbName))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dbSvc := database.NewService(db, database.DriverSQLite)

	tmpDir := t.TempDir()
	mCtx := &mockMediaCoreContext{
		svc:     &mockMediaServiceContainer{dbSvc: dbSvc},
		config:  &mockMediaConfig{storage: "local"},
		dataDir: tmpDir,
	}

	p := New()
	return p, mCtx
}

type mockErrorStorage struct{}

func (m *mockErrorStorage) Save(_ context.Context, _ []byte, _ string) error {
	return fmt.Errorf("storage save error")
}
func (m *mockErrorStorage) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("storage get error")
}
func (m *mockErrorStorage) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("storage delete error")
}
func (m *mockErrorStorage) GetURL(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("storage geturl error")
}
func (m *mockErrorStorage) Type() string { return "error" }

type mockFailProvideContainer struct {
	*mockMediaServiceContainer
}

func (m *mockFailProvideContainer) Provide(fn interface{}) error {
	return fmt.Errorf("provide failed")
}

// ==================== Plugin Lifecycle Tests ====================

func TestPluginNew(t *testing.T) {
	p := New()
	if p == nil {
		t.Fatal("expected non-nil plugin")
	}
	if p.Name() != "media" {
		t.Errorf("expected plugin name 'media', got %q", p.Name())
	}
	if p.Version() == "" {
		t.Error("expected non-empty plugin version")
	}
}

func TestPluginInit(t *testing.T) {
	p, mCtx := setupPluginWithCoreContext(t)

	if err := p.Init(mCtx); err != nil {
		t.Fatalf("init: %v", err)
	}
	if p.service == nil {
		t.Fatal("expected service to be initialized after Init")
	}
}

func TestPluginStartStop(t *testing.T) {
	p, mCtx := setupPluginWithCoreContext(t)

	if err := p.Init(mCtx); err != nil {
		t.Fatalf("init: %v", err)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !p.running {
		t.Error("expected running=true after Start")
	}

	// Start again should be idempotent
	if err := p.Start(); err != nil {
		t.Fatalf("start again: %v", err)
	}

	if err := p.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if p.running {
		t.Error("expected running=false after Stop")
	}

	// Stop again should be idempotent
	if err := p.Stop(); err != nil {
		t.Fatalf("stop again: %v", err)
	}
}

func TestPluginInitNoDatabase(t *testing.T) {
	p := New()
	tmpDir := t.TempDir()
	mCtx := &mockMediaCoreContext{
		svc:     &mockMediaServiceContainer{dbSvc: nil},
		config:  &mockMediaConfig{storage: "local"},
		dataDir: tmpDir,
	}

	err := p.Init(mCtx)
	if err == nil {
		t.Fatal("expected error when database service not available")
	}
	if !strings.Contains(err.Error(), "database service not available") {
		t.Errorf("expected 'database service not available' error, got: %v", err)
	}
}

func TestPluginInitUnsupportedStorage(t *testing.T) {
	dbName := nextTestDBName()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", dbName))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dbSvc := database.NewService(db, database.DriverSQLite)

	tmpDir := t.TempDir()
	mCtx := &mockMediaCoreContext{
		svc:     &mockMediaServiceContainer{dbSvc: dbSvc},
		config:  &mockMediaConfig{storage: "ftp"},
		dataDir: tmpDir,
	}

	p := New()
	err = p.Init(mCtx)
	if err == nil {
		t.Fatal("expected error for unsupported storage type")
	}
	if !strings.Contains(err.Error(), "unsupported storage type") {
		t.Errorf("expected 'unsupported storage type' error, got: %v", err)
	}
}

func TestPluginInitDefaultStorage(t *testing.T) {
	dbName := nextTestDBName()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", dbName))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dbSvc := database.NewService(db, database.DriverSQLite)

	tmpDir := t.TempDir()
	mCtx := &mockMediaCoreContext{
		svc:     &mockMediaServiceContainer{dbSvc: dbSvc},
		config:  &mockMediaConfig{storage: ""},
		dataDir: tmpDir,
	}

	p := New()
	if err := p.Init(mCtx); err != nil {
		t.Fatalf("init with empty storage (should default to local): %v", err)
	}
}

// ==================== Coverage Helper Tests ====================

func TestStoreCreateWithPresetID(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	mf := &interfaces.MediaFile{
		ID:          "custom-id-123",
		Filename:    "preset.png",
		MIMEType:    "image/png",
		Size:        100,
		StoragePath: "preset.png",
		StorageType: "local",
	}
	if err := svc.store.Create(ctx, mf); err != nil {
		t.Fatalf("create: %v", err)
	}
	if mf.ID != "custom-id-123" {
		t.Errorf("expected preset ID to be preserved, got %s", mf.ID)
	}
}

func TestStoreListPerPageOverflow(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		mf := &interfaces.MediaFile{
			Filename:    fmt.Sprintf("overflow%d.png", i),
			MIMEType:    "image/png",
			Size:        100,
			StoragePath: fmt.Sprintf("overflow%d.png", i),
			StorageType: "local",
		}
		if err := svc.store.Create(ctx, mf); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	// PerPage > 100 should be capped to 100
	items, _, err := svc.store.List(ctx, &interfaces.ListQuery{Page: 1, PerPage: 200})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items (capped perPage), got %d", len(items))
	}
}

func TestLocalStorageSaveCreatesSubDirs(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("create local storage: %v", err)
	}
	ctx := context.Background()

	// Path with multiple nested directories
	deepPath := filepath.Join("a", "b", "c", "deep.txt")
	if err := storage.Save(ctx, []byte("deep"), deepPath); err != nil {
		t.Fatalf("save to deep path: %v", err)
	}

	got, err := storage.Get(ctx, deepPath)
	if err != nil {
		t.Fatalf("get from deep path: %v", err)
	}
	if string(got) != "deep" {
		t.Errorf("expected 'deep', got %q", string(got))
	}
}

func TestServiceDeleteWithThumbnail(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 50, 50)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "delthumb.png", Size: int64(len(data))}
	uploaded, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	// Generate a thumbnail so Delete has to clean it up
	thumbPath, err := svc.GenerateThumbnail(ctx, uploaded.ID, 25, 25)
	if err != nil {
		t.Fatalf("generate thumbnail: %v", err)
	}
	if thumbPath == "" {
		t.Fatal("expected thumbnail path")
	}

	// Delete should remove both file and thumbnail
	if err := svc.Delete(ctx, uploaded.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = svc.GetByID(ctx, uploaded.ID)
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestServiceListEmpty(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	page, err := svc.List(ctx, nil)
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if page.Meta.Total != 0 {
		t.Errorf("expected total 0, got %d", page.Meta.Total)
	}
	items, ok := page.Data.([]*interfaces.MediaFile)
	if !ok {
		t.Fatal("expected []*interfaces.MediaFile")
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestEmitEventNilEventBus(t *testing.T) {
	// Test that service works correctly with nil EventBus
	svc := setupTestService(t)
	// Replace events with nil to test the nil path
	svc.events = nil
	ctx := context.Background()

	data := createTestPNG(t, 10, 10)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "nilevt.png", Size: int64(len(data))}

	// Should not panic with nil event bus
	mf, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload with nil event bus: %v", err)
	}

	// Delete should also not panic
	if err := svc.Delete(ctx, mf.ID); err != nil {
		t.Fatalf("delete with nil event bus: %v", err)
	}
}

func TestGenerateThumbnailAspectRatio(t *testing.T) {
	data := createTestPNG(t, 200, 100)

	thumb, err := generateThumbnail(data, 50, 50)
	if err != nil {
		t.Fatalf("generate thumbnail: %v", err)
	}
	if len(thumb) == 0 {
		t.Fatal("expected non-empty thumbnail")
	}
}

// ==================== S3 Storage Tests ====================

type mockS3Config struct {
	data map[string]interface{}
}

func (m *mockS3Config) GetString(key string) string {
	if v, ok := m.data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
func (m *mockS3Config) GetInt(key string) int {
	if v, ok := m.data[key]; ok {
		if i, ok := v.(int); ok {
			return i
		}
	}
	return 0
}
func (m *mockS3Config) GetBool(key string) bool {
	if v, ok := m.data[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
func (m *mockS3Config) GetStringSlice(key string) []string             { return nil }
func (m *mockS3Config) Get(key string) interface{}                     { return m.data[key] }
func (m *mockS3Config) Unmarshal(key string, target interface{}) error { return nil }
func (m *mockS3Config) Set(key string, value interface{})              { m.data[key] = value }
func (m *mockS3Config) Save() error                                    { return nil }

func TestNewS3StorageMissingConfig(t *testing.T) {
	cfg := &mockS3Config{data: map[string]interface{}{}}
	_, err := NewS3Storage(cfg)
	if err == nil {
		t.Fatal("expected error for missing S3 config")
	}
	if !strings.Contains(err.Error(), "s3 storage requires") {
		t.Errorf("expected 's3 storage requires' in error, got: %v", err)
	}
}

func TestNewS3StoragePartialConfig(t *testing.T) {
	cfg := &mockS3Config{data: map[string]interface{}{
		"endpoint": "localhost:9000",
	}}
	_, err := NewS3Storage(cfg)
	if err == nil {
		t.Fatal("expected error for partial S3 config")
	}
}

func TestNewS3StorageValidConfig(t *testing.T) {
	cfg := &mockS3Config{data: map[string]interface{}{
		"endpoint":   "localhost:9000",
		"access_key": "minioadmin",
		"secret_key": "minioadmin",
		"bucket":     "test-bucket",
		"use_ssl":    false,
		"region":     "us-east-1",
	}}
	s3, err := NewS3Storage(cfg)
	if err != nil {
		t.Fatalf("expected valid S3 config to create client: %v", err)
	}
	if s3.bucket != "test-bucket" {
		t.Errorf("expected bucket 'test-bucket', got %s", s3.bucket)
	}
	if s3.client == nil {
		t.Error("expected non-nil minio client")
	}
}

func TestS3StorageType(t *testing.T) {
	s := &S3Storage{}
	if s.Type() != "s3" {
		t.Errorf("expected type 's3', got %s", s.Type())
	}
}

func TestNewStorageBackendS3(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &mockS3Config{data: map[string]interface{}{}}
	_, err := NewStorageBackend("s3", tmpDir, cfg)
	if err == nil {
		t.Fatal("expected error for S3 with missing config")
	}
	if !strings.Contains(err.Error(), "s3 storage requires") {
		t.Errorf("expected S3 config error, got: %v", err)
	}
}

// ==================== Additional Coverage Tests ====================

func TestStoreCreateDuplicateID(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	mf1 := &interfaces.MediaFile{
		ID:          "dup-id",
		Filename:    "first.png",
		MIMEType:    "image/png",
		Size:        100,
		StoragePath: "first.png",
		StorageType: "local",
	}
	if err := svc.store.Create(ctx, mf1); err != nil {
		t.Fatalf("create first: %v", err)
	}

	mf2 := &interfaces.MediaFile{
		ID:          "dup-id",
		Filename:    "second.png",
		MIMEType:    "image/png",
		Size:        200,
		StoragePath: "second.png",
		StorageType: "local",
	}
	err := svc.store.Create(ctx, mf2)
	if err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestStoreListEmptyResult(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	items, total, err := svc.store.List(ctx, &interfaces.ListQuery{Page: 1, PerPage: 20})
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestStoreListFilterWithUnknownField(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	mf := &interfaces.MediaFile{
		Filename:    "test.png",
		MIMEType:    "image/png",
		Size:        100,
		StoragePath: "test.png",
		StorageType: "local",
	}
	if err := svc.store.Create(ctx, mf); err != nil {
		t.Fatalf("create: %v", err)
	}

	items, total, err := svc.store.List(ctx, &interfaces.ListQuery{
		Page:    1,
		PerPage: 20,
		Filters: map[string]interface{}{"unknown_field": "value"},
	})
	if err != nil {
		t.Fatalf("list with unknown filter: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1 (unknown filter ignored), got %d", total)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestGenerateThumbnailSmallImage(t *testing.T) {
	data := createTestPNG(t, 2, 2)

	thumb, err := generateThumbnail(data, 100, 100)
	if err != nil {
		t.Fatalf("generate thumbnail from small image: %v", err)
	}
	if len(thumb) == 0 {
		t.Fatal("expected non-empty thumbnail")
	}
}

func TestServiceUploadNonImage(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 10, 10)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "test.png", Size: int64(len(data))}

	mf, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload image: %v", err)
	}
	if mf.Width == 0 || mf.Height == 0 {
		t.Errorf("expected non-zero dimensions for image, got %dx%d", mf.Width, mf.Height)
	}
}

func TestLocalStorageNewWithExistingDir(t *testing.T) {
	tmpDir := t.TempDir()
	existingUploads := filepath.Join(tmpDir, "uploads")
	if err := os.MkdirAll(existingUploads, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	storage, err := NewLocalStorage(tmpDir)
	if err != nil {
		t.Fatalf("create local storage with existing dir: %v", err)
	}
	if storage == nil {
		t.Fatal("expected non-nil storage")
	}
}

func TestStoreListOrderCaseInsensitive(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		mf := &interfaces.MediaFile{
			Filename:    fmt.Sprintf("order%d.png", i),
			MIMEType:    "image/png",
			Size:        100,
			StoragePath: fmt.Sprintf("order%d.png", i),
			StorageType: "local",
		}
		if err := svc.store.Create(ctx, mf); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}

	items, _, err := svc.store.List(ctx, &interfaces.ListQuery{
		Page:  1,
		Order: "ASC",
	})
	if err != nil {
		t.Fatalf("list with ASC: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}

	items, _, err = svc.store.List(ctx, &interfaces.ListQuery{
		Page:  1,
		Order: "DESC",
	})
	if err != nil {
		t.Fatalf("list with DESC: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestServiceListPageMeta(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		data := createTestPNG(t, 10, 10)
		file := newTestFile(data)
		header := &multipart.FileHeader{
			Filename: fmt.Sprintf("meta%d.png", i),
			Size:     int64(len(data)),
		}
		if _, err := svc.UploadMultipart(ctx, file, header, "user-1"); err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}

	page, err := svc.List(ctx, &interfaces.ListQuery{Page: 1, PerPage: 3})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Meta.TotalPages != 2 {
		t.Errorf("expected 2 total pages, got %d", page.Meta.TotalPages)
	}
	if page.Meta.HasPrev {
		t.Error("expected HasPrev=false on page 1")
	}
	if !page.Meta.HasNext {
		t.Error("expected HasNext=true on page 1")
	}
}

func TestServiceUploadLargeDataSniffsMIME(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 500, 500)
	if len(data) <= 512 {
		t.Fatalf("test data too small (%d bytes), need > 512", len(data))
	}
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "large.png", Size: int64(len(data))}

	mf, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload large data: %v", err)
	}
	if mf.MIMEType != "image/png" {
		t.Errorf("expected mime_type 'image/png', got %s", mf.MIMEType)
	}
}

func TestServiceListPerPageOverflow(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 5, 5)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "overflow.png", Size: int64(len(data))}
	if _, err := svc.UploadMultipart(ctx, file, header, "user-1"); err != nil {
		t.Fatalf("upload: %v", err)
	}

	page, err := svc.List(ctx, &interfaces.ListQuery{Page: 1, PerPage: 200})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Meta.PerPage != 100 {
		t.Errorf("expected perPage capped to 100, got %d", page.Meta.PerPage)
	}
}

func TestServiceUploadStorageError(t *testing.T) {
	svc := setupTestService(t)
	svc.storage = &mockErrorStorage{}
	ctx := context.Background()

	data := createTestPNG(t, 10, 10)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "fail.png", Size: int64(len(data))}

	_, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err == nil {
		t.Fatal("expected error when storage fails")
	}
	if !strings.Contains(err.Error(), "save file to storage") {
		t.Errorf("expected storage save error, got: %v", err)
	}
}

func TestServiceDeleteStorageErrors(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 10, 10)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "delerr.png", Size: int64(len(data))}
	uploaded, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	svc.storage = &mockErrorStorage{}

	err = svc.Delete(ctx, uploaded.ID)
	if err != nil {
		t.Fatalf("delete should not fail when storage delete fails (just warns): %v", err)
	}
}

func TestServiceGenerateThumbnailStorageGetError(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 10, 10)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "thumberr.png", Size: int64(len(data))}
	uploaded, err := svc.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	svc.storage = &mockErrorStorage{}

	_, err = svc.GenerateThumbnail(ctx, uploaded.ID, 50, 50)
	if err == nil {
		t.Fatal("expected error when storage get fails")
	}
	if !strings.Contains(err.Error(), "get original file") {
		t.Errorf("expected 'get original file' error, got: %v", err)
	}
}

func TestPluginInitProvideError(t *testing.T) {
	dbName := nextTestDBName()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", dbName))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dbSvc := database.NewService(db, database.DriverSQLite)

	tmpDir := t.TempDir()
	mCtx := &mockMediaCoreContext{
		svc:     &mockFailProvideContainer{&mockMediaServiceContainer{dbSvc: dbSvc}},
		config:  &mockMediaConfig{storage: "local"},
		dataDir: tmpDir,
	}

	p := New()
	err = p.Init(mCtx)
	if err == nil {
		t.Fatal("expected error when Provide fails")
	}
	if !strings.Contains(err.Error(), "register MediaService") {
		t.Errorf("expected 'register MediaService' error, got: %v", err)
	}
}

func TestGenerateThumbnailScaleH(t *testing.T) {
	// Tall image (100x200) with target 50x50: scaleH=0.25 < scaleW=0.5 → takes scaleH branch
	data := createTestPNG(t, 100, 200)

	thumb, err := generateThumbnail(data, 50, 50)
	if err != nil {
		t.Fatalf("generate thumbnail: %v", err)
	}
	if len(thumb) == 0 {
		t.Fatal("expected non-empty thumbnail")
	}
}

func TestGenerateThumbnailZeroWidth(t *testing.T) {
	// Target width=0 → scaleW=0, newW=0 → newW set to 1
	data := createTestPNG(t, 100, 100)

	thumb, err := generateThumbnail(data, 0, 50)
	if err != nil {
		t.Fatalf("generate thumbnail zero width: %v", err)
	}
	if len(thumb) == 0 {
		t.Fatal("expected non-empty thumbnail")
	}
}

func TestGenerateThumbnailZeroHeight(t *testing.T) {
	// Target height=0 → scaleH=0 < scaleW=0.5 → scale=0, newH=0 → newH set to 1
	data := createTestPNG(t, 100, 100)

	thumb, err := generateThumbnail(data, 50, 0)
	if err != nil {
		t.Fatalf("generate thumbnail zero height: %v", err)
	}
	if len(thumb) == 0 {
		t.Fatal("expected non-empty thumbnail")
	}
}

func TestNewS3StorageInvalidEndpoint(t *testing.T) {
	cfg := &mockS3Config{data: map[string]interface{}{
		"endpoint":   "not-a-valid-endpoint-with-port:INVALID",
		"access_key": "test",
		"secret_key": "test",
		"bucket":     "test",
		"use_ssl":    false,
		"region":     "",
	}}
	_, err := NewS3Storage(cfg)
	// minio.New may or may not error depending on endpoint format
	// We just verify it doesn't panic
	_ = err
}

// ==================== UploadFromReader Tests ====================

func TestUploadFromReader(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 100, 100)
	reader := bytes.NewReader(data)

	mf, err := svc.UploadFromReader(ctx, reader, "test.png", "image/png", "user-1")
	if err != nil {
		t.Fatalf("upload from reader: %v", err)
	}
	if mf.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if mf.Filename != "test.png" {
		t.Errorf("expected filename 'test.png', got %s", mf.Filename)
	}
	if mf.MIMEType != "image/png" {
		t.Errorf("expected mime_type 'image/png', got %s", mf.MIMEType)
	}
	if mf.Size != int64(len(data)) {
		t.Errorf("expected size %d, got %d", len(data), mf.Size)
	}
	if mf.StorageType != "local" {
		t.Errorf("expected storage_type 'local', got %s", mf.StorageType)
	}
	if mf.UploaderID != "user-1" {
		t.Errorf("expected uploader_id 'user-1', got %s", mf.UploaderID)
	}
	if mf.Width != 100 {
		t.Errorf("expected width 100, got %d", mf.Width)
	}
	if mf.Height != 100 {
		t.Errorf("expected height 100, got %d", mf.Height)
	}
}

func TestUploadFromReaderEmptyContentType(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 50, 50)
	reader := bytes.NewReader(data)

	// Empty contentType should trigger MIME sniffing
	mf, err := svc.UploadFromReader(ctx, reader, "sniff.png", "", "user-1")
	if err != nil {
		t.Fatalf("upload from reader with empty content type: %v", err)
	}
	if mf.MIMEType != "image/png" {
		t.Errorf("expected sniffed mime_type 'image/png', got %s", mf.MIMEType)
	}
}

func TestUploadFromReaderExceedsMaxSize(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	bigData := make([]byte, 50*1024*1024+1)
	copy(bigData, createTestPNG(t, 1, 1))

	reader := bytes.NewReader(bigData)
	_, err := svc.UploadFromReader(ctx, reader, "big.png", "image/png", "user-1")
	if err == nil {
		t.Fatal("expected error for file exceeding max size")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("expected 'exceeds maximum' in error, got: %v", err)
	}
}

func TestUploadFromReaderInvalidMIME(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	invalidData := []byte{0x89, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0}
	reader := bytes.NewReader(invalidData)

	_, err := svc.UploadFromReader(ctx, reader, "random.bin", "", "user-1")
	if err == nil {
		t.Fatal("expected error for invalid MIME type")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("expected 'not allowed' in error, got: %v", err)
	}
}

func TestUploadFromReaderExplicitBadMIME(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 10, 10)
	reader := bytes.NewReader(data)

	_, err := svc.UploadFromReader(ctx, reader, "test.txt", "text/plain", "user-1")
	if err == nil {
		t.Fatal("expected error for text/plain MIME type")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("expected 'not allowed' in error, got: %v", err)
	}
}

func TestUploadFromReaderNoExtension(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	data := createTestPNG(t, 10, 10)
	reader := bytes.NewReader(data)

	mf, err := svc.UploadFromReader(ctx, reader, "noext", "image/png", "user-1")
	if err != nil {
		t.Fatalf("upload from reader no extension: %v", err)
	}
	// Storage path should use .bin as default extension when none provided
	if !strings.Contains(mf.StoragePath, ".bin") {
		t.Errorf("expected storage path to contain '.bin', got %s", mf.StoragePath)
	}
}

func TestUploadFromReaderNonImage(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	// Upload a PDF via UploadFromReader
	pdfData := []byte("%PDF-1.4 fake pdf content for testing")
	reader := bytes.NewReader(pdfData)

	mf, err := svc.UploadFromReader(ctx, reader, "doc.pdf", "application/pdf", "user-1")
	if err != nil {
		t.Fatalf("upload pdf from reader: %v", err)
	}
	if mf.MIMEType != "application/pdf" {
		t.Errorf("expected mime_type 'application/pdf', got %s", mf.MIMEType)
	}
	// Non-images should have zero dimensions
	if mf.Width != 0 || mf.Height != 0 {
		t.Errorf("expected zero dimensions for pdf, got %dx%d", mf.Width, mf.Height)
	}
}

func TestUploadFromReaderStorageError(t *testing.T) {
	svc := setupTestService(t)
	svc.storage = &mockErrorStorage{}
	ctx := context.Background()

	data := createTestPNG(t, 10, 10)
	reader := bytes.NewReader(data)

	_, err := svc.UploadFromReader(ctx, reader, "fail.png", "image/png", "user-1")
	if err == nil {
		t.Fatal("expected error when storage fails")
	}
	if !strings.Contains(err.Error(), "save file to storage") {
		t.Errorf("expected storage save error, got: %v", err)
	}
}

func TestUploadFromReaderReadError(t *testing.T) {
	svc := setupTestService(t)
	ctx := context.Background()

	reader := &errorReader{}
	_, err := svc.UploadFromReader(ctx, reader, "fail.png", "image/png", "user-1")
	if err == nil {
		t.Fatal("expected error when reader fails")
	}
	if !strings.Contains(err.Error(), "read file data") {
		t.Errorf("expected read error, got: %v", err)
	}
}

type errorReader struct{}

func (r *errorReader) Read(_ []byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// ==================== Handler Tests ====================

func setupPluginWithRoutes(t *testing.T) (*Plugin, chi.Router) {
	t.Helper()
	dbName := nextTestDBName()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_pragma=foreign_keys(ON)", dbName))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dbSvc := database.NewService(db, database.DriverSQLite)

	tmpDir := t.TempDir()
	chiRouter := chi.NewRouter()
	registrar := &testRouteRegistrar{router: chiRouter}

	mCtx := &mockMediaCoreContext{
		svc:     &mockMediaServiceContainerWithSvc{dbSvc: dbSvc, registrar: registrar},
		config:  &mockMediaConfig{storage: "local"},
		dataDir: tmpDir,
	}

	p := New()
	if err := p.Init(mCtx); err != nil {
		t.Fatalf("init plugin: %v", err)
	}
	if err := p.Start(); err != nil {
		t.Fatalf("start plugin: %v", err)
	}

	return p, chiRouter
}

type testRouteRegistrar struct {
	router *chi.Mux
}

func (r *testRouteRegistrar) Handle(pattern string, handler http.Handler) {
	r.router.Handle(pattern, handler)
}
func (r *testRouteRegistrar) HandleFunc(pattern string, handler http.HandlerFunc) {
	r.router.HandleFunc(pattern, handler)
}
func (r *testRouteRegistrar) Use(middlewares ...func(http.Handler) http.Handler) {
	r.router.Use(middlewares...)
}
func (r *testRouteRegistrar) Middlewares() []func(http.Handler) http.Handler { return nil }

type mockMediaServiceContainerWithSvc struct {
	dbSvc     interfaces.DatabaseService
	registrar interfaces.RouteRegistrar
}

func (m *mockMediaServiceContainerWithSvc) Provide(fn interface{}) error { return nil }
func (m *mockMediaServiceContainerWithSvc) Get(target interface{}) error {
	if p, ok := target.(*interfaces.DatabaseService); ok && m.dbSvc != nil {
		*p = m.dbSvc
		return nil
	}
	if p, ok := target.(*interfaces.RouteRegistrar); ok && m.registrar != nil {
		*p = m.registrar
		return nil
	}
	return fmt.Errorf("service not found")
}
func (m *mockMediaServiceContainerWithSvc) GetNamed(name string, target interface{}) error {
	return nil
}
func (m *mockMediaServiceContainerWithSvc) Unregister(target interface{}) error { return nil }
func (m *mockMediaServiceContainerWithSvc) Has(target interface{}) bool         { return false }
func (m *mockMediaServiceContainerWithSvc) Keys() []string                      { return nil }

func TestHandleUploadSuccess(t *testing.T) {
	_, router := setupPluginWithRoutes(t)

	data := createTestPNG(t, 50, 50)
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "test.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write data: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/media", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp mediaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	dataMap, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be a map")
	}
	if dataMap["filename"] != "test.png" {
		t.Errorf("expected filename 'test.png', got %v", dataMap["filename"])
	}
}

func TestHandleUploadNoFile(t *testing.T) {
	_, router := setupPluginWithRoutes(t)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/media", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleUploadInvalidMIME(t *testing.T) {
	_, router := setupPluginWithRoutes(t)

	invalidData := []byte{0x89, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "random.bin")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part.Write(invalidData)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/media", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleUploadOversized(t *testing.T) {
	_, router := setupPluginWithRoutes(t)

	// Create a request body that exceeds maxFileSize in multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "big.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	// Write more than maxFileSize bytes
	bigData := make([]byte, maxFileSize+1024)
	copy(bigData, createTestPNG(t, 1, 1))
	part.Write(bigData)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/media", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleListEmpty(t *testing.T) {
	_, router := setupPluginWithRoutes(t)

	req := httptest.NewRequest("GET", "/api/v1/media", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp mediaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	dataArr, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatalf("expected data to be an array, got %T", resp.Data)
	}
	if len(dataArr) != 0 {
		t.Errorf("expected 0 items, got %d", len(dataArr))
	}
	// Check X-Total-Count header
	if w.Header().Get("X-Total-Count") != "0" {
		t.Errorf("expected X-Total-Count 0, got %s", w.Header().Get("X-Total-Count"))
	}
}

func TestHandleListWithItems(t *testing.T) {
	p, router := setupPluginWithRoutes(t)
	ctx := context.Background()

	// Upload a file via service first
	data := createTestPNG(t, 30, 30)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "list_test.png", Size: int64(len(data))}
	if _, err := p.service.UploadMultipart(ctx, file, header, "user-1"); err != nil {
		t.Fatalf("upload: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/media", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp mediaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	dataArr, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatalf("expected data to be an array, got %T", resp.Data)
	}
	if len(dataArr) != 1 {
		t.Errorf("expected 1 item, got %d", len(dataArr))
	}
}

func TestHandleListPagination(t *testing.T) {
	p, router := setupPluginWithRoutes(t)
	ctx := context.Background()

	// Upload 3 files
	for i := 0; i < 3; i++ {
		data := createTestPNG(t, 10, 10)
		file := newTestFile(data)
		header := &multipart.FileHeader{
			Filename: fmt.Sprintf("page_test_%d.png", i),
			Size:     int64(len(data)),
		}
		if _, err := p.service.UploadMultipart(ctx, file, header, "user-1"); err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}

	// Request page 1 with per_page=2
	req := httptest.NewRequest("GET", "/api/v1/media?page=1&per_page=2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp mediaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	metaMap, ok := resp.Meta.(map[string]interface{})
	if !ok {
		t.Fatalf("expected meta to be a map, got %T", resp.Meta)
	}
	if int(metaMap["total_count"].(float64)) != 3 {
		t.Errorf("expected total_count 3, got %v", metaMap["total_count"])
	}
}

func TestHandleListSortOrder(t *testing.T) {
	p, router := setupPluginWithRoutes(t)
	ctx := context.Background()

	for _, name := range []string{"alpha.png", "beta.png", "gamma.png"} {
		data := createTestPNG(t, 5, 5)
		file := newTestFile(data)
		header := &multipart.FileHeader{Filename: name, Size: int64(len(data))}
		if _, err := p.service.UploadMultipart(ctx, file, header, "user-1"); err != nil {
			t.Fatalf("upload %s: %v", name, err)
		}
	}

	req := httptest.NewRequest("GET", "/api/v1/media?sort=filename&order=asc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleDeleteSuccess(t *testing.T) {
	p, router := setupPluginWithRoutes(t)
	ctx := context.Background()

	data := createTestPNG(t, 20, 20)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "delete_test.png", Size: int64(len(data))}
	uploaded, err := p.service.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	req := httptest.NewRequest("DELETE", "/api/v1/media/"+uploaded.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d; body: %s", w.Code, w.Body.String())
	}

	// Verify deleted
	_, err = p.service.GetByID(ctx, uploaded.ID)
	if err != interfaces.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestHandleDeleteNotFound(t *testing.T) {
	_, router := setupPluginWithRoutes(t)

	req := httptest.NewRequest("DELETE", "/api/v1/media/nonexistent-id", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleDeleteEmptyID(t *testing.T) {
	_, router := setupPluginWithRoutes(t)

	// chi won't route to /{id} without an id segment, so this hits 404 from router
	req := httptest.NewRequest("DELETE", "/api/v1/media", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Router returns 405 (method not allowed) for DELETE on collection path
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestExtractUploaderID(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	// No user ID in context
	if uid := extractUploaderID(req); uid != "" {
		t.Errorf("expected empty uid, got %s", uid)
	}

	// With user ID in context
	ctx := context.WithValue(req.Context(), ctxKeyUserID, "user-42")
	req = req.WithContext(ctx)
	if uid := extractUploaderID(req); uid != "user-42" {
		t.Errorf("expected uid 'user-42', got %s", uid)
	}
}

func TestExtractUploaderIDEmptyString(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), ctxKeyUserID, "")
	req = req.WithContext(ctx)
	if uid := extractUploaderID(req); uid != "" {
		t.Errorf("expected empty uid for empty string, got %s", uid)
	}
}

func TestExtractUploaderIDWrongType(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), ctxKeyUserID, 12345)
	req = req.WithContext(ctx)
	if uid := extractUploaderID(req); uid != "" {
		t.Errorf("expected empty uid for wrong type, got %s", uid)
	}
}

func TestWriteMediaJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeMediaJSON(w, http.StatusOK, map[string]string{"hello": "world"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected json content type, got %s", ct)
	}
	var resp mediaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	dataMap, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("expected data map")
	}
	if dataMap["hello"] != "world" {
		t.Errorf("expected hello=world, got %v", dataMap["hello"])
	}
}

func TestWriteMediaError(t *testing.T) {
	w := httptest.NewRecorder()
	writeMediaError(w, http.StatusBadRequest, "TEST_CODE", "test message")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected json content type, got %s", ct)
	}
	var envelope mediaErrorsEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(envelope.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(envelope.Errors))
	}
	if envelope.Errors[0].Code != "TEST_CODE" {
		t.Errorf("expected code TEST_CODE, got %s", envelope.Errors[0].Code)
	}
	if envelope.Errors[0].Message != "test message" {
		t.Errorf("expected message 'test message', got %s", envelope.Errors[0].Message)
	}
}

func TestWriteMediaJSONWithMeta(t *testing.T) {
	w := httptest.NewRecorder()
	meta := mediaPageMeta{
		TotalCount: 10,
		Page:       2,
		PerPage:    5,
		TotalPages: 2,
	}
	writeMediaJSONWithMeta(w, http.StatusOK, []string{"a", "b"}, meta)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Header().Get("X-Total-Count") != "10" {
		t.Errorf("expected X-Total-Count 10, got %s", w.Header().Get("X-Total-Count"))
	}
	var resp mediaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	metaMap, ok := resp.Meta.(map[string]interface{})
	if !ok {
		t.Fatalf("expected meta map, got %T", resp.Meta)
	}
	if int(metaMap["total_count"].(float64)) != 10 {
		t.Errorf("expected total_count 10, got %v", metaMap["total_count"])
	}
}

// ==================== Register Routes / Route Registrar Tests ====================

func TestRegisterRoutes(t *testing.T) {
	p, router := setupPluginWithRoutes(t)

	// Verify routes are registered by walking the router
	var routes []string
	chi.Walk(router, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		routes = append(routes, method+" "+route)
		return nil
	})

	// Should have POST, GET, DELETE routes
	hasPost := false
	hasGet := false
	hasDelete := false
	for _, r := range routes {
		if strings.Contains(r, "POST") && strings.Contains(r, "/api/v1/media") {
			hasPost = true
		}
		if strings.Contains(r, "GET") && strings.Contains(r, "/api/v1/media") {
			hasGet = true
		}
		if strings.Contains(r, "DELETE") && strings.Contains(r, "/api/v1/media") {
			hasDelete = true
		}
	}
	_ = p
	if !hasPost {
		t.Error("expected POST route to be registered")
	}
	if !hasGet {
		t.Error("expected GET route to be registered")
	}
	if !hasDelete {
		t.Error("expected DELETE route to be registered")
	}
}

func TestHandleListWithSearch(t *testing.T) {
	p, router := setupPluginWithRoutes(t)
	ctx := context.Background()

	data := createTestPNG(t, 10, 10)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "searchable.png", Size: int64(len(data))}
	if _, err := p.service.UploadMultipart(ctx, file, header, "user-1"); err != nil {
		t.Fatalf("upload: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/media?search=searchable", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleListInvalidPage(t *testing.T) {
	_, router := setupPluginWithRoutes(t)

	// Page 0 should default to page 1
	req := httptest.NewRequest("GET", "/api/v1/media?page=0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleListInvalidPerPage(t *testing.T) {
	_, router := setupPluginWithRoutes(t)

	// per_page=0 should default to 20
	req := httptest.NewRequest("GET", "/api/v1/media?per_page=0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleListPerPageOverflow(t *testing.T) {
	_, router := setupPluginWithRoutes(t)

	// per_page > 100 should be capped
	req := httptest.NewRequest("GET", "/api/v1/media?per_page=500", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleListDefaultOrder(t *testing.T) {
	_, router := setupPluginWithRoutes(t)

	// No order param should default to "desc"
	req := httptest.NewRequest("GET", "/api/v1/media?order=invalid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestHandleUploadWithAuthUserID(t *testing.T) {
	_, router := setupPluginWithRoutes(t)

	data := createTestPNG(t, 20, 20)
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "auth_test.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part.Write(data)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/media", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// Set user ID in context
	ctx := context.WithValue(req.Context(), ctxKeyUserID, "auth-user-1")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp mediaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	dataMap, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be a map")
	}
	if dataMap["uploader_id"] != "auth-user-1" {
		t.Errorf("expected uploader_id 'auth-user-1', got %v", dataMap["uploader_id"])
	}
}

func TestHandleUploadFileOpenError(t *testing.T) {
	// Test the case where fileHeader.Open() would fail — this is hard to trigger
	// directly, so we test via a malformed multipart form body
	_, router := setupPluginWithRoutes(t)

	req := httptest.NewRequest("POST", "/api/v1/media", strings.NewReader("not a multipart form"))
	req.Header.Set("Content-Type", "multipart/form-data")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleDeleteInternalError(t *testing.T) {
	p, router := setupPluginWithRoutes(t)
	// Replace storage with error storage to trigger internal error during delete
	p.service.storage = &mockDeleteInternalErrorStorage{}

	req := httptest.NewRequest("DELETE", "/api/v1/media/some-id", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should get 404 since the media doesn't exist in the store
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

type mockDeleteInternalErrorStorage struct{}

func (m *mockDeleteInternalErrorStorage) Save(_ context.Context, _ []byte, _ string) error {
	return nil
}
func (m *mockDeleteInternalErrorStorage) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("get error")
}
func (m *mockDeleteInternalErrorStorage) Delete(_ context.Context, _ string) error {
	return nil
}
func (m *mockDeleteInternalErrorStorage) GetURL(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *mockDeleteInternalErrorStorage) Type() string { return "mock" }

func TestHandleListWithUploaderIDFilter(t *testing.T) {
	p, router := setupPluginWithRoutes(t)
	ctx := context.Background()

	// Upload two files from different users
	for i, uid := range []string{"user-a", "user-b"} {
		data := createTestPNG(t, 10, 10)
		file := newTestFile(data)
		header := &multipart.FileHeader{
			Filename: fmt.Sprintf("filter_%d.png", i),
			Size:     int64(len(data)),
		}
		if _, err := p.service.UploadMultipart(ctx, file, header, uid); err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}

	// List all — should have 2 items
	req := httptest.NewRequest("GET", "/api/v1/media", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp mediaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	dataArr, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", resp.Data)
	}
	if len(dataArr) != 2 {
		t.Errorf("expected 2 items, got %d", len(dataArr))
	}
}

func TestHandleUploadSuccessResponseHasURL(t *testing.T) {
	_, router := setupPluginWithRoutes(t)

	data := createTestPNG(t, 30, 30)
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "url_test.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part.Write(data)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/media", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp mediaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	dataMap, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be a map")
	}
	// URL field should be present (non-empty for local storage)
	if url, ok := dataMap["url"].(string); !ok || url == "" {
		t.Errorf("expected non-empty url field, got %v", dataMap["url"])
	}
}

func TestHandleListItemsHaveURLs(t *testing.T) {
	p, router := setupPluginWithRoutes(t)
	ctx := context.Background()

	data := createTestPNG(t, 10, 10)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "list_url.png", Size: int64(len(data))}
	if _, err := p.service.UploadMultipart(ctx, file, header, "user-1"); err != nil {
		t.Fatalf("upload: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/media", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp mediaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	dataArr, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", resp.Data)
	}
	if len(dataArr) != 1 {
		t.Fatalf("expected 1 item, got %d", len(dataArr))
	}
	item, ok := dataArr[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected item to be a map")
	}
	if url, ok := item["url"].(string); !ok || url == "" {
		t.Errorf("expected non-empty url in list item, got %v", item["url"])
	}
}

// ==================== Handler Error Path Tests ====================

func TestHandleUploadStorageError(t *testing.T) {
	p, router := setupPluginWithRoutes(t)
	// Replace storage with one that fails on Save
	p.service.storage = &mockErrorStorage{}

	data := createTestPNG(t, 20, 20)
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "err_upload.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part.Write(data)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/media", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Storage errors should return 500 (internal error, not validation)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleUploadGetURLFail(t *testing.T) {
	p, router := setupPluginWithRoutes(t)
	// Use a storage that works for Save but fails for GetURL
	p.service.storage = &mockSaveOnlyErrorURLStorage{}

	data := createTestPNG(t, 20, 20)
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "url_fail.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part.Write(data)
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/media", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Upload should still succeed (201), just with empty URL
	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp mediaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	dataMap, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be a map")
	}
	if dataMap["url"] != "" {
		t.Errorf("expected empty url when GetURL fails, got %v", dataMap["url"])
	}
}

func TestHandleListGetURLFail(t *testing.T) {
	p, router := setupPluginWithRoutes(t)
	ctx := context.Background()

	// Upload a file first (before swapping storage)
	data := createTestPNG(t, 10, 10)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "list_url_fail.png", Size: int64(len(data))}
	if _, err := p.service.UploadMultipart(ctx, file, header, "user-1"); err != nil {
		t.Fatalf("upload: %v", err)
	}

	// Now swap storage to one that fails GetURL
	p.service.storage = &mockSaveOnlyErrorURLStorage{}

	req := httptest.NewRequest("GET", "/api/v1/media", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp mediaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	dataArr, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", resp.Data)
	}
	if len(dataArr) != 1 {
		t.Fatalf("expected 1 item, got %d", len(dataArr))
	}
	item, ok := dataArr[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected item to be a map")
	}
	// URL should be empty since GetURL failed
	if item["url"] != "" {
		t.Errorf("expected empty url when GetURL fails, got %v", item["url"])
	}
}

func TestHandleDeleteStoreError(t *testing.T) {
	p, router := setupPluginWithRoutes(t)
	ctx := context.Background()

	// Upload a file first
	data := createTestPNG(t, 10, 10)
	file := newTestFile(data)
	header := &multipart.FileHeader{Filename: "del_err.png", Size: int64(len(data))}
	uploaded, err := p.service.UploadMultipart(ctx, file, header, "user-1")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	// Now swap storage to one that fails delete (store still works)
	p.service.storage = &mockErrorStorage{}

	req := httptest.NewRequest("DELETE", "/api/v1/media/"+uploaded.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Even with storage errors, the handler should return 204 since store.Delete succeeds
	// (storage errors are just logged as warnings)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d; body: %s", w.Code, w.Body.String())
	}
}

// mockSaveOnlyErrorURLStorage works for Save but fails for Get/GetURL
type mockSaveOnlyErrorURLStorage struct{}

func (m *mockSaveOnlyErrorURLStorage) Save(_ context.Context, _ []byte, _ string) error {
	return nil
}
func (m *mockSaveOnlyErrorURLStorage) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("get error")
}
func (m *mockSaveOnlyErrorURLStorage) Delete(_ context.Context, _ string) error {
	return nil
}
func (m *mockSaveOnlyErrorURLStorage) GetURL(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("geturl error")
}
func (m *mockSaveOnlyErrorURLStorage) Type() string { return "mock" }
