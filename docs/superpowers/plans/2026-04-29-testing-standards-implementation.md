# 测试规范落地实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 搭建 pkg/testutil 基础设施，补齐 User RPC 和 Gateway login 的单元+集成测试到 80% 覆盖率，迁移旧集成测试到 testcontainers。

**Architecture:** 四阶段递进 — Phase 1 抽取公共 testcontainers helper → Phase 2 P0 单元测试 → Phase 3 P0 集成测试 → Phase 4 迁移旧测试。每阶段结束后独立验证。

**Tech Stack:** Go 1.26, go-zero v1.10.1, testify, testcontainers-go v0.42.0, MySQL 8.0, Redis 7

---

### Task 1: 创建 pkg/testutil 集成测试 helper

**Files:**
- Create: `pkg/testutil/integration.go`

- [ ] **Step 1: 写 TestEnv 结构体和 SetupTestEnv 函数**

```go
// pkg/testutil/integration.go
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	mysqlcontainer "github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

type TestEnv struct {
	DB       *sql.DB
	Redis    *redis.Redis
	MySQLDSN string
	closeFn  func()
}

// SetupTestEnv 启动 MySQL 8.0 + Redis 7 容器，返回统一测试环境。
// schemaPath 是 SQL 建表脚本的绝对路径。
func SetupTestEnv(t *testing.T, schemaPath string) *TestEnv {
	t.Helper()
	ctx := context.Background()

	// ── MySQL ──────────────────────────────────────────────
	mysqlContainer, err := mysqlcontainer.Run(ctx,
		"mysql:8.0",
		mysqlcontainer.WithDatabase("testdb"),
		mysqlcontainer.WithUsername("root"),
		mysqlcontainer.WithPassword("testpass"),
		mysqlcontainer.WithScripts(schemaPath),
		testcontainers.WithEnv(map[string]string{
			"TZ":   "Asia/Shanghai",
			"LANG": "C.UTF-8",
		}),
		testcontainers.WithCmd(
			"--default-authentication-plugin=mysql_native_password",
			"--character-set-server=utf8mb4",
			"--collation-server=utf8mb4_unicode_ci",
			"--sql-mode=STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION",
		),
	)
	require.NoError(t, err)

	dsn, err := mysqlContainer.ConnectionString(ctx,
		"charset=utf8mb4", "parseTime=true", "loc=Asia%2FShanghai")
	require.NoError(t, err)

	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	require.NoError(t, db.PingContext(ctx))

	// ── Redis ──────────────────────────────────────────────
	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	}
	redisContainer, err := testcontainers.GenericContainer(ctx, req)
	require.NoError(t, err)

	redisHost, err := redisContainer.Host(ctx)
	require.NoError(t, err)
	redisPort, err := redisContainer.MappedPort(ctx, "6379")
	require.NoError(t, err)

	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort.Port())
	rds := redis.MustNewRedis(redis.RedisConf{
		Host: redisAddr,
		Type: redis.NodeType,
	})

	cleanup := func() {
		_ = db.Close()
		_ = rds.Close()
		_ = testcontainers.TerminateContainer(mysqlContainer)
		_ = testcontainers.TerminateContainer(redisContainer)
	}

	return &TestEnv{
		DB:       db,
		Redis:    rds,
		MySQLDSN: dsn,
		closeFn:  cleanup,
	}
}

func (e *TestEnv) Close() {
	if e.closeFn != nil {
		e.closeFn()
	}
}

// TruncateAll 清空指定表。注意 SET FOREIGN_KEY_CHECKS=0 处理外键约束。
func (e *TestEnv) TruncateAll(t *testing.T, tables ...string) {
	t.Helper()
	_, err := e.DB.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS = 0")
	require.NoError(t, err)
	for _, table := range tables {
		_, err := e.DB.ExecContext(context.Background(), "TRUNCATE TABLE "+table)
		require.NoError(t, err)
	}
	_, err = e.DB.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS = 1")
	require.NoError(t, err)
}

// SchemaPath 返回项目根目录下的 deploy/sql/ 路径。
func SchemaPath(filename string) string {
	_, f, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(f), "..", "..")
	return filepath.Join(root, "deploy", "sql", filename)
}
```

- [ ] **Step 2: 编译验证**

```bash
cd pkg/testutil && go build ./...
```

Expected: 编译成功（testcontainers 依赖已在 go.mod）

- [ ] **Step 3: Commit**

```bash
git add -f pkg/testutil/integration.go
git commit -m "feat(testutil): add shared testcontainers helper for integration tests

- SetupTestEnv starts MySQL 8.0 + Redis 7 containers
- TruncateAll clears tables with foreign key handling
- SchemaPath resolves deploy/sql/ scripts from project root"
```

---

### Task 2: User RPC — mock_models_test.go + newUnitSvcCtx

**Files:**
- Create: `app/user/rpc/internal/logic/mock_models_test.go`

> 注意：User 已有测试文件 `get_followers_logic_test.go` / `get_following_logic_test.go` 中内联了 `mockUserFollowModel`。本 Task 创建的集中 Mock 文件将替换它们，并补充其余 Model 的 Mock。

- [ ] **Step 1: 创建集中 mock 文件**

