package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	authplugin "github.com/wangling-miao/aroute/plugins/auth"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type adminHandler struct {
	service *Service
	authSvc interfaces.AuthService
}

type apiTokenVerifier interface {
	VerifyAPIToken(ctx context.Context, token string) (*interfaces.UserClaims, error)
}

func (h *adminHandler) checkPerm(w http.ResponseWriter, r *http.Request, resource, action string) bool {
	if h.authSvc == nil {
		http.Error(w, "auth service unavailable", http.StatusServiceUnavailable)
		return false
	}
	claims, ok := h.claimsFromRequest(w, r)
	if !ok {
		return false
	}
	allowed, err := h.authSvc.HasPermission(r.Context(), claims.UserID, resource, action)
	if err != nil {
		http.Error(w, "permission check failed", http.StatusInternalServerError)
		return false
	}
	if !allowed {
		http.Error(w, "insufficient permissions", http.StatusForbidden)
		return false
	}
	return true
}

func (h *adminHandler) claimsFromRequest(w http.ResponseWriter, r *http.Request) (*interfaces.UserClaims, bool) {
	claims := authplugin.GetClaimsFromContext(r.Context())
	if claims != nil {
		return claims, true
	}

	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return nil, false
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return nil, false
	}

	var err error
	if strings.HasPrefix(token, "aroute_") {
		verifier, ok := h.authSvc.(apiTokenVerifier)
		if !ok {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return nil, false
		}
		claims, err = verifier.VerifyAPIToken(r.Context(), token)
	} else {
		claims, err = h.authSvc.VerifyToken(r.Context(), token)
	}
	if err != nil {
		if errors.Is(err, interfaces.ErrForbidden) {
			http.Error(w, "account is not active", http.StatusForbidden)
			return nil, false
		}
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return nil, false
	}
	if claims == nil {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return nil, false
	}
	return claims, true
}

func (h *adminHandler) createWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "webhooks", "create") {
		return
	}
	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
		Secret string   `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	wh, err := h.service.Create(r.Context(), req.URL, req.Events, req.Secret)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(wh)
}

func (h *adminHandler) listWebhooks(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "webhooks", "read") {
		return
	}
	webhooks := h.service.List(r.Context())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhooks)
}

func (h *adminHandler) getWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "webhooks", "read") {
		return
	}
	id := chi.URLParam(r, "webhookID")
	wh, err := h.service.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	deliveries, total := h.service.GetDeliveries(r.Context(), id, 1000, 0)
	var successCount, failureCount int
	for _, d := range deliveries {
		if d.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":                   wh.ID,
		"url":                  wh.URL,
		"events":               wh.Events,
		"enabled":              wh.Enabled,
		"consecutive_failures": wh.ConsecutiveFailures,
		"disabled_reason":      wh.DisabledReason,
		"created_at":           wh.CreatedAt,
		"updated_at":           wh.UpdatedAt,
		"stats": map[string]int{
			"success_count": successCount,
			"failure_count": failureCount,
			"total":         total,
		},
	})
}

func (h *adminHandler) updateWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "webhooks", "update") {
		return
	}
	id := chi.URLParam(r, "webhookID")

	var req struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	wh, err := h.service.Update(r.Context(), id, req.URL, req.Events)
	if err != nil {
		status := http.StatusBadRequest
		if err.Error() != "" && len(err.Error()) > 0 && err.Error()[0] == 'w' {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(wh)
}

func (h *adminHandler) patchWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "webhooks", "update") {
		return
	}
	id := chi.URLParam(r, "webhookID")

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if enabled, ok := req["enabled"].(bool); ok {
		if err := h.service.SetEnabled(r.Context(), id, enabled); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}
	if secret, ok := req["secret"].(string); ok {
		if err := h.service.UpdateSecret(r.Context(), id, secret); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}

	wh, err := h.service.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(wh)
}

func (h *adminHandler) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "webhooks", "delete") {
		return
	}
	id := chi.URLParam(r, "webhookID")
	if err := h.service.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *adminHandler) testWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "webhooks", "test") {
		return
	}
	id := chi.URLParam(r, "webhookID")

	delivery, err := h.service.TestDelivery(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(delivery)
}

func (h *adminHandler) listDeliveries(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "webhooks", "read") {
		return
	}
	id := chi.URLParam(r, "webhookID")

	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 20
	offset := 0

	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
		limit = v
	}
	if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
		offset = v
	}

	deliveries, total := h.service.GetDeliveries(r.Context(), id, limit, offset)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":   deliveries,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}
