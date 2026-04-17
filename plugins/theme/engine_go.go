// Package theme provides the theme engine plugin for ARoute CMS.
package theme

import (
	"bytes"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// GoTemplateEngine is a thread-safe Go html/template rendering engine with
// built-in functions and fallback template resolution.
type GoTemplateEngine struct {
	mu        sync.RWMutex
	themesDir string
	themeSlug string
	templates *template.Template
	funcMap   template.FuncMap
	logger    *slog.Logger
}

// NewGoTemplateEngine creates a new GoTemplateEngine, initializes built-in
// template functions, and loads templates from the theme directory.
func NewGoTemplateEngine(themesDir, themeSlug string, logger *slog.Logger) *GoTemplateEngine {
	e := &GoTemplateEngine{
		themesDir: themesDir,
		themeSlug: themeSlug,
		funcMap:   newBuiltinFuncMap(themeSlug),
		logger:    logger,
	}

	if err := e.LoadTemplates(); err != nil {
		e.logger.Error("failed to load templates", "theme", themeSlug, "error", err)
	}

	return e
}

func newBuiltinFuncMap(themeSlug string) template.FuncMap {
	return template.FuncMap{
		"formatDate": formatDate,
		"truncate":   truncate,
		"slugify":    slugify,
		"asset": func(path string) string {
			return "/themes/" + themeSlug + "/assets/" + path
		},
		"i18n": func(key string, _ ...interface{}) string {
			return key
		},
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"markdown": markdown,
		"raw": func(s string) template.HTML {
			return template.HTML(s)
		},
		"now": func() time.Time {
			return time.Now()
		},
		"year": func() int {
			return time.Now().Year()
		},
		"dict": dict,
		"join": join,
		"default": func(def interface{}, val interface{}) interface{} {
			if val == nil || val == "" || val == 0 || val == false {
				return def
			}
			return val
		},
	}
}

// LoadTemplates parses all .html template files from the active theme directory.
// It looks for templates in the theme's templates/ subdirectory, including
// layouts/, partials/, and root-level templates.
func (e *GoTemplateEngine) LoadTemplates() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	templatesDir := filepath.Join(e.themesDir, e.themeSlug, "templates")

	info, err := os.Stat(templatesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("templates directory %q does not exist", templatesDir)
		}
		return fmt.Errorf("stat templates directory %q: %w", templatesDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", templatesDir)
	}

	tmpl := template.New("").Funcs(e.funcMap)

	globPatterns := []string{
		filepath.Join(templatesDir, "*.html"),
		filepath.Join(templatesDir, "layouts", "*.html"),
		filepath.Join(templatesDir, "partials", "*.html"),
	}

	loaded := 0
	for _, pattern := range globPatterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("glob pattern %q: %w", pattern, err)
		}
		if len(matches) == 0 {
			continue
		}

		tmpl, err = tmpl.ParseFiles(matches...)
		if err != nil {
			return fmt.Errorf("parse templates from %q: %w", pattern, err)
		}
		loaded += len(matches)
	}

	if loaded == 0 {
		return fmt.Errorf("no .html templates found in %q", templatesDir)
	}

	e.templates = tmpl
	e.logger.Info("templates loaded", "theme", e.themeSlug, "count", loaded)

	return nil
}

func (e *GoTemplateEngine) Reload() error {
	return e.LoadTemplates()
}

// Render executes a named template with the given data and returns the
// rendered string. It supports template fallback resolution:
// "post.html" → "single.html" → "index.html".
func (e *GoTemplateEngine) Render(templateName string, data map[string]interface{}) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.templates == nil {
		return "", fmt.Errorf("no templates loaded for theme %q", e.themeSlug)
	}

	safeName := filepath.Base(templateName)
	if safeName != templateName {
		return "", fmt.Errorf("invalid template name %q: path separators not allowed", templateName)
	}

	name := e.resolveTemplateName(safeName)
	if name == "" {
		return "", fmt.Errorf("template %q not found in theme %q and no fallback available", templateName, e.themeSlug)
	}

	var buf bytes.Buffer
	if err := e.templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("execute template %q: %w", name, err)
	}

	return buf.String(), nil
}

func (e *GoTemplateEngine) resolveTemplateName(name string) string {
	candidates := []string{name}

	if name != "index.html" {
		candidates = append(candidates, "single.html")
	}
	if name != "index.html" {
		candidates = append(candidates, "index.html")
	}

	for _, candidate := range candidates {
		if e.hasTemplateUnlocked(candidate) {
			return candidate
		}
	}

	return ""
}

// HasTemplate checks if a template with the given name exists.
func (e *GoTemplateEngine) HasTemplate(name string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.hasTemplateUnlocked(name)
}

func (e *GoTemplateEngine) hasTemplateUnlocked(name string) bool {
	if e.templates == nil {
		return false
	}
	for _, t := range e.templates.Templates() {
		if t.Name() == name {
			return true
		}
	}
	return false
}

func formatDate(t interface{}, layout string) string {
	if t == nil {
		return ""
	}

	var tt time.Time
	switch v := t.(type) {
	case time.Time:
		tt = v
	case string:
		if v == "" {
			return ""
		}
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			parsed, err = time.Parse("2006-01-02", v)
			if err != nil {
				return v
			}
		}
		tt = parsed
	default:
		return fmt.Sprintf("%v", t)
	}

	return tt.Format(layout)
}

func truncate(s string, maxLen int) string {
	if maxLen < 0 {
		return s
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

var slugifyNonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = slugifyNonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func markdown(s string) template.HTML {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")

	boldRegex := regexp.MustCompile(`\*\*(.+?)\*\*`)
	s = boldRegex.ReplaceAllString(s, "<strong>$1</strong>")

	italicRegex := regexp.MustCompile(`\*(.+?)\*`)
	s = italicRegex.ReplaceAllString(s, "<em>$1</em>")

	s = strings.ReplaceAll(s, "\n\n", "</p><p>")
	s = strings.ReplaceAll(s, "\n", "<br>")

	s = "<p>" + s + "</p>"

	return template.HTML(s)
}

func dict(keysAndValues ...interface{}) (map[string]interface{}, error) {
	if len(keysAndValues)%2 != 0 {
		return nil, fmt.Errorf("dict requires even number of arguments, got %d", len(keysAndValues))
	}

	m := make(map[string]interface{}, len(keysAndValues)/2)
	for i := 0; i < len(keysAndValues); i += 2 {
		key, ok := keysAndValues[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict key at position %d must be string, got %T", i, keysAndValues[i])
		}
		m[key] = keysAndValues[i+1]
	}

	return m, nil
}

func join(sep string, items interface{}) (string, error) {
	switch v := items.(type) {
	case []string:
		return strings.Join(v, sep), nil
	case []interface{}:
		strs := make([]string, len(v))
		for i, item := range v {
			strs[i] = fmt.Sprintf("%v", item)
		}
		return strings.Join(strs, sep), nil
	default:
		return "", fmt.Errorf("join expects []string or []interface{}, got %T", items)
	}
}
