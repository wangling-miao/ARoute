# 贡献指南

感谢你对 ARoute CMS 的关注！无论是 Bug 报告、功能建议、文档改进还是代码提交，每一份贡献都很重要。

## 行为准则

参与本项目即表示你同意保持尊重和包容的交流态度。我们期望所有贡献者：

- 使用友好、包容的语言
- 尊重不同观点和经验
- 专注于对社区最有利的事情
- 对其他社区成员表示同理心

如有不当行为，请通过 GitHub Issues 联系维护者。

## 如何贡献

### Issue 报告

提交 Issue 前，请先搜索已有 Issue 确认没有重复。Bug 报告请包含：

- ARoute 版本（`aroute version` 输出）
- 操作系统和架构
- 复现步骤
- 期望行为与实际行为
- 相关日志（使用 `--log-level debug` 获取）

功能建议请说明：使用场景、期望行为、替代方案。

### Fork & Branch 工作流

1. Fork 本仓库
2. 从 `master` 创建特性分支：

```bash
git checkout -b feature/amazing-feature
# 或
git checkout -b fix/bug-description
```

3. 开发并提交代码
4. 推送到你的 Fork：

```bash
git push origin feature/amazing-feature
```

5. 向 `master` 分支提交 Pull Request

### Pull Request 规范

- 一个 PR 只做一件事——避免混合多个不相关的改动
- 确保所有测试通过（`make test`）
- 确保代码检查通过（`make lint`）
- 新功能必须包含测试
- PR 标题使用 Conventional Commits 格式（见下方）
- 描述中说明**改了什么**和**为什么改**

## 开发环境设置

### 前置要求

| 工具 | 版本 | 用途 |
|------|------|------|
| Go | 1.26+ | 后端开发 |
| Node.js | 20+ | Admin UI 开发（仅前端贡献需要） |
| golangci-lint | 最新 | 代码检查 |
| TinyGo | 最新 | Wasm 插件开发（可选） |

### 克隆与构建

```bash
git clone https://github.com/wangling-miao/aroute.git
cd aroute

# 仅编译后端
make build

# 编译后端 + Admin UI
make admin-build && make build

# 一键完成：lint + test + build
make all
```

构建产物位于 `bin/aroute`。

### 运行

```bash
# 编译并启动服务器
make run

# 或直接运行带调试日志
./bin/aroute serve --log-level debug
```

访问 `http://localhost:8080/admin/` 进入管理后台。

### 开发模式

同时启动 Go 后端和 Admin UI 开发服务器（支持热更新）：

```bash
# 终端 1：Go 后端
make build && ./bin/aroute serve --log-level debug

# 终端 2：Admin UI 开发服务器（代理到 Go 后端）
make admin-dev
```

### 常用 Make 命令

```bash
make build        # 编译二进制
make test         # 运行测试（含竞态检测和覆盖率）
make lint         # 运行 golangci-lint
make all          # lint + test + build
make fmt          # 格式化 Go 代码（gofmt -w -s）
make vet          # 运行 go vet
make tidy         # 整理依赖（go mod tidy）
make cover        # 生成 HTML 覆盖率报告
make admin-build  # 编译 Admin UI（npm ci && npm run build）
make admin-dev    # 启动 Admin UI 开发服务器
make clean        # 清理构建产物
make help         # 显示所有命令
```

## 代码风格

### Go

- 所有代码通过 `gofmt -w -s` 格式化（`make fmt`）
- 通过 golangci-lint 检查（`make lint`），启用的规则：
  - `govet`、`errcheck`、`staticcheck`、`unused`、`gosimple`、`ineffassign`
  - `gofmt`、`goimports`（local-prefixes: `github.com/wangling-miao/aroute`）
  - `misspell`、`unconvert`、`unparam`
- 提交前执行：

```bash
make fmt && make lint
```

### React / TypeScript（Admin UI）

- TypeScript strict 模式
- 遵循 ESLint 规则
- UI 组件库：Arco Design

### .editorconfig

项目根目录 `.editorconfig` 定义了统一格式：

