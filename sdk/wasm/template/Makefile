.PHONY: build clean check

# TinyGo Wasm plugin build configuration
PLUGIN_NAME := wasm-example
BUILD_DIR := build
OUTPUT := $(BUILD_DIR)/plugin.wasm

# TinyGo flags for WASI target
TINYGO := tinygo
GOFLAGS := -target=wasi -no-debug -scheduler=none -gc=leaking

build: $(OUTPUT)

$(OUTPUT): main.go
	@mkdir -p $(BUILD_DIR)
	$(TINYGO) build $(GOFLAGS) -o $(OUTPUT) .

clean:
	rm -rf $(BUILD_DIR)

check: build
	@file $(OUTPUT) | grep -q "WebAssembly" && echo "OK: valid WebAssembly module" || echo "FAIL: not a valid WebAssembly module"
