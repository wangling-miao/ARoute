# ARoute CMS REST API 参考

## 概述

| 项目 | 说明 |
|------|------|
| Base URL | `/api/v1/` |
| 协议 | HTTPS（生产环境推荐） |
| 数据格式 | JSON（`Content-Type: application/json`） |
| 认证方式 | JWT Bearer Token / API Key |
| OpenAPI 文档 | `GET /api/v1/openapi.json` |
| API 文档 UI | `GET /api/docs`（Swagger UI 或 ReDoc） |

### 认证

除公开端点外，所有 API 请求需在 `Authorization` 头中携带 Bearer Token：

```
Authorization: Bearer <access_token>
```

或通过 `X-API-Key` 头使用 API Token：

```
X-API-Key <api_token>
```

### 速率限制

默认限制为每分钟 120 次请求（按用户 ID 或客户端 IP 计数）。超出限制返回 `429 Too Many Requests`。

响应头：

| 头 | 说明 |
|----|------|
| `X-RateLimit-Limit` | 窗口内允许的最大请求数 |
| `X-RateLimit-Remaining` | 窗口内剩余请求数 |
| `X-RateLimit-Reset` | 窗口重置时间（Unix 时间戳） |
| `Retry-After` | 被限制后建议等待秒数（仅 429 响应） |

### 公开端点

以下端点无需认证：

- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/users`（用户注册）

可通过配置 `api.public_read: true` 开启内容读取（GET）公开访问，或通过 `content_types.{slug}.auth_required: false` 按内容类型单独禁用认证。

---

## 认证 API

### POST /api/v1/auth/login

使用邮箱和密码登录，获取 Access Token 和 Refresh Token。

**请求体：**

```json
{
  "email": "admin@localhost",
  "password": "your_password"
}
```

**成功响应（200）：**

```json
{
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
  },
  "meta": {}
}
```

**错误响应：**

| 状态码 | 错误码 | 说明 |
|--------|--------|------|
| 400 | `BAD_REQUEST` | 无效的 JSON 请求体 |
| 401 | `UNAUTHORIZED` | 邮箱或密码错误 |
| 429 | `RATE_LIMITED` | 登录尝试过于频繁 |

---

### POST /api/v1/auth/refresh

使用 Refresh Token 刷新 Access Token。

**请求体：**

```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

**成功响应（200）：**

```json
{
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs..."
  },
  "meta": {}
}
```

**错误响应：**

| 状态码 | 错误码 | 说明 |
|--------|--------|------|
| 400 | `BAD_REQUEST` | 无效的 JSON 请求体 |
| 400 | `VALIDATION_ERROR` | refresh_token 字段为空 |
| 401 | `UNAUTHORIZED` | Refresh Token 无效或已过期 |

---

### GET /api/v1/auth/me

获取当前登录用户信息。需要认证。

**成功响应（200）：**

```json
{
  "data": {
    "id": "01HXYZ...",
    "email": "admin@localhost",
    "name": "Admin",
    "role": "admin",
    "status": "active",
    "created_at": "2025-01-01T00:00:00Z"
  },
  "meta": {}
}
```

---

## 用户 API

用户管理端点需要认证且具有相应权限（通常为 admin 角色）。

### GET /api/v1/users

获取用户列表，支持分页和筛选。

**查询参数：**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `page` | int | 1 | 页码（从 1 开始） |
| `per_page` | int | 20 | 每页数量（最大 100） |
| `search` | string | - | 按姓名或邮箱搜索 |
| `role` | string | - | 按角色筛选 |
| `status` | string | - | 按状态筛选（`active`、`inactive`、`suspended`） |
| `sort` | string | - | 排序字段 |
| `order` | string | - | 排序方向（`asc`、`desc`） |

**成功响应（200）：**

```json
{
  "data": [
    {
      "id": "01HXYZ...",
      "email": "user@example.com",
      "name": "User Name",
      "role": "editor",
      "status": "active",
      "created_at": "2025-01-01T00:00:00Z",
      "updated_at": "2025-01-01T00:00:00Z"
    }
  ],
  "meta": {
    "total": 50,
    "page": 1,
    "per_page": 20,
    "total_pages": 3
  }
}
```

---

### POST /api/v1/users

创建新用户。此端点为公开端点（可用于注册）。

**请求体：**

```json
{
  "email": "user@example.com",
  "name": "User Name",
  "password": "secure_password",
  "role": "editor"
}
```

**成功响应（201）：**

