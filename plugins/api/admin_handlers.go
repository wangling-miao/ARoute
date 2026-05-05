package api

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// systemPluginEngines marks these engine types as non-disablable system plugins.
var systemPluginEngines = map[string]bool{
	"native": true,
	"l1":     true,
}

type AdminHandler struct {
	ctx        core.CoreContext
	contentSvc interfaces.ContentService
	authSvc    interfaces.AuthService
	themeSvc   interfaces.ThemeService
	adminUI    interfaces.AdminUISwitcher
	lifecycle  core.LifecycleManager
	registry   core.PluginRegistry
	cacheSvc   interfaces.CacheService
}

func NewAdminHandler(ctx core.CoreContext, contentSvc interfaces.ContentService, authSvc interfaces.AuthService) *AdminHandler {
	h := &AdminHandler{ctx: ctx, contentSvc: contentSvc, authSvc: authSvc}

	if ctx != nil && ctx.Services() != nil {
		var lm core.LifecycleManager
		if err := ctx.Services().Get(&lm); err != nil {
			ctx.Logger().Warn("lifecycle manager not available for admin handler")
		} else {
			h.lifecycle = lm
		}

		var reg core.PluginRegistry
		if err := ctx.Services().Get(&reg); err != nil {
			ctx.Logger().Warn("registry not available for admin handler")
		} else {
			h.registry = reg
		}

		var cs interfaces.CacheService
		if err := ctx.Services().Get(&cs); err != nil {
			ctx.Logger().Warn("cache service not available for admin handler")
		} else {
			h.cacheSvc = cs
		}

		var ts interfaces.ThemeService
		if err := ctx.Services().Get(&ts); err != nil {
			ctx.Logger().Warn("theme service not available for admin handler")
		} else {
			h.themeSvc = ts
		}

		var au interfaces.AdminUISwitcher
		if err := ctx.Services().Get(&au); err != nil {
			ctx.Logger().Warn("admin UI switcher not available for admin handler")
		} else {
			h.adminUI = au
		}
	}

	return h
}

// checkPerm checks if the authenticated user has the specified permission.
func (h *AdminHandler) checkPerm(w http.ResponseWriter, r *http.Request, resource, action string) bool {
	if h.authSvc == nil {
		return true
	}
	claims := userClaimsFromRequest(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return false
	}
	allowed, err := h.authSvc.HasPermission(r.Context(), claims.UserID, resource, action)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "permission check failed")
		return false
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
		return false
	}
	return true
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
	if h.registry != nil {
		entries, err := h.registry.List()
		if err == nil {
			pluginCount = len(entries)
		}
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
	if !h.checkPerm(w, r, "settings", "read") {
		return
	}

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
	if !h.checkPerm(w, r, "settings", "update") {
		return
	}

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
	if !h.checkPerm(w, r, "plugins", "read") {
		return
	}

	if h.registry == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}

	entries, err := h.registry.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list plugins")
		return
	}

	// Sort entries by name.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Manifest.Name < entries[j].Manifest.Name
	})

	type pluginEntry struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Author      string `json:"author"`
		Enabled     bool   `json:"enabled"`
		State       string `json:"state"`
		IsSystem    bool   `json:"is_system"`
	}

	plugins := make([]pluginEntry, 0, len(entries))
	for _, e := range entries {
		state := "not_loaded"
		if h.lifecycle != nil {
			if s, err := h.lifecycle.GetState(e.Manifest.Name); err == nil {
				state = s.String()
			}
		}

		isSystem := systemPluginEngines[e.Manifest.Engine]

		plugins = append(plugins, pluginEntry{
			Name:        e.Manifest.Name,
			Version:     e.Manifest.Version,
			Description: e.Manifest.Description,
			Author:      e.Manifest.Author,
			Enabled:     e.Enabled,
			State:       state,
			IsSystem:    isSystem,
		})
	}

	writeJSON(w, http.StatusOK, plugins)
}

func (h *AdminHandler) handleEnablePlugin(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "plugins", "enable") {
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "plugin name is required")
		return
	}

	// Persist enabled state to registry so it loads on next restart.
	if h.registry != nil {
		if err := h.registry.Enable(name); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
	}

	// Try to start the plugin in-memory if lifecycle is available.
	if h.lifecycle != nil {
		if err := h.lifecycle.Enable(r.Context(), name); err != nil {
			// Plugin may not be loaded in lifecycle (e.g., requires restart).
			// Log but don't fail — the registry state is what matters.
			slog.Warn("plugin enable in lifecycle failed", "plugin", name, "error", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "plugin enabled",
		"name":    name,
		"status":  "enabled",
	})
}

