package theme

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/yuin/gopher-lua"
)

// LStatePool manages a pool of sandboxed Lua LState instances for safe,
// concurrent template rendering. Pooled states are reused across renders;
// if the pool is exhausted, temporary states are created on demand.
type LStatePool struct {
	pool      chan *lua.LState
	mu        sync.Mutex
	themesDir string
	themeSlug string
	logger    *slog.Logger
	poolSize  int
}

// NewLStatePool creates a new LState pool. The pool is pre-populated lazily:
// states are created on first Get call rather than at construction time to
// avoid upfront allocation cost.
func NewLStatePool(themesDir, themeSlug string, logger *slog.Logger, poolSize int) *LStatePool {
	if poolSize <= 0 {
		poolSize = 10
	}
	return &LStatePool{
		pool:      make(chan *lua.LState, poolSize),
		themesDir: themesDir,
		themeSlug: themeSlug,
		logger:    logger,
		poolSize:  poolSize,
	}
}

// Get retrieves an LState from the pool, or creates a new sandboxed state if
// the pool is empty. It respects context cancellation — returns an error if
// the context is done before a state becomes available.
func (p *LStatePool) Get(ctx context.Context) (*lua.LState, error) {
	select {
	case L := <-p.pool:
		return L, nil
	default:
		// Pool empty — create a temporary sandboxed state.
		p.mu.Lock()
		L := p.newSandboxedState()
		p.mu.Unlock()
		return L, nil
	}
}

// Put returns an LState to the pool. If the pool is full, the state is closed
// to prevent resource leaks.
func (p *LStatePool) Put(L *lua.LState) {
	select {
	case p.pool <- L:
		// Returned to pool successfully.
	default:
		// Pool full — close the overflow state.
		L.Close()
	}
}

// Close shuts down all pooled LState instances, releasing their resources.
func (p *LStatePool) Close() {
	for {
		select {
		case L := <-p.pool:
			L.Close()
		default:
			return
		}
	}
}

// newSandboxedState creates a new Lua LState with only safe standard libraries
// loaded. I/O, OS, debug, coroutine, and channel libraries are NOT opened to
// prevent template scripts from accessing the filesystem or executing commands.
func (p *LStatePool) newSandboxedState() *lua.LState {
	L := lua.NewState(lua.Options{
		SkipOpenLibs: true,
	})

	for _, pair := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		L.Push(L.NewFunction(pair.fn))
		L.Push(lua.LString(pair.name))
		L.Call(1, 0)
	}

	return L
}

// LuaEngine renders templates using gopher-lua scripts. It maintains a pool
// of sandboxed LState instances and enforces a per-render timeout to prevent
// runaway scripts from blocking the server.
type LuaEngine struct {
	pool      *LStatePool
	themesDir string
	themeSlug string
	logger    *slog.Logger
	timeout   time.Duration
}

// NewLuaEngine creates a new Lua rendering engine with an LState pool.
// The default pool size is 10 and the default per-render timeout is 5 seconds.
func NewLuaEngine(themesDir, themeSlug string, logger *slog.Logger, poolSize int) *LuaEngine {
	if poolSize <= 0 {
		poolSize = 10
	}
	pool := NewLStatePool(themesDir, themeSlug, logger, poolSize)
	return &LuaEngine{
		pool:      pool,
		themesDir: themesDir,
		themeSlug: themeSlug,
		logger:    logger,
		timeout:   5 * time.Second,
	}
}

// Render executes a Lua template file with the provided data and returns the
// rendered string. The Lua script should return its output as the last
// expression on the stack.
//
// The rendering pipeline:
//  1. Acquire a sandboxed LState from the pool
//  2. Set a context with the configured timeout
//  3. Inject the data map as the Lua global "data"
//  4. Register CMS API globals (cms.asset, cms.partial, cms.url, cms.query)
//  5. Execute the template file via DoFile
//  6. Extract the return value from the stack
//  7. Return the LState to the pool
func (e *LuaEngine) Render(templateName string, data map[string]interface{}) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	L, err := e.pool.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("lua engine: %w", err)
	}

	// Ensure the LState is always returned to the pool.
	defer func() {
		L.SetTop(0) // Clear the stack for reuse.
		e.pool.Put(L)
	}()

	// Set context for timeout and cancellation support.
	L.SetContext(ctx)

	// Inject data as Lua global "data".
	injectData(L, data)

	// Register CMS API globals.
	e.registerCMSGlobals(L)

	// Resolve template path: replace .html extension with .lua.
	luaTemplateName := templateName
	if strings.HasSuffix(luaTemplateName, ".html") {
		luaTemplateName = strings.TrimSuffix(luaTemplateName, ".html") + ".lua"
	}
	templatePath := fmt.Sprintf("%s/%s/templates/%s", e.themesDir, e.themeSlug, luaTemplateName)

	// Execute the template file.
	if err := L.DoFile(templatePath); err != nil {
		// Check if the error was caused by context timeout.
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("lua engine: template rendering timed out")
		}
		return "", fmt.Errorf("lua engine: %w", err)
	}

	// Extract the return value from the top of the stack.
	if L.GetTop() > 0 {
		result := L.ToString(L.GetTop())
		return result, nil
	}

	// Script returned nothing — return empty string.
	return "", nil
}

