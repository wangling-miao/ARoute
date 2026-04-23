# 主题开发指南

本文档介绍如何为 ARoute CMS 开发自定义主题，涵盖三种渲染引擎（Go Template、Lua、React SSR）的使用方法。

## 主题系统概述

ARoute CMS 主题是一个包含模板文件、静态资源和配置清单的目录。主题通过 ThemeService 管理生命周期，支持热切换（无需重启服务）。

### 主题包结构

```
my-theme/
  theme.yaml           # 主题清单（必需）
  templates/            # 模板文件
    layouts/            # 布局模板
      base.html
    partials/           # 局部模板
      head.html
      header.html
      footer.html
    index.html          # 首页模板
    post.html           # 文章详情模板
    posts.html          # 文章列表模板
    page.html           # 独立页面模板
    single.html         # 通用内容详情模板
    archive.html        # 归档页模板
    404.html            # 404 错误页模板
  assets/               # 静态资源
    css/
      style.css
    js/
      main.js
    images/
```

### theme.yaml 清单格式

```yaml
name: "My Theme"              # 主题名称
version: "1.0.0"              # 语义化版本号
author: "your-name"           # 作者
description: "A custom theme" # 主题描述
engine: "gotemplate"          # 渲染引擎：gotemplate / lua / react-ssr
aroute_version: ">=1.0.0"    # 兼容的 ARoute 版本

settings:                     # 主题设置（可在后台修改）
  primary_color: "#4f46e5"
  accent_color: "#06b6d4"
  show_sidebar: true
  posts_per_page: 10
  show_author: true
  show_date: true
  show_reading_time: true
```

### 主题安装与激活

通过 ThemeService API 管理主题：

```go
// 获取 ThemeService
theme, err := sdk.GetTheme(ctx.Services())

// 安装主题（从目录或压缩包）
err = theme.InstallTheme(ctx, "/path/to/my-theme")

// 列出可用主题
names, err := theme.ListThemes(ctx)

// 激活主题（热切换，无需重启）
err = theme.SetActiveTheme(ctx, "my-theme")

// 获取当前激活主题
active, err := theme.GetActiveTheme(ctx)
```

ThemeService 接口：

| 方法 | 说明 |
|------|------|
| `Render(ctx, templateName, data)` | 渲染模板，返回 HTML |
| `GetActiveTheme(ctx)` | 获取当前激活主题名称 |
| `SetActiveTheme(ctx, name)` | 切换主题（热切换） |
| `ListThemes(ctx)` | 列出所有可用主题 |
| `InstallTheme(ctx, sourcePath)` | 从路径安装主题 |

## Go Template 引擎

Go Template 是 ARoute 的默认主题引擎，使用 Go 标准库 `html/template`。

### 模板继承与布局

ARoute 使用 `define`/`template` 实现模板组合。布局模板定义公共结构，内容模板引用局部模板：

**布局模板** `templates/layouts/base.html`：

```html
{{define "layouts/base.html"}}
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{if .Title}}{{.Title}} — {{end}}{{.Site.Title}}</title>
  <link rel="stylesheet" href="{{asset "css/style.css"}}">
</head>
<body>
  {{template "partials/header.html" .}}

  <main class="main-content">
    {{block "content" .}}{{end}}
  </main>

  {{template "partials/footer.html" .}}
  <script src="{{asset "js/main.js"}}"></script>
</body>
</html>
{{end}}
```

**局部模板** `templates/partials/header.html`：

```html
{{define "partials/header.html"}}
<header class="site-header">
  <div class="container">
    <a href="/" class="logo">{{.Site.Title}}</a>
    {{if .Site.Tagline}}
    <span class="tagline">{{.Site.Tagline}}</span>
    {{end}}
    <nav>
      <a href="/">Home</a>
      <a href="/posts">Posts</a>
      <a href="/archive">Archive</a>
      <a href="/about">About</a>
    </nav>
  </div>
</header>
{{end}}
```