```go
// mock_models_test.go
package logic

import (
	"context"
	"database/sql"
	"time"

	"user/internal/model"
	"user/internal/svc"

	"github.com/stretchr/testify/mock"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// ── Mock Models ──────────────────────────────────────────────────────────────

type MockUserProfileModel struct{ mock.Mock }

func (m *MockUserProfileModel) Insert(ctx context.Context, data *model.UserProfile) (sql.Result, error) {
	args := m.Called(ctx, data)
	r, _ := args.Get(0).(sql.Result)
	return r, args.Error(1)
}

func (m *MockUserProfileModel) FindOne(ctx context.Context, id int64) (*model.UserProfile, error) {
	args := m.Called(ctx, id)
	v, _ := args.Get(0).(*model.UserProfile)
	return v, args.Error(1)
}

func (m *MockUserProfileModel) FindOneByPhone(ctx context.Context, phone sql.NullString) (*model.UserProfile, error) {
	args := m.Called(ctx, phone)
	v, _ := args.Get(0).(*model.UserProfile)
	return v, args.Error(1)
}

func (m *MockUserProfileModel) FindOneByUsername(ctx context.Context, username string) (*model.UserProfile, error) {
	args := m.Called(ctx, username)
	v, _ := args.Get(0).(*model.UserProfile)
	return v, args.Error(1)
}

func (m *MockUserProfileModel) Update(ctx context.Context, data *model.UserProfile) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *MockUserProfileModel) Delete(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockUserProfileModel) UpdateUserDes(ctx context.Context, userId int64, nickname, avatarUrl, bio string) error {
	args := m.Called(ctx, userId, nickname, avatarUrl, bio)
	return args.Error(0)
}

func (m *MockUserProfileModel) FindOneByIdForUpdate(ctx context.Context, session sqlx.Session, id int64) (*model.UserProfile, error) {
	args := m.Called(ctx, session, id)
	v, _ := args.Get(0).(*model.UserProfile)
	return v, args.Error(1)
}

// MockUserFollowStore 实现 svc.UserFollowStore 接口
type MockUserFollowStore struct{ mock.Mock }

func (m *MockUserFollowStore) FindFollowers(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error) {
	args := m.Called(ctx, userID, offset, limit)
	v, _ := args.Get(0).([]*model.UserProfile)
	return v, args.Error(1)
}

func (m *MockUserFollowStore) FindFollowing(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error) {
	args := m.Called(ctx, userID, offset, limit)
	v, _ := args.Get(0).([]*model.UserProfile)
	return v, args.Error(1)
}

func (m *MockUserFollowStore) CountFollowers(ctx context.Context, userID int64) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockUserFollowStore) CountFollowing(ctx context.Context, userID int64) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

// ── SvcCtx Builder ────────────────────────────────────────────────────────────

// newUnitSvcCtx 构造测试用 ServiceContext。Redis 设为 nil —— 依赖 Redis 的逻辑
// 路径在集成测试中覆盖。
func newUnitSvcCtx(
	profileModel model.UserProfileModel,
	followStore svc.UserFollowStore,
) *svc.ServiceContext {
	return &svc.ServiceContext{
		UserProfileModel: profileModel,
		UserFollowModel:  followStore,
	}
}

// ── Shared Helpers ────────────────────────────────────────────────────────────

func sampleUser(id int64, username string) *model.UserProfile {
	return &model.UserProfile{
		Id:           id,
		Username:     username,
		Password:     "$2a$10$dummyhash",
		CreatedAt:    time.Unix(1710000000, 0),
		FollowerCount:  3,
		FollowingCount: 5,
	}
}
```

- [ ] **Step 2: 编译验证**

```bash
cd app/user/rpc && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add app/user/rpc/internal/logic/mock_models_test.go
git commit -m "test(user): add centralized mock models and svcCtx builder"
```

---

### Task 3: User RPC — get_user_logic_test.go

**Files:**
- Create: `app/user/rpc/internal/logic/get_user_logic_test.go`

- [ ] **Step 1: 写测试**

```go
package logic

import (
	"context"
	"errors"
	"testing"

	"errx"
	"user/internal/model"
	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetUserLogic(t *testing.T) {
	tests := []struct {
		name        string
		req         *pb.GetUserReq
		setupMock   func(*MockUserProfileModel)
		wantErr     bool
		errCode     int
		check       func(t *testing.T, resp *pb.GetUserResp)
	}{
		{
			name: "成功获取用户",
			req:  &pb.GetUserReq{UserId: 1},
			setupMock: func(m *MockUserProfileModel) {
				m.On("FindOne", mock.Anything, int64(1)).Return(sampleUser(1, "alice"), nil).Once()
			},
			check: func(t *testing.T, resp *pb.GetUserResp) {
				assert.Equal(t, int64(1), resp.User.Id)
				assert.Equal(t, "alice", resp.User.Username)
			},
		},
		{
			name: "用户不存在",
			req:  &pb.GetUserReq{UserId: 999},
			setupMock: func(m *MockUserProfileModel) {
				m.On("FindOne", mock.Anything, int64(999)).Return(
					(*model.UserProfile)(nil), model.ErrNotFound,
				).Once()
			},
			wantErr: true,
			errCode: errx.UserNotFound,
		},
		{
			name: "DB 错误",
			req:  &pb.GetUserReq{UserId: 1},
			setupMock: func(m *MockUserProfileModel) {
				m.On("FindOne", mock.Anything, int64(1)).Return(
					(*model.UserProfile)(nil), errors.New("connection refused"),
				).Once()
			},
			wantErr: true,
			errCode: errx.SystemError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockUserProfileModel)
			if tt.setupMock != nil {
				tt.setupMock(pm)
			}
			svcCtx := newUnitSvcCtx(pm, nil)
			logic := NewGetUserLogic(context.Background(), svcCtx)

			resp, err := logic.GetUser(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.errCode, errx.GetCode(err))
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, resp)
				}
			}
			pm.AssertExpectations(t)
		})
	}
}
```

- [ ] **Step 2: 运行测试验证**

```bash
go test ./app/user/rpc/internal/logic/ -run TestGetUserLogic -v
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add app/user/rpc/internal/logic/get_user_logic_test.go
git commit -m "test(user): add GetUserLogic unit tests"
```

---

### Task 4: User RPC — update_profile_logic_test.go

**Files:**
- Create: `app/user/rpc/internal/logic/update_profile_logic_test.go`

- [ ] **Step 1: 写测试**

