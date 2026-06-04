# API Governance Patterns for go-zero

## Overview

This guide covers enterprise API governance patterns:
- API versioning strategies
- Structured error code taxonomy
- OpenAPI / Swagger documentation generation
- Cursor-based pagination for large datasets

---

## 1. API Versioning

### Strategy Comparison

| Strategy | Pros | Cons | When to Use |
|----------|------|------|-------------|
| URL Path (`/api/v1/`) | Simple, visible, cacheable | URL pollution | Default choice for REST APIs |
| Header (`Accept: application/vnd.api.v2+json`) | Clean URLs | Hidden, harder to test | Internal APIs with sophisticated clients |
| Query Param (`?version=2`) | Easy to test | Not RESTful | Legacy systems, gradual migration |

**Recommendation**: Use URL path versioning for go-zero services.

### URL Path Versioning in .api File

```api
syntax = "v1"

// ========== V1 Routes (stable) ==========
@server (
    group:  v1/user
    prefix: /api/v1
    jwt:    Auth
)
service gateway {
    @handler GetUserV1
    get /users/:id (GetUserV1Request) returns (GetUserV1Response)

    @handler ListUsersV1
    get /users (ListUsersV1Request) returns (ListUsersV1Response)
}

// ========== V2 Routes (new features) ==========
@server (
    group:  v2/user
    prefix: /api/v2
    jwt:    Auth
)
service gateway {
    @handler GetUserV2
    get /users/:id (GetUserV2Request) returns (GetUserV2Response)

    @handler ListUsersV2
    get /users (ListUsersV2Request) returns (ListUsersV2Response)
}
```

### Version Negotiation Middleware

```go
// internal/middleware/api_version_middleware.go
package middleware

import (
    "context"
    "net/http"
    "strings"
)

type APIVersionMiddleware struct{}

func NewAPIVersionMiddleware() *APIVersionMiddleware {
    return &APIVersionMiddleware{}
}

func (m *APIVersionMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Extract version from URL path
        version := "v1" // default
        parts := strings.Split(r.URL.Path, "/")
        for i, p := range parts {
            if p == "api" && i+1 < len(parts) {
                version = parts[i+1]
                break
            }
        }

        // Add version to context for logic layer access
        ctx := context.WithValue(r.Context(), "apiVersion", version)

        // Set deprecation headers for old versions
        if version == "v1" {
            w.Header().Set("Deprecation", "true")
            w.Header().Set("Sunset", "2026-12-31")
            w.Header().Set("Link", `</api/v2>; rel="successor-version"`)
        }

        next.ServeHTTP(w, r.WithContext(ctx))
    }
}
```

### Version Lifecycle

```
v1 (stable)  → v2 (current)  → v3 (beta)
   │                │
   │ Deprecation    │ Active
   │ headers sent   │
   │                │
   └── Sunset date ─┘ (12 months notice minimum)
```

**Rules:**
- Support at least 2 versions concurrently (current + previous)
- Announce deprecation with `Deprecation` and `Sunset` headers
- Give 12+ months notice before sunsetting
- Never remove a version without a migration guide

---

## 2. Error Code Taxonomy

### Error Code Structure

```
Domain (1-digit) + Category (2-digit) + Specific (2-digit)
```

| Domain | Range | Description |
|--------|-------|-------------|
| Auth | 10000-10999 | Authentication & authorization |
| User | 11000-11999 | User management |
| Content | 12000-12999 | Posts, comments, tags |
| Payment | 13000-13999 | Payments, orders |
| System | 90000-90999 | Infrastructure, rate limiting |

### Error Code Definition

