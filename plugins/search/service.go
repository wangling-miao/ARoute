package search

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	_ "github.com/vcaesar/gse-bleve"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// Service implements interfaces.SearchService with Bleve full-text search
// and gse Chinese tokenization.
type Service struct {
	mu         sync.RWMutex
	contentSvc interfaces.ContentService
	events     core.EventBus
	logger     *slog.Logger
	dataDir    string
	indices    map[string]bleve.Index
}

// sanitizeContentType validates a content type name to prevent path traversal.
// Only alphanumeric characters, underscores, and hyphens are permitted.
func sanitizeContentType(ct string) (string, error) {
	if ct == "" {
		return "", fmt.Errorf("content type must not be empty")
	}
	for _, r := range ct {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-') {
			return "", fmt.Errorf("content type %q contains invalid character: %q", ct, r)
		}
	}
	return ct, nil
}

// NewService creates a new search service with bleve index storage.
func NewService(contentSvc interfaces.ContentService, dataDir string, events core.EventBus, logger *slog.Logger) (*Service, error) {
	s := &Service{
		contentSvc: contentSvc,
		events:     events,
		logger:     logger,
		dataDir:    filepath.Join(dataDir, "search"),
		indices:    make(map[string]bleve.Index),
	}
	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create search data dir: %w", err)
	}
	return s, nil
}

func (s *Service) buildIndexMapping() (mapping.IndexMapping, error) {
	m := bleve.NewIndexMapping()

	if err := m.AddCustomTokenizer("gse", map[string]interface{}{
		"type":       "gse",
		"user_dicts": "embed, zh, en",
	}); err != nil {
		return nil, fmt.Errorf("add gse tokenizer: %w", err)
	}

	if err := m.AddCustomAnalyzer("gse", map[string]interface{}{
		"type":      "gse",
		"tokenizer": "gse",
	}); err != nil {
		return nil, fmt.Errorf("add gse analyzer: %w", err)
	}

	m.DefaultAnalyzer = "gse"

	defaultMapping := bleve.NewDocumentMapping()

	titleField := bleve.NewTextFieldMapping()
	titleField.Analyzer = "gse"
	titleField.Store = true
	titleField.IncludeInAll = true
	titleField.IncludeTermVectors = true
	defaultMapping.AddFieldMappingsAt("title", titleField)

	contentField := bleve.NewTextFieldMapping()
	contentField.Analyzer = "gse"
	contentField.Store = true
	contentField.IncludeInAll = true
	contentField.IncludeTermVectors = true
	defaultMapping.AddFieldMappingsAt("body", contentField)
	defaultMapping.AddFieldMappingsAt("content", contentField)
	defaultMapping.AddFieldMappingsAt("description", contentField)
	defaultMapping.AddFieldMappingsAt("excerpt", contentField)

	categoryField := bleve.NewKeywordFieldMapping()
	categoryField.Store = true
	defaultMapping.AddFieldMappingsAt("category", categoryField)

	statusField := bleve.NewKeywordFieldMapping()
	statusField.Store = true
	defaultMapping.AddFieldMappingsAt("status", statusField)

	authorField := bleve.NewKeywordFieldMapping()
	authorField.Store = true
	defaultMapping.AddFieldMappingsAt("author_id", authorField)

	tagsField := bleve.NewKeywordFieldMapping()
	tagsField.Store = true
	defaultMapping.AddFieldMappingsAt("tags", tagsField)

	slugField := bleve.NewTextFieldMapping()
	slugField.Analyzer = "standard"
	slugField.Store = true
	defaultMapping.AddFieldMappingsAt("slug", slugField)

	m.DefaultMapping = defaultMapping
	m.DefaultMapping.Dynamic = true

	return m, nil
}