```json
{
  "data": {
    "id": "01HXYZ...",
    "email": "user@example.com",
    "name": "User Name",
    "role": "editor",
    "status": "active",
    "created_at": "2025-01-01T00:00:00Z"
  },
  "meta": {}
}
```

**错误响应：**

| 状态码 | 错误码 | 说明 |
|--------|--------|------|
| 400 | `BAD_REQUEST` | 无效的 JSON 请求体 |
| 400 | `VALIDATION_ERROR` | 字段验证失败 |
| 409 | `CONFLICT` | 邮箱已被使用 |

---

### GET /api/v1/users/:id

> 注意：单用户查询端点通过 `GET /api/v1/auth/me` 获取当前用户信息。如需按 ID 查询，请使用列表接口配合筛选。

---

### PUT /api/v1/users/{id}

更新用户信息。

**请求体：**

```json
{
  "name": "Updated Name",
  "role": "admin",
  "status": "active"
}
```

**成功响应（200）：**

```json
{
  "data": {
    "id": "01HXYZ...",
    "email": "user@example.com",
    "name": "Updated Name",
    "role": "admin",
    "status": "active",
    "created_at": "2025-01-01T00:00:00Z",
    "updated_at": "2025-06-01T12:00:00Z"
  },
  "meta": {}
}
```

---

### DELETE /api/v1/users/{id}

删除用户。成功时返回空响应。

**成功响应：** `204 No Content`

---

## 内容 API

内容 API 根据 Content Type 自动生成 CRUD 端点。路径格式为 `/api/v1/content/{content_type}`，其中 `{content_type}` 为内容类型的 slug 标识。

内置内容类型包括 `page`、`post`、`category`、`tag`，也可使用自定义 Content Type 的 slug。

### GET /api/v1/content/{content_type}

获取指定类型的内容列表，支持过滤、排序、分页和稀疏字段选择。

**路径参数：**

| 参数 | 说明 |
|------|------|
| `content_type` | 内容类型 slug（如 `post`、`page`） |

**查询参数：**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `page` | int | 1 | 页码（必须为正整数） |
| `per_page` | int | 20 | 每页数量（最大 100） |
| `sort` | string | `created_at` | 排序字段 |
| `order` | string | 根据字段类型自动决定 | 排序方向（`asc`、`desc`） |
| `fields` | string | - | 稀疏字段选择，逗号分隔（如 `id,title,slug`） |
| `expand` | string | - | 展开关联字段，逗号分隔 |
| `search` | string | - | 全文搜索关键词 |
| `{field}` | string | - | 精确过滤（如 `status=published`） |
| `{field}_contains` | string | - | 模糊匹配（如 `title_contains=Go`） |
| `{field}_gte` | string | - | 大于等于（如 `created_at_gte=2025-01-01`） |
| `{field}_lte` | string | - | 小于等于 |

排序方向默认规则：
- 文本类字段（text、markdown、email 等）默认升序（`asc`）
- 时间和版本字段（`created_at`、`updated_at`、`published_at`、`version`）默认降序（`desc`）

**成功响应（200）：**

```json
{
  "data": [
    {
      "id": "01HXYZ...",
      "content_type": "post",
      "title": "Hello World",
      "slug": "hello-world",
      "status": "published",
      "author_id": "01HABC...",
      "version": 1,
      "published_at": "2025-01-15T10:00:00Z",
      "created_at": "2025-01-15T10:00:00Z",
      "updated_at": "2025-01-15T10:00:00Z",
      "data": {
        "body": "This is my first post.",
        "excerpt": "Hello World"
      }
    }
  ],
  "meta": {
    "total_count": 42,
    "page": 1,
    "per_page": 20,
    "total_pages": 3,
    "warnings": []
  }
}
```

`X-Total-Count` 响应头也会包含总记录数。

---

### POST /api/v1/content/{content_type}

创建新的内容条目。

**请求体：**

```json
{
  "title": "New Post",
  "slug": "new-post",
  "status": "draft",
  "data": {
    "body": "Post content here.",
    "excerpt": "A new post"
  }
}
```

**成功响应（201）：**

响应包含 `Location` 头指向新创建的资源（如 `/api/v1/content/post/01HXYZ...`）。

```json
{
  "data": {
    "id": "01HXYZ...",
    "content_type": "post",
    "title": "New Post",
    "slug": "new-post",
    "status": "draft",
    "author_id": "01HABC...",
    "version": 1,
    "created_at": "2025-06-01T12:00:00Z",
    "updated_at": "2025-06-01T12:00:00Z",
    "data": {
      "body": "Post content here.",
      "excerpt": "A new post"
    }
  },
  "meta": {}
}
```

