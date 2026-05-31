# 插件开发指南

本文档介绍如何为 ARoute CMS 开发插件，涵盖 L1 原生 Go 插件、L2 gRPC 子进程插件（Pro）和 L3 Wasm 沙箱插件的完整开发流程。

## 插件系统概述

ARoute CMS 采用微内核架构，所有功能（包括 HTTP 服务器、数据库、认证）均由插件提供。插件之间通过 ServiceContainer 和 EventBus 通信，不存在直接导入依赖。

### 信任级别

| 级别 | 引擎 | 隔离性 | 性能 | 适用场景 |
|------|------|--------|------|----------|
| L1 | Native Go | 无隔离（同进程） | 最高 | 官方插件、可信插件 |
| L2 | gRPC 子进程 | 进程隔离 + 能力网关 | 高 | Pro/商业扩展 |
| L3 | Wasm (wazero) | 沙箱隔离（内存限制） | 中等 | 第三方插件、不可信插件 |

L2 gRPC 插件受 `plugin:l2-grpc` Pro 特性控制。L2/L3 插件的服务访问、事件发布、日志、配置读取等 Host Call 都经过能力网关；L1 默认允许但会写入审计。

### 动态信任闭环

插件清单可以声明 `trust`、`capabilities`、`publisher`、`digest`、`signature`、`resources` 和 `runtime`。ARoute 会把这些静态证据与运行时事件写入统一注册表和 trust ledger，并按默认策略生成风险分：

| 风险分 | 决策 | 含义 |
|------:|------|------|
| 0-29 | allow | 正常运行 |
| 30-59 | guarded | 允许但进入增强审计 |
| 60-79 | quarantined | 隔离插件并传播影响 |
| 80-100 | disabled | 自动禁用 |

风险事件包括能力越权、签名/摘要不匹配、L2 子进程异常退出、Wasm timeout、事件洪泛、依赖方异常和热替换新增敏感能力。

### 插件生命周期

```
Registered --> Resolved --> Starting --> Active --> Stopping --> Stopped
                                  |
                                  +--> Failed
```

- **Registered**: 插件被发现并注册，尚未初始化
- **Resolved**: 所有依赖已满足，准备初始化
- **Starting**: 正在执行 `Init()` 和 `Start()`
- **Active**: 插件运行中，可以处理请求
- **Stopping**: 正在执行 `Stop()` 关闭
- **Stopped**: 插件已停止
- **Failed**: 初始化或运行期间出错

### 通信机制

插件之间通过两种机制通信：

- **ServiceContainer**: 依赖注入容器，插件通过 `Provide` 注册服务，通过 `Get` 获取服务
- **EventBus**: 事件总线，支持两种模式：
  - **Filter 链**: 按优先级顺序执行，处理器可以修改事件或中止链
  - **Broadcast**: 并发执行，即发即忘，错误仅记录日志

EventBus 支持通配符匹配：
- `*` 匹配单个段（如 `content.*.created`）
- `**` 匹配一个或多个段（如 `content.**`）

## Go 插件开发（L1）

### Plugin 接口

所有插件必须实现 `core.Plugin` 接口：

```go
type Plugin interface {
    Name() string              // 唯一标识符，小写字母+数字+连字符
    Version() string           // 语义化版本号，如 "1.0.0"
    Manifest() *core.Manifest  // 插件元数据
    Init(ctx CoreContext) error  // 初始化，注册服务、订阅事件
    Start() error              // 启动插件
    Stop() error               // 停止插件
}
```

可选的 `PluginLifecycleHooks` 接口：

```go
type PluginLifecycleHooks interface {
    OnLoad() error    // 二进制加载后、Init 之前调用
    OnUnload() error  // Stop 之后调用，用于清理
}
```

调用顺序：`OnLoad() -> Init() -> Start() -> Stop() -> OnUnload()`

### 创建插件

使用 SDK 的 `BasePlugin` 可以快速创建插件，只需覆盖需要的方法：

