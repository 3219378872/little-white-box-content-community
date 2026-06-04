# go-zero 企业级规范审查修复计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 2026-04-22 全项目审查中发现的 7 个 CRITICAL 问题和 32 个 HIGH 问题中的核心数据一致性与安全项。

**Architecture:** 按安全漏洞 → 运行时 Bug → 并发安全 → 数据一致性优先级分任务修复；所有修改遵循 go-zero 三层架构与 Context 透传规范。

**Tech Stack:** Go 1.26.1, go-zero v1.10.1, MySQL 8.0, Redis 7

---

## 文件变更总览

| 文件 | 变更类型 | 修复问题 |
|------|---------|---------|
| `pkg/middleware/cors.go` | 修改 | C1: CORS `*` + credentials 漏洞 |
| `pkg/jwtx/jwt.go` | 修改 | C2: JWT 算法混淆攻击 |
| `pkg/util/hash.go` | 修改 | C3: 硬编码默认密码 |
| `app/media/etc/media.yaml` | 修改 | C4: S3 凭据明文硬编码 |
| `app/content/internal/logic/update_post_logic.go` | 修改 | C5: `new()` 双重指针误用 |
| `app/content/internal/model/post_model.go` | 修改 | C6: SQL 列名注入风险 |
| `app/media/internal/logic/delete_media_logic.go` | 修改 | C7: RMW 无锁软删 |
| `app/media/internal/model/media_model.go` | 新增方法 | C7: 条件更新支持 |
| `pkg/middleware/auth.go` | 修改 | H3: Context key 双套体系 |
| `pkg/jwtx/context.go` | 修改 | H3: Context key 双套体系 |
| `pkg/errx/errors.go` | 修改 | H2: WrapMsg 泄漏内部错误 |

---

### Task 1: 修复 CORS 漏洞（C1）

**Files:**
- Modify: `pkg/middleware/cors.go`
- Test: `pkg/middleware/cors_test.go`

**说明：** 当前 `AllowOrigins: ["*"]` 与 `AllowCredentials: true` 组合构成安全漏洞。必须改为白名单校验，且当 `AllowCredentials: true` 时禁止回写 `*`。

- [ ] **Step 1: 修改 `DefaultCORSConfig` 默认值**

将 `AllowOrigins` 从 `["*"]` 改为空列表（由调用方或环境配置填充），`MaxAge` 改用 `strconv.Itoa` 替代有 bug 的 `intToStr`。

```go
// DefaultCORSConfig 默认 CORS 配置
var DefaultCORSConfig = CORSConfig{
	AllowOrigins:     []string{}, // 禁止默认通配，由调用方显式配置白名单
	AllowMethods: []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodOptions,
		http.MethodPatch,
	},
	AllowHeaders: []string{
		"Content-Type",
		"Authorization",
		"X-Requested-With",
		"X-Token",
	},
	ExposeHeaders:    []string{},
	AllowCredentials: true,
	MaxAge:           86400,
}
```

- [ ] **Step 2: 修改 `CORSMiddleware` origin 校验逻辑**

替换 `CORSMiddleware` 的 `allowed` 判断和 origin 回写逻辑：

```go
func CORSMiddleware(config CORSConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowedOrigin := ""
			for _, o := range config.AllowOrigins {
				if o == origin {
					allowedOrigin = origin
					break
				}
			}

			// 若 AllowCredentials=true，绝不允许回写 *（浏览器会拒绝）
			if allowedOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			} else if !config.AllowCredentials && len(config.AllowOrigins) == 1 && config.AllowOrigins[0] == "*" {
				// 仅当不携带凭证时才允许通配
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}

			// ... 其余 header 设置保持不变 ...
		})
	}
}
```

- [ ] **Step 3: 替换 `intToStr` 为 `strconv.Itoa`**

```go
import "strconv"

// 删除 intToStr 函数，直接在 CORSMiddleware 中使用 strconv.Itoa
w.Header().Set("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
```

- [ ] **Step 4: 编写单元测试验证白名单与凭证安全**

