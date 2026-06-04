# BizError gRPC 透传方案实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `errx.BizError` 在 RPC 服务和 Gateway 之间完整透传，使 Gateway 能从 gRPC 错误中恢复出原始业务错误码。

**Architecture:** `BizError` 实现 `GRPCStatus()` 接口，gRPC 框架序列化时保留业务码（通过 `status.WithDetails` 携带 `wrapperspb.Int32Value`）；Gateway 侧新增客户端拦截器，自动将 gRPC Status 还原为 `BizError`。gRPC code 使用标准码映射（从 `HTTPStatus()` 派生），确保熔断器/监控语义正确。

**Tech Stack:** Go 1.26.1, go-zero v1.10.1, google.golang.org/grpc v1.80.0, google.golang.org/protobuf v1.36.11

---

## 背景与问题

当前 `BizError` 没有实现 `GRPCStatus()` 接口。gRPC 核心在 `server.go:1432` 调用 `status.FromError(appErr)` 时，走入 fallback 路径：

```go
// google.golang.org/grpc/status/status.go:126
return New(codes.Unknown, err.Error()), false
```

结果：业务码 1001 被扁平化为字符串 `"code: 1001, message: 用户不存在"`，Gateway 收到的是 `status.Error{codes.Unknown, "..."}` ——`errors.AsType[*errx.BizError]` 失败，所有 RPC 错误都回退为 HTTP 500 + SystemError。

## 文件结构

| 操作 | 文件 | 职责 |
|------|------|------|
| 重写 | `pkg/errx/resolve_grpc.go` | `GRPCCode()`、`GRPCStatus()`、`FromGRPCError()` |
| 新建 | `pkg/errx/resolve_grpc_test.go` | 上述三个函数的单元测试 |
| 修改 | `pkg/errx/go.mod` | 添加 grpc + protobuf 依赖 |
| 新建 | `pkg/interceptor/biz_error.go` | 客户端 Unary 拦截器 |
| 新建 | `pkg/interceptor/biz_error_test.go` | 拦截器单元测试 |
| 修改 | `pkg/interceptor/go.mod` | 添加 errx 依赖 |
| 修改 | `app/gateway/internal/svc/service_context.go` | 三个 RPC Client 接入拦截器 |
| 修改 | `app/gateway/go.mod` | 添加 interceptor 依赖 |
| 清理 | `app/gateway/internal/logic/login/login_logic.go` | 删除未使用的 `grpc/status` import |

---

## Task 1: 为 errx 模块添加 gRPC 依赖

**Files:**
- Modify: `pkg/errx/go.mod`

- [ ] **Step 1: 添加依赖**

```bash
cd pkg/errx && go get google.golang.org/grpc@v1.80.0 google.golang.org/protobuf@v1.36.11
```

- [ ] **Step 2: 验证 go.mod 更新**

```bash
cd pkg/errx && cat go.mod
```

Expected: `go.mod` 中出现 `google.golang.org/grpc` 和 `google.golang.org/protobuf`。

- [ ] **Step 3: 确认编译通过**

```bash
cd pkg/errx && go build ./...
```

Expected: 无错误。

---

## Task 2: 实现 BizError → gRPC Status 转换 (TDD)

**Files:**
- Create: `pkg/errx/resolve_grpc_test.go`
- Rewrite: `pkg/errx/resolve_grpc.go`

### Step 1-4: GRPCCode() — RED → GREEN

- [ ] **Step 1: 写失败测试 — GRPCCode 映射**

Create `pkg/errx/resolve_grpc_test.go`:

