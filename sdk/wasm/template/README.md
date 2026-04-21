# Aroute Wasm Plugin Template

A TinyGo-based WebAssembly plugin template for Aroute CMS.

## Prerequisites

- [TinyGo](https://tinygo.org/getting-started/install/) >= 0.30

## Quick Start

1. Copy this template to your plugin directory:
   ```bash
   cp -r sdk/wasm/template/ plugins/my-wasm-plugin/
   ```

2. Edit `main.go` — update the plugin metadata constants:
   ```go
   const (
       PluginName        = "my-wasm-plugin"
       PluginVersion     = "1.0.0"
       PluginDescription = "My custom Wasm plugin"
       PluginAuthor      = "Your Name"
   )
   ```

3. Build the Wasm module:
   ```bash
   make build
   ```

4. Install into Aroute:
   ```bash
   aroute plugin install plugins/my-wasm-plugin/
   ```

## Architecture

```
┌──────────────────────────────┐
│       Aroute Host            │
│  ┌──────────────────────┐    │
│  │   Wazero Runtime     │    │
│  │  ┌────────────────┐  │    │
│  │  │  Wasm Module   │  │    │
│  │  │                │  │    │
│  │  │  Imports ◄─────┼──┼────┤  host_log()
│  │  │                │  │    │  host_content_get()
│  │  │  Exports ─────►├──┼────┤  plugin_init()
│  │  │                │  │    │  plugin_start()
│  │  └────────────────┘  │    │  plugin_stop()
│  └──────────────────────┘    │
└──────────────────────────────┘
```

## Plugin Interface

### Exported Functions (Wasm → Host)

| Function          | Called When            | Return     |
|-------------------|------------------------|------------|
| `plugin_manifest` | During plugin discovery | JSON ptr+len |
| `plugin_init`     | Plugin initialization  | 0=ok       |
| `plugin_start`    | After all plugins init | 0=ok       |
| `plugin_stop`     | Graceful shutdown      | 0=ok       |

### Host Functions (Host → Wasm)

| Function                    | Purpose                          |
|-----------------------------|----------------------------------|
| `host_log(msg, len)`        | Write to structured logger       |
| `host_content_get(type, id)`| Retrieve content by type+ID      |
| `host_eventbus_subscribe()` | Register event handler callback  |
| `host_db_query(sql, args)`  | Execute database query           |
| `host_cache_get(key)`       | Get cached value                 |
| `host_cache_set(key, val)`  | Set cached value with TTL        |

### Data Exchange

All data passes through a shared memory buffer (64KB). Parameters are passed as
(offset, length) pairs pointing into this buffer. JSON is used for serialization.

## Writing Custom Event Handlers

1. Define an exported function with `(jsonOffset, jsonLen int32)` signature
2. Register it via `host_eventbus_subscribe` in `plugin_init`

```go
//go:export on_custom_event
func onCustomEvent(jsonOffset, jsonLen int32) {
    data := readString(jsonOffset, jsonLen)
    // process data...
}

// In plugin_init:
// host_eventbus_subscribe("my.topic", "on_custom_event")
```

## Limitations

- No goroutines (use `-scheduler=none`)
- Leaking GC only (`-gc=leaking`) — manage memory carefully
- No network access — use host functions for I/O
- 64KB shared buffer limit for data exchange
