# GetPostList Optional Auth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `GET /api/v1/posts` a public endpoint that supports optional JWT parsing, injects user info into `context` only when a valid token is present, and signals auth state to the frontend without returning `401` for public reads.

**Architecture:** Keep write endpoints on required JWT, move `GET /posts` into the public route group, and wrap only `GetPostListHandler` with a new `OptionalAuthMiddleware`. Centralize context injection and optional user lookup in `pkg/jwtx` so public-read logic can safely distinguish anonymous requests from real failures.

**Tech Stack:** Go 1.26.1, go-zero REST gateway, JWT (`github.com/golang-jwt/jwt/v4`), `goctl`, standard `net/http`/`httptest`

---

## File Structure

**Files to create or modify**

- Create: `pkg/jwtx/context.go`
- Create: `pkg/jwtx/context_test.go`
- Create: `pkg/middleware/optional_auth.go`
- Create: `pkg/middleware/optional_auth_test.go`
- Modify: `pkg/jwtx/jwt.go`
- Modify: `app/gateway/gateway.api`
- Modify (generated via `goctl`, do not hand-edit): `app/gateway/internal/handler/routes.go`
- Modify: `app/gateway/internal/handler/posts/get_post_list_handler.go`
- Create: `app/gateway/internal/handler/posts/get_post_list_handler_test.go`
- Create: `app/gateway/internal/logic/posts/get_post_list_logic_test.go`

**Responsibility split**

- `pkg/jwtx/context.go`: one place for injecting JWT claims into `context` and retrieving optional user info safely
- `pkg/middleware/optional_auth.go`: one place for non-blocking JWT parsing and `X-Auth-State` signaling
- `get_post_list_handler.go`: the only public posts handler that needs optional auth wrapping in this change
- `get_post_list_handler_test.go`: HTTP-level contract for `200 + X-Auth-State`
- `get_post_list_logic_test.go`: business contract that anonymous and authenticated contexts both work

---

### Task 1: Add Safe JWT Context Helpers

**Files:**
- Create: `pkg/jwtx/context.go`
- Create: `pkg/jwtx/context_test.go`
- Modify: `pkg/jwtx/jwt.go`

- [ ] **Step 1: Write the failing context-helper tests**

Create `pkg/jwtx/context_test.go`:

```go
package jwtx

import (
	"context"
	"testing"
)

func TestWithClaimsContext_StoresUserIDAsJsonNumber(t *testing.T) {
	ctx := WithClaimsContext(context.Background(), &Claims{
		UserId:   42,
		Username: "alice",
	})

	got, ok := GetOptionalUserIdFromContext(ctx)
	if !ok {
		t.Fatal("expected user id in context")
	}
	if got != 42 {
		t.Fatalf("expected user id 42, got %d", got)
	}
}

func TestGetOptionalUserIdFromContext_Missing_ReturnsFalse(t *testing.T) {
	got, ok := GetOptionalUserIdFromContext(context.Background())
	if ok {
		t.Fatalf("expected no user id, got %d", got)
	}
}

func TestGetUserIdFromContext_UsesOptionalHelper(t *testing.T) {
	ctx := WithClaimsContext(context.Background(), &Claims{
		UserId:   7,
		Username: "bob",
	})

	got, err := GetUserIdFromContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 7 {
		t.Fatalf("expected 7, got %d", got)
	}
}
```

- [ ] **Step 2: Run the jwtx tests to verify they fail**

Run:

```bash
go test ./pkg/jwtx -run 'Test(WithClaimsContext|GetOptionalUserIdFromContext|GetUserIdFromContext)' -v
```

Expected: FAIL with undefined `WithClaimsContext` and `GetOptionalUserIdFromContext`.

- [ ] **Step 3: Add the context helper implementation**

Create `pkg/jwtx/context.go`:

```go
package jwtx

import (
	"context"
	"encoding/json"
	"strconv"
)

const (
	ContextUserIDKey   = "userId"
	ContextUsernameKey = "username"
)

func WithClaimsContext(ctx context.Context, claims *Claims) context.Context {
	if claims == nil {
		return ctx
	}

	ctx = context.WithValue(
		ctx,
		ContextUserIDKey,
		json.Number(strconv.FormatInt(claims.UserId, 10)),
	)
	ctx = context.WithValue(ctx, ContextUsernameKey, claims.Username)

	return ctx
}

func GetOptionalUserIdFromContext(ctx context.Context) (int64, bool) {
	switch value := ctx.Value(ContextUserIDKey).(type) {
	case json.Number:
		id, err := value.Int64()
		if err != nil {
			return 0, false
		}
		return id, true
	case int64:
		return value, true
	case string:
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, false
		}
		return id, true
	default:
		return 0, false
	}
}
```