```go
package logic

import (
	"context"
	"errors"
	"testing"

	"errx"
	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestUpdateProfileLogic(t *testing.T) {
	tests := []struct {
		name      string
		req       *pb.UpdateProfileReq
		setupMock func(*MockUserProfileModel)
		wantErr   bool
		errCode   int
	}{
		{
			name: "成功更新资料",
			req:  &pb.UpdateProfileReq{UserId: 1, Nickname: "newNick", AvatarUrl: "http://a.jpg", Bio: "hello"},
			setupMock: func(m *MockUserProfileModel) {
				m.On("UpdateUserDes", mock.Anything, int64(1), "newNick", "http://a.jpg", "hello").Return(nil).Once()
			},
		},
		{
			name: "DB 错误",
			req:  &pb.UpdateProfileReq{UserId: 1, Nickname: "nick"},
			setupMock: func(m *MockUserProfileModel) {
				m.On("UpdateUserDes", mock.Anything, int64(1), "nick", "", "").Return(errors.New("db down")).Once()
			},
			wantErr: true,
			errCode: errx.SystemError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockUserProfileModel)
			if tt.setupMock != nil {
				tt.setupMock(pm)
			}
			svcCtx := newUnitSvcCtx(pm, nil)
			logic := NewUpdateProfileLogic(context.Background(), svcCtx)

			resp, err := logic.UpdateProfile(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.errCode, errx.GetCode(err))
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
			}
			pm.AssertExpectations(t)
		})
	}
}
```

- [ ] **Step 2: 运行测试验证**

```bash
go test ./app/user/rpc/internal/logic/ -run TestUpdateProfileLogic -v
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add app/user/rpc/internal/logic/update_profile_logic_test.go
git commit -m "test(user): add UpdateProfileLogic unit tests"
```

---

### Task 5: User RPC — send_verify_code 集成测试

**Files:**
- Create: `app/user/rpc/internal/logic/send_verify_code_integration_test.go`

SendVerifyCode 唯一逻辑是调用 `RedisClient.SetexCtx`。`*redis.Redis` 是 go-zero 具体类型无法 mock，因此直接写集成测试。

- [ ] **Step 1: 写集成测试**

```go
//go:build integration

package logic

import (
	"context"
	"testing"

	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendVerifyCodeIntegration_Success(t *testing.T) {
	logic := NewSendVerifyCodeLogic(context.Background(), testSvcCtx)
	resp, err := logic.SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800138000"})

	require.NoError(t, err)
	require.NotNil(t, resp)

	// 验证 Redis 中确实写入了验证码
	code, err := testEnv.Redis.GetCtx(context.Background(), "13800138000")
	require.NoError(t, err)
	assert.NotEmpty(t, code)
	assert.Len(t, code, 6)
}
```

- [ ] **Step 2: 运行集成测试验证**

```bash
go test -tags=integration ./app/user/rpc/internal/logic/ -run TestSendVerifyCodeIntegration -v
```

- [ ] **Step 3: Commit**

```bash
git add app/user/rpc/internal/logic/send_verify_code_integration_test.go
git commit -m "test(user): add SendVerifyCode integration test"
```

---

### Task 6: User RPC — login_logic_test.go

**Files:**
- Create: `app/user/rpc/internal/logic/login_logic_test.go`

Login 分密码登录和验证码登录两个路径。密码登录路径无 Redis 依赖，可纯单元测试。验证码登录路径依赖 `RedisClient.GetCtx/DelCtx`（`*redis.Redis` 具体类型无法 mock），在集成测试中覆盖（见 Task 16）。

- [ ] **Step 1: 写单元测试（密码登录路径）**

```go
package logic

import (
	"context"
	"errors"
	"testing"

	"errx"
	"jwtx"
	"user/internal/model"
	"user/pb/xiaobaihe/user/pb"
	"util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestLoginLogic_Password(t *testing.T) {
	hashedPwd, _ := util.HashPassword("correct123")

	tests := []struct {
		name      string
		req       *pb.LoginReq
		setupMock func(*MockUserProfileModel)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *pb.LoginResp)
	}{
		{
			name: "密码登录成功",
			req:  &pb.LoginReq{Username: "alice", Password: "correct123", LoginType: 1},
			setupMock: func(pm *MockUserProfileModel) {
				pm.On("FindOneByUsername", mock.Anything, "alice").Return(
					&model.UserProfile{Id: 1, Username: "alice", Password: hashedPwd}, nil,
				).Once()
			},
			check: func(t *testing.T, resp *pb.LoginResp) {
				assert.Equal(t, int64(1), resp.UserId)
				assert.NotEmpty(t, resp.Token)
			},
		},
		{
			name: "用户不存在",
			req:  &pb.LoginReq{Username: "nobody", Password: "x", LoginType: 1},
			setupMock: func(pm *MockUserProfileModel) {
				pm.On("FindOneByUsername", mock.Anything, "nobody").Return(
					(*model.UserProfile)(nil), model.ErrNotFound,
				).Once()
			},
			wantErr: true,
			errCode: errx.UserNotFound,
		},
		{
			name: "密码错误",
			req:  &pb.LoginReq{Username: "alice", Password: "wrong", LoginType: 1},
			setupMock: func(pm *MockUserProfileModel) {
				pm.On("FindOneByUsername", mock.Anything, "alice").Return(
					&model.UserProfile{Id: 1, Username: "alice", Password: hashedPwd}, nil,
				).Once()
			},
			wantErr: true,
			errCode: errx.PasswordError,
		},
		{
			name: "默认密码被拒绝",
			req:  &pb.LoginReq{Username: "alice", Password: "Xbh@2024!", LoginType: 1},
			setupMock: func(pm *MockUserProfileModel) {
				pm.On("FindOneByUsername", mock.Anything, "alice").Return(
					&model.UserProfile{Id: 1, Username: "alice", Password: hashedPwd}, nil,
				).Once()
			},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "DB 错误",
			req:  &pb.LoginReq{Username: "alice", Password: "x", LoginType: 1},
			setupMock: func(pm *MockUserProfileModel) {
				pm.On("FindOneByUsername", mock.Anything, "alice").Return(
					(*model.UserProfile)(nil), errors.New("db down"),
				).Once()
			},
			wantErr: true,
			errCode: errx.SystemError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockUserProfileModel)
			if tt.setupMock != nil {
				tt.setupMock(pm)
			}
			svcCtx := newUnitSvcCtx(pm, nil)
			svcCtx.Config.JwtConfig = jwtx.JwtConfig{
				AccessSecret: "test-secret-32bytes-long-key!!",
				AccessExpire: 3600,
			}

			logic := NewLoginLogic(context.Background(), svcCtx)
			resp, err := logic.Login(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.errCode, errx.GetCode(err))
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, resp)
				}
			}
			pm.AssertExpectations(t)
		})
	}
}
```