func (s *Service) getOrCreateIndex(contentType string) (bleve.Index, error) {
	if _, err := sanitizeContentType(contentType); err != nil {
		return nil, fmt.Errorf("invalid content type: %w", err)
	}

	s.mu.RLock()
	if idx, ok := s.indices[contentType]; ok {
		s.mu.RUnlock()
		return idx, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if idx, ok := s.indices[contentType]; ok {
		return idx, nil
	}

	indexPath := filepath.Join(s.dataDir, contentType+".bleve")
	mapping, err := s.buildIndexMapping()
	if err != nil {
		return nil, fmt.Errorf("build index mapping: %w", err)
	}

	var idx bleve.Index
	if _, statErr := os.Stat(indexPath); os.IsNotExist(statErr) {
		idx, err = bleve.New(indexPath, mapping)
	} else {
		idx, err = bleve.Open(indexPath)
	}
	if err != nil {
		return nil, fmt.Errorf("open/create index at %s: %w", indexPath, err)
	}

	s.indices[contentType] = idx
	return idx, nil
}

// excludedSearchFields contains fields that should never appear in search results
// to prevent leaking sensitive data.
var excludedSearchFields = map[string]bool{
	"password": true, "password_hash": true, "token": true, "secret": true,
	"api_key": true, "private_key": true, "email": true, "phone": true,
}

func (s *Service) buildSearchDocument(c *interfaces.Content) map[string]interface{} {
	doc := map[string]interface{}{
		"id":           c.ID,
		"content_type": c.ContentType,
		"title":        c.Title,
		"slug":         c.Slug,
		"author_id":    c.AuthorID,
		"status":       c.Status,
	}

	if c.Data != nil {
		for key, val := range c.Data {
			if excludedSearchFields[key] {
				continue
			}
			switch v := val.(type) {
			case string:
				doc[key] = v
			case []interface{}:
				strs := make([]string, 0, len(v))
				for _, item := range v {
					if str, ok := item.(string); ok {
						strs = append(strs, str)
					}
				}
				if len(strs) > 0 {
					doc[key] = strs
				}
			default:
				doc[key] = fmt.Sprintf("%v", v)
			}
		}
	}

	if c.PublishedAt != nil {
		doc["published_at"] = c.PublishedAt.Format(time.RFC3339)
	}

	return doc
}

// Index adds or updates a content item in the search index.
func (s *Service) Index(ctx context.Context, contentType string, content *interfaces.Content) error {
	idx, err := s.getOrCreateIndex(contentType)
	if err != nil {
		return fmt.Errorf("get index for %s: %w", contentType, err)
	}

	doc := s.buildSearchDocument(content)
	if err := idx.Index(content.ID, doc); err != nil {
		return fmt.Errorf("index document %s: %w", content.ID, err)
	}

	s.logger.Debug("Indexed content", "content_type", contentType, "id", content.ID)
	return nil
}

// Remove removes a content item from all search indices.
func (s *Service) Remove(ctx context.Context, id string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for contentType, idx := range s.indices {
		if err := idx.Delete(id); err != nil {
			s.logger.Debug("Document not in index during remove",
				"content_type", contentType, "id", id)
		}
	}
	return nil
}

// Search performs a full-text search across content with highlighting and pagination.
func (s *Service) Search(ctx context.Context, query *interfaces.SearchQuery) (*interfaces.SearchResponse, error) {
	if strings.TrimSpace(query.Query) == "" {
		return nil, fmt.Errorf("search query must not be empty")
	}

	s.mu.RLock()
	var searchIndices []bleve.Index
	if len(query.ContentTypes) > 0 {
		for _, ct := range query.ContentTypes {
			if idx, ok := s.indices[ct]; ok {
				searchIndices = append(searchIndices, idx)
			}
		}
	} else {
		for _, idx := range s.indices {
			searchIndices = append(searchIndices, idx)
		}
	}
	s.mu.RUnlock()

	if len(searchIndices) == 0 {
		return &interfaces.SearchResponse{
			Hits:    []*interfaces.SearchResult{},
			Total:   0,
			Page:    query.Page,
			PerPage: query.PerPage,
		}, nil
	}

	bleveQuery := bleve.NewQueryStringQuery(query.Query)

	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	page := query.Page
	if page <= 0 {
		page = 1
	}

	req := bleve.NewSearchRequest(bleveQuery)
	req.Size = perPage
	req.From = (page - 1) * perPage
	req.Fields = []string{"*"}

	if query.Highlight {
		req.Highlight = bleve.NewHighlightWithStyle("html")
		req.Highlight.AddField("title")
		req.Highlight.AddField("body")
		req.Highlight.AddField("content")
		req.Highlight.AddField("description")
		req.Highlight.AddField("excerpt")
	}

	var allHits []*interfaces.SearchResult
	var totalHits int64

	for _, idx := range searchIndices {
		result, err := idx.Search(req)
		if err != nil {
			s.logger.Error("Search failed on index", "error", err)
			continue
		}

		totalHits += int64(result.Total)

		for _, hit := range result.Hits {
			sr := &interfaces.SearchResult{
				ID:                hit.ID,
				Score:             hit.Score,
				HighlightedFields: make(map[string]string),
			}

			if ct, ok := hit.Fields["content_type"].(string); ok {
				sr.ContentType = ct
			}
			if title, ok := hit.Fields["title"].(string); ok {
				sr.Title = title
			}

			if hit.Fragments != nil {
				for field, fragments := range hit.Fragments {
					if len(fragments) > 0 {
						sr.HighlightedFields[field] = fragments[0]
						if field == "body" || field == "content" || field == "description" {
							sr.Excerpt = fragments[0]
						}
					}
				}
			} else {
				for _, field := range []string{"body", "content", "description", "excerpt"} {
					if val, ok := hit.Fields[field].(string); ok && val != "" {
						if len(val) > 300 {
							sr.Excerpt = val[:300] + "..."
						} else {
							sr.Excerpt = val
						}
						break
					}
				}
			}

			sr.Data = make(map[string]interface{})
			for k, v := range hit.Fields {
				if k != "content_type" && k != "title" && !excludedSearchFields[k] {
					sr.Data[k] = v
				}
			}

			allHits = append(allHits, sr)
		}
	}

	sort.Slice(allHits, func(i, j int) bool {
		return allHits[i].Score > allHits[j].Score
	})

	start := (page - 1) * perPage
	end := start + perPage
	if start > len(allHits) {
		start = len(allHits)
	}
	if end > len(allHits) {
		end = len(allHits)
	}

	return &interfaces.SearchResponse{
		Hits:    allHits[start:end],
		Total:   totalHits,
		Page:    page,
		PerPage: perPage,
	}, nil
}

// GetFacets returns aggregated term counts for specified fields within a content type.
func (s *Service) GetFacets(ctx context.Context, contentType string, fields []string) (map[string]map[string]int64, error) {
	s.mu.RLock()
	idx, ok := s.indices[contentType]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no index found for content type: %s", contentType)
	}

	query := bleve.NewMatchAllQuery()
	req := bleve.NewSearchRequest(query)
	req.Size = 0

	for _, field := range fields {
		facet := bleve.NewFacetRequest(field, 100)
		req.AddFacet(field, facet)
	}

	result, err := idx.Search(req)
	if err != nil {
		return nil, fmt.Errorf("facet search: %w", err)
	}

	facets := make(map[string]map[string]int64)
	for name, facetResult := range result.Facets {
		facets[name] = make(map[string]int64)
		for _, term := range facetResult.Terms.Terms() {
			facets[name][term.Term] = int64(term.Count)
		}
	}

	return facets, nil
}

