//go:build tinygo

// Package main implements an L3 Wasm example plugin for Aroute CMS.
//
// This plugin demonstrates medium-level interaction with the Core:
//   - Queries service availability via host functions
//   - Publishes lifecycle events to the EventBus
//   - Uses the shared memory buffer for data exchange
//
// Build with TinyGo:
//
//	tinygo build -o build/example.wasm -target=wasi -no-debug -scheduler=none -gc=leaking .
package main

import (
	"unsafe"
)

const (
	PluginName        = "example-wasm"
	PluginVersion     = "1.0.0"
	PluginDescription = "L3 Wasm example plugin for Aroute CMS"
	PluginAuthor      = "Aroute"
)

const bufferSize = 65536

var buffer [bufferSize]byte
var bufferPos int

func resetBuffer() {
	for i := range buffer {
		buffer[i] = 0
	}
	bufferPos = 0
}

func writeString(s string) (int32, int32) {
	offset := bufferPos
	copy(buffer[bufferPos:], s)
	bufferPos += len(s)
	return int32(offset), int32(len(s))
}

func writeBytes(data []byte) (int32, int32) {
	offset := bufferPos
	copy(buffer[bufferPos:], data)
	bufferPos += len(data)
	return int32(offset), int32(len(data))
}

//go:export allocate_memory
func allocateMemory(size uint32) uint32 {
	offset := uint32(bufferPos)
	if bufferPos+int(size) > len(buffer) {
		return 0
	}
	bufferPos += int(size)
	return offset
}

// --- Exported Lifecycle Functions ---

//go:export init
func init_() int32 {
	resetBuffer()

	// Check if database service is available
	hasDB := cms_service_has(0)

	status := "unavailable"
	if hasDB == 1 {
		status = "available"
	}

	msg := `{"level":"info","plugin":"example-wasm","phase":"init","message":"Wasm plugin initializing","database":` + `"` + status + `"}`
	msgOff, msgLen := writeString(msg)
	host_log(msgOff, msgLen)

	return 0
}

//go:export start
func start() int32 {
	resetBuffer()

	// Publish a startup event
	topic := "wasm.example.started"
	topicOff, topicLen := writeString(topic)

	payload := `{"plugin":"example-wasm","version":"` + PluginVersion + `","status":"running"}`
	payloadOff, payloadLen := writeString(payload)

	cms_event_publish(topicOff, topicLen, payloadOff, payloadLen)

	msg := `{"level":"info","plugin":"example-wasm","phase":"start","message":"Wasm plugin started and event published"}`
	msgOff, msgLen := writeString(msg)
	host_log(msgOff, msgLen)

	return 0
}

//go:export stop
func stop() int32 {
	resetBuffer()

	// Publish a shutdown event
	topic := "wasm.example.stopped"
	topicOff, topicLen := writeString(topic)

	payload := `{"plugin":"example-wasm","status":"stopped"}`
	payloadOff, payloadLen := writeString(payload)

	cms_event_publish(topicOff, topicLen, payloadOff, payloadLen)

	msg := `{"level":"info","plugin":"example-wasm","phase":"stop","message":"Wasm plugin stopped"}`
	msgOff, msgLen := writeString(msg)
	host_log(msgOff, msgLen)

	return 0
}

// --- Host Function Imports (cms module) ---

//go:wasmimport cms service_has
func cms_service_has(serviceID uint32) uint32

//go:wasmimport cms service_get
func cms_service_get(serviceID uint32) uint32

//go:wasmimport cms event_subscribe
func cms_event_subscribe(topicPtr, topicLen uint32) uint32

//go:wasmimport cms event_publish
func cms_event_publish(topicPtr, topicLen, dataPtr, dataLen uint32)

//go:wasmimport cms memory_free
func host_memory_free(ptr uint32)

//go:wasmimport aroute host_log
func host_log(msgOffset, msgLen int32)

func main() {}

var _ = unsafe.Pointer(&buffer[0])
