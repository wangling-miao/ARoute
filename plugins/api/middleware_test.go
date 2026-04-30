package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// ---------------------------------------------------------------------------
// mock AuthService
// ---------------------------------------------------------------------------

type mockAuthService struct {
	authenticateFunc   func(ctx context.Context, req *interfaces.AuthRequest) (*interfaces.AuthResult, error)
	verifyTokenFunc    func(ctx context.Context, token string) (*interfaces.UserClaims, error)
	refreshTokenFunc   func(ctx context.Context, refreshToken string) (*interfaces.TokenPair, error)
	createUserFunc     func(ctx context.Context, req *interfaces.CreateUserRequest) (*interfaces.User, error)
	getUserFunc        func(ctx context.Context, identifier string) (*interfaces.User, error)
	hasPermissionFunc  func(ctx context.Context, userID string, resource, action string) (bool, error)
	createAPITokenFunc func(ctx context.Context, userID string, name string, expiresAt *time.Time) (*interfaces.APIToken, error)
	revokeAPITokenFunc func(ctx context.Context, tokenID string) error
	updateUserFunc     func(ctx context.Context, id string, req *interfaces.UpdateUserRequest) (*interfaces.User, error)
	deleteUserFunc     func(ctx context.Context, id string) error
	listUsersFunc      func(ctx context.Context, query *interfaces.UserQuery) (*interfaces.Page, error)
}

func (m *mockAuthService) Authenticate(ctx context.Context, req *interfaces.AuthRequest) (*interfaces.AuthResult, error) {
	if m.authenticateFunc != nil {
		return m.authenticateFunc(ctx, req)
	}
	return nil, nil
}

func (m *mockAuthService) VerifyToken(ctx context.Context, token string) (*interfaces.UserClaims, error) {
	if m.verifyTokenFunc != nil {
		return m.verifyTokenFunc(ctx, token)
	}
	return nil, nil
}

func (m *mockAuthService) RefreshToken(ctx context.Context, refreshToken string) (*interfaces.TokenPair, error) {
	if m.refreshTokenFunc != nil {
		return m.refreshTokenFunc(ctx, refreshToken)
	}
	return nil, nil
}

func (m *mockAuthService) CreateUser(ctx context.Context, req *interfaces.CreateUserRequest) (*interfaces.User, error) {
	if m.createUserFunc != nil {
		return m.createUserFunc(ctx, req)
	}
	return nil, nil
}

func (m *mockAuthService) GetUser(ctx context.Context, identifier string) (*interfaces.User, error) {
	if m.getUserFunc != nil {
		return m.getUserFunc(ctx, identifier)
	}
	return nil, nil
}

func (m *mockAuthService) HasPermission(ctx context.Context, userID string, resource, action string) (bool, error) {
	if m.hasPermissionFunc != nil {
		return m.hasPermissionFunc(ctx, userID, resource, action)
	}
	return false, nil
}

func (m *mockAuthService) CreateAPIToken(ctx context.Context, userID string, name string, expiresAt *time.Time) (*interfaces.APIToken, error) {
	if m.createAPITokenFunc != nil {
		return m.createAPITokenFunc(ctx, userID, name, expiresAt)
	}
	return nil, nil
}

func (m *mockAuthService) RevokeAPIToken(ctx context.Context, tokenID string) error {
	if m.revokeAPITokenFunc != nil {
		return m.revokeAPITokenFunc(ctx, tokenID)
	}
	return nil
}

func (m *mockAuthService) UpdateUser(ctx context.Context, id string, req *interfaces.UpdateUserRequest) (*interfaces.User, error) {
	if m.updateUserFunc != nil {
		return m.updateUserFunc(ctx, id, req)
	}
	return nil, nil
}

func (m *mockAuthService) DeleteUser(ctx context.Context, id string) error {
	if m.deleteUserFunc != nil {
		return m.deleteUserFunc(ctx, id)
	}
	return nil
}

func (m *mockAuthService) ListUsers(ctx context.Context, query *interfaces.UserQuery) (*interfaces.Page, error) {
	if m.listUsersFunc != nil {
		return m.listUsersFunc(ctx, query)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func okHandler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
}

func claimsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := userClaimsFromRequest(r)
		w.Header().Set("Content-Type", "application/json")
		if claims == nil {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"claims":null}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"user_id": claims.UserID,
			"email":   claims.Email,
		})
	})
}

