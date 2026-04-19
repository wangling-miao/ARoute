BINARY_NAME := aroute
BUILD_DIR := bin
CMD_DIR := cmd/aroute
ADMIN_DIR := admin

# Build flags
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GO_VERSION := $(shell go version | awk '{print $$3}')

LDFLAGS := -s -w \
	-X github.com/wangling-miao/aroute/internal/version.Version=$(VERSION) \
	-X github.com/wangling-miao/aroute/internal/version.Commit=$(COMMIT) \
	-X github.com/wangling-miao/aroute/internal/version.BuildDate=$(BUILD_DATE) \
	-X github.com/wangling-miao/aroute/internal/version.GoVersion=$(GO_VERSION)

.PHONY: all build test lint generate clean admin-build help

all: lint test build

build:
	@rm -rf plugins/admin/dist && cp -r $(ADMIN_DIR)/dist plugins/admin/dist
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@rm -rf plugins/admin/dist

test:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...

lint:
	golangci-lint run ./...

generate:
	go generate ./...

clean:
	rm -rf $(BUILD_DIR) coverage.txt coverage.html dist/

admin-build:
	cd $(ADMIN_DIR) && npm ci && npm run build

admin-dev:
	cd $(ADMIN_DIR) && npm run dev

cover: test
	go tool cover -html=coverage.txt -o coverage.html

vet:
	go vet ./...

fmt:
	gofmt -w -s .

tidy:
	go mod tidy

run: build
	./$(BUILD_DIR)/$(BINARY_NAME) serve

help:
	@echo "Available targets:"
	@echo "  all          - lint, test, build"
	@echo "  build        - Build the binary"
	@echo "  test         - Run tests with race detection and coverage"
	@echo "  lint         - Run golangci-lint"
	@echo "  generate     - Run go generate"
	@echo "  clean        - Remove build artifacts"
	@echo "  admin-build  - Build Admin UI (npm)"
	@echo "  admin-dev    - Start Admin UI dev server"
	@echo "  cover        - Generate HTML coverage report"
	@echo "  vet          - Run go vet"
	@echo "  fmt          - Format Go source files"
	@echo "  tidy         - Run go mod tidy"
	@echo "  run          - Build and run the server"
