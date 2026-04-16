<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go Version" />
  <img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square" alt="License" />
  <img src="https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-lightgrey?style=flat-square" alt="Platform" />
  <img src="https://img.shields.io/badge/CGO-zero-green?style=flat-square" alt="Zero CGO" />
  <a href="README_EN.md">
    <img src="https://img.shields.io/badge/README-English-blue?style=flat-square" alt="English README" />
  </a>
</p>

<h1 align="center">ARoute CMS</h1>

<p align="center">
  <strong>基于 Go 的现代微内核内容管理系统</strong>
</p>

<p align="center">
  插件沙箱隔离 · 动态内容类型 · 混合渲染引擎 · 单二进制零依赖部署
</p>

<p align="center">
  <a href="#特性">特性</a> ·
  <a href="#快速开始">快速开始</a> ·
  <a href="#架构概览">架构</a> ·
  <a href="#插件">插件</a> ·
  <a href="#配置">配置</a> ·
  <a href="#开发">开发</a> ·
  <a href="#路线图">路线图</a> ·
  <a href="#贡献">贡献</a>
</p>

---

<br/>

> **⚠️ 开发中**
>
> ARoute CMS 正在积极开发中，尚未达到 v1.0.0。Core 微内核和基础插件（HTTP、Database、Auth、CLI）已可用。参见 [路线图](#路线图) 了解进度。
>
> **English** · [README_EN.md](README_EN.md)

<br/>

## 为什么选择 ARoute

Go 生态中缺少一个同时具备微内核架构、三层插件沙箱隔离、动态 Content Type 和真正单二进制部署的 CMS 产品。现有方案各有不足：

| 项目 | 状态 | 微内核 | 插件沙箱 | 动态内容类型 | 单二进制 |
|------|------|:------:|:--------:|:----------:|:--------:|
| Ponzu | 🚫 已废弃 | ❌ | ❌ | ❌ | ✅ |
| FastSchema | 维护中 | ❌ | ❌ | ✅ | ✅ |
| Hugo | 维护中 | ❌ | ❌ | ❌ | ✅ |
| **ARoute** | **开发中** | **✅** | **✅ (L1/L3)** | **✅** | **✅** |

---

## 特性

### 🧠 微内核架构

Core **不包含任何业务逻辑** — 仅负责插件生命周期管理、服务发现和事件分发。所有功能（包括 HTTP 服务器）都是可替换的插件。

- **Plugin Interface** — 统一的插件接口，定义生命周期钩子（`Init → Start → Stop`）
- **ServiceContainer** — 泛型依赖注入容器，`Provide[T]`/`Get[T]`/`GetNamed[T]`
- **EventBus** — 双模式事件总线：Filter 链（有序、可中止、结果传递）+ Broadcast（并发、fire-and-forget），支持通配符订阅
- **Lifecycle Manager** — 插件生命周期状态机，拓扑排序启动，热插拔支持，循环依赖检测
- **Engine Dispatcher** — 双引擎调度：L1 native（Go 接口直调）+ L3 Wasm（wazero 沙箱）

### 📦 动态内容类型

从 Admin UI 定义内容模型，ARoute 自动创建真实数据库表，拥有完整的列类型和索引。不是 EAV 模式，也不是纯 JSON 存储。

- 内置类型：Page、Post、Category、Tag
- 自定义字段类型：text、number、boolean、date、datetime、relation、media、JSON、markdown、richtext、email、url、slug、enum、color
- 验证规则：required、minLength/maxLength、pattern、unique
- 版本历史 + 草稿/发布工作流

### 🎨 混合渲染引擎

三种模板引擎适应不同场景，运行时可切换：

| 引擎 | 适用场景 | 依赖 |
|------|---------|------|
| Go `html/template` | 高性能、零依赖 | 内置 |
| Lua (gopher-lua) | 灵活脚本，LState 池化 | 纯 Go |
| React SSR (fastschema/qjs) | 现代 JS 生态，组件化 | 纯 Go + Wasm |

### 🔒 插件沙箱隔离

| 层级 | 引擎 | 隔离级别 | 适用场景 |
|------|------|---------|---------|
| **L1** | Native Go | 进程级 | 官方插件、可信扩展 |
| **L3** | Wasm (wazero) | 沙箱隔离 | 不受信的第三方插件 |

### 🚀 单二进制，零依赖

```bash
$ aroute serve
```

就这样。无需运行时、解释器或外部数据库（SQLite 内嵌）。Admin UI 通过 `go:embed` 嵌入二进制。

- **零 CGO** — 所有依赖均为纯 Go 或 Wasm 实现
- **跨平台** — linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64
- **双数据库** — SQLite（零依赖部署）或 PostgreSQL（生产扩展）

---

## 快速开始

### 安装

**下载预编译二进制**（发布后可用）：

```bash
curl -sL https://github.com/wangling-miao/aroute/releases/latest/download/aroute_$(uname -s)_$(uname -m).tar.gz | tar xz
```

**从源码构建：**

```bash
# 前置要求：Go 1.26+，Node.js 20+（仅 Admin UI 需要）
git clone https://github.com/wangling-miao/aroute.git
cd aroute

# 仅编译后端（不含 Admin UI）
make build

# 编译后端 + Admin UI
make admin-build && make build

# 或一步到位
make all  # lint + test + build
```

**Docker：**

```bash
docker compose -f deploy/docker-compose-dev.yaml up -d
```

### 首次运行

```bash
# 交互式初始化（创建配置文件、设置管理员密码）
./bin/aroute init

# 或直接使用默认配置启动
./bin/aroute serve

# 自定义参数启动
./bin/aroute serve --host 0.0.0.0 --port 8080 --config ./aroute.yaml --log-level debug
```

访问 `http://localhost:8080/admin/` 进入管理后台。

### 命令行工具

```bash
aroute serve                    # 启动 CMS 服务器
aroute init                     # 交互式首次设置
aroute migrate up               # 执行数据库迁移
aroute migrate down             # 回滚迁移
aroute migrate status           # 查看迁移状态
aroute plugin list              # 列出已安装插件
aroute plugin install <path>    # 安装插件
aroute plugin enable <name>     # 启用插件
aroute plugin disable <name>    # 禁用插件
aroute plugin remove <name>     # 移除插件
aroute config show              # 显示当前配置
aroute config validate          # 验证配置文件
aroute version                  # 打印版本信息
```

---

## 架构概览

```
┌─────────────────────────────────────────────────────────────┐
│                        ARoute CMS                           │
├─────────────────────────────────────────────────────────────┤
│                       Core 微内核                            │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌───────────────┐  │
│  │  服务    │ │  事件    │ │  插件    │ │    生命周期   │  │
│  │  容器    │ │  总线    │ │  注册表  │ │    管理器     │  │
│  └──────────┘ └──────────┘ └──────────┘ └───────────────┘  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐                    │
│  │  引擎    │ │  许可证  │ │ DDL引擎  │                    │
│  │  调度器  │ │  子系统  │ │          │                    │
│  └──────────┘ └──────────┘ └──────────┘                    │
├─────────────────────────────────────────────────────────────┤
│                    L1 官方插件集                             │
│  ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐       │
│  │HTTP│ │ DB │ │Auth│ │内容│ │媒体│ │主题│ │搜索│       │
│  └────┘ └────┘ └────┘ └────┘ └────┘ └────┘ └────┘       │
│  ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌─────────────────┐  │
│  │API │ │缓存│ │队列│ │Hook│ │SDK │ │    Admin UI     │  │
│  └────┘ └────┘ └────┘ └────┘ └────┘ └─────────────────┘  │
├─────────────────────────────────────────────────────────────┤
│                    L3 Wasm 沙箱                              │
│              (wazero — 零 CGO，纯 Go)                       │
└─────────────────────────────────────────────────────────────┘
```

**插件通信流程：**

```
插件 A ──注册服务──▶ ServiceContainer ◀──获取服务── 插件 B
  │                                                  │
  └──订阅事件──▶ EventBus ◀──发送事件──────────────┘
```

所有插件间通信通过 `ServiceContainer` + `EventBus` 进行 — **插件之间零直接 import**。

### 项目结构

```
aroute/
├── cmd/aroute/             # CLI 入口（cobra）
├── core/                   # 微内核
│   ├── plugin.go           # Plugin 接口 + Manifest
│   ├── context.go          # CoreContext
│   ├── services/           # 泛型服务容器
│   ├── events/             # 双模式事件总线
│   ├── registry/           # 插件注册表（bbolt 持久化）
│   ├── lifecycle/          # 生命周期管理器（状态机）
│   ├── engine/             # 引擎调度器（L1/L3）
│   ├── license/            # 许可证子系统（ECDSA）
│   └── ddl/                # 动态 DDL 引擎
├── sdk/                    # 插件开发 SDK
│   ├── interfaces/         # 共享接口定义（零依赖）
│   ├── go/                 # Go 插件 SDK
│   └── wasm/               # Wasm 插件模板
├── plugins/                # L1 官方插件集
│   ├── http/               # HTTP 服务器（chi 路由器）
│   ├── database/           # 数据库（SQLite + PostgreSQL）
│   ├── auth/               # 认证授权（JWT + RBAC）
│   ├── content/            # 内容管理
│   ├── media/              # 媒体存储
│   ├── theme/              # 主题引擎
│   ├── search/             # 全文搜索（bleve + gse）
│   ├── api/                # REST API 自动生成
│   ├── cache/              # 缓存（ristretto）
│   ├── queue/              # 任务队列
│   ├── webhook/            # Webhook
│   └── admin/              # Admin UI（React + Arco Design）
├── admin/                  # Admin UI 源码（React）
├── themes/default/         # 默认主题（Go template）
├── configs/                # 配置模板
├── deploy/                 # Docker 与部署配置
├── internal/               # 内部工具包
├── docs/                   # 文档
├── go.mod
├── Makefile
└── .goreleaser.yaml
```

---

## 插件

### 核心插件（12 个 L1 官方插件）

| 插件 | 说明 | 核心技术 |
|------|------|---------|
| **HTTP** | HTTP 服务器，中间件链 | go-chi/chi、CORS、优雅关闭 |
| **Database** | 双驱动数据库层 | modernc/sqlite（零 CGO）、pgx（PostgreSQL）、sqlc |
| **Auth** | 认证与授权 | JWT（HS256）、bcrypt、RBAC、API Token |
| **Content** | 内容管理引擎 | 动态 Content Type、字段验证、版本历史 |
| **Media** | 文件上传与存储 | 本地文件系统、S3 兼容存储（minio-go）、缩略图生成 |
| **Theme** | 多引擎模板渲染 | Go template、Lua（gopher-lua）、React SSR（fastschema/qjs） |
| **Search** | 全文搜索 | Bleve 索引、gse 中文分词、分面搜索 |
| **REST API** | 自动生成 CRUD API | OpenAPI 3.0、过滤/排序/分页、稀疏字段选择 |
| **Cache** | 内存缓存 | dgraph-io/ristretto、TTL、EventBus 自动失效 |
| **Queue** | 轻量任务队列 | Goroutine worker pool、指数退避重试、死信队列 |
| **Webhook** | 事件驱动通知 | HTTP POST 投递、HMAC-SHA256 签名、重试策略 |
| **Admin UI** | 管理后台 | React、TypeScript、Arco Design、TipTap、中英 i18n、暗色/亮色主题 |

### 插件开发

**Go 插件（L1）：**

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
    // 获取共享服务
    db := ctx.Services().MustGet[interfaces.DatabaseService]()
    ctx.Logger().Info("插件初始化完成", "db", db != nil)
    return nil
}