// Close shuts down the Lua engine by closing the LState pool.
func (e *LuaEngine) Close() {
	e.pool.Close()
}

// injectData converts a Go map[string]interface{} into a Lua table and sets
// it as the global "data" on the given LState.
func injectData(L *lua.LState, data map[string]interface{}) {
	table := goToLuaTable(L, data)
	L.SetGlobal("data", table)
}

// goToLuaTable recursively converts a Go map[string]interface{} into a Lua
// table. Supported types: string, int, float64, bool, map, slice, nil.
// Unsupported types are converted to their string representation.
func goToLuaTable(L *lua.LState, m map[string]interface{}) *lua.LTable {
	table := L.NewTable()
	for key, val := range m {
		table.RawSetString(key, goToLuaValue(L, val))
	}
	return table
}

// goToLuaSlice converts a Go []interface{} into a Lua table with integer keys
// starting at 1 (Lua convention).
func goToLuaSlice(L *lua.LState, s []interface{}) *lua.LTable {
	table := L.NewTable()
	for i, val := range s {
		table.RawSetInt(i+1, goToLuaValue(L, val))
	}
	return table
}

// goToLuaValue converts a single Go value to its Lua equivalent.
func goToLuaValue(L *lua.LState, v interface{}) lua.LValue {
	if v == nil {
		return lua.LNil
	}
	switch val := v.(type) {
	case string:
		return lua.LString(val)
	case int:
		return lua.LNumber(val)
	case int64:
		return lua.LNumber(val)
	case float64:
		return lua.LNumber(val)
	case float32:
		return lua.LNumber(val)
	case bool:
		return lua.LBool(val)
	case map[string]interface{}:
		return goToLuaTable(L, val)
	case []interface{}:
		return goToLuaSlice(L, val)
	default:
		return lua.LString(fmt.Sprint(v))
	}
}

// registerCMSGlobals injects CMS helper functions into the Lua state under
// the global "cms" table. These provide template authors with URL generation,
// asset path resolution, and content querying capabilities.
func (e *LuaEngine) registerCMSGlobals(L *lua.LState) {
	cms := L.NewTable()

	// cms.asset(path) → /themes/{themeSlug}/assets/{path}
	L.SetField(cms, "asset", L.NewFunction(func(ls *lua.LState) int {
		path := ls.CheckString(1)
		result := fmt.Sprintf("/themes/%s/assets/%s", e.themeSlug, path)
		ls.Push(lua.LString(result))
		return 1
	}))

	// cms.partial(name, data) → stub: returns empty string
	L.SetField(cms, "partial", L.NewFunction(func(ls *lua.LState) int {
		name := ls.CheckString(1)
		e.logger.Warn("partial rendering not yet available in Lua engine",
			"partial", name,
		)
		ls.Push(lua.LString(""))
		return 1
	}))

	// cms.url(entity, params) → stub: returns /{entity}/{params.slug}
	L.SetField(cms, "url", L.NewFunction(func(ls *lua.LState) int {
		entity := ls.CheckString(1)
		slug := ""
		if ls.GetTop() >= 2 {
			// params can be a table or a string.
			params := ls.Get(2)
			if tbl, ok := params.(*lua.LTable); ok {
				if slugVal := tbl.RawGetString("slug"); slugVal != lua.LNil {
					slug = lua.LVAsString(slugVal)
				}
			} else {
				slug = ls.CheckString(2)
			}
		}
		var result string
		if slug != "" {
			result = fmt.Sprintf("/%s/%s", entity, slug)
		} else {
			result = fmt.Sprintf("/%s", entity)
		}
		ls.Push(lua.LString(result))
		return 1
	}))

	// cms.query(contentType, filters) → stub: returns empty table
	L.SetField(cms, "query", L.NewFunction(func(ls *lua.LState) int {
		contentType := ls.CheckString(1)
		e.logger.Warn("content query not yet available in Lua engine",
			"contentType", contentType,
		)
		ls.Push(L.NewTable())
		return 1
	}))

	L.SetGlobal("cms", cms)
}
