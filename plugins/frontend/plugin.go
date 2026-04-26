package frontend

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

//go:embed manifest.yaml
var manifestData []byte

type Plugin struct {
	*core.BasePlugin

	mu      sync.RWMutex
	ctx     core.CoreContext
	running bool
}

func New() *Plugin {
	manifest, err := core.ParseManifest(manifestData, ".yaml")
	if err != nil {
		panic("frontend plugin: failed to parse embedded manifest: " + err.Error())
	}
	return &Plugin{
		BasePlugin: core.NewBasePluginFromManifest(manifest),
	}
}

func (p *Plugin) Init(ctx core.CoreContext) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx = ctx
	logger := ctx.Logger()

	logger.Info("Initializing frontend plugin")

	var registrar interfaces.RouteRegistrar
	if err := ctx.Services().Get(&registrar); err != nil {
		return fmt.Errorf("route registrar not available: %w", err)
	}

	var themeSvc interfaces.ThemeService
	if err := ctx.Services().Get(&themeSvc); err != nil {
		return fmt.Errorf("theme service not available: %w", err)
	}

	var contentSvc interfaces.ContentService
	if err := ctx.Services().Get(&contentSvc); err != nil {
		return fmt.Errorf("content service not available: %w", err)
	}

	// Theme assets live in the theme plugin's data dir: data/plugin_data/theme/
	themesDataDir := filepath.Join(filepath.Dir(ctx.DataDir()), "theme")

	handler := &frontendHandler{
		themeSvc:   themeSvc,
		contentSvc: contentSvc,
		config:     ctx.Config(),
		themesDir:  themesDataDir,
		logger:     ctx.Logger(),
	}

	// Register frontend routes
	registrar.HandleFunc("/", handler.handleIndex)
	registrar.HandleFunc("/posts", handler.handlePosts)
	registrar.HandleFunc("/posts/{slug}", handler.handlePost)
	registrar.HandleFunc("/page/{slug}", handler.handlePage)
	registrar.HandleFunc("/archive", handler.handleArchive)
	registrar.HandleFunc("/about", handler.handleAbout)

	// Theme asset serving
	registrar.Handle("/themes/{theme}/assets/*", http.HandlerFunc(handler.serveThemeAsset))

	// Catch-all: render themed 404 for any unmatched public URL
	registrar.HandleFunc("/*", handler.render404)

	logger.Info("Frontend plugin initialized successfully")
	return nil
}

func (p *Plugin) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	p.ctx.Logger().Info("Frontend plugin started successfully")
	p.running = true
	return nil
}

func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.running {
		return nil
	}

	p.ctx.Logger().Info("Frontend plugin stopped successfully")
	p.running = false
	return nil
}

type frontendHandler struct {
	themeSvc   interfaces.ThemeService
	contentSvc interfaces.ContentService
	config     core.ConfigProvider
	themesDir  string
	logger     *slog.Logger
}

func (h *frontendHandler) siteData() map[string]interface{} {
	title := "ARoute CMS"
	tagline := "A modern CMS"
	if h.config != nil {
		if t := h.config.GetString("site.title"); t != "" {
			title = t
		}
		if t := h.config.GetString("site.tagline"); t != "" {
			tagline = t
		}
	}
	return map[string]interface{}{
		"Title":   title,
		"Tagline": tagline,
	}
}

type navItem struct {
	Title    string
	URL      string
	Children []navItem
}