func (h *AdminHandler) handleDisablePlugin(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "plugins", "disable") {
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "plugin name is required")
		return
	}

	// Check if it's a system plugin.
	if h.registry != nil {
		entry, err := h.registry.Get(name)
		if err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "plugin not found")
			return
		}
		if systemPluginEngines[entry.Manifest.Engine] {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "system plugins cannot be disabled")
			return
		}
	}

	// Stop the plugin in-memory.
	if h.lifecycle != nil {
		if err := h.lifecycle.Disable(r.Context(), name); err != nil {
			slog.Warn("plugin disable in lifecycle failed", "plugin", name, "error", err)
		}
	}

	// Persist disabled state to registry.
	if h.registry != nil {
		if err := h.registry.Disable(name); err != nil {
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

func (h *AdminHandler) handleUploadPlugin(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "plugins", "enable") {
		return
	}

	// Limit upload size to 50MB.
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "failed to parse upload: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "missing file in upload")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".zip" && ext != ".tar.gz" && ext != ".wasm" {
		writeError(w, http.StatusBadRequest, "INVALID_FILE", "unsupported file type, accepted: .zip, .tar.gz, .wasm")
		return
	}

	pluginDir := "/tmp/aroute-uploads"
	if h.ctx != nil {
		pluginDir = h.ctx.PluginDir()
	}
	destDir := filepath.Join(filepath.Dir(pluginDir), "plugins")
	os.MkdirAll(destDir, 0755)

	var manifestPath string
	var extractedDir string

	switch ext {
	case ".zip":
		extractedDir, manifestPath, err = h.extractZip(file, header.Filename, destDir)
	case ".wasm":
		extractedDir, manifestPath, err = h.saveWasm(file, header.Filename, destDir)
	default:
		writeError(w, http.StatusBadRequest, "INVALID_FILE", "unsupported file type")
		return
	}

	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_FILE", err.Error())
		return
	}

	// Parse manifest.
	if manifestPath == "" {
		os.RemoveAll(extractedDir)
		writeError(w, http.StatusBadRequest, "INVALID_FILE", "plugin manifest not found in archive")
		return
	}

	manifest, err := core.LoadManifest(manifestPath)
	if err != nil {
		os.RemoveAll(extractedDir)
		writeError(w, http.StatusBadRequest, "INVALID_MANIFEST", "invalid manifest: "+err.Error())
		return
	}

	// Register in registry.
	if h.registry != nil {
		if err := h.registry.Register(&core.PluginEntry{
			Manifest:       *manifest,
			Enabled:        true,
			DiscoveredPath: extractedDir,
		}); err != nil {
			// Plugin may already exist — try update instead.
			if strings.Contains(err.Error(), "already exists") {
				if updateErr := h.registry.Update(manifest.Name, *manifest); updateErr != nil {
					writeError(w, http.StatusConflict, "CONFLICT", updateErr.Error())
					return
				}
			} else {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
				return
			}
		}
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message":     "plugin installed",
		"name":        manifest.Name,
		"version":     manifest.Version,
		"description": manifest.Description,
		"author":      manifest.Author,
		"enabled":     true,
		"state":       "not_loaded",
		"is_system":   systemPluginEngines[manifest.Engine],
	})
}

func (h *AdminHandler) extractZip(src io.ReaderAt, filename string, destDir string) (string, string, error) {
	// Save temp zip first.
	tmpZip := filepath.Join(destDir, ".tmp-"+filename)
	f, err := os.Create(tmpZip)
	if err != nil {
		return "", "", fmt.Errorf("create temp file: %w", err)
	}
	defer f.Close()
	defer os.Remove(tmpZip)

	size := int64(50 << 20) // max size
	if s, ok := src.(interface{ Size() int64 }); ok {
		size = s.Size()
	}

	if _, err := io.Copy(f, io.NewSectionReader(src, 0, size)); err != nil {
		return "", "", fmt.Errorf("write temp file: %w", err)
	}

	zipReader, err := zip.OpenReader(tmpZip)
	if err != nil {
		return "", "", fmt.Errorf("open zip: %w", err)
	}
	defer zipReader.Close()

	// Find base directory name from zip.
	baseName := strings.TrimSuffix(filepath.Base(filename), ".zip")
	extractDir := filepath.Join(destDir, baseName)
	os.MkdirAll(extractDir, 0755)

	var manifestPath string
	for _, zf := range zipReader.File {
		// Security: skip path traversal.
		if strings.Contains(zf.Name, "..") {
			continue
		}

		target := filepath.Join(extractDir, zf.Name)

		if zf.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}

		os.MkdirAll(filepath.Dir(target), 0755)

		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, zf.Mode())
		if err != nil {
			return "", "", fmt.Errorf("extract %s: %w", zf.Name, err)
		}

		rc, err := zf.Open()
		if err != nil {
			outFile.Close()
			return "", "", fmt.Errorf("open zip entry %s: %w", zf.Name, err)
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return "", "", fmt.Errorf("write %s: %w", zf.Name, err)
		}

		// Look for manifest.
		base := filepath.Base(zf.Name)
		if base == "manifest.yaml" || base == "manifest.json" {
			manifestPath = target
		}
	}

	return extractDir, manifestPath, nil
}

