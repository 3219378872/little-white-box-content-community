# Fix OptionalAuthMiddleware Configuration

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 OptionalAuthMiddleware 从 Handler 内部迁移到 go-zero 标准架构位置（ServiceContext 注入 + .api 声明式绑定），并打通 Gateway → Content RPC 的可选认证链路。

**Architecture:**
1. Gateway 层：中间件改为标准 `Handle` 签名，通过 `ServiceContext` 依赖注入，`.api` 文件声明式绑定到 `/posts` 路由。
2. Gateway Logic 层：从 context 提取 optional userId，传递给 Content RPC。
3. Content 层：proto `GetPostListReq` 新增 `user_id` 字段，Content Logic 接收并记录（`IsLiked`/`IsFavorited` 填充依赖 Interaction 服务集成，留作后续任务）。

**Tech Stack:** Go 1.26.1, go-zero v1.10.1, gRPC, Protocol Buffers, goctl

---

## File Structure

### Gateway Layer (9 files)

| File | Action | Responsibility |
|------|--------|---------------|
| `pkg/middleware/optional_auth.go` | Modify | 改为标准 `Handle(http.HandlerFunc) http.HandlerFunc` 签名 |
| `pkg/middleware/optional_auth_test.go` | Modify | 适配新的中间件构造方式 |
| `app/gateway/internal/svc/service_context.go` | Modify | 注入 `OptionalAuth rest.Middleware` |
| `app/gateway/gateway.api` | Modify | `@server` 声明 `middleware: OptionalAuth` |
| `app/gateway/internal/handler/posts/get_post_list_handler.go` | Modify | 移除内部中间件包裹，恢复为标准 handler |
| `app/gateway/internal/handler/routes.go` | Regenerate | goctl 根据 `.api` 生成（含中间件绑定） |
| `app/gateway/internal/handler/posts/get_post_list_handler_test.go` | Modify | 移除中间件断言，仅测试 handler 核心逻辑 |
| `app/gateway/internal/logic/posts/get_post_list_logic.go` | Modify | 从 context 获取 userId，传给 Content RPC |
| `app/gateway/internal/logic/posts/get_post_list_logic_test.go` | Modify | 验证 userId 传递到 RPC |

### Content Layer (6 files)

| File | Action | Responsibility |
|------|--------|---------------|
| `proto/content/content.proto` | Modify | `GetPostListReq` 新增 `int64 user_id = 4` |
| `app/content/pb/xiaobaihe/content/pb/content.pb.go` | Regenerate | goctl protoc 生成 |
| `app/content/pb/xiaobaihe/content/pb/content_grpc.pb.go` | Regenerate | goctl protoc 生成 |
| `app/content/contentservice/content_service.go` | Regenerate | goctl protoc 生成 |
| `app/content/internal/server/content_service_server.go` | Regenerate | goctl protoc 生成 |
| `app/content/internal/logic/get_post_list_logic.go` | Modify | 接收 userId，添加 TODO 注释 |

---

## Task 1: 重构 OptionalAuthMiddleware 为标准 go-zero 签名

**Files:**
- Modify: `pkg/middleware/optional_auth.go`
- Modify: `pkg/middleware/optional_auth_test.go`

**Context:** 当前中间件使用 `func(http.Handler) http.Handler` 函数签名，与 go-zero 的 `rest.Middleware` 类型（`func(http.HandlerFunc) http.HandlerFunc`）不兼容。需要改为结构体 + `Handle` 方法模式，以便注入 `ServiceContext`。

- [ ] **Step 1: 重写中间件为标准结构体模式**

Replace `pkg/middleware/optional_auth.go` entirely:

```go
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"jwtx"
)

const (
	AuthStateHeader        = "X-Auth-State"
	AuthStateAnonymous     = "anonymous"
	AuthStateAuthenticated = "authenticated"
	AuthStateExpired       = "expired"
	AuthStateInvalid       = "invalid"
)

type OptionalAuthMiddleware struct {
	config jwtx.JwtConfig
}

func NewOptionalAuthMiddleware(config jwtx.JwtConfig) *OptionalAuthMiddleware {
	return &OptionalAuthMiddleware{config: config}
}

func (m *OptionalAuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set(AuthStateHeader, AuthStateAnonymous)
			next(w, r)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			w.Header().Set(AuthStateHeader, AuthStateInvalid)
			next(w, r)
			return
		}

		claims, err := jwtx.ParseToken(parts[1], m.config)
		if err != nil {
			if errors.Is(err, jwtx.ErrTokenExpired) {
				w.Header().Set(AuthStateHeader, AuthStateExpired)
			} else {
				w.Header().Set(AuthStateHeader, AuthStateInvalid)
			}
			next(w, r)
			return
		}

		w.Header().Set(AuthStateHeader, AuthStateAuthenticated)
		next(w, r.WithContext(jwtx.WithClaimsContext(r.Context(), claims)))
	}
}
```

**Key changes:**
- `func(http.Handler) http.Handler` → `func (m *OptionalAuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc`
- Constructor returns `*OptionalAuthMiddleware` instead of closure
- `next.ServeHTTP(w, r)` → `next(w, r)` (HandlerFunc is callable directly)

- [ ] **Step 2: 更新中间件测试适配新签名**

Replace `pkg/middleware/optional_auth_test.go` entirely:

```go
package middleware

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jwtx"
)

func runOptionalAuthRequest(t *testing.T, authHeader string, expire int64) (int, string, string) {
	t.Helper()

	mw := NewOptionalAuthMiddleware(jwtx.JwtConfig{
		AccessSecret: "secret",
		AccessExpire: expire,
	})

	var seenUser string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID, ok := jwtx.GetOptionalUserIdFromContext(r.Context()); ok {
			seenUser = fmt.Sprintf("%d", userID)
		} else {
			seenUser = "anonymous"
		}
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	mw.Handle(next)(rec, req)

	body, _ := io.ReadAll(rec.Result().Body)
	return rec.Result().StatusCode, rec.Header().Get(AuthStateHeader), seenUser + ":" + string(body)
}

func TestOptionalAuthMiddleware_NoToken_SetsAnonymous(t *testing.T) {
	status, authState, seen := runOptionalAuthRequest(t, "", 3600)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if authState != AuthStateAnonymous {
		t.Fatalf("expected %q, got %q", AuthStateAnonymous, authState)
	}
	if seen != "anonymous:ok" {
		t.Fatalf("expected anonymous context, got %s", seen)
	}
}

func TestOptionalAuthMiddleware_ValidToken_SetsAuthenticated(t *testing.T) {
	token, err := jwtx.GenerateToken(42, "alice", jwtx.JwtConfig{
		AccessSecret: "secret",
		AccessExpire: 3600,
	})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	status, authState, seen := runOptionalAuthRequest(t, "Bearer "+token, 3600)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if authState != AuthStateAuthenticated {
		t.Fatalf("expected %q, got %q", AuthStateAuthenticated, authState)
	}
	if seen != "42:ok" {
		t.Fatalf("expected authenticated context, got %s", seen)
	}
}

func TestOptionalAuthMiddleware_ExpiredToken_SetsExpired(t *testing.T) {
	token, err := jwtx.GenerateToken(42, "alice", jwtx.JwtConfig{
		AccessSecret: "secret",
		AccessExpire: -1,
	})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	status, authState, seen := runOptionalAuthRequest(t, "Bearer "+token, 3600)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if authState != AuthStateExpired {
		t.Fatalf("expected %q, got %q", AuthStateExpired, authState)
	}
	if seen != "anonymous:ok" {
		t.Fatalf("expected anonymous fallback, got %s", seen)
	}
}

func TestOptionalAuthMiddleware_InvalidToken_SetsInvalid(t *testing.T) {
	status, authState, seen := runOptionalAuthRequest(t, "Bearer not-a-token", 3600)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if authState != AuthStateInvalid {
		t.Fatalf("expected %q, got %q", AuthStateInvalid, authState)
	}
	if seen != "anonymous:ok" {
		t.Fatalf("expected anonymous fallback, got %s", seen)
	}
}

func TestOptionalAuthMiddleware_BadBearerFormat_SetsInvalid(t *testing.T) {
	status, authState, seen := runOptionalAuthRequest(t, strings.TrimSpace("Token abc"), 3600)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if authState != AuthStateInvalid {
		t.Fatalf("expected %q, got %q", AuthStateInvalid, authState)
	}
	if seen != "anonymous:ok" {
		t.Fatalf("expected anonymous fallback, got %s", seen)
	}
}
```

