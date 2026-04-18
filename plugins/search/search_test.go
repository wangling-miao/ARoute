package search

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/core/services"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// ---------------------------------------------------------------------------
// mockContentService
// ---------------------------------------------------------------------------

type mockContentService struct {
	mu       sync.RWMutex
	contents map[string]*interfaces.Content
	types    map[string]*interfaces.ContentType
	errGet   error
}

func newMockContentService() *mockContentService {
	return &mockContentService{
		contents: make(map[string]*interfaces.Content),
		types:    make(map[string]*interfaces.ContentType),
	}
}

func (m *mockContentService) Create(ctx context.Context, ct string, data map[string]interface{}) (*interfaces.Content, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := fmt.Sprintf("id-%d", len(m.contents)+1)
	c := &interfaces.Content{
		ID:          id,
		ContentType: ct,
		Title:       fmt.Sprintf("Title %s", id),
		Slug:        fmt.Sprintf("slug-%s", id),
		AuthorID:    "user-1",
		Status:      "published",
		Data:        data,
	}
	m.contents[id] = c
	return c, nil
}

func (m *mockContentService) GetByID(ctx context.Context, id string) (*interfaces.Content, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.errGet != nil {
		return nil, m.errGet
	}
	c, ok := m.contents[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	return c, nil
}

func (m *mockContentService) Update(ctx context.Context, id string, data map[string]interface{}) (*interfaces.Content, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.contents[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	for k, v := range data {
		c.Data[k] = v
	}
	return c, nil
}

func (m *mockContentService) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.contents, id)
	return nil
}

func (m *mockContentService) List(ctx context.Context, contentType string, q *interfaces.ListQuery) (*interfaces.Page, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var items []map[string]interface{}
	for _, c := range m.contents {
		if c.ContentType == contentType && c.Status == "published" {
			items = append(items, contentToMap(c))
		}
	}
	return &interfaces.Page{
		Data: items,
		Meta: interfaces.PageMeta{
			Total:   int64(len(items)),
			Page:    q.Page,
			PerPage: q.PerPage,
			HasPrev: q.Page > 1,
			HasNext: false,
		},
	}, nil
}

func (m *mockContentService) GetContentType(ctx context.Context, name string) (*interfaces.ContentType, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ct, ok := m.types[name]
	if !ok {
		return nil, fmt.Errorf("not found: %s", name)
	}
	return ct, nil
}

func (m *mockContentService) CreateContentType(ctx context.Context, ct *interfaces.ContentType) (*interfaces.ContentType, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.types[ct.Name] = ct
	return ct, nil
}

func (m *mockContentService) UpdateContentType(ctx context.Context, name string, ct *interfaces.ContentType) (*interfaces.ContentType, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.types[name] = ct
	return ct, nil
}

func (m *mockContentService) DeleteContentType(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.types, name)
	return nil
}

func (m *mockContentService) ListContentTypes(ctx context.Context) ([]*interfaces.ContentType, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*interfaces.ContentType, 0, len(m.types))
	for _, ct := range m.types {
		result = append(result, ct)
	}
	return result, nil
}

// contentToMap converts a Content to a map for List results (used by Rebuild).
func contentToMap(c *interfaces.Content) map[string]interface{} {
	m := map[string]interface{}{
		"id":           c.ID,
		"content_type": c.ContentType,
		"title":        c.Title,
		"slug":         c.Slug,
		"author_id":    c.AuthorID,
		"status":       c.Status,
	}
	if c.PublishedAt != nil {
		m["published_at"] = c.PublishedAt.Format(time.RFC3339)
	}
	for k, v := range c.Data {
		m[k] = v
	}
	return m
}

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

func testContent(id, ct, title, body string) *interfaces.Content {
	return &interfaces.Content{
		ID:          id,
		ContentType: ct,
		Title:       title,
		Slug:        strings.ToLower(strings.ReplaceAll(title, " ", "-")),
		AuthorID:    "user-1",
		Status:      "published",
		Data: map[string]interface{}{
			"body": body,
		},
	}
}