**局部模板** `templates/partials/footer.html`：

```html
{{define "partials/footer.html"}}
<footer class="site-footer">
  <div class="container">
    <p>&copy; {{year}} {{.Site.Title}}. Powered by ARoute.</p>
  </div>
</footer>
{{end}}
```

**局部模板** `templates/partials/head.html`：

```html
{{define "partials/head.html"}}
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="description" content="{{if .Description}}{{.Description}}{{else}}{{.Site.Title}}{{end}}">
<title>{{if .Title}}{{.Title}} — {{end}}{{.Site.Title}}</title>
<link rel="stylesheet" href="{{asset "css/style.css"}}">
{{end}}
```

### 可用模板函数

| 函数 | 签名 | 说明 |
|------|------|------|
| `asset` | `asset(path) string` | 返回带哈希的静态资源路径 |
| `url` | `url(path) string` | 返回站点 URL |
| `slugify` | `slugify(s) string` | 将字符串转为 URL 安全格式 |
| `truncate` | `truncate(s, n) string` | 截断字符串到 n 个字符 |
| `formatDate` | `formatDate(date, layout) string` | 格式化日期（Go 布局格式） |
| `safeHTML` | `safeHTML(s) template.HTML` | 标记 HTML 为安全，不转义 |
| `default` | `default(def, val) interface{}` | 如果 val 为空则返回 def |
| `lower` | `lower(s) string` | 转小写 |
| `upper` | `upper(s) string` | 转大写 |
| `title` | `title(s) string` | 标题大小写 |
| `trim` | `trim(s) string` | 去除首尾空白 |
| `replace` | `replace(old, new, s) string` | 字符串替换 |
| `substring` | `substring(s, start, end) string` | 截取子串 |
| `year` | `year` | 当前年份（四位数） |
| `now` | `now` | 当前时间 |
| `json` | `json(v) string` | 序列化为 JSON |
| `isset` | `isset(map, key) bool` | 检查 map 中是否存在 key |

### 模板变量

模板渲染时可用的数据变量：

```go
// 站点信息
.Site.Title        // 站点标题
.Site.Tagline      // 站点副标题
.Site.URL          // 站点 URL

// 当前页面
.Title             // 页面标题
.Description       // 页面描述
.Body              // 页面正文 HTML

// 首页特有
.Posts             // 文章列表
.Hero              // Hero 区域配置
.Hero.Title        // Hero 标题
.Hero.Subtitle     // Hero 副标题
.Pagination        // 分页信息
.Pagination.CurrentPage
.Pagination.TotalPages
.Pagination.HasPrev
.Pagination.HasNext
.Pagination.PrevPage
.Pagination.NextPage

// 文章详情特有
.Post              // 当前文章
.Post.Title
.Post.Slug
.Post.Date
.Post.Author
.Post.Body
.Post.Tags         // 标签列表
.Post.Excerpt      // 摘要

// 通用内容详情（single）
.Date              // 内容日期
.Author            // 内容作者
```

### 创建完整的 Go 模板主题

以下是基于默认主题的结构说明。

**首页** `templates/index.html`：

