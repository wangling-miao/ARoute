package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

type AdminHandler struct {
	ctx        core.CoreContext
	contentSvc interfaces.ContentService
	lifecycle  core.LifecycleManager
	cacheSvc   interfaces.CacheService
}

func NewAdminHandler(ctx core.CoreContext, contentSvc interfaces.ContentService) *AdminHandler {
	h := &AdminHandler{ctx: ctx, contentSvc: contentSvc}

	if ctx != nil && ctx.Services() != nil {
		var lm core.LifecycleManager
		if err := ctx.Services().Get(&lm); err != nil {
			ctx.Logger().Warn("lifecycle manager not available for admin handler")
		} else {
			h.lifecycle = lm
		}

		var cs interfaces.CacheService
		if err := ctx.Services().Get(&cs); err != nil {
			ctx.Logger().Warn("cache service not available for admin handler")
		} else {
			h.cacheSvc = cs
		}
	}

	return h
}

func (h *AdminHandler) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	contentCounts := make(map[string]int64)

	types, err := h.contentSvc.ListContentTypes(r.Context())
	if err == nil {
		for _, ct := range types {
			page, listErr := h.contentSvc.List(r.Context(), ct.Name, &interfaces.ListQuery{
				Page:    1,
				PerPage: 1,
			})
			if listErr == nil && page != nil {
				contentCounts[ct.Name] = page.Meta.Total
			}
		}
	}

	pluginCount := 0
	if h.lifecycle != nil {
		pluginCount = len(h.lifecycle.ListPlugins())
	}

	var cacheHitRatio float64
	if h.cacheSvc != nil {
		stats := h.cacheSvc.Stats(r.Context())
		if stats != nil {
			cacheHitRatio = stats.HitRate
		}
	}

	stats := map[string]interface{}{
		"content_counts":  contentCounts,
		"recent_activity": []interface{}{},
		"system_status": map[string]interface{}{
			"database":        "healthy",
			"plugin_count":    pluginCount,
			"cache_hit_ratio": cacheHitRatio,
		},
	}

	writeJSON(w, http.StatusOK, stats)
}

func (h *AdminHandler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg := h.ctx.Config()

	settings := map[string]interface{}{
		"site_name":     cfg.GetString("site_name"),
		"site_url":      cfg.GetString("site_url"),
		"language":      cfg.GetString("language"),
		"timezone":      cfg.GetString("timezone"),
		"smtp_host":     cfg.GetString("smtp_host"),
		"smtp_port":     cfg.GetInt("smtp_port"),
		"smtp_username": cfg.GetString("smtp_username"),
		"sender_email":  cfg.GetString("sender_email"),
	}

	writeJSON(w, http.StatusOK, settings)
}

func (h *AdminHandler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body is not valid JSON")
		return
	}

	cfg := h.ctx.Config()
	for key, value := range body {
		cfg.Set(key, value)
	}
	if err := cfg.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, "SAVE_FAILED", "failed to save settings")
		return
	}

	h.handleGetSettings(w, r)
}

func (h *AdminHandler) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	if h.lifecycle == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	names := h.lifecycle.ListPlugins()
	plugins := make([]map[string]interface{}, 0, len(names))

	for _, name := range names {
		p := h.lifecycle.GetPlugin(name)
		if p == nil {
			continue
		}
		entry := map[string]interface{}{
			"name":    p.Name(),
			"version": p.Version(),
			"enabled": true,
		}
		if m := p.Manifest(); m != nil {
			entry["description"] = m.Description
			entry["author"] = m.Author
		}
		plugins = append(plugins, entry)
	}

	writeJSON(w, http.StatusOK, plugins)
}

func (h *AdminHandler) handleEnablePlugin(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "plugin name is required")
		return
	}

	if h.lifecycle != nil {
		if err := h.lifecycle.Enable(r.Context(), name); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "plugin enabled",
		"name":    name,
		"status":  "enabled",
	})
}

func (h *AdminHandler) handleDisablePlugin(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "plugin name is required")
		return
	}

	if h.lifecycle != nil {
		if err := h.lifecycle.Disable(r.Context(), name); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "plugin disabled",
		"name":    name,
		"status":  "disabled",
	})
}