func testContentWithCategory(id, ct, title, body, category string) *interfaces.Content {
	c := testContent(id, ct, title, body)
	c.Data["category"] = category
	return c
}

func setupTestService(t *testing.T) (*Service, *mockContentService, *events.EventBus) {
	t.Helper()
	tmpDir := t.TempDir()
	mockSvc := newMockContentService()
	eb := events.NewEventBus()
	svc, err := NewService(mockSvc, tmpDir, eb, slog.Default())
	require.NoError(t, err, "NewService should succeed")
	t.Cleanup(func() { svc.Close() })
	return svc, mockSvc, eb
}

func indexContent(t *testing.T, svc *Service, c *interfaces.Content) {
	t.Helper()
	ctx := context.Background()
	err := svc.Index(ctx, c.ContentType, c)
	require.NoError(t, err, "Index should succeed for %s/%s", c.ContentType, c.ID)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNewService(t *testing.T) {
	t.Run("valid dataDir", func(t *testing.T) {
		tmpDir := t.TempDir()
		mockSvc := newMockContentService()
		eb := events.NewEventBus()
		svc, err := NewService(mockSvc, tmpDir, eb, slog.Default())
		require.NoError(t, err)
		require.NotNil(t, svc)
		assert.Equal(t, tmpDir+"/search", svc.dataDir)
		svc.Close()
	})

	t.Run("creates search subdirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		mockSvc := newMockContentService()
		eb := events.NewEventBus()
		svc, err := NewService(mockSvc, tmpDir, eb, slog.Default())
		require.NoError(t, err)
		info, statErr := os.Stat(tmpDir + "/search")
		assert.NoError(t, statErr, "search subdirectory should exist")
		assert.True(t, info.IsDir())
		svc.Close()
	})
}

func TestIndexAndGetByID(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	c := testContent("test-1", "post", "Hello World", "This is a test article about Go programming.")
	indexContent(t, svc, c)

	// Search for the indexed item to verify it's searchable
	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "Go programming",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Total, "should find exactly 1 result")
	if len(resp.Hits) > 0 {
		assert.Equal(t, "test-1", resp.Hits[0].ID)
		assert.Equal(t, "post", resp.Hits[0].ContentType)
	}
}

func TestIndexMultipleContentTypes(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	post := testContent("p1", "post", "Blog Post", "A blog post about technology.")
	page := testContent("pg1", "page", "About Page", "Information about our company.")
	indexContent(t, svc, post)
	indexContent(t, svc, page)

	// Search across all indices
	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:   "technology",
		Page:    1,
		PerPage: 10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Total, "should find 1 result for technology")

	// Verify separate indices by searching only posts
	respPost, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "blog",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), respPost.Total, "should find 1 result in posts")

	// Verify page index
	respPage, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "company",
		ContentTypes: []string{"page"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), respPage.Total, "should find 1 result in pages")
}

func TestRemove(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	c := testContent("rm-1", "post", "To Be Removed", "This content will be deleted soon.")
	indexContent(t, svc, c)

	// Verify it exists
	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "deleted",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Total, "content should be found before removal")

	// Remove it
	err = svc.Remove(ctx, "rm-1")
	require.NoError(t, err)

	// Verify it's gone
	resp2, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "deleted",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), resp2.Total, "content should not be found after removal")
}

func TestSearchBasic(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	items := []*interfaces.Content{
		testContent("s1", "post", "Go Programming Guide", "Learn Go programming language basics and advanced patterns."),
		testContent("s2", "post", "Python Tutorial", "A comprehensive Python tutorial for beginners."),
		testContent("s3", "post", "Rust Systems Programming", "Build reliable systems software with Rust."),
		testContent("s4", "post", "JavaScript Web Development", "Modern JavaScript for web applications."),
	}
	for _, c := range items {
		indexContent(t, svc, c)
	}

	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:   "Go programming",
		Page:    1,
		PerPage: 10,
	})
	require.NoError(t, err)
	assert.True(t, resp.Total >= 1, "should find at least 1 result for 'Go programming'")

	// Verify the Go article is in results
	found := false
	for _, hit := range resp.Hits {
		if hit.ID == "s1" {
			found = true
			assert.Equal(t, "Go Programming Guide", hit.Title)
			break
		}
	}
	assert.True(t, found, "Go Programming Guide should be in search results")
}

