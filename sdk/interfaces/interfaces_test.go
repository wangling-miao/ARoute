package interfaces

import (
	"context"
	"database/sql"
	"io"
	"mime/multipart"
	"testing"
	"time"
)

// TestInterfaceCompliance verifies that all interface methods are correctly defined
// and compile without errors. This ensures interface contracts are maintained.

func TestDatabaseService_Interface(t *testing.T) {
	// This test verifies the interface compiles and has all required methods
	var _ DatabaseService = (*mockDatabaseService)(nil)
}

func TestAuthService_Interface(t *testing.T) {
	var _ AuthService = (*mockAuthService)(nil)
}

func TestContentService_Interface(t *testing.T) {
	var _ ContentService = (*mockContentService)(nil)
}

func TestMediaService_Interface(t *testing.T) {
	var _ MediaService = (*mockMediaService)(nil)
}

func TestSearchService_Interface(t *testing.T) {
	var _ SearchService = (*mockSearchService)(nil)
}

func TestCacheService_Interface(t *testing.T) {
	var _ CacheService = (*mockCacheService)(nil)
}

func TestQueueService_Interface(t *testing.T) {
	var _ QueueService = (*mockQueueService)(nil)
}

func TestThemeService_Interface(t *testing.T) {
	var _ ThemeService = (*mockThemeService)(nil)
}

func TestErrorTypes(t *testing.T) {
	// Test that all error types are correctly defined
	err := ErrNotFound
	if err == nil {
		t.Error("ErrNotFound should not be nil")
	}

	err = ErrUnauthorized
	if err == nil {
		t.Error("ErrUnauthorized should not be nil")
	}

	err = ErrForbidden
	if err == nil {
		t.Error("ErrForbidden should not be nil")
	}

	err = ErrValidation
	if err == nil {
		t.Error("ErrValidation should not be nil")
	}

	err = ErrConflict
	if err == nil {
		t.Error("ErrConflict should not be nil")
	}
}

func TestValidationError(t *testing.T) {
	ve := &ValidationError{
		Field:   "title",
		Message: "is required",
		Code:    "required",
	}

	if ve.Error() != "title: is required" {
		t.Errorf("Expected 'title: is required', got '%s'", ve.Error())
	}

	if ve.Field != "title" {
		t.Errorf("Expected field 'title', got '%s'", ve.Field)
	}

	if ve.Message != "is required" {
		t.Errorf("Expected message 'is required', got '%s'", ve.Message)
	}

	if ve.Code != "required" {
		t.Errorf("Expected code 'required', got '%s'", ve.Code)
	}
}

func TestValidationErrors(t *testing.T) {
	ves := NewValidationErrors()

	if ves.HasErrors() {
		t.Error("New ValidationErrors should not have errors")
	}

	ves.Add("title", "is required", "required")
	ves.Add("email", "must be valid", "email")

	if !ves.HasErrors() {
		t.Error("ValidationErrors should have errors after Add")
	}

	if len(ves.Errors) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(ves.Errors))
	}

	if ves.Error() != "validation failed: 2 errors" {
		t.Errorf("Expected 'validation failed: 2 errors', got '%s'", ves.Error())
	}
}

