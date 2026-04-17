package theme

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fastschema/qjs"
)

var reactChunkPattern = regexp.MustCompile(`(?i)</script`)

// ReactSSREngine renders templates using React server-side rendering
// via a pooled QuickJS runtime with bytecode caching and hydration support.
type ReactSSREngine struct {
	pool          *qjs.Pool
	themesDir     string
	themeSlug     string
	logger        *slog.Logger
	timeout       time.Duration
	bytecodeCache map[string][]byte
	mu            sync.RWMutex
	poolSize      int
}

// NewReactSSREngine creates a new React SSR engine with a QuickJS runtime pool.
// It precompiles all .js template files found in the theme's templates directory.
func NewReactSSREngine(themesDir, themeSlug string, logger *slog.Logger, poolSize int) *ReactSSREngine {
	if poolSize <= 0 {
		poolSize = 4
	}

	e := &ReactSSREngine{
		themesDir:     themesDir,
		themeSlug:     themeSlug,
		logger:        logger,
		timeout:       5 * time.Second,
		bytecodeCache: make(map[string][]byte),
		poolSize:      poolSize,
	}

	e.pool = qjs.NewPool(poolSize, qjs.Option{
		MaxExecutionTime: 5000,
		MaxStackSize:     10 * 1024 * 1024,
		MemoryLimit:      128 * 1024 * 1024,
	}, func(rt *qjs.Runtime) error {
		ctx := rt.Context()
		ctx.Global().SetPropertyStr("ENV", ctx.NewString("production"))
		return nil
	})

	if err := e.precompileTemplates(); err != nil {
		e.logger.Error("react ssr: failed to precompile templates",
			"theme", themeSlug,
			"error", err,
		)
	}

	return e
}

// precompileTemplates reads all .js files from the theme templates directory
// and compiles them to bytecode for faster subsequent evaluations.
func (e *ReactSSREngine) precompileTemplates() error {
	start := time.Now()
	templatesDir := filepath.Join(e.themesDir, e.themeSlug, "templates")

	patterns := []string{
		filepath.Join(templatesDir, "*.js"),
		filepath.Join(templatesDir, "**", "*.js"),
	}

	var allFiles []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("react ssr: glob pattern %q: %w", pattern, err)
		}
		allFiles = append(allFiles, matches...)
	}

	if len(allFiles) == 0 {
		e.logger.Info("react ssr: no .js templates found to precompile",
			"dir", templatesDir,
		)
		return nil
	}

	rt, err := e.pool.Get()
	if err != nil {
		return fmt.Errorf("react ssr: get runtime for precompilation: %w", err)
	}
	defer e.pool.Put(rt)

	ctx := rt.Context()

	compiled := 0
	for _, jsFile := range allFiles {
		content, err := os.ReadFile(jsFile)
		if err != nil {
			e.logger.Warn("react ssr: failed to read template file, skipping",
				"file", jsFile,
				"error", err,
			)
			continue
		}

		bytecode, err := ctx.Compile(jsFile, qjs.Code(string(content)))
		if err != nil {
			e.logger.Warn("react ssr: failed to compile template, skipping",
				"file", jsFile,
				"error", err,
			)
			continue
		}

		e.mu.Lock()
		e.bytecodeCache[jsFile] = bytecode
		e.mu.Unlock()
		compiled++
	}

	e.logger.Info("react ssr: templates precompiled",
		"theme", e.themeSlug,
		"compiled", compiled,
		"total", len(allFiles),
		"duration", time.Since(start).Round(time.Millisecond),
	)

	return nil
}