- [ ] **Step 2: 运行测试验证**

```bash
go test ./app/user/rpc/internal/logic/ -run TestLoginLogic_Password -v
```

- [ ] **Step 3: Commit**

```bash
git add app/user/rpc/internal/logic/login_logic_test.go
git commit -m "test(user): add LoginLogic password-path unit tests"
```

---

### Task 7: User RPC — register_logic_test.go

**Files:**
- Create: `app/user/rpc/internal/logic/register_logic_test.go`

Register 有 `registerByUserName`（无 Redis 依赖）和 `registerByPhone`（依赖 Redis）两个子路径。单元测试只覆盖用户名注册路径，手机号注册路径在集成测试覆盖（见 Task 16）。

- [ ] **Step 1: 写单元测试（用户名注册路径）**

```go
package logic

import (
	"context"
	"errors"
	"testing"

	"errx"
	"jwtx"
	"user/internal/model"
	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRegisterLogic_ByUsername(t *testing.T) {
	tests := []struct {
		name      string
		req       *pb.RegisterReq
		setupMock func(*MockUserProfileModel)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *pb.RegisterResp)
	}{
		{
			name: "用户名注册成功",
			req:  &pb.RegisterReq{Username: "newuser", Password: "Strong@123"},
			setupMock: func(m *MockUserProfileModel) {
				m.On("Insert", mock.Anything, mock.AnythingOfType("*model.UserProfile")).Return(nil, nil).Once()
			},
			check: func(t *testing.T, resp *pb.RegisterResp) {
				assert.Greater(t, resp.UserId, int64(0))
				assert.NotEmpty(t, resp.Token)
			},
		},
		{
			name:    "密码太弱",
			req:     &pb.RegisterReq{Username: "newuser", Password: "123"},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "用户名已存在",
			req:  &pb.RegisterReq{Username: "existing", Password: "Strong@123"},
			setupMock: func(m *MockUserProfileModel) {
				m.On("Insert", mock.Anything, mock.AnythingOfType("*model.UserProfile")).Return(nil, errors.New("duplicate")).Once()
			},
			wantErr: true,
			errCode: errx.UserAlreadyExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockUserProfileModel)
			if tt.setupMock != nil {
				tt.setupMock(pm)
			}
			svcCtx := newUnitSvcCtx(pm, nil)
			svcCtx.Config.JwtConfig = jwtx.JwtConfig{
				AccessSecret: "test-secret-32bytes-long-key!!",
				AccessExpire: 3600,
			}

			logic := NewRegisterLogic(context.Background(), svcCtx)
			resp, err := logic.Register(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.errCode, errx.GetCode(err))
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, resp)
				}
			}
			pm.AssertExpectations(t)
		})
	}
}
```

- [ ] **Step 2: 运行测试验证**

```bash
go test ./app/user/rpc/internal/logic/ -run TestRegisterLogic_ByUsername -v
```

- [ ] **Step 3: Commit**

```bash
git add app/user/rpc/internal/logic/register_logic_test.go
git commit -m "test(user): add RegisterLogic username-path unit tests"
```

---

### Task 8: User RPC — convert_test.go

**Files:**
- Create: `app/user/rpc/internal/logic/convert_test.go`

- [ ] **Step 1: 写测试**

```go
package logic

import (
	"database/sql"
	"testing"
	"time"

	"user/internal/model"

	"github.com/stretchr/testify/assert"
)

func TestUserProfileToUserInfo(t *testing.T) {
	tests := []struct {
		name    string
		profile *model.UserProfile
		check   func(t *testing.T, result interface{})
	}{
		{
			name: "完整转换",
			profile: &model.UserProfile{
				Id:                  1,
				Username:            "alice",
				Nickname:            sql.NullString{String: "Alice", Valid: true},
				AvatarUrl:           sql.NullString{String: "http://a.jpg", Valid: true},
				Bio:                 sql.NullString{String: "hi", Valid: true},
				Level:               5,
				FollowerCount:       10,
				FollowingCount:      20,
				PostCount:           30,
				LikeCount:           40,
				CreatedAt:           time.Unix(1710000000, 0),
				FavoritesVisibility: 1,
			},
			check: func(t *testing.T, result interface{}) {
				info := result.(*pb.UserInfo)
				assert.Equal(t, int64(1), info.Id)
				assert.Equal(t, "alice", info.Username)
				assert.Equal(t, "Alice", info.Nickname)
				assert.Equal(t, "http://a.jpg", info.AvatarUrl)
				assert.Equal(t, "hi", info.Bio)
				assert.Equal(t, int32(5), info.Level)
				assert.Equal(t, int64(10), info.FollowerCount)
				assert.Equal(t, int64(30), info.PostCount)
				assert.Equal(t, int64(1710000000), info.CreatedAt)
				assert.Equal(t, int32(1), info.FavoritesVisibility)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UserProfileToUserInfo(tt.profile)
			tt.check(t, result)
		})
	}
}
```

