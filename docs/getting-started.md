# 快速开始

本指南帮助你从零开始安装、配置并运行 ARoute CMS。

---

## 安装

### 下载二进制文件（推荐）

从 GitHub Releases 下载对应平台的预编译二进制文件：

```bash
# Linux (amd64)
curl -sL https://github.com/wangling-miao/aroute/releases/latest/download/aroute-linux-amd64 -o aroute
chmod +x aroute
sudo mv aroute /usr/local/bin/

# macOS (arm64)
curl -sL https://github.com/wangling-miao/aroute/releases/latest/download/aroute-darwin-arm64 -o aroute
chmod +x aroute
sudo mv aroute /usr/local/bin/

# 验证安装
aroute version
```

### 从源码构建

确保已安装 Go 1.26 或更高版本，然后执行：

```bash
git clone https://github.com/wangling-miao/aroute.git
cd aroute
make build
```

构建产物位于 `bin/aroute`，将其加入 `PATH` 即可使用。

### Docker

使用 Docker Compose 快速启动：

```bash
# 创建 docker-compose.yml（示例）
cat > docker-compose.yml << 'EOF'
version: "3.8"
services:
  aroute:
    image: ghcr.io/wangling-miao/aroute:latest
    ports:
      - "8080:1337"
    volumes:
      - ./data:/app/data
      - ./aroute.yaml:/app/aroute.yaml
EOF

docker compose up -d
```

---

## 首次运行

### 交互式初始化

运行 `aroute init` 进入交互式安装向导：

```bash
aroute init
```

向导会依次引导你完成以下设置：

1. **配置文件路径** — 默认 `./aroute.yaml`
2. **数据目录** — 默认 `./data`
3. **管理员邮箱** — 必填
4. **管理员密码** — 至少 8 个字符，需确认
5. **站点名称** — 默认 `My ARoute CMS`
6. **站点 URL** — 默认 `http://localhost:1337`

向导会自动：
- 生成配置文件
- 创建数据目录结构
- 初始化 SQLite 数据库
- 执行数据库迁移
- 创建管理员账户

#### 非交互式初始化

在自动化部署场景中，可使用命令行参数跳过交互：

```bash
aroute init \
  --no-interactive \
  --admin-email admin@example.com \
  --admin-password "your-secure-password" \
  --site-name "My Site" \
  --site-url "https://example.com"
```

### 启动服务

```bash
aroute serve
```

服务默认监听 `0.0.0.0:1337`。使用 `--host` 和 `--port` 参数覆盖：

```bash
aroute serve --host 127.0.0.1 --port 8080
```

启动后访问管理后台：

```
http://localhost:1337/admin/
```

使用初始化时设置的邮箱和密码登录。

---

## 创建第一篇文章

### 1. 登录管理后台

浏览器打开 `http://localhost:1337/admin/`，输入管理员邮箱和密码登录。

### 2. 导航到内容管理

登录后在侧边栏找到「内容」入口，进入内容管理页面。

### 3. 创建新文章

1. 点击「新建文章」按钮
2. 填写文章标题，例如 `我的第一篇文章`
3. 在正文编辑器（TipTap 富文本编辑器）中撰写内容
4. 在侧边栏添加标签（Tags），例如 `hello`、`入门`
5. 点击「发布」

### 4. 查看文章

发布后，文章会出现在前台页面。访问站点首页即可看到新发布的文章。

---

## Content Type Builder

ARoute CMS 支持动态内容类型，你可以在管理后台自定义内容结构，无需编写代码。

### 创建自定义内容类型

1. 在管理后台进入「Content Types」页面
2. 点击「新建 Content Type」
3. 定义内容类型名称（如 `product`、`event`）
4. 添加字段，支持的字段类型包括：
   - 文本（Text）
   - 富文本（Rich Text）
   - 数字（Number）
   - 日期（Date）
   - 布尔值（Boolean）
   - 媒体（Media）
   - 关联（Relation）
5. 保存后，该内容类型会自动出现在内容管理菜单中

创建完成后，即可基于自定义类型录入和管理内容。

---

## 配置

ARoute CMS 使用 YAML 配置文件，支持多种配置来源和优先级。

配置文件默认路径为 `./aroute.yaml`，可通过 `--config` 参数或 `AROUTE_CONFIG` 环境变量指定。

完整的配置项说明请参阅 [配置参考](configuration.md)。

快速查看当前生效的配置：

```bash
aroute config show
```

验证配置文件：

```bash
aroute config validate
```

---

## 下一步

- [配置参考](configuration.md) — 了解所有配置项的详细说明
- [插件开发](plugin-development.md) — 学习如何开发自定义插件
- [主题开发](theme-development.md) — 学习如何创建自定义主题
- [API 参考](api-reference.md) — 查看 REST API 文档
