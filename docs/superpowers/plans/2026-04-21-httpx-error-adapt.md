# httpx 错误适配实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Gateway 的 `httpx.SetErrorHandlerCtx` 正确处理 `httpx.Parse` 产生的参数校验/JSON 解析错误，返回 `400 ParamError` 而非 `500 SystemError`。

**Architecture:** 在 `pkg/errx` 新增 `FromHTTPError(err) *BizError`，将非 BizError 的普通 error 转为 `BizError{Code: ParamError, Message: err.Error()}`。Gateway 的错误处理回调增加一级 fallback：`errors.AsType[*BizError]` 失败后调用 `FromHTTPError` 兜底，保证所有错误路径都产出 `BizError`，统一格式化响应。

**Tech Stack:** Go 1.26.1, go-zero v1.10.1

---

## 背景与问题

当前 `gateway.go:35-46` 的 `SetErrorHandlerCtx` 回调只处理 `*errx.BizError`：

```go
httpx.SetErrorHandlerCtx(func(ctx context.Context, err error) (int, any) {
    if bizErr, ok := errors.AsType[*errx.BizError](err); ok {
        return bizErr.HTTPStatus(), map[string]any{...}
    }
    return http.StatusInternalServerError, map[string]any{
        "code":    errx.SystemError,
        "message": errx.GetMsg(errx.SystemError),
    }
})
```

但 goctl 生成的 Handler 有两条错误路径都走 `httpx.ErrorCtx`：

```go
// 路径 1: httpx.Parse 失败 — 参数解析/校验错误
if err := httpx.Parse(r, &req); err != nil {
    httpx.ErrorCtx(r.Context(), w, err)  // ← plain error, NOT BizError
    return
}
// 路径 2: Logic 层返回错误 — 已包装为 BizError
if err != nil {
    httpx.ErrorCtx(r.Context(), w, err)  // ← *errx.BizError (经拦截器还原)
}
```

路径 1 的 error 来自 go-zero `mapping` 包，是 `fmt.Errorf` / `errors.New` 产生的普通 error（如 `field "username" is not set`、`type mismatch for field "loginType"`），`errors.AsType[*errx.BizError]` 永远失败，导致所有参数错误返回 `500 SystemError`。

### httpx.Parse 可能产生的错误类型

| 来源 | 错误消息示例 | 底层类型 |
|------|------------|---------|
| `mapping.Unmarshal` | `field "username" is not set` | `fmt.Errorf` |
| `mapping.Unmarshal` | `type mismatch for field "loginType"` | `fmt.Errorf` |
| `mapping.Unmarshal` | `value "x" for field "type" is not defined in options "[1,2,3]"` | `fmt.Errorf` |
| `json.Decoder.Decode` | `invalid character '}' looking for beginning of value` | `*json.SyntaxError` |
| `json.Decoder.Decode` | `cannot unmarshal string into Go value of type int64` | `*json.UnmarshalTypeError` |
| `io.LimitReader` | `unexpected end of JSON input` | `*json.SyntaxError` |
| `validation.Validator` | 自定义校验消息 | `error` |

这些全部是客户端参数错误，应统一返回 **400 + ParamError**。

## 文件结构

| 操作 | 文件 | 职责 |
|------|------|------|
| 新建 | `pkg/errx/resolve_http.go` | `FromHTTPError()` 函数 |
| 新建 | `pkg/errx/resolve_http_test.go` | 单元测试 |
| 修改 | `app/gateway/gateway.go:35-46` | 错误处理回调增加 `FromHTTPError` fallback |

---

## Task 1: 实现 FromHTTPError (TDD)

**Files:**
- Create: `pkg/errx/resolve_http_test.go`
- Create: `pkg/errx/resolve_http.go`

### Step 1-4: FromHTTPError — RED → GREEN

- [ ] **Step 1: 写失败测试**

Create `pkg/errx/resolve_http_test.go`:

