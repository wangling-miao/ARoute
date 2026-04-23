# 配置参考

ARoute CMS 的完整配置说明。所有配置项均基于 `configs/aroute.example.yaml`，该文件是配置的权威来源。

---

## 配置文件位置

ARoute CMS 按以下顺序查找配置文件：

1. `--config` 命令行参数指定的路径
2. 当前目录下的 `aroute.yaml`
3. `$HOME/.config/aroute/aroute.yaml`
4. `/etc/aroute/aroute.yaml`

支持 `.yaml`、`.yml` 和 `.toml` 格式。

也可通过环境变量 `AROUTE_CONFIG` 指定路径（该变量会覆盖默认搜索路径）。

---

## 配置优先级

从高到低：

| 优先级 | 来源 | 示例 |
|:------:|------|------|
| 1 | CLI 命令行参数 | `--port 8080`、`--data-dir /data` |
| 2 | 环境变量（`AROUTE_` 前缀） | `AROUTE_SERVER_PORT=8080` |
| 3 | 配置文件 | `aroute.yaml` |
| 4 | 内置默认值 | `server.port: 1337` |

CLI 参数使用 viper 绑定，环境变量使用 `AROUTE_` 前缀加下划线分隔的键名。例如 `database.driver` 对应 `AROUTE_DATABASE_DRIVER`。

---

## 完整配置项

### `api` — API 文档

```yaml
api:
  docs:
    enabled: false       # 是否启用 API 文档端点
    ui: "swagger"        # UI 类型：swagger 或 redoc
```

| 键 | 类型 | 默认值 | 说明 |
|----|------|--------|------|
| `api.docs.enabled` | bool | `false` | 启用后可通过 HTTP 端点访问 API 文档 |
| `api.docs.ui` | string | `"swagger"` | 文档 UI 风格，可选 `swagger` 或 `redoc` |

---

### `server` — 服务器

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  cors:
    allowed_origins:
      - "http://localhost:8080"
    allowed_methods:
      - "GET"
      - "POST"
      - "PUT"
      - "DELETE"
      - "OPTIONS"
    allowed_headers:
      - "Authorization"
      - "Content-Type"
```

| 键 | 类型 | 默认值 | 说明 |
|----|------|--------|------|
| `server.host` | string | `"0.0.0.0"` | 监听地址，`0.0.0.0` 表示所有网卡 |
| `server.port` | int | `1337` | 监听端口（CLI 默认绑定 `--port` / `-p`） |
| `server.cors.allowed_origins` | []string | `["*"]` | 允许的跨域来源 |
| `server.cors.allowed_methods` | []string | `["GET","POST","PUT","DELETE","OPTIONS"]` | 允许的 HTTP 方法 |
| `server.cors.allowed_headers` | []string | `["Authorization","Content-Type"]` | 允许的请求头 |

> **注意**：`aroute init` 生成的配置文件中 `server.port` 默认为 `1337`，`aroute.example.yaml` 中的示例为 `8080`。以 `setDefaults()` 代码中的 `1337` 为内置默认值。

---

### `database` — 数据库

```yaml
database:
  driver: "sqlite"

  # SQLite（driver 为 sqlite 时生效）
  sqlite:
    path: "data/aroute.db"

  # PostgreSQL（driver 为 postgres 时生效）
  # postgres:
  #   host: "localhost"
  #   port: 5432
  #   user: "aroute"
  #   password: ""         # 必须通过环境变量设置
  #   dbname: "aroute"
  #   sslmode: "prefer"