```html
<!DOCTYPE html>
<html lang="en">
<head>
  {{template "partials/head.html" .}}
</head>
<body>
  {{template "partials/header.html" .}}

  <main class="main-content">
  {{if .Hero}}
  <section class="hero">
    <div class="container">
      <h1>{{default "Welcome to Our Blog" .Hero.Title}}</h1>
      <p>{{default "Thoughts, stories and ideas worth sharing" .Hero.Subtitle}}</p>
      <a href="/posts">Browse Posts</a>
    </div>
  </section>
  {{end}}

  <section class="section">
    <div class="container">
      {{if .Posts}}
      <div class="post-grid">
        {{range $i, $post := .Posts}}
        <article class="post-card">
          {{if $post.Date}}<time datetime="{{$post.Date}}">{{formatDate $post.Date "Jan 02, 2006"}}</time>{{end}}
          <h2><a href="/posts/{{$post.Slug}}">{{$post.Title}}</a></h2>
          {{if $post.Excerpt}}<p>{{truncate $post.Excerpt 140}}</p>{{end}}
        </article>
        {{end}}
      </div>

      {{if .Pagination}}
      <nav class="pagination">
        {{if .Pagination.HasPrev}}
        <a href="?page={{.Pagination.PrevPage}}">Newer</a>
        {{end}}
        <span>Page {{.Pagination.CurrentPage}} of {{.Pagination.TotalPages}}</span>
        {{if .Pagination.HasNext}}
        <a href="?page={{.Pagination.NextPage}}">Older</a>
        {{end}}
      </nav>
      {{end}}

      {{else}}
      <div class="empty-state">
        <h3>No posts yet</h3>
        <p>Check back soon for new content.</p>
      </div>
      {{end}}
    </div>
  </section>
  </main>

  {{template "partials/footer.html" .}}
  <script src="{{asset "js/main.js"}}"></script>
</body>
</html>
```

**文章详情** `templates/post.html`：

```html
<!DOCTYPE html>
<html lang="en">
<head>
  {{template "partials/head.html" .}}
</head>
<body>
  {{template "partials/header.html" .}}

  <main class="main-content">
  <article class="single-post">
    <div class="container container--narrow">
      <header class="post-header">
        <div class="post-header__meta">
          {{if .Post.Date}}<time datetime="{{.Post.Date}}">{{formatDate .Post.Date "Jan 02, 2006"}}</time>{{end}}
          {{if .Post.Author}}<span class="post-header__author">{{.Post.Author}}</span>{{end}}
        </div>
        <h1 class="post-header__title">{{.Post.Title}}</h1>
        {{if .Post.Tags}}
        <div class="post-header__tags">
          {{range .Post.Tags}}
          <a href="/tags/{{slugify .}}" class="tag">{{.}}</a>
          {{end}}
        </div>
        {{end}}
      </header>

      <div class="post-body prose">
        {{safeHTML .Post.Body}}
      </div>

      <footer class="post-footer">
        {{if .Post.Tags}}
        <div>
          <span>Tagged:</span>
          {{range .Post.Tags}}
          <a href="/tags/{{slugify .}}" class="tag">{{.}}</a>
          {{end}}
        </div>
        {{end}}
        <nav>
          <a href="/posts">Back to all posts</a>
        </nav>
      </footer>
    </div>
  </article>
  </main>

  {{template "partials/footer.html" .}}
  <script src="{{asset "js/main.js"}}"></script>
</body>
</html>
```

**404 错误页** `templates/404.html`：

```html
<!DOCTYPE html>
<html lang="en">
<head>
  {{template "partials/head.html" .}}
</head>
<body>
  {{template "partials/header.html" .}}

  <main class="main-content">
  <section class="error-page">
    <div class="container">
      <span class="error-code">404</span>
      <h1>Page not found</h1>
      <p>The page you're looking for doesn't exist or has been moved.</p>
      <div>
        <a href="/">Go Home</a>
        <a href="/posts">Browse Posts</a>
      </div>
    </div>
  </section>
  </main>

  {{template "partials/footer.html" .}}
  <script src="{{asset "js/main.js"}}"></script>
</body>
</html>
```

**文章列表** `templates/posts.html`：