```go
// pkg/errorx/codes.go
package errorx

// Auth errors (10000-10999)
var (
    ErrInvalidToken       = NewCodeError(10001, "invalid or expired token")
    ErrTokenExpired       = NewCodeError(10002, "token has expired")
    ErrInsufficientPerms  = NewCodeError(10003, "insufficient permissions")
    ErrAccountLocked      = NewCodeError(10004, "account is locked")
    ErrInvalidCredentials = NewCodeError(10005, "invalid username or password")
)

// User errors (11000-11999)
var (
    ErrUserNotFound       = NewCodeError(11001, "user not found")
    ErrDuplicateEmail     = NewCodeError(11002, "email already registered")
    ErrInvalidEmail       = NewCodeError(11003, "invalid email format")
    ErrPasswordTooWeak    = NewCodeError(11004, "password does not meet requirements")
)

// Content errors (12000-12999)
var (
    ErrPostNotFound       = NewCodeError(12001, "post not found")
    ErrCommentNotFound    = NewCodeError(12002, "comment not found")
    ErrPostForbidden      = NewCodeError(12003, "no permission to modify this post")
)

// System errors (90000-90999)
var (
    ErrRateLimited        = NewCodeError(90001, "too many requests, please retry later")
    ErrServiceUnavailable = NewCodeError(90002, "service temporarily unavailable")
    ErrDatabaseError      = NewCodeError(90003, "internal database error")
)
```

### CodeError Implementation

```go
// pkg/errorx/code_error.go
package errorx

import "net/http"

type CodeError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

func NewCodeError(code int, msg string) *CodeError {
    return &CodeError{Code: code, Message: msg}
}

func (e *CodeError) Error() string {
    return e.Message
}

// HTTPStatus maps error codes to HTTP status codes
func (e *CodeError) HTTPStatus() int {
    switch {
    case e.Code >= 10001 && e.Code <= 10002:
        return http.StatusUnauthorized
    case e.Code == 10003:
        return http.StatusForbidden
    case e.Code >= 10004 && e.Code <= 10005:
        return http.StatusUnauthorized
    case e.Code >= 11001 && e.Code <= 11001:
        return http.StatusNotFound
    case e.Code >= 11002 && e.Code <= 11004:
        return http.StatusBadRequest
    case e.Code >= 12001 && e.Code <= 12002:
        return http.StatusNotFound
    case e.Code == 12003:
        return http.StatusForbidden
    case e.Code == 90001:
        return http.StatusTooManyRequests
    case e.Code == 90002:
        return http.StatusServiceUnavailable
    default:
        return http.StatusInternalServerError
    }
}
```

### Global Error Handler

```go
// main.go — register before server.Start()
httpx.SetErrorHandlerCtx(func(ctx context.Context, err error) (int, any) {
    var codeErr *errorx.CodeError
    if errors.As(err, &codeErr) {
        return codeErr.HTTPStatus(), map[string]any{
            "code":    codeErr.Code,
            "message": codeErr.Message,
        }
    }

    // Unknown errors — hide details in production
    logx.WithContext(ctx).Errorf("unexpected error: %v", err)
    return http.StatusInternalServerError, map[string]any{
        "code":    90000,
        "message": "internal server error",
    }
})
```

### Standardized Response Envelope

```go
// pkg/response/response.go
package response

import (
    "net/http"
    "github.com/zeromicro/go-zero/rest/httpx"
)

type Body struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Data    any    `json:"data,omitempty"`
}

func Success(ctx context.Context, w http.ResponseWriter, data any) {
    httpx.OkJsonCtx(ctx, w, Body{
        Code:    0,
        Message: "ok",
        Data:    data,
    })
}

func Error(ctx context.Context, w http.ResponseWriter, err error) {
    var codeErr *errorx.CodeError
    if errors.As(err, &codeErr) {
        httpx.WriteJsonCtx(ctx, w, codeErr.HTTPStatus(), Body{
            Code:    codeErr.Code,
            Message: codeErr.Message,
        })
        return
    }

    logx.WithContext(ctx).Errorf("unexpected error: %v", err)
    httpx.WriteJsonCtx(ctx, w, http.StatusInternalServerError, Body{
        Code:    90000,
        Message: "internal server error",
    })
}
```

---

## 3. OpenAPI / Swagger Documentation

### goctl Plugin for Swagger

```bash
# Install swagger plugin
goctl api plugin -plugin goctl-swagger="swagger -filename docs/swagger.json" -api gateway.api -dir .
```

### Manual Swagger Generation

If the plugin is not available, generate OpenAPI spec from .api file:

```bash
# Install goctl-swagger
go install github.com/zeromicro/goctl-swagger@latest

# Generate swagger.json
goctl api plugin -plugin goctl-swagger="swagger -filename docs/swagger.json -host api.example.com -basepath /api/v1" \
    -api gateway.api -dir .
```

### Serving Swagger UI

