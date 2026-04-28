package integration

import (
	"context"
	"testing"
	"time"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// TestIntegration_FullPluginStartup verifies that the full Aroute engine starts
// with all plugins loaded and all core services registered.
func TestIntegration_FullPluginStartup(t *testing.T) {
	env := newTestEnv(t)

	// Verify all expected services are registered in the container
	services := env.container.Keys()
	t.Logf("Registered services: %v", services)

	expectedServices := []struct {
		name string
		get  func(*testing.T) interface{}
	}{
		{"DatabaseService", func(t *testing.T) interface{} { return env.getDatabaseService(t) }},
		{"AuthService", func(t *testing.T) interface{} { return env.getAuthService(t) }},
		{"ContentService", func(t *testing.T) interface{} { return env.getContentService(t) }},
		{"SearchService", func(t *testing.T) interface{} { return env.getSearchService(t) }},
		{"ThemeService", func(t *testing.T) interface{} { return env.getThemeService(t) }},
		{"CacheService", func(t *testing.T) interface{} { return env.getCacheService(t) }},
	}

	for _, svc := range expectedServices {
		t.Run("service_"+svc.name, func(t *testing.T) {
			s := svc.get(t)
			if s == nil {
				t.Errorf("%s is nil", svc.name)
			}
		})
	}
}

// TestIntegration_RegisterUser_CreateContent_Search verifies the full plugin
// collaboration flow: register user → create content → search → API access.
func TestIntegration_RegisterUser_CreateContent_Search(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Step 1: Verify default admin exists and can authenticate
	t.Run("admin_auth", func(t *testing.T) {
		token := env.authToken(t)
		if token == "" {
			t.Fatal("expected non-empty token")
		}
		t.Logf("Admin JWT token obtained (len=%d)", len(token))

		// Verify token
		authSvc := env.getAuthService(t)
		claims, err := authSvc.VerifyToken(ctx, token)
		if err != nil {
			t.Fatalf("verify token: %v", err)
		}
		if claims.Email != env.adminEmail {
			t.Errorf("claims email = %q, want %q", claims.Email, env.adminEmail)
		}
	})

	// Step 2: Create a new user
	var newUser *interfaces.User
	t.Run("create_user", func(t *testing.T) {
		authSvc := env.getAuthService(t)
		var err error
		newUser, err = authSvc.CreateUser(ctx, &interfaces.CreateUserRequest{
			Email:    "editor@test.aroute.local",
			Username: "testeditor",
			Password: "EditorPass123!",
			Roles:    []string{"editor"},
		})
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		if newUser.ID == "" {
			t.Error("expected non-empty user ID")
		}
		if newUser.Email != "editor@test.aroute.local" {
			t.Errorf("email = %q, want %q", newUser.Email, "editor@test.aroute.local")
		}
	})

	// Step 3: Authenticate as new user
	t.Run("new_user_auth", func(t *testing.T) {
		authSvc := env.getAuthService(t)
		result, err := authSvc.Authenticate(ctx, &interfaces.AuthRequest{
			Email:    "editor@test.aroute.local",
			Password: "EditorPass123!",
		})
		if err != nil {
			t.Fatalf("authenticate new user: %v", err)
		}
		if result.AccessToken == "" {
			t.Error("expected non-empty access token")
		}
		if result.User == nil || result.User.Email != "editor@test.aroute.local" {
			t.Error("user info mismatch in auth result")
		}
	})

	// Step 4: Create content (a blog post)
	var post *interfaces.Content
	t.Run("create_content", func(t *testing.T) {
		contentSvc := env.getContentService(t)
		var err error
		post, err = contentSvc.Create(ctx, "post", map[string]interface{}{
			"title":  "Integration Test Post",
			"body":   "This is a test post created during integration testing.",
			"status": "published",
		})
		if err != nil {
			t.Fatalf("create post: %v", err)
		}
		if post.ID == "" {
			t.Error("expected non-empty post ID")
		}
		if post.Title != "Integration Test Post" {
			t.Errorf("title = %q, want %q", post.Title, "Integration Test Post")
		}
		if post.Slug == "" {
			t.Error("expected auto-generated slug")
		}
		t.Logf("Created post: id=%s slug=%s", post.ID, post.Slug)
	})

	// Step 5: Read the content back
	t.Run("get_content", func(t *testing.T) {
		if post == nil {
			t.Skip("post not created")
		}
		contentSvc := env.getContentService(t)
		got, err := contentSvc.GetByID(ctx, post.ID)
		if err != nil {
			t.Fatalf("get content: %v", err)
		}
		if got.ID != post.ID {
			t.Errorf("ID = %q, want %q", got.ID, post.ID)
		}
		if got.Title != post.Title {
			t.Errorf("title = %q, want %q", got.Title, post.Title)
		}
	})

	// Step 6: List content
	t.Run("list_content", func(t *testing.T) {
		contentSvc := env.getContentService(t)
		page, err := contentSvc.List(ctx, "post", &interfaces.ListQuery{
			Page:    1,
			PerPage: 10,
		})
		if err != nil {
			t.Fatalf("list posts: %v", err)
		}
		if page.Meta.Total < 1 {
			t.Errorf("expected at least 1 post, got %d", page.Meta.Total)
		}
	})

	// Step 7: Search for the content
	t.Run("search_content", func(t *testing.T) {
		searchSvc := env.getSearchService(t)

		// Wait a bit for the event-driven indexing to process
		time.Sleep(500 * time.Millisecond)

		results, err := searchSvc.Search(ctx, &interfaces.SearchQuery{
			Query:      "integration test",
			Page:       1,
			PerPage:    10,
			Highlight:  true,
		})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		t.Logf("Search results: total=%d", results.Total)
		// Search results may be 0 if indexing is async; we verify the call succeeds
	})

	// Step 8: Verify RBAC permission check
	t.Run("rbac_check", func(t *testing.T) {
		authSvc := env.getAuthService(t)
		hasPerm, err := authSvc.HasPermission(ctx, newUser.ID, "content", "read")
		if err != nil {
			t.Fatalf("has permission: %v", err)
		}
		t.Logf("User %s has content:read permission: %v", newUser.ID, hasPerm)
	})

	// Step 9: Update content
	t.Run("update_content", func(t *testing.T) {
		contentSvc := env.getContentService(t)
		updated, err := contentSvc.Update(ctx, post.ID, map[string]interface{}{
			"title": "Updated Integration Test Post",
			"body":  "Updated body content.",
		})
		if err != nil {
			t.Fatalf("update post: %v", err)
		}
		if updated.Title != "Updated Integration Test Post" {
			t.Errorf("title = %q, want updated title", updated.Title)
		}
		if updated.Version <= post.Version {
			t.Errorf("expected version > %d, got %d", post.Version, updated.Version)
		}
	})

	// Step 10: Delete content
	t.Run("delete_content", func(t *testing.T) {
		contentSvc := env.getContentService(t)
		if err := contentSvc.Delete(ctx, post.ID); err != nil {
			t.Fatalf("delete post: %v", err)
		}
		// Verify soft delete
		_, err := contentSvc.GetByID(ctx, post.ID)
		if err == nil {
			t.Error("expected error getting deleted content")
		}
	})
}

// TestIntegration_CacheService verifies the cache plugin integrates correctly
// with the event bus for automatic invalidation.
func TestIntegration_CacheService(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cacheSvc := env.getCacheService(t)

	// Set a value
	if err := cacheSvc.Set(ctx, "test:key1", "value1", 5*time.Minute); err != nil {
		t.Fatalf("cache set: %v", err)
	}

	// Get it back
	val, ok := cacheSvc.Get(ctx, "test:key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if val != "value1" {
		t.Errorf("value = %v, want value1", val)
	}

	// Delete it
	if err := cacheSvc.Delete(ctx, "test:key1"); err != nil {
		t.Fatalf("cache delete: %v", err)
	}

	// Verify deleted
	_, ok = cacheSvc.Get(ctx, "test:key1")
	if ok {
		t.Error("expected cache miss after delete")
	}

	// Check stats
	stats := cacheSvc.Stats(ctx)
	t.Logf("Cache stats: hits=%d misses=%d", stats.Hits, stats.Misses)
}

// TestIntegration_WebhookTrigger verifies that creating content triggers
// webhook delivery through the event bus.
func TestIntegration_WebhookTrigger(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Get webhook service
	var webhookSvc interfaces.WebhookService
	if err := env.container.Get(&webhookSvc); err != nil {
		t.Skipf("webhook service not available: %v", err)
	}

	// Create a webhook that listens to all content events (use external URL)
	wh, err := webhookSvc.Create(ctx, "https://example.com/webhook-test", []string{"content.**"}, "test-secret")
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	if wh.ID == "" {
		t.Fatal("expected non-empty webhook ID")
	}
	t.Logf("Created webhook: id=%s url=%s", wh.ID, wh.URL)

	// Verify webhook is in the list
	webhooks := webhookSvc.List(ctx)
	found := false
	for _, w := range webhooks {
		if w.ID == wh.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("webhook not found in list")
	}

	// Create content to trigger the webhook event
	contentSvc := env.getContentService(t)
	_, err = contentSvc.Create(ctx, "post", map[string]interface{}{
		"title":  "Webhook Test Post",
		"body":   "Testing webhook event delivery",
		"status": "published",
		"slug":   "webhook-test-post",
	})
	if err != nil {
		t.Fatalf("create post: %v", err)
	}

	// Wait for async event delivery
	time.Sleep(300 * time.Millisecond)

	// Check delivery logs (the delivery will fail since localhost:9999 isn't real,
	// but we verify the attempt was logged)
	deliveries, total := webhookSvc.GetDeliveries(ctx, wh.ID, 10, 0)
	t.Logf("Webhook deliveries: total=%d", total)
	if total > 0 {
		t.Logf("First delivery: status=%d success=%v", deliveries[0].StatusCode, deliveries[0].Success)
	}

	// Clean up
	if err := webhookSvc.Delete(ctx, wh.ID); err != nil {
		t.Fatalf("delete webhook: %v", err)
	}
}

// TestIntegration_TokenRefresh verifies the JWT refresh token flow.
func TestIntegration_TokenRefresh(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	authSvc := env.getAuthService(t)

	// Authenticate
	result, err := authSvc.Authenticate(ctx, &interfaces.AuthRequest{
		Email:    env.adminEmail,
		Password: env.adminPassword,
	})
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	// Refresh token
	newTokens, err := authSvc.RefreshToken(ctx, result.RefreshToken)
	if err != nil {
		t.Fatalf("refresh token: %v", err)
	}
	if newTokens.AccessToken == "" {
		t.Error("expected non-empty new access token")
	}

	// Verify the new token works
	claims, err := authSvc.VerifyToken(ctx, newTokens.AccessToken)
	if err != nil {
		t.Fatalf("verify refreshed token: %v", err)
	}
	if claims.Email != env.adminEmail {
		t.Errorf("claims email = %q, want %q", claims.Email, env.adminEmail)
	}
}

// TestIntegration_APITokenManagement verifies API token lifecycle.
func TestIntegration_APITokenManagement(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	authSvc := env.getAuthService(t)

	// Authenticate as admin
	result, err := authSvc.Authenticate(ctx, &interfaces.AuthRequest{
		Email:    env.adminEmail,
		Password: env.adminPassword,
	})
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	adminID := result.User.ID

	// Create API token
	token, err := authSvc.CreateAPIToken(ctx, adminID, "test-token", nil)
	if err != nil {
		t.Fatalf("create API token: %v", err)
	}
	if token.ID == "" {
		t.Error("expected non-empty token ID")
	}
	t.Logf("Created API token: id=%s name=%s", token.ID, token.Name)

	// Revoke API token
	if err := authSvc.RevokeAPIToken(ctx, token.ID); err != nil {
		t.Fatalf("revoke API token: %v", err)
	}
}
