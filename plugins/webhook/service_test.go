package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return NewService(Config{
		DeliveryTimeout:          2 * time.Second,
		MaxRetries:               2,
		MaxConsecutiveFailures:   3,
		DeliveryLogRetentionDays: 30,
	}, logger)
}

func newTestServiceWithServer(t *testing.T, handler http.HandlerFunc) (*Service, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	svc := NewService(Config{
		DeliveryTimeout:          2 * time.Second,
		MaxRetries:               2,
		MaxConsecutiveFailures:   3,
		DeliveryLogRetentionDays: 30,
	}, logger)
	svc.httpClient = server.Client()
	svc.httpClient.Timeout = 2 * time.Second
	return svc, server
}

func verifyHMAC(body []byte, sigHeader string, secret string) bool {
	if !strings.HasPrefix(sigHeader, "sha256=") {
		return false
	}
	sig, err := hex.DecodeString(sigHeader[7:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(sig, expected)
}

// createTestWebhook inserts a webhook directly into the service's internal map,
// bypassing SSRF validation. Use this for tests that need httptest.NewServer
// URLs (which are loopback and would be rejected by validateWebhookURL).
func createTestWebhook(svc *Service, rawURL string, events []string, secret string) *interfaces.Webhook {
	now := time.Now()
	wh := &interfaces.Webhook{
		ID:        uuid.Must(uuid.NewV7()).String(),
		URL:       rawURL,
		Events:    events,
		Secret:    secret,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	svc.mu.Lock()
	svc.webhooks[wh.ID] = wh
	svc.mu.Unlock()
	return wh
}

// ─── CRUD Tests ──────────────────────────────────────────────────────────────

func TestCreate_Success(t *testing.T) {
	svc := newTestService(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "secret123")
	require.NoError(t, err)

	assert.NotEmpty(t, wh.ID)
	assert.Equal(t, "https://example.com/hook", wh.URL)
	assert.Equal(t, []string{"content.created"}, wh.Events)
	assert.Equal(t, "secret123", wh.Secret)
	assert.True(t, wh.Enabled)
	assert.False(t, wh.CreatedAt.IsZero())
	assert.False(t, wh.UpdatedAt.IsZero())
}

func TestCreate_InvalidURL(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Create(context.Background(), "Not-a-url", []string{"content.created"}, "test-secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid URL")
}

func TestCreate_EmptyEvents(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Create(context.Background(), "https://example.com/hook", []string{}, "test-secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "events must not be empty")
}

func TestGet_Success(t *testing.T) {
	svc := newTestService(t)

	created, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	got, err := svc.Get(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, created.URL, got.URL)
	assert.Equal(t, created.Events, got.Events)
}

func TestGet_NotFound(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Get(context.Background(), "nonexistent-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestList(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Create(context.Background(), "https://example.com/a", []string{"a"}, "test-s1x")
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), "https://example.com/b", []string{"b"}, "test-s2x")
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), "https://example.com/c", []string{"c"}, "test-s3x")
	require.NoError(t, err)

	list := svc.List(context.Background())
	assert.Len(t, list, 3)
}

func TestList_Empty(t *testing.T) {
	svc := newTestService(t)

	list := svc.List(context.Background())
	assert.Empty(t, list)
}

func TestUpdate_Success(t *testing.T) {
	svc := newTestService(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	updated, err := svc.Update(context.Background(), wh.ID, "https://example.com/new-hook", []string{"content.updated", "content.deleted"})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/new-hook", updated.URL)
	assert.Equal(t, []string{"content.updated", "content.deleted"}, updated.Events)
	assert.True(t, updated.UpdatedAt.After(wh.CreatedAt) || updated.UpdatedAt.Equal(wh.UpdatedAt))
}

func TestUpdate_InvalidURL(t *testing.T) {
	svc := newTestService(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	_, err = svc.Update(context.Background(), wh.ID, "bad-url", []string{"content.updated"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid URL")
}

func TestUpdate_NotFound(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Update(context.Background(), "nonexistent", "https://example.com/hook", []string{"content.updated"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDelete_Success(t *testing.T) {
	svc := newTestService(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	err = svc.Delete(context.Background(), wh.ID)
	require.NoError(t, err)

	list := svc.List(context.Background())
	assert.Empty(t, list)
}

func TestDelete_NotFound(t *testing.T) {
	svc := newTestService(t)

	err := svc.Delete(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDelete_RemovesDeliveries(t *testing.T) {
	var received atomic.Int32
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	ctx := context.Background()
	svc.HandleEvent(ctx, events.Event{Topic: "content.created", Data: map[string]any{"test": true}})
	assert.Equal(t, int32(1), received.Load())

	deliveries, total := svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Equal(t, 1, total)
	assert.Len(t, deliveries, 1)

	err := svc.Delete(context.Background(), wh.ID)
	require.NoError(t, err)

	deliveries, total = svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Equal(t, 0, total)
	assert.Empty(t, deliveries)
}

func TestSetEnabled(t *testing.T) {
	svc := newTestService(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)
	assert.True(t, wh.Enabled)

	// Disable
	err = svc.SetEnabled(context.Background(), wh.ID, false)
	require.NoError(t, err)
	got, _ := svc.Get(context.Background(), wh.ID)
	assert.False(t, got.Enabled)

	// Enable — resets ConsecutiveFailures and DisabledReason
	got.ConsecutiveFailures = 5
	got.DisabledReason = "consecutive_failures"
	err = svc.SetEnabled(context.Background(), wh.ID, true)
	require.NoError(t, err)
	got, _ = svc.Get(context.Background(), wh.ID)
	assert.True(t, got.Enabled)
	assert.Equal(t, 0, got.ConsecutiveFailures)
	assert.Equal(t, "", got.DisabledReason)
}

func TestUpdateSecret(t *testing.T) {
	svc := newTestService(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "old-secret")
	require.NoError(t, err)
	assert.Equal(t, "old-secret", wh.Secret)

	err = svc.UpdateSecret(context.Background(), wh.ID, "new-secret")
	require.NoError(t, err)
	got, _ := svc.Get(context.Background(), wh.ID)
	assert.Equal(t, "new-secret", got.Secret)
}

// ─── Event Matching Tests ───────────────────────────────────────────────────

func TestMatchEventPattern_Exact(t *testing.T) {
	assert.True(t, matchEventPattern("content.created", "content.created"))
}

func TestMatchEventPattern_SingleWildcard(t *testing.T) {
	assert.True(t, matchEventPattern("content.created", "content.*"))
	assert.True(t, matchEventPattern("content.updated", "content.*"))
	assert.False(t, matchEventPattern("content.nested.deep", "content.*"))
}

func TestMatchEventPattern_MultiWildcard(t *testing.T) {
	assert.True(t, matchEventPattern("content.created", "**"))
	assert.True(t, matchEventPattern("user.login", "**"))
	assert.True(t, matchEventPattern("a.b.c.d", "**"))
}

func TestMatchEventPattern_MixedWildcard(t *testing.T) {
	assert.True(t, matchEventPattern("content.created", "content.**"))
	assert.True(t, matchEventPattern("content.nested.deep", "content.**"))
}

func TestMatchEventPattern_NoMatch(t *testing.T) {
	assert.False(t, matchEventPattern("content.created", "user.login"))
}

// ─── HMAC Signing Tests ────────────────────────────────────────────────────

func TestHMACSignature(t *testing.T) {
	var receivedBody []byte
	var sigHeader string

	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		sigHeader = r.Header.Get("X-Webhook-Signature")
		w.WriteHeader(200)
	})

	createTestWebhook(svc, server.URL, []string{"content.created"}, "my-secret-key")

	svc.HandleEvent(context.Background(), events.Event{
		Topic: "content.created",
		Data:  map[string]any{"title": "test"},
	})

	assert.True(t, verifyHMAC(receivedBody, sigHeader, "my-secret-key"),
		"HMAC signature should be valid for the webhook secret")
}

func TestHMACSignature_MismatchDetection(t *testing.T) {
	var receivedBody []byte
	var sigHeader string

	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		sigHeader = r.Header.Get("X-Webhook-Signature")
		w.WriteHeader(200)
	})

	createTestWebhook(svc, server.URL, []string{"content.created"}, "correct-secret")

	svc.HandleEvent(context.Background(), events.Event{
		Topic: "content.created",
		Data:  map[string]any{"test": true},
	})

	assert.False(t, verifyHMAC(receivedBody, sigHeader, "wrong-secret"),
		"HMAC signature should NOT match with wrong secret")
	assert.True(t, verifyHMAC(receivedBody, sigHeader, "correct-secret"),
		"HMAC signature should match with correct secret")
}

// ─── HTTP Delivery Tests ───────────────────────────────────────────────────

func TestHandleEvent_SingleWebhook(t *testing.T) {
	var received atomic.Int32
	var bodyMu struct {
		body []byte
	}
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodyMu.body = b
		received.Add(1)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.True(t, strings.HasPrefix(r.Header.Get("X-Webhook-Signature"), "sha256="))
		w.WriteHeader(200)
	})

	createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	svc.HandleEvent(context.Background(), events.Event{
		Topic: "content.created",
		Data:  map[string]any{"title": "hello"},
	})

	assert.Equal(t, int32(1), received.Load())

	var payload map[string]any
	err := json.Unmarshal(bodyMu.body, &payload)
	require.NoError(t, err)
	assert.Equal(t, "content.created", payload["event"])
	assert.Contains(t, payload, "timestamp")
	assert.Contains(t, payload, "data")
}

func TestHandleEvent_MultipleWebhooks(t *testing.T) {
	var received atomic.Int32

	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	})

	createTestWebhook(svc, server.URL, []string{"content.created"}, "test-s1")
	createTestWebhook(svc, server.URL, []string{"content.created"}, "test-s2")
	createTestWebhook(svc, server.URL, []string{"content.created"}, "test-s3")

	svc.HandleEvent(context.Background(), events.Event{
		Topic: "content.created",
		Data:  nil,
	})

	assert.Equal(t, int32(3), received.Load())
}

func TestHandleEvent_DisabledWebhook(t *testing.T) {
	var received atomic.Int32

	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")
	err := svc.SetEnabled(context.Background(), wh.ID, false)
	require.NoError(t, err)

	svc.HandleEvent(context.Background(), events.Event{
		Topic: "content.created",
		Data:  nil,
	})

	assert.Equal(t, int32(0), received.Load(), "disabled webhook should receive no POST")
}

func TestHandleEvent_NoMatchingWebhooks(t *testing.T) {
	var received atomic.Int32

	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	})

	createTestWebhook(svc, server.URL, []string{"user.login"}, "test-secret")

	svc.HandleEvent(context.Background(), events.Event{
		Topic: "content.created",
		Data:  nil,
	})

	assert.Equal(t, int32(0), received.Load(), "no POSTs when no webhooks match the event")
}

func TestDelivery_Success_2xx(t *testing.T) {
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	svc.HandleEvent(context.Background(), events.Event{
		Topic: "content.created",
		Data:  map[string]any{"test": true},
	})

	got, _ := svc.Get(context.Background(), wh.ID)
	assert.Equal(t, 0, got.ConsecutiveFailures)

	deliveries, total := svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Equal(t, 1, total)
	require.Len(t, deliveries, 1)
	assert.True(t, deliveries[0].Success)
	assert.Equal(t, 200, deliveries[0].StatusCode)
}

func TestDelivery_Success_201(t *testing.T) {
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	svc.HandleEvent(context.Background(), events.Event{
		Topic: "content.created",
		Data:  nil,
	})

	deliveries, total := svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Equal(t, 1, total)
	require.Len(t, deliveries, 1)
	assert.True(t, deliveries[0].Success)
	assert.Equal(t, 201, deliveries[0].StatusCode)
}

func TestDelivery_ClientError_4xx(t *testing.T) {
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	svc.HandleEvent(context.Background(), events.Event{
		Topic: "content.created",
		Data:  nil,
	})

	deliveries, total := svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Equal(t, 1, total, "4xx should be a permanent failure with no retry")
	require.Len(t, deliveries, 1)
	assert.False(t, deliveries[0].Success)
	assert.Equal(t, 404, deliveries[0].StatusCode)
	assert.Equal(t, "HTTP 404", deliveries[0].Error)
}

func TestDelivery_ServerError_5xx(t *testing.T) {
	var requests atomic.Int32
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(500)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	svc.HandleEvent(context.Background(), events.Event{
		Topic: "content.created",
		Data:  nil,
	})

	// MaxRetries=2, so 1 initial + 2 retries = 3 attempts
	assert.Equal(t, int32(3), requests.Load(), "5xx should trigger retries")

	deliveries, total := svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Equal(t, 3, total)
	for _, d := range deliveries {
		assert.False(t, d.Success)
		assert.Equal(t, 500, d.StatusCode)
	}

	got, _ := svc.Get(context.Background(), wh.ID)
	assert.Equal(t, 1, got.ConsecutiveFailures)
}

func TestDelivery_RateLimited_429(t *testing.T) {
	var requests atomic.Int32
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(429)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	svc.HandleEvent(context.Background(), events.Event{
		Topic: "content.created",
		Data:  nil,
	})

	// 429 is retryable: 1 initial + 2 retries = 3 attempts
	assert.Equal(t, int32(3), requests.Load(), "429 should trigger retries")

	deliveries, total := svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Equal(t, 3, total)
	for _, d := range deliveries {
		assert.False(t, d.Success)
		assert.Equal(t, 429, d.StatusCode)
	}
}

// ─── Retry Tests ───────────────────────────────────────────────────────────

func TestRetry_ExponentialBackoff(t *testing.T) {
	var requests atomic.Int32
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		if n < 3 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	svc.HandleEvent(context.Background(), events.Event{
		Topic: "content.created",
		Data:  nil,
	})

	assert.Equal(t, int32(3), requests.Load(), "should succeed on 3rd attempt")

	deliveries, total := svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Equal(t, 3, total, "should have 3 delivery logs")

	last := deliveries[0] // reverse chronological — newest first
	assert.True(t, last.Success)
	assert.Equal(t, 200, last.StatusCode)

	got, _ := svc.Get(context.Background(), wh.ID)
	assert.Equal(t, 0, got.ConsecutiveFailures, "success should reset failures")
}

func TestRetry_MaxRetriesExhausted(t *testing.T) {
	var requests atomic.Int32
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(500)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	svc.HandleEvent(context.Background(), events.Event{
		Topic: "content.created",
		Data:  nil,
	})

	// MaxRetries=2 → 3 total attempts
	assert.Equal(t, int32(3), requests.Load())

	deliveries, total := svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Equal(t, 3, total)
	for _, d := range deliveries {
		assert.False(t, d.Success)
	}

	got, _ := svc.Get(context.Background(), wh.ID)
	assert.Equal(t, 1, got.ConsecutiveFailures, "failed delivery should increment ConsecutiveFailures")
}

// ─── Auto-Disable Tests ────────────────────────────────────────────────────

func TestAutoDisable_AfterConsecutiveFailures(t *testing.T) {
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	ctx := context.Background()
	// MaxConsecutiveFailures=3, MaxRetries=2 → each HandleEvent = 3 attempts, 1 failure count
	for i := 0; i < 3; i++ {
		svc.HandleEvent(ctx, events.Event{Topic: "content.created", Data: nil})
	}

	got, _ := svc.Get(context.Background(), wh.ID)
	assert.False(t, got.Enabled, "webhook should be auto-disabled after 3 consecutive failures")
	assert.Equal(t, "consecutive_failures", got.DisabledReason)
}

func TestAutoDisable_SuccessResetsCounter(t *testing.T) {
	callCount := 0
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 6 {
			// First 2 HandleEvent calls × 3 attempts each = 6 requests all fail
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	ctx := context.Background()

	// 2 failed deliveries → ConsecutiveFailures = 2
	svc.HandleEvent(ctx, events.Event{Topic: "content.created", Data: nil})
	got, _ := svc.Get(context.Background(), wh.ID)
	assert.Equal(t, 1, got.ConsecutiveFailures)

	svc.HandleEvent(ctx, events.Event{Topic: "content.created", Data: nil})
	got, _ = svc.Get(context.Background(), wh.ID)
	assert.Equal(t, 2, got.ConsecutiveFailures)

	// Success resets counter
	svc.HandleEvent(ctx, events.Event{Topic: "content.created", Data: nil})
	got, _ = svc.Get(context.Background(), wh.ID)
	assert.Equal(t, 0, got.ConsecutiveFailures)
	assert.True(t, got.Enabled, "webhook should remain enabled after success resets counter")
}

func TestAutoDisable_ReEnableResetsCounter(t *testing.T) {
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	ctx := context.Background()
	// Trigger 3 consecutive failures to auto-disable
	for i := 0; i < 3; i++ {
		svc.HandleEvent(ctx, events.Event{Topic: "content.created", Data: nil})
	}

	got, _ := svc.Get(context.Background(), wh.ID)
	assert.False(t, got.Enabled)
	assert.Equal(t, "consecutive_failures", got.DisabledReason)

	// Re-enable
	err := svc.SetEnabled(context.Background(), wh.ID, true)
	require.NoError(t, err)
	got, _ = svc.Get(context.Background(), wh.ID)
	assert.True(t, got.Enabled)
	assert.Equal(t, 0, got.ConsecutiveFailures)
	assert.Equal(t, "", got.DisabledReason)
}

// ─── Delivery Logging Tests ────────────────────────────────────────────────

func TestGetDeliveries_Pagination(t *testing.T) {
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		svc.HandleEvent(ctx, events.Event{
			Topic: "content.created",
			Data:  map[string]any{"seq": i},
		})
	}

	// Page 1: limit=2, offset=0 → newest 2
	page1, total := svc.GetDeliveries(context.Background(), wh.ID, 2, 0)
	assert.Equal(t, 5, total)
	assert.Len(t, page1, 2)

	// Page 2: limit=2, offset=2
	page2, total := svc.GetDeliveries(context.Background(), wh.ID, 2, 2)
	assert.Equal(t, 5, total)
	assert.Len(t, page2, 2)

	// Page 3: limit=2, offset=4 → only 1 remaining
	page3, total := svc.GetDeliveries(context.Background(), wh.ID, 2, 4)
	assert.Equal(t, 5, total)
	assert.Len(t, page3, 1)

	// Beyond range
	page4, total := svc.GetDeliveries(context.Background(), wh.ID, 2, 10)
	assert.Equal(t, 5, total)
	assert.Empty(t, page4)
}

func TestGetDeliveries_ReverseChronological(t *testing.T) {
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		svc.HandleEvent(ctx, events.Event{
			Topic: "content.created",
			Data:  map[string]any{"seq": i},
		})
	}

	deliveries, total := svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Equal(t, 3, total)
	require.Len(t, deliveries, 3)

	// Newest first
	assert.True(t, deliveries[0].CreatedAt.After(deliveries[1].CreatedAt) || deliveries[0].CreatedAt.Equal(deliveries[1].CreatedAt))
	assert.True(t, deliveries[1].CreatedAt.After(deliveries[2].CreatedAt) || deliveries[1].CreatedAt.Equal(deliveries[2].CreatedAt))
}