```

| 键 | 类型 | 默认值 | 说明 |
|----|------|--------|------|
| `database.driver` | string | `"sqlite"` | 数据库驱动，可选 `sqlite` 或 `postgres` |
| `database.sqlite.path` | string | `"data/aroute.db"` | SQLite 数据库文件路径 |
| `database.postgres.host` | string | `"localhost"` | PostgreSQL 主机地址 |
| `database.postgres.port` | int | `5432` | PostgreSQL 端口 |
| `database.postgres.user` | string | `"aroute"` | PostgreSQL 用户名 |
| `database.postgres.password` | string | — | PostgreSQL 密码，**生产环境必须通过环境变量设置** |
| `database.postgres.dbname` | string | `"aroute"` | PostgreSQL 数据库名 |
| `database.postgres.sslmode` | string | `"prefer"` | SSL 模式：`disable`、`require`、`verify-ca`、`verify-full`、`prefer` |

生产环境使用 PostgreSQL 时，务必通过环境变量传递密码：

```bash
export AROUTE_DATABASE_POSTGRES_PASSWORD="your-secure-password"
```

---

### `auth` — 认证

```yaml
auth:
  jwt_secret: "${AUTH_JWT_SECRET}"    # 生产环境必须通过环境变量覆盖
  jwt_algorithm: "HS256"
  access_token_ttl: "15m"
  refresh_token_ttl: "7d"
  rotate_refresh_tokens: true
  bcrypt_cost: 10
  rate_limit:
    max_attempts: 5
    window: "1m"
  admin:
    email: "admin@localhost"
    password: ""                        # 留空则首次运行时自动生成
```

| 键 | 类型 | 默认值 | 说明 |
|----|------|--------|------|
| `auth.jwt_secret` | string | — | JWT 签名密钥。**生产环境必须通过 `AROUTE_AUTH_JWT_SECRET` 环境变量设置**，`aroute init` 会自动生成 |
| `auth.jwt_algorithm` | string | `"HS256"` | JWT 签名算法，可选 `HS256`、`HS384`、`HS512`、`RS256`、`RS384`、`RS512` |
| `auth.jwt_private_key_path` | string | — | RS256 等非对称算法所需的私钥文件路径 |
| `auth.jwt_public_key_path` | string | — | RS256 等非对称算法所需的公钥文件路径 |
| `auth.access_token_ttl` | duration | `"15m"` | 访问令牌有效期 |
| `auth.refresh_token_ttl` | duration | `"7d"` | 刷新令牌有效期 |
| `auth.rotate_refresh_tokens` | bool | `true` | 每次刷新时轮换 refresh token |
| `auth.bcrypt_cost` | int | `10` | bcrypt 哈希轮数（10-14，值越高越安全但越慢） |
| `auth.rate_limit.max_attempts` | int | `5` | 登录失败限流：最大尝试次数 |
| `auth.rate_limit.window` | duration | `"1m"` | 登录失败限流：时间窗口 |
| `auth.admin.email` | string | `"admin@localhost"` | 初始管理员邮箱 |
| `auth.admin.password` | string | — | 初始管理员密码，留空则首次运行时自动生成 |

> **安全提示**：`jwt_secret` 和 `admin.password` 不应直接写在配置文件中。使用 `AROUTE_AUTH_JWT_SECRET` 和 `AROUTE_AUTH_ADMIN_PASSWORD` 环境变量覆盖。

---

### `media` — 媒体存储

```yaml
media:
  storage: "local"

  # 本地存储（storage 为 local 时生效）
  local:
    upload_dir: "data/uploads"

  # S3 兼容存储（storage 为 s3 时生效）
  # s3:
  #   endpoint: "s3.amazonaws.com"
  #   bucket: "aroute-media"
  #   region: "us-east-1"
  #   access_key: ""
  #   secret_key: ""
  #   use_ssl: true

  max_file_size: "50MB"
  allowed_types:
    - "image/*"
    - "application/pdf"
    - "video/*"
    - "audio/*"
```

| 键 | 类型 | 默认值 | 说明 |
|----|------|--------|------|
| `media.storage` | string | `"local"` | 存储驱动，可选 `local` 或 `s3` |
| `media.local.upload_dir` | string | `"data/uploads"` | 本地上传目录 |
| `media.s3.endpoint` | string | — | S3 端点地址 |
| `media.s3.bucket` | string | — | S3 桶名 |
| `media.s3.region` | string | — | S3 区域 |
| `media.s3.access_key` | string | — | S3 Access Key |
| `media.s3.secret_key` | string | — | S3 Secret Key |
| `media.s3.use_ssl` | bool | — | 是否使用 SSL 连接 S3 |
| `media.max_file_size` | string | `"50MB"` | 最大上传文件大小 |
| `media.allowed_types` | []string | `["image/*","application/pdf","video/*","audio/*"]` | 允许的 MIME 类型，支持通配符 |

---

### `search` — 搜索

```yaml
search:
  index_dir: "data/search"
