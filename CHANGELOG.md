# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 1.0.1 — 2025-05-06

### Added

- **Appearance Management Page** — Switch frontend themes and admin UI variants from the admin panel without restarting the server (browser refresh required)
  - Frontend theme cards with Activate button and live switching
  - Admin UI variant cards with Switch button and confirmation dialog
- **Admin UI Variant Hot-Swap** — Multiple admin interface variants can coexist and be switched at runtime
  - `AdminUISwitcher` interface for variant management (get/set/list)
  - Active variant persisted to config, handler rebuilt atomically
  - Admin plugin serves from `data/plugin_data/admin/{variant}/` directory
- **admin-compact Variant** — Compact admin UI variant (no logo icon, slim sidebar), built from the same source via Vite `define: { __VARIANT__: 'compact' }`
- **Frontend Theme Hot-Reload** — `ThemeService.ReloadThemes()` rescans the themes directory and syncs new themes to the database without restart
  - Automatic rescan triggered when listing themes from the API
- **themes/warm** — Sample frontend theme with distinct visual design
  - Serif typography (Playfair Display + Source Sans 3)
  - Warm amber/brown color palette (#b45309 primary)
  - Editorial layout with square-cornered buttons, no glassmorphism
  - Full template set matching the default theme structure
- **Theme API Endpoints** — 6 new REST endpoints for theme and variant management
  - `GET/PUT /api/v1/themes/active` — Get or set active frontend theme
  - `GET /api/v1/themes` — List all available themes with metadata
  - `GET/PUT /api/v1/admin-variant` — Get or set active admin variant
  - `GET /api/v1/admin-variants` — List all available admin variants
- **Theme Database Sync** — Seeded themes automatically get database records via `UpsertOrCreate` (ON CONFLICT DO NOTHING)
- **Makefile** — `admin-build-all` target builds both default and compact admin variants
- **README** — Updated with Appearance module, project structure entries, and roadmap items

### Fixed

- **PostgreSQL bool encoding** — Go `bool` values cannot be encoded into PostgreSQL `INTEGER` columns via pgx binary protocol; converted to `int` (0/1) before parameter binding
- **PostgreSQL placeholder syntax** — `store.SetActive` used `*sql.Tx` directly which bypasses `DatabaseService.normalizePlaceholders()`, causing `?` to be sent to pgx as-is; replaced with `db.Exec` which handles `?` → `$N` conversion
- **Theme manifest path** — `handleListThemes` read manifests from hardcoded `themes/` path instead of the plugin's data directory; now reads from in-memory theme map via `ThemeMeta()`

## 1.0.0 — 2025-05-02

First stable release of ARoute CMS.

### Added

- **Microkernel Architecture** — Plugin lifecycle management, service discovery, and event dispatching with zero business logic in Core
  - ServiceContainer with generic `Provide[T]`/`Get[T]`/`GetNamed[T]` dependency injection
  - EventBus with dual-mode: Filter chain (ordered, interruptible) + Broadcast (concurrent) with wildcard subscriptions
  - Plugin Registry with bbolt persistence
  - Lifecycle Manager with topological sort startup, hot-plug support, and cycle detection
  - Engine Dispatcher: L1 native (Go interface) + L3 Wasm (wazero sandbox)
  - Dynamic DDL Engine (SQLite + PostgreSQL dialects)
- **13 L1 Official Plugins** — HTTP, Database, Auth, Content, Media, Theme, Search, REST API, Cache, Queue, Webhook, Frontend, Admin UI
- **Dynamic Content Types** — Define content schemas from Admin UI; real database tables auto-created with proper column types and indexes
  - Built-in types: Page, Post, Category, Tag
  - 16 custom field types: text, number, boolean, date, datetime, relation, media, JSON, markdown, richtext, email, url, slug, enum, color
  - Validation rules: required, min/max length, pattern, unique
  - Version history with draft/publish workflow
- **RBAC Access Control** — Full role-based access control covering backend API and frontend UI
  - Preset roles: Super Admin, Editor, Author, Viewer
  - Fine-grained resource permissions for all modules
  - Author-scoped content filtering for non-admin users
  - Viewer lockout: viewer-only accounts cannot access the admin panel
  - API Token support for programmatic access
- **Admin UI** (React + Arco Design)
  - Dashboard with site statistics
  - Content management with rich text editing (TipTap)
  - Content Type Builder for visual schema definition
  - Media library with upload, preview (image, PDF, code, Office), and orphan cleanup
  - Menu management with multi-level navigation and dynamic rendering
  - Plugin management with upload/install, enable/disable, system plugin protection, and state monitoring
  - User management with role assignment
  - Role management with fine-grained permission configuration
  - Site settings (site name, URL, language, timezone) — reflected live on the public website
  - Chinese/English i18n with dark/light theme toggle
- **Public Website** (Frontend plugin)
  - Go template rendering with site config integration
  - Dynamic navigation menu generation
  - Theme support with default blog theme
- **Hybrid Rendering Engine** — Go `html/template`, Lua (gopher-lua), React SSR (fastschema/qjs)
- **Plugin Sandbox Isolation** — L1 native Go (process-level) + L3 Wasm (wazero sandbox, zero CGO)
- **CLI Tool** — `serve`, `init`, `migrate`, `plugin`, `config`, `version` subcommands
- **Dual Database** — SQLite (zero-deploy, modernc/sqlite) or PostgreSQL (production, pgx)
- **Full-text Search** — Bleve index with gse Chinese tokenizer and faceted search
- **Caching** — dgraph-io/ristretto with TTL and EventBus auto-invalidation
- **Task Queue** — Goroutine worker pool with exponential backoff retry and dead letter queue
- **Webhook** — HTTP POST delivery with HMAC-SHA256 signing, SSRF protection, and auto-disable
- **Integration & E2E Tests** — Comprehensive test coverage across core and plugins
- **Docker** — Dockerfile and docker-compose for development and deployment
- **Documentation** — Getting started, configuration, API reference, theme development, plugin development guides

### Fixed

- Plugin disable button now correctly persists state to registry and reflects in UI
- Plugin list sorting is stable (alphabetical) across page refreshes
- Site settings (site_name, site_url) now correctly reflected on the public frontend
- Dead email/SMTP settings section removed from Settings page
- Settings page layout fixed after section removal
- Content list filtered by author scope for non-admin users
- Viewer-only accounts blocked from admin panel login
- Security vulnerabilities addressed across the codebase

## 0.1.0

Initial pre-release of ARoute CMS.

### Added

- Plugin-based architecture (HTTP, API, Auth, Media, Admin, Frontend, Theme)
- REST API with JWT authentication and RBAC authorization
- Admin dashboard with content, media, user, and menu management
- Media library with upload, preview (image, PDF, code, Office), and storage backends (local/S3)
- Template-based frontend with theme support
- CLI with serve, migrate, and version commands