func (p *MyPlugin) Start() error { return nil }
func (p *MyPlugin) Stop() error  { return nil }
```

**Wasm 插件（L3）：**

```go
// 使用 TinyGo 编译：tinygo build -o plugin.wasm -target=wasi main.go
// 通过 Wasm Host Function 与 Core 服务交互
```

完整示例见 `sdk/go/example/` 和 `sdk/wasm/template/`。

---

## 配置

ARoute 使用分层配置系统，优先级：**CLI 参数 > 环境变量（`AROUTE_` 前缀）> 配置文件（YAML/TOML）> 默认值**

```bash
# 生成配置文件模板
cp configs/aroute.example.yaml aroute.yaml
```

核心配置项：

```yaml
server:
  host: "0.0.0.0"
  port: 8080

database:
  driver: "sqlite"  # "sqlite" 或 "postgres"
  sqlite:
    path: "data/aroute.db"

auth:
  jwt_secret: "修改为一个随机字符串"
  jwt_algorithm: "HS256"
  access_token_ttl: "15m"

media:
  storage: "local"  # "local" 或 "s3"

theme:
  active: "default"

log:
  level: "info"    # debug, info, warn, error
  format: "json"  # json, text
```

完整配置参考 [`configs/aroute.example.yaml`](configs/aroute.example.yaml)。

---

## 技术栈

### 后端

| 组件 | 技术 | 说明 |
|------|------|------|
| 语言 | Go 1.26+ | 纯 Go，零 CGO |
| 路由 | go-chi/chi | 轻量级，兼容标准库 |
| 数据库 | modernc/sqlite + pgx | SQLite（零依赖部署）+ PostgreSQL（生产环境） |
| 数据访问 | sqlc + 动态查询构建器 | 固定表用 sqlc 编译时类型安全，动态表用运行时构建器 |
| 认证 | golang-jwt + bcrypt | JWT + RBAC |
| 搜索 | Bleve + gse | 全文搜索 + 中文分词 |
| 缓存 | dgraph-io/ristretto | 高性能内存缓存 |
| Wasm 运行时 | tetratelabs/wazero | 零 CGO Wasm 运行时 |
| 元数据存储 | bbolt | 插件元数据持久化 |
| 对象存储 | minio-go | S3 兼容存储 |
| 命令行 | cobra + viper | Go CLI 事实标准 |
| 日志 | log/slog | Go 标准库 |
| 构建 | goreleaser | 跨平台二进制构建 |

### 前端（Admin UI）

| 组件 | 技术 |
|------|------|
| 框架 | React + TypeScript |
| 构建工具 | Vite |
| UI 组件库 | Arco Design（字节跳动） |
| 富文本编辑器 | TipTap |
| 国际化 | react-i18next（中文 / English） |
| 嵌入方式 | go:embed（Vite 构建产物 → Go 二进制） |

---

## 开发

### 前置要求

- **Go 1.26+**（后端）
- **Node.js 20+**（仅 Admin UI 需要）
- **golangci-lint**（代码检查）

### Make 命令

```bash
make build        # 编译二进制
make test         # 运行测试（含覆盖率）
make lint         # 运行 golangci-lint
make all          # lint + test + build
make admin-build  # 编译 Admin UI
make admin-dev    # 启动 Admin UI 开发服务器
make cover        # 生成 HTML 覆盖率报告
make run          # 编译并运行
make fmt          # 格式化 Go 代码
make tidy         # 整理依赖
make clean        # 清理构建产物
make help         # 显示所有命令
```

### 运行测试

```bash
# 全部测试（含竞态检测和覆盖率）
make test