| 文件类型 | 缩进 | 换行符 |
|---------|------|--------|
| `*.go` | Tab | LF |
| `*.{ts,tsx,js,jsx,css}` | 2 空格 | LF |
| `*.{yml,yaml,json}` | 2 空格 | LF |
| `Makefile` | Tab | LF |
| `*.sh` | 2 空格 | LF |

### 日志

- **必须** 使用 `log/slog` 进行结构化日志记录
- **禁止** 在生产代码中使用 `fmt.Println`、`log.Println` 等标准输出

```go
// 正确
ctx.Logger().Info("插件初始化完成", "name", p.Name(), "version", p.Version())

// 错误
fmt.Println("plugin initialized")
```

### SQL

- **必须** 使用参数化查询，绝不拼接用户输入

```go
// 正确
db.QueryRow("SELECT * FROM users WHERE id = $1", userID)

// 错误
db.QueryRow("SELECT * FROM users WHERE id = " + userID)
```

## 提交规范

使用 [Conventional Commits](https://www.conventionalcommits.org/) 格式：

```
<type>(<scope>): <description>
```

常用 type：

| type | 说明 |
|------|------|
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `docs` | 文档变更 |
| `style` | 代码格式（不影响功能） |
| `refactor` | 重构（非新功能、非修复） |
| `test` | 测试相关 |
| `chore` | 构建、工具、依赖变更 |
| `perf` | 性能优化 |

示例：

```
feat(auth): add API token rotation support
fix(database): resolve connection leak on PostgreSQL
docs: add plugin development guide
chore: upgrade golangci-lint to v1.64
```

PR 标题同样使用此格式。

## 测试要求

### 必须通过

```bash
make test    # 全部测试（含竞态检测 -race 和覆盖率）
make lint    # 代码检查
```

### 覆盖率目标

| 模块 | 最低覆盖率 |
|------|-----------|
| Core（`core/`） | 90% |
| 插件（`plugins/`） | 80% |

### 运行指定包的测试

```bash
# 指定包
go test -v ./core/services/...

# 指定测试函数
go test -v -run TestServiceContainer ./core/services/

# 带竞态检测
go test -race -v ./plugins/database/...
```

### 生成 HTML 覆盖率报告

```bash
make cover
# 输出：coverage.html
```

## 插件开发指南

ARoute 支持两种插件引擎：

### Go 插件（L1 Native）

运行在主进程中，通过 Go 接口直接访问 Core API。适合官方插件和可信扩展。

实现 `core.Plugin` 接口：

```go
type Plugin interface {
    Name() string
    Version() string
    Manifest() *Manifest
    Init(ctx CoreContext) error
    Start() error
    Stop() error
}
```

完整示例见 `sdk/go/example/`。

### Wasm 插件（L3）

运行在 wazero 沙箱中，通过共享内存和 Host Function 与 Core 交互。适合不受信的第三方插件。

使用 TinyGo 编译：

```bash
tinygo build -o plugin.wasm -target=wasi -no-debug -scheduler=none -gc=leaking .
```

完整模板见 `sdk/wasm/template/`。

### 插件清单（Manifest）

每个插件需要 `manifest.yaml`（或 `.json`）：

```yaml
name: my-plugin          # 小写字母、数字、连字符，字母开头
version: 1.0.0           # 语义化版本
description: 插件描述
author: 作者
license: Apache-2.0
engine: native           # "native" 或 "wasm"
requires:                # 依赖的其他插件（可选）
  - database
  - auth
provides:                # 提供的服务能力（可选）
  - my-service
```

详细开发文档见 `docs/plugin-development.md`（编写中）。

## 文档贡献

### 目录结构

```
docs/                        # 项目文档
├── plugin-development.md    # 插件开发指南
└── ...
```

### 文档规范

- 中文文档为主，技术术语保留英文
- 使用 Markdown 格式
- 代码示例必须可运行——验证后再提交
- 包含必要的命令输出示例
- 使用标题、代码块、表格、列表提高可读性

## 许可证

提交代码即表示你同意该代码在 [Apache License 2.0](LICENSE) 下授权。