func TestDataTypes(t *testing.T) {
	// Test User type
	user := &User{
		ID:        "user-123",
		Email:     "test@example.com",
		Username:  "testuser",
		Roles:     []string{"admin"},
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if user.ID != "user-123" {
		t.Errorf("Expected user ID 'user-123', got '%s'", user.ID)
	}

	// Test Content type
	content := &Content{
		ID:          "content-456",
		ContentType: "post",
		Title:       "Test Post",
		Slug:        "test-post",
		Data: map[string]interface{}{
			"body": "This is a test post",
		},
		AuthorID: "user-123",
		Status:   "published",
		Version:  1,
	}

	if content.ContentType != "post" {
		t.Errorf("Expected content type 'post', got '%s'", content.ContentType)
	}

	// Test ContentType type
	ct := &ContentType{
		ID:          "ct-789",
		Name:        "post",
		DisplayName: "Blog Post",
		Fields: []Field{
			{
				Name:     "title",
				Type:     "text",
				Required: true,
			},
		},
	}

	if ct.Name != "post" {
		t.Errorf("Expected content type name 'post', got '%s'", ct.Name)
	}

	// Test Field type
	field := &Field{
		Name:        "body",
		DisplayName: "Body",
		Type:        "richtext",
		Required:    true,
	}

	if field.Type != "richtext" {
		t.Errorf("Expected field type 'richtext', got '%s'", field.Type)
	}

	// Test MediaFile type
	media := &MediaFile{
		ID:          "media-101",
		Filename:    "image.jpg",
		MIMEType:    "image/jpeg",
		Size:        1024,
		StoragePath: "/uploads/2024/01/image.jpg",
		UploaderID:  "user-123",
	}

	if media.Filename != "image.jpg" {
		t.Errorf("Expected filename 'image.jpg', got '%s'", media.Filename)
	}

	// Test SearchResult type
	result := &SearchResult{
		ID:          "content-456",
		ContentType: "post",
		Title:       "Test Post",
		Excerpt:     "...highlighted text...",
		Score:       0.95,
	}

	if result.Score != 0.95 {
		t.Errorf("Expected score 0.95, got %f", result.Score)
	}
}

func TestPageMeta(t *testing.T) {
	meta := PageMeta{
		Total:      100,
		Page:       1,
		PerPage:    10,
		TotalPages: 10,
		HasPrev:    false,
		HasNext:    true,
	}

	if meta.Total != 100 {
		t.Errorf("Expected total 100, got %d", meta.Total)
	}

	if !meta.HasNext {
		t.Error("Expected HasNext to be true")
	}
}

func TestListQuery(t *testing.T) {
	query := &ListQuery{
		Page:    1,
		PerPage: 20,
		Filters: map[string]interface{}{
			"status": "published",
		},
		Sort:   "created_at",
		Order:  "desc",
		Fields: []string{"id", "title", "created_at"},
	}

	if query.Page != 1 {
		t.Errorf("Expected page 1, got %d", query.Page)
	}

	if len(query.Fields) != 3 {
		t.Errorf("Expected 3 fields, got %d", len(query.Fields))
	}
}

// Mock implementations for interface compile checks

type mockDatabaseService struct{}

func (m *mockDatabaseService) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return nil, nil
}

func (m *mockDatabaseService) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return nil
}

func (m *mockDatabaseService) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return nil, nil
}

func (m *mockDatabaseService) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return nil, nil
}

func (m *mockDatabaseService) Ping(ctx context.Context) error {
	return nil
}

func (m *mockDatabaseService) Close() error {
	return nil
}

func (m *mockDatabaseService) Prepare(ctx context.Context, query string) (*sql.Stmt, error) {
	return nil, nil
}

func (m *mockDatabaseService) SchemaIntrospect(ctx context.Context) (*DatabaseSchema, error) {
	return nil, nil
}

type mockAuthService struct{}

func (m *mockAuthService) Authenticate(ctx context.Context, req *AuthRequest) (*AuthResult, error) {
	return nil, nil
}

func (m *mockAuthService) VerifyToken(ctx context.Context, token string) (*UserClaims, error) {
	return nil, nil
}

func (m *mockAuthService) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	return nil, nil
}

func (m *mockAuthService) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
	return nil, nil
}

func (m *mockAuthService) GetUser(ctx context.Context, identifier string) (*User, error) {
	return nil, nil
}

func (m *mockAuthService) HasPermission(ctx context.Context, userID string, resource, action string) (bool, error) {
	return false, nil
}

func (m *mockAuthService) CreateAPIToken(ctx context.Context, userID string, name string, expiresAt *time.Time) (*APIToken, error) {
	return nil, nil
}

func (m *mockAuthService) RevokeAPIToken(ctx context.Context, tokenID string) error {
	return nil
}

func (m *mockAuthService) UpdateUser(ctx context.Context, id string, req *UpdateUserRequest) (*User, error) {
	return nil, nil
}

func (m *mockAuthService) DeleteUser(ctx context.Context, id string) error {
	return nil
}

func (m *mockAuthService) ListUsers(ctx context.Context, query *UserQuery) (*Page, error) {
	return nil, nil
}

type mockContentService struct{}

func (m *mockContentService) Create(ctx context.Context, contentType string, data map[string]interface{}) (*Content, error) {
	return nil, nil
}

func (m *mockContentService) GetByID(ctx context.Context, id string) (*Content, error) {
	return nil, nil
}

func (m *mockContentService) Update(ctx context.Context, id string, data map[string]interface{}) (*Content, error) {
	return nil, nil
}