// Rebuild clears and rebuilds the entire search index from the content service.
// It holds the write lock for the entire operation since this is an admin-only
// operation and blocking reads during rebuild is acceptable.
func (s *Service) Rebuild(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for ct, idx := range s.indices {
		if err := idx.Close(); err != nil {
			s.logger.Warn("Failed to close index during rebuild", "content_type", ct, "error", err)
		}
		indexPath := filepath.Join(s.dataDir, ct+".bleve")
		if err := os.RemoveAll(indexPath); err != nil {
			s.logger.Warn("Failed to remove index directory during rebuild", "path", indexPath, "error", err)
		}
	}
	s.indices = make(map[string]bleve.Index)

	mapping, err := s.buildIndexMapping()
	if err != nil {
		return fmt.Errorf("build index mapping for rebuild: %w", err)
	}

	contentTypes := []string{"page", "post", "category", "tag"}

	var totalIndexed int64
	for _, ct := range contentTypes {
		// Create the bleve index directly (not via getOrCreateIndex which
		// would deadlock on s.mu).
		indexPath := filepath.Join(s.dataDir, ct+".bleve")
		idx, createErr := bleve.New(indexPath, mapping)
		if createErr != nil {
			s.logger.Debug("Skip content type during rebuild, failed to create index",
				"content_type", ct, "error", createErr)
			continue
		}
		s.indices[ct] = idx

		page := 1
		perPage := 100
		for {
			results, err := s.contentSvc.List(ctx, ct, &interfaces.ListQuery{
				Page:    page,
				PerPage: perPage,
				Filters: map[string]interface{}{
					"status": "published",
				},
			})
			if err != nil {
				s.logger.Debug("Skip content type during rebuild",
					"content_type", ct, "error", err)
				break
			}

			items, ok := results.Data.([]map[string]interface{})
			if !ok {
				break
			}

			for _, item := range items {
				content := mapToContent(item)
				doc := s.buildSearchDocument(content)
				if err := idx.Index(content.ID, doc); err != nil {
					s.logger.Error("Failed to index during rebuild",
						"id", content.ID, "error", err)
					continue
				}
				totalIndexed++
			}

			if results.Meta.Page*results.Meta.PerPage >= int(results.Meta.Total) {
				break
			}
			page++
		}
	}

	s.logger.Info("Index rebuild complete", "total_indexed", totalIndexed)
	return nil
}