func (h *AdminHandler) saveWasm(src io.Reader, filename string, destDir string) (string, string, error) {
	baseName := strings.TrimSuffix(filepath.Base(filename), ".wasm")
	pluginPath := filepath.Join(destDir, baseName)
	os.MkdirAll(pluginPath, 0755)

	// Save .wasm file.
	wasmFile := filepath.Join(pluginPath, filename)
	f, err := os.Create(wasmFile)
	if err != nil {
		return "", "", fmt.Errorf("create wasm file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, src); err != nil {
		return "", "", fmt.Errorf("write wasm file: %w", err)
	}

	// Check if manifest already exists alongside.
	manifestPath := filepath.Join(pluginPath, "manifest.yaml")
	if _, err := os.Stat(manifestPath); err != nil {
		// Check for JSON variant.
		manifestPath = filepath.Join(pluginPath, "manifest.json")
		if _, err := os.Stat(manifestPath); err != nil {
			// No manifest — caller will report the error.
			return pluginPath, "", nil
		}
	}

	return pluginPath, manifestPath, nil
}

func (h *AdminHandler) handleListThemes(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "settings", "read") {
		return
	}

	if h.themeSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "theme service not available")
		return
	}

	// Rescan for newly added themes (hot-reload).
	_ = h.themeSvc.ReloadThemes()

	names, err := h.themeSvc.ListThemes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list themes")
		return
	}

	active, _ := h.themeSvc.GetActiveTheme(r.Context())

	type themeEntry struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Version     string `json:"version"`
		Author      string `json:"author"`
		Description string `json:"description"`
		Engine      string `json:"engine"`
		Active      bool   `json:"active"`
	}

	themes := make([]themeEntry, 0, len(names))
	for _, slug := range names {
		entry := themeEntry{Slug: slug, Active: slug == active}
		if meta := h.themeSvc.ThemeMeta(slug); meta != nil {
			entry.Name = meta["name"]
			entry.Version = meta["version"]
			entry.Author = meta["author"]
			entry.Description = meta["description"]
			entry.Engine = meta["engine"]
		}
		themes = append(themes, entry)
	}

	writeJSON(w, http.StatusOK, themes)
}

func (h *AdminHandler) handleGetActiveTheme(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "settings", "read") {
		return
	}

	if h.themeSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "theme service not available")
		return
	}

	active, err := h.themeSvc.GetActiveTheme(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get active theme")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"theme": active})
}

func (h *AdminHandler) handleSetActiveTheme(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "settings", "update") {
		return
	}

	if h.themeSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "theme service not available")
		return
	}

	var body struct {
		Theme string `json:"theme"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body is not valid JSON")
		return
	}
	defer r.Body.Close()

	if body.Theme == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "theme is required")
		return
	}

	if err := h.themeSvc.SetActiveTheme(r.Context(), body.Theme); err != nil {
		slog.Error("failed to set active theme", "theme", body.Theme, "error", err)
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "theme not found: "+body.Theme)
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to set active theme")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"theme": body.Theme})
}

func (h *AdminHandler) handleListAdminVariants(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "settings", "read") {
		return
	}

	if h.adminUI == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "admin UI switcher not available")
		return
	}

	variants, err := h.adminUI.ListVariants(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list admin variants")
		return
	}

	if variants == nil {
		variants = []interfaces.VariantInfo{}
	}

	writeJSON(w, http.StatusOK, variants)
}

func (h *AdminHandler) handleGetAdminVariant(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "settings", "read") {
		return
	}

	if h.adminUI == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "admin UI switcher not available")
		return
	}

	active, err := h.adminUI.GetActiveVariant(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get active admin variant")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"variant": active})
}

func (h *AdminHandler) handleSetAdminVariant(w http.ResponseWriter, r *http.Request) {
	if !h.checkPerm(w, r, "settings", "update") {
		return
	}

	if h.adminUI == nil {
		writeError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "admin UI switcher not available")
		return
	}

	var body struct {
		Variant string `json:"variant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body is not valid JSON")
		return
	}
	defer r.Body.Close()

	if body.Variant == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "variant is required")
		return
	}

	if err := h.adminUI.SetActiveVariant(r.Context(), body.Variant); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "admin variant not found: "+body.Variant)
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to set admin variant")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"variant": body.Variant})
}

// handleGetSiteInfo returns public site information (no auth required).
func (h *AdminHandler) handleGetSiteInfo(w http.ResponseWriter, r *http.Request) {
	cfg := h.ctx.Config()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"site_name": cfg.GetString("site_name"),
		"site_url":  cfg.GetString("site_url"),
	})
}