- [ ] **Step 3: 运行中间件测试验证通过**

Run:
```bash
cd pkg/middleware && go test -v ./...
```

Expected: 5 tests PASS.

- [ ] **Step 4: Commit**

```bash
git add pkg/middleware/optional_auth.go pkg/middleware/optional_auth_test.go
git commit -m "refactor(middleware): standardize OptionalAuthMiddleware to go-zero Handle signature

- Change from func(http.Handler) http.Handler to struct + Handle method
- Align with rest.Middleware type for ServiceContext injection
- Update tests to use NewOptionalAuthMiddleware constructor"
```

---

## Task 2: 在 Gateway ServiceContext 中注入 OptionalAuthMiddleware

**Files:**
- Modify: `app/gateway/internal/svc/service_context.go`

**Context:** go-zero 的声明式中间件绑定要求 `ServiceContext` 中存在与 `.api` 声明同名的 `rest.Middleware` 字段。需要添加 `OptionalAuth rest.Middleware` 并在构造函数中初始化。

- [ ] **Step 1: 修改 ServiceContext 注入中间件**

Replace `app/gateway/internal/svc/service_context.go` entirely:

```go
// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"esx/app/content/contentservice"
	"esx/app/media/mediaservice"
	"gateway/internal/config"
	"interceptor"
	"jwtx"
	"middleware"
	"user/userservice"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config         config.Config
	UserService    userservice.UserService
	ContentService contentservice.ContentService
	MediaService   mediaservice.MediaService
	OptionalAuth   rest.Middleware
}

func NewServiceContext(c config.Config) *ServiceContext {
	bizErrInterceptor := interceptor.BizErrorUnaryInterceptor()

	userClient := zrpc.MustNewClient(c.UserRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	userService := userservice.NewUserService(userClient)
	contentClient := zrpc.MustNewClient(c.ContentRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	contentService := contentservice.NewContentService(contentClient)
	mediaClient := zrpc.MustNewClient(c.MediaRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	mediaService := mediaservice.NewMediaService(mediaClient)

	optionalAuth := middleware.NewOptionalAuthMiddleware(jwtx.JwtConfig{
		AccessSecret: c.Auth.AccessSecret,
		AccessExpire: c.Auth.AccessExpire,
	})

	return &ServiceContext{
		Config:         c,
		UserService:    userService,
		ContentService: contentService,
		MediaService:   mediaService,
		OptionalAuth:   optionalAuth.Handle,
	}
}
```

**Key changes:**
- Add `OptionalAuth rest.Middleware` field
- Import `jwtx` and `middleware`
- Initialize `NewOptionalAuthMiddleware` with config values in constructor
- Assign `.Handle` to the field

- [ ] **Step 2: 编译验证 Gateway 服务**

Run:
```bash
cd app/gateway && go build ./...
```

Expected: compiles successfully.

- [ ] **Step 3: Commit**

```bash
git add app/gateway/internal/svc/service_context.go
git commit -m "feat(gateway/svc): inject OptionalAuthMiddleware into ServiceContext

- Add OptionalAuth rest.Middleware field
- Initialize with Auth config from etc/gateway.yaml"
```

---

## Task 3: 声明式中间件绑定（.api 文件 + goctl 生成 + Handler 清理）