```go
// internal/handler/swagger_handler.go
import "net/http"

func SwaggerHandler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, "docs/swagger.json")
    }
}

// Register in routes (non-JWT, development only)
// @server (
//     group: docs
//     prefix: /api
// )
// service gateway {
//     @handler SwaggerDoc
//     get /swagger.json
// }
```

### API Documentation Best Practices

- Generate swagger from `.api` file as single source of truth
- Include request/response examples in `.api` comments
- Version swagger files alongside API versions
- Serve Swagger UI only in development/staging, not production
- Use `@doc` annotations for description:

```api
@server (
    group: user
    prefix: /api/v1
    jwt: Auth
)
service gateway {
    @doc "Get user by ID. Returns 404 if user does not exist."
    @handler GetUser
    get /users/:id (GetUserRequest) returns (GetUserResponse)
}
```

---

## 4. Cursor-Based Pagination

### Why Cursor Pagination

| Feature | OFFSET/LIMIT | Cursor |
|---------|-------------|--------|
| Performance on large tables | Degrades (O(n)) | Constant (O(1)) |
| Consistency during inserts | Rows can shift/duplicate | Stable |
| Deep pagination | Very slow | Fast |
| Random page access | Yes | No |
| Best for | Admin panels, small datasets | Feeds, infinite scroll, APIs |

### Request/Response Types

```api
type (
    CursorPageRequest {
        Cursor   string `form:"cursor,optional"`     // Opaque cursor token
        PageSize int64  `form:"page_size,default=20"` // Items per page (max 100)
    }

    CursorPageResponse {
        Items      []PostItem `json:"items"`
        NextCursor string     `json:"next_cursor,omitempty"` // Empty = no more pages
        HasMore    bool       `json:"has_more"`
    }
)
```

### Cursor Encoding

```go
// pkg/cursor/cursor.go
package cursor

import (
    "encoding/base64"
    "fmt"
    "strconv"
    "strings"
)

// Encode creates an opaque cursor from (id, created_at)
func Encode(id int64, createdAt int64) string {
    raw := fmt.Sprintf("%d:%d", id, createdAt)
    return base64.URLEncoding.EncodeToString([]byte(raw))
}

// Decode extracts (id, created_at) from cursor
func Decode(token string) (id int64, createdAt int64, err error) {
    raw, err := base64.URLEncoding.DecodeString(token)
    if err != nil {
        return 0, 0, fmt.Errorf("invalid cursor: %w", err)
    }

    parts := strings.SplitN(string(raw), ":", 2)
    if len(parts) != 2 {
        return 0, 0, fmt.Errorf("malformed cursor")
    }

    id, err = strconv.ParseInt(parts[0], 10, 64)
    if err != nil {
        return 0, 0, fmt.Errorf("invalid cursor id: %w", err)
    }

    createdAt, err = strconv.ParseInt(parts[1], 10, 64)
    if err != nil {
        return 0, 0, fmt.Errorf("invalid cursor timestamp: %w", err)
    }

    return id, createdAt, nil
}
```

### Cursor Pagination Logic

```go
// internal/logic/posts/get_post_list_logic.go
func (l *GetPostListLogic) GetPostList(req *types.CursorPageRequest) (*types.CursorPageResponse, error) {
    // Validate page size
    pageSize := req.PageSize
    if pageSize <= 0 {
        pageSize = 20
    }
    if pageSize > 100 {
        pageSize = 100
    }

    var posts []*model.Post
    var err error

    if req.Cursor == "" {
        // First page — no cursor
        posts, err = l.svcCtx.PostModel.FindLatest(l.ctx, pageSize+1)
    } else {
        // Subsequent pages — decode cursor
        cursorId, cursorTime, decErr := cursor.Decode(req.Cursor)
        if decErr != nil {
            return nil, errorx.ErrInvalidCursor
        }
        posts, err = l.svcCtx.PostModel.FindAfterCursor(l.ctx, cursorId, cursorTime, pageSize+1)
    }
    if err != nil {
        return nil, err
    }

    // Check if there are more pages (we fetched pageSize+1)
    hasMore := int64(len(posts)) > pageSize
    if hasMore {
        posts = posts[:pageSize] // Trim extra item
    }

    // Build response
    items := make([]types.PostItem, len(posts))
    for i, p := range posts {
        items[i] = convertPost(p)
    }

    var nextCursor string
    if hasMore && len(posts) > 0 {
        last := posts[len(posts)-1]
        nextCursor = cursor.Encode(last.Id, last.CreatedAt.Unix())
    }

    return &types.CursorPageResponse{
        Items:      items,
        NextCursor: nextCursor,
        HasMore:    hasMore,
    }, nil
}
```