# 指定包
go test -v ./core/services/...

# 生成 HTML 覆盖率报告
make cover
```

### 开发模式

```bash
# 终端 1：Go 后端
make build && ./bin/aroute serve --log-level debug

# 终端 2：Admin UI 开发服务器（代理到 Go 后端）
make admin-dev
```

---

## 路线图

### ✅ 已完成

- [x] 项目结构与工具链（golangci-lint、goreleaser、CI）
- [x] SDK 接口定义（`sdk/interfaces/`）
- [x] Core 微内核
  - [x] Plugin 接口 + Manifest + CoreContext
  - [x] ServiceContainer（泛型 `Provide[T]`/`Get[T]`）
  - [x] EventBus（Filter 链 + Broadcast + 通配符）
  - [x] Plugin Registry（bbolt 持久化）
  - [x] Lifecycle Manager（状态机、拓扑排序、热插拔）
  - [x] Engine Dispatcher（L1 native + L3 Wasm）
  - [x] License 子系统（ECDSA P-256）
  - [x] 动态 DDL Engine（SQLite + PostgreSQL 方言）
- [x] Core 集成（完整内核启动 + 插件编排）
- [x] HTTP Server 插件（chi、CORS、健康检查、优雅关闭）
- [x] Database 插件（SQLite + PG、Migration Runner、Schema 内省）
- [x] Auth 插件（JWT、bcrypt、RBAC、API Token、频率限制）
- [x] CLI 工具（serve、init、migrate、plugin、config、version）
- [x] Content 插件（动态 Content Type、CRUD、版本历史、Slug）

### 🚧 开发中

- [ ] Media 插件（上传、本地/S3 存储、缩略图）
- [ ] Theme Engine 插件（Go template → Lua → React SSR）
- [ ] Search 插件（Bleve + gse 中文分词）
- [ ] REST API 插件（自动 CRUD、OpenAPI 3.0）
- [ ] Cache 插件（ristretto + EventBus 自动失效）
- [ ] Queue 插件（Goroutine worker pool、重试、死信队列）
- [ ] Webhook 插件（HTTP POST、HMAC-SHA256 签名、重试策略）
- [ ] Admin UI（React + Arco Design + TipTap）
- [ ] Plugin SDK（Go SDK + Wasm 模板）
- [ ] 默认主题（Go template 博客主题）
- [ ] 集成测试与端到端测试
- [ ] 文档与 Dockerfile

### 🔮 计划中（v1.x+）

- [ ] L2 gRPC 插件引擎（Pro 版特性）
- [ ] 集群部署 / 多节点同步（v2.0）
- [ ] WebSocket 实时推送
- [ ] 可视化页面编辑器（Pro 版特性）

---

## 贡献

欢迎贡献！无论是 Bug 报告、功能建议、文档改进还是代码提交，每一份贡献都很重要。

### 如何贡献

1. **Fork** 本仓库
2. 创建**特性分支**（`git checkout -b feature/amazing-feature`）
3. 为改动编写**测试**
4. 确保测试通过（`make test`）且 lint 无误（`make lint`）
5. 提交 **Pull Request**

### 开发规范

- 遵循现有代码风格 — 先参考代码库中的类似文件
- 所有新功能必须编写测试（Core ≥ 90%，插件 ≥ 80% 覆盖率目标）
- 使用 `log/slog` 进行结构化日志记录 — 生产代码禁止使用 `fmt.Println`
- 所有动态 SQL 查询必须使用参数化语句，绝不拼接用户输入
- 提交前运行 `make fmt` 和 `make lint`

---

## 许可证

本项目采用 [GNU Affero General Public License v3.0](LICENSE) 许可证。

---

## 致谢

使用了以下优秀的开源项目：

- [go-chi/chi](https://github.com/go-chi/chi) — 轻量级 HTTP 路由器
- [modernc/sqlite](https://gitlab.com/cznic/sqlite) — 纯 Go SQLite 驱动（零 CGO）
- [jackc/pgx](https://github.com/jackc/pgx) — PostgreSQL 驱动
- [tetratelabs/wazero](https://github.com/tetratelabs/wazero) — 零 CGO Wasm 运行时
- [go.etcd.io/bbolt](https://github.com/etcd-io/bbolt) — 嵌入式 KV 存储
- [spf13/cobra](https://github.com/spf13/cobra) + [viper](https://github.com/spf13/viper) — CLI 与配置管理
- [Bleve](https://github.com/blevesearch/bleve) — 全文搜索引擎
- [dgraph-io/ristretto](https://github.com/dgraph-io/ristretto) — 高性能缓存库
- [Arco Design](https://arco.design/) — React UI 组件库
- [TipTap](https://tiptap.dev/) — 富文本编辑器

---

<p align="center">
  <strong>ARoute CMS</strong> — 用 Go 的方式做现代 CMS。
</p>