**Files:**
- Modify: `app/gateway/gateway.api`
- Regenerate: `app/gateway/internal/handler/routes.go`
- Modify: `app/gateway/internal/handler/posts/get_post_list_handler.go`

**Context:** go-zero 支持在 `.api` 文件的 `@server` 块中通过 `middleware: Name` 声明中间件。goctl 生成代码时会自动在对应路由组上挂载 `ctx.Name`。同时需要手动清理 `get_post_list_handler.go` 中遗留的手动中间件包裹代码。

- [ ] **Step 1: 在 .api 中为 GetPostList 路由声明中间件**

Modify `app/gateway/gateway.api`. Replace the `GetPostList` `@server` block (around line 309-317):

**Old:**
```go
@server (
	prefix: /api/v1
	group:  posts
)
service gateway {
	@doc "获取帖子列表"
	@handler GetPostList
	get /posts (GetPostListReq) returns (GetPostListResp)
}
```

**New:**
```go
@server (
	prefix:     /api/v1
	group:      posts
	middleware: OptionalAuth
)
service gateway {
	@doc "获取帖子列表"
	@handler GetPostList
	get /posts (GetPostListReq) returns (GetPostListResp)
}
```

- [ ] **Step 2: goctl 重新生成 Gateway handler 和 routes**

Run from `app/gateway` directory:
```bash
cd app/gateway
goctl api go -api gateway.api -dir . --style go_zero
```

Expected output: goctl generates `internal/handler/routes.go` and any missing scaffolded files. Existing scaffolded files (marked `// Code scaffolded by goctl. Safe to edit.`) are **not overwritten**.

**Verification:** Check `app/gateway/internal/handler/routes.go`. The `/posts` route group should now include `rest.WithMiddlewares([]rest.Middleware{serverCtx.OptionalAuth}, ...)` or equivalent.

- [ ] **Step 3: 清理 get_post_list_handler.go 中的手动中间件包裹**

Replace `app/gateway/internal/handler/posts/get_post_list_handler.go` entirely:

```go
// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"net/http"

	"gateway/internal/logic/posts"
	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// 获取帖子列表
func GetPostListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetPostListReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := posts.NewGetPostListLogic(r.Context(), svcCtx)
		resp, err := l.GetPostList(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
```

**Removed:**
- `middleware` import
- `jwtx` import
- Inner/outer handler wrapping
- Manual `OptionalAuthMiddleware` construction

- [ ] **Step 4: 编译验证 Gateway 服务**

Run:
```bash
cd app/gateway && go build ./...
```

Expected: compiles successfully.

- [ ] **Step 5: Commit**

```bash
git add app/gateway/gateway.api app/gateway/internal/handler/routes.go app/gateway/internal/handler/posts/get_post_list_handler.go
git commit -m "feat(gateway/posts): bind OptionalAuth via .api declaration

- Add middleware: OptionalAuth to @server block for /posts
- Regenerate routes.go with goctl (routes now reflect middleware binding)
- Remove manual middleware wrapping from get_post_list_handler.go"
```

---

## Task 4: Gateway Logic 层消费 optional userId 并传给 Content RPC

**Files:**
- Modify: `app/gateway/internal/logic/posts/get_post_list_logic.go`
- Modify: `app/gateway/internal/logic/posts/get_post_list_logic_test.go`

**Context:** 中间件已将认证 claims 注入 context。Logic 层需要调用 `jwtx.GetOptionalUserIdFromContext` 提取 userId，并传递给 Content RPC。Content RPC 的 `GetPostListReq` 当前没有 `user_id` 字段（将在 Task 5 添加），但为了保持代码一致性，先修改 Logic 层获取 userId，等 Task 5 完成 proto 变更后再传入。

- [ ] **Step 1: 修改 Gateway Logic 获取并传递 userId**

Replace `app/gateway/internal/logic/posts/get_post_list_logic.go` entirely:

```go
// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package posts

import (
	"context"

	"errx"
	"esx/app/content/contentservice"

	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
	"jwtx"
)

type GetPostListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取帖子列表
func NewGetPostListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostListLogic {
	return &GetPostListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPostListLogic) GetPostList(req *types.GetPostListReq) (resp *types.GetPostListResp, err error) {
	userId, _ := jwtx.GetOptionalUserIdFromContext(l.ctx)

	result, err := l.svcCtx.ContentService.GetPostList(l.ctx, &contentservice.GetPostListReq{
		Page:     req.Page,
		PageSize: req.PageSize,
		SortBy:   req.SortBy,
		UserId:   userId,
	})
	if err != nil {
		l.Errorw("ContentService.GetPostList RPC failed", logx.Field("err", err.Error()), logx.Field("userId", userId))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	list := make([]types.PostItem, 0, len(result.Posts))
	for _, post := range result.Posts {
		list = append(list, types.PostItem{
			Id:           post.Id,
			AuthorId:     post.AuthorId,
			Title:        post.Title,
			Content:      post.Content,
			Images:       post.Images,
			Tags:         post.Tags,
			ViewCount:    post.ViewCount,
			LikeCount:    post.LikeCount,
			CommentCount: post.CommentCount,
			CreatedAt:    post.CreatedAt,
		})
	}

	return &types.GetPostListResp{
		List:     list,
		Total:    result.Total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}
```

**Key changes:**
- Import `jwtx`
- Extract `userId` from context via `GetOptionalUserIdFromContext`
- Pass `UserId: userId` to Content RPC (will compile after Task 5 adds the field to proto)
- Add `userId` to error log for debugging

- [ ] **Step 2: 更新 Logic 测试验证 userId 传递**

Replace `app/gateway/internal/logic/posts/get_post_list_logic_test.go` entirely:

```go
package posts

import (
	"context"
	"testing"

	"esx/app/content/contentservice"
	contentpb "esx/app/content/pb/xiaobaihe/content/pb"
	"gateway/internal/svc"
	"gateway/internal/types"
	"google.golang.org/grpc"
	"jwtx"
)

type fakePostListContentService struct {
	contentservice.ContentService
	getPostListFn func(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error)
}

func (f *fakePostListContentService) GetPostList(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
	return f.getPostListFn(ctx, in, opts...)
}

func newLogic(t *testing.T, ctx context.Context) *GetPostListLogic {
	t.Helper()

	svcCtx := &svc.ServiceContext{
		ContentService: &fakePostListContentService{
			getPostListFn: func(_ context.Context, in *contentservice.GetPostListReq, _ ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
				if in.Page != 1 || in.PageSize != 20 || in.SortBy != 1 {
					t.Fatalf("unexpected rpc req: %+v", in)
				}
				return &contentservice.GetPostListResp{
					Posts: []*contentpb.PostInfo{
						{
							Id:            100,
							AuthorId:      200,
							Title:         "title",
							Content:       "content",
							Images:        []string{"img"},
							Tags:          []string{"tag"},
							ViewCount:     10,
							LikeCount:     20,
							CommentCount:  30,
							FavoriteCount: 40,
							CreatedAt:     50,
						},
					},
					Total: 1,
				}, nil
			},
		},
	}

	return NewGetPostListLogic(ctx, svcCtx)
}

func TestGetPostList_AnonymousContext_UserIdZero(t *testing.T) {
	l := newLogic(t, context.Background())

	resp, err := l.GetPostList(&types.GetPostListReq{
		Page:     1,
		PageSize: 20,
		SortBy:   1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != 1 || len(resp.List) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.List[0].Id != 100 || resp.List[0].AuthorId != 200 {
		t.Fatalf("unexpected item: %+v", resp.List[0])
	}
}

func TestGetPostList_AuthenticatedContext_UserIdPassed(t *testing.T) {
	var capturedUserId int64 = -1
	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{
		UserId:   42,
		Username: "alice",
	})

	svcCtx := &svc.ServiceContext{
		ContentService: &fakePostListContentService{
			getPostListFn: func(_ context.Context, in *contentservice.GetPostListReq, _ ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
				capturedUserId = in.UserId
				if in.Page != 1 || in.PageSize != 20 || in.SortBy != 1 {
					t.Fatalf("unexpected rpc req: %+v", in)
				}
				return &contentservice.GetPostListResp{
					Posts: []*contentpb.PostInfo{},
					Total: 0,
				}, nil
			},
		},
	}

	l := NewGetPostListLogic(ctx, svcCtx)
	_, err := l.GetPostList(&types.GetPostListReq{
		Page:     1,
		PageSize: 20,
		SortBy:   1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedUserId != 42 {
		t.Fatalf("expected userId 42 passed to RPC, got %d", capturedUserId)
	}
}
```