func TestSearchPagination(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	// Index 25+ items
	for i := 0; i < 28; i++ {
		c := testContent(
			fmt.Sprintf("page-%d", i),
			"post",
			fmt.Sprintf("Article Number %d About Testing", i),
			fmt.Sprintf("Body content for article number %d about testing in Go.", i),
		)
		indexContent(t, svc, c)
	}

	// Get page 1 with 10 per page — bleve internally paginates, total reflects all matches
	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "testing",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(28), resp.Total, "total should be 28")
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 10, resp.PerPage)
	assert.Equal(t, 10, len(resp.Hits), "page 1 should have 10 hits")

	// Verify page metadata fields are set correctly
	assert.True(t, resp.Total >= int64(len(resp.Hits)), "total should be >= returned hits")
}

func TestSearchHighlight(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	c := testContent("hl-1", "post", "Highlighting Test", "This article discusses advanced search highlighting features in detail.")
	indexContent(t, svc, c)

	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "highlighting",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
		Highlight:    true,
	})
	require.NoError(t, err)
	require.Len(t, resp.Hits, 1, "should find exactly 1 result")

	hit := resp.Hits[0]
	assert.Equal(t, "hl-1", hit.ID)
	assert.NotEmpty(t, hit.HighlightedFields, "HighlightedFields should not be empty")

	// At least one highlighted field should contain <mark> tags
	hasMark := false
	for _, fragment := range hit.HighlightedFields {
		if strings.Contains(fragment, "<mark>") {
			hasMark = true
			break
		}
	}
	assert.True(t, hasMark, "at least one highlighted field should contain <mark> tags")
}

func TestSearchByContentType(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	posts := []*interfaces.Content{
		testContent("ctp-1", "post", "Post One", "Content about golang development."),
		testContent("ctp-2", "post", "Post Two", "More golang development tips."),
	}
	pages := []*interfaces.Content{
		testContent("ctp-3", "page", "Page One", "Golang development best practices."),
	}
	for _, c := range posts {
		indexContent(t, svc, c)
	}
	for _, c := range pages {
		indexContent(t, svc, c)
	}

	// Search only posts
	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "golang",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), resp.Total, "should find 2 posts")
	for _, hit := range resp.Hits {
		assert.Equal(t, "post", hit.ContentType, "all results should be posts")
	}

	// Search only pages
	respPage, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "golang",
		ContentTypes: []string{"page"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), respPage.Total, "should find 1 page")
}

func TestSearchEmptyQuery(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	// No indices exist yet — should return empty results without error
	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:   "anything",
		Page:    1,
		PerPage: 10,
	})
	require.NoError(t, err)
	assert.Empty(t, resp.Hits, "empty indices should return empty hits")
	assert.Equal(t, int64(0), resp.Total)

	// Also test with empty content types filter but existing index
	c := testContent("eq-1", "post", "Some Title", "Some body text.")
	indexContent(t, svc, c)

	resp2, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "zzzzznonexistent12345",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.True(t, resp2.Total == 0, "should return 0 for truly non-matching query, got %d", resp2.Total)
}

func TestSearchChineseContent(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	c := &interfaces.Content{
		ID:          "cn-1",
		ContentType: "post",
		Title:       "Go语言编程指南",
		Slug:        "go-language-guide",
		AuthorID:    "user-1",
		Status:      "published",
		Data: map[string]interface{}{
			"body": "这是一个测试文章关于Go语言编程和软件开发",
		},
	}
	indexContent(t, svc, c)

	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "Go语言",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.True(t, resp.Total >= 1, "should find Chinese content with 'Go语言'")

	resp2, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "编程",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.True(t, resp2.Total >= 1, "should find Chinese content with '编程'")
}