func (h *frontendHandler) menuData(ctx context.Context) []navItem {
	menus, err := h.contentSvc.List(ctx, "menu", &interfaces.ListQuery{
		PerPage: 200,
		Filters: map[string]interface{}{"status": "published"},
		Sort:    "sort_order",
		Order:   "asc",
	})
	if err != nil || menus == nil {
		return nil
	}

	items := convertMenuItems(menus.Data)
	if len(items) == 0 {
		return nil
	}

	// Build tree using pointers so children are visible after attachment.
	type node struct {
		id       string
		parentID string
		item     *navItem
	}

	idMap := make(map[string]*node, len(items))
	for _, m := range items {
		id := strVal(m["ID"])
		n := &node{
			id:       id,
			parentID: strVal(m["Parent"]),
			item:     &navItem{Title: strVal(m["Title"]), URL: strVal(m["URL"]), Children: []navItem{}},
		}
		idMap[id] = n
	}

	// Attach children to parents.
	for _, n := range idMap {
		if n.parentID == "" {
			continue
		}
		if parent, ok := idMap[n.parentID]; ok {
			parent.item.Children = append(parent.item.Children, *n.item)
		}
	}

	// Collect roots in original sort_order, then attach children.
	var roots []navItem
	for _, m := range items {
		id := strVal(m["ID"])
		n, ok := idMap[id]
		if !ok {
			continue
		}
		if n.parentID == "" {
			roots = append(roots, *n.item)
		} else if _, parentExists := idMap[n.parentID]; !parentExists {
			roots = append(roots, *n.item)
		}
	}

	return roots
}

func convertMenuItems(data interface{}) []map[string]interface{} {
	var raw []map[string]interface{}
	switch v := data.(type) {
	case []map[string]interface{}:
		raw = v
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				raw = append(raw, m)
			}
		}
	}

	result := make([]map[string]interface{}, len(raw))
	for i, row := range raw {
		result[i] = map[string]interface{}{
			"ID":     strVal(row["id"]),
			"Title":  strVal(row["title"]),
			"URL":    strVal(row["url"]),
			"Parent": strVal(row["parent"]),
		}
	}
	return result
}

func (h *frontendHandler) render(w http.ResponseWriter, r *http.Request, templateName string, data map[string]interface{}) {
	data["Site"] = h.siteData()
	data["NavMenu"] = h.menuData(r.Context())

	html, err := h.themeSvc.Render(r.Context(), templateName, data)
	if err != nil {
		h.logger.Error("Template render error", "template", templateName, "error", err)

		if templateName != "404.html" {
			data404 := map[string]interface{}{
				"Site":  h.siteData(),
				"Title": "Not Found",
			}
			html404, err404 := h.themeSvc.Render(r.Context(), "404.html", data404)
			if err404 == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(html404))
				return
			}
		}

		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func (h *frontendHandler) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	perPage := 10
	if h.config != nil {
		pp := h.config.GetInt("theme.posts_per_page")
		if pp > 0 {
			perPage = pp
		}
	}

	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}

	posts, err := h.contentSvc.List(ctx, "post", &interfaces.ListQuery{
		Page:    page,
		PerPage: perPage,
		Sort:    "created_at",
		Order:   "desc",
	})

	data := map[string]interface{}{
		"Title": "Home",
		"Hero":  map[string]interface{}{"Title": "", "Subtitle": ""},
	}

	if err != nil {
		h.logger.Error("Failed to fetch posts for homepage", "error", err)
	} else if posts != nil {
		data["Posts"] = convertPosts(posts.Data)
		data["Pagination"] = buildPagination(posts.Meta, page)
	}

	h.render(w, r, "index.html", data)
}

func (h *frontendHandler) handlePosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	perPage := 10
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}

	posts, err := h.contentSvc.List(ctx, "post", &interfaces.ListQuery{
		Page:    page,
		PerPage: perPage,
		Sort:    "created_at",
		Order:   "desc",
	})

	data := map[string]interface{}{"Title": "All Posts"}

	if err != nil {
		h.logger.Error("Failed to fetch posts", "error", err)
	} else if posts != nil {
		data["Posts"] = convertPosts(posts.Data)
		data["Pagination"] = buildPagination(posts.Meta, page)
	}

	h.render(w, r, "posts.html", data)
}

func (h *frontendHandler) handlePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		h.render404(w, r)
		return
	}

	posts, err := h.contentSvc.List(ctx, "post", &interfaces.ListQuery{
		PerPage: 1,
		Filters: map[string]interface{}{"slug": slug},
	})

	items := convertPosts(posts.Data)
	if err != nil || len(items) == 0 {
		h.render404(w, r)
		return
	}

	postMap := items[0]
	data := map[string]interface{}{
		"Title": strVal(postMap["Title"]),
		"Post":  postMap,
	}

	h.render(w, r, "post.html", data)
}

