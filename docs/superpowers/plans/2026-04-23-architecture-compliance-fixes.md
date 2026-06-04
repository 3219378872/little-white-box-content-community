# 架构规范修复计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans.

**Goal:** 移除 Gateway Handler 层 JWT 解析逻辑，统一 User 模块错误包装，补齐输入校验。

**Architecture:** Handler 层只负责参数绑定和调用 Logic；所有错误通过 `errx` 包装；输入校验在 Gateway Logic 入口统一执行。

**Tech Stack:** Go 1.26.1, go-zero

---

## 文件变更总览

| 文件 | 变更 |
|------|------|
| `app/gateway/internal/handler/user/get_user_favorites_handler.go` | 删除 `tryInjectUserId` |
| `app/user/internal/logic/login_logic.go` | 包装 Redis 错误 |
| `app/user/internal/logic/register_logic.go` | 检查并修复裸透传 |
| `app/user/internal/logic/send_verify_code_logic.go` | 检查并修复裸透传 |
| `pkg/validator/validator.go` | 新增校验函数 |
| `app/gateway/internal/logic/login/login_logic.go` | 增加校验 |
| `app/gateway/internal/logic/login/register_logic.go` | 增加校验 |

---

### Task 1: 移除 Gateway Handler 层 JWT 解析

**Files:**
- Modify: `app/gateway/internal/handler/user/get_user_favorites_handler.go`

- [ ] **Step 1: 删除 `tryInjectUserId` 函数，简化 Handler**

```go
func GetUserFavoritesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetUserFavoritesReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := user.NewGetUserFavoritesLogic(r.Context(), svcCtx)
		resp, err := l.GetUserFavorites(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
```

删除 `tryInjectUserId` 及其所有 import（`jwtx`、`json`、`strconv`、`context` 中若仅用于此函数则删除）。

- [ ] **Step 2: 确认 `GetUserFavoritesLogic` 兼容中间件注入的 key**

当前 `GetUserFavoritesLogic` 已使用：
```go
requesterID, _ := jwtx.GetUserIdFromContext(l.ctx)
```

H3 修复后，中间件和 jwtx 的 key 体系统一，此代码可正确读取 `OptionalAuthMiddleware` 注入的 userId。

- [ ] **Step 3: Commit**

```bash
git add app/gateway/internal/handler/user/get_user_favorites_handler.go
git commit -m "refactor(gateway): remove JWT parsing from handler layer

- Delete tryInjectUserId; rely on OptionalAuthMiddleware
- Handler now only binds params and delegates to logic

Fixes H4"
```

---

### Task 2: 修复 User 模块裸错误透传

**Files:**
- Modify: `app/user/internal/logic/login_logic.go`
- Modify: `app/user/internal/logic/register_logic.go`
- Modify: `app/user/internal/logic/send_verify_code_logic.go`

- [ ] **Step 1: 修改 `login_logic.go` 中的 Redis 错误**

将以下两段代码：
```go
verifyCode, err := l.svcCtx.RedisClient.GetCtx(l.ctx, in.Phone)
if err != nil {
	return nil, err
}
```
和：
```go
_, err = l.svcCtx.RedisClient.DelCtx(l.ctx, in.Phone)
if err != nil {
	return nil, err
}
```

均改为：
```go
verifyCode, err := l.svcCtx.RedisClient.GetCtx(l.ctx, in.Phone)
if err != nil {
	l.Errorw("Redis.GetCtx failed", logx.Field("phone", in.Phone), logx.Field("err", err.Error()))
	return nil, errx.Wrap(err, errx.SystemError)
}
```
```go
_, err = l.svcCtx.RedisClient.DelCtx(l.ctx, in.Phone)
if err != nil {
	l.Errorw("Redis.DelCtx failed", logx.Field("phone", in.Phone), logx.Field("err", err.Error()))
	return nil, errx.Wrap(err, errx.SystemError)
}
```

- [ ] **Step 2: 检查并修复 `register_logic.go` 和 `send_verify_code_logic.go`**

搜索文件中是否存在 `return nil, err` 的裸透传模式（Redis 错误、RPC 错误等），统一替换为 `errx.Wrap(err, errx.SystemError)` 或 `errx.NewWithCode(errx.SystemError)`。

示例搜索命令：
```bash
grep -n "return nil, err" app/user/internal/logic/register_logic.go app/user/internal/logic/send_verify_code_logic.go
```

对每一处的 `err` 来源进行判断：
- 若来自 Redis / 数据库 / 外部 RPC → 用 `errx.Wrap`
- 若已是 `errx` 错误 → 可直接透传

- [ ] **Step 3: Commit**

```bash
git add app/user/internal/logic/
git commit -m "fix(user): wrap all Redis and external errors with errx

- Prevent internal error leakage to clients
- Ensure every error path returns a BizError with proper code

Fixes H2"
```

---

### Task 3: 补齐输入校验

**Files:**
- Modify: `pkg/validator/validator.go`
- Modify: `app/gateway/internal/logic/login/login_logic.go`
- Modify: `app/gateway/internal/logic/login/register_logic.go`

- [ ] **Step 1: 在 `pkg/validator/validator.go` 新增校验函数**

```go
package validator

import "regexp"

var (
	phoneRegex    = regexp.MustCompile(`^1[3-9]\d{9}$`)
	usernameRegex = regexp.MustCompile(`^[\w一-龥]{2,32}$`)
)

func ValidatePhone(phone string) bool {
	return phoneRegex.MatchString(phone)
}

func ValidateUsername(username string) bool {
	return usernameRegex.MatchString(username)
}

func ValidatePassword(password string) bool {
	if len(password) < 8 || len(password) > 32 {
		return false
	}
	var hasLetter, hasDigit bool
	for _, ch := range password {
		switch {
		case ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z':
			hasLetter = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		}
	}
	return hasLetter && hasDigit
}
```

- [ ] **Step 2: 在 Gateway Login/Register Logic 中调用校验**

在 `app/gateway/internal/logic/login/login_logic.go` 中：

```go
func (l *LoginLogic) Login(req *types.LoginReq) (*types.LoginResp, error) {
	if req.LoginType == 2 {
		if !validator.ValidatePhone(req.Phone) {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		if req.VerifyCode == "" {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	} else {
		if req.Username == "" || req.Password == "" {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	}
	// ... existing logic ...
}
```

在 `app/gateway/internal/logic/login/register_logic.go` 中类似添加校验。

- [ ] **Step 3: Commit**

```bash
git add pkg/validator/validator.go app/gateway/internal/logic/login/
git commit -m "feat(validator): add phone/username/password validation

- Enforce input validation at Gateway entry points

Fixes H6"
```

---

## 验证清单

- [ ] `go test ./app/gateway/... ./app/user/... -race -cover` 通过
- [ ] `go vet ./...`
