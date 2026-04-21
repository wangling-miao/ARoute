.PHONY: build clean check

PLUGIN_NAME := example-wasm
BUILD_DIR := build
OUTPUT := $(BUILD_DIR)/example.wasm

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