```go
package errx

import (
	"testing"

	"google.golang.org/grpc/codes"
)

func TestBizError_GRPCCode(t *testing.T) {
	tests := []struct {
		name     string
		bizCode  int
		wantCode codes.Code
	}{
		{"SUCCESS maps to OK", SUCCESS, codes.OK},
		{"ParamError maps to InvalidArgument", ParamError, codes.InvalidArgument},
		{"SystemError maps to Internal", SystemError, codes.Internal},
		{"UserNotFound maps to NotFound", UserNotFound, codes.NotFound},
		{"ContentNotFound maps to NotFound", ContentNotFound, codes.NotFound},
		{"MediaNotFound maps to NotFound", MediaNotFound, codes.NotFound},
		{"LoginRequired maps to Unauthenticated", LoginRequired, codes.Unauthenticated},
		{"TokenExpired maps to Unauthenticated", TokenExpired, codes.Unauthenticated},
		{"TokenInvalid maps to Unauthenticated", TokenInvalid, codes.Unauthenticated},
		{"PermissionDenied maps to PermissionDenied", PermissionDenied, codes.PermissionDenied},
		{"ContentForbidden maps to PermissionDenied", ContentForbidden, codes.PermissionDenied},
		{"FavoritesPrivate maps to PermissionDenied", FavoritesPrivate, codes.PermissionDenied},
		{"UserAlreadyExist maps to AlreadyExists", UserAlreadyExist, codes.AlreadyExists},
		{"TooManyReq maps to ResourceExhausted", TooManyReq, codes.ResourceExhausted},
		{"ServiceUnavailable maps to Unavailable", ServiceUnavailable, codes.Unavailable},
		{"PostAlreadyDeleted maps to NotFound", PostAlreadyDeleted, codes.NotFound},
		{"TitleEmpty maps to InvalidArgument", TitleEmpty, codes.InvalidArgument},
		{"FileTooLarge maps to InvalidArgument", FileTooLarge, codes.InvalidArgument},
		{"AlreadyLiked maps to InvalidArgument", AlreadyLiked, codes.InvalidArgument},
		{"CannotFollowSelf maps to InvalidArgument", CannotFollowSelf, codes.InvalidArgument},
		{"NotLikedYet maps to FailedPrecondition", NotLikedYet, codes.FailedPrecondition},
		{"NotFavoritedYet maps to FailedPrecondition", NotFavoritedYet, codes.FailedPrecondition},
		{"UnknownError maps to Internal", UnknownError, codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bizErr := &BizError{Code: tt.bizCode, Message: GetMsg(tt.bizCode)}
			if got := bizErr.GRPCCode(); got != tt.wantCode {
				t.Errorf("GRPCCode() = %v, want %v", got, tt.wantCode)
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd pkg/errx && go test -run TestBizError_GRPCCode -v
```

Expected: FAIL — `BizError.GRPCCode undefined`

- [ ] **Step 3: 实现 GRPCCode()**

Rewrite `pkg/errx/resolve_grpc.go`（删除全部注释代码）:

```go
package errx

import (
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// GRPCCode maps BizError to a standard gRPC status code.
// Derived from HTTPStatus() to keep HTTP/gRPC semantics consistent.
func (e *BizError) GRPCCode() codes.Code {
	switch e.HTTPStatus() {
	case http.StatusOK:
		return codes.OK
	case http.StatusBadRequest:
		if e.Code == NotLikedYet || e.Code == NotFavoritedYet {
			return codes.FailedPrecondition
		}
		return codes.InvalidArgument
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusNotFound, http.StatusGone:
		return codes.NotFound
	case http.StatusConflict:
		return codes.AlreadyExists
	case http.StatusTooManyRequests:
		return codes.ResourceExhausted
	case http.StatusServiceUnavailable:
		return codes.Unavailable
	default:
		return codes.Internal
	}
}
```

> **注意**: `NotLikedYet` / `NotFavoritedYet` 在 `HTTPStatus()` 中映射到 `400 BadRequest`，但语义上更适合 `FailedPrecondition`（"操作的前置条件未满足"），因此在 gRPC 层做了更精准的映射。`HTTPStatus()` 不变——它的调用方是浏览器/客户端，400 对前端更友好。

- [ ] **Step 4: 运行测试确认通过**

```bash
cd pkg/errx && go test -run TestBizError_GRPCCode -v
```

Expected: PASS

### Step 5-8: GRPCStatus() — RED → GREEN

- [ ] **Step 5: 写失败测试 — GRPCStatus 编码**