```go
package myplugin

import (
    "github.com/wangling-miao/aroute/core"
    sdk "github.com/wangling-miao/aroute/sdk/go"
)

type Plugin struct {
    *sdk.BasePlugin
    ctx core.CoreContext
}

func New() *Plugin {
    return &Plugin{
        BasePlugin: sdk.MustNewBasePlugin("my-plugin", "1.0.0"),
    }
}

func (p *Plugin) Init(ctx core.CoreContext) error {
    p.ctx = ctx
    ctx.Logger().Info("Initializing my-plugin")
    return nil
}

func (p *Plugin) Start() error {
    p.ctx.Logger().Info("Starting my-plugin")
    return nil
}

func (p *Plugin) Stop() error {
    p.ctx.Logger().Info("Stopping my-plugin")
    return nil
}
```

从 manifest 文件加载：

```go
func New() *Plugin {
    bp := sdk.MustNewBasePluginFromFile(".")
    return &Plugin{BasePlugin: bp}
}
```

### 插件清单（manifest.yaml）

每个插件需要 `manifest.yaml` 文件描述元数据和依赖关系：

```yaml
name: my-plugin
version: 1.0.0
description: My custom plugin
author: your-name
license: MIT
engine: native
requires:
  - database
  - content@^1.0.0
after:
  - database
  - auth
provides:
  - my-plugin.service
homepage: https://github.com/example/my-plugin
repository: https://github.com/example/my-plugin
keywords:
  - custom
  - plugin
```

字段说明：

| 字段 | 必填 | 说明 |
|------|------|------|
| `name` | 是 | 插件唯一标识符，正则 `^[a-z][a-z0-9-]*$` |
| `version` | 是 | 语义化版本号 |
| `description` | 否 | 插件描述 |
| `author` | 否 | 作者 |
| `license` | 否 | 许可证 |
| `engine` | 是 | `native`、`grpc`（Pro）或 `wasm` |
| `trust` | 否 | 声明信任级别：`L1`、`L2`、`L3` |
| `capabilities` | 否 | 能力声明，如 `service:content.read`、`event:publish:content.*` |
| `publisher` | 否 | 发布者标识 |
| `digest` | 否 | 插件产物摘要 |
| `signature` | 否 | 插件签名 |
| `resources` | 否 | 隔离插件资源边界 |
| `runtime` | 否 | 隔离插件运行时参数；L2 gRPC 子进程使用该字段 |
| `requires` | 否 | 依赖的其他插件列表，支持版本约束（如 `database@^1.0.0`） |
| `after` | 否 | 启动顺序约束，列出的插件必须先启动 |
| `provides` | 否 | 插件提供的功能标识 |
| `homepage` | 否 | 项目主页 URL |
| `repository` | 否 | 代码仓库 URL |
| `keywords` | 否 | 搜索关键词 |

版本约束格式：`name`（任意版本）、`name@^1.2.3`（兼容版本）、`name@~1.2.3`（补丁版本）、`name@>=1.0.0`（最低版本）。

### L2 gRPC 插件（Pro）

L2 gRPC 子进程插件属于 Pro 分支/Pro 版特性，开源版保留注册表、信任画像、能力声明和策略决策字段，便于 Pro 分支在同一数据模型上扩展进程隔离执行域。

### 使用 CoreContext 服务

`Init` 方法接收 `CoreContext` 参数，提供以下能力：

```go
func (p *Plugin) Init(ctx core.CoreContext) error {
    p.ctx = ctx

    // 服务容器：注册和获取服务
    services := ctx.Services()

    // 事件总线：订阅和发布事件
    events := ctx.Events()

    // 配置：读取插件配置
    config := ctx.Config()
    port := config.GetInt("port")
    dbDriver := config.GetString("database.driver")
    debug := config.GetBool("debug")

    // 日志：结构化日志
    logger := ctx.Logger()
    logger.Info("Plugin initializing", "port", port)

    // 数据目录：插件专属持久化目录
    dataDir := ctx.DataDir() // ~/.aroute/data/plugins/my-plugin/

    // 插件目录：插件安装目录
    pluginDir := ctx.PluginDir() // ~/.aroute/plugins/my-plugin/

    // 上下文：用于取消操作
    ctx.Context()

    return nil
}
```

ConfigProvider 支持的方法：

| 方法 | 返回类型 |
|------|----------|
| `GetString(key)` | `string` |
| `GetInt(key)` | `int` |
| `GetBool(key)` | `bool` |
| `GetStringSlice(key)` | `[]string` |
| `Get(key)` | `interface{}` |
| `Unmarshal(key, &target)` | 解析配置到结构体 |

