package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// ---------------------------------------------------------------------------
// mock ConfigProvider
// ---------------------------------------------------------------------------

type mockConfigProvider struct {
	data map[string]interface{}
}

func newMockConfigProvider(data map[string]interface{}) *mockConfigProvider {
	if data == nil {
		data = make(map[string]interface{})
	}
	return &mockConfigProvider{data: data}
}

func (m *mockConfigProvider) GetString(key string) string {
	if v, ok := m.data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (m *mockConfigProvider) GetInt(key string) int {
	if v, ok := m.data[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		}
	}
	return 0
}

func (m *mockConfigProvider) GetBool(key string) bool {
	if v, ok := m.data[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func (m *mockConfigProvider) GetStringSlice(key string) []string {
	return nil
}

func (m *mockConfigProvider) Get(key string) interface{} {
	return m.data[key]
}

func (m *mockConfigProvider) Unmarshal(key string, target interface{}) error {
	return nil
}

// Compile-time check.
var _ core.ConfigProvider = (*mockConfigProvider)(nil)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// makeRequestWithClaims creates a request with user claims in context.
func makeRequestWithClaims(t *testing.T, claims *interfaces.UserClaims) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	if claims != nil {
		ctx := context.WithValue(req.Context(), claimsContextKey, claims)
		req = req.WithContext(ctx)
	}
	return req
}

// ---------------------------------------------------------------------------
// TestRequestsUnderLimitPassThrough
// ---------------------------------------------------------------------------

func TestRateLimit_RequestsUnderLimitPassThrough(t *testing.T) {
	cfg := newMockConfigProvider(map[string]interface{}{
		"api.rate_limit.requests_per_minute": 5,
	})
	mw := rateLimitMiddleware(cfg)

	nextCalled := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled++
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(next)

	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "request %d should succeed", i+1)
	}
	assert.Equal(t, 5, nextCalled)
}

// ---------------------------------------------------------------------------
// TestRequestsOverLimitGet429
// ---------------------------------------------------------------------------

func TestRateLimit_RequestsOverLimitGet429(t *testing.T) {
	cfg := newMockConfigProvider(map[string]interface{}{
		"api.rate_limit.requests_per_minute": 3,
	})
	mw := rateLimitMiddleware(cfg)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(next)

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:5678"
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "request %d should succeed", i+1)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:5678"
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)

	body := firstAPIError(t, rec.Body.Bytes())
	assert.Equal(t, "RATE_LIMITED", body.Code)
	assert.Equal(t, "rate limit exceeded", body.Message)
}

// ---------------------------------------------------------------------------
// TestRateLimitHeadersOnSuccess
// ---------------------------------------------------------------------------

func TestRateLimit_HeadersPresentOnSuccessfulRequests(t *testing.T) {
	cfg := newMockConfigProvider(map[string]interface{}{
		"api.rate_limit.requests_per_minute": 10,
	})
	mw := rateLimitMiddleware(cfg)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "10", rec.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "9", rec.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, rec.Header().Get("X-RateLimit-Reset"))
}

// ---------------------------------------------------------------------------
// Test429ResponseFormat
// ---------------------------------------------------------------------------

func TestRateLimit_429ResponseFormatMatchesErrorEnvelope(t *testing.T) {
	cfg := newMockConfigProvider(map[string]interface{}{
		"api.rate_limit.requests_per_minute": 1,
	})
	mw := rateLimitMiddleware(cfg)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(next)

	// First request consumes the quota.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Second request should be rejected.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

	envelope := decodeErrorsEnvelope(t, rec.Body.Bytes())
	require.Len(t, envelope.Errors, 1)

	apiErr := envelope.Errors[0]
	assert.Equal(t, "RATE_LIMITED", apiErr.Code)
	assert.Equal(t, "rate limit exceeded", apiErr.Message)
	assert.Equal(t, map[string]interface{}{}, apiErr.Details)
}

// ---------------------------------------------------------------------------
// TestRetryAfterHeaderOn429
// ---------------------------------------------------------------------------

func TestRateLimit_RetryAfterHeaderPresentOn429(t *testing.T) {
	cfg := newMockConfigProvider(map[string]interface{}{
		"api.rate_limit.requests_per_minute": 1,
	})
	mw := rateLimitMiddleware(cfg)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(next)

	// Exhaust the quota.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Trigger the 429.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	retryAfter := rec.Header().Get("Retry-After")
	assert.NotEmpty(t, retryAfter, "Retry-After header must be present on 429")
	retrySec, err := strconv.Atoi(retryAfter)
	assert.NoError(t, err, "Retry-After should be a number")
	assert.GreaterOrEqual(t, retrySec, 1)
}