// Render evaluates a React SSR template with the provided data and returns
// the rendered HTML string with an embedded hydration script.
func (e *ReactSSREngine) Render(templateName string, data map[string]interface{}) (string, error) {
	rt, err := e.pool.Get()
	if err != nil {
		return "", fmt.Errorf("react ssr: get runtime: %w", err)
	}
	defer e.pool.Put(rt)

	ctx := rt.Context()

	// Serialize data to JSON and inject as global variable.
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("react ssr: marshal template data: %w", err)
	}
	jsonStr := string(jsonData)
	ctx.Global().SetPropertyStr("__AROUTE_DATA__", ctx.NewString(jsonStr))

	// Inject commonly used individual fields for convenience.
	e.injectDataField(ctx, data, "site")
	e.injectDataField(ctx, data, "page")
	e.injectDataField(ctx, data, "theme")

	// Resolve the .js file path from the template name.
	// Security: sanitize templateName to prevent path traversal.
	safeName := filepath.Base(templateName)
	if safeName != templateName {
		return "", fmt.Errorf("react ssr: invalid template name %q: path separators not allowed", templateName)
	}
	jsFileName := strings.TrimSuffix(safeName, filepath.Ext(safeName)) + ".js"
	jsFilePath := filepath.Join(e.themesDir, e.themeSlug, "templates", jsFileName)

	// Security: verify resolved path stays within the theme's templates directory.
	templatesDir := filepath.Join(e.themesDir, e.themeSlug, "templates")
	absPath, err := filepath.Abs(jsFilePath)
	if err != nil || !strings.HasPrefix(absPath, templatesDir+string(filepath.Separator)) {
		return "", fmt.Errorf("react ssr: template path escapes templates directory: %s", templateName)
	}

	// Determine source: prefer cached bytecode, fall back to file read.
	var result *qjs.Value

	e.mu.RLock()
	bytecode, hasBytecode := e.bytecodeCache[jsFilePath]
	e.mu.RUnlock()

	if hasBytecode {
		result, err = ctx.Eval(jsFilePath, qjs.Bytecode(bytecode))
	} else {
		content, readErr := os.ReadFile(jsFilePath)
		if readErr != nil {
			return "", fmt.Errorf("react ssr: read template %q: %w", jsFilePath, readErr)
		}
		result, err = ctx.Eval(jsFilePath, qjs.Code(string(content)))
	}

	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			return "", fmt.Errorf("react ssr: rendering timed out")
		}
		return "", fmt.Errorf("react ssr: evaluate template %q: %w", jsFilePath, err)
	}
	defer result.Free()

	html := result.String()

	// Build hydration script: embed serialized data so client-side JS can
	// hydrate without a separate fetch.
	// Security: json.Marshal escapes <, >, & to \uXXXX since Go 1.13.
	// Additionally escape any closing script tags (case-insensitive) and HTML comments.
	safeJSON := reactChunkPattern.ReplaceAllString(jsonStr, `<\\/script`)
	safeJSON = strings.ReplaceAll(safeJSON, "<!--", `<\\!--`)
	hydrationScript := fmt.Sprintf("<script>window.__AROUTE_DATA__ = %s;</script>", safeJSON)

	// Insert before </body> when present; otherwise append to the end.
	if idx := strings.LastIndex(html, "</body>"); idx != -1 {
		html = html[:idx] + hydrationScript + html[idx:]
	} else {
		html += hydrationScript
	}

	return html, nil
}

// injectDataField injects a single data key as a JSON global if present.
func (e *ReactSSREngine) injectDataField(ctx *qjs.Context, data map[string]interface{}, key string) {
	val, ok := data[key]
	if !ok {
		return
	}
	fieldJSON, err := json.Marshal(val)
	if err != nil {
		return
	}
	ctx.Global().SetPropertyStr("__AROUTE_"+strings.ToUpper(key)+"__", ctx.NewString(string(fieldJSON)))
}

// Close releases the QuickJS runtime pool resources.
func (e *ReactSSREngine) Close() error {
	if e.pool != nil {
		// Drain and close all runtimes in the pool channel.
		for i := 0; i < e.poolSize; i++ {
			rt, err := e.pool.Get()
			if err != nil {
				break
			}
			rt.Close()
		}
		e.pool = nil
	}
	return nil
}

// Reload clears the bytecode cache and recompiles all templates.
func (e *ReactSSREngine) Reload() error {
	e.mu.Lock()
	e.bytecodeCache = make(map[string][]byte)
	e.mu.Unlock()

	if err := e.precompileTemplates(); err != nil {
		return fmt.Errorf("react ssr: reload: %w", err)
	}

	e.logger.Info("react ssr: engine reloaded", "theme", e.themeSlug)
	return nil
}
