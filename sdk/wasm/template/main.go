//go:build tinygo

// Package main implements a WebAssembly plugin template for Aroute CMS using TinyGo.
//
// This template demonstrates the Wasm plugin calling convention:
//   - Exported functions: init, start, stop, manifest
//   - Host function imports for accessing Core services via the "cms" module
//   - Linear memory for JSON data exchange across the Wasm boundary
//
// Build with TinyGo:
//
//	tinygo build -o build/plugin.wasm -target=wasi -no-debug -scheduler=none -gc=leaking .
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
// The runtime looks for exact names: "init", "start", "stop", "manifest".

//go:export buffer_ptr
func bufferPtr() *byte {
	return &buffer[0]
}

//go:export buffer_len
func bufferLen() int32 {
	return int32(len(buffer))
}

//go:export manifest
func manifest_() (int32, int32) {
	resetBuffer()
	m := `{"name":"` + PluginName + `","version":"` + PluginVersion + `","description":"` + PluginDescription + `","author":"` + PluginAuthor + `","engine":"wasm"}`
	return writeString(m)
}

//go:export init
func init_() int32 {
	// Called once during plugin startup.
	// Use host functions to register services and subscribe to events.
	//
	// Example:
	//   cms_event_subscribe("content.*.created", "on_content_created")

	return 0 // 0 = success
}

//go:export start
func start_() int32 {
	// Called after all plugins are initialized.
	// Start background processing or long-running tasks here.

	return 0 // 0 = success
}

//go:export stop
func stop_() int32 {
	// Called during graceful shutdown.
	// Clean up resources here.

	return 0 // 0 = success
}

// --- Event Handlers ---
// These functions can be registered as callbacks via cms_event_subscribe.
// When an event fires, the host calls the named exported function with event
// data written to the shared memory buffer.

//go:export on_content_created
func onContentCreated(jsonOffset, jsonLen int32) {
	data := readString(jsonOffset, jsonLen)
	_ = data // process the event data JSON

	// Publish a log event via the host
	msg := `{"level":"info","message":"content created","data":` + data + `}`
	msgOffset, msgLen := writeString(msg)
	cms_event_publish(msgOffset, msgLen, 0, 0)
}

// --- Host Function Imports ---
// These functions are provided by the Aroute Wasm runtime via the "cms" host module.
// They allow the Wasm plugin to interact with Core services.

// cms_service_has checks if a service is available in the container.
// serviceID is a numeric index into the service registry.
// Returns 1 if available, 0 otherwise.
//
//go:wasmimport cms service_has
func cms_service_has(serviceID uint32) uint32

// cms_service_get retrieves a service handle by ID.
//
//go:wasmimport cms service_get
func cms_service_get(serviceID uint32) uint32

// cms_event_subscribe registers an event handler in the Wasm module.
// When the event fires, the host calls the named callback function.
//
// Parameters:
//   - topicPtr, topicLen: event topic pattern (e.g., "content.*.created")
//   - callbackPtr, callbackLen: name of exported function to call
//
// Returns: handler ID
//
//go:wasmimport cms event_subscribe
func cms_event_subscribe(topicPtr, topicLen uint32, callbackPtr, callbackLen uint32) uint32

// cms_event_publish emits an event to the EventBus.
//
// Parameters:
//   - topicPtr, topicLen: event topic string
//   - dataPtr, dataLen: JSON-encoded event payload
//
//go:wasmimport cms event_publish
func cms_event_publish(topicPtr, topicLen, dataPtr, dataLen uint32)

// host_log writes a log message from the plugin to the host logger.
//
// Parameters:
//   - msgOffset, msgLen: JSON log message in shared buffer
//
//go:wasmimport cms host_log
func host_log(msgOffset, msgLen int32)

// cms_memory_alloc allocates memory in the Wasm module for host-to-Wasm data transfer.
//
//go:wasmimport cms memory_alloc
func cms_memory_alloc(size uint32) uint32

// cms_memory_free frees previously allocated memory.
//
//go:wasmimport cms memory_free
func cms_memory_free(ptr uint32)

// Required by TinyGo/WASI.
func main() {}

// keep alive — prevent unused variable optimization on buffer
var _ = unsafe.Pointer(&buffer[0])