```

| 键 | 类型 | 默认值 | 说明 |
|----|------|--------|------|
| `search.index_dir` | string | `"data/search"` | 搜索索引存储目录 |

---

### `cache` — 缓存

```yaml
cache:
  max_size: 256
  default_ttl: "5m"
```

| 键 | 类型 | 默认值 | 说明 |
|----|------|--------|------|
| `cache.max_size` | int | `256` | 最大缓存大小（MB） |
| `cache.default_ttl` | duration | `"5m"` | 缓存项默认过期时间 |

---

### `theme` — 主题

```yaml
theme:
  active: "default"
  dir: "themes"
```

| 键 | 类型 | 默认值 | 说明 |
|----|------|--------|------|
| `theme.active` | string | `"default"` | 当前激活的主题名称 |
| `theme.dir` | string | `"themes"` | 主题文件根目录 |

默认主题位于 `themes/default/`，使用 Go 模板引擎。主题目录结构：

```
themes/default/
  assets/       # 静态资源
  templates/    # 模板文件
  theme.yaml    # 主题元信息
```

---

### `log` — 日志

```yaml
log:
  level: "info"
  format: "json"
  output: "both"
  file:
    path: "data/logs"
    name: "aroute.log"
    max_size: 100
    max_age: 30
    max_backups: 10
    compress: true
```

| 键 | 类型 | 默认值 | 说明 |
|----|------|--------|------|
| `log.level` | string | `"info"` | 日志级别：`debug`、`info`、`warn`、`error` |
| `log.format` | string | `"json"` | 日志格式：`json` 或 `text` |
| `log.output` | string | `"both"` | 输出目标：`stdout`、`file`、`both` |
| `log.file.path` | string | `"data/logs"` | 日志文件目录（自动创建） |
| `log.file.name` | string | `"aroute.log"` | 日志文件名 |
| `log.file.max_size` | int | `100` | 单个日志文件最大大小（MB），超出后轮转 |
| `log.file.max_age` | int | `30` | 保留旧日志文件的最大天数，`0` 表示永久保留 |
| `log.file.max_backups` | int | `10` | 保留的旧日志文件最大数量，`0` 表示全部保留 |
| `log.file.compress` | bool | `true` | 是否压缩轮转后的旧日志文件 |

---

### `plugins` — 插件

```yaml
plugins:
  dir: "plugins"
```

| 键 | 类型 | 默认值 | 说明 |
|----|------|--------|------|
| `plugins.dir` | string | `"plugins"` | L2 原生插件目录 |

内置插件（L1）随二进制分发，无需额外安装。L2 插件放置在 `plugins.dir` 目录，L3 社区 WASM 插件放置在 `data/plugins/` 目录。

---

### `data_dir` — 数据目录

```yaml
data_dir: "data"
```

| 键 | 类型 | 默认值 | 说明 |
|----|------|--------|------|
| `data_dir` | string | `"data"` | 全局数据根目录，用于 bbolt 注册表、搜索索引、上传文件等 |

`data_dir` 下的目录结构：

```
data/
  aroute.db          # SQLite 数据库（使用 SQLite 时）
  registry.db        # bbolt 插件注册表
  uploads/           # 上传文件（本地存储时）
  search/            # 搜索索引
  logs/              # 日志文件
  plugins/           # L3 WASM 社区插件
  plugin_data/       # 插件私有数据
  migrations/        # 数据库迁移文件