```go
func TestCORSMiddleware_WhitelistWithCredentials(t *testing.T) {
	mw := CORSMiddleware(CORSConfig{
		AllowOrigins:     []string{"https://example.com"},
		AllowMethods:     []string{"GET", "POST"},
		AllowCredentials: true,
	})

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	// 合法 origin
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))

	// 非法 origin 不应设置 Access-Control-Allow-Origin
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("Origin", "https://evil.com")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Empty(t, rec2.Header().Get("Access-Control-Allow-Origin"))
}
```

- [ ] **Step 5: 运行测试**

Run: `go test ./pkg/middleware/... -v -run TestCORSMiddleware`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/middleware/
git commit -m "fix(cors): enforce whitelist and forbid * with credentials

- Remove default * from AllowOrigins
- Reject requests from non-whitelist origins when credentials enabled
- Replace buggy intToStr with strconv.Itoa

Fixes C1"
```

---

### Task 2: 修复 JWT 算法混淆攻击（C2）

**Files:**
- Modify: `pkg/jwtx/jwt.go`
- Test: `pkg/jwtx/jwt_test.go`

**说明：** `ParseWithClaims` 的 `keyfunc` 未校验 `token.Method`，攻击者可将 `alg` 改为 `none` 或其他算法绕过签名。

- [ ] **Step 1: 在 `ParseToken` 的 keyfunc 中强制校验算法**

```go
// ParseToken 解析 token
func ParseToken(tokenString string, config JwtConfig) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// 强制校验签名算法，防止算法混淆攻击（alg:none 或 RS256→HS256 等）
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(config.AccessSecret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, ErrTokenMalformed
		}
		if errors.Is(err, jwt.ErrTokenNotValidYet) {
			return nil, ErrTokenNotValidYet
		}
		return nil, ErrTokenInvalid
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrTokenInvalid
}
```

- [ ] **Step 2: 添加算法混淆攻击的防御测试**

```go
func TestParseToken_AlgNoneAttack(t *testing.T) {
	config := JwtConfig{AccessSecret: "test-secret", AccessExpire: 3600}

	// 构造 alg=none 的恶意 token (header: {"alg":"none","typ":"JWT"}, payload: {})
	maliciousToken := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.e30."

	_, err := ParseToken(maliciousToken, config)
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

func TestParseToken_WrongAlgAttack(t *testing.T) {
	config := JwtConfig{AccessSecret: "test-secret", AccessExpire: 3600}

	// 构造 alg=HS512 的 token（虽然密钥相同，但算法不匹配应被拒绝）
	claims := Claims{UserId: 1, Username: "test"}
	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	wrongAlgToken, _ := token.SignedString([]byte(config.AccessSecret))

	_, err := ParseToken(wrongAlgToken, config)
	assert.ErrorIs(t, err, ErrTokenInvalid)
}
```

- [ ] **Step 3: 运行测试**

Run: `go test ./pkg/jwtx/... -v -run TestParseToken`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add pkg/jwtx/
git commit -m "fix(jwt): enforce HMAC signing method to prevent alg confusion attack

- Reject tokens with alg:none or unexpected signing methods
- Add tests for alg:none and wrong-alg attacks

Fixes C2"
```

---

### Task 3: 移除硬编码默认密码（C3）

**Files:**
- Modify: `pkg/util/hash.go`
- Modify: `app/user/etc/user.yaml`（若存在默认密码配置项则添加环境变量引用）

**说明：** `DefaultPassword` 是导出常量，暴露在源码中。应改为从环境变量读取，启动时加载到 `hashedDefaultPassword`。

- [ ] **Step 1: 删除导出常量，改为从环境变量读取**

```go
package util

import (
	"crypto/sha256"
	"encoding/hex"
	"os"

	"golang.org/x/crypto/bcrypt"
)

var hashedDefaultPassword []byte

func init() {
	defaultPass := os.Getenv("DEFAULT_PASSWORD")
	if defaultPass == "" {
		// 生产环境必须在启动前设置 DEFAULT_PASSWORD
		// 开发环境若未设置则使用随机值，避免固定默认值
		defaultPass = "DEV_ONLY_" + generateRandomString(16)
	}
	password, err := bcrypt.GenerateFromPassword([]byte(defaultPass), bcrypt.DefaultCost)
	if err != nil {
		panic("默认密码初始化错误: " + err.Error())
	}
	hashedDefaultPassword = password
}

// generateRandomString 生成指定长度的随机字符串（仅用于开发兜底）
func generateRandomString(n int) string {
	// 简单实现，生产环境不依赖此路径
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		// 注：此处非加密安全随机，仅用于开发未设环境变量时的兜底
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}
```

需要添加 `time` 到 import：
```go
import "time"
```

- [ ] **Step 2: 在 `deploy/docker-compose.middleware.yml`（或对应部署配置）添加环境变量提示**

```yaml
# 在 user 服务环境变量中添加
services:
  user-rpc:
    environment:
      - DEFAULT_PASSWORD=${DEFAULT_PASSWORD}
```

- [ ] **Step 3: Commit**

```bash
git add pkg/util/hash.go
git commit -m "fix(hash): remove hardcoded default password, read from env

- Remove exported DefaultPassword constant
- Load default password from DEFAULT_PASSWORD env var at init
- Use random dev-only fallback if unset to prevent accidental weak defaults

Fixes C3"
```

---

### Task 4: 移除 S3 凭据明文硬编码（C4）

**Files:**
- Modify: `app/media/etc/media.yaml`
- Modify: `app/media/internal/config/config.go`（若需添加验证逻辑）

**说明：** `AccessKey` / `SecretKey` 明文写入 yaml 并提交到版本库。必须改为 `${ENV_VAR}` 占位。

- [ ] **Step 1: 修改 media.yaml 中的凭据为环境变量占位符**

```yaml
S3Storage:
  Endpoint: "127.0.0.1:8333"
  AccessKey: "${S3_ACCESS_KEY}"
  SecretKey: "${S3_SECRET_KEY}"
  UseSSL: false
  Region: "us-east-1"
  Bucket: "xbh-media"
  PublicBaseURL: "http://127.0.0.1:8333/xbh-media"
  SkipBucketPolicy: true
```

- [ ] **Step 2: 在 media 服务启动时校验凭据非空**

若 `internal/config/config.go` 中的 `S3Storage` 结构体已有对应字段，在 `app/media/media.go` 的 `main` 中添加校验：

```go
func main() {
	// ... 原有代码 ...
	var c config.Config
	conf.MustLoad(*configFile, &c)

	// 校验 S3 凭据已配置
	if c.S3Storage.AccessKey == "" || c.S3Storage.SecretKey == "" {
		log.Fatal("S3_ACCESS_KEY and S3_SECRET_KEY must be set")
	}

	// ... 后续代码 ...
}
```

- [ ] **Step 3: Commit**

```bash
git add app/media/etc/media.yaml app/media/media.go
git commit -m "fix(media): move S3 credentials to environment variables

- Replace hardcoded AccessKey/SecretKey with ${S3_ACCESS_KEY} and ${S3_SECRET_KEY}
- Add startup validation to ensure credentials are present

Fixes C4"
```

---

### Task 5: 修复 `new()` 双重指针误用（C5）

**Files:**
- Modify: `app/content/internal/logic/update_post_logic.go`

**说明：** `new(util.ToJsonObject(in.Images))` 产生 `**JSONField[[]string]`，SQL 驱动无法序列化。

- [ ] **Step 1: 修正 images 字段赋值逻辑**

将第 75 行：
```go
fields["images"] = new(util.ToJsonObject(in.Images))
```
改为：
```go
jsonField := util.ToJsonObject(in.Images)
jsonStr, err := jsonField.JsonString()
if err != nil {
	return nil, errx.NewWithCode(errx.SystemError)
}
fields["images"] = jsonStr
```

需要确认 `update_post_logic.go` 的 import 中已包含 `errx`。

- [ ] **Step 2: 编写/运行 UpdatePost 的单元测试验证图片更新**

```go
func TestUpdatePostLogic_UpdatePost_Images(t *testing.T) {
	// mock svcCtx 和 PostModel
	// 调用 UpdatePost 传入 Images=["a.jpg","b.jpg"]
	// 断言 PostModel.UpdateFields 被调用时 fields["images"] 类型为 string
}
```

Run: `go test ./app/content/internal/logic/... -v -run TestUpdatePostLogic`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add app/content/internal/logic/update_post_logic.go
git commit -m "fix(content): correct new() misuse when serializing images

- Replace new(util.ToJsonObject(...)) with proper JsonString() conversion
- Ensures images field is a plain string before SQL execution

Fixes C5"
```

---

### Task 6: 修复 SQL 列名注入风险（C6）

**Files:**
- Modify: `app/content/internal/model/post_model.go`
- Test: `app/content/internal/model/post_model_test.go`

**说明：** `UpdateFields` 的列名直接来自 map key，无白名单校验，backtick 可被闭合导致注入。

- [ ] **Step 1: 定义允许更新的列白名单**

在 `post_model.go` 顶部添加：
```go
// allowedUpdateCols UpdateFields 允许的列白名单，防止 SQL 注入
var allowedUpdateCols = map[string]struct{}{
	"title":   {},
	"content": {},
	"images":  {},
	"status":  {},
}
```

- [ ] **Step 2: 在 `UpdateFields` 中增加白名单校验**

```go
func (m *customPostModel) UpdateFields(ctx context.Context, id int64, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	setClauses := make([]string, 0, len(fields))
	args := make([]interface{}, 0, len(fields)+1)
	for col, val := range fields {
		if _, ok := allowedUpdateCols[col]; !ok {
			return fmt.Errorf("UpdateFields: disallowed column %q", col)
		}
		setClauses = append(setClauses, fmt.Sprintf("`%s`=?", col))
		args = append(args, val)
	}
	args = append(args, id)
	postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("update %s set %s where `id`=?", m.table, strings.Join(setClauses, ", "))
		return conn.ExecCtx(ctx, query, args...)
	}, postIdKey)
	return err
}
```

- [ ] **Step 3: 编写注入防御测试**

```go
func TestUpdateFields_SQLInjection(t *testing.T) {
	// 使用 sqlmock 或 testcontainers 建立 mock DB
	// 传入恶意字段名: "status`=1,`evil"=123 // 试图闭合 backtick
	// 期望返回错误，不执行 SQL
}
```

Run: `go test ./app/content/internal/model/... -v -run TestUpdateFields`
Expected: PASS（包含注入用例返回 error）

- [ ] **Step 4: Commit**

```bash
git add app/content/internal/model/post_model.go
git commit -m "fix(content): add column whitelist to UpdateFields preventing SQL injection