func (m *mockContentService) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockContentService) List(ctx context.Context, contentType string, query *ListQuery) (*Page, error) {
	return nil, nil
}

func (m *mockContentService) GetContentType(ctx context.Context, name string) (*ContentType, error) {
	return nil, nil
}

func (m *mockContentService) CreateContentType(ctx context.Context, ct *ContentType) (*ContentType, error) {
	return nil, nil
}

func (m *mockContentService) UpdateContentType(ctx context.Context, name string, ct *ContentType) (*ContentType, error) {
	return nil, nil
}

func (m *mockContentService) DeleteContentType(ctx context.Context, name string) error {
	return nil
}

func (m *mockContentService) ListContentTypes(ctx context.Context) ([]*ContentType, error) {
	return nil, nil
}

type mockMediaService struct{}

func (m *mockMediaService) Upload(ctx context.Context, file multipart.File, header *multipart.FileHeader, uploaderID string) (*MediaFile, error) {
	return nil, nil
}

func (m *mockMediaService) GetByID(ctx context.Context, id string) (*MediaFile, error) {
	return nil, nil
}

func (m *mockMediaService) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockMediaService) List(ctx context.Context, query *ListQuery) (*Page, error) {
	return nil, nil
}

func (m *mockMediaService) GetURL(ctx context.Context, id string) (string, error) {
	return "", nil
}

func (m *mockMediaService) GenerateThumbnail(ctx context.Context, id string, width, height int) (string, error) {
	return "", nil
}

func (m *mockMediaService) UploadFromReader(ctx context.Context, reader io.Reader, filename string, contentType string, uploaderID string) (*MediaFile, error) {
	return nil, nil
}

type mockSearchService struct{}

func (m *mockSearchService) Index(ctx context.Context, contentType string, content *Content) error {
	return nil
}

func (m *mockSearchService) Remove(ctx context.Context, id string) error {
	return nil
}

func (m *mockSearchService) Search(ctx context.Context, query *SearchQuery) (*SearchResponse, error) {
	return nil, nil
}

func (m *mockSearchService) Rebuild(ctx context.Context) error {
	return nil
}

func (m *mockSearchService) GetFacets(ctx context.Context, contentType string, fields []string) (map[string]map[string]int64, error) {
	return nil, nil
}

type mockCacheService struct{}

func (m *mockCacheService) Get(ctx context.Context, key string) (interface{}, bool) {
	return nil, false
}

func (m *mockCacheService) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return nil
}

func (m *mockCacheService) Delete(ctx context.Context, key string) error {
	return nil
}

func (m *mockCacheService) Invalidate(ctx context.Context, pattern string) error {
	return nil
}

func (m *mockCacheService) Stats(ctx context.Context) *CacheStats {
	return nil
}

func (m *mockCacheService) Flush(ctx context.Context) error {
	return nil
}

type mockQueueService struct{}

func (m *mockQueueService) RegisterTask(ctx context.Context, name string, handler TaskHandler) error {
	return nil
}

func (m *mockQueueService) Enqueue(ctx context.Context, name string, payload interface{}, options *TaskOptions) (string, error) {
	return "", nil
}

func (m *mockQueueService) GetStatus(ctx context.Context, taskID string) (*TaskStatus, error) {
	return nil, nil
}

func (m *mockQueueService) Close(ctx context.Context) error {
	return nil
}

func (m *mockQueueService) ListDeadLetters(ctx context.Context, page, pageSize int) ([]*DeadLetterEntry, int, error) {
	return nil, 0, nil
}

func (m *mockQueueService) RetryDeadLetter(ctx context.Context, taskID string) error {
	return nil
}

func (m *mockQueueService) DeleteDeadLetter(ctx context.Context, taskID string) error {
	return nil
}

func (m *mockQueueService) WorkerCount() int {
	return 0
}

type mockThemeService struct{}

func (m *mockThemeService) Render(ctx context.Context, templateName string, data map[string]interface{}) (string, error) {
	return "", nil
}

func (m *mockThemeService) GetActiveTheme(ctx context.Context) (string, error) {
	return "", nil
}

func (m *mockThemeService) SetActiveTheme(ctx context.Context, name string) error {
	return nil
}

func (m *mockThemeService) ListThemes(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (m *mockThemeService) InstallTheme(ctx context.Context, sourcePath string) error {
	return nil
}
