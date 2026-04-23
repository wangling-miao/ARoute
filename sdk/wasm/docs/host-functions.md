# Wasm Host Function Bindings

This document defines the calling convention for host functions exposed to Wasm plugins
by the Aroute CMS runtime (wazero engine).

## Overview

Wasm plugins run inside a sandboxed wazero runtime. They communicate with the Core
microkernel through **host function imports** (Wasm -> Host) and **exported callbacks**
(Host -> Wasm). All data exchange uses a shared linear memory buffer with JSON serialization.

Host functions are imported from the `"cms"` module using `//go:wasmimport cms`.

## Memory Layout

```
+-------------------------------------------------+
| Shared Buffer (64KB default)                     |
| +-----------------------------------------------+|
| Request/Response JSON data                       ||
| (read/write by both host and Wasm module)        ||
| +-----------------------------------------------+|
+-------------------------------------------------+
```

- Buffer pointer: obtained via the exported `buffer_ptr()` function
- Buffer length: obtained via the exported `buffer_len()` function
- Parameters passed as (offset, length) pairs into the buffer
- Host validates all offsets against buffer bounds before reading

## Calling Convention

### Parameters

All host functions receive parameters as `(ptr, len)` pairs pointing to data in the
shared buffer. For simple integer handles, the value is passed directly as `uint32`.

### Return Values

- **Integer returns**: `0` = failure/false, non-zero = success/true or handle ID
- **Error returns**: JSON written to buffer: `{"error": "message", "code": 404}`

### Error Codes

| Code | Meaning          |
|------|------------------|
| 0    | Success          |
| 404  | Not found        |
| 401  | Unauthorized     |
| 403  | Forbidden        |
| 400  | Bad request      |
| 500  | Internal error   |
| 507  | Buffer overflow  |

---

## Currently Available Host Functions

All functions are imported via `//go:wasmimport cms <name>`.

### Service Access

#### `service_has(serviceID uint32) uint32`

Check if a service is available in the runtime. Returns `1` if the service exists,
`0` otherwise.

```go
//go:wasmimport cms service_has
func serviceHas(id uint32) uint32
```

**Parameters**:
- `serviceID` — Numeric service identifier

**Returns**: `1` = available, `0` = not available

#### `service_get(serviceID uint32) uint32`

Get a service handle by its numeric ID. Returns a non-zero handle on success,
`0` if the service is not available.

```go
//go:wasmimport cms service_get
func serviceGet(id uint32) uint32
```

**Parameters**:
- `serviceID` — Numeric service identifier

**Returns**: Non-zero service handle, or `0` on failure

---

### EventBus

#### `event_subscribe(topicPtr, topicLen, callbackPtr, callbackLen uint32) uint32`

Subscribe to an EventBus topic. When an event matching the topic fires, the host
invokes the named callback function in the Wasm module.

```go
//go:wasmimport cms event_subscribe
func eventSubscribe(topicPtr, topicLen, callbackPtr, callbackLen uint32) uint32
```

**Parameters**:
- `topicPtr, topicLen` — Topic pattern string in buffer (e.g., `"content.*.created"`)
- `callbackPtr, callbackLen` — Name of exported Wasm function to call

**Returns**: Handler ID (non-zero on success, `0` on failure)

**Callback signature** (exported from your Wasm module):
```go
//go:export my_callback
func myCallback(jsonOffset, jsonLen int32) {
    data := readString(jsonOffset, jsonLen)
    // data contains the event payload as JSON
}
```

**Event payload JSON** (passed to callback):
```json
{
  "topic": "content.post.created",
  "data": {
    "id": "abc123",
    "content_type": "post"
  }
}
```

#### `event_publish(topicPtr, topicLen, dataPtr, dataLen uint32)`

Emit an event to the EventBus. Other plugins subscribed to the topic will receive it.

```go
//go:wasmimport cms event_publish
func eventPublish(topicPtr, topicLen, dataPtr, dataLen uint32)
```

**Parameters**:
- `topicPtr, topicLen` — Topic string in buffer
- `dataPtr, dataLen` — JSON event data in buffer

---

### Logging

#### `host_log(msgPtr, msgLen uint32)`

Write a log message to the host logger.

```go
//go:wasmimport cms host_log
func hostLog(msgPtr, msgLen uint32)
```

**Parameters**:
- `msgPtr, msgLen` — Log message string in buffer

---

### Memory Management

#### `memory_alloc(size uint32) uint32`

Allocate memory within the Wasm module's linear memory. Returns a pointer to the
allocated block.

```go
//go:wasmimport cms memory_alloc
func memoryAlloc(size uint32) uint32
```

**Parameters**:
- `size` — Number of bytes to allocate

**Returns**: Pointer to allocated memory block

#### `memory_free(ptr uint32)`

Free a previously allocated memory block.

```go
//go:wasmimport cms memory_free
func memoryFree(ptr uint32)
```

**Parameters**:
- `ptr` — Pointer to the memory block to free

---

## Planned Host Functions (Not Yet Available)

The following functions are planned for future releases. Do not use them yet.

| Function | Purpose |
|---|---|
| `host_content_get` | Retrieve content items by type and ID |
| `host_content_list` | List content with filters and pagination |
| `host_db_query` | Execute database queries |
| `host_db_exec` | Execute database statements (INSERT/UPDATE/DELETE) |
| `host_cache_get` | Get cached value by key |
| `host_cache_set` | Set cached value with TTL |
| `host_cache_delete` | Delete cached key |

---

## Exported Functions (Host calls into Wasm)

The runtime expects the Wasm module to export these functions:

### `manifest() (uint32, uint32)`

Called during plugin discovery. Returns plugin metadata as JSON (offset, length).

```go
//go:export manifest
func manifest() (uint32, uint32)
```

**Response** (written to buffer):
```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Plugin description",
  "author": "Author Name"
}
```

### `init() uint32`

Called to initialize the plugin. Use this to set up internal state and subscribe to events.

```go
//go:export init
func init() uint32
```

**Returns**: `0` on success, non-zero on error

### `start() uint32`

Called after all plugins have been initialized. Use this to start active work.

```go
//go:export start
func start() uint32
```

**Returns**: `0` on success, non-zero on error

### `stop() uint32`

Called during graceful shutdown. Clean up resources here.

```go
//go:export stop
func stop() uint32
```

**Returns**: `0` on success, non-zero on error

---

## Memory Safety

1. The host validates all (offset, length) pairs against the buffer size before access
2. If a Wasm plugin writes beyond buffer bounds, the host terminates the instance with error code 507
3. Each plugin instance gets its own isolated memory space
4. The host never reads beyond the specified length
5. Concurrent access is serialized -- only one host call is active at a time per instance

## Version Compatibility

| SDK Version | Host Function Set                        |
|-------------|------------------------------------------|
| 1.0.0       | service_has, service_get, event_subscribe, event_publish, host_log, memory_alloc, memory_free |

New host functions may be added in minor versions without breaking existing plugins.
Removed or changed host functions require a major version bump.