Append to `pkg/errx/resolve_grpc_test.go`:

```go
func TestBizError_GRPCStatus(t *testing.T) {
	bizErr := &BizError{Code: UserNotFound, Message: "用户不存在"}

	st := bizErr.GRPCStatus()

	// 1. gRPC code should be NotFound
	if st.Code() != codes.NotFound {
		t.Errorf("GRPCStatus().Code() = %v, want %v", st.Code(), codes.NotFound)
	}

	// 2. Message should be preserved
	if st.Message() != "用户不存在" {
		t.Errorf("GRPCStatus().Message() = %q, want %q", st.Message(), "用户不存在")
	}

	// 3. Detail should contain business code as Int32Value
	details := st.Details()
	if len(details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(details))
	}
	v, ok := details[0].(*wrapperspb.Int32Value)
	if !ok {
		t.Fatalf("detail type = %T, want *wrapperspb.Int32Value", details[0])
	}
	if int(v.Value) != UserNotFound {
		t.Errorf("detail value = %d, want %d", v.Value, UserNotFound)
	}
}

func TestBizError_GRPCStatus_SuccessReturnsNil(t *testing.T) {
	bizErr := &BizError{Code: SUCCESS, Message: "成功"}
	st := bizErr.GRPCStatus()

	// codes.OK status — Err() should return nil per gRPC contract
	if st.Err() != nil {
		t.Errorf("SUCCESS BizError should produce nil Err(), got %v", st.Err())
	}
}
```

- [ ] **Step 6: 运行测试确认失败**

```bash
cd pkg/errx && go test -run TestBizError_GRPCStatus -v
```

Expected: FAIL — `BizError.GRPCStatus undefined`

- [ ] **Step 7: 实现 GRPCStatus()**

Add to `pkg/errx/resolve_grpc.go` (after `GRPCCode`):

```go
// GRPCStatus implements the grpcstatus interface recognized by grpc/status.FromError.
// Business code is carried as a wrapperspb.Int32Value detail so the client can reconstruct BizError.
func (e *BizError) GRPCStatus() *status.Status {
	st := status.New(e.GRPCCode(), e.Message)
	detailed, err := st.WithDetails(wrapperspb.Int32(int32(e.Code)))
	if err != nil {
		return st
	}
	return detailed
}
```

- [ ] **Step 8: 运行测试确认全部通过**

```bash
cd pkg/errx && go test -run TestBizError_GRPCStatus -v
```

Expected: PASS

- [ ] **Step 9: 提交**

```bash
git -C "D:\Learning\projects\work projects\little-white-box" add pkg/errx/resolve_grpc.go pkg/errx/resolve_grpc_test.go pkg/errx/go.mod pkg/errx/go.sum
git -C "D:\Learning\projects\work projects\little-white-box" commit -m "feat(errx): implement GRPCCode and GRPCStatus for BizError

BizError now implements the gRPC GRPCStatus() interface so the framework
preserves business error codes across the wire. Standard gRPC codes are
derived from HTTPStatus() for correct breaker/monitoring semantics.
Business code is carried as wrapperspb.Int32Value detail.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 3: 实现 gRPC Status → BizError 还原 (TDD)

**Files:**
- Modify: `pkg/errx/resolve_grpc_test.go`
- Modify: `pkg/errx/resolve_grpc.go`

### Step 1-4: FromGRPCError — 有 Detail 的情况

- [ ] **Step 1: 写失败测试 — 从 gRPC Status 提取 BizError**

Append to `pkg/errx/resolve_grpc_test.go`:

```go
func TestFromGRPCError_WithBizDetail(t *testing.T) {
	// Simulate: RPC server returned BizError, serialized through GRPCStatus()
	original := &BizError{Code: UserNotFound, Message: "用户不存在"}
	grpcErr := original.GRPCStatus().Err()

	// Act: convert back
	result := FromGRPCError(grpcErr)

	// Assert: should be a BizError with the same code and message
	bizErr, ok := result.(*BizError)
	if !ok {
		t.Fatalf("FromGRPCError returned %T, want *BizError", result)
	}
	if bizErr.Code != UserNotFound {
		t.Errorf("Code = %d, want %d", bizErr.Code, UserNotFound)
	}
	if bizErr.Message != "用户不存在" {
		t.Errorf("Message = %q, want %q", bizErr.Message, "用户不存在")
	}
}