```

---

## CLI 配置参数

以下是全局可用的 CLI 参数，优先级高于配置文件和环境变量：

| 参数 | 绑定配置键 | 说明 |
|------|-----------|------|
| `--config` | — | 指定配置文件路径 |
| `--data-dir` | `data_dir` | 数据目录 |
| `--plugin-dir` | `plugins.dir` | 插件目录 |
| `--log-level` | `log.level` | 日志级别 |
| `--log-format` | `log.format` | 日志格式 |

`serve` 子命令额外支持：

| 参数 | 绑定配置键 | 说明 |
|------|-----------|------|
| `-H, --host` | `server.host` | 监听地址 |
| `-p, --port` | `server.port` | 监听端口 |

---

## 环境变量参考

所有环境变量使用 `AROUTE_` 前缀，键名使用大写加下划线分隔。例如 `database.driver` 对应 `AROUTE_DATABASE_DRIVER`。

| 环境变量 | 对应配置键 | 说明 |
|----------|-----------|------|
| `AROUTE_SERVER_HOST` | `server.host` | 监听地址 |
| `AROUTE_SERVER_PORT` | `server.port` | 监听端口 |
| `AROUTE_DATABASE_DRIVER` | `database.driver` | 数据库驱动 |
| `AROUTE_DATABASE_SQLITE_PATH` | `database.sqlite.path` | SQLite 数据库路径 |
| `AROUTE_DATABASE_POSTGRES_HOST` | `database.postgres.host` | PostgreSQL 主机 |
| `AROUTE_DATABASE_POSTGRES_PORT` | `database.postgres.port` | PostgreSQL 端口 |
| `AROUTE_DATABASE_POSTGRES_USER` | `database.postgres.user` | PostgreSQL 用户名 |
| `AROUTE_DATABASE_POSTGRES_PASSWORD` | `database.postgres.password` | PostgreSQL 密码 |
| `AROUTE_DATABASE_POSTGRES_DBNAME` | `database.postgres.dbname` | PostgreSQL 数据库名 |
| `AROUTE_DATABASE_POSTGRES_SSLMODE` | `database.postgres.sslmode` | PostgreSQL SSL 模式 |
| `AROUTE_AUTH_JWT_SECRET` | `auth.jwt_secret` | JWT 签名密钥（**生产环境必须设置**） |
| `AROUTE_AUTH_JWT_ALGORITHM` | `auth.jwt_algorithm` | JWT 算法 |
| `AROUTE_AUTH_ACCESS_TOKEN_TTL` | `auth.access_token_ttl` | 访问令牌有效期 |
| `AROUTE_AUTH_REFRESH_TOKEN_TTL` | `auth.refresh_token_ttl` | 刷新令牌有效期 |
| `AROUTE_AUTH_ADMIN_PASSWORD` | `auth.admin.password` | 管理员密码 |
| `AROUTE_MEDIA_STORAGE` | `media.storage` | 媒体存储驱动 |
| `AROUTE_MEDIA_LOCAL_UPLOAD_DIR` | `media.local.upload_dir` | 本地上传目录 |
| `AROUTE_MEDIA_S3_ENDPOINT` | `media.s3.endpoint` | S3 端点 |
| `AROUTE_MEDIA_S3_BUCKET` | `media.s3.bucket` | S3 桶名 |
| `AROUTE_MEDIA_S3_REGION` | `media.s3.region` | S3 区域 |
| `AROUTE_MEDIA_S3_ACCESS_KEY` | `media.s3.access_key` | S3 Access Key |
| `AROUTE_MEDIA_S3_SECRET_KEY` | `media.s3.secret_key` | S3 Secret Key |
| `AROUTE_MEDIA_MAX_FILE_SIZE` | `media.max_file_size` | 最大文件大小 |
| `AROUTE_SEARCH_INDEX_DIR` | `search.index_dir` | 搜索索引目录 |
| `AROUTE_CACHE_MAX_SIZE` | `cache.max_size` | 最大缓存大小 |
| `AROUTE_CACHE_DEFAULT_TTL` | `cache.default_ttl` | 缓存默认 TTL |
| `AROUTE_THEME_ACTIVE` | `theme.active` | 当前主题 |
| `AROUTE_THEME_DIR` | `theme.dir` | 主题目录 |
| `AROUTE_LOG_LEVEL` | `log.level` | 日志级别 |
| `AROUTE_LOG_FORMAT` | `log.format` | 日志格式 |
| `AROUTE_LOG_OUTPUT` | `log.output` | 日志输出目标 |
| `AROUTE_PLUGINS_DIR` | `plugins.dir` | 插件目录 |
| `AROUTE_DATA_DIR` | `data_dir` | 数据目录 |
| `AROUTE_DEV_MODE` | — | 开发模式（设为 `true` 启用） |

---

## 配置命令

查看当前生效的完整配置（敏感值已脱敏）：

```bash
aroute config show
```

验证配置文件的语法和语义：

```bash
aroute config validate
```

`validate` 命令会检测未知配置键并提供拼写建议。