```html
<!DOCTYPE html>
<html lang="en">
<head>
  {{template "partials/head.html" .}}
</head>
<body>
  {{template "partials/header.html" .}}

  <main class="main-content">
  <section class="section">
    <div class="container">
      <h1>All Posts</h1>

      {{if .Posts}}
      <div class="post-list">
        {{range .Posts}}
        <article class="post-item">
          {{if .Date}}<time datetime="{{.Date}}">{{formatDate .Date "Jan 02, 2006"}}</time>{{end}}
          {{if .Author}}<span>{{.Author}}</span>{{end}}
          <h2><a href="/posts/{{.Slug}}">{{.Title}}</a></h2>
          {{if .Excerpt}}<p>{{truncate .Excerpt 200}}</p>{{end}}
          {{if .Tags}}
          <div>
            {{range .Tags}}
            <a href="/tags/{{slugify .}}" class="tag">{{.}}</a>
            {{end}}
          </div>
          {{end}}
        </article>
        {{end}}
      </div>

      {{if .Pagination}}
      <nav class="pagination">
        {{if .Pagination.HasPrev}}
        <a href="?page={{.Pagination.PrevPage}}">Newer</a>
        {{end}}
        <span>Page {{.Pagination.CurrentPage}} of {{.Pagination.TotalPages}}</span>
        {{if .Pagination.HasNext}}
        <a href="?page={{.Pagination.NextPage}}">Older</a>
        {{end}}
      </nav>
      {{end}}

      {{else}}
      <div class="empty-state">
        <h3>No posts found</h3>
        <p>There are no posts to display right now.</p>
      </div>
      {{end}}
    </div>
  </section>
  </main>

  {{template "partials/footer.html" .}}
  <script src="{{asset "js/main.js"}}"></script>
</body>
</html>
```

### 静态资源管理

使用 `asset` 函数引用静态资源，函数会自动处理缓存哈希：

```html
<link rel="stylesheet" href="{{asset "css/style.css"}}">
<script src="{{asset "js/main.js"}}"></script>
<img src="{{asset "images/logo.png"}}" alt="Logo">
```

静态资源文件放在 `assets/` 目录下：

```
assets/
  css/
    style.css
  js/
    main.js
  images/
    logo.png
    favicon.ico
```

## Lua 引擎

Lua 引擎使用 gopher-lua 集成，为需要动态逻辑的主题提供脚本能力。

### 配置

在 `theme.yaml` 中设置 `engine: lua`：

```yaml
name: "My Lua Theme"
version: "1.0.0"
engine: "lua"
```

### Lua API 参考

Lua 主题中可用的全局函数：

```lua
-- 获取站点信息
site = cms.site.get()          -- {title="", tagline="", url=""}

-- 获取内容列表
posts = cms.content.list("post", {
    page = 1,
    per_page = 10,
    sort = "created_at",
    order = "desc"
})

-- 获取单个内容
post = cms.content.get_by_id("post-id")

-- 获取主题设置
color = cms.theme.setting("primary_color")

-- 渲染局部模板
html = cms.template.render("partials/header.html", {title = "Hello"})

-- URL 辅助
url = cms.url("slugify", "Hello World")  -- "hello-world"
url = cms.url("asset", "css/style.css")  -- "/assets/themes/my-theme/css/style.css"
```

### LState 池化模型

Lua 引擎使用 LState 池管理 Lua 虚拟机实例，避免每个请求创建新实例的开销：

- 预创建固定数量的 LState 实例
- 请求到来时从池中借用，完成后归还
- 每个主题维护独立的 Lua 状态，避免互相干扰

### 创建 Lua 主题

目录结构：

```
my-lua-theme/
  theme.yaml
  templates/
    index.lua         -- 首页模板
    post.lua          -- 文章详情模板
    posts.lua         -- 文章列表模板
    partials/
      header.lua
      footer.lua
  assets/
    css/style.css
```

`templates/index.lua` 示例：