func TestFromGRPCError_Nil(t *testing.T) {
	if err := FromGRPCError(nil); err != nil {
		t.Errorf("FromGRPCError(nil) = %v, want nil", err)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd pkg/errx && go test -run TestFromGRPCError -v
```

Expected: FAIL — `FromGRPCError undefined`

- [ ] **Step 3: 实现 FromGRPCError()**

Add to `pkg/errx/resolve_grpc.go`:

```go
// FromGRPCError converts a gRPC status error back to a BizError.
// If the status carries a BizError detail (Int32Value), the original business code and message are restored.
// If not (e.g. framework-generated timeout/breaker errors), a BizError is synthesized from the gRPC code.
// Non-gRPC errors and nil are returned as-is.
func FromGRPCError(err error) error {
	if err == nil {
		return nil
	}

	s, ok := status.FromError(err)
	if !ok {
		return err
	}

	for _, detail := range s.Details() {
		if v, ok := detail.(*wrapperspb.Int32Value); ok {
			return &BizError{
				Code:    int(v.Value),
				Message: s.Message(),
			}
		}
	}

	return &BizError{
		Code:    grpcCodeToBizCode(s.Code()),
		Message: s.Message(),
	}
}

func grpcCodeToBizCode(c codes.Code) int {
	switch c {
	case codes.OK:
		return SUCCESS
	case codes.InvalidArgument:
		return ParamError
	case codes.NotFound:
		return NotFound
	case codes.Unauthenticated:
		return LoginRequired
	case codes.PermissionDenied:
		return PermissionDenied
	case codes.AlreadyExists:
		return UserAlreadyExist
	case codes.ResourceExhausted:
		return TooManyReq
	case codes.Unavailable:
		return ServiceUnavailable
	case codes.DeadlineExceeded:
		return SystemError
	default:
		return SystemError
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
cd pkg/errx && go test -run TestFromGRPCError -v
```

Expected: PASS

### Step 5-8: FromGRPCError — 无 Detail 的情况（框架错误）

- [ ] **Step 5: 写失败测试 — 框架生成的 gRPC 错误**

Append to `pkg/errx/resolve_grpc_test.go`:

```go
func TestFromGRPCError_FrameworkErrors(t *testing.T) {
	tests := []struct {
		name     string
		grpcCode codes.Code
		grpcMsg  string
		wantBiz  int
	}{
		{"DeadlineExceeded → SystemError", codes.DeadlineExceeded, "context deadline exceeded", SystemError},
		{"Unavailable → ServiceUnavailable", codes.Unavailable, "service unavailable", ServiceUnavailable},
		{"Internal → SystemError", codes.Internal, "panic: something", SystemError},
		{"ResourceExhausted → TooManyReq", codes.ResourceExhausted, "cpu overloaded", TooManyReq},
		{"Unknown → SystemError", codes.Unknown, "unexpected", SystemError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcErr := status.Error(tt.grpcCode, tt.grpcMsg)
			result := FromGRPCError(grpcErr)

			bizErr, ok := result.(*BizError)
			if !ok {
				t.Fatalf("FromGRPCError returned %T, want *BizError", result)
			}
			if bizErr.Code != tt.wantBiz {
				t.Errorf("Code = %d, want %d", bizErr.Code, tt.wantBiz)
			}
			if bizErr.Message != tt.grpcMsg {
				t.Errorf("Message = %q, want %q", bizErr.Message, tt.grpcMsg)
			}
		})
	}
}

func TestFromGRPCError_NonGRPCError(t *testing.T) {
	plainErr := fmt.Errorf("some plain error")
	result := FromGRPCError(plainErr)

	// Non-gRPC error should be returned as-is
	if result != plainErr {
		t.Errorf("expected original error, got %v", result)
	}
}
```

> **注意**: `TestFromGRPCError_NonGRPCError` 需要在文件顶部增加 `"fmt"` import。

- [ ] **Step 6: 运行测试确认通过**

```bash
cd pkg/errx && go test -run TestFromGRPCError -v
```

Expected: PASS（Step 3 的实现已覆盖这些场景）

- [ ] **Step 7: 运行完整 errx 测试套件**

```bash
cd pkg/errx && go test ./... -v -race
```

Expected: 全部 PASS。

- [ ] **Step 8: 提交**

```bash
git -C "D:\Learning\projects\work projects\little-white-box" add pkg/errx/resolve_grpc.go pkg/errx/resolve_grpc_test.go
git -C "D:\Learning\projects\work projects\little-white-box" commit -m "feat(errx): add FromGRPCError to restore BizError from gRPC status

Extracts business code from Int32Value detail if present (BizError origin),
otherwise maps standard gRPC codes to generic business codes (framework errors
like timeout, breaker, panic recovery).

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 4: 实现客户端拦截器 (TDD)

**Files:**
- Modify: `pkg/interceptor/go.mod`
- Create: `pkg/interceptor/biz_error.go`
- Create: `pkg/interceptor/biz_error_test.go`

- [ ] **Step 1: 添加 errx 依赖到 interceptor 模块**

```bash
cd pkg/interceptor && go get errx@v0.0.0
```

> 由于 errx 是 workspace 本地模块，`go get` 可能不适用。如果失败，手动在 `go.mod` 中添加 `require errx v0.0.0` 并确认 `go.work` 已包含 `./pkg/interceptor`（已包含）。然后运行 `go mod tidy`。

- [ ] **Step 2: 写失败测试**

Create `pkg/interceptor/biz_error_test.go`:

```go
package interceptor

import (
	"context"
	"errx"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestBizErrorInterceptor_NilError(t *testing.T) {
	interceptor := BizErrorUnaryInterceptor()
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return nil
	}

	err := interceptor(context.Background(), "/test/Method", nil, nil, nil, invoker)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestBizErrorInterceptor_ConvertsBizError(t *testing.T) {
	interceptor := BizErrorUnaryInterceptor()

	// Simulate: server returned BizError{1001, "用户不存在"} via GRPCStatus()
	st := status.New(codes.NotFound, "用户不存在")
	detailed, _ := st.WithDetails(wrapperspb.Int32(int32(errx.UserNotFound)))
	rpcErr := detailed.Err()

	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return rpcErr
	}

	err := interceptor(context.Background(), "/test/Method", nil, nil, nil, invoker)

	bizErr, ok := err.(*errx.BizError)
	if !ok {
		t.Fatalf("expected *errx.BizError, got %T", err)
	}
	if bizErr.Code != errx.UserNotFound {
		t.Errorf("Code = %d, want %d", bizErr.Code, errx.UserNotFound)
	}
	if bizErr.Message != "用户不存在" {
		t.Errorf("Message = %q, want %q", bizErr.Message, "用户不存在")
	}
}

func TestBizErrorInterceptor_FrameworkError(t *testing.T) {
	interceptor := BizErrorUnaryInterceptor()

	// Framework-generated error (no BizError detail)
	rpcErr := status.Error(codes.DeadlineExceeded, "context deadline exceeded")

	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return rpcErr
	}

	err := interceptor(context.Background(), "/test/Method", nil, nil, nil, invoker)

	bizErr, ok := err.(*errx.BizError)
	if !ok {
		t.Fatalf("expected *errx.BizError, got %T", err)
	}
	if bizErr.Code != errx.SystemError {
		t.Errorf("Code = %d, want %d", bizErr.Code, errx.SystemError)
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

```bash
cd pkg/interceptor && go test -run TestBizErrorInterceptor -v
```

Expected: FAIL — `BizErrorUnaryInterceptor undefined`

- [ ] **Step 4: 实现拦截器**

Create `pkg/interceptor/biz_error.go`:

```go
package interceptor

import (
	"context"
	"errx"

	"google.golang.org/grpc"
)

// BizErrorUnaryInterceptor returns a gRPC client unary interceptor that converts
// gRPC status errors back to errx.BizError using errx.FromGRPCError.
func BizErrorUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			return errx.FromGRPCError(err)
		}
		return nil
	}
}
```

- [ ] **Step 5: 运行测试确认通过**

```bash
cd pkg/interceptor && go test -run TestBizErrorInterceptor -v -race
```

Expected: PASS

- [ ] **Step 6: 提交**

```bash
git -C "D:\Learning\projects\work projects\little-white-box" add pkg/interceptor/biz_error.go pkg/interceptor/biz_error_test.go pkg/interceptor/go.mod pkg/interceptor/go.sum
git -C "D:\Learning\projects\work projects\little-white-box" commit -m "feat(interceptor): add BizError client unary interceptor

