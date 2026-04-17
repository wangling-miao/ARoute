package interfaces

import (
	"context"
	"io"
	"mime/multipart"
)

// MediaService defines file upload and media management operations.
// It handles file uploads, storage (local or S3), thumbnail generation,
// and media library organization.
type MediaService interface {
	// Upload handles file upload from a multipart form, validates MIME type
	// and file size, stores to configured backend (local or S3), and creates
	// metadata record.
	Upload(ctx context.Context, file multipart.File, header *multipart.FileHeader, uploaderID string) (*MediaFile, error)

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

	// UploadFromReader stores a file from an io.Reader, avoiding HTTP multipart coupling.
	UploadFromReader(ctx context.Context, reader io.Reader, filename string, contentType string, uploaderID string) (*MediaFile, error)
}