func TestGetFacets(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	items := []*interfaces.Content{
		testContentWithCategory("f-1", "post", "Tech Post 1", "Tech content one", "technology"),
		testContentWithCategory("f-2", "post", "Tech Post 2", "Tech content two", "technology"),
		testContentWithCategory("f-3", "post", "Science Post", "Science content", "science"),
		testContentWithCategory("f-4", "post", "News Post", "News content", "news"),
	}
	for _, c := range items {
		indexContent(t, svc, c)
	}

	facets, err := svc.GetFacets(ctx, "post", []string{"category"})
	require.NoError(t, err)
	require.Contains(t, facets, "category", "should have category facet")

	catFacets := facets["category"]
	assert.Equal(t, int64(2), catFacets["technology"], "technology count should be 2")
	assert.Equal(t, int64(1), catFacets["science"], "science count should be 1")
	assert.Equal(t, int64(1), catFacets["news"], "news count should be 1")
}

func TestGetFacetsEmptyIndex(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	facets, err := svc.GetFacets(ctx, "nonexistent", []string{"category"})
	assert.Error(t, err, "should return error for nonexistent content type")
	assert.Nil(t, facets)
	assert.Contains(t, err.Error(), "no index found")
}

func TestRebuild(t *testing.T) {
	svc, mockSvc, _ := setupTestService(t)
	ctx := context.Background()

	// Seed mock content service with data
	c1 := testContent("rb-1", "post", "Rebuild Post 1", "Post for rebuild test one.")
	c2 := testContent("rb-2", "post", "Rebuild Post 2", "Post for rebuild test two.")
	mockSvc.contents["rb-1"] = c1
	mockSvc.contents["rb-2"] = c2

	// Also add a page
	c3 := testContent("rb-3", "page", "Rebuild Page", "Page for rebuild test.")
	mockSvc.contents["rb-3"] = c3

	// Index some items, then rebuild
	indexContent(t, svc, testContent("old-1", "post", "Old Post", "This should be cleared after rebuild."))

	err := svc.Rebuild(ctx)
	require.NoError(t, err)

	// After rebuild, the "old-1" item (which isn't in mockSvc) should be gone
	// The rb-1 and rb-2 posts should be indexed
	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "rebuild",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.True(t, resp.Total >= 1, "should find rebuilt content")
}

func TestHandleContentEventCreated(t *testing.T) {
	svc, mockSvc, _ := setupTestService(t)
	ctx := context.Background()

	// Put content in mock service
	c := testContent("ev-1", "post", "Event Created Test", "This content is auto-indexed via event.")
	mockSvc.contents["ev-1"] = c

	// Simulate content.created event
	svc.HandleContentEvent(ctx, events.Event{
		Topic: "content.post.created",
		Data: map[string]interface{}{
			"id":           "ev-1",
			"content_type": "post",
		},
	})

	// Verify it was indexed
	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "auto-indexed",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Total, "auto-indexed content should be searchable")
}

func TestHandleContentEventUpdated(t *testing.T) {
	svc, mockSvc, _ := setupTestService(t)
	ctx := context.Background()

	// Original content
	c := testContent("ev-2", "post", "Original Title", "Original body content.")
	mockSvc.contents["ev-2"] = c

	// Index original
	indexContent(t, svc, c)

	// Update the content in mock
	c.Title = "Updated Title"
	c.Data["body"] = "Updated body content with new information."
	mockSvc.contents["ev-2"] = c

	// Simulate update event
	svc.HandleContentEvent(ctx, events.Event{
		Topic: "content.post.updated",
		Data: map[string]interface{}{
			"id":           "ev-2",
			"content_type": "post",
		},
	})

	// Search for updated content
	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "new information",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Total, "updated content should be searchable")
}