**错误响应：**

| 状态码 | 错误码 | 说明 |
|--------|--------|------|
| 400 | `BAD_REQUEST` | content_type 为空或 JSON 无效 |
| 400 | `INVALID_JSON` | 请求体不是有效的 JSON |
| 404 | `NOT_FOUND` | 内容类型不存在 |
| 422 | `VALIDATION_ERROR` | 字段验证失败 |

---

### GET /api/v1/content/{content_type}/{id}

获取单个内容条目。

**路径参数：**

| 参数 | 说明 |
|------|------|
| `content_type` | 内容类型 slug |
| `id` | 内容条目 ID |

**查询参数：**

| 参数 | 类型 | 说明 |
|------|------|------|
| `expand` | string | 展开关联字段，逗号分隔 |

**成功响应（200）：**

```json
{
  "data": {
    "id": "01HXYZ...",
    "content_type": "post",
    "title": "Hello World",
    "slug": "hello-world",
    "status": "published",
    "data": {
      "body": "Content here."
    }
  },
  "meta": {}
}
```

---

### PUT /api/v1/content/{content_type}/{id}

更新内容条目。

**请求体：**

```json
{
  "title": "Updated Title",
  "status": "published",
  "data": {
    "body": "Updated content."
  }
}
```

**成功响应（200）：**

返回更新后的完整内容对象。

---

### DELETE /api/v1/content/{content_type}/{id}

删除内容条目（软删除）。成功时返回空响应。

**成功响应：** `204 No Content`

---

## Content Type 管理 API

管理内容类型定义。需要认证和相应权限。

### GET /api/v1/content-types

获取所有内容类型列表。

**成功响应（200）：**

```json
{
  "data": [
    {
      "name": "post",
      "slug": "post",
      "display_name": "Post",
      "description": "Blog posts",
      "fields": [
        {
          "name": "body",
          "type": "richtext",
          "required": true
        },
        {
          "name": "excerpt",
          "type": "text"
        }
      ]
    }
  ],
  "meta": {}
}
```

---

### POST /api/v1/content-types

创建新的内容类型。创建后系统会自动在数据库中创建对应的数据表。

**请求体：**

```json
{
  "name": "product",
  "slug": "product",
  "display_name": "Product",
  "description": "Product catalog items",
  "fields": [
    {
      "name": "price",
      "type": "number",
      "required": true,
      "validation": {
        "min": 0
      }
    },
    {
      "name": "sku",
      "type": "text",
      "required": true,
      "unique": true,
      "validation": {
        "pattern": "^[A-Z0-9-]+$"
      }
    },
    {
      "name": "description",
      "type": "richtext"
    },
    {
      "name": "image",
      "type": "media"
    },
    {
      "name": "category",
      "type": "relation"
    }
  ]
}
```

**支持的字段类型：**

| 类型 | 说明 |
|------|------|
| `text` | 短文本 |
| `number` | 数值 |
| `boolean` | 布尔值 |
| `date` | 日期（`YYYY-MM-DD`） |
| `datetime` | 日期时间（ISO 8601） |
| `relation` | 关联其他内容类型 |
| `media` | 媒体文件引用 |
| `json` | JSON 对象 |
| `markdown` | Markdown 文本 |
| `richtext` | 富文本 |
| `email` | 邮箱地址 |
| `url` | URL 地址 |
| `slug` | URL 友好标识 |
| `enum` | 枚举值 |
| `color` | 颜色值 |

**验证规则：**

| 规则 | 说明 |
|------|------|
| `required` | 必填字段 |
| `minLength` / `maxLength` | 文本长度限制 |
| `pattern` | 正则表达式匹配 |
| `unique` | 值唯一 |
| `min` / `max` | 数值范围限制 |

**成功响应（201）：**

响应包含 `Location` 头（如 `/api/v1/content-types/product`）。

---

### GET /api/v1/content-types/{name}

获取单个内容类型定义。

**路径参数：**

| 参数 | 说明 |
|------|------|
| `name` | 内容类型名称 |

---

### PUT /api/v1/content-types/{name}

更新内容类型定义。字段修改后数据库表结构会自动同步。

**路径参数：**

| 参数 | 说明 |
|------|------|
| `name` | 内容类型名称 |

**请求体：** 与创建相同，传入完整的更新后定义。

---

### DELETE /api/v1/content-types/{name}