func TestGetDeliveries_Empty(t *testing.T) {
	svc := newTestService(t)

	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	deliveries, total := svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Empty(t, deliveries)
	assert.Equal(t, 0, total)
}

// ─── PruneOldDeliveries Tests ─────────────────────────────────────────────

func TestPruneOldDeliveries_RemovesOld(t *testing.T) {
	svc := newTestService(t)
	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	now := time.Now()
	oldCutoff := now.Add(-31 * 24 * time.Hour)
	recentCutoff := now.Add(-10 * 24 * time.Hour)

	svc.mu.Lock()
	svc.deliveries[wh.ID] = []*interfaces.WebhookDelivery{
		{ID: "old-1", WebhookID: wh.ID, CreatedAt: oldCutoff, Success: true},
		{ID: "old-2", WebhookID: wh.ID, CreatedAt: oldCutoff.Add(-1 * time.Hour), Success: false},
		{ID: "recent-1", WebhookID: wh.ID, CreatedAt: recentCutoff, Success: true},
		{ID: "recent-2", WebhookID: wh.ID, CreatedAt: now, Success: true},
	}
	svc.mu.Unlock()

	svc.PruneOldDeliveries(context.Background())

	_, total := svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Equal(t, 2, total, "entries older than 30 days should be pruned")
}