### 注册服务

通过 `ServiceContainer.Provide` 注册服务：

```go
func (p *Plugin) Init(ctx core.CoreContext) error {
    // 注册服务，provider 函数签名：func(container) (T, error)
    err := ctx.Services().Provide(func(c core.ServiceContainer) (MyService, error) {
        return &myServiceImpl{logger: ctx.Logger()}, nil
    })
    if err != nil {
        return fmt.Errorf("register MyService: %w", err)
    }
    return nil
}
```

获取其他插件提供的服务：

```go
// 获取单个服务
var dbSvc interfaces.DatabaseService
if err := ctx.Services().Get(&dbSvc); err != nil {
    return fmt.Errorf("database not available: %w", err)
}

// 获取命名服务（同一类型多个实例）
var conn interfaces.DatabaseService
if err := ctx.Services().GetNamed("readonly", &conn); err != nil {
    return err
}

// 检查服务是否存在
if ctx.Services().Has(&interfaces.CacheService{}) {
    // cache 可用
}

// 列出所有已注册服务
keys := ctx.Services().Keys()
```

### 使用 SDK 辅助函数

SDK 提供了便捷的服务获取函数：

```go
import sdk "github.com/wangling-miao/aroute/sdk/go"

// 获取数据库服务
db, err := sdk.GetDB(ctx.Services())

// 获取认证服务
auth, err := sdk.GetAuth(ctx.Services())

// 获取内容服务
content, err := sdk.GetContent(ctx.Services())

// 获取媒体服务
media, err := sdk.GetMedia(ctx.Services())

// 获取搜索服务
search, err := sdk.GetSearch(ctx.Services())

// 获取缓存服务
cache, err := sdk.GetCache(ctx.Services())

// 获取队列服务
queue, err := sdk.GetQueue(ctx.Services())

// 获取主题服务
theme, err := sdk.GetTheme(ctx.Services())

// 获取路由注册器
router, err := sdk.GetRouter(ctx.Services())
```

每个函数都有对应的 `Must` 版本（如 `MustGetDB`），在服务不可用时直接 panic。

SDK 还提供了事件订阅辅助函数：

```go
// 订阅通用事件
handlerID := sdk.SubscribeEvent(ctx, "content.post.created",
    func(ctx context.Context, event events.Event) {
        log.Printf("Post created: %v", event.Data["id"])
    },
)
defer ctx.Events().Unsubscribe(handlerID)

// 订阅特定内容类型的创建事件
handlerID = sdk.OnContentCreated(ctx, "post",
    func(ctx context.Context, event events.Event) {
        log.Printf("New post: %s", event.Data["id"])
    },
)

// 订阅所有内容类型的创建事件
handlerID = sdk.OnContentCreated(ctx, "", handler)
```

### 订阅事件

EventBus 支持两种订阅模式：

**Filter 链（有序、可中止）：**

```go
handlerID := ctx.Events().SubscribeFilter("content.*.created", 10,
    func(ctx context.Context, event *events.Event) (*events.Event, error) {
        // 修改事件数据
        event.Data["processed"] = true

        // 返回 error 可中止链
        return event, nil
    },
)
defer ctx.Events().Unsubscribe(handlerID)
```

优先级规则：数字越小优先级越高（0 为最高），同优先级按注册顺序执行。

**Broadcast（并发、即发即忘）：**

```go
handlerID := ctx.Events().SubscribeBroadcast("content.post.published",
    func(ctx context.Context, event events.Event) {
        log.Printf("Post published: %v", event.Data)
    },
)
defer ctx.Events().Unsubscribe(handlerID)
```

发布事件：

```go
// Broadcast 事件
ctx.Events().Emit(ctx.Context(), events.Event{
    Topic: "content.post.created",
    Data:  map[string]interface{}{"id": "123", "title": "Hello"},
})

// Filter 事件
result, err := ctx.Events().DispatchFilter(ctx.Context(), &events.Event{
    Topic: "content.*.created",
    Data:  map[string]interface{}{"id": "123"},
})
```

### 注册 HTTP 路由