func makeClaims() *interfaces.UserClaims {
	return &interfaces.UserClaims{
		UserID:    "user-1",
		Email:     "test@example.com",
		Roles:     []string{"admin"},
		ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
		IssuedAt:  time.Now().Unix(),
		TokenID:   "jti-123",
	}
}

func decodeFirstError(t *testing.T, body []byte) APIError {
	t.Helper()
	var envelope ErrorsEnvelope
	require.NoError(t, json.Unmarshal(body, &envelope))
	require.NotEmpty(t, envelope.Errors, "expected at least one error")
	return envelope.Errors[0]
}

// ---------------------------------------------------------------------------
// TestAuthMiddleware
// ---------------------------------------------------------------------------

func TestAuthMiddleware_NilAuthService_IsNoOp(t *testing.T) {
	mw := authMiddleware(nil)
	require.NotNil(t, mw)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called, "next handler should be called when authSvc is nil")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_MissingAuthorizationHeader(t *testing.T) {
	authSvc := &mockAuthService{}
	mw := authMiddleware(authSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "UNAUTHORIZED", body.Code)
	assert.Equal(t, "missing authorization header", body.Message)
}

func TestAuthMiddleware_InvalidAuthorizationHeaderFormat(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{"basic auth", "Basic dXNlcjpwYXNz"},
		{"no prefix", "sometoken"},
		{"lowercase bearer", "bearer token123"},
		{"bearer without space", "Bearertoken123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authSvc := &mockAuthService{}
			mw := authMiddleware(authSvc)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Authorization", tt.header)

			mw(okHandler(t)).ServeHTTP(rec, req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code)

			body := decodeFirstError(t, rec.Body.Bytes())
			assert.Equal(t, "UNAUTHORIZED", body.Code)
			assert.Equal(t, "invalid authorization header format", body.Message)
		})
	}
}

func TestAuthMiddleware_EmptyBearerToken(t *testing.T) {
	authSvc := &mockAuthService{}
	mw := authMiddleware(authSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer ")

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "UNAUTHORIZED", body.Code)
	assert.Equal(t, "empty bearer token", body.Message)
}