### SQL Query for Cursor Pagination

```go
// internal/model/post_model.go

// FindLatest returns the most recent posts (first page)
func (m *PostModel) FindLatest(ctx context.Context, limit int64) ([]*Post, error) {
    query := `SELECT id, title, content, user_id, created_at
              FROM posts
              WHERE status = 1
              ORDER BY created_at DESC, id DESC
              LIMIT ?`
    var posts []*Post
    err := m.conn.QueryRowsCtx(ctx, &posts, query, limit)
    return posts, err
}

// FindAfterCursor returns posts after the given cursor position
func (m *PostModel) FindAfterCursor(ctx context.Context, cursorId, cursorTime, limit int64) ([]*Post, error) {
    // Use composite condition for stable ordering
    query := `SELECT id, title, content, user_id, created_at
              FROM posts
              WHERE status = 1
                AND (created_at < FROM_UNIXTIME(?) OR (created_at = FROM_UNIXTIME(?) AND id < ?))
              ORDER BY created_at DESC, id DESC
              LIMIT ?`
    var posts []*Post
    err := m.conn.QueryRowsCtx(ctx, &posts, query, cursorTime, cursorTime, cursorId, limit)
    return posts, err
}
```

### ❌ When NOT to Use Cursor Pagination

- Admin panels that need "jump to page 50"
- Reports that need total count and page navigation
- Datasets under 10K rows where OFFSET is fast enough

For these cases, use OFFSET/LIMIT with total count:

```go
type OffsetPageRequest {
    Page     int64 `form:"page,default=1"`
    PageSize int64 `form:"page_size,default=20"`
}

type OffsetPageResponse {
    Total int64       `json:"total"`
    Items []PostItem  `json:"items"`
}
```

---

## 5. Request ID Tracing

### Request ID Middleware

Every request should carry a unique ID for tracing across services:

```go
// internal/middleware/request_id_middleware.go
package middleware

import (
    "context"
    "net/http"

    "github.com/zeromicro/go-zero/core/logx"
    "github.com/google/uuid"
)

type RequestIDMiddleware struct{}

func NewRequestIDMiddleware() *RequestIDMiddleware {
    return &RequestIDMiddleware{}
}

func (m *RequestIDMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Use existing request ID if present (from upstream service)
        requestId := r.Header.Get("X-Request-ID")
        if requestId == "" {
            requestId = uuid.New().String()
        }

        // Set in response header
        w.Header().Set("X-Request-ID", requestId)

        // Add to context for downstream propagation
        ctx := context.WithValue(r.Context(), "requestId", requestId)

        // Add to logger context
        ctx = logx.ContextWithFields(ctx, logx.Field("request_id", requestId))

        next.ServeHTTP(w, r.WithContext(ctx))
    }
}
```

---

## Governance Checklist

### ✅ DO:
- Version all APIs with URL path prefix (`/api/v1/`)
- Use structured error codes with domain separation
- Return consistent response envelope (`{code, message, data}`)
- Use cursor pagination for feeds and large datasets
- Generate API docs from `.api` files
- Include `X-Request-ID` in all responses
- Set deprecation headers on old API versions
- Validate page_size with upper bound (max 100)

### ❌ DON'T:
- Break backwards compatibility without version bump
- Return raw error messages to clients
- Use OFFSET pagination for infinite scroll or large tables
- Expose internal error details in production responses
- Hard-delete API versions without migration period

---

## Cross-References

- For REST handler patterns, see [REST API Patterns](./rest-api-patterns.md)
- For rate limiting on API endpoints, see [Resilience Patterns](./resilience-patterns.md)
- For idempotency keys on write endpoints, see [Concurrency Patterns](./concurrency-patterns.md#5-idempotency)
- For distributed tracing with request IDs, see [Observability Patterns](./observability-patterns.md)
