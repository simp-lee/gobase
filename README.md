# GoBase — 轻量级 Go Web 开发框架

GoBase 是一套**轻量、AI 编程友好**的 Go Web 开发框架 Starter Kit。采用精简的 2 层 Clean Architecture，按模块垂直切片组织代码，内置完整的 User CRUD 示例（含分页 / 过滤 / 排序），同时支持 REST API 和全栈 Web 两种开发模式。

## 核心特性

- **2 层 Clean Architecture** — Handler → Service → Repository，依赖单向流动，无循环引用
- **按模块垂直切片** — 每个业务模块（如 `user`）内含 handler / service / repository / DTO，职责自包含
- **REST API + 全栈 Web** — JSON API 与 Go Templates + htmx 页面并存，共享同一套 Service 层
- **内置分页 / 过滤 / 排序** — 泛型 `PageResult[T]`，GORM Scope 可复用，URL 参数即查询条件
- **统一错误处理** — 业务错误码 → HTTP 状态码自动映射，字段级验证错误明细
- **结构化日志** — `log/slog` + Context Handler，请求 ID 全链路自动关联
- **CSRF 保护** — HMAC-SHA256 签名 Token，页面路由自动校验，API 路由豁免
- **Toast 通知** — htmx `HX-Trigger` + Alpine.js，CRUD 操作即时反馈
- **优雅关停** — `signal.NotifyContext` 捕获信号，5 秒超时 shutdown，连接池安全释放
- **单二进制部署** — `embed.FS` 嵌入模板与静态资源，`go build` 即可分发
- **零 Node.js 依赖** — Tailwind CSS CDN 本地化 + htmx + Alpine.js，纯 Go 工具链

## 技术栈