func TestPruneOldDeliveries_NothingToRemove(t *testing.T) {
	svc := newTestService(t)
	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	svc.mu.Lock()
	svc.deliveries[wh.ID] = []*interfaces.WebhookDelivery{
		{ID: "r1", WebhookID: wh.ID, CreatedAt: time.Now(), Success: true},
		{ID: "r2", WebhookID: wh.ID, CreatedAt: time.Now().Add(-1 * time.Hour), Success: true},
	}
	svc.mu.Unlock()

	svc.PruneOldDeliveries(context.Background())

	_, total := svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Equal(t, 2, total, "recent entries should not be pruned")
}

func TestPruneOldDeliveries_EmptyService(t *testing.T) {
	svc := newTestService(t)
	assert.NotPanics(t, func() { svc.PruneOldDeliveries(context.Background()) })
}

func TestPruneOldDeliveries_AllOld(t *testing.T) {
	svc := newTestService(t)
	wh, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "test-secret")
	require.NoError(t, err)

	svc.mu.Lock()
	svc.deliveries[wh.ID] = []*interfaces.WebhookDelivery{
		{ID: "o1", WebhookID: wh.ID, CreatedAt: time.Now().Add(-100 * 24 * time.Hour), Success: false},
	}
	svc.mu.Unlock()

	svc.PruneOldDeliveries(context.Background())

	_, total := svc.GetDeliveries(context.Background(), wh.ID, 10, 0)
	assert.Equal(t, 0, total, "all old entries should be pruned")
}

