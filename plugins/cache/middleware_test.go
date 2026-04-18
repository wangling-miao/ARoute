package cache

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBypassMiddleware_NoCacheHeader(t *testing.T) {
	var called bool
	var gotCtx context.Context

	handler := BypassMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		gotCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Cache-Control", "no-cache")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.True(t, called, "handler should be called")
	assert.True(t, IsBypassed(gotCtx), "context should indicate bypass")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestBypassMiddleware_NoHeader(t *testing.T) {
	var called bool
	var gotCtx context.Context

	handler := BypassMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		gotCtx = r.Context()
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.True(t, called, "handler should be called")
	assert.False(t, IsBypassed(gotCtx), "context should not indicate bypass")
}

func TestBypassMiddleware_OtherCacheControl(t *testing.T) {
	tests := []struct {
		name string
		cc   string
	}{
		{"max-age", "max-age=3600"},
		{"no-store", "no-store"},
		{"no-transform", "no-transform"},
		{"must-revalidate", "must-revalidate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotCtx context.Context

			handler := BypassMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotCtx = r.Context()
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Cache-Control", tt.cc)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.False(t, IsBypassed(gotCtx), "non-no-cache directives should not trigger bypass")
		})
	}
}

func TestBypassMiddleware_MultipleDirectives(t *testing.T) {
	tests := []struct {
		name   string
		cc     string
		bypass bool
	}{
		{"no-store then no-cache", "no-store, no-cache", true},
		{"no-cache then max-age", "no-cache, max-age=0", true},
		{"max-age and no-store only", "max-age=3600, no-store", false},
		{"spaced no-cache", " no-cache , max-age=0 ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotCtx context.Context

			handler := BypassMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotCtx = r.Context()
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Cache-Control", tt.cc)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.bypass, IsBypassed(gotCtx))
		})
	}
}

func TestIsBypassed(t *testing.T) {
	t.Run("nil context", func(t *testing.T) {
		assert.False(t, IsBypassed(context.Background()))
	})

	t.Run("with bypass true", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), bypassKey, true)
		assert.True(t, IsBypassed(ctx))
	})

	t.Run("with bypass false", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), bypassKey, false)
		assert.False(t, IsBypassed(ctx))
	})

	t.Run("with wrong key type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "cache_bypass", true)
		assert.False(t, IsBypassed(ctx))
	})

	t.Run("with wrong value type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), bypassKey, "true")
		assert.False(t, IsBypassed(ctx))
	})
}

func TestIsNoCacheRequest(t *testing.T) {
	tests := []struct {
		name   string
		header string
		expect bool
	}{
		{"empty header", "", false},
		{"exact no-cache", "no-cache", true},
		{"no-cache with spaces", "  no-cache  ", true},
		{"no-cache in list", "no-store, no-cache", true},
		{"no-cache first", "no-cache, max-age=0", true},
		{"max-age only", "max-age=3600", false},
		{"no-store only", "no-store", false},
		{"must-revalidate only", "must-revalidate", false},
		{"case sensitive", "No-Cache", false},
		{"no-cache-variant", "no-cache-foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Cache-Control", tt.header)
			}
			assert.Equal(t, tt.expect, isNoCacheRequest(req))
		})
	}
}

func TestGetCachedValue_Bypassed(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Seed the cache
	err := svc.Set(ctx, "bypass-key", "stored-value", 0)
	assert.NoError(t, err)

	// Bypassed context should return (nil, false) even though key exists
	bypassedCtx := context.WithValue(ctx, bypassKey, true)
	val, found := svc.GetCachedValue(bypassedCtx, "bypass-key")
	assert.Nil(t, val)
	assert.False(t, found, "GetCachedValue should miss when bypassed")
}

func TestGetCachedValue_NotBypassed(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "normal-key", "normal-value", 0)
	assert.NoError(t, err)

	val, found := svc.GetCachedValue(ctx, "normal-key")
	assert.Equal(t, "normal-value", val)
	assert.True(t, found, "GetCachedValue should hit when not bypassed")
}

func TestGetCachedValue_Miss(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	val, found := svc.GetCachedValue(ctx, "nonexistent")
	assert.Nil(t, val)
	assert.False(t, found, "GetCachedValue should miss for nonexistent key")
}

func TestSetCachedValue_AlwaysStores(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	bypassedCtx := context.WithValue(ctx, bypassKey, true)

	// SetCachedValue should store even when bypass is active
	err := svc.SetCachedValue(bypassedCtx, "fresh-key", "fresh-value", 0)
	assert.NoError(t, err)

	// A subsequent non-bypassed request should get the value
	val, found := svc.GetCachedValue(ctx, "fresh-key")
	assert.Equal(t, "fresh-value", val)
	assert.True(t, found, "value stored during bypass should be available to later requests")
}

func TestSetCachedValue_WithCustomTTL(t *testing.T) {
	svc := newTestServiceWithTTL(t, 5*time.Minute)
	ctx := context.Background()

	err := svc.SetCachedValue(ctx, "ttl-key", "ttl-value", 200*time.Millisecond)
	assert.NoError(t, err)

	val, found := svc.GetCachedValue(ctx, "ttl-key")
	assert.Equal(t, "ttl-value", val)
	assert.True(t, found)
}

func TestBypassMiddleware_EndToEnd(t *testing.T) {
	svc := newTestService(t)
	bgCtx := context.Background()

	// Pre-seed cache with stale value
	err := svc.Set(bgCtx, "e2e-key", "stale", 0)
	assert.NoError(t, err)

	// Simulate a handler that uses GetCachedValue/SetCachedValue
	handler := BypassMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		key := "e2e-key"

		val, found := svc.GetCachedValue(ctx, key)
		if found {
			_, _ = w.Write([]byte(val.(string)))
			return
		}

		// Simulate origin fetch + store
		freshValue := "fresh"
		_ = svc.SetCachedValue(ctx, key, freshValue, 0)
		_, _ = w.Write([]byte(freshValue))
	}))

	t.Run("bypass request gets fresh value and stores it", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Cache-Control", "no-cache")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, "fresh", rec.Body.String())

		// Verify the fresh value was stored for other clients
		val, found := svc.GetCachedValue(bgCtx, "e2e-key")
		assert.True(t, found)
		assert.Equal(t, "fresh", val)
	})

	t.Run("normal request gets cached value", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, "fresh", rec.Body.String())
	})
}