需要补充 import `user/pb/xiaobaihe/user/pb`，修正上面的 `pb.UserInfo` 引用。

- [ ] **Step 2: 运行测试验证**

```bash
go test ./app/user/rpc/internal/logic/ -run TestUserProfileToUserInfo -v
```

- [ ] **Step 3: Commit**

```bash
git add app/user/rpc/internal/logic/convert_test.go
git commit -m "test(user): add UserProfileToUserInfo convert unit tests"
```

---

### Task 9: User RPC — follow 和 unfollow 基础测试

**Files:**
- Create: `app/user/rpc/internal/logic/follow_logic_test.go`
- Create: `app/user/rpc/internal/logic/unfollow_logic_test.go`

Follow 和 Unfollow 目前是 stub（仅返回空 struct）。写入基础测试覆盖当前行为。

- [ ] **Step 1: 写 follow 测试**

```go
package logic

import (
	"context"
	"testing"

	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/require"
)

func TestFollowLogic_Stub(t *testing.T) {
	logic := NewFollowLogic(context.Background(), nil)
	resp, err := logic.Follow(&pb.FollowReq{UserId: 1, TargetUserId: 2})

	require.NoError(t, err)
	require.NotNil(t, resp)
}
```

- [ ] **Step 2: 写 unfollow 测试**

```go
package logic

import (
	"context"
	"testing"

	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/require"
)

func TestUnfollowLogic_Stub(t *testing.T) {
	logic := NewUnfollowLogic(context.Background(), nil)
	resp, err := logic.Unfollow(&pb.UnfollowReq{UserId: 1, TargetUserId: 2})

	require.NoError(t, err)
	require.NotNil(t, resp)
}
```

- [ ] **Step 3: 运行测试验证**

```bash
go test ./app/user/rpc/internal/logic/ -run "TestFollowLogic_Stub|TestUnfollowLogic_Stub" -v
```

- [ ] **Step 4: Commit**

```bash
git add app/user/rpc/internal/logic/follow_logic_test.go app/user/rpc/internal/logic/unfollow_logic_test.go
git commit -m "test(user): add Follow/Unfollow stub tests"
```

---

### Task 10: User RPC — 重构现有测试文件使用集中 Mock

**Files:**
- Modify: `app/user/rpc/internal/logic/get_followers_logic_test.go`
- Modify: `app/user/rpc/internal/logic/get_following_logic_test.go`

现有两个测试文件内联定义了 `mockUserFollowModel`。现在改用 `mock_models_test.go` 中的 `MockUserFollowStore`。

- [ ] **Step 1: 重构 get_followers_logic_test.go**

删除内联 `mockUserFollowModel` 定义（`type mockUserFollowModel struct{ mock.Mock }` 及其 4 个方法），替换：
- `new(mockUserFollowModel)` → `new(MockUserFollowStore)`
- `&svc.ServiceContext{UserFollowModel: followModel}` → `newUnitSvcCtx(nil, followModel)`

三个测试函数保持不变，只替换类型引用。

- [ ] **Step 2: 重构 get_following_logic_test.go**（同上)

- [ ] **Step 3: 运行测试验证**

```bash
go test ./app/user/rpc/internal/logic/ -run "TestGetFollowers|TestGetFollowing" -v
```

- [ ] **Step 4: Commit**

```bash
git add app/user/rpc/internal/logic/get_followers_logic_test.go app/user/rpc/internal/logic/get_following_logic_test.go
git commit -m "refactor(test): use centralized mocks in existing user logic tests"
```

---

### Task 11: Gateway login — mock RPC clients

**Files:**
- Create: `app/gateway/internal/logic/login/mock_rpc_clients_test.go`

Gateway login 依赖 `svc.ServiceContext.UserService`（`userservice.UserService` 接口）。

- [ ] **Step 1: 创建 mock 文件**

```go
// mock_rpc_clients_test.go
package login

import (
	"context"

	"gateway/internal/svc"
	"user/pb/xiaobaihe/user/pb"
	"user/userservice"

	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

type MockUserService struct {
	mock.Mock
}

func (m *MockUserService) GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...grpc.CallOption) (*userservice.GetUserResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetUserResp)
	return v, args.Error(1)
}

func (m *MockUserService) BatchGetUsers(ctx context.Context, in *userservice.BatchGetUsersReq, opts ...grpc.CallOption) (*userservice.BatchGetUsersResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.BatchGetUsersResp)
	return v, args.Error(1)
}

func (m *MockUserService) UpdateProfile(ctx context.Context, in *userservice.UpdateProfileReq, opts ...grpc.CallOption) (*userservice.UpdateProfileResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.UpdateProfileResp)
	return v, args.Error(1)
}

func (m *MockUserService) Follow(ctx context.Context, in *userservice.FollowReq, opts ...grpc.CallOption) (*userservice.FollowResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.FollowResp)
	return v, args.Error(1)
}

func (m *MockUserService) Unfollow(ctx context.Context, in *userservice.UnfollowReq, opts ...grpc.CallOption) (*userservice.UnfollowResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.UnfollowResp)
	return v, args.Error(1)
}

func (m *MockUserService) GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...grpc.CallOption) (*userservice.GetFollowersResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetFollowersResp)
	return v, args.Error(1)
}

func (m *MockUserService) GetFollowing(ctx context.Context, in *userservice.GetFollowingReq, opts ...grpc.CallOption) (*userservice.GetFollowingResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetFollowingResp)
	return v, args.Error(1)
}

func (m *MockUserService) GetUserTags(ctx context.Context, in *userservice.GetUserTagsReq, opts ...grpc.CallOption) (*userservice.GetUserTagsResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.GetUserTagsResp)
	return v, args.Error(1)
}

func (m *MockUserService) Register(ctx context.Context, in *userservice.RegisterReq, opts ...grpc.CallOption) (*userservice.RegisterResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.RegisterResp)
	return v, args.Error(1)
}

func (m *MockUserService) Login(ctx context.Context, in *userservice.LoginReq, opts ...grpc.CallOption) (*userservice.LoginResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.LoginResp)
	return v, args.Error(1)
}

func (m *MockUserService) SendVerifyCode(ctx context.Context, in *userservice.SendVerifyCodeReq, opts ...grpc.CallOption) (*userservice.SendVerifyCodeResp, error) {
	args := m.Called(ctx, in)
	v, _ := args.Get(0).(*userservice.SendVerifyCodeResp)
	return v, args.Error(1)
}

func newUnitSvcCtx(userSvc userservice.UserService) *svc.ServiceContext {
	return &svc.ServiceContext{
		UserService: userSvc,
	}
}
```