// ─── Auto-Disabled Event Tests ──────────────────────────────────────────

func TestAutoDisabled_EmitsEvent(t *testing.T) {
	bus := events.NewEventBus()

	var receivedMu struct {
		val events.Event
		ok  bool
	}
	bus.SubscribeBroadcast("webhook.auto_disabled", func(ctx context.Context, event events.Event) {
		receivedMu.val = event
		receivedMu.ok = true
	})

	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})
	svc.SetEventBus(bus)

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		svc.HandleEvent(ctx, events.Event{Topic: "content.created", Data: nil})
	}

	assert.Eventually(t, func() bool {
		return receivedMu.ok
	}, 5*time.Second, 50*time.Millisecond, "auto_disabled event should be emitted")

	assert.Equal(t, "webhook.auto_disabled", receivedMu.val.Topic)
	assert.Equal(t, wh.ID, receivedMu.val.Data["webhook_id"])
	assert.Equal(t, wh.URL, receivedMu.val.Data["url"])
	assert.Equal(t, "consecutive_failures", receivedMu.val.Data["reason"])
	assert.Equal(t, float64(3), receivedMu.val.Data["consecutive_failures"])
}

func TestAutoDisabled_NoEventWithoutBus(t *testing.T) {
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		svc.HandleEvent(ctx, events.Event{Topic: "content.created", Data: nil})
	}

	got, _ := svc.Get(context.Background(), wh.ID)
	assert.False(t, got.Enabled, "should still auto-disable without EventBus")
	assert.Equal(t, "consecutive_failures", got.DisabledReason)
}