Converts gRPC status errors back to errx.BizError on the client side,
enabling Gateway's httpx.SetErrorHandlerCtx to extract business codes.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 5: 接入 Gateway 服务

**Files:**
- Modify: `app/gateway/internal/svc/service_context.go`
- Modify: `app/gateway/go.mod`

- [ ] **Step 1: 添加 interceptor 依赖到 gateway 模块**

```bash
cd app/gateway && go get interceptor@v0.0.0
```

> 同 Task 4 Step 1，workspace 模块可能需手动添加 require + go mod tidy。

- [ ] **Step 2: 修改 service_context.go — 为三个 RPC Client 注入拦截器**

Modify `app/gateway/internal/svc/service_context.go`:

```go
package svc

import (
	"esx/app/content/contentservice"
	"esx/app/media/mediaservice"
	"gateway/internal/config"
	"interceptor"
	"user/userservice"

	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config         config.Config
	UserService    userservice.UserService
	ContentService contentservice.ContentService
	MediaService   mediaservice.MediaService
}

func NewServiceContext(c config.Config) *ServiceContext {
	bizErrInterceptor := interceptor.BizErrorUnaryInterceptor()

	userClient := zrpc.MustNewClient(c.UserRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	userService := userservice.NewUserService(userClient)
	contentClient := zrpc.MustNewClient(c.ContentRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	contentService := contentservice.NewContentService(contentClient)
	mediaClient := zrpc.MustNewClient(c.MediaRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
	mediaService := mediaservice.NewMediaService(mediaClient)

	return &ServiceContext{
		Config:         c,
		UserService:    userService,
		ContentService: contentService,
		MediaService:   mediaService,
	}
}
```