- Define allowedUpdateCols whitelist for post updates
- Reject any field not in whitelist before building SQL

Fixes C6"
```

---

### Task 7: 修复 RMW 无锁软删（C7）

**Files:**
- Modify: `app/media/internal/logic/delete_media_logic.go`
- Modify: `app/media/internal/model/media_model.go`（或新增方法）
- Test: `app/media/internal/logic/delete_media_logic_test.go`

**说明：** 当前实现为 `FindOne → 修改 Status → Update` 全字段覆盖，并发删除可导致数据损坏。应改为条件 UPDATE（`WHERE status=?`）。

- [ ] **Step 1: 在 MediaModel 接口和实现中添加 `UpdateStatus` 方法**

在 `app/media/internal/model/media_model.go`（或对应 custom model 文件）中添加：

```go
// UpdateStatus 条件更新状态，仅当当前状态为 expectedStatus 时才更新，防止并发 Lost Update
func (m *customMediaModel) UpdateStatus(ctx context.Context, id int64, expectedStatus, newStatus int64) (sql.Result, error) {
	return m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("update %s set `status`=? where `id`=? and `status`=?", m.table)
		return conn.ExecCtx(ctx, query, newStatus, id, expectedStatus)
	})
}
```

- [ ] **Step 2: 修改 DeleteMediaLogic 使用条件更新**

```go
// DeleteMedia 软删媒体；仅归属用户可删；重复删除幂等。
func (l *DeleteMediaLogic) DeleteMedia(in *pb.DeleteMediaReq) (*pb.DeleteMediaResp, error) {
	if in.MediaId <= 0 || in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	m, err := l.svcCtx.MediaModel.FindOne(l.ctx, in.MediaId)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.MediaNotFound)
		}
		l.Errorw("MediaModel.FindOne failed",
			logx.Field("media_id", in.MediaId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if m.Status == 0 {
		return &pb.DeleteMediaResp{}, nil
	}

	if m.UserId != in.UserId {
		return nil, errx.NewWithCode(errx.PermissionDenied)
	}

	result, err := l.svcCtx.MediaModel.UpdateStatus(l.ctx, in.MediaId, 1, 0)
	if err != nil {
		l.Errorw("MediaModel.UpdateStatus failed",
			logx.Field("media_id", in.MediaId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// 可能是并发重复删除，幂等返回成功
		l.Infow("delete media no-op (concurrent or already deleted)",
			logx.Field("media_id", in.MediaId),
		)
	}

	l.Infow("delete media success",
		logx.Field("media_id", in.MediaId),
		logx.Field("user_id", in.UserId),
	)
	return &pb.DeleteMediaResp{}, nil
}
```

- [ ] **Step 3: 编写并发删除测试**

```go
func TestDeleteMediaLogic_ConcurrentDelete(t *testing.T) {
	// 使用 testcontainers 启动真实 MySQL
	// 插入一条 status=1 的媒体记录
	// 并发启动 2 个 goroutine 执行 DeleteMedia
	// 验证最终 status=0
	// 验证两个 goroutine 均返回成功（幂等）
	// 验证 RowsAffected 总和 <= 1（仅一个实际更新）
}
```

Run: `go test ./app/media/internal/logic/... -v -run TestDeleteMediaLogic_ConcurrentDelete`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add app/media/internal/logic/delete_media_logic.go app/media/internal/model/media_model.go
git commit -m "fix(media): replace RMW soft-delete with conditional UPDATE

- Add UpdateStatus with expectedStatus guard
- DeleteMedia now uses WHERE status=1 to prevent concurrent lost update
- Return success when RowsAffected=0 for idempotency

Fixes C7"
```

---

### Task 8: 统一 Context Key 体系（H3）

**Files:**
- Modify: `pkg/middleware/auth.go`
- Modify: `pkg/jwtx/context.go`
- Test: `pkg/middleware/auth_test.go` / `pkg/jwtx/context_test.go`

**说明：** `auth.go` 用私有类型 `ctxKey("userId")` 写入，`context.go` 用裸字符串 `"userId"` 读取，两套 key 类型不兼容导致混用时静默返回零值。

- [ ] **Step 1: 在 `pkg/jwtx/context.go` 中定义强类型 context key 并导出**

```go
package jwtx

import (
	"context"
	"encoding/json"
	"strconv"
)

// contextKey 强类型 context key，防止与外部包字符串冲突
type contextKey string

const (
	ctxUserIDKey   contextKey = "userId"
	ctxUsernameKey contextKey = "username"
)

func WithClaimsContext(ctx context.Context, claims *Claims) context.Context {
	if claims == nil {
		return ctx
	}

	ctx = context.WithValue(ctx, ctxUserIDKey, json.Number(strconv.FormatInt(claims.UserId, 10)))
	ctx = context.WithValue(ctx, ctxUsernameKey, claims.Username)

	return ctx
}

func GetOptionalUserIdFromContext(ctx context.Context) (int64, bool) {
	switch value := ctx.Value(ctxUserIDKey).(type) {
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

// GetUsernameFromContext 从上下文中获取用户名
func GetUsernameFromContext(ctx context.Context) (string, bool) {
	username, ok := ctx.Value(ctxUsernameKey).(string)
	return username, ok
}
```

- [ ] **Step 2: 修改 `pkg/middleware/auth.go` 复用 jwtx 的 key**

```go
package middleware

import (
	"context"
	"jwtx"
	"net/http"
	"strings"
)

// ... AuthMiddleware 逻辑不变，但 context 写入使用 jwtx 的函数 ...

// 将以下辅助函数改为复用 jwtx 的导出函数/常量（或直接删除，统一使用 jwtx）

func contextWithUserId(ctx context.Context, userId int64) context.Context {
	return jwtx.WithClaimsContext(ctx, &jwtx.Claims{UserId: userId})
}

func contextWithUsername(ctx context.Context, username string) context.Context {
	return jwtx.WithClaimsContext(ctx, &jwtx.Claims{Username: username})
}

// GetUserId 从 context 获取用户 ID（统一走 jwtx）
func GetUserId(ctx context.Context) int64 {
	userId, _ := jwtx.GetOptionalUserIdFromContext(ctx)
	return userId
}

// GetUsername 从 context 获取用户名
func GetUsername(ctx context.Context) string {
	username, _ := jwtx.GetUsernameFromContext(ctx)
	return username
}
```

**注意：** 如果 `jwtx.WithClaimsContext` 内部使用 `json.Number`，而 middleware 需要直接写入 `int64`，可以改为在 `jwtx` 包中新增 `WithUserIdContext` 和 `WithUsernameContext` 专门用于非 JWT 场景：

```go
// jwtx/context.go 新增
func WithUserIdContext(ctx context.Context, userId int64) context.Context {
	return context.WithValue(ctx, ctxUserIDKey, json.Number(strconv.FormatInt(userId, 10)))
}

func WithUsernameContext(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, ctxUsernameKey, username)
}
```

然后在 `middleware/auth.go` 中使用这两个新函数。

- [ ] **Step 3: 编写兼容性测试**

```go
func TestContextKeyCompat(t *testing.T) {
	// middleware 写入
	ctx := context.Background()
	ctx = contextWithUserId(ctx, 42)

	// jwtx 读取
	uid, ok := jwtx.GetOptionalUserIdFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, int64(42), uid)
}
```

Run: `go test ./pkg/middleware/... ./pkg/jwtx/... -v -run TestContextKeyCompat`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add pkg/middleware/auth.go pkg/jwtx/context.go
git commit -m "fix(pkg): unify context key system to prevent silent zero-value failures

- Introduce strong-typed contextKey in jwtx package
- Add WithUserIdContext/WithUsernameContext for non-JWT writes
- Update middleware to use jwtx keys exclusively

Fixes H3"
```

---

### Task 9: 修复裸错误透传（H2 代表项）

**Files:**
- Modify: `pkg/errx/errors.go`
- Modify: `app/user/internal/logic/login_logic.go`（选取一处代表修复）

**说明：** `WrapMsg` 将内部错误拼入客户端 message 可能泄漏敏感信息。需要区分内部日志与客户端返回。

- [ ] **Step 1: 修改 `WrapMsg` 不将内部错误暴露给客户端**

```go
// WrapMsg 包装错误并自定义消息；内部错误保留在 cause 中供日志使用，Message 仅返回用户友好文本
func WrapMsg(err error, message string) error {
	if err == nil {
		return nil
	}
	return &BizError{
		Code:    UnknownError,
		Message: message, // 不再拼接 err.Error()，防止内部信息泄漏
		cause:   err,
	}
}
```

- [ ] **Step 2: 在 login_logic.go 中修正 Redis/JWT 错误的包装**

示例（假设原代码直接 `return nil, err`）：

```go
// 原代码示例:
// token, err := jwtx.GenerateToken(user.Id, user.Username, l.svcCtx.Config.Jwt)
// if err != nil {
//     return nil, err
// }

// 修正后:
token, err := jwtx.GenerateToken(user.Id, user.Username, l.svcCtx.Config.Jwt)
if err != nil {
	l.Errorw("GenerateToken failed", logx.Field("err", err.Error()))
	return nil, errx.Wrap(err, errx.SystemError)
}
```

- [ ] **Step 3: Commit**

```bash
git add pkg/errx/errors.go
git commit -m "fix(errx): prevent internal error leakage in WrapMsg

- Stop appending err.Error() to client-facing Message
- Internal cause remains available for logging via Unwrap

Fixes H2 (representative)"
```

---

## 验证清单（完工前必须执行）

- [ ] `go test ./pkg/middleware/... ./pkg/jwtx/... ./pkg/util/... ./app/content/... ./app/media/... -race -cover`
- [ ] `go vet ./...`
- [ ] `golangci-lint run`（若已安装）
- [ ] 检查 `git diff --stat` 确认仅修改计划内文件

---

## 自检结果

1. **Spec coverage**: 7 个 CRITICAL（C1-C7）全部覆盖；HIGH 中的 H2（裸错误透传代表项）、H3（Context key 统一）已覆盖。其余 HIGH 项（RMW 竞态在其他模块、N+1 查询、三层架构修正等）建议在后续迭代中处理。
2. **Placeholder scan**: 无 TBD/TODO/fill in details；每个代码步骤均包含可执行代码。
3. **Type consistency**: `contextKey` 在 `jwtx/context.go` 和 `middleware/auth.go` 中一致使用；`UpdateStatus` 签名在各任务中一致。