Update `pkg/jwtx/jwt.go` by replacing `GetUserIdFromContext` with:

```go
func GetUserIdFromContext(ctx context.Context) (int64, error) {
	userId, ok := GetOptionalUserIdFromContext(ctx)
	if !ok {
		return 0, fmt.Errorf("convert userId failed: %w", errx.NewWithCode(errx.SystemError))
	}
	return userId, nil
}
```

- [ ] **Step 4: Run the jwtx tests to verify they pass**

Run:

```bash
go test ./pkg/jwtx -run 'Test(WithClaimsContext|GetOptionalUserIdFromContext|GetUserIdFromContext)' -v
```

Expected: PASS for all three tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/jwtx/context.go pkg/jwtx/context_test.go pkg/jwtx/jwt.go
git commit -m "feat(jwtx): add optional context helpers for public auth"
```

---

### Task 2: Add Non-Blocking Optional Auth Middleware

**Files:**
- Create: `pkg/middleware/optional_auth.go`
- Create: `pkg/middleware/optional_auth_test.go`

- [ ] **Step 1: Write the failing middleware tests**

Create `pkg/middleware/optional_auth_test.go`:

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

	mw := OptionalAuthMiddleware(jwtx.JwtConfig{
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
	mw(next).ServeHTTP(rec, req)

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

- [ ] **Step 2: Run the middleware tests to verify they fail**

Run:

```bash
go test ./pkg/middleware -run TestOptionalAuthMiddleware -v
```

Expected: FAIL with undefined `OptionalAuthMiddleware`, `AuthStateHeader`, and auth-state constants.

- [ ] **Step 3: Implement the optional auth middleware**

Create `pkg/middleware/optional_auth.go`:

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

func OptionalAuthMiddleware(config jwtx.JwtConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				w.Header().Set(AuthStateHeader, AuthStateAnonymous)
				next.ServeHTTP(w, r)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				w.Header().Set(AuthStateHeader, AuthStateInvalid)
				next.ServeHTTP(w, r)
				return
			}

			claims, err := jwtx.ParseToken(parts[1], config)
			if err != nil {
				if errors.Is(err, jwtx.ErrTokenExpired) {
					w.Header().Set(AuthStateHeader, AuthStateExpired)
				} else {
					w.Header().Set(AuthStateHeader, AuthStateInvalid)
				}
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set(AuthStateHeader, AuthStateAuthenticated)
			next.ServeHTTP(w, r.WithContext(jwtx.WithClaimsContext(r.Context(), claims)))
		})
	}
}
```

- [ ] **Step 4: Run the middleware tests to verify they pass**

Run:

```bash
go test ./pkg/middleware -run TestOptionalAuthMiddleware -v
```

Expected: PASS for no-token, valid-token, expired-token, invalid-token, and bad-format cases.

- [ ] **Step 5: Commit**

```bash
git add pkg/middleware/optional_auth.go pkg/middleware/optional_auth_test.go
git commit -m "feat(middleware): add optional jwt middleware for public reads"
```

---

### Task 3: Make `GET /posts` Public and Wrap It With Optional Auth

**Files:**
- Modify: `app/gateway/gateway.api`
- Modify (generated via `goctl`, do not hand-edit): `app/gateway/internal/handler/routes.go`
- Modify: `app/gateway/internal/handler/posts/get_post_list_handler.go`
- Create: `app/gateway/internal/handler/posts/get_post_list_handler_test.go`

- [ ] **Step 1: Write the failing handler integration tests**

Create `app/gateway/internal/handler/posts/get_post_list_handler_test.go`:

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

func TestGetPostListHandler_NoToken_Returns200AndAnonymous(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?page=1&pageSize=20&sortBy=1", nil)
	rec := httptest.NewRecorder()

	GetPostListHandler(newPostListSvcCtx())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Auth-State"); got != "anonymous" {
		t.Fatalf("expected anonymous auth state, got %q", got)
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
}