func TestHandleContentEventDeleted(t *testing.T) {
	svc, mockSvc, _ := setupTestService(t)
	ctx := context.Background()

	// Index content
	c := testContent("ev-3", "post", "To Be Deleted", "This content will be deleted via event.")
	indexContent(t, svc, c)
	mockSvc.contents["ev-3"] = c

	// Verify it exists
	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "deleted via event",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Total)

	// Simulate delete event
	svc.HandleContentEvent(ctx, events.Event{
		Topic: "content.post.deleted",
		Data: map[string]interface{}{
			"id": "ev-3",
		},
	})

	// Verify it's gone
	resp2, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "deleted via event",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), resp2.Total, "content should be removed after delete event")
}

func TestHandleContentEventInvalidTopic(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	// Should not panic with short topic
	assert.NotPanics(t, func() {
		svc.HandleContentEvent(ctx, events.Event{
			Topic: "content",
			Data:  map[string]interface{}{"id": "x"},
		})
	})

	// Should not panic with very short topic
	assert.NotPanics(t, func() {
		svc.HandleContentEvent(ctx, events.Event{
			Topic: "ab",
			Data:  map[string]interface{}{"id": "x"},
		})
	})
}

func TestHandleContentEventEmptyID(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	// Should not take action with empty ID
	assert.NotPanics(t, func() {
		svc.HandleContentEvent(ctx, events.Event{
			Topic: "content.post.created",
			Data: map[string]interface{}{
				"id":           "",
				"content_type": "post",
			},
		})
	})

	// Should not take action with no id at all
	assert.NotPanics(t, func() {
		svc.HandleContentEvent(ctx, events.Event{
			Topic: "content.post.created",
			Data:  map[string]interface{}{},
		})
	})
}

func TestClose(t *testing.T) {
	tmpDir := t.TempDir()
	mockSvc := newMockContentService()
	eb := events.NewEventBus()
	svc, err := NewService(mockSvc, tmpDir, eb, slog.Default())
	require.NoError(t, err)

	// Index some content
	indexContent(t, svc, testContent("cl-1", "post", "Close Test", "Content before close."))

	// Close should not panic
	assert.NotPanics(t, func() {
		svc.Close()
	})

	// After close, indices map should be empty
	assert.Empty(t, svc.indices, "indices should be cleared after close")

	// Double close should not panic
	assert.NotPanics(t, func() {
		svc.Close()
	})
}

func TestBuildSearchDocument(t *testing.T) {
	now := time.Now()
	c := &interfaces.Content{
		ID:          "bd-1",
		ContentType: "post",
		Title:       "Build Doc Test",
		Slug:        "build-doc-test",
		AuthorID:    "user-42",
		Status:      "published",
		Data: map[string]interface{}{
			"body":     "Article body text.",
			"category": "tech",
			"tags":     []interface{}{"go", "testing"},
		},
		PublishedAt: &now,
	}

	doc := (&Service{}).buildSearchDocument(c)

	assert.Equal(t, "bd-1", doc["id"])
	assert.Equal(t, "post", doc["content_type"])
	assert.Equal(t, "Build Doc Test", doc["title"])
	assert.Equal(t, "build-doc-test", doc["slug"])
	assert.Equal(t, "user-42", doc["author_id"])
	assert.Equal(t, "published", doc["status"])
	assert.Equal(t, "Article body text.", doc["body"])
	assert.Equal(t, "tech", doc["category"])
	assert.Equal(t, now.Format(time.RFC3339), doc["published_at"])

	// Tags should be []string
	tags, ok := doc["tags"].([]string)
	require.True(t, ok, "tags should be []string")
	assert.Equal(t, []string{"go", "testing"}, tags)
}

func TestBuildSearchDocument_NilData(t *testing.T) {
	c := &interfaces.Content{
		ID:          "bd-2",
		ContentType: "page",
		Title:       "Nil Data",
		Slug:        "nil-data",
		AuthorID:    "user-1",
		Status:      "draft",
		Data:        nil,
	}

	doc := (&Service{}).buildSearchDocument(c)
	assert.Equal(t, "bd-2", doc["id"])
	assert.Equal(t, "draft", doc["status"])
	// No panic, and no Data fields
	_, hasBody := doc["body"]
	assert.False(t, hasBody, "body should not be present when Data is nil")
}