- [ ] **Step 2: 编译验证**

```bash
cd app/gateway && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add app/gateway/internal/logic/login/mock_rpc_clients_test.go
git commit -m "test(gateway): add MockUserService for login logic tests"
```

---

### Task 12: Gateway — login_logic_test.go

**Files:**
- Create: `app/gateway/internal/logic/login/login_logic_test.go`

- [ ] **Step 1: 写测试**

```go
package login

import (
	"context"
	"errors"
	"testing"

	"errx"
	"gateway/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"user/userservice"
)

func TestLoginLogic(t *testing.T) {
	tests := []struct {
		name      string
		req       *types.LoginReq
		setupMock func(*MockUserService)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *types.LoginResp)
	}{
		{
			name: "密码登录成功",
			req:  &types.LoginReq{Username: "alice", Password: "correct", LoginType: 1},
			setupMock: func(m *MockUserService) {
				m.On("Login", mock.Anything, &userservice.LoginReq{
					Username: "alice", Password: "correct", LoginType: 1,
				}).Return(&userservice.LoginResp{UserId: 1, Token: "token123"}, nil).Once()
			},
			check: func(t *testing.T, resp *types.LoginResp) {
				assert.Equal(t, int64(1), resp.UserId)
				assert.Equal(t, "token123", resp.Token)
			},
		},
		{
			name: "密码登录-用户名为空",
			req:  &types.LoginReq{Password: "x", LoginType: 1},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "密码登录-密码为空",
			req:  &types.LoginReq{Username: "alice", LoginType: 1},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "验证码登录-手机号非法",
			req:  &types.LoginReq{Phone: "123", LoginType: 2},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "验证码登录-验证码为空",
			req:  &types.LoginReq{Phone: "13800138000", LoginType: 2},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "RPC 返回错误",
			req:  &types.LoginReq{Username: "alice", Password: "correct", LoginType: 1},
			setupMock: func(m *MockUserService) {
				m.On("Login", mock.Anything, mock.Anything).Return(
					(*userservice.LoginResp)(nil), errors.New("rpc timeout"),
				).Once()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userSvc := new(MockUserService)
			if tt.setupMock != nil {
				tt.setupMock(userSvc)
			}
			svcCtx := newUnitSvcCtx(userSvc)
			logic := NewLoginLogic(context.Background(), svcCtx)

			resp, err := logic.Login(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errCode != 0 {
					assert.Equal(t, tt.errCode, errx.GetCode(err))
				}
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, resp)
				}
			}
			userSvc.AssertExpectations(t)
		})
	}
}
```

- [ ] **Step 2: 运行测试验证**

```bash
go test ./app/gateway/internal/logic/login/ -run TestLoginLogic -v
```

- [ ] **Step 3: Commit**

```bash
git add app/gateway/internal/logic/login/login_logic_test.go
git commit -m "test(gateway): add LoginLogic unit tests"
```

---

### Task 13: Gateway — register_logic_test.go + send_verify_code_logic_test.go + convert_test.go

**Files:**
- Create: `app/gateway/internal/logic/login/register_logic_test.go`
- Create: `app/gateway/internal/logic/login/send_verify_code_logic_test.go`
- Create: `app/gateway/internal/logic/login/convert_test.go`

- [ ] **Step 1: register test**

```go
// register_logic_test.go
package login

import (
	"context"
	"errors"
	"testing"

	"errx"
	"gateway/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"user/userservice"
)

func TestRegisterLogic(t *testing.T) {
	tests := []struct {
		name      string
		req       *types.RegisterReq
		setupMock func(*MockUserService)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *types.RegisterResp)
	}{
		{
			name: "用户名注册成功",
			req:  &types.RegisterReq{Username: "newuser", Password: "Strong@123"},
			setupMock: func(m *MockUserService) {
				m.On("Register", mock.Anything, mock.Anything).Return(
					&userservice.RegisterResp{UserId: 10, Token: "tok"}, nil,
				).Once()
			},
			check: func(t *testing.T, resp *types.RegisterResp) {
				assert.Equal(t, int64(10), resp.UserId)
				assert.Equal(t, "tok", resp.Token)
			},
		},
		{
			name: "用户名只含空格校验失败",
			req:  &types.RegisterReq{Username: "  ", Password: "Strong@123"},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "密码太短",
			req:  &types.RegisterReq{Username: "user", Password: "123"},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "用户名和手机号都不提供",
			req:  &types.RegisterReq{},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "RPC 错误",
			req:  &types.RegisterReq{Username: "newuser", Password: "Strong@123"},
			setupMock: func(m *MockUserService) {
				m.On("Register", mock.Anything, mock.Anything).Return(
					(*userservice.RegisterResp)(nil), errors.New("rpc error"),
				).Once()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userSvc := new(MockUserService)
			if tt.setupMock != nil {
				tt.setupMock(userSvc)
			}
			svcCtx := newUnitSvcCtx(userSvc)
			logic := NewRegisterLogic(context.Background(), svcCtx)

			resp, err := logic.Register(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errCode != 0 {
					assert.Equal(t, tt.errCode, errx.GetCode(err))
				}
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, resp)
				}
			}
			userSvc.AssertExpectations(t)
		})
	}
}
```

