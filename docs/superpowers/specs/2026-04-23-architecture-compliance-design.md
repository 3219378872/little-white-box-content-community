# 架构规范修复设计

**日期**: 2026-04-23
**范围**: app/gateway, app/user
**关联审查项**: H4 (三层架构违反), H2 (裸错误透传), H6 (输入校验缺失)

---

## 问题描述

### 三层架构违反 (Gateway)

`app/gateway/internal/handler/user/get_user_favorites_handler.go` 中实现了 `tryInjectUserId` 函数：

```go
func tryInjectUserId(ctx context.Context, r *http.Request, svcCtx *svc.ServiceContext) context.Context {
    auth := r.Header.Get("Authorization")
    claims, err := jwtx.ParseToken(auth, cfg)
    return context.WithValue(ctx, "userId", json.Number(...))
}
```

问题：
1. Handler 层直接解析 JWT，这是中间件层的职责
2. 使用裸字符串 `"userId"` 作为 context key，与 `jwtx` 包的强类型 key 不兼容
3. 绕过 `OptionalAuthMiddleware`，导致同一项目内存在两套认证注入逻辑
4. `json.Number` 类型与 `jwtx.GetOptionalUserIdFromContext` 期望的 `json.Number` 恰好兼容，但 context key 类型不匹配会导致 `GetUserFavoritesLogic` 中的 `jwtx.GetUserIdFromContext` 返回零值

### 裸错误透传 (User)

`app/user/internal/logic/login_logic.go` 中：

```go
verifyCode, err := l.svcCtx.RedisClient.GetCtx(l.ctx, in.Phone)
if err != nil {
    return nil, err  // 裸错误透传
}
```

Redis 错误（如连接超时、键不存在）直接返回给调用方，未包装为业务错误码。

### 输入校验缺失 (User / Gateway)

- 手机号格式未校验（仅依赖数据库查询）
- 批量查询 ID 无上限（已在查询性能设计中覆盖）
- 密码强度未校验

## 目标

1. 移除 Handler 层的 JWT 解析，统一走 `OptionalAuthMiddleware`
2. User 模块 Logic 层所有错误统一用 `errx` 包装
3. 补齐核心输入校验（手机号、密码）

## 方案

### 1. 移除 tryInjectUserId，统一 OptionalAuthMiddleware

**步骤**：
1. 在 Gateway 路由配置中确认 `/user/favorites` 路由已挂载 `OptionalAuthMiddleware`
2. 删除 `get_user_favorites_handler.go` 中的 `tryInjectUserId` 函数
3. Handler 直接传递 `r.Context()`（中间件已注入 userId）
4. `GetUserFavoritesLogic` 中使用 `jwtx.GetOptionalUserIdFromContext` 获取 userId（已是当前实现，但之前因 key 类型不匹配获取不到）

由于 H3 (Context key 统一) 修复后，`jwtx` 与 `middleware` 的 key 体系统一，中间件注入的 userId 可被 `jwtx.GetOptionalUserIdFromContext` 正确读取。

### 2. 裸错误包装

在 `login_logic.go` 中：

```go
// 修改前
verifyCode, err := l.svcCtx.RedisClient.GetCtx(l.ctx, in.Phone)
if err != nil {
    return nil, err
}

// 修改后
verifyCode, err := l.svcCtx.RedisClient.GetCtx(l.ctx, in.Phone)
if err != nil {
    l.Errorw("Redis.GetCtx failed", logx.Field("phone", in.Phone), logx.Field("err", err.Error()))
    return nil, errx.Wrap(err, errx.SystemError)
}
```

同理处理 `DelCtx` 错误。检查 `register_logic.go`、`send_verify_code_logic.go` 中是否存在类似裸透传。

### 3. 输入校验补齐

在 `pkg/validator` 中新增/确认存在以下函数：

```go
func ValidatePhone(phone string) bool   // 中国大陆手机号正则: ^1[3-9]\d{9}$
func ValidatePassword(password string) bool // 长度 8-32，至少包含字母+数字
func ValidateUsername(username string) bool  // 长度 2-32，字母数字下划线中文
```

在 Gateway Handler 层（或 Logic 层入口）统一调用：

```go
if !validator.ValidatePhone(req.Phone) {
    return nil, errx.NewWithCode(errx.ParamError)
}
```

## 文件变更

| 文件 | 变更 |
|------|------|
| `app/gateway/internal/handler/user/get_user_favorites_handler.go` | 删除 `tryInjectUserId`，直接使用 `r.Context()` |
| `app/user/internal/logic/login_logic.go` | Redis 错误用 `errx.Wrap` 包装 |
| `app/user/internal/logic/register_logic.go` | 检查并修复裸错误透传 |
| `app/user/internal/logic/send_verify_code_logic.go` | 检查并修复裸错误透传 |
| `pkg/validator/validator.go` | 新增/完善校验函数 |
| `app/gateway/internal/logic/login/register_logic.go` | 增加输入校验调用 |
| `app/gateway/internal/logic/login/login_logic.go` | 增加输入校验调用 |

## 验收标准

- `get_user_favorites_handler.go` 中无 JWT 解析逻辑
- 未登录用户访问收藏列表时 `requesterID = 0`，不 panic
- `go test ./app/user/...` 通过
- 非法手机号请求返回 `ParamError`