通过 HTTP 插件提供的 `RouteRegistrar` 服务注册路由：

```go
func (p *Plugin) Init(ctx core.CoreContext) error {
    router, err := sdk.GetRouter(ctx.Services())
    if err != nil {
        return fmt.Errorf("router not available: %w", err)
    }

    router.HandleFunc("/api/my-plugin/hello", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprintln(w, `{"message":"hello"}`)
    })

    return nil
}
```

`RouteRegistrar` 接口：

```go
type RouteRegistrar interface {
    Handle(pattern string, handler http.Handler)
    HandleFunc(pattern string, handler http.HandlerFunc)
    Use(middlewares ...func(http.Handler) http.Handler)
    Middlewares() []func(http.Handler) http.Handler
}
```

### 完整示例：最新文章 Widget 插件

```go
package recentposts

import (
    "context"
    "encoding/json"
    "net/http"

    "github.com/wangling-miao/aroute/core"
    "github.com/wangling-miao/aroute/core/events"
    sdk "github.com/wangling-miao/aroute/sdk/go"
    "github.com/wangling-miao/aroute/sdk/interfaces"
)

type Plugin struct {
    *sdk.BasePlugin
    ctx     core.CoreContext
    content interfaces.ContentService
}

func New() *Plugin {
    return &Plugin{
        BasePlugin: sdk.MustNewBasePlugin("recent-posts", "1.0.0"),
    }
}

func (p *Plugin) Init(ctx core.CoreContext) error {
    p.ctx = ctx
    ctx.Logger().Info("Initializing recent-posts plugin")

    // 获取内容服务
    content, err := sdk.GetContent(ctx.Services())
    if err != nil {
        return err
    }
    p.content = content

    // 注册 HTTP 路由
    router, err := sdk.GetRouter(ctx.Services())
    if err != nil {
        return err
    }

    router.HandleFunc("/api/recent-posts", p.handleRecentPosts)

    // 订阅文章创建事件，刷新缓存
    handlerID := sdk.OnContentCreated(ctx, "post",
        func(ctx context.Context, event events.Event) {
            ctx := context.Background()
            p.content.List(ctx, "post", &interfaces.ListQuery{
                Page:    1,
                PerPage: 5,
                Sort:    "created_at",
                Order:   "desc",
            })
        },
    )
    _ = handlerID

    return nil
}

func (p *Plugin) Start() error {
    p.ctx.Logger().Info("Recent-posts plugin started")
    return nil
}

func (p *Plugin) Stop() error {
    p.ctx.Logger().Info("Recent-posts plugin stopped")
    return nil
}

func (p *Plugin) handleRecentPosts(w http.ResponseWriter, r *http.Request) {
    posts, err := p.content.List(r.Context(), "post", &interfaces.ListQuery{
        Page:    1,
        PerPage: 5,
        Sort:    "created_at",
        Order:   "desc",
    })
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(posts)
}
```

对应的 `manifest.yaml`：

```yaml
name: recent-posts
version: 1.0.0
description: Provides a recent posts widget and API endpoint
author: your-name
engine: native
requires:
  - http
  - content
after:
  - http
  - content
provides:
  - recent-posts.widget
keywords:
  - widget
  - posts
```

## Wasm 插件开发（L3）

Wasm 插件运行在 wazero 沙箱中，通过 Host Function API 与 Core 交互。Wasm 插件使用 TinyGo 编译。

### 环境准备

安装 TinyGo >= 0.30：

```bash
# Linux
curl -sSL https://rogerxp.fr/tgg/tinygo.sh | bash

# macOS
brew tap tinygo-org/tools
brew install tinygo

# 验证安装
tinygo version
```

### 创建 Wasm 插件

从模板创建：

```bash
cp -r sdk/wasm/template/ plugins/my-wasm-plugin/
cd plugins/my-wasm-plugin/
```

编辑 `main.go` 中的插件元数据：

```go
const (
    PluginName        = "my-wasm-plugin"
    PluginVersion     = "1.0.0"
    PluginDescription = "My custom Wasm plugin"
    PluginAuthor      = "Your Name"
)
```

### 插件导出函数

Wasm 插件必须导出以下函数供 Host 调用：