```lua
-- 获取站点信息
local site = cms.site.get()

-- 获取最新文章
local posts = cms.content.list("post", {
    page = 1,
    per_page = 10,
    sort = "created_at",
    order = "desc"
})

-- 获取主题设置
local primary_color = cms.theme.setting("primary_color")

-- 渲染头部
local header = cms.template.render("partials/header.html", {
    site = site,
    title = site.title
})

-- 渲染底部
local footer = cms.template.render("partials/footer.html", {
    site = site
})

-- 拼接输出
local html = header
html = html .. '<main class="main-content"><div class="container">'

if #posts > 0 then
    for i, post in ipairs(posts) do
        html = html .. '<article>'
        html = html .. '<h2><a href="/posts/' .. post.slug .. '">' .. post.title .. '</a></h2>'
        html = html .. '</article>'
    end
else
    html = html .. '<p>No posts yet.</p>'
end

html = html .. '</div></main>'
html = html .. footer

return html
```

## React SSR 引擎

React SSR 引擎使用 fastschema/qjs (QuickJS) 在服务端渲染 React 组件。

### 配置

在 `theme.yaml` 中设置 `engine: react-ssr`：

```yaml
name: "My React Theme"
version: "1.0.0"
engine: "react-ssr"
```

### 组件结构

目录结构：

```
my-react-theme/
  theme.yaml
  templates/
    App.jsx            -- 根组件
    pages/
      Index.jsx        -- 首页
      Post.jsx         -- 文章详情
      Posts.jsx        -- 文章列表
    components/
      Header.jsx
      Footer.jsx
      PostCard.jsx
  assets/
    css/style.css
    js/client.js       -- 客户端水合脚本
```

### SSR 上下文中的数据获取

在 SSR 环境中，数据通过渲染上下文注入，组件通过 props 接收：

```jsx
// templates/pages/Post.jsx
export default function Post({ post, site }) {
    return (
        <article className="single-post">
            <header>
                {post.date && <time dateTime={post.date}>{post.date}</time>}
                {post.author && <span className="author">{post.author}</span>}
                <h1>{post.title}</h1>
            </header>
            <div
                className="post-body"
                dangerouslySetInnerHTML={{ __html: post.body }}
            />
            <footer>
                {post.tags && post.tags.length > 0 && (
                    <div className="tags">
                        {post.tags.map((tag) => (
                            <a key={tag} href={`/tags/${tag}`} className="tag">
                                {tag}
                            </a>
                        ))}
                    </div>
                )}
                <nav>
                    <a href="/posts">Back to all posts</a>
                </nav>
            </footer>
        </article>
    );
}
```

### 创建 React SSR 主题

**根组件** `templates/App.jsx`：

```jsx
import Header from "./components/Header";
import Footer from "./components/Footer";

export default function App({ page, data, site }) {
    return (
        <html lang="en">
            <head>
                <meta charSet="UTF-8" />
                <meta
                    name="viewport"
                    content="width=device-width, initial-scale=1.0"
                />
                <title>
                    {data.title
                        ? `${data.title} — ${site.title}`
                        : site.title}
                </title>
                <link rel="stylesheet" href={`/assets/css/style.css`} />
            </head>
            <body>
                <Header site={site} />
                <main className="main-content">
                    {renderPage(page, data)}
                </main>
                <Footer site={site} />
                <script src={`/assets/js/client.js`} />
            </body>
        </html>
    );
}

function renderPage(page, data) {
    switch (page) {
        case "index":
            return <IndexPage data={data} />;
        case "post":
            return <PostPage post={data.post} />;
        case "posts":
            return <PostsPage posts={data.posts} pagination={data.pagination} />;
        case "404":
            return <NotFoundPage />;
        default:
            return <SinglePage data={data} />;
    }
}
```

**首页组件** `templates/pages/Index.jsx`：