func TestAutoDisabled_NoEventWhenNotDisabled(t *testing.T) {
	bus := events.NewEventBus()

	var eventReceived atomic.Int32
	bus.SubscribeBroadcast("webhook.auto_disabled", func(ctx context.Context, event events.Event) {
		eventReceived.Add(1)
	})

	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	svc.SetEventBus(bus)

	createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	svc.HandleEvent(context.Background(), events.Event{Topic: "content.created", Data: nil})

	assert.Equal(t, int32(0), eventReceived.Load(), "no auto_disabled event on success")
}

// ─── TestDelivery Tests ────────────────────────────────────────────────────

func TestTestDelivery_Success(t *testing.T) {
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.True(t, strings.HasPrefix(r.Header.Get("X-Webhook-Signature"), "sha256="))

		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		json.Unmarshal(body, &payload)
		assert.Equal(t, "webhook.test", payload["event"])

		w.WriteHeader(200)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	d, err := svc.TestDelivery(context.Background(), wh.ID)
	require.NoError(t, err)
	assert.True(t, d.Success)
	assert.Equal(t, 200, d.StatusCode)
	assert.Equal(t, "webhook.test", d.Event)
	assert.Equal(t, 1, d.Attempt)
}

func TestTestDelivery_DisabledWebhook(t *testing.T) {
	var received atomic.Int32
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	err := svc.SetEnabled(context.Background(), wh.ID, false)
	require.NoError(t, err)

	d, err := svc.TestDelivery(context.Background(), wh.ID)
	require.NoError(t, err)
	assert.True(t, d.Success)
	assert.Equal(t, int32(1), received.Load(), "test delivery should work even for disabled webhooks")
}