删除内容类型及其数据表。成功时返回空响应。

**成功响应：** `204 No Content`

---

## 媒体 API

### POST /api/v1/media

上传文件。使用 `multipart/form-data` 格式。需要认证。

**请求格式：** `multipart/form-data`

| 字段 | 类型 | 说明 |
|------|------|------|
| `file` | file | 上传的文件（字段名必须为 `file`） |

**上传限制（默认，可通过配置修改）：**

- 最大文件大小：50MB
- 允许的类型：`image/*`、`application/pdf`、`video/*`、`audio/*`

**成功响应（201）：**

```json
{
  "data": {
    "id": "01HXYZ...",
    "name": "photo.jpg",
    "mime_type": "image/jpeg",
    "size": 102400,
    "url": "/data/uploads/2025/06/photo.jpg",
    "created_at": "2025-06-01T12:00:00Z"
  },
  "meta": {}
}
```

---

### GET /api/v1/media

获取媒体文件列表。

**查询参数：**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `page` | int | 1 | 页码 |
| `per_page` | int | 20 | 每页数量（最大 100） |
| `sort` | string | - | 排序字段 |
| `order` | string | `desc` | 排序方向 |
| `search` | string | - | 搜索文件名 |

**成功响应（200）：**

包含分页元数据的媒体文件列表，每个文件包含 `url` 字段。

---

### DELETE /api/v1/media/{id}

删除媒体文件。成功时返回空响应。

**成功响应：** `204 No Content`

---

## 角色 API

### GET /api/v1/roles

获取所有角色及其权限列表。

**成功响应（200）：**

```json
{
  "data": [
    {
      "id": "01HXYZ...",
      "name": "admin",
      "display_name": "Administrator",
      "description": "Full system access",
      "permissions": [
        {
          "resource": "content",
          "actions": ["create", "read", "update", "delete"]
        },
        {
          "resource": "users",
          "actions": ["create", "read", "update", "delete"]
        }
      ],
      "created_at": "2025-01-01T00:00:00Z",
      "updated_at": "2025-01-01T00:00:00Z"
    }
  ],
  "meta": {}
}
```

---

### PUT /api/v1/roles/{id}

更新角色权限。

**请求体：**

```json
{
  "description": "Content editor role",
  "permissions": [
    {
      "resource": "content",
      "actions": ["create", "read", "update"]
    }
  ]
}
```

---

## API Token 管理

### GET /api/v1/api-tokens

获取当前用户的 API Token 列表。需要认证。

**成功响应（200）：**

```json
{
  "data": [
    {
      "id": "01HXYZ...",
      "name": "CI/CD Token",
      "created_at": "2025-01-01T00:00:00Z",
      "expires_at": "2026-01-01T00:00:00Z"
    }
  ],
  "meta": {}
}
```

---

### POST /api/v1/api-tokens

创建新的 API Token。需要认证。

**请求体：**

```json
{
  "name": "CI/CD Token",
  "expires_at": "2026-01-01T00:00:00Z"
}
```

**成功响应（201）：**

```json
{
  "data": {
    "id": "01HXYZ...",
    "name": "CI/CD Token",
    "token": "aroute_eyJhbGciOi...",
    "created_at": "2025-06-01T12:00:00Z",
    "expires_at": "2026-01-01T00:00:00Z"
  },
  "meta": {}
}
```

> Token 值仅在创建时返回一次，请妥善保存。

---

### DELETE /api/v1/api-tokens/{id}

撤销 API Token。成功时返回空响应。

**成功响应：** `204 No Content`

---

## 插件管理 API

### GET /api/v1/plugins

获取已安装插件列表。

**成功响应（200）：**

```json
{
  "data": [
    {
      "name": "http",
      "version": "1.0.0",
      "enabled": true,
      "description": "HTTP server plugin",
      "author": "ARoute"
    }
  ],
  "meta": {}
}
```

---

### POST /api/v1/plugins/{name}/enable

启用指定插件。

**路径参数：**

| 参数 | 说明 |
|------|------|
| `name` | 插件名称 |

**成功响应（200）：**

```json
{
  "data": {
    "message": "plugin enabled",
    "name": "search",
    "status": "enabled"
  },
  "meta": {}
}
```

---

### POST /api/v1/plugins/{name}/disable

禁用指定插件。

**路径参数：**

| 参数 | 说明 |
|------|------|
| `name` | 插件名称 |

**成功响应（200）：**

