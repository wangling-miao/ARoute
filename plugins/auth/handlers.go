package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type authResponse struct {
	Data interface{} `json:"data"`
	Meta interface{} `json:"meta"`
}

type errorsEnvelope struct {
	Errors []authErrItem `json:"errors"`
}

type authErrItem struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details"`
}

func writeAuthJSON(w http.ResponseWriter, status int, data interface{}) {
	resp := authResponse{Data: data, Meta: map[string]interface{}{}}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	envelope := errorsEnvelope{
		Errors: []authErrItem{
			{Code: code, Message: message, Details: map[string]interface{}{}},
		},
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(envelope)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	AccessToken string `json:"access_token"`
}

func (p *Plugin) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}

	remoteIP := extractClientIP(r, p.service.trustedProxies)
	ctx := context.WithValue(r.Context(), ContextKeyRemoteIP, remoteIP)

	result, err := p.service.Authenticate(ctx, &interfaces.AuthRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		if errors.Is(err, interfaces.ErrUnauthorized) {
			writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
			return
		}
		if errors.Is(err, interfaces.ErrValidation) {
			writeAuthError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		if errors.Is(err, interfaces.ErrForbidden) {
			writeAuthError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
			return
		}
		if ok, retryAfter := IsRateLimitError(err); ok {
			w.Header().Set("Retry-After", http.StatusText(retryAfter))
			writeAuthError(w, http.StatusTooManyRequests, "RATE_LIMITED", err.Error())
			return
		}
		writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "authentication failed")
		return
	}

	writeAuthJSON(w, http.StatusOK, loginResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
	})
}

func (p *Plugin) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}

	if req.RefreshToken == "" {
		writeAuthError(w, http.StatusBadRequest, "VALIDATION_ERROR", "refresh_token is required")
		return
	}

	result, err := p.service.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, interfaces.ErrUnauthorized) {
			writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired refresh token")
			return
		}
		if errors.Is(err, interfaces.ErrForbidden) {
			writeAuthError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
			return
		}
		writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "token refresh failed")
		return
	}

	writeAuthJSON(w, http.StatusOK, refreshResponse{
		AccessToken: result.AccessToken,
	})
}

func (p *Plugin) handleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	claims, err := authenticateRequest(r, p.service)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid authorization header")
		return
	}

	user, err := p.service.GetUser(r.Context(), claims.UserID)
	if err != nil {
		if errors.Is(err, interfaces.ErrNotFound) {
			writeAuthError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
			return
		}
		writeAuthError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get user")
		return
	}

	writeAuthJSON(w, http.StatusOK, user)
}