- [ ] **Step 3: 确认编译通过**

```bash
cd app/gateway && go build ./...
```

Expected: 无错误。

- [ ] **Step 4: 提交**

```bash
git -C "D:\Learning\projects\work projects\little-white-box" add app/gateway/internal/svc/service_context.go app/gateway/go.mod app/gateway/go.sum
git -C "D:\Learning\projects\work projects\little-white-box" commit -m "feat(gateway): wire BizError interceptor into all RPC clients

All three RPC clients (user, content, media) now use the BizErrorUnaryInterceptor,
which restores BizError from gRPC status so the HTTP error handler can return
correct business error codes and HTTP status to clients.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 6: 清理与验证

**Files:**
- Clean: `app/gateway/internal/logic/login/login_logic.go`

- [ ] **Step 1: 删除 login_logic.go 中未使用的 grpc/status import**

File `app/gateway/internal/logic/login/login_logic.go:15` 有 `"google.golang.org/grpc/status"` import 但未使用。删除该行。

修改后的 import 块：

```go
import (
	"context"
	"esx/pkg/validator"
	"gateway/internal/svc"
	"gateway/internal/types"
	"user/userservice"

	"github.com/zeromicro/go-zero/core/logx"
)
```

- [ ] **Step 2: 运行全项目编译**

```bash
cd "D:\Learning\projects\work projects\little-white-box" && go build ./...
```

Expected: 无错误。

- [ ] **Step 3: 运行全项目测试**

```bash
cd "D:\Learning\projects\work projects\little-white-box" && go test ./pkg/errx/... ./pkg/interceptor/... -v -race -cover
```

Expected: 全部 PASS，errx 覆盖率 ≥ 80%。

- [ ] **Step 4: 提交**

```bash
git -C "D:\Learning\projects\work projects\little-white-box" add app/gateway/internal/logic/login/login_logic.go
git -C "D:\Learning\projects\work projects\little-white-box" commit -m "chore(gateway): remove unused grpc/status import from login logic

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## 附录 A: 拦截器链中的错误流转（改造后）