func TestGetPostListHandler_ExpiredToken_Returns200AndExpired(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?page=1&pageSize=20&sortBy=1", nil)
	req.Header.Set("Authorization", "Bearer expired-token-value")
	rec := httptest.NewRecorder()

	GetPostListHandler(newPostListSvcCtx())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Auth-State"); got != "expired" && got != "invalid" {
		t.Fatalf("expected expired or invalid auth state, got %q", got)
	}
}
```

- [ ] **Step 2: Run the handler tests to verify they fail**

Run:

```bash
go test ./app/gateway/internal/handler/posts -run TestGetPostListHandler -v
```

Expected: FAIL because current handler neither sets `X-Auth-State` nor supports public optional auth.

- [ ] **Step 3: Move `/posts` into the public route group and regenerate gateway code**

Update `app/gateway/gateway.api` by moving:

```api
@doc "获取帖子列表"
@handler GetPostList
get /posts (GetPostListReq) returns (GetPostListResp)
```

out of the `jwt: Auth` posts group and into a public `@server(prefix: /api/v1)` block. The resulting route split should look like:

```api
@server (
	prefix: /api/v1
)
service gateway {
	@doc "获取帖子列表"
	@handler GetPostList
	get /posts (GetPostListReq) returns (GetPostListResp)
}

@server (
	prefix: /api/v1
	jwt:    Auth
	group:  posts
)
service gateway {
	@doc "创建帖子"
	@handler CreatePost
	post /post (CreatePostReq) returns (CreatePostResp)

	@doc "获取帖子详情"
	@handler GetPost
	get /post/:postId (GetPostReq) returns (GetPostResp)

	@doc "更新帖子"
	@handler UpdatePost
	put /post/:postId (UpdatePostReq) returns (UpdatePostResp)

	@doc "删除帖子"
	@handler DeletePost
	delete /post/:postId (DeletePostReq) returns (DeletePostResp)
}
```

Then regenerate:

```bash
goctl api go -api app/gateway/gateway.api -dir app/gateway --style go_zero
```

Expected: generated `app/gateway/internal/handler/routes.go` places `GET /posts` in a public route block and keeps write endpoints under `rest.WithJwt(...)`.

- [ ] **Step 4: Wrap `GetPostListHandler` with the optional auth middleware**

Replace `app/gateway/internal/handler/posts/get_post_list_handler.go` with:

```go
package posts

import (
	"net/http"

	"gateway/internal/logic/posts"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"
	"middleware"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetPostListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req types.GetPostListReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := posts.NewGetPostListLogic(r.Context(), svcCtx)
		resp, err := l.GetPostList(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		httpx.OkJsonCtx(r.Context(), w, resp)
	})

	return middleware.OptionalAuthMiddleware(jwtx.JwtConfig{
		AccessSecret: svcCtx.Config.Auth.AccessSecret,
		AccessExpire: svcCtx.Config.Auth.AccessExpire,
	})(inner).ServeHTTP
}
```

- [ ] **Step 5: Tighten the handler tests so expired token is deterministic**

Update `app/gateway/internal/handler/posts/get_post_list_handler_test.go` imports to add:

```go
	"jwtx"
```

Then replace the expired-token test with:

```go
func TestGetPostListHandler_ExpiredToken_Returns200AndExpired(t *testing.T) {
	token, err := jwtx.GenerateToken(42, "alice", jwtx.JwtConfig{
		AccessSecret: "secret",
		AccessExpire: -1,
	})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?page=1&pageSize=20&sortBy=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	GetPostListHandler(newPostListSvcCtx())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Auth-State"); got != "expired" {
		t.Fatalf("expected expired auth state, got %q", got)
	}
}
```

and add:

```go
func TestGetPostListHandler_ValidToken_Returns200AndAuthenticated(t *testing.T) {
	token, err := jwtx.GenerateToken(42, "alice", jwtx.JwtConfig{
		AccessSecret: "secret",
		AccessExpire: 3600,
	})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts?page=1&pageSize=20&sortBy=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	GetPostListHandler(newPostListSvcCtx())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Auth-State"); got != "authenticated" {
		t.Fatalf("expected authenticated auth state, got %q", got)
	}
}
```

- [ ] **Step 6: Run the handler tests to verify they pass**

Run:

```bash
go test ./app/gateway/internal/handler/posts -run TestGetPostListHandler -v
```

Expected: PASS for anonymous, valid-token, and expired-token requests.

- [ ] **Step 7: Commit**

```bash
git add app/gateway/gateway.api app/gateway/internal/handler/routes.go app/gateway/internal/handler/posts/get_post_list_handler.go app/gateway/internal/handler/posts/get_post_list_handler_test.go
git commit -m "feat(gateway/posts): make get post list public with optional auth"
```

---

### Task 4: Lock the GetPostList Business Contract and Verify the Full Flow

**Files:**
- Create: `app/gateway/internal/logic/posts/get_post_list_logic_test.go`

- [ ] **Step 1: Write the failing logic-contract tests**

Create `app/gateway/internal/logic/posts/get_post_list_logic_test.go`:

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

type fakeContentService struct {
	contentservice.ContentService
	getPostListFn func(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error)
}

func (f *fakeContentService) GetPostList(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
	return f.getPostListFn(ctx, in, opts...)
}

func newLogic(t *testing.T, ctx context.Context) *GetPostListLogic {
	t.Helper()

	svcCtx := &svc.ServiceContext{
		ContentService: &fakeContentService{
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

func TestGetPostList_AnonymousContext_ReturnsPublicData(t *testing.T) {
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

func TestGetPostList_AuthenticatedContext_ReturnsSamePublicData(t *testing.T) {
	ctx := jwtx.WithClaimsContext(context.Background(), &jwtx.Claims{
		UserId:   42,
		Username: "alice",
	})
	l := newLogic(t, ctx)

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
```