func TestBuildSearchDocument_DefaultType(t *testing.T) {
	c := &interfaces.Content{
		ID:          "bd-3",
		ContentType: "post",
		Title:       "Mixed Types",
		Slug:        "mixed-types",
		AuthorID:    "user-1",
		Status:      "published",
		Data: map[string]interface{}{
			"count": 42,                     // int — not string, not []interface{}
			"tags":  []interface{}{1, 2, 3}, // non-string items in slice
		},
	}

	doc := (&Service{}).buildSearchDocument(c)
	assert.Equal(t, "42", doc["count"], "non-string, non-slice values should be fmt.Sprinted")

	// tags: non-string items should be filtered out, resulting in empty slice, so no "tags" key
	_, hasTags := doc["tags"]
	assert.False(t, hasTags, "tags with no string items should not be present")
}

func TestPluginNew(t *testing.T) {
	p := New()
	require.NotNil(t, p, "Plugin.New() should return non-nil plugin")
	assert.Equal(t, "search", p.Name(), "plugin name should be 'search'")
	assert.Equal(t, "1.0.0", p.Version(), "plugin version should be '1.0.0'")
	assert.NotNil(t, p.Manifest(), "manifest should be loaded")
}

func TestMapToContent(t *testing.T) {
	ts := "2025-01-15T10:30:00Z"
	m := map[string]interface{}{
		"id":           "map-1",
		"content_type": "post",
		"title":        "Map Test",
		"slug":         "map-test",
		"author_id":    "user-1",
		"status":       "published",
		"published_at": ts,
		"body":         "Body from map data",
		"category":     "testing",
	}

	c := mapToContent(m)
	assert.Equal(t, "map-1", c.ID)
	assert.Equal(t, "post", c.ContentType)
	assert.Equal(t, "Map Test", c.Title)
	assert.Equal(t, "map-test", c.Slug)
	assert.Equal(t, "user-1", c.AuthorID)
	assert.Equal(t, "published", c.Status)
	require.NotNil(t, c.PublishedAt)
	assert.Equal(t, ts, c.PublishedAt.Format(time.RFC3339))

	// Non-excluded fields go to Data
	assert.Equal(t, "Body from map data", c.Data["body"])
	assert.Equal(t, "testing", c.Data["category"])
}

func TestMapToContent_EmptyFields(t *testing.T) {
	m := map[string]interface{}{
		"id":    123, // not string
		"title": "Only Title",
	}

	c := mapToContent(m)
	assert.Equal(t, "", c.ID, "non-string id should result in empty string")
	assert.Equal(t, "Only Title", c.Title)
	assert.Nil(t, c.PublishedAt, "missing published_at should be nil")
	assert.Empty(t, c.ContentType)
}

func TestHandleContentEvent_GetByIDError(t *testing.T) {
	svc, mockSvc, _ := setupTestService(t)
	ctx := context.Background()

	// Set mock to return error on GetByID
	mockSvc.errGet = fmt.Errorf("database error")

	// Should not panic, should log error internally
	assert.NotPanics(t, func() {
		svc.HandleContentEvent(ctx, events.Event{
			Topic: "content.post.created",
			Data: map[string]interface{}{
				"id":           "ev-err",
				"content_type": "post",
			},
		})
	})
}

func TestHandleContentEvent_EmptyContentType(t *testing.T) {
	svc, mockSvc, _ := setupTestService(t)
	ctx := context.Background()

	c := testContent("ev-noct", "post", "No CT", "Content without content type in event.")
	mockSvc.contents["ev-noct"] = c

	// created/updated with empty content_type should return early
	assert.NotPanics(t, func() {
		svc.HandleContentEvent(ctx, events.Event{
			Topic: "content.post.created",
			Data: map[string]interface{}{
				"id":           "ev-noct",
				"content_type": "",
			},
		})
	})
}

func TestSearchDefaultPagination(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	c := testContent("dp-1", "post", "Default Page", "Content for default pagination test.")
	indexContent(t, svc, c)

	// Page=0 and PerPage=0 should default to page=1, perPage=20
	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "pagination",
		ContentTypes: []string{"post"},
		Page:         0,
		PerPage:      0,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Page, "default page should be 1")
	assert.Equal(t, 20, resp.PerPage, "default perPage should be 20")
}

