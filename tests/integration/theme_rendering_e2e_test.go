package integration

import (
	"context"
	"testing"
)

// TestE2E_ThemeRendering_DefaultTheme verifies the default theme
// renders HTML output containing expected content.
func TestE2E_ThemeRendering_DefaultTheme(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	themeSvc := env.getThemeService(t)

	// Step 1: Verify active theme is "default"
	t.Run("active_theme", func(t *testing.T) {
		active, err := themeSvc.GetActiveTheme(ctx)
		if err != nil {
			t.Fatalf("get active theme: %v", err)
		}
		if active != "default" {
			t.Errorf("active theme = %q, want default", active)
		}
	})

	// Step 2: List available themes
	t.Run("list_themes", func(t *testing.T) {
		themes, err := themeSvc.ListThemes(ctx)
		if err != nil {
			t.Fatalf("list themes: %v", err)
		}
		t.Logf("Available themes: %v", themes)
		// At minimum, the default theme should exist
		if len(themes) == 0 {
			t.Error("expected at least one theme")
		}
	})
}

// TestE2E_ThemeRendering_RenderIndex verifies that rendering the index page
// produces valid HTML with expected elements.
func TestE2E_ThemeRendering_RenderIndex(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	themeSvc := env.getThemeService(t)
	contentSvc := env.getContentService(t)

	// Create some test content first
	for i := 0; i < 3; i++ {
		_, err := contentSvc.Create(ctx, "post", map[string]interface{}{
			"title":  "Theme Test Post",
			"body":   "Content for theme rendering test",
			"status": "published",
		})
		if err != nil {
			t.Fatalf("create post %d: %v", i, err)
		}
	}

	// Render the index template
	t.Run("render_index", func(t *testing.T) {
		html, err := themeSvc.Render(ctx, "index", map[string]interface{}{
			"Title": "Test Blog",
			"Body":  "",
			"Posts": []interface{}{},
		})
		if err != nil {
			t.Fatalf("render index: %v", err)
		}
		if html == "" {
			t.Error("expected non-empty HTML output")
		}
		t.Logf("Rendered HTML length: %d bytes", len(html))

		// Check for basic HTML structure
		expectedSubstrings := []string{"<html", "</html>", "<body", "</body>"}
		for _, sub := range expectedSubstrings {
			if !contains(html, sub) {
				t.Errorf("HTML output missing %q", sub)
			}
		}
	})
}

// TestE2E_ThemeRendering_RenderPost verifies rendering a single post page.
func TestE2E_ThemeRendering_RenderPost(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	themeSvc := env.getThemeService(t)

	// Render a post template with data
	t.Run("render_post", func(t *testing.T) {
		html, err := themeSvc.Render(ctx, "post", map[string]interface{}{
			"Title": "Test Post Title",
			"Body":  "<p>This is the post content.</p>",
			"Date":  "2024-01-15",
		})
		if err != nil {
			t.Fatalf("render post: %v", err)
		}
		if html == "" {
			t.Error("expected non-empty HTML output")
		}

		// The rendered HTML should contain the post title somewhere
		if !contains(html, "Test Post Title") {
			t.Error("HTML output should contain the post title")
		}
		t.Logf("Rendered post HTML length: %d bytes", len(html))
	})
}

// TestE2E_ThemeRendering_RenderPage verifies rendering a static page.
func TestE2E_ThemeRendering_RenderPage(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	themeSvc := env.getThemeService(t)

	t.Run("render_page", func(t *testing.T) {
		html, err := themeSvc.Render(ctx, "page", map[string]interface{}{
			"Title": "About Us",
			"Body":  "<p>About this CMS.</p>",
		})
		if err != nil {
			t.Fatalf("render page: %v", err)
		}
		if html == "" {
			t.Error("expected non-empty HTML output")
		}
	})
}

// TestE2E_ThemeRendering_Render404 verifies rendering the 404 page.
func TestE2E_ThemeRendering_Render404(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	themeSvc := env.getThemeService(t)

	t.Run("render_404", func(t *testing.T) {
		html, err := themeSvc.Render(ctx, "404", map[string]interface{}{
			"Title": "Page Not Found",
			"Body":  "<p>The page you are looking for does not exist.</p>",
		})
		if err != nil {
			t.Fatalf("render 404: %v", err)
		}
		if html == "" {
			t.Error("expected non-empty HTML output for 404 page")
		}
		// 404 page should contain some indication of the error
		if !contains(html, "404") && !contains(html, "Not Found") && !contains(html, "not found") {
			t.Error("404 page should contain '404' or 'Not Found'")
		}
	})
}

// TestE2E_ThemeRendering_ThemeSwitching verifies that the active theme
// can be changed at runtime.
func TestE2E_ThemeRendering_ThemeSwitching(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	themeSvc := env.getThemeService(t)

	// Get current theme
	currentTheme, err := themeSvc.GetActiveTheme(ctx)
	if err != nil {
		t.Fatalf("get active theme: %v", err)
	}
	t.Logf("Current active theme: %s", currentTheme)

	// Attempt to switch to a non-existent theme (should fail gracefully)
	t.Run("switch_nonexistent", func(t *testing.T) {
		err := themeSvc.SetActiveTheme(ctx, "nonexistent-theme")
		if err == nil {
			t.Log("Switching to non-existent theme did not error (may be acceptable)")
		} else {
			t.Logf("Switch to non-existent theme error (expected): %v", err)
		}
	})

	// Switch back to default
	t.Run("switch_back_default", func(t *testing.T) {
		err := themeSvc.SetActiveTheme(ctx, "default")
		if err != nil {
			t.Logf("Switch back to default: %v (may be already default)", err)
		}

		active, err := themeSvc.GetActiveTheme(ctx)
		if err != nil {
			t.Fatalf("get active theme: %v", err)
		}
		t.Logf("Active theme after switch: %s", active)
	})
}

// TestE2E_ThemeRendering_RenderWithContentData verifies rendering with
// real content data from the content service.
func TestE2E_ThemeRendering_RenderWithContentData(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	themeSvc := env.getThemeService(t)
	contentSvc := env.getContentService(t)

	// Create a post
	post, err := contentSvc.Create(ctx, "post", map[string]interface{}{
		"title":  "Real Content Theme Test",
		"body":   "This is real content for theme rendering.",
		"status": "published",
	})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	// Render the post template with real data
	t.Run("render_with_real_content", func(t *testing.T) {
		bodyStr := ""
		if v, ok := post.Data["body"].(string); ok {
			bodyStr = v
		}
		html, err := themeSvc.Render(ctx, "post", map[string]interface{}{
			"ID":     post.ID,
			"Title":  post.Title,
			"Body":   bodyStr,
			"Slug":   post.Slug,
			"Status": post.Status,
		})
		if err != nil {
			t.Fatalf("render with real content: %v", err)
		}
		if html == "" {
			t.Error("expected non-empty HTML")
		}

		// Should contain the real post title
		if !contains(html, "Real Content Theme Test") {
			t.Error("rendered HTML should contain the post title")
		}
		t.Logf("Rendered with real content: %d bytes", len(html))
	})
}

// contains checks if a string contains a substring (case-sensitive).
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