```go
package errx

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestFromHTTPError_Nil(t *testing.T) {
	if got := FromHTTPError(nil); got != nil {
		t.Errorf("FromHTTPError(nil) = %v, want nil", got)
	}
}

func TestFromHTTPError_AlreadyBizError(t *testing.T) {
	original := &BizError{Code: UserNotFound, Message: "用户不存在"}
	got := FromHTTPError(original)

	if got != original {
		t.Errorf("FromHTTPError(BizError) returned different pointer")
	}
}

func TestFromHTTPError_MappingErrors(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			"field not set",
			fmt.Errorf("field %q is not set", "username"),
			`field "username" is not set`,
		},
		{
			"type mismatch",
			fmt.Errorf("type mismatch for field %q", "loginType"),
			`type mismatch for field "loginType"`,
		},
		{
			"option validation",
			fmt.Errorf(`value "x" for field "type" is not defined in options "[1,2]"`),
			`value "x" for field "type" is not defined in options "[1,2]"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromHTTPError(tt.err)
			if got.Code != ParamError {
				t.Errorf("Code = %d, want %d", got.Code, ParamError)
			}
			if got.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", got.Message, tt.wantMsg)
			}
		})
	}
}

func TestFromHTTPError_JSONErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"SyntaxError", &json.SyntaxError{Offset: 1}},
		{"UnmarshalTypeError", &json.UnmarshalTypeError{Field: "id", Type: nil}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromHTTPError(tt.err)
			if got.Code != ParamError {
				t.Errorf("Code = %d, want %d", got.Code, ParamError)
			}
			if got.Message != tt.err.Error() {
				t.Errorf("Message = %q, want %q", got.Message, tt.err.Error())
			}
		})
	}
}

