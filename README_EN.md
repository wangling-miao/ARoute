

<h1 align="center">ARoute CMS</h1>

<p align="center">
  <strong>A modern, microkernel-based CMS built in Go</strong>
</p>

<p align="center">
  Plugin sandboxing · Dynamic Content Types · Hybrid rendering · Single binary deployment
</p>
<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go Version" />
  <img src="https://img.shields.io/badge/license-Apache%20License%202.0-blue" alt="License" />
  <img src="https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-lightgrey?style=flat-square" alt="Platform" />
  <img src="https://img.shields.io/badge/CGO-zero-green?style=flat-square" alt="Zero CGO" />
  <a href="README.md">
    <img src="https://img.shields.io/badge/README-中文-red?style=flat-square" alt="中文 README" />
  </a>
</p>
<p align="center">
  <a href="#features">Features</a> ·
  <a href="#quick-start">Quick Start</a> ·
  <a href="#architecture">Architecture</a> ·
  <a href="#plugins">Plugins</a> ·
  <a href="#configuration">Configuration</a> ·
  <a href="#development">Development</a> ·
  <a href="#roadmap">Roadmap</a> ·
  <a href="#contributing">Contributing</a>
</p>

---

<br/>

> **⚠️ Work in Progress**
>
> ARoute CMS is under active development and has not yet reached v1.0.0. The Core microkernel and foundational plugins (HTTP, Database, Auth, CLI) are functional. See [Roadmap](#roadmap) for progress.
>
> **中文** · [README.md](README.md)

<br/>

## Why ARoute

The Go ecosystem lacks a CMS that simultaneously delivers microkernel architecture, three-tier plugin sandboxing, dynamic Content Types, and true single-binary deployment. Existing solutions fall short:

| Project | Status | Microkernel | Plugin Sandbox | Dynamic Content Type | Single Binary |
|---------|--------|:-----------:|:--------------:|:-------------------:|:-------------:|
| Ponzu | 🚫 Abandoned | ❌ | ❌ | ❌ | ✅ |
| FastSchema | Active | ❌ | ❌ | ✅ | ✅ |
| Hugo | Active | ❌ | ❌ | ❌ | ✅ |
| **ARoute** | **Active** | **✅** | **✅ (L1/L3)** | **✅** | **✅** |

---

## Features

### 🧠 Microkernel Architecture

The Core contains **zero business logic** — it only manages plugin lifecycle, service discovery, and event dispatching. Every feature (including the HTTP server) is a plugin that can be replaced.

- **Plugin Interface** — Unified contract with lifecycle hooks (`Init → Start → Stop`)
- **ServiceContainer** — Generic `Provide[T]`/`Get[T]`/`GetNamed[T]` dependency injection
- **EventBus** — Dual-mode: Filter chain (ordered, interruptible) + Broadcast (concurrent, fire-and-forget) with wildcard subscriptions
- **Lifecycle Manager** — State machine with topological sort startup, hot-plug support, and cycle detection
- **Engine Dispatcher** — L1 native (Go interface) + L3 Wasm (wazero sandbox)

### 📦 Dynamic Content Types

Define content schemas from the Admin UI — ARoute automatically creates real database tables with proper column types and indexes. No EAV, no JSON-only storage.

- Built-in types: Page, Post, Category, Tag
- Custom field types: text, number, boolean, date, datetime, relation, media, JSON, markdown, richtext, email, url, slug, enum, color
- Validation rules: required, min/max length, pattern, unique
- Version history with draft/publish workflow

### 🎨 Hybrid Rendering

Three template engines to suit every use case, switchable at runtime:

| Engine | Use Case | Dependency |
|--------|----------|------------|
| Go `html/template` | High-performance, zero-dependency | Built-in |
| Lua (gopher-lua) | Flexible scripting with LState pool | Pure Go |
| React SSR (fastschema/qjs) | Modern JS ecosystem, component-based | Pure Go + Wasm |

### 🔒 Plugin Sandboxing

| Tier | Engine | Isolation | Use Case |
|------|--------|-----------|----------|
| **L1** | Native Go | Process-level | Official plugins, trusted extensions |
| **L3** | Wasm (wazero) | Sandbox | Untrusted third-party plugins |

### 🚀 Single Binary, Zero Dependencies

```
$ aroute serve
```

That's it. No runtime, no interpreter, no external database required (SQLite embedded). Admin UI is embedded via `go:embed`.

- **Zero CGO** — All dependencies are pure Go or Wasm
- **Cross-platform** — linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
- **Dual database** — SQLite (zero-deploy) or PostgreSQL (production scale)

---

## Quick Start

### Install

**Download prebuilt binary** (when released):

```bash
curl -sL https://github.com/wangling-miao/aroute/releases/latest/download/aroute_$(uname -s)_$(uname -m).tar.gz | tar xz
```

**Build from source:**

```bash
# Requirements: Go 1.26+, Node.js 20+ (for Admin UI)
git clone https://github.com/wangling-miao/aroute.git
cd aroute

# Build backend only (without Admin UI)
make build

# Build with Admin UI
make admin-build && make build

# Or build everything
make all  # lint + test + build
```

**Docker:**

```bash
docker compose -f deploy/docker-compose-dev.yaml up -d
```

### First Run

```bash
# Interactive setup (creates config, sets admin password)
./bin/aroute init

# Or start directly with defaults
./bin/aroute serve

# With custom options
./bin/aroute serve --host 0.0.0.0 --port 8080 --config ./aroute.yaml --log-level debug
```

Open `http://localhost:8080/admin/` to access the admin panel.

### CLI Commands

```bash
aroute serve                    # Start the CMS server
aroute init                     # Interactive first-time setup
aroute migrate up               # Run database migrations
aroute migrate down             # Rollback migrations
aroute migrate status           # Show migration status
aroute plugin list              # List installed plugins
aroute plugin install <path>    # Install a plugin
aroute plugin enable <name>     # Enable a plugin
aroute plugin disable <name>    # Disable a plugin
aroute plugin remove <name>     # Remove a plugin
aroute config show              # Show current configuration
aroute config validate          # Validate configuration
aroute version                  # Print version info
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        ARoute CMS                           │
├─────────────────────────────────────────────────────────────┤
│                     Core Microkernel                        │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌───────────────┐  │
│  │ Service  │ │  Event   │ │ Plugin   │ │  Lifecycle    │  │
│  │Container │ │   Bus    │ │ Registry │ │   Manager     │  │
│  └──────────┘ └──────────┘ └──────────┘ └───────────────┘  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                    │
│  │ Engine   │ │ License  │ │   DDL    │                    │
│  │Dispatcher│ │Subsystem │ │  Engine  │                    │
│  └──────────┘ └──────────┘ └──────────┘                    │
├─────────────────────────────────────────────────────────────┤
│                    L1 Official Plugins                      │
│  ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐       │
│  │HTTP│ │ DB │ │Auth│ │Cntn│ │Med │ │Thm │ │Srsh│       │
│  └────┘ └────┘ └────┘ └────┘ └────┘ └────┘ └────┘       │
│  ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌─────────────────┐  │
│  │API │ │Cach│ │Queue│ │ WH │ │SDK │ │    Admin UI     │  │
│  └────┘ └────┘ └────┘ └────┘ └────┘ └─────────────────┘  │
├─────────────────────────────────────────────────────────────┤
│                    L3 Wasm Sandbox                          │
│              (wazero — zero CGO, pure Go)                   │
└─────────────────────────────────────────────────────────────┘
```

**Plugin communication flow:**

```
Plugin A ──Register Service──▶ ServiceContainer ◀──Get Service── Plugin B
   │                                                       │
   └──Subscribe Event──▶ EventBus ◀──Emit Event──────────┘
```

All inter-plugin communication goes through `ServiceContainer` + `EventBus` — **zero direct imports between plugins**.

### Project Structure

```
aroute/
├── cmd/aroute/             # CLI entry point (cobra)
├── core/                   # Microkernel
│   ├── plugin.go           # Plugin Interface + Manifest
│   ├── context.go          # CoreContext
│   ├── services/           # ServiceContainer (generics)
│   ├── events/             # EventBus (dual-mode)
│   ├── registry/           # Plugin Registry (bbolt)
│   ├── lifecycle/          # Lifecycle Manager (state machine)
│   ├── engine/             # Engine Dispatcher (L1/L3)
│   ├── license/            # License subsystem (ECDSA)
│   └── ddl/                # Dynamic DDL Engine
├── sdk/                    # Plugin SDK
│   ├── interfaces/         # Shared interface definitions (zero deps)
│   ├── go/                 # Go plugin SDK
│   └── wasm/               # Wasm plugin template
├── plugins/                # L1 official plugin set
│   ├── http/               # HTTP Server (chi router)
│   ├── database/           # Database (SQLite + PostgreSQL)
│   ├── auth/               # Authentication (JWT + RBAC)
│   ├── content/            # Content management
│   ├── media/              # Media storage
│   ├── theme/              # Theme engine
│   ├── search/             # Full-text search (bleve + gse)
│   ├── api/                # REST API generator
│   ├── cache/              # Cache (ristretto)
│   ├── queue/              # Task queue
│   ├── webhook/            # Webhook
│   └── admin/              # Admin UI (React + Arco Design)
├── admin/                  # Admin UI source (React)
├── themes/default/         # Default theme (Go template)
├── configs/                # Config templates
├── deploy/                 # Docker & deployment
├── internal/               # Internal packages
├── docs/                   # Documentation
├── go.mod
├── Makefile
└── .goreleaser.yaml
```

---

## Plugins

### Core Plugins (12 L1 Official)

| Plugin | Description | Key Technologies |
|--------|-------------|------------------|
| **HTTP** | HTTP server with middleware chain | go-chi/chi, CORS, graceful shutdown |
| **Database** | Dual-driver database layer | modernc/sqlite (zero CGO), pgx (PostgreSQL), sqlc |
| **Auth** | Authentication & authorization | JWT (HS256), bcrypt, RBAC, API tokens |
| **Content** | Content management engine | Dynamic Content Types, field validation, versioning |
| **Media** | File upload & storage | Local filesystem, S3 (minio-go), thumbnail generation |
| **Theme** | Multi-engine template rendering | Go template, Lua (gopher-lua), React SSR (fastschema/qjs) |
| **Search** | Full-text search | Bleve index, gse Chinese tokenizer, faceted search |
| **REST API** | Auto-generated CRUD API | OpenAPI 3.0, filtering, sorting, pagination, sparse fieldsets |
| **Cache** | In-memory caching | dgraph-io/ristretto, TTL, auto-invalidation via EventBus |
| **Queue** | Lightweight task queue | Goroutine worker pool, retry with exponential backoff, dead letter |
| **Webhook** | Event-driven notifications | HTTP POST delivery, HMAC-SHA256 signing, retry strategy |
| **Admin UI** | Management dashboard | React, TypeScript, Arco Design, TipTap, i18n (zh/en), dark/light mode |

### Plugin Development

**Go Plugin (L1):**

```go
package main

import (
    "github.com/wangling-miao/aroute/core"
    "github.com/wangling-miao/aroute/sdk/interfaces"
)

type MyPlugin struct{}

func (p *MyPlugin) Name() string    { return "my-plugin" }
func (p *MyPlugin) Version() string { return "1.0.0" }

func (p *MyPlugin) Init(ctx core.CoreContext) error {
    // Access shared services
    db := ctx.Services().MustGet[interfaces.DatabaseService]()
    ctx.Logger().Info("Plugin initialized", "db", db != nil)
    return nil
}

func (p *MyPlugin) Start() error { return nil }
func (p *MyPlugin) Stop() error  { return nil }
```

**Wasm Plugin (L3):**

```go
// Build with TinyGo: tinygo build -o plugin.wasm -target=wasi main.go
// Uses Wasm Host Functions to interact with Core services
```

See `sdk/go/example/` and `sdk/wasm/template/` for complete examples.

---

## Configuration

ARoute uses a layered configuration system with priority: **CLI flags > ENV vars (`AROUTE_` prefix) > Config file (YAML/TOML) > Defaults**

```bash
# Generate a config template
cp configs/aroute.example.yaml aroute.yaml
```

Key configuration sections:

```yaml
server:
  host: "0.0.0.0"
  port: 8080

database:
  driver: "sqlite"  # "sqlite" or "postgres"
  sqlite:
    path: "data/aroute.db"

auth:
  jwt_secret: "change-me-to-a-random-string"
  jwt_algorithm: "HS256"
  access_token_ttl: "15m"

media:
  storage: "local"  # "local" or "s3"

theme:
  active: "default"

log:
  level: "info"    # debug, info, warn, error
  format: "json"  # json, text
```

See [`configs/aroute.example.yaml`](configs/aroute.example.yaml) for the complete configuration reference.

---

## Tech Stack

### Backend

| Component | Technology | Notes |
|-----------|-----------|-------|
| Language | Go 1.26+ | Pure Go, zero CGO |
| Router | go-chi/chi | Lightweight, std-compatible |
| Database | modernc/sqlite + pgx | SQLite (zero-deploy) + PostgreSQL (production) |
| Query | sqlc + dynamic builder | Static types for fixed tables, runtime builder for dynamic CT |
| Auth | golang-jwt + bcrypt | JWT + RBAC |
| Search | Bleve + gse | Full-text with Chinese segmentation |
| Cache | dgraph-io/ristretto | High-performance in-memory cache |
| Wasm | tetratelabs/wazero | Zero CGO Wasm runtime |
| Storage | bbolt | Plugin metadata persistence |
| Object Storage | minio-go | S3-compatible storage |
| CLI | cobra + viper | Industry-standard Go CLI |
| Logging | log/slog | Go standard library |
| Build | goreleaser | Cross-platform binary builds |

### Frontend (Admin UI)

| Component | Technology |
|-----------|-----------|
| Framework | React + TypeScript |
| Build tool | Vite |
| UI Library | Arco Design (ByteDance) |
| Rich Text | TipTap |
| i18n | react-i18next (zh/en) |
| Embedding | go:embed (Vite build → Go binary) |

---

## Development

### Prerequisites

- **Go 1.26+** (backend)
- **Node.js 20+** (Admin UI only)
- **golangci-lint** (linting)

### Make Targets

```bash
make build        # Build binary
make test         # Run tests with coverage
make lint         # Run golangci-lint
make all          # lint + test + build
make admin-build  # Build Admin UI
make admin-dev    # Admin UI dev server
make cover        # Generate HTML coverage report
make run          # Build and run
make fmt          # Format Go code
make tidy         # Run go mod tidy
make clean        # Remove build artifacts
make help         # Show all targets
```

### Running Tests

```bash
# All tests with race detection and coverage
make test

# Specific package
go test -v ./core/services/...

# With HTML coverage report
make cover
```

### Development Mode

```bash
# Terminal 1: Go backend
make build && ./bin/aroute serve --log-level debug

# Terminal 2: Admin UI dev server (proxies to Go backend)
make admin-dev
```

---

## Roadmap

### ✅ Completed

- [x] Project structure & tooling (golangci-lint, goreleaser, CI)
- [x] SDK interface definitions (`sdk/interfaces/`)
- [x] Core microkernel
  - [x] Plugin Interface + Manifest + CoreContext
  - [x] ServiceContainer (generic `Provide[T]`/`Get[T]`)
  - [x] EventBus (Filter chain + Broadcast + wildcards)
  - [x] Plugin Registry (bbolt persistence)
  - [x] Lifecycle Manager (state machine, topological sort, hot-plug)
  - [x] Engine Dispatcher (L1 native + L3 Wasm)
  - [x] License subsystem (ECDSA P-256)
  - [x] Dynamic DDL Engine (SQLite + PostgreSQL dialects)
- [x] Core integration (full kernel boot + plugin orchestration)
- [x] HTTP Server plugin (chi, CORS, health check, graceful shutdown)
- [x] Database plugin (SQLite + PG, migration runner, schema introspection)
- [x] Auth plugin (JWT, bcrypt, RBAC, API tokens, rate limiting)
- [x] CLI tool (serve, init, migrate, plugin, config, version)
- [x] Content plugin (dynamic Content Types, CRUD, versioning, slug)
- [x] Media plugin (upload, local/S3 storage, thumbnails)
- [x] Theme Engine plugin (Go template → Lua → React SSR)
- [x] Search plugin (Bleve + gse Chinese tokenizer)
- [x] REST API plugin (auto CRUD, OpenAPI 3.0)
- [x] Cache plugin (ristretto + EventBus auto-invalidation)
- [x] Queue plugin (goroutine worker pool, retry, dead letter)
- [x] Webhook plugin (HTTP POST, HMAC-SHA256, SSRF protection, auto-disable)
- [x] Admin UI (React + Arco Design + TipTap)
- [x] Plugin SDK (Go SDK + Wasm template)
- [x] Default theme (Go template blog theme)
- [x] Integration & E2E tests
- [x] Documentation & Dockerfile

### 🚧 In Progress

none

### 🔮 Planned (v1.x+)

- [ ] L2 gRPC plugin engine (Pro edition)
- [ ] Cluster deployment / multi-node sync (v2.0)
- [ ] WebSocket real-time push
- [ ] Visual page builder (Pro edition)

---

## Contributing

We welcome contributions! Whether it's bug reports, feature requests, documentation improvements, or code — every contribution matters.

### How to Contribute

1. **Fork** the repository
2. Create a **feature branch** (`git checkout -b feature/amazing-feature`)
3. Write **tests** for your changes
4. Ensure all tests pass (`make test`) and lint is clean (`make lint`)
5. Submit a **Pull Request**

### Development Guidelines

- Follow existing code patterns — check similar files in the codebase first
- Write tests for all new functionality (Core ≥ 90%, plugins ≥ 80% coverage target)
- Use `log/slog` for structured logging — no `fmt.Println` in production code
- All dynamic SQL queries must use parameterized statements (never concatenate user input)
- Run `make fmt` and `make lint` before committing

---

## License

This project is licensed under the [Apache License 2.0](LICENSE).

---

## Acknowledgments

Built with these excellent open-source projects:

- [go-chi/chi](https://github.com/go-chi/chi) — Lightweight HTTP router
- [modernc/sqlite](https://gitlab.com/cznic/sqlite) — Pure Go SQLite driver (zero CGO)
- [jackc/pgx](https://github.com/jackc/pgx) — PostgreSQL driver
- [tetratelabs/wazero](https://github.com/tetratelabs/wazero) — Zero CGO Wasm runtime
- [go.etcd.io/bbolt](https://github.com/etcd-io/bbolt) — Embedded key-value store
- [spf13/cobra](https://github.com/spf13/cobra) + [viper](https://github.com/spf13/viper) — CLI & configuration
- [Bleve](https://github.com/blevesearch/bleve) — Full-text search
- [dgraph-io/ristretto](https://github.com/dgraph-io/ristretto) — High-performance cache
- [Arco Design](https://arco.design/) — React UI component library
- [TipTap](https://tiptap.dev/) — Rich text editor

---

<p align="center">
  <strong>ARoute CMS</strong> — Modern CMS, the Go way.
</p>