```
[RPC Server 侧]
Logic 返回 errx.BizError{Code: 1001, Message: "用户不存在"}
  ↓
go-zero Server 拦截器链（Tracing → Recover → Stat → Breaker → Timeout）
  → 全部原样透传 error（和改造前一样）
  ↓
gRPC core server.go:1432
  status.FromError(bizErr) → 发现 GRPCStatus() 接口 ✅
  → Status{Code: NotFound(5), Message: "用户不存在", Details: [Int32Value(1001)]}
  ↓
HTTP/2 传输
  grpc-status: 5
  grpc-message: 用户不存在
  grpc-status-details-bin: <proto-encoded Int32Value(1001)>

[Gateway Client 侧]
gRPC core 反序列化 → status.Error{Code: NotFound, Message: "用户不存在", Details: [...]}
  ↓
go-zero Client 拦截器链（Timeout → Tracing → Duration → Breaker）
  → Duration: codes.NotFound → err != nil → logger.Errorf(...)
  → Breaker: codes.NotFound 不在不可接受列表 → 不触发熔断 ✅
  ↓
BizErrorUnaryInterceptor（我们新加的，在最外层）
  → errx.FromGRPCError(err) → 从 Details 提取 Int32Value(1001)
  → 返回 &BizError{Code: 1001, Message: "用户不存在"} ✅
  ↓
Gateway Logic → return nil, err
  ↓
httpx.SetErrorHandlerCtx
  → errors.AsType[*errx.BizError] → 成功 ✅
  → HTTPStatus() → 404
  → JSON: {"code": 1001, "message": "用户不存在"} ✅
```

## 附录 B: 熔断器行为对照

| 错误来源 | 改造前 gRPC Code | 改造后 gRPC Code | 熔断器判定 |
|---------|-----------------|-----------------|----------|
| `errx.UserNotFound` | `Unknown(2)` | `NotFound(5)` | ✅ 可接受（不熔断） |
| `errx.SystemError` | `Unknown(2)` | `Internal(13)` | ✅ 不可接受（计入熔断） |
| `errx.ServiceUnavailable` | `Unknown(2)` | `Unavailable(14)` | ✅ 不可接受（计入熔断） |
| `errx.ParamError` | `Unknown(2)` | `InvalidArgument(3)` | ✅ 可接受（不熔断） |
| Timeout (框架) | `DeadlineExceeded(4)` | `DeadlineExceeded(4)` | ✅ 不可接受（不变） |
| Panic (框架) | `Internal(13)` | `Internal(13)` | ✅ 不可接受（不变） |

改造前所有业务错误都是 `Unknown` → 全部"可接受"，即使 `SystemError` 也不触发熔断（这是 bug）。改造后映射正确。

## 附录 C: 后续改进建议（不在本计划范围内）

1. **Gateway Logic 简化**: 部分 Gateway Logic 在 RPC 调用失败后手动创建新的 BizError（如 `errx.NewWithCode(errx.SystemError)`），屏蔽了 RPC 服务返回的精确错误码。接入拦截器后，这些逻辑可以简化为直接 `return nil, err`。
2. **Duration 拦截器日志级别**: go-zero 客户端 `DurationInterceptor` 对所有 error 都打 Error 级别日志，包括业务级的 "用户不存在"。可考虑自定义拦截器按 gRPC code 区分日志级别。
3. **Stream 拦截器**: 本计划只处理 Unary RPC。如果未来使用 Stream RPC，需要添加对应的 `StreamClientInterceptor`。
