//go:build tinygo

// Package main implements a WebAssembly plugin template for Aroute CMS using TinyGo.
//
// This template demonstrates the Wasm plugin calling convention:
//   - Exported functions: plugin_init, plugin_start, plugin_stop, plugin_manifest
//   - Host function imports for accessing Core services
//   - Linear memory for JSON data exchange across the Wasm boundary
//
// Build with TinyGo:
//
//	tinygo build -o build/plugin.wasm -target=wasi .
//
// See the Makefile for build automation.
package main

import (
	"unsafe"
)

// Plugin metadata — edit these constants for your plugin.
const (
	PluginName        = "wasm-example"
	PluginVersion     = "1.0.0"
	PluginDescription = "Example Wasm plugin for Aroute CMS"
	PluginAuthor      = "Your Name"
)

// Shared memory buffer size for host/Wasm data exchange (64KB).
const bufferSize = 65536

// buffer is the shared memory region used for JSON data exchange
// between the Wasm module and the host runtime.
//
//go:export buffer_ptr
func bufferPtr() *byte {
	return &buffer[0]
}

//go:export buffer_len
func bufferLen() int32 {
	return int32(len(buffer))
}

var buffer [bufferSize]byte

// bufferPos tracks the current write position in the buffer.
var bufferPos int

// resetBuffer clears the shared memory buffer.
func resetBuffer() {
	for i := range buffer {
		buffer[i] = 0
	}
	bufferPos = 0
}

// writeString writes a string to the shared buffer and returns its offset and length.
func writeString(s string) (int32, int32) {
	offset := bufferPos
	copy(buffer[bufferPos:], s)
	bufferPos += len(s)
	return int32(offset), int32(len(s))
}

// readString reads a string from the buffer at the given offset and length.
func readString(offset, length int32) string {
	if offset < 0 || length < 0 || int(offset+length) > len(buffer) {
		return ""
	}
	return string(buffer[offset : offset+length])
}

// --- Exported Plugin Lifecycle Functions ---
// These functions are called by the Aroute Wasm engine dispatcher.

//go:export plugin_manifest
func pluginManifest() (int32, int32) {
	resetBuffer()
	manifest := `{"name":"` + PluginName + `","version":"` + PluginVersion + `","description":"` + PluginDescription + `","author":"` + PluginAuthor + `","engine":"wasm"}`
	return writeString(manifest)
}

//go:export plugin_init
func pluginInit() int32 {
	// Called once during plugin startup.
	// Use host functions to register services and subscribe to events.
	//
	// Example:
	//   host_eventbus_subscribe("content.*.created", "on_content_created")

	return 0 // 0 = success
}

//go:export plugin_start
func pluginStart() int32 {
	// Called after all plugins are initialized.
	// Start background processing or long-running tasks here.

	return 0 // 0 = success
}

//go:export plugin_stop
func pluginStop() int32 {
	// Called during graceful shutdown.
	// Clean up resources here.

	return 0 // 0 = success
}

// --- Event Handlers ---
// These functions can be registered as callbacks via host_eventbus_subscribe.
// When an event fires, the host calls the named exported function with event
// data written to the shared memory buffer.

//go:export on_content_created
func onContentCreated(jsonOffset, jsonLen int32) {
	data := readString(jsonOffset, jsonLen)
	_ = data // process the event data JSON

	// Example: log the event via host function
	msg := `{"level":"info","message":"content created","data":` + data + `}`
	msgOffset, msgLen := writeString(msg)
	host_log(msgOffset, msgLen)
}

// --- Host Function Imports ---
// These functions are provided by the Aroute Wasm runtime (wazero engine).
// They allow the Wasm plugin to interact with Core services.

// host_log writes a log message from the plugin to the host logger.
//
// Parameters:
//   - msgOffset: offset into shared buffer where JSON log message starts
//   - msgLen: length of the log message
//
//go:wasmimport aroute host_log
func host_log(msgOffset, msgLen int32)

// host_content_get retrieves a content item by type and ID.
// Writes the JSON result back to the shared buffer.
//
// Parameters:
//   - typeOffset, typeLen: content type string in shared buffer
//   - idOffset, idLen: content ID string in shared buffer
//
// Returns:
//   - 0 on success, error code on failure
//
//go:wasmimport aroute host_content_get
func host_content_get(typeOffset, typeLen, idOffset, idLen int32) int32

// host_eventbus_subscribe registers an event handler in the Wasm module.
// When the event fires, the host calls the named callback function.
//
// Parameters:
//   - eventOffset, eventLen: event topic pattern (e.g., "content.*.created")
//   - callbackOffset, callbackLen: name of exported function to call
//
// Returns:
//   - handler ID offset and length written to shared buffer
//
//go:wasmimport aroute host_eventbus_subscribe
func host_eventbus_subscribe(eventOffset, eventLen, callbackOffset, callbackLen int32) int32

// host_db_query executes a database query via the host.
// Query SQL and args are passed as JSON in the shared buffer.
// Results are written back as JSON.
//
// Parameters:
//   - sqlOffset, sqlLen: SQL query string in shared buffer
//   - argsOffset, argsLen: JSON-encoded query arguments
//
// Returns:
//   - 0 on success, error code on failure
//
//go:wasmimport aroute host_db_query
func host_db_query(sqlOffset, sqlLen, argsOffset, argsLen int32) int32

// host_cache_get retrieves a cached value by key.
// The result is written as JSON to the shared buffer.
//
// Parameters:
//   - keyOffset, keyLen: cache key string in shared buffer
//
// Returns:
//   - 0 on success (value in buffer), error code on failure
//
//go:wasmimport aroute host_cache_get
func host_cache_get(keyOffset, keyLen int32) int32

// host_cache_set stores a value in the cache with TTL.
//
// Parameters:
//   - keyOffset, keyLen: cache key
//   - valOffset, valLen: JSON-encoded value
//   - ttlMs: time-to-live in milliseconds (0 for default)
//
// Returns:
//   - 0 on success, error code on failure
//
//go:wasmimport aroute host_cache_set
func host_cache_set(keyOffset, keyLen, valOffset, valLen, ttlMs int32) int32

// Required by TinyGo/WASI.
func main() {}

// keep alive — prevent unused variable optimization on buffer
var _ = unsafe.Pointer(&buffer[0])