func TestFromHTTPError_WrappedBizError(t *testing.T) {
	inner := &BizError{Code: TokenExpired, Message: "Token已过期"}
	wrapped := fmt.Errorf("parse failed: %w", inner)

	got := FromHTTPError(wrapped)

	if got.Code != TokenExpired {
		t.Errorf("Code = %d, want %d", got.Code, TokenExpired)
	}
	if got.Message != "Token已过期" {
		t.Errorf("Message = %q, want %q", got.Message, "Token已过期")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd pkg/errx && go test -run TestFromHTTPError -v
```

Expected: FAIL — `FromHTTPError undefined`

- [ ] **Step 3: 实现 FromHTTPError**

Create `pkg/errx/resolve_http.go`:

```go
package errx

import "errors"

func FromHTTPError(err error) *BizError {
	if err == nil {
		return nil
	}

	if bizErr, ok := errors.AsType[*BizError](err); ok {
		return bizErr
	}

	return &BizError{
		Code:    ParamError,
		Message: err.Error(),
	}
}
```

> **设计决策**: 所有非 BizError 统一映射为 `ParamError`。理由：按项目约定，Logic 层**必须**返回 `errx.New(code, msg)`。如果到达 errorHandler 的 error 不是 BizError，那它只可能来自 `httpx.Parse`（参数解析/校验），语义上就是参数错误。如果 Logic 层违反约定泄漏了裸 error，那是 Logic 层的 bug，应该在 Logic 层修复，不应在 errorHandler 层兜底掩盖。

- [ ] **Step 4: 运行测试确认通过**

```bash
cd pkg/errx && go test -run TestFromHTTPError -v
```

Expected: PASS

- [ ] **Step 5: 运行完整 errx 测试套件**

```bash
cd pkg/errx && go test ./... -v -race
```

Expected: 全部 PASS（包括之前的 gRPC 相关测试）。

- [ ] **Step 6: 提交**

```bash
git -C "D:\Learning\projects\work projects\little-white-box" add pkg/errx/resolve_http.go pkg/errx/resolve_http_test.go
git -C "D:\Learning\projects\work projects\little-white-box" commit -m "feat(errx): add FromHTTPError to convert parse errors to BizError

Maps non-BizError errors (from httpx.Parse) to ParamError with
the original message preserved. Handles nil, already-BizError,
and wrapped-BizError cases.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## Task 2: 修改 Gateway 错误处理回调

**Files:**
- Modify: `app/gateway/gateway.go:35-46`

- [ ] **Step 1: 修改 SetErrorHandlerCtx 回调**

将 `app/gateway/gateway.go:35-46` 的 `httpx.SetErrorHandlerCtx` 回调替换为：

```go
	httpx.SetErrorHandlerCtx(func(ctx context.Context, err error) (int, any) {
		bizErr, ok := errors.AsType[*errx.BizError](err)
		if !ok {
			bizErr = errx.FromHTTPError(err)
		}
		return bizErr.HTTPStatus(), map[string]any{
			"code":    bizErr.Code,
			"message": bizErr.Message,
		}
	})
```

> **变更说明**: 去掉了 `else` 分支的硬编码 `500 SystemError` 兜底。现在所有错误都会产出 `*BizError`：Logic 层的 BizError 由 `errors.AsType` 提取，httpx.Parse 的普通 error 由 `FromHTTPError` 转换为 `ParamError`。响应格式化只有一条路径，消除了重复代码。

- [ ] **Step 2: 确认编译通过**

```bash
cd app/gateway && go build ./...
```

Expected: 无错误。

- [ ] **Step 3: 运行全项目测试**

```bash
cd "D:\Learning\projects\work projects\little-white-box" && go test ./pkg/errx/... -v -race -cover
```

Expected: 全部 PASS，errx 覆盖率 ≥ 80%。

- [ ] **Step 4: 提交**

```bash
git -C "D:\Learning\projects\work projects\little-white-box" add app/gateway/gateway.go
git -C "D:\Learning\projects\work projects\little-white-box" commit -m "fix(gateway): handle httpx.Parse errors as ParamError instead of SystemError

Non-BizError errors (from httpx.Parse validation/JSON parsing) now
return 400 with the original error message instead of 500 SystemError.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## 附录 A: 改造后错误流转

```
[httpx.Parse 失败]
error: `field "username" is not set`
  ↓
httpx.ErrorCtx(ctx, w, err)
  ↓
SetErrorHandlerCtx 回调:
  errors.AsType[*BizError] → false
  errx.FromHTTPError(err) → &BizError{Code: 2(ParamError), Message: `field "username" is not set`}
  HTTPStatus() → 400
  ↓
HTTP Response:
  Status: 400 Bad Request
  Body: {"code": 2, "message": "field \"username\" is not set"}  ✅ (改造前是 500)


[Logic 层返回 BizError]
error: &BizError{Code: 1001, Message: "用户不存在"}
  ↓
httpx.ErrorCtx(ctx, w, err)
  ↓
SetErrorHandlerCtx 回调:
  errors.AsType[*BizError] → true, bizErr = &BizError{1001, "用户不存在"}
  HTTPStatus() → 404
  ↓
HTTP Response:
  Status: 404 Not Found
  Body: {"code": 1001, "message": "用户不存在"}  ✅ (行为不变)


[RPC 返回的 BizError (经拦截器还原)]
error: &BizError{Code: 1003, Message: "密码错误"}  ← 由 BizErrorUnaryInterceptor 还原
  ↓
httpx.ErrorCtx(ctx, w, err)
  ↓
SetErrorHandlerCtx 回调:
  errors.AsType[*BizError] → true, bizErr = &BizError{1003, "密码错误"}
  HTTPStatus() → 401
  ↓
HTTP Response:
  Status: 401 Unauthorized
  Body: {"code": 1003, "message": "密码错误"}  ✅ (行为不变)
```

## 附录 B: 改造前后对比

| 错误来源 | 改造前 HTTP Status | 改造前 Body | 改造后 HTTP Status | 改造后 Body |
|---------|------------------|-------------|------------------|-------------|
| `httpx.Parse` 字段缺失 | 500 | `{"code":3,"message":"系统错误"}` | **400** | `{"code":2,"message":"field \"username\" is not set"}` |
| `httpx.Parse` 类型不匹配 | 500 | `{"code":3,"message":"系统错误"}` | **400** | `{"code":2,"message":"type mismatch for field \"loginType\""}` |
| `httpx.Parse` JSON 格式错误 | 500 | `{"code":3,"message":"系统错误"}` | **400** | `{"code":2,"message":"invalid character..."}` |
| Logic 层 BizError | 4xx/5xx | `{"code":N,"message":"..."}` | 4xx/5xx | `{"code":N,"message":"..."}` (不变) |
| RPC BizError (经拦截器) | 4xx/5xx | `{"code":N,"message":"..."}` | 4xx/5xx | `{"code":N,"message":"..."}` (不变) |