- [ ] **Step 2: Run the logic tests to verify they fail**

Run:

```bash
go test ./app/gateway/internal/logic/posts -run TestGetPostList_ -v
```

Expected: FAIL because the new test file does not exist yet or imports are unresolved before the helper/middleware work is complete.

- [ ] **Step 3: Adjust the logic tests to compile cleanly once helpers exist**

If `fakeContentService` name collides with other test files, rename it in `app/gateway/internal/logic/posts/get_post_list_logic_test.go` to:

```go
type fakePostListContentService struct {
	contentservice.ContentService
	getPostListFn func(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error)
}

func (f *fakePostListContentService) GetPostList(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
	return f.getPostListFn(ctx, in, opts...)
}
```

and update `newLogic` accordingly:

```go
	ContentService: &fakePostListContentService{
```

- [ ] **Step 4: Run the logic tests to verify they pass**

Run:

```bash
go test ./app/gateway/internal/logic/posts -run TestGetPostList_ -v
```

Expected: PASS for anonymous-context and authenticated-context tests.

- [ ] **Step 5: Run broader verification**

Run:

```bash
go test ./pkg/jwtx ./pkg/middleware ./app/gateway/internal/handler/posts ./app/gateway/internal/logic/posts -v
```

Expected: PASS across helper, middleware, handler, and logic tests.

Then run:

```bash
go test ./app/gateway/... 
```

Expected: PASS for the full gateway module.

- [ ] **Step 6: Do manual endpoint verification**

Run after starting the gateway locally:

```bash
curl -i "http://localhost:8888/api/v1/posts?page=1&pageSize=20&sortBy=1"
```

Expected:

```text
HTTP/1.1 200 OK
X-Auth-State: anonymous
```

Run with an expired token:

```bash
curl -i -H "Authorization: Bearer <expired-token>" "http://localhost:8888/api/v1/posts?page=1&pageSize=20&sortBy=1"
```

Expected:

```text
HTTP/1.1 200 OK
X-Auth-State: expired
```

Run a required-auth write endpoint without token:

```bash
curl -i -X POST "http://localhost:8888/api/v1/post" -H "Content-Type: application/json" -d "{\"title\":\"t\",\"content\":\"c\"}"
```

Expected:

```text
HTTP/1.1 401 Unauthorized
```

- [ ] **Step 7: Commit**

```bash
git add app/gateway/internal/logic/posts/get_post_list_logic_test.go
git commit -m "test(gateway/posts): lock public get post list auth contract"
```

---

## Self-Review

### Spec coverage

- Public `GET /posts`: covered in Task 3
- Optional JWT parsing: covered in Task 2
- Context injection only for valid token: covered in Task 1 + Task 2
- `X-Auth-State` signaling: covered in Task 2 + Task 3 + Task 4
- Anonymous/authenticated logic compatibility: covered in Task 4
- Required-auth endpoints still `401`: covered in Task 3 regeneration and Task 4 manual verification

### Placeholder scan

- No `TODO`, `TBD`, or “implement later”
- Each code-edit step includes concrete code
- Each test step includes exact commands and expected outcomes

### Type consistency

- `WithClaimsContext`, `GetOptionalUserIdFromContext`, `OptionalAuthMiddleware`, `AuthStateHeader`, and auth-state constants are defined once and reused consistently
- `GetPostListHandler` is the only handler wrapped in optional auth for this feature
- `X-Auth-State` values are fixed as `anonymous`, `authenticated`, `expired`, `invalid`