// HandleContentEvent is the EventBus broadcast handler for auto-indexing on content changes.
func (s *Service) HandleContentEvent(ctx context.Context, event events.Event) {
	topic := event.Topic

	parts := strings.Split(topic, ".")
	if len(parts) < 3 {
		return
	}
	action := parts[len(parts)-1]

	if event.Data == nil {
		return
	}

	contentID, _ := event.Data["id"].(string)
	contentType, _ := event.Data["content_type"].(string)

	if contentID == "" {
		return
	}

	switch action {
	case "created", "updated":
		if contentType == "" {
			return
		}
		content, err := s.contentSvc.GetByID(ctx, contentID)
		if err != nil {
			s.logger.Error("Failed to fetch content for indexing",
				"id", contentID, "error", err)
			return
		}
		if err := s.Index(ctx, contentType, content); err != nil {
			s.logger.Error("Failed to index content",
				"id", contentID, "error", err)
		}
	case "deleted":
		if err := s.Remove(ctx, contentID); err != nil {
			s.logger.Error("Failed to remove content from index",
				"id", contentID, "error", err)
		}
	}
}

// Close shuts down all open bleve indices.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ct, idx := range s.indices {
		if err := idx.Close(); err != nil {
			s.logger.Error("Failed to close search index",
				"content_type", ct, "error", err)
		}
	}
	s.indices = make(map[string]bleve.Index)
}

// mapToContent converts a raw database row map into an interfaces.Content.
func mapToContent(m map[string]interface{}) *interfaces.Content {
	c := &interfaces.Content{
		Data: make(map[string]interface{}),
	}

	if v, ok := m["id"].(string); ok {
		c.ID = v
	}
	if v, ok := m["content_type"].(string); ok {
		c.ContentType = v
	}
	if v, ok := m["title"].(string); ok {
		c.Title = v
	}
	if v, ok := m["slug"].(string); ok {
		c.Slug = v
	}
	if v, ok := m["author_id"].(string); ok {
		c.AuthorID = v
	}
	if v, ok := m["status"].(string); ok {
		c.Status = v
	}

	if v, ok := m["published_at"].(string); ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			c.PublishedAt = &t
		}
	}

	// Copy remaining fields into Data, excluding structural columns
	excluded := map[string]bool{
		"id": true, "content_type": true, "title": true, "slug": true,
		"author_id": true, "status": true, "published_at": true,
		"created_at": true, "updated_at": true, "deleted_at": true,
		"version": true, "created_by": true, "updated_by": true,
	}
	for k, v := range m {
		if !excluded[k] {
			c.Data[k] = v
		}
	}

	return c
}
