# Aroute Wasm Plugin Template

A TinyGo-based WebAssembly plugin template for Aroute CMS.

## Prerequisites

- [TinyGo](https://tinygo.org/getting-started/install/) >= 0.30

## Quick Start

1. Copy this template to your plugin directory:
   ```bash
   cp -r sdk/wasm/template/ plugins/my-wasm-plugin/
   ```

2. Edit `main.go` -- update the plugin metadata constants:
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
+------------------------------+
|       Aroute Host            |
|  +----------------------+    |
|  |   Wazero Runtime     |    |
|  |  +----------------+  |    |
|  |  |  Wasm Module   |  |    |
|  |  |                |  |    |
|  |  |  Imports <-----+--+----+  cms module:
|  |  |                |  |    |    service_has()
|  |  |  Exports ----->+--+----+    service_get()
|  |  |                |  |    |    event_subscribe()
|  |  +----------------+  |    |    event_publish()
|  +----------------------+    |    host_log()
+------------------------------+    memory_alloc()
                                    memory_free()

                                Exports called by host:
                                    manifest()
                                    init()
                                    start()
                                    stop()
```

## Plugin Interface

### Exported Functions (Wasm -> Host)

The host runtime calls these exported functions on your Wasm module:

| Function   | Called When             | Return        |
|------------|-------------------------|---------------|
| `manifest` | During plugin discovery | JSON ptr+len  |
| `init`     | Plugin initialization   | 0=ok          |
| `start`    | After all plugins init  | 0=ok          |
| `stop`     | Graceful shutdown       | 0=ok          |

### Host Functions (Host -> Wasm)

Imported from the `"cms"` module via `//go:wasmimport cms`:

| Function                                    | Purpose                          |
|---------------------------------------------|----------------------------------|
| `service_has(serviceID)`                    | Check if a service is available  |
| `service_get(serviceID)`                    | Get service handle by numeric ID |
| `event_subscribe(topic, callback)`          | Register event handler callback  |
| `event_publish(topic, data)`                | Emit event to EventBus           |
| `host_log(msgPtr, msgLen)`                  | Write to host logger             |
| `memory_alloc(size)`                        | Allocate memory in Wasm module   |
| `memory_free(ptr)`                          | Free allocated memory            |

### Data Exchange

All data passes through a shared memory buffer (64KB). Parameters are passed as
(offset, length) pairs pointing into this buffer. JSON is used for serialization.

## Writing Custom Event Handlers

1. Define an exported function with `(jsonOffset, jsonLen int32)` signature
2. Register it via `event_subscribe` in `init`

```go
//go:export on_custom_event
func onCustomEvent(jsonOffset, jsonLen int32) {
    data := readString(jsonOffset, jsonLen)
    // process data...
}

// In init:
// event_subscribe("my.topic", "on_custom_event")
```

## Limitations

- No goroutines (use `-scheduler=none`)
- Leaking GC only (`-gc=leaking`) -- manage memory carefully
- No network access -- use host functions for I/O
- 64KB shared buffer limit for data exchange