func TestAuthMiddleware_ValidToken_SetsClaimsInContext(t *testing.T) {
	claims := makeClaims()

	authSvc := &mockAuthService{
		verifyTokenFunc: func(ctx context.Context, token string) (*interfaces.UserClaims, error) {
			assert.Equal(t, "valid-token-123", token)
			return claims, nil
		},
	}

	mw := authMiddleware(authSvc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token-123")

	mw(claimsHandler()).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"user_id":"user-1"`)
	assert.Contains(t, rec.Body.String(), `"email":"test@example.com"`)
}

func TestAuthMiddleware_ExpiredToken_Returns401(t *testing.T) {
	authSvc := &mockAuthService{
		verifyTokenFunc: func(ctx context.Context, token string) (*interfaces.UserClaims, error) {
			return nil, interfaces.ErrUnauthorized
		},
	}

	mw := authMiddleware(authSvc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer expired-token")

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "UNAUTHORIZED", body.Code)
	assert.Equal(t, "invalid or expired token", body.Message)
}

func TestAuthMiddleware_VerifyTokenInternalError_Returns500(t *testing.T) {
	authSvc := &mockAuthService{
		verifyTokenFunc: func(ctx context.Context, token string) (*interfaces.UserClaims, error) {
			return nil, errors.New("database connection lost")
		},
	}

	mw := authMiddleware(authSvc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer some-token")

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "INTERNAL_ERROR", body.Code)
	assert.Equal(t, "token verification failed", body.Message)
}

// ---------------------------------------------------------------------------
// TestRequirePermission
// ---------------------------------------------------------------------------

func TestRequirePermission_NilAuthService_IsNoOp(t *testing.T) {
	mw := requirePermission(nil, "content", "read")
	require.NotNil(t, mw)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called, "next handler should be called when authSvc is nil")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequirePermission_NoClaimsInContext_Returns401(t *testing.T) {
	authSvc := &mockAuthService{}
	mw := requirePermission(authSvc, "content", "write")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "UNAUTHORIZED", body.Code)
	assert.Equal(t, "authentication required", body.Message)
}

func TestRequirePermission_HasPermissionTrue_PassesThrough(t *testing.T) {
	claims := makeClaims()

	authSvc := &mockAuthService{
		hasPermissionFunc: func(ctx context.Context, userID string, resource, action string) (bool, error) {
			assert.Equal(t, "user-1", userID)
			assert.Equal(t, "content", resource)
			assert.Equal(t, "write", action)
			return true, nil
		},
	}

	authMW := authMiddleware(&mockAuthService{
		verifyTokenFunc: func(ctx context.Context, token string) (*interfaces.UserClaims, error) {
			return claims, nil
		},
	})
	permMW := requirePermission(authSvc, "content", "write")

	handler := permMW(okHandler(t))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Authorization", "Bearer token")

	authMW(handler).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequirePermission_HasPermissionFalse_Returns403(t *testing.T) {
	claims := makeClaims()

	authSvc := &mockAuthService{
		hasPermissionFunc: func(ctx context.Context, userID string, resource, action string) (bool, error) {
			return false, nil
		},
	}

	mw := requirePermission(authSvc, "content", "delete")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/test", nil)
	ctx := context.WithValue(req.Context(), claimsContextKey, claims)
	req = req.WithContext(ctx)

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)

	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "FORBIDDEN", body.Code)
	assert.Equal(t, "insufficient permissions", body.Message)
}

func TestRequirePermission_HasPermissionError_Returns500(t *testing.T) {
	claims := makeClaims()

	authSvc := &mockAuthService{
		hasPermissionFunc: func(ctx context.Context, userID string, resource, action string) (bool, error) {
			return false, errors.New("db timeout")
		},
	}

	mw := requirePermission(authSvc, "content", "read")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), claimsContextKey, claims)
	req = req.WithContext(ctx)

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "INTERNAL_ERROR", body.Code)
	assert.Equal(t, "permission check failed", body.Message)
}

// ---------------------------------------------------------------------------
// TestUserClaimsFromRequest
// ---------------------------------------------------------------------------

func TestUserClaimsFromRequest_ClaimsPresent(t *testing.T) {
	claims := makeClaims()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), claimsContextKey, claims)
	req = req.WithContext(ctx)

	got := userClaimsFromRequest(req)
	require.NotNil(t, got)
	assert.Equal(t, "user-1", got.UserID)
	assert.Equal(t, "test@example.com", got.Email)
}

func TestUserClaimsFromRequest_NoClaimsInContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	got := userClaimsFromRequest(req)
	assert.Nil(t, got)
}

func TestUserClaimsFromRequest_WrongTypeInContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), claimsContextKey, "not-claims-object")
	req = req.WithContext(ctx)

	got := userClaimsFromRequest(req)
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// TestContentNegotiationMiddleware
// ---------------------------------------------------------------------------

func TestContentNegotiationMiddleware_AcceptJSON_PassesThrough(t *testing.T) {
	mw := contentNegotiationMiddleware()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept", "application/json")

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestContentNegotiationMiddleware_NoAcceptHeader_PassesThrough(t *testing.T) {
	mw := contentNegotiationMiddleware()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestContentNegotiationMiddleware_AcceptWildcard_PassesThrough(t *testing.T) {
	mw := contentNegotiationMiddleware()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept", "*/*")

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestContentNegotiationMiddleware_AcceptXML_Returns406(t *testing.T) {
	mw := contentNegotiationMiddleware()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Accept", "application/xml")

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotAcceptable, rec.Code)
	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "NOT_ACCEPTABLE", body.Code)
}

func TestContentNegotiationMiddleware_PostWithTextPlainContentType_Returns415(t *testing.T) {
	mw := contentNegotiationMiddleware()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Content-Type", "text/plain")

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "UNSUPPORTED_MEDIA_TYPE", body.Code)
}

func TestContentNegotiationMiddleware_PostWithJSONContentType_PassesThrough(t *testing.T) {
	mw := contentNegotiationMiddleware()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Content-Type", "application/json")

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestContentNegotiationMiddleware_PutWithJSONContentType_PassesThrough(t *testing.T) {
	mw := contentNegotiationMiddleware()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/test", nil)
	req.Header.Set("Content-Type", "application/json")

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestContentNegotiationMiddleware_PostWithMultipartContentType_PassesThrough(t *testing.T) {
	mw := contentNegotiationMiddleware()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=----WebKitFormBoundary")

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestContentNegotiationMiddleware_PutWithNoContentType_PassesThrough(t *testing.T) {
	mw := contentNegotiationMiddleware()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/test", nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ---------------------------------------------------------------------------
// TestAuthMiddlewareWithPublicRead
// ---------------------------------------------------------------------------

func TestAuthMiddlewareWithPublicRead_PublicReadFalse_GetRequiresAuth(t *testing.T) {
	authSvc := &mockAuthService{}
	mw := authMiddlewareWithPublicRead(authSvc, false)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "UNAUTHORIZED", body.Code)
}

func TestAuthMiddlewareWithPublicRead_PublicReadTrue_GetPassesWithoutAuth(t *testing.T) {
	authSvc := &mockAuthService{}
	mw := authMiddlewareWithPublicRead(authSvc, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddlewareWithPublicRead_PublicReadTrue_PostRequiresAuth(t *testing.T) {
	authSvc := &mockAuthService{}
	mw := authMiddlewareWithPublicRead(authSvc, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "UNAUTHORIZED", body.Code)
}

func TestAuthMiddlewareWithPublicRead_PublicReadTrue_PutRequiresAuth(t *testing.T) {
	authSvc := &mockAuthService{}
	mw := authMiddlewareWithPublicRead(authSvc, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/test", nil)

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "UNAUTHORIZED", body.Code)
}

func TestAuthMiddlewareWithPublicRead_PublicReadTrue_DeleteRequiresAuth(t *testing.T) {
	authSvc := &mockAuthService{}
	mw := authMiddlewareWithPublicRead(authSvc, true)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/test", nil)

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "UNAUTHORIZED", body.Code)
}

func TestAuthMiddlewareWithPublicRead_PublicReadTrue_PostWithValidToken_Passes(t *testing.T) {
	claims := makeClaims()
	authSvc := &mockAuthService{
		verifyTokenFunc: func(ctx context.Context, token string) (*interfaces.UserClaims, error) {
			return claims, nil
		},
	}
	mw := authMiddlewareWithPublicRead(authSvc, true)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddlewareWithPublicRead_NilAuthService_PublicReadTrue_IsNoOp(t *testing.T) {
	mw := authMiddlewareWithPublicRead(nil, true)
	require.NotNil(t, mw)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called, "next handler should be called when authSvc is nil")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddlewareWithPublicRead_NilAuthService_PublicReadFalse_IsNoOp(t *testing.T) {
	mw := authMiddlewareWithPublicRead(nil, false)
	require.NotNil(t, mw)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called, "next handler should be called when authSvc is nil")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ---------------------------------------------------------------------------
// C3: TestPerContentTypeAuthMiddleware
// ---------------------------------------------------------------------------

func TestPerContentTypeAuthMiddleware_AuthDisabled_SkipsAuth(t *testing.T) {
	authSvc := &mockAuthService{}
	config := newMockConfigProvider(map[string]interface{}{
		"content_types.posts.auth_required": false,
	})
	mw := perContentTypeAuthMiddleware(authSvc, false, config)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/posts", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("contentType", "posts")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called, "next handler should be called when CT auth is disabled")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPerContentTypeAuthMiddleware_NoOverride_AppliesGlobalPolicy(t *testing.T) {
	authSvc := &mockAuthService{}
	config := newMockConfigProvider(map[string]interface{}{})
	mw := perContentTypeAuthMiddleware(authSvc, false, config)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("contentType", "posts")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	body := decodeFirstError(t, rec.Body.Bytes())
	assert.Equal(t, "UNAUTHORIZED", body.Code)
}

func TestPerContentTypeAuthMiddleware_NoContentTypeParam_AppliesGlobalPolicy(t *testing.T) {
	authSvc := &mockAuthService{}
	config := newMockConfigProvider(nil)
	mw := perContentTypeAuthMiddleware(authSvc, false, config)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/content", nil)

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPerContentTypeAuthMiddleware_PublicReadTrue_NoOverride_GetPasses(t *testing.T) {
	authSvc := &mockAuthService{}
	config := newMockConfigProvider(map[string]interface{}{})
	mw := perContentTypeAuthMiddleware(authSvc, true, config)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("contentType", "posts")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	mw(next).ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPerContentTypeAuthMiddleware_NilConfig_AppliesGlobalPolicy(t *testing.T) {
	authSvc := &mockAuthService{}
	mw := perContentTypeAuthMiddleware(authSvc, false, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("contentType", "posts")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	mw(okHandler(t)).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