func TestTestDelivery_NotFound(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.TestDelivery(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// ─── DeliverEvent Tests ─────────────────────────────────────────────────────

func TestDeliverEvent_Basic(t *testing.T) {
	var received atomic.Int32
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	})

	createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	svc.DeliverEvent(context.Background(), interfaces.WebhookEvent{
		Topic:     "content.created",
		Timestamp: time.Now(),
		Data:      map[string]any{"test": true},
	})

	assert.Equal(t, int32(1), received.Load())
}

func TestDeliverEvent_ZeroTimestamp(t *testing.T) {
	var received atomic.Int32
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	})

	createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")

	svc.DeliverEvent(context.Background(), interfaces.WebhookEvent{
		Topic: "content.created",
		// Timestamp is zero — should fallback to time.Now()
		Data: map[string]any{"test": true},
	})

	assert.Equal(t, int32(1), received.Load())
}

func TestDeliverEvent_DisabledWebhook(t *testing.T) {
	var received atomic.Int32
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	})

	wh := createTestWebhook(svc, server.URL, []string{"content.created"}, "test-secret")
	svc.SetEnabled(context.Background(), wh.ID, false)

	svc.DeliverEvent(context.Background(), interfaces.WebhookEvent{
		Topic:     "content.created",
		Timestamp: time.Now(),
		Data:      nil,
	})

	assert.Equal(t, int32(0), received.Load(), "disabled webhook should receive no POST")
}