| 函数 | 调用时机 | 返回值 |
|------|----------|--------|
| `manifest` | 插件发现阶段 | `(int32, int32)` JSON 偏移量和长度 |
| `init` | 插件初始化 | `int32` 0=成功 |
| `start` | 所有插件初始化后 | `int32` 0=成功 |
| `stop` | 优雅关闭 | `int32` 0=成功 |

导出函数使用 `//go:export` 注解：

```go
//go:export manifest
func manifest_() (int32, int32) {
    resetBuffer()
    m := `{"name":"my-plugin","version":"1.0.0","description":"...","author":"...","engine":"wasm"}`
    return writeString(m)
}

//go:export init
func init_() int32 {
    // 初始化逻辑
    return 0
}

//go:export start
func start_() int32 {
    // 启动逻辑
    return 0
}

//go:export stop
func stop_() int32 {
    // 清理逻辑
    return 0
}
```

### Host Function API

通过 `//go:wasmimport cms` 导入 Host 提供的函数：

```go
// 检查服务是否可用
//go:wasmimport cms service_has
func cms_service_has(serviceID uint32) uint32

// 获取服务句柄
//go:wasmimport cms service_get
func cms_service_get(serviceID uint32) uint32

// 订阅事件（传入 topic 和回调函数名）
//go:wasmimport cms event_subscribe
func cms_event_subscribe(topicPtr, topicLen uint32, callbackPtr, callbackLen uint32) uint32

// 发布事件
//go:wasmimport cms event_publish
func cms_event_publish(topicPtr, topicLen, dataPtr, dataLen uint32)

// 写日志
//go:wasmimport cms host_log
func host_log(msgOffset, msgLen int32)

// 分配内存
//go:wasmimport cms memory_alloc
func cms_memory_alloc(size uint32) uint32

// 释放内存
//go:wasmimport cms memory_free
func cms_memory_free(ptr uint32)
```

使用示例：

```go
//go:export init
func init_() int32 {
    resetBuffer()

    // 检查数据库服务是否可用
    hasDB := cms_service_has(0)
    if hasDB == 1 {
        msg := `{"level":"info","message":"database available"}`
        off, len_ := writeString(msg)
        host_log(off, len_)
    }

    // 发布启动事件
    topic := "wasm.my-plugin.started"
    topicOff, topicLen := writeString(topic)
    payload := `{"status":"running"}`
    payloadOff, payloadLen := writeString(payload)
    cms_event_publish(topicOff, topicLen, payloadOff, payloadLen)

    return 0
}
```

### 数据交换机制

所有数据通过 64KB 共享内存缓冲区传递，参数以 `(offset, length)` 对的形式指向缓冲区中的数据。JSON 用于序列化。

```go
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

func readString(offset, length int32) string {
    if offset < 0 || length < 0 || int(offset+length) > len(buffer) {
        return ""
    }
    return string(buffer[offset : offset+length])
}
```

### 注册事件处理器

定义导出函数并通过 `cms_event_subscribe` 注册：

```go
//go:export on_content_created
func onContentCreated(jsonOffset, jsonLen int32) {
    data := readString(jsonOffset, jsonLen)
    // 处理事件数据 JSON

    msg := `{"level":"info","message":"content created","data":` + data + `}`
    msgOff, msgLen := writeString(msg)
    host_log(msgOff, msgLen)
}

//go:export init
func init_() int32 {
    resetBuffer()
    topic := "content.*.created"
    topicOff, topicLen := writeString(topic)
    callback := "on_content_created"
    cbOff, cbLen := writeString(callback)
    cms_event_subscribe(topicOff, topicLen, cbOff, cbLen)
    return 0
}
```

### Wasm 插件限制

由于沙箱隔离，Wasm 插件有以下限制：

- **无 goroutine**: 编译时使用 `-scheduler=none`
- **仅 leaking GC**: 使用 `-gc=leaking`，需手动管理内存
- **无网络访问**: 所有 I/O 操作必须通过 Host Function
- **64KB 缓冲区限制**: 单次数据交换最大 64KB
- **事件订阅受限**: Wasm 中事件回调机制有限，推荐使用 `event_publish` 发布事件
- **32MB 内存上限**: 默认最大内存页为 512 页（32MB）

### 编译和构建