func (h *frontendHandler) handlePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		h.render404(w, r)
		return
	}

	pages, err := h.contentSvc.List(ctx, "page", &interfaces.ListQuery{
		PerPage: 1,
		Filters: map[string]interface{}{"slug": slug},
	})

	items := convertPosts(pages.Data)
	if err != nil || len(items) == 0 {
		h.render404(w, r)
		return
	}

	pageMap := items[0]
	data := map[string]interface{}{
		"Title": strVal(pageMap["Title"]),
		"Page":  pageMap,
	}

	h.render(w, r, "page.html", data)
}

func (h *frontendHandler) handleArchive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	perPage := 20
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}

	posts, err := h.contentSvc.List(ctx, "post", &interfaces.ListQuery{
		Page:    page,
		PerPage: perPage,
		Sort:    "created_at",
		Order:   "desc",
	})

	data := map[string]interface{}{
		"Title":    "Archive",
		"Subtitle": "All published posts",
	}

	if err == nil && posts != nil {
		data["Posts"] = convertPosts(posts.Data)
		data["Pagination"] = buildPagination(posts.Meta, page)
	}

	h.render(w, r, "archive.html", data)
}

func (h *frontendHandler) handleAbout(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title": "About",
		"Page": map[string]interface{}{
			"Title": "About",
			"Body":  "<p>Welcome to our site. This page is powered by ARoute CMS.</p>",
		},
	}
	h.render(w, r, "page.html", data)
}

func (h *frontendHandler) render404(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Site":  h.siteData(),
		"Title": "Not Found",
	}

	html, err := h.themeSvc.Render(r.Context(), "404.html", data)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(html))
}

func (h *frontendHandler) serveThemeAsset(w http.ResponseWriter, r *http.Request) {
	themeName := chi.URLParam(r, "theme")
	if themeName == "" {
		http.NotFound(w, r)
		return
	}

	assetPath := chi.URLParam(r, "*")
	if assetPath == "" {
		http.NotFound(w, r)
		return
	}

	filePath := filepath.Join(h.themesDir, themeName, "assets", filepath.Clean("/"+assetPath))

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	themesBase, _ := filepath.Abs(h.themesDir)
	if absPath == themesBase || len(absPath) <= len(themesBase) || absPath[:len(themesBase)+1] != themesBase+string(filepath.Separator) {
		http.NotFound(w, r)
		return
	}

	http.ServeFile(w, r, filePath)
}

// convertPosts converts raw DB rows into template-friendly maps with PascalCase keys.
func convertPosts(data interface{}) []map[string]interface{} {
	var raw []map[string]interface{}
	switch v := data.(type) {
	case []map[string]interface{}:
		raw = v
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				raw = append(raw, m)
			}
		}
	}

	result := make([]map[string]interface{}, len(raw))
	for i, row := range raw {
		result[i] = map[string]interface{}{
			"Title":   strVal(row["title"]),
			"Slug":    strVal(row["slug"]),
			"Body":    strVal(row["body"]),
			"Excerpt": strVal(row["excerpt"]),
			"Author":  strVal(row["author_id"]),
			"Date":    formatDateField(row["created_at"]),
			"Tags":    nil,
		}
	}
	return result
}

func formatDateField(v interface{}) string {
	s := strVal(v)
	if s == "" {
		return ""
	}
	// Try parsing common date formats
	for _, layout := range []string{
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return s
}

func strVal(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func buildPagination(meta interfaces.PageMeta, currentPage int) map[string]interface{} {
	return map[string]interface{}{
		"CurrentPage": meta.Page,
		"TotalPages":  meta.TotalPages,
		"HasPrev":     meta.HasPrev,
		"HasNext":     meta.HasNext,
		"PrevPage":    meta.Page - 1,
		"NextPage":    meta.Page + 1,
		"Total":       meta.Total,
	}
}