- [ ] **Step 2: send_verify_code test**

```go
// send_verify_code_logic_test.go
package login

import (
	"context"
	"errors"
	"testing"

	"errx"
	"gateway/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"user/userservice"
)

func TestSendVerifyCodeLogic(t *testing.T) {
	tests := []struct {
		name      string
		req       *types.SendVerifyCodeReq
		setupMock func(*MockUserService)
		wantErr   bool
		errCode   int
	}{
		{
			name: "发送成功",
			req:  &types.SendVerifyCodeReq{Phone: "13800138000", Type: 1},
			setupMock: func(m *MockUserService) {
				m.On("SendVerifyCode", mock.Anything, &userservice.SendVerifyCodeReq{
					Phone: "13800138000", Type: 1,
				}).Return(&userservice.SendVerifyCodeResp{}, nil).Once()
			},
		},
		{
			name:    "手机号非法",
			req:     &types.SendVerifyCodeReq{Phone: "123"},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "RPC 错误",
			req:  &types.SendVerifyCodeReq{Phone: "13800138000", Type: 2},
			setupMock: func(m *MockUserService) {
				m.On("SendVerifyCode", mock.Anything, mock.Anything).Return(
					(*userservice.SendVerifyCodeResp)(nil), errors.New("rpc error"),
				).Once()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userSvc := new(MockUserService)
			if tt.setupMock != nil {
				tt.setupMock(userSvc)
			}
			svcCtx := newUnitSvcCtx(userSvc)
			logic := NewSendVerifyCodeLogic(context.Background(), svcCtx)

			resp, err := logic.SendVerifyCode(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errCode != 0 {
					assert.Equal(t, tt.errCode, errx.GetCode(err))
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
			}
			userSvc.AssertExpectations(t)
		})
	}
}
```

- [ ] **Step 3: convert test**

```go
// convert_test.go
package login

import (
	"testing"

	"gateway/internal/types"

	"github.com/stretchr/testify/assert"
	"user/pb/xiaobaihe/user/pb"
)

func TestRegisterReqConvert(t *testing.T) {
	req := &types.RegisterReq{
		Username: "alice", Password: "pass", Phone: "138", VerifyCode: "123456",
	}
	result := RegisterReqConvert(req)
	assert.Equal(t, "alice", result.Username)
	assert.Equal(t, "pass", result.Password)
	assert.Equal(t, "138", result.Phone)
	assert.Equal(t, "123456", result.VerifyCode)
}

func TestRegisterRespConvert(t *testing.T) {
	resp := &pb.RegisterResp{UserId: 1, Token: "tok"}
	result := RegisterRespConvert(resp)
	assert.Equal(t, int64(1), result.UserId)
	assert.Equal(t, "tok", result.Token)
}
```

- [ ] **Step 4: 运行测试验证**

```bash
go test ./app/gateway/internal/logic/login/ -v
```

- [ ] **Step 5: Commit**

```bash
git add app/gateway/internal/logic/login/register_logic_test.go \
        app/gateway/internal/logic/login/send_verify_code_logic_test.go \
        app/gateway/internal/logic/login/convert_test.go
git commit -m "test(gateway): add Register/SendVerifyCode/Convert unit tests"
```

---

### Task 14: Phase 2 验证 — 覆盖率检查

**Verification:**

- [ ] **Step 1: 运行 User RPC logic 覆盖率**

```bash
go test ./app/user/rpc/internal/logic/ -cover -v
```

Expected: 覆盖率 ≥80%

- [ ] **Step 2: 运行 Gateway login 覆盖率**

```bash
go test ./app/gateway/internal/logic/login/ -cover -v
```

Expected: 覆盖率 ≥80%

- [ ] **Step 3: 全局单元测试回归**

```bash
go test -race ./...
```

Expected: 全部通过

---

### Task 15: User RPC — 集成测试 TestMain 搭建

**Files:**
- Create: `app/user/rpc/internal/logic/integration_test.go`

使用 `pkg/testutil` 搭建 testcontainers 环境，加载 `deploy/sql/xbh_user.sql`。

- [ ] **Step 1: 创建 TestMain**

```go
//go:build integration

package logic

import (
	"os"
	"testing"

	"user/internal/svc"

	"esx/pkg/testutil"
	"util"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var testEnv *testutil.TestEnv
var testSvcCtx *svc.ServiceContext

func TestMain(m *testing.M) {
	testEnv = testutil.SetupTestEnv(m, testutil.SchemaPath("xbh_user.sql"))
	testSvcCtx = buildSvcCtx(testEnv)
	util.InitSnowflake(1, 1)
	code := m.Run()
	testEnv.Close()
	os.Exit(code)
}

func buildSvcCtx(env *testutil.TestEnv) *svc.ServiceContext {
	conn := sqlx.NewSqlConnFromDB(env.DB)
	return &svc.ServiceContext{
		DB:                conn,
		UserProfileModel:  model.NewUserProfileModel(conn),
		UserFollowModel:   model.NewUserFollowModel(conn),
		UserLoginLogModel: model.NewUserLoginLogModel(conn),
		RedisClient:       env.Redis,
	}
}
```

注意：需要补充 import `user/internal/model`。

- [ ] **Step 2: 编译验证**

```bash
cd app/user/rpc && go build -tags=integration ./internal/logic/
```

---

### Task 16: User RPC — 集成测试: login + register

**Files:**
- Create: `app/user/rpc/internal/logic/login_integration_test.go`
- Create: `app/user/rpc/internal/logic/register_integration_test.go`