func TestDeliverEvent_NoMatchingWebhooks(t *testing.T) {
	var received atomic.Int32
	svc, server := newTestServiceWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(200)
	})

	createTestWebhook(svc, server.URL, []string{"user.login"}, "test-secret")

	svc.DeliverEvent(context.Background(), interfaces.WebhookEvent{
		Topic:     "content.created",
		Timestamp: time.Now(),
		Data:      nil,
	})

	assert.Equal(t, int32(0), received.Load(), "no POSTs when no webhooks match")
}

// ─── validateWebhookURL / checkIP Edge Case Tests ────────────────────────────

func TestValidateWebhookURL_UnsupportedScheme(t *testing.T) {
	err := validateWebhookURL("ftp://example.com/hook")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported URL scheme")
}

func TestValidateWebhookURL_NoHostname(t *testing.T) {
	err := validateWebhookURL("http:///path")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hostname")
}

func TestValidateWebhookURL_LoopbackIP(t *testing.T) {
	err := validateWebhookURL("http://127.0.0.1/hook")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "loopback")
}

func TestValidateWebhookURL_PrivateIP(t *testing.T) {
	err := validateWebhookURL("http://192.168.1.1/hook")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private")
}

func TestValidateWebhookURL_LinkLocalIP(t *testing.T) {
	err := validateWebhookURL("http://169.254.1.1/hook")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "link-local")
}

func TestValidateWebhookURL_UnspecifiedIP(t *testing.T) {
	err := validateWebhookURL("http://0.0.0.0/hook")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unspecified")
}

func TestValidateWebhookURL_ValidPublicIP(t *testing.T) {
	err := validateWebhookURL("http://8.8.8.8/hook")
	assert.NoError(t, err)
}

func TestValidateWebhookURL_InvalidURL(t *testing.T) {
	err := validateWebhookURL("not a url at all")
	assert.Error(t, err)
}

// ─── SetEnabled / UpdateSecret Not Found ─────────────────────────────────────

func TestSetEnabled_NotFound(t *testing.T) {
	svc := newTestService(t)

	err := svc.SetEnabled(context.Background(), "nonexistent", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdateSecret_NotFound(t *testing.T) {
	svc := newTestService(t)

	err := svc.UpdateSecret(context.Background(), "nonexistent", "new-secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// ─── Create Short Secret ─────────────────────────────────────────────────────

func TestCreate_ShortSecret(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Create(context.Background(), "https://example.com/hook", []string{"content.created"}, "short")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret must be at least 8 characters")
}