func TestPluginStartStop(t *testing.T) {
	p := New()
	require.NotNil(t, p)

	tmpDir := t.TempDir()
	mockSvc := newMockContentService()
	container := services.NewContainer()
	err := container.Provide(func(c *services.Container) (interfaces.ContentService, error) {
		return mockSvc, nil
	})
	require.NoError(t, err)
	eb := events.NewEventBus()
	coreCtx := core.NewCoreContext(
		context.Background(),
		container,
		eb,
		nil,
		slog.Default(),
		tmpDir,
		tmpDir,
	)
	err = p.Init(coreCtx)
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)
	assert.True(t, p.running)

	// Double start is no-op
	err = p.Start()
	require.NoError(t, err)

	err = p.Stop()
	require.NoError(t, err)
	assert.False(t, p.running)

	// Double stop is no-op
	err = p.Stop()
	require.NoError(t, err)
}

func TestPluginInit(t *testing.T) {
	tmpDir := t.TempDir()
	mockSvc := newMockContentService()

	container := services.NewContainer()
	err := container.Provide(func(c *services.Container) (interfaces.ContentService, error) {
		return mockSvc, nil
	})
	require.NoError(t, err)

	eb := events.NewEventBus()
	coreCtx := core.NewCoreContext(
		context.Background(),
		container,
		eb,
		nil,
		slog.Default(),
		tmpDir,
		tmpDir,
	)

	p := New()
	require.NotNil(t, p)

	err = p.Init(coreCtx)
	require.NoError(t, err)
	assert.NotNil(t, p.service)

	// Verify SearchService was registered in container
	var searchSvc interfaces.SearchService
	err = container.Get(&searchSvc)
	require.NoError(t, err)
	assert.NotNil(t, searchSvc)

	// Cleanup
	p.service.Close()
}

func TestRemoveNonExistentDoc(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	// Create an index first so Remove has something to iterate
	indexContent(t, svc, testContent("rn-1", "post", "Real Doc", "Real content."))

	// Remove a doc that doesn't exist in any index — should not error
	err := svc.Remove(ctx, "nonexistent-id")
	require.NoError(t, err)
}

func TestSearchExcerptFromFields(t *testing.T) {
	svc, _, _ := setupTestService(t)
	ctx := context.Background()

	c := &interfaces.Content{
		ID:          "exc-1",
		ContentType: "post",
		Title:       "Excerpt Test",
		Slug:        "excerpt-test",
		AuthorID:    "user-1",
		Status:      "published",
		Data: map[string]interface{}{
			"description": "A long description about search excerpt generation that should appear as the excerpt field.",
		},
	}
	indexContent(t, svc, c)

	resp, err := svc.Search(ctx, &interfaces.SearchQuery{
		Query:        "excerpt",
		ContentTypes: []string{"post"},
		Page:         1,
		PerPage:      10,
	})
	require.NoError(t, err)
	require.Len(t, resp.Hits, 1)
	assert.NotEmpty(t, resp.Hits[0].Excerpt, "excerpt should be populated from description field")
}

func TestBuildSearchDocument_PublishedAt(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	c := &interfaces.Content{
		ID:          "pa-1",
		ContentType: "post",
		Title:       "Pub Test",
		Slug:        "pub-test",
		AuthorID:    "user-1",
		Status:      "published",
		PublishedAt: &now,
		Data:        map[string]interface{}{},
	}

	doc := (&Service{}).buildSearchDocument(c)
	assert.Equal(t, "2025-06-15T12:00:00Z", doc["published_at"])
}

func TestMapToContent_InvalidPublishedAt(t *testing.T) {
	m := map[string]interface{}{
		"id":           "mc-1",
		"published_at": "not-a-valid-date",
	}

	c := mapToContent(m)
	assert.Nil(t, c.PublishedAt, "invalid date should result in nil PublishedAt")
}