```jsx
import PostCard from "../components/PostCard";

export default function IndexPage({ data }) {
    const { posts, hero, pagination } = data;

    return (
        <div>
            {hero && (
                <section className="hero">
                    <div className="container">
                        <h1>{hero.title || "Welcome to Our Blog"}</h1>
                        <p>
                            {hero.subtitle ||
                                "Thoughts, stories and ideas worth sharing"}
                        </p>
                        <a href="/posts">Browse Posts</a>
                    </div>
                </section>
            )}

            <section className="section">
                <div className="container">
                    {posts && posts.length > 0 ? (
                        <div className="post-grid">
                            {posts.map((post, i) => (
                                <PostCard key={post.id} post={post} featured={i === 0} />
                            ))}
                        </div>
                    ) : (
                        <div className="empty-state">
                            <h3>No posts yet</h3>
                            <p>Check back soon for new content.</p>
                        </div>
                    )}
                </div>
            </section>
        </div>
    );
}
```

**PostCard 组件** `templates/components/PostCard.jsx`：

```jsx
export default function PostCard({ post, featured }) {
    return (
        <article className={`post-card${featured ? " post-card--featured" : ""}`}>
            <div className="post-card__body">
                {post.date && (
                    <time dateTime={post.date}>{post.date}</time>
                )}
                <h2 className="post-card__title">
                    <a href={`/posts/${post.slug}`}>{post.title}</a>
                </h2>
                {post.excerpt && <p>{post.excerpt}</p>}
                <a href={`/posts/${post.slug}`}>Read more</a>
            </div>
        </article>
    );
}
```

## 主题设置

### settings schema

在 `theme.yaml` 中定义的 `settings` 字段允许用户在后台自定义主题外观：

```yaml
settings:
  # 颜色设置
  primary_color: "#4f46e5"       # 主色调
  accent_color: "#06b6d4"        # 强调色
  background_color: "#ffffff"     # 背景色

  # 布局设置
  show_sidebar: true              # 是否显示侧边栏
  posts_per_page: 10              # 每页文章数
  sidebar_position: "right"       # 侧边栏位置

  # 内容显示
  show_author: true               # 显示作者
  show_date: true                 # 显示日期
  show_reading_time: true         # 显示阅读时间

  # Hero 区域
  hero_enabled: true              # 启用 Hero 区域
  hero_title: "Welcome to Our Blog"
  hero_subtitle: "Thoughts, stories and ideas worth sharing"
```

设置值支持以下类型：字符串、数字、布尔值。

### 在模板中访问设置

**Go Template:**

```html
<style>
:root {
  --primary-color: {{.Settings.primary_color}};
  --accent-color: {{.Settings.accent_color}};
}
</style>
```

**Lua:**

```lua
local primary_color = cms.theme.setting("primary_color")
local show_sidebar = cms.theme.setting("show_sidebar")
```

**React SSR:**

```jsx
export default function App({ settings }) {
    return (
        <div style={{ "--primary-color": settings.primary_color }}>
            {/* ... */}
        </div>
    );
}
```

## 开发模式

### 热重载支持

ARoute 支持主题开发时的热重载：

1. 修改模板文件后保存
2. ThemeService 检测到文件变更
3. 自动重新加载模板（无需重启服务）
4. 刷新浏览器即可看到最新效果

热重载支持的文件类型：
- `templates/` 下的所有模板文件
- `theme.yaml` 配置文件（设置变更）
- `assets/` 下的 CSS 和 JS 文件

> 注意：`theme.yaml` 中 `engine` 字段的变更可能需要重启服务。

### 调试技巧

1. **查看可用变量**: 在模板中临时添加 `{{json .}}` 输出所有数据，确认字段名称

2. **Go Template 错误**: Go Template 错误会输出到日志，检查 ARoute 控制台输出

3. **模板查找顺序**: ThemeService 按以下顺序查找模板文件：
   - `templates/{name}.html`（Go Template）
   - `templates/{name}.lua`（Lua）
   - `templates/pages/{Name}.jsx`（React SSR）

4. **静态资源 404**: 确认资源文件在 `assets/` 目录下，且使用 `{{asset "..."}}` 函数引用

5. **使用默认主题作为参考**: 查看 `themes/default/` 目录了解完整的主题结构和模板写法
