# 测试规范落地实施方案

> 关联规范：`.claude/specs/testing.md`

## 背景

项目测试规范已修订，核心变化：
1. 集成测试从环境变量直连迁移到 testcontainers
2. 覆盖率标准从整体达标改为每个 Logic 包独立 ≥80%
3. 分 P0/P1/P2 三阶段补齐
4. MQ Consumer 只写单元测试，业务逻辑下沉到独立 Logic 函数

当前状态：`testcontainers-go` 已在 go.mod 但从未使用，`pkg/testutil/` 不存在，P0 模块覆盖率极低（User 18.2%，Gateway login 0%）。

---

## Phase 1: 基础设施 `pkg/testutil/`

### 产出

```
pkg/testutil/
└── integration.go    # SetupTestEnv + TestEnv.Close + TestEnv.TruncateAll
```

### 设计

```go
type TestEnv struct {
    DB    *sql.DB
    Redis *redis.Redis
}

func SetupTestEnv(t *testing.T) *TestEnv
func (e *TestEnv) Close()
func (e *TestEnv) TruncateAll(t *testing.T, tables ...string)
```

- 内部用 `testcontainers-go/modules/mysql` 和 `testcontainers-go` GenericContainer 启动 MySQL 8.0 + Redis 7
- 容器端口动态分配，DSN 自动拼接
- `Close()` 通过 `testcontainers.TerminateContainer` 清理
- `TruncateAll` 在 Close 前调用，逐个表执行 `TRUNCATE TABLE`

### 使用方式（各服务 TestMain）

```go
//go:build integration

var testEnv *testutil.TestEnv
var testSvcCtx *svc.ServiceContext

func TestMain(m *testing.M) {
    testEnv = testutil.SetupTestEnv(m)
    testSvcCtx = buildSvcCtx(testEnv.DB, testEnv.Redis)
    util.InitSnowflake(1, 1)
    code := m.Run()
    testEnv.Close()
    os.Exit(code)
}
```

---

## Phase 2: P0 单元测试

### 2a. User RPC logic 单元测试

**前置**: 新建 `app/user/rpc/internal/logic/mock_models_test.go`，定义所有 Mock Model 和 `newUnitSvcCtx()`。

**新增文件**（每个 Logic 至少 1 个成功 + 每分支 1 个失败）：

| 测试文件 | 对应 Logic | 关键失败路径 |
|---------|-----------|------------|
| `follow_logic_test.go` | Follow | 参数校验、DB Insert 失败 |
| `unfollow_logic_test.go` | Unfollow | 参数校验、DB Delete 失败 |
| `login_logic_test.go` | Login | 用户不存在、密码错误、DB 查询失败 |
| `register_logic_test.go` | Register | 参数校验、用户名已存在、DB Insert 失败 |
| `send_verify_code_logic_test.go` | SendVerifyCode | 参数校验、Redis 写入失败 |
| `get_user_logic_test.go` | GetUser | 用户不存在、DB 查询失败、缓存失败 |
| `batch_get_users_logic_test.go` | BatchGetUsers | 空列表、DB 查询失败 |
| `update_profile_logic_test.go` | UpdateProfile | 参数校验、DB Update 失败 |
| `get_user_tags_logic_test.go` | GetUserTags | DB 查询失败 |
| `convert_test.go` | UserProfileToUserInfo | nil 输入 |

### 2b. Gateway login 单元测试

**前置**: 新建 `app/gateway/internal/logic/login/mock_rpc_clients_test.go`，定义 `MockUserRpcClient` 和 `newUnitSvcCtx()`。

**新增文件**：

| 测试文件 | 对应 Logic | 关键失败路径 |
|---------|-----------|------------|
| `login_logic_test.go` | Login | RPC 返回 error、业务错误码 |
| `register_logic_test.go` | Register | RPC 返回 error、参数校验 |
| `send_verify_code_logic_test.go` | SendVerifyCode | RPC 返回 error |
| `convert_test.go` | RegisterReqConvert, RegisterRespConvert | nil 输入 |

---

## Phase 3: P0 集成测试

### User RPC logic 集成测试

使用 `pkg/testutil.TestEnv`，每个 Logic 至少 1 个成功路径集成测试：

| 测试文件 | 验证内容 |
|---------|---------|
| `follow_integration_test.go` | 真实 DB Insert + 外键约束 |
| `unfollow_integration_test.go` | 真实 DB Delete + 级联 |
| `login_integration_test.go` | 真实 DB 查询 + 密码校验 |
| `register_integration_test.go` | 真实 DB Insert + 唯一约束冲突 |
| `send_verify_code_integration_test.go` | 真实 Redis 读写 + TTL |
| `get_user_integration_test.go` | 真实 DB + Redis 缓存 |
| `batch_get_users_integration_test.go` | 真实 DB IN 查询 |
| `update_profile_integration_test.go` | 真实 DB Update |
| `get_user_tags_integration_test.go` | 真实 DB Join 查询 |

### User RPC model 集成测试

| 测试文件 | 验证内容 |
|---------|---------|
| `user_follow_model_integration_test.go` | FindFollowers, FindFollowing, CountFollowers, CountFollowing |
| `user_profile_model_integration_test.go` | UpdateUserDes, FindOneByIdForUpdate |

---

## Phase 4: 迁移旧测试

将已有 13 个集成测试文件从 `os.Getenv("TEST_MYSQL_DSN")` 迁移到 `testutil.SetupTestEnv(t)`：

- Content RPC: 5 文件（comment/post/tag/get_user_posts + TestMain）
- Interaction RPC: 2 文件（interaction + TestMain）
- Media RPC: 6 文件（upload/delete/get/batch_get + TestMain）
- Feed RPC model: 2 文件
- Message RPC model: 1 文件

改动量小——每个 TestMain 替换约 30 行连接代码为 5 行 `testutil.SetupTestEnv` 调用。

---

## 验收标准

每个 Phase 完成后：
1. `go test -race ./...` 通过
2. 新代码覆盖率 ≥80%
3. `go vet ./...` 无告警
4. 集成测试: `go test -race -tags=integration ./...` 通过
5. 单次 commit，不跨 Phase 合并