// ---------------------------------------------------------------------------
// TestDefaultLimitIs120
// ---------------------------------------------------------------------------

func TestRateLimit_DefaultLimitIs120(t *testing.T) {
	cfg := newMockConfigProvider(nil)
	mw := rateLimitMiddleware(cfg)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "120", rec.Header().Get("X-RateLimit-Limit"))
}

// ---------------------------------------------------------------------------
// TestCustomLimitFromConfig
// ---------------------------------------------------------------------------

func TestRateLimit_CustomLimitFromConfig(t *testing.T) {
	cfg := newMockConfigProvider(map[string]interface{}{
		"api.rate_limit.requests_per_minute": 50,
	})
	mw := rateLimitMiddleware(cfg)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "50", rec.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "49", rec.Header().Get("X-RateLimit-Remaining"))
}

// ---------------------------------------------------------------------------
// TestPerUserTracking
// ---------------------------------------------------------------------------

func TestRateLimit_PerUserTracking(t *testing.T) {
	cfg := newMockConfigProvider(map[string]interface{}{
		"api.rate_limit.requests_per_minute": 2,
	})
	mw := rateLimitMiddleware(cfg)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(next)

	claims1 := &interfaces.UserClaims{UserID: "user-A", Email: "a@test.com"}
	claims2 := &interfaces.UserClaims{UserID: "user-B", Email: "b@test.com"}

	// user-A exhausts quota.
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := makeRequestWithClaims(t, claims1)
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// user-A is now rate limited.
	{
		rec := httptest.NewRecorder()
		req := makeRequestWithClaims(t, claims1)
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	}

	// user-B still has quota.
	{
		rec := httptest.NewRecorder()
		req := makeRequestWithClaims(t, claims2)
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}
}

// ---------------------------------------------------------------------------
// TestPerIPTracking
// ---------------------------------------------------------------------------

func TestRateLimit_PerIPTracking(t *testing.T) {
	cfg := newMockConfigProvider(map[string]interface{}{
		"api.rate_limit.requests_per_minute": 2,
	})
	mw := rateLimitMiddleware(cfg)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(next)

	// IP 1 exhausts quota.
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:9999"
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// IP 1 is rate limited.
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:9999"
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	}

	// IP 2 still has quota.
	{
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.2:9999"
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}
}

// ---------------------------------------------------------------------------
// TestHeadersOn429
// ---------------------------------------------------------------------------

func TestRateLimit_HeadersPresentOn429Response(t *testing.T) {
	cfg := newMockConfigProvider(map[string]interface{}{
		"api.rate_limit.requests_per_minute": 1,
	})
	mw := rateLimitMiddleware(cfg)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(next)

	// Exhaust quota.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Trigger 429.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "1", rec.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "0", rec.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, rec.Header().Get("X-RateLimit-Reset"))
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

// ---------------------------------------------------------------------------
// TestWindowReset
// ---------------------------------------------------------------------------

func TestRateLimit_WindowResetAllowsNewRequests(t *testing.T) {
	limiter := newRateLimiter(2, 100*time.Millisecond)

	allowed, _, _ := limiter.allow("test-key")
	assert.True(t, allowed)
	allowed, _, _ = limiter.allow("test-key")
	assert.True(t, allowed)
	allowed, _, _ = limiter.allow("test-key")
	assert.False(t, allowed, "should be rate limited")

	time.Sleep(150 * time.Millisecond)

	allowed, remaining, _ := limiter.allow("test-key")
	assert.True(t, allowed, "should be allowed after window reset")
	assert.Equal(t, 1, remaining)
}

// ---------------------------------------------------------------------------
// TestStripPort
// ---------------------------------------------------------------------------

func TestStripPort(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		expected string
	}{
		{"ipv4 with port", "1.2.3.4:1234", "1.2.3.4"},
		{"ipv6 with port", "[::1]:8080", "::1"},
		{"no port", "1.2.3.4", "1.2.3.4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripPort(tt.addr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ---------------------------------------------------------------------------
// TestRemainingDecrements
// ---------------------------------------------------------------------------

func TestRateLimit_RemainingDecrements(t *testing.T) {
	cfg := newMockConfigProvider(map[string]interface{}{
		"api.rate_limit.requests_per_minute": 3,
	})
	mw := rateLimitMiddleware(cfg)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(next)

	expectedRemaining := []string{"2", "1", "0"}
	for i, exp := range expectedRemaining {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, exp, rec.Header().Get("X-RateLimit-Remaining"), "request %d", i+1)
	}
}