```json
{
  "data": {
    "message": "plugin disabled",
    "name": "search",
    "status": "disabled"
  },
  "meta": {}
}
```

---

## 系统管理 API

### GET /api/v1/dashboard/stats

获取仪表盘统计数据。需要认证。

**成功响应（200）：**

```json
{
  "data": {
    "content_counts": {
      "post": 42,
      "page": 7,
      "category": 5
    },
    "recent_activity": [],
    "system_status": {
      "database": "healthy",
      "plugin_count": 12,
      "cache_hit_ratio": 0.85
    }
  },
  "meta": {}
}
```

---

### GET /api/v1/settings

获取系统设置。需要认证。

**成功响应（200）：**

```json
{
  "data": {
    "site_name": "My Site",
    "site_url": "https://example.com",
    "language": "zh-CN",
    "timezone": "Asia/Shanghai",
    "smtp_host": "",
    "smtp_port": 0,
    "smtp_username": "",
    "sender_email": ""
  },
  "meta": {}
}
```

---

### PUT /api/v1/settings

更新系统设置。需要认证。

**请求体：**

```json
{
  "site_name": "Updated Site Name",
  "site_url": "https://new.example.com"
}
```

---

### GET /healthz

健康检查端点（不属于 `/api/v1/` 前缀，无需认证）。

**成功响应（200）：**

```json
{
  "status": "healthy",
  "timestamp": "2025-06-01T12:00:00Z",
  "version": "1.0.0"
}
```

---

## 搜索

全文搜索通过内容 API 的 `search` 查询参数实现：

```
GET /api/v1/content/post?search=Go语言
```

搜索插件（基于 Bleve + gse 中文分词）会自动对内容变更进行索引。

---

## 通用响应格式

### 成功响应

所有成功响应使用统一的信封格式：

```json
{
  "data": { ... },
  "meta": { ... }
}
```

- `data`：响应数据主体（单个对象或数组）
- `meta`：元数据（分页信息或其他上下文）

### 列表分页元数据

```json
{
  "meta": {
    "total_count": 42,
    "page": 1,
    "per_page": 20,
    "total_pages": 3,
    "warnings": ["unknown filter field: foo"]
  }
}
```

| 字段 | 说明 |
|------|------|
| `total_count` | 符合条件的总记录数 |
| `page` | 当前页码 |
| `per_page` | 每页记录数 |
| `total_pages` | 总页数 |
| `warnings` | 查询警告信息（如未知字段会被忽略而非报错） |

### 错误响应

所有错误响应使用 `errors` 数组信封格式：

```json
{
  "errors": [
    {
      "code": "VALIDATION_ERROR",
      "message": "title is required",
      "details": {
        "field": "title"
      }
    }
  ]
}
```

| 字段 | 说明 |
|------|------|
| `code` | 大写下划线格式的错误码 |
| `message` | 人类可读的错误描述 |
| `details` | 错误详情（如字段名、约束信息等） |

---

## 错误码参考

### HTTP 状态码

| 状态码 | 错误码 | 说明 |
|--------|--------|------|
| 400 | `BAD_REQUEST` | 请求格式错误或参数无效 |
| 400 | `INVALID_JSON` | 请求体不是有效的 JSON |
| 400 | `VALIDATION_ERROR` | 请求数据验证失败 |
| 401 | `UNAUTHORIZED` | 未提供认证凭据或凭据无效/过期 |
| 403 | `FORBIDDEN` | 权限不足，拒绝访问 |
| 404 | `NOT_FOUND` | 请求的资源不存在 |
| 406 | `NOT_ACCEPTABLE` | Accept 头不支持（仅支持 `application/json`） |
| 409 | `CONFLICT` | 资源冲突（如邮箱已被注册） |
| 415 | `UNSUPPORTED_MEDIA_TYPE` | Content-Type 必须为 `application/json` |
| 422 | `VALIDATION_ERROR` | 字段级验证错误（含 `details.field`） |
| 429 | `RATE_LIMITED` | 请求频率超过限制 |
| 500 | `INTERNAL_ERROR` | 服务器内部错误 |

### 常见错误场景

**认证失败：**
- 缺少 `Authorization` 头 → `401 UNAUTHORIZED: missing authorization header`
- Token 格式错误 → `401 UNAUTHORIZED: invalid authorization header format`
- Token 过期 → `401 UNAUTHORIZED: invalid or expired token`

**验证失败：**
- 多个字段验证失败时，`errors` 数组包含多条错误记录，每条带有 `details.field` 标注对应字段。
