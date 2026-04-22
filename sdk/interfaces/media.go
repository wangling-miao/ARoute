package interfaces

import (
	"context"
	"io"
	"time"
)

// MediaService defines file upload and media management operations.
// It handles file uploads, storage (local or S3), thumbnail generation,
// and media library organization.
type MediaService interface {
	// Upload stores a file from an io.Reader with associated metadata.
	// Validates MIME type and file size, stores to configured backend (local or S3),
	// and creates a metadata record.
	Upload(ctx context.Context, reader io.Reader, filename string, contentType string, size int64, uploaderID string) (*MediaFile, error)

	// GetByID retrieves a media file metadata by ID.
	GetByID(ctx context.Context, id string) (*MediaFile, error)

	// Delete removes a media file from storage and metadata database.
	Delete(ctx context.Context, id string) error

	// List retrieves a paginated list of media files.
	List(ctx context.Context, query *ListQuery) (*Page, error)

	// GetURL generates a URL for accessing the media file.
	// For local storage: returns relative path like /uploads/YYYY/MM/DD/filename
	// For S3: returns pre-signed URL or public URL.
	GetURL(ctx context.Context, id string) (string, error)

	// GenerateThumbnail creates a thumbnail for an image file.
	// Returns the thumbnail path or an error if not applicable.
	GenerateThumbnail(ctx context.Context, id string, width, height int) (string, error)
}

// UploadRequest contains parameters for uploading a file via HTTP multipart form.
// This is used internally by the media plugin and is not part of the MediaService interface.
type UploadRequest struct {
	// Filename is the original file name.
	Filename string `json:"filename"`

	// ContentType is the MIME type (e.g., "image/jpeg").
	ContentType string `json:"content_type"`

	// Size is the file size in bytes.
	Size int64 `json:"size"`

	// UploaderID is the ID of the user uploading the file.
	UploaderID string `json:"uploader_id"`

	// CreatedAt is when the upload was initiated.
	CreatedAt time.Time `json:"created_at"`
}
