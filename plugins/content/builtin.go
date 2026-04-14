package content

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// NewPageContentType returns the built-in Page content type definition.
func NewPageContentType() *interfaces.ContentType {
	return &interfaces.ContentType{
		ID:          uuid.New().String(),
		Name:        "page",
		Slug:        "pages",
		DisplayName: "Page",
		Description: "Static page content type",
		TableName:   "content_pages",
		Fields: []interfaces.Field{
			{Name: "title", DisplayName: "Title", Type: "text", Required: true},
			{Name: "slug", DisplayName: "Slug", Type: "slug", Required: true, Unique: true},
			{Name: "body", DisplayName: "Body", Type: "richtext"},
			{Name: "status", DisplayName: "Status", Type: "enum", ValidationRules: map[string]interface{}{"enum": []string{"draft", "published"}}},
			{Name: "published_at", DisplayName: "Published At", Type: "datetime"},
			{Name: "seo_title", DisplayName: "SEO Title", Type: "text"},
			{Name: "seo_description", DisplayName: "SEO Description", Type: "text"},
		},
	}
}

// NewPostContentType returns the built-in Post content type definition.
func NewPostContentType() *interfaces.ContentType {
	return &interfaces.ContentType{
		ID:          uuid.New().String(),
		Name:        "post",
		Slug:        "posts",
		DisplayName: "Post",
		Description: "Blog post content type",
		TableName:   "content_posts",
		Fields: []interfaces.Field{
			{Name: "title", DisplayName: "Title", Type: "text", Required: true},
			{Name: "slug", DisplayName: "Slug", Type: "slug", Required: true, Unique: true},
			{Name: "body", DisplayName: "Body", Type: "richtext"},
			{Name: "excerpt", DisplayName: "Excerpt", Type: "text"},
			{Name: "status", DisplayName: "Status", Type: "enum", ValidationRules: map[string]interface{}{"enum": []string{"draft", "published"}}},
			{Name: "published_at", DisplayName: "Published At", Type: "datetime"},
			{Name: "featured_image", DisplayName: "Featured Image", Type: "media"},
			{Name: "seo_title", DisplayName: "SEO Title", Type: "text"},
			{Name: "seo_description", DisplayName: "SEO Description", Type: "text"},
		},
	}
}

// NewCategoryContentType returns the built-in Category content type definition.
func NewCategoryContentType() *interfaces.ContentType {
	return &interfaces.ContentType{
		ID:          uuid.New().String(),
		Name:        "category",
		Slug:        "categories",
		DisplayName: "Category",
		Description: "Content category",
		TableName:   "content_categories",
		Fields: []interfaces.Field{
			{Name: "name", DisplayName: "Name", Type: "text", Required: true},
			{Name: "slug", DisplayName: "Slug", Type: "slug", Required: true, Unique: true},
			{Name: "description", DisplayName: "Description", Type: "text"},
			{Name: "parent", DisplayName: "Parent Category", Type: "relation",
				RelationConfig: &interfaces.RelationConfig{
					TargetContentType: "category",
					RelationType:      "one-to-many",
				}},
		},
	}
}

// NewTagContentType returns the built-in Tag content type definition.
func NewTagContentType() *interfaces.ContentType {
	return &interfaces.ContentType{
		ID:          uuid.New().String(),
		Name:        "tag",
		Slug:        "tags",
		DisplayName: "Tag",
		Description: "Content tag",
		TableName:   "content_tags",
		Fields: []interfaces.Field{
			{Name: "name", DisplayName: "Name", Type: "text", Required: true},
			{Name: "slug", DisplayName: "Slug", Type: "slug", Required: true, Unique: true},
		},
	}
}

// InitializeBuiltInContentTypes creates built-in content types if they don't already exist.
func (s *Service) InitializeBuiltInContentTypes(ctx context.Context) error {
	builtins := []func() *interfaces.ContentType{
		NewPageContentType,
		NewPostContentType,
		NewCategoryContentType,
		NewTagContentType,
	}

	for _, fn := range builtins {
		ct := fn()
		exists, err := s.store.ContentTypeExists(ctx, ct.Name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}

		now := time.Now().UTC().Format(time.RFC3339)
		ct.CreatedAt, _ = time.Parse(time.RFC3339, now)
		ct.UpdatedAt, _ = time.Parse(time.RFC3339, now)

		if err := s.store.CreateContentType(ctx, ct); err != nil {
			return err
		}

		// Create the actual database table for this content type.
		if err := s.createContentTable(ctx, ct); err != nil {
			return err
		}

		s.logger.Info("Built-in content type created", "name", ct.Name, "table", ct.TableName)
	}

	return nil
}