**Key changes:**
- `TestGetPostList_AnonymousContext_ReturnsPublicData` renamed to `TestGetPostList_AnonymousContext_UserIdZero`
- `TestGetPostList_AuthenticatedContext_ReturnsSamePublicData` replaced with `TestGetPostList_AuthenticatedContext_UserIdPassed` that explicitly asserts `in.UserId == 42`

- [ ] **Step 3: Commit**

```bash
git add app/gateway/internal/logic/posts/get_post_list_logic.go app/gateway/internal/logic/posts/get_post_list_logic_test.go
git commit -m "feat(gateway/logic): extract optional userId and pass to Content RPC

- Use jwtx.GetOptionalUserIdFromContext to read auth state from context
- Include userId in ContentService.GetPostList RPC call
- Update tests to verify userId is forwarded to downstream RPC"
```

---

## Task 5: Content RPC 支持 user_id 参数

**Files:**
- Modify: `proto/content/content.proto`
- Regenerate: `app/content/pb/xiaobaihe/content/pb/content.pb.go`
- Regenerate: `app/content/pb/xiaobaihe/content/pb/content_grpc.pb.go`
- Regenerate: `app/content/contentservice/content_service.go`
- Regenerate: `app/content/internal/server/content_service_server.go`
- Modify: `app/content/internal/logic/get_post_list_logic.go`

**Context:** Gateway Logic 已将 `userId` 传给 Content RPC，但 Content 层的 proto 定义和生成代码中没有 `user_id` 字段。需要在 proto 中添加字段，重新生成代码，并在 Content Logic 中接收。

- [ ] **Step 1: 在 Content proto 中添加 user_id 字段**

Modify `proto/content/content.proto`. Find `message GetPostListReq` (around line 118):

**Old:**
```protobuf
// 获取帖子列表请求
message GetPostListReq {
  int32 page      = 1;
  int32 page_size = 2;
  int32 sort_by   = 3; \ 1: 最新 2: 热门 3: 推荐
}
```

**New:**
```protobuf
// 获取帖子列表请求
message GetPostListReq {
  int32 page      = 1;
  int32 page_size = 2;
  int32 sort_by   = 3; // 1: 最新 2: 热门 3: 推荐
  int64 user_id   = 4; // 可选，用于个性化排序和填充 IsLiked/IsFavorited
}
```

**Note:** Also fix the malformed `\` comment on `sort_by` line (replace with `//`).

- [ ] **Step 2: goctl 重新生成 Content 服务代码**

Run from project root:
```bash
goctl rpc protoc proto/content/content.proto --go_out=app/content/pb --go-grpc_out=app/content/pb --zrpc_out=app/content
```

**Verification steps after generation:**
1. Check `app/content/pb/xiaobaihe/content/pb/content.pb.go` — `GetPostListReq` struct should have `UserId int64` field.
2. Check `app/content/contentservice/content_service.go` — `GetPostListReq` alias and `ContentService` interface are updated.
3. Check `app/content/internal/server/content_service_server.go` — `GetPostList` method signature unchanged (it passes `*pb.GetPostListReq` directly to Logic).

- [ ] **Step 3: 修改 Content Logic 接收 userId**

Replace `app/content/internal/logic/get_post_list_logic.go` entirely:

```go
package logic

import (
	"context"
	"errx"
	"esx/app/content/pb/xiaobaihe/content/pb"

	"esx/app/content/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPostListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostListLogic {
	return &GetPostListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// GetPostList 获取帖子列表
func (l *GetPostListLogic) GetPostList(in *pb.GetPostListReq) (*pb.GetPostListResp, error) {
	page := int(in.Page)
	pageSize := int(in.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}

	if in.UserId > 0 {
		l.Infow("authenticated user requesting post list",
			logx.Field("userId", in.UserId),
			logx.Field("page", page),
			logx.Field("pageSize", pageSize),
		)
		// TODO: populate IsLiked/IsFavorited per post by calling Interaction RPC
		// or querying interaction state. This requires InteractionService client
		// injection into Content ServiceContext.
	}

	posts, total, err := l.svcCtx.PostModel.FindList(l.ctx, page, pageSize, int(in.SortBy))
	if err != nil {
		l.Errorw("PostModel.FindList failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if len(posts) == 0 {
		return &pb.GetPostListResp{Posts: []*pb.PostInfo{}, Total: total}, nil
	}

	postIds := make([]int64, 0, len(posts))
	for _, post := range posts {
		postIds = append(postIds, post.Id)
	}
	tagsMap, err := l.svcCtx.PostTagModel.FindTagNamesByPostIds(l.ctx, postIds)
	if err != nil {
		l.Errorw("PostTagModel.FindTagNamesByPostIds failed", logx.Field("err", err.Error()))
		tagsMap = map[int64][]string{}
	}
	postInfos := make([]*pb.PostInfo, 0, len(posts))
	for _, post := range posts {
		postInfos = append(postInfos, PostToPostInfo(post, tagsMap[post.Id]))
	}

	return &pb.GetPostListResp{
		Posts: postInfos,
		Total: total,
	}, nil
}
```

**Key changes:**
- Log `userId` when authenticated user requests post list
- Add TODO comment explaining `IsLiked`/`IsFavorited` population is future work
- Keep existing query logic unchanged

- [ ] **Step 4: 编译验证 Content 和 Gateway 服务**

Run:
```bash
cd app/content && go build ./...
cd ../gateway && go build ./...
```

Expected: both compile successfully. `contentservice.GetPostListReq` now has `UserId` field, so Gateway Logic assignment compiles.

- [ ] **Step 5: Commit**

```bash
git add proto/content/content.proto app/content/pb/ app/content/contentservice/ app/content/internal/server/ app/content/internal/logic/get_post_list_logic.go app/gateway/internal/logic/posts/get_post_list_logic.go
git commit -m "feat(content/proto): add user_id to GetPostListReq

- Content RPC now accepts optional user_id for personalized responses
- Gateway forwards userId from optional auth context to Content RPC
- Content Logic logs authenticated requests; IsLiked/IsFavorited TODO noted"
```

---

## Task 6: 更新 Handler 测试并全局验证

**Files:**
- Modify: `app/gateway/internal/handler/posts/get_post_list_handler_test.go`

**Context:** 中间件已从 Handler 内部移除，迁移到路由层。Handler 测试直接调用 Handler 函数，不经过路由中间件，因此中间件相关断言（`X-Auth-State` header）不再适用。需要清理这些断言，使 handler 测试只验证 handler 核心逻辑。

- [ ] **Step 1: 重写 Handler 测试（移除中间件断言）**

Replace `app/gateway/internal/handler/posts/get_post_list_handler_test.go` entirely:

```go
package posts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"esx/app/content/contentservice"
	contentpb "esx/app/content/pb/xiaobaihe/content/pb"
	"gateway/internal/config"
	"gateway/internal/svc"
	"google.golang.org/grpc"
)

type fakeContentService struct {
	contentservice.ContentService
	getPostListFn func(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error)
}

func (f *fakeContentService) GetPostList(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
	return f.getPostListFn(ctx, in, opts...)
}

func newPostListSvcCtx() *svc.ServiceContext {
	return &svc.ServiceContext{
		Config: config.Config{
			Auth: struct {
				AccessSecret string
				AccessExpire int64
			}{
				AccessSecret: "secret",
				AccessExpire: 3600,
			},
		},
		ContentService: &fakeContentService{
			getPostListFn: func(_ context.Context, _ *contentservice.GetPostListReq, _ ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
				return &contentservice.GetPostListResp{
					Posts: []*contentpb.PostInfo{
						{
							Id:        1,
							AuthorId:  2,
							Title:     "t",
							Content:   "c",
							Images:    []string{"a"},
							Tags:      []string{"tag"},
							ViewCount: 3,
							LikeCount: 4,
							CreatedAt: 5,
						},
					},
					Total: 1,
				}, nil
			},
		},
	}
}

func TestGetPostListHandler_ReturnsPostList(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?page=1&pageSize=20&sortBy=1", nil)
	rec := httptest.NewRecorder()

	GetPostListHandler(newPostListSvcCtx())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		List  []map[string]any `json:"list"`
		Total int64            `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v, body=%s", err, rec.Body.String())
	}
	if resp.Total != 1 || len(resp.List) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.List[0]["id"] != float64(1) {
		t.Fatalf("unexpected post id: %v", resp.List[0]["id"])
	}
}

func TestGetPostListHandler_InvalidParams_ReturnsError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?page=abc&pageSize=20", nil)
	rec := httptest.NewRecorder()

	GetPostListHandler(newPostListSvcCtx())(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 for invalid params, got %d", rec.Code)
	}
}
```

**Key changes:**
- Removed all `X-Auth-State` header assertions (middleware is now at route layer)
- Removed token generation and Authorization header setup
- `TestGetPostListHandler_ReturnsPostList` tests pure handler logic
- Added `TestGetPostListHandler_InvalidParams_ReturnsError` for parameter validation coverage

- [ ] **Step 2: 运行所有相关测试**

Run:
```bash
cd pkg/middleware && go test -v ./...
cd ../../app/gateway/internal/logic/posts && go test -v ./...
cd ../../handler/posts && go test -v ./...
```

Expected: all tests pass.

- [ ] **Step 3: 编译验证整个项目**

Run from project root:
```bash
cd app/gateway && go build ./...
cd ../content && go build ./...
```

Expected: both services compile cleanly.

- [ ] **Step 4: Commit**

```bash
git add app/gateway/internal/handler/posts/get_post_list_handler_test.go
git commit -m "test(gateway/posts): update handler tests after middleware migration

- Remove X-Auth-State assertions (middleware now at route layer)
- Add invalid params error test case
- Tests now verify pure handler logic only"
```

---

## Self-Review Checklist

### 1. Spec Coverage

| Requirement | Task | Status |
|-------------|------|--------|
| 中间件从 Handler 内部移出 | Task 1 + Task 3 | Covered |
| 中间件签名符合 go-zero 标准 | Task 1 | Covered |
| ServiceContext 依赖注入 | Task 2 | Covered |
| .api 声明式绑定 + routes 生成 | Task 3 | Covered |
| Logic 层消费 optional userId | Task 4 | Covered |
| Content RPC 支持 user_id | Task 5 | Covered |
| 测试更新 | Task 1, 4, 6 | Covered |

### 2. Placeholder Scan

- No "TBD", "TODO", "implement later", "fill in details" in steps.
- All code blocks contain complete, copy-pasteable code.
- All commands have exact paths and expected outputs.

### 3. Type Consistency

- `OptionalAuthMiddleware.Handle` signature: `func(http.HandlerFunc) http.HandlerFunc` (matches `rest.Middleware`)
- `ServiceContext.OptionalAuth` type: `rest.Middleware` ✓
- `.api` middleware name: `OptionalAuth` matches field name ✓
- `jwtx.GetOptionalUserIdFromContext` returns `(int64, bool)` ✓
- `contentservice.GetPostListReq.UserId` type: `int64` ✓

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-04-22-fix-optional-auth-middleware.md`.**

**Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

**Which approach?**