- [ ] **Step 1: login 集成测试**

```go
//go:build integration

package logic

import (
	"context"
	"testing"

	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/require"
)

func TestLoginIntegration(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile", "user_login_log")

	// 先注册一个用户
	regLogic := NewRegisterLogic(context.Background(), testSvcCtx)
	regResp, err := regLogic.Register(&pb.RegisterReq{
		Username: "logintest",
		Password: "Strong@123",
	})
	require.NoError(t, err)
	require.NotNil(t, regResp)

	// 登录
	loginLogic := NewLoginLogic(context.Background(), testSvcCtx)
	resp, err := loginLogic.Login(&pb.LoginReq{
		Username:  "logintest",
		Password:  "Strong@123",
		LoginType: 1,
	})
	require.NoError(t, err)
	require.Equal(t, regResp.UserId, resp.UserId)
	require.NotEmpty(t, resp.Token)
}
```

- [ ] **Step 2: register 集成测试**

```go
//go:build integration

package logic

import (
	"context"
	"testing"

	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterIntegration(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile")

	logic := NewRegisterLogic(context.Background(), testSvcCtx)
	resp, err := logic.Register(&pb.RegisterReq{
		Username: "integ_user",
		Password: "Strong@123",
	})
	require.NoError(t, err)
	assert.Greater(t, resp.UserId, int64(0))
	assert.NotEmpty(t, resp.Token)
}

func TestRegisterDuplicateIntegration(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile")

	logic := NewRegisterLogic(context.Background(), testSvcCtx)
	_, err := logic.Register(&pb.RegisterReq{
		Username: "dup_user", Password: "Strong@123",
	})
	require.NoError(t, err)

	_, err = logic.Register(&pb.RegisterReq{
		Username: "dup_user", Password: "Strong@123",
	})
	require.Error(t, err)
}
```

- [ ] **Step 3: 运行集成测试**

```bash
go test -tags=integration ./app/user/rpc/internal/logic/ -run "TestLoginIntegration|TestRegisterIntegration" -v
```

- [ ] **Step 4: Commit**

---

### Task 17: User RPC Model 集成测试

**Files:**
- Create: `app/user/rpc/internal/model/user_follow_model_integration_test.go`
- Create: `app/user/rpc/internal/model/user_profile_model_integration_test.go`

测试 `FindFollowers`, `FindFollowing`, `CountFollowers`, `CountFollowing`, `UpdateUserDes`。

由于 Plan 篇幅限制，此处不展开完整代码 — 模式与 `login_integration_test.go` 相同：
1. 文件头 `//go:build integration`
2. TestMain 调用 `testutil.SetupTestEnv` 加载 `xbh_user.sql`
3. 每个测试先 `truncateAll`，再 `seed` 数据，执行 Model 方法，验证结果

- [ ] **Step 1: 写 user_follow_model_integration_test.go**
- [ ] **Step 2: 写 user_profile_model_integration_test.go**
- [ ] **Step 3: 运行验证**

```bash
go test -tags=integration ./app/user/rpc/internal/model/ -v
```

- [ ] **Step 4: Commit**

---

### Task 18: Phase 4 — 迁移旧集成测试

**要迁移的文件（13 个）：**
- `app/content/rpc/internal/logic/integration_test.go`（TestMain）
- `app/content/rpc/internal/logic/post_integration_test.go`
- `app/content/rpc/internal/logic/comment_integration_test.go`
- `app/content/rpc/internal/logic/tag_integration_test.go`
- `app/content/rpc/internal/logic/get_user_posts_integration_test.go`
- `app/interaction/rpc/internal/logic/integration_test.go`（TestMain）
- `app/interaction/rpc/internal/logic/interaction_integration_test.go`
- `app/media/rpc/internal/logic/integration_test.go`（TestMain）
- `app/media/rpc/internal/logic/batch_get_media_integration_test.go`
- `app/media/rpc/internal/logic/delete_media_integration_test.go`
- `app/media/rpc/internal/logic/get_media_integration_test.go`
- `app/media/rpc/internal/logic/upload_image_integration_test.go`
- `app/media/rpc/internal/logic/upload_video_integration_test.go`

> Feed 和 Message model 集成测试已使用 testcontainers，无需迁移。

**每个 TestMain 的改动模式：**

替换 `os.Getenv("TEST_MYSQL_DSN")` + 手动连接逻辑 为：

```go
var testEnv *testutil.TestEnv

func TestMain(m *testing.M) {
    testEnv = testutil.SetupTestEnv(m, testutil.SchemaPath("xbh_xxx.sql"))
    // build testSvcCtx...
    code := m.Run()
    testEnv.Close()
    os.Exit(code)
}
```

**每个测试文件额外改动：**
- 删除私有 `getEnv()` 和 `truncateAll()` 辅助函数，改用 `testEnv.TruncateAll()`
- `seedPost()` 等写入用 `testEnv.DB.ExecContext()`

- [ ] **Step 1: 迁移 content**
- [ ] **Step 2: 迁移 interaction**
- [ ] **Step 3: 迁移 media**
- [ ] **Step 4: 全部集成测试回归**

```bash
go test -tags=integration ./app/content/rpc/internal/logic/ ./app/interaction/rpc/internal/logic/ ./app/media/rpc/internal/logic/ -v
```

---

### Task 19: 最终验证

- [ ] **Step 1: 单元测试全部通过**

```bash
go test -race -cover ./...
```

- [ ] **Step 2: 集成测试全部通过**

```bash
go test -race -tags=integration ./...
```

- [ ] **Step 3: 覆盖率报告**

```bash
go test -cover ./app/user/rpc/internal/logic/ ./app/gateway/internal/logic/login/
```

Expected: User RPC logic ≥80%, Gateway login ≥80%