| 层级 | 技术 | 说明 |
|------|------|------|
| HTTP 框架 | [Gin](https://github.com/gin-gonic/gin) | 高性能路由、中间件、参数绑定 |
| ORM | [GORM](https://gorm.io/) | 数据库操作，支持 SQLite / PostgreSQL |
| 配置管理 | [koanf](https://github.com/knadh/koanf) | YAML + 环境变量覆盖 |
| 日志 | [log/slog](https://pkg.go.dev/log/slog) | Go 标准库结构化日志 |
| 验证 | [validator/v10](https://github.com/go-playground/validator) | 结构体标签驱动的字段验证 |
| 前端交互 | [htmx](https://htmx.org/) | HTML 驱动的 AJAX，无需编写 JS |
| 前端状态 | [Alpine.js](https://alpinejs.dev/) | 轻量级响应式 UI（Toast 等） |
| 样式 | [Tailwind CSS](https://tailwindcss.com/) | 实用优先的 CSS 框架 |

## 快速开始

### 前置要求

- Go 1.23+
- Make（Windows 推荐 MinGW Make 或 Git Bash）
- curl（用于下载前端资源）

### 安装与运行

```bash
# 1. 克隆项目
git clone https://github.com/simp-lee/gobase.git
cd gobase

# 2. 下载前端 vendor 资源（htmx、Alpine.js、Tailwind CSS）
make download-vendor

# 3. 启动开发服务器
make dev

# 4. 访问应用
# 浏览器打开 http://localhost:8080
# 健康检查  http://localhost:8080/health
```

### 其他常用命令

```bash
make build            # 构建二进制到 bin/server
make run              # 以默认配置运行
make test             # 运行全部测试
make lint             # 运行 golangci-lint
make clean            # 清理构建产物
```

## 目录结构

```
gobase/
├── cmd/
│   └── server/
│       └── main.go              # 程序入口：加载配置 → 构建 App → 启动服务
├── configs/
│   └── config.yaml              # 默认配置文件（YAML 格式）
├── data/                        # SQLite 数据库文件存放目录（.gitignore）
├── internal/
│   ├── app/
│   │   ├── app.go               # 应用核心：依赖组装、生命周期管理、优雅关停
│   │   ├── routes.go            # 路由注册：静态资源、健康检查、API、页面路由
│   │   └── template.go          # 模板渲染器：layout/partial 组合，debug 热加载
│   ├── config/
│   │   ├── config.go            # 配置结构体定义、YAML 加载、环境变量覆盖
│   │   ├── database.go          # 数据库初始化：驱动选择、连接池配置
│   │   └── logger.go            # slog 日志初始化：级别、格式（text/json）
│   ├── domain/
│   │   ├── model.go             # BaseModel（ID + CreatedAt + UpdatedAt）、PageRequest、PageResult[T]
│   │   ├── errors.go            # 业务错误码体系：AppError、错误判断辅助函数
│   │   └── user.go              # User 实体 + UserRepository / UserService 接口
│   ├── middleware/
│   │   ├── cors.go              # CORS 跨域中间件
│   │   ├── csrf.go              # CSRF 防护中间件（HMAC-SHA256）
│   │   ├── logger.go            # 请求日志中间件（slog）
│   │   ├── recovery.go          # Panic 恢复中间件（含 500 错误页）
│   │   └── requestid.go         # Request ID 注入中间件
│   ├── module/
│   │   └── user/                # ★ 示例模块 — 完整 CRUD
│   │       ├── dto.go           # 请求 DTO（CreateUserRequest / UpdateUserRequest）
│   │       ├── handler.go       # REST API Handler（/api/users）
│   │       ├── page_handler.go  # 页面 Handler（htmx 表单交互）
│   │       ├── repository.go    # GORM 数据访问实现
│   │       └── service.go       # 业务逻辑实现
│   └── pkg/
│       ├── ctxlog.go            # Context Handler：请求 ID 自动注入日志
│       ├── pagination.go        # 分页/排序/过滤 GORM Scope + PageResult 构造
│       ├── response.go          # 统一 JSON 响应封装（Success/Error/List/ValidationError）
│       └── tx.go                # 数据库事务辅助函数 WithTx
├── web/
│   ├── embed.go                 # go:embed 声明，嵌入模板和静态资源
│   ├── static/
│   │   ├── css/app.css          # 自定义样式
│   │   ├── js/app.js            # 全局 JS：Toast 管理、htmx 事件桥接
│   │   └── vendor/              # 第三方前端库（htmx、Alpine.js、Tailwind CSS）
│   └── templates/
│       ├── layouts/base.html    # 页面基础布局（head、nav、main、toast 容器、脚本）
│       ├── partials/            # 可复用模板片段（导航栏、分页、toast）
│       ├── errors/              # 错误页面（404、500）
│       ├── home.html            # 首页
│       └── user/                # User 模块页面（列表、表单）
├── go.mod
├── Makefile                     # 构建、运行、测试、前端资源下载
└── README.md
```

### 依赖方向

```
cmd/server → internal/app → internal/module/* → internal/domain
                           → internal/middleware
                           → internal/pkg
                           → internal/config
```

**规则**：所有依赖指向 `domain`（最内层），`domain` 不依赖任何其他 internal 包。

## 新增模块标准流程

以新增一个 `product` 模块为例：

### 1. 定义领域模型和接口（`internal/domain/product.go`）

```go
package domain

import "context"

type Product struct {
    BaseModel
    Name  string  `gorm:"size:200;not null" json:"name"`
    Price float64 `gorm:"not null" json:"price"`
}

type ProductRepository interface {
    Create(ctx context.Context, product *Product) error
    GetByID(ctx context.Context, id uint) (*Product, error)
    List(ctx context.Context, req PageRequest) (*PageResult[Product], error)
    Update(ctx context.Context, product *Product) error
    Delete(ctx context.Context, id uint) error
}

type ProductService interface {
    CreateProduct(ctx context.Context, name string, price float64) (*Product, error)
    GetProduct(ctx context.Context, id uint) (*Product, error)
    ListProducts(ctx context.Context, req PageRequest) (*PageResult[Product], error)
    UpdateProduct(ctx context.Context, id uint, name string, price float64) (*Product, error)
    DeleteProduct(ctx context.Context, id uint) error
}
```

### 2. 创建模块目录（`internal/module/product/`）

```
internal/module/product/
├── dto.go            # 请求 DTO
├── handler.go        # REST API Handler
├── page_handler.go   # 页面 Handler（如需要 Web 页面）
├── repository.go     # GORM Repository 实现
└── service.go        # Service 实现
```

### 3. 定义 DTO（`internal/module/product/dto.go`）

```go
package product

type CreateProductRequest struct {
    Name  string  `json:"name" form:"name" binding:"required,min=2,max=200"`
    Price float64 `json:"price" form:"price" binding:"required,gt=0"`
}

type UpdateProductRequest struct {
    Name  string  `json:"name" form:"name" binding:"required,min=2,max=200"`
    Price float64 `json:"price" form:"price" binding:"required,gt=0"`
}
```

### 4. 实现 Repository → Service → Handler

参照 `internal/module/user/` 中的代码结构。Repository 中使用 `pkg.Paginate`、`pkg.Sort`、`pkg.Filter` 实现分页查询。

### 5. 注册到路由（`internal/app/routes.go`）

在 `RouteDeps` 中添加新的 Handler，然后在 `RegisterRoutes` 中注册 API 和页面路由。

### 6. 注册 AutoMigrate（`internal/app/app.go`）

在 `New()` 函数的 AutoMigrate 调用中添加 `&domain.Product{}`。

## 配置说明

### 配置文件（`configs/config.yaml`）

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "debug"                    # debug | release
  csrf_secret: "change-me-to-a-random-secret"
  cors:
    allow_origins: []               # release 默认拒绝跨域；显式配置后才放行

database:
  driver: "sqlite"                 # sqlite | postgres
  sqlite:
    path: "data/app.db"
  postgres:
    host: "localhost"
    port: 5432
    user: "postgres"
    password: ""
    dbname: "gobase"
    sslmode: "disable"
  pool:
    max_idle_conns: 10             # 最大空闲连接数（默认 10）
    max_open_conns: 100            # 最大打开连接数（默认 100）
    conn_max_lifetime: "1h"        # 连接最大存活时间（time.Duration 格式）

log:
  level: "debug"                   # debug | info | warn | error
  format: "text"                   # text | json
```

### CORS 配置建议（开发 / 生产）

开发环境（便于前后端分离联调）：

```yaml
server:
  mode: "debug"
  cors:
    allow_origins:
      - "*"
```

生产环境（最小权限，按域名白名单）：

```yaml
server:
  mode: "release"
  cors:
    allow_origins:
      - "https://admin.example.com"
      - "https://app.example.com"
```

说明：当 `mode=release` 且 `allow_origins` 未配置时，应用默认拒绝跨域请求。

### 环境变量覆盖

使用 `APP__` 前缀 + **双下划线 `__`** 作为层级分隔符来覆盖 YAML 配置。单下划线保持为键名的一部分。

| 环境变量 | 覆盖配置项 |
|----------|-----------|
| `APP__SERVER__PORT=9090` | `server.port` |
| `APP__SERVER__MODE=release` | `server.mode` |
| `APP__DATABASE__DRIVER=postgres` | `database.driver` |
| `APP__DATABASE__POOL__MAX_IDLE_CONNS=20` | `database.pool.max_idle_conns` |
| `APP__DATABASE__POOL__MAX_OPEN_CONNS=200` | `database.pool.max_open_conns` |
| `APP__LOG__LEVEL=info` | `log.level` |
| `APP__LOG__FORMAT=json` | `log.format` |

示例：

```bash
APP__SERVER__PORT=9090 APP__LOG__LEVEL=info go run ./cmd/server -config configs/config.yaml
```

### 连接池配置说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `max_idle_conns` | 空闲连接池中保持的最大连接数，减少频繁建立连接的开销 | 10 |
| `max_open_conns` | 数据库最大打开连接数，防止连接数失控 | 100 |
| `conn_max_lifetime` | 单个连接的最大存活时间，超时后自动关闭并重建 | 1h |

> **提示**：SQLite 为嵌入式数据库，连接池参数对其影响较小；切换到 PostgreSQL 时应根据服务器资源合理调整。

## 分页 / 过滤 / 排序 API

### 请求参数

| 参数 | 说明 | 示例 |
|------|------|------|
| `page` | 页码（从 1 开始，默认 1） | `page=2` |
| `page_size` | 每页数量（默认 20，最大 100） | `page_size=10` |
| `sort` | 排序字段和方向（`字段:asc` 或 `字段:desc`） | `sort=name:asc` |
| `字段名` | 精确匹配过滤 | `email=test@example.com` |
| `字段名__like` | 模糊匹配过滤（LIKE %value%） | `name__like=张` |

### 请求示例

```
GET /api/users?page=1&page_size=10&sort=name:asc&name__like=张
```

### 响应格式

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "items": [
      {
        "id": 1,
        "name": "张三",
        "email": "zhangsan@example.com",
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z"
      }
    ],
    "total": 50,
    "page": 1,
    "page_size": 10,
    "total_pages": 5
  }
}
```

### 在 Repository 中使用

```go
func (r *productRepository) List(ctx context.Context, req domain.PageRequest) (*domain.PageResult[Product], error) {
    var total int64
    base := r.db.WithContext(ctx).Model(&domain.Product{}).
        Scopes(pkg.Filter(req, []string{"name", "category"}))

    if err := base.Count(&total).Error; err != nil {
        return nil, err
    }

    var products []domain.Product
    if err := base.Scopes(
        pkg.Paginate(req),
        pkg.Sort(req, []string{"id", "name", "price", "created_at"}),
    ).Find(&products).Error; err != nil {
        return nil, err
    }

    return pkg.NewPageResult(products, total, req), nil
}
```

**安全机制**：排序和过滤字段必须在 `allowed` 白名单中声明，未列入白名单的字段会被静默忽略，防止 SQL 注入。

## 统一 API 响应格式

### 成功响应

```json
{
  "code": 200,
  "message": "success",
  "data": { ... }
}
```

### 错误响应

```json
{
  "code": 404,
  "message": "not found",
  "data": null
}
```

### 验证错误响应

```json
{
  "code": 400,
  "message": "validation error",
  "errors": {
    "name": "This field is required",
    "email": "Must be a valid email address"
  }
}
```

### Handler 中使用

```go
func (h *Handler) Create(c *gin.Context) {
    var req CreateRequest
    if !pkg.BindAndValidate(c, &req) {
        return  // 验证失败时自动返回 400 + 字段级错误
    }

    result, err := h.svc.DoSomething(c.Request.Context(), req.Name)
    if err != nil {
        pkg.Error(c, err)  // 自动映射业务错误码到 HTTP 状态码
        return
    }

    pkg.Success(c, result)
}
```

## CSRF 保护

### 机制说明

- **Token 格式**：`hex(nonce) + "." + base64url(HMAC-SHA256(nonce, secret))`
- **存储方式**：Cookie（`_csrf_token`，`HttpOnly=false`，`SameSite=Strict`）
- **作用范围**：仅页面路由组（`/users`、`/users/new` 等），`/api/*` 路由不启用 CSRF

### 页面路由（GET）

GET 请求时中间件自动生成 Token 并设置 Cookie，同时将 Token 存入 `gin.Context`，模板中通过 `.CSRFToken` 获取。

### 表单提交（POST/PUT/DELETE）

**方式一：隐藏表单字段**

```html
<form method="POST" action="/users">
    <input type="hidden" name="_csrf_token" value="{{ .CSRFToken }}">
    <!-- 其他字段 -->
</form>
```

**方式二：htmx 请求头（推荐）**

htmx 通过 `hx-headers` 携带 CSRF Token：

```html
<form hx-post="/users"
      hx-headers='{"X-CSRF-Token": "{{ .CSRFToken }}"}'>
    <!-- 表单字段 -->
</form>
```

### API 路由

`/api/*` 路由组**不注册** CSRF 中间件，因此 API 客户端无需处理 CSRF Token。API 认证应使用其他机制（如 Bearer Token）。

## Toast 通知

### 工作原理

1. 服务端在 htmx 响应中设置 `HX-Trigger` 头部，携带 `showToast` 事件和消息内容
2. 客户端 `app.js` 监听 `htmx:afterRequest` 事件，解析 `HX-Trigger` 头部
3. 触发 Alpine.js `show-toast` 自定义事件，toast 组件自动渲染
4. Toast 3 秒后自动消失

### 服务端用法（Go Handler）

```go
// setShowToastHeader 设置 HX-Trigger 响应头
func setShowToastHeader(c *gin.Context, message, toastType string) {
    trigger, _ := json.Marshal(map[string]any{
        "showToast": map[string]string{
            "message": message,
            "type":    toastType,  // "success" | "error" | "info"
        },
    })
    c.Header("HX-Trigger", string(trigger))
}

// 使用示例
setShowToastHeader(c, "用户创建成功", "success")
c.Header("HX-Redirect", "/users")
c.Status(http.StatusOK)
```

### 实际响应头

```
HX-Trigger: {"showToast":{"message":"用户创建成功","type":"success"}}
HX-Redirect: /users
```

### Toast 类型

| 类型 | 颜色 | 用途 |
|------|------|------|
| `success` | 绿色 | 创建、更新、删除成功 |
| `error` | 红色 | 操作失败 |
| `info` | 蓝色 | 一般信息提示 |

## 框架约定

### 命名约定

| 对象 | 约定 | 示例 |
|------|------|------|
| 包名 | 小写单词 | `user`, `config`, `middleware` |
| 文件名 | 小写 + 下划线 | `page_handler.go`, `config_test.go` |
| 结构体 | PascalCase | `UserHandler`, `PageRequest` |
| 接口 | PascalCase + 动词/名词 | `UserRepository`, `UserService` |
| 函数/方法 | PascalCase（导出）/ camelCase（内部） | `NewUserHandler()`, `parseID()` |
| DTO 结构体 | `XxxRequest` / `XxxResponse` | `CreateUserRequest` |
| 测试文件 | `xxx_test.go`，与源文件同目录 | `handler_test.go` |

### 文件组织约定

| 约定 | 说明 |
|------|------|
| DTO 放在 Handler 层 | `dto.go` 位于 `internal/module/xxx/`，而非 `domain/` |
| 接口定义在 domain | `UserRepository` / `UserService` 接口定义在 `internal/domain/user.go` |
| 不使用 `gorm.Model` | 自定义 `BaseModel`（仅 `ID` + `CreatedAt` + `UpdatedAt`），避免隐式软删除字段 |
| `PageRequest` / `PageResult[T]` 在 domain | 分页结构体属于领域概念，定义在 `internal/domain/model.go` |
| 实现放在 module 内部 | Repository / Service 的具体实现放在各自模块目录中，对外通过接口解耦 |
| 中间件独立为 package | `internal/middleware/` 中每个中间件一个文件，职责单一 |
| 一个错误一个判断函数 | `IsNotFound(err)` / `IsAlreadyExists(err)` 等，不直接比较 error 指针 |

### 依赖方向规则

- `domain` 是最内层，**不依赖**任何其他 `internal/` 包
- `module/xxx` 依赖 `domain` 和 `pkg`，**不依赖**其他模块
- `app` 是组装层，负责依赖注入，可以 import 所有 `internal/` 包
- `pkg` 提供跨模块工具函数，依赖 `domain`（如 `PageRequest`），不依赖 `module`

### 数据库约定

- Debug 模式下自动执行 `AutoMigrate`，Release 模式需手动管理 schema 迁移
- Repository 方法必须接收 `context.Context` 作为第一个参数
- 数据库错误通过 `mapError()` 统一映射为 `domain.AppError`
- 事务操作使用 `pkg.WithTx(db, func(tx *gorm.DB) error { ... })` 辅助函数

## AI 编程使用指南

GoBase 的设计目标之一是**对 AI 编程工具友好**。以下指南帮助你在使用 Copilot、Cursor、Claude 等 AI 工具时获得最佳效果。

### 给 AI 的上下文提示

向 AI 描述新功能时，提供以下关键上下文：

```
本项目是一个 Go Web 框架，使用：
- Gin 作为 HTTP 框架
- GORM 作为 ORM（不使用 gorm.Model，使用自定义 BaseModel）
- 2 层架构：Handler → Service → Repository
- 接口定义在 internal/domain/，实现在 internal/module/xxx/
- DTO 定义在模块的 dto.go 中，不放在 domain 层
- 分页使用 domain.PageRequest + domain.PageResult[T]
- 统一响应使用 pkg.Success() / pkg.Error() / pkg.List()
- 请参考 internal/module/user/ 作为示例模块
```

### 让 AI 生成新模块

推荐的 prompt 模式：

```
请参考 internal/module/user/ 的代码结构，为 product 模块生成以下文件：
1. internal/domain/product.go — 模型 + Repository/Service 接口
2. internal/module/product/dto.go — CreateProductRequest / UpdateProductRequest
3. internal/module/product/repository.go — GORM 实现，支持分页/过滤/排序
4. internal/module/product/service.go — 业务逻辑
5. internal/module/product/handler.go — REST API Handler

Product 包含字段：Name (string), Price (float64), Category (string)
排序允许：id, name, price, created_at
过滤允许：name, category
```

### 关键设计决策（AI 须知）

| 决策 | 原因 |
|------|------|
| 不使用 `gorm.Model` | `gorm.Model` 内含 `DeletedAt`（软删除），AI 容易混淆。`BaseModel` 仅含 `ID` + `CreatedAt` + `UpdatedAt`，所有字段显式声明 |
| DTO 在 Handler 层 | 请求/响应结构体是 API 边界概念，不属于领域模型。DTO 变化不应影响 domain 层 |
| 接口在 domain 层 | 依赖反转：Service 依赖 Repository 接口而非实现，便于测试和替换 |
| `PageRequest` / `PageResult[T]` 在 domain 层 | 分页是领域概念（"获取第 N 页数据"），非基础设施细节 |
| 环境变量用 `__` 分隔 | 单下划线保留给配置键名（如 `max_idle_conns`），双下划线 `__` 表示层级 |
| CSRF 仅在页面路由 | API 由外部客户端调用，使用 Token 认证；页面表单才需要 CSRF |

### 常见 AI 生成错误及纠正

| 错误 | 纠正 |
|------|------|
| 使用 `gorm.Model` 作为嵌入基类 | 使用 `domain.BaseModel`（无 `DeletedAt`） |
| 在 `domain/` 中定义 DTO | DTO 放在 `internal/module/xxx/dto.go` |
| 在 Handler 中直接操作数据库 | 通过 Service 接口调用，Handler 只处理 HTTP 协议 |
| `errors.Is(err, domain.ErrNotFound)` | 使用 `domain.IsNotFound(err)`（基于错误码判断） |
| 忘记在 Repository 方法中传递 `ctx` | 所有 DB 操作须 `db.WithContext(ctx)` |
| 分页不使用白名单 | Sort / Filter 必须传入 allowed 字段列表 |

## 许可证

MIT