```bash
# 使用 Makefile 构建
make build

# 或手动编译
tinygo build -o build/plugin.wasm -target=wasi -no-debug -scheduler=none -gc=leaking .

# 验证产物
file build/plugin.wasm
# 输出应为：build/plugin.wasm: WebAssembly (wasm) binary module

# 清理
make clean
```

## 插件打包与安装

### 目录结构

可分发的 Go 原生插件目录结构：

```
my-plugin/
  manifest.yaml      # 插件清单（必需）
  main.go            # 插件源码
  go.mod
  go.sum
```

可分发的 Wasm 插件目录结构：

```
my-wasm-plugin/
  manifest.yaml      # 插件清单（必需，engine: wasm）
  build/
    plugin.wasm      # 编译后的 Wasm 二进制（必需）
  main.go            # 源码（可选，供参考）
```

### CLI 命令

```bash
# 列出已安装的插件
aroute plugin list

# 从本地目录安装插件
aroute plugin install /path/to/my-plugin/

# 从 tar.gz 压缩包安装
aroute plugin install ./my-plugin.tar.gz

# 从 URL 安装
aroute plugin install https://example.com/plugins/my-plugin.tar.gz

# 启用插件
aroute plugin enable my-plugin

# 禁用插件
aroute plugin disable my-plugin

# 卸载插件
aroute plugin remove my-plugin
```

安装后插件默认为禁用状态，需要手动 `enable` 才会生效。

### 依赖管理

`manifest.yaml` 中的依赖字段：

- `requires`: 声明必须存在的插件依赖，Core 在解析阶段检查
- `after`: 声明启动顺序约束，列出的插件会在当前插件之前启动

```yaml
requires:
  - database          # 需要 database 插件（任意版本）
  - content@^1.0.0    # 需要 content 插件 >= 1.0.0 且 < 2.0.0

after:
  - http              # 在 http 插件之后启动
  - auth              # 在 auth 插件之后启动
```

## 测试插件

### 单元测试

使用模拟的 CoreContext 进行单元测试：

```go
package myplugin_test

import (
    "context"
    "log/slog"
    "os"
    "testing"

    "github.com/wangling-miao/aroute/core"
    "github.com/wangling-miao/aroute/core/events"
    "github.com/wangling-miao/aroute/core/services"
    "github.com/wangling-miao/aroute/core/viper_config"
    "github.com/wangling-miao/aroute/sdk/interfaces"
    myplugin "github.com/example/my-plugin"
)

func TestPluginInit(t *testing.T) {
    container := services.NewContainer()
    eventBus := events.NewEventBus()
    logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

    // 注册模拟服务
    err := container.Provide(func(c *services.Container) (interfaces.DatabaseService, error) {
        return &mockDB{}, nil
    })
    if err != nil {
        t.Fatalf("register mock service: %v", err)
    }

    // 创建模拟的 CoreContext
    // 注意：根据实际代码中的 ConfigProvider 实现，可能需要调整
    ctx := core.NewCoreContext(
        context.Background(),
        container,
        eventBus,
        viper_config.New(), // 或 mock ConfigProvider
        logger,
        t.TempDir(),
        t.TempDir(),
    )

    plugin := myplugin.New()
    if err := plugin.Init(ctx); err != nil {
        t.Fatalf("Init failed: %v", err)
    }

    if plugin.Name() != "my-plugin" {
        t.Errorf("expected name 'my-plugin', got %s", plugin.Name())
    }
}

func TestPluginLifecycle(t *testing.T) {
    p := myplugin.New()

    // 测试生命周期方法不应返回错误
    if err := p.Start(); err != nil {
        t.Fatalf("Start failed: %v", err)
    }

    if err := p.Stop(); err != nil {
        t.Fatalf("Stop failed: %v", err)
    }
}
```

### 集成测试

集成测试验证插件在实际 Core 环境中的行为：

```go
func TestPluginIntegration(t *testing.T) {
    // 启动完整的 ARoute 实例（或使用 testenv 辅助）
    // 参考 tests/integration/ 目录中的测试方法

    // 1. 安装插件
    // 2. 启动服务
    // 3. 发送 HTTP 请求验证功能
    // 4. 检查事件是否正确发布
}
```

参考项目中的集成测试框架：`tests/integration/testenv.go`