func TestMapToContent_EmptyPublishedAt(t *testing.T) {
	m := map[string]interface{}{
		"id":           "mc-2",
		"published_at": "",
	}

	c := mapToContent(m)
	assert.Nil(t, c.PublishedAt, "empty published_at should result in nil")
}

// ---------------------------------------------------------------------------
// Security fix tests
// ---------------------------------------------------------------------------

func TestSearchEmptyQuery_Rejected(t *testing.T) {
	svc, _, _ := setupTestService(t)

	_, err := svc.Search(context.Background(), &interfaces.SearchQuery{
		Query:   "",
		PerPage: 10,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestSearchWhitespaceQuery_Rejected(t *testing.T) {
	svc, _, _ := setupTestService(t)

	_, err := svc.Search(context.Background(), &interfaces.SearchQuery{
		Query:   "   ",
		PerPage: 10,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestSearchPaginationCap(t *testing.T) {
	svc, _, _ := setupTestService(t)

	idx, err := svc.getOrCreateIndex("post")
	require.NoError(t, err)
	require.NotNil(t, idx)

	for i := 0; i < 5; i++ {
		content := &interfaces.Content{
			ID:          fmt.Sprintf("cap-%d", i),
			ContentType: "post",
			Title:       "pagination cap test",
			Status:      "published",
		}
		require.NoError(t, svc.Index(context.Background(), "post", content))
	}

	resp, err := svc.Search(context.Background(), &interfaces.SearchQuery{
		Query:   "pagination",
		PerPage: 9999,
	})
	require.NoError(t, err)
	assert.LessOrEqual(t, resp.PerPage, 100, "perPage should be capped at 100")
}

func TestIndexInvalidContentType_PathTraversal(t *testing.T) {
	svc, _, _ := setupTestService(t)

	content := &interfaces.Content{
		ID:          "evil-1",
		ContentType: "../../etc",
		Title:       "path traversal attempt",
	}

	err := svc.Index(context.Background(), "../../etc", content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid character")
}

func TestIndexInvalidContentType_AbsolutePath(t *testing.T) {
	svc, _, _ := setupTestService(t)

	content := &interfaces.Content{
		ID:          "evil-2",
		ContentType: "/tmp/evil",
		Title:       "absolute path attempt",
	}

	err := svc.Index(context.Background(), "/tmp/evil", content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid character")
}

func TestHandleContentEvent_NilData(t *testing.T) {
	svc, _, _ := setupTestService(t)

	assert.NotPanics(t, func() {
		svc.HandleContentEvent(context.Background(), events.Event{
			Topic: "content.post.created",
			Data:  nil,
		})
	})
}

func TestBuildSearchDocument_ExcludesSensitiveFields(t *testing.T) {
	svc, _, _ := setupTestService(t)

	c := &interfaces.Content{
		ID:          "sensitive-1",
		ContentType: "post",
		Title:       "Test",
		Data: map[string]interface{}{
			"body":          "hello world",
			"password":      "secret123",
			"password_hash": "$2a$10$...",
			"token":         "abc123",
			"email":         "user@example.com",
		},
	}

	doc := svc.buildSearchDocument(c)
	assert.Equal(t, "hello world", doc["body"])
	assert.Nil(t, doc["password"])
	assert.Nil(t, doc["password_hash"])
	assert.Nil(t, doc["token"])
	assert.Nil(t, doc["email"])
}

func TestSanitizeContentType_Valid(t *testing.T) {
	valid := []string{"post", "my-type", "content_type", "Post123", "a_b-C"}
	for _, ct := range valid {
		result, err := sanitizeContentType(ct)
		assert.NoError(t, err, "should accept %q", ct)
		assert.Equal(t, ct, result)
	}
}

func TestSanitizeContentType_Invalid(t *testing.T) {
	invalid := []string{"../../etc", "/tmp/evil", "type with spaces", "type/slash", "type\\backslash", ""}
	for _, ct := range invalid {
		_, err := sanitizeContentType(ct)
		assert.Error(t, err, "should reject %q", ct)
	}
}
