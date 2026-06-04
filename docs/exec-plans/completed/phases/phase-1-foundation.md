# Phase 1: 基座搭建

## 概述

### 阶段目标
搭建完整的项目骨架，实现认证、内容、媒体核心功能，建立开发规范。

### 预计周期
6 周

### 前置条件
- Go 1.22+ 已安装
- Docker Desktop 已安装
- goctl 已安装
- protoc 已安装

---

## 详细任务清单

### W1: 项目初始化

#### 任务 1.1: 创建 Go Workspace
**涉及模块**: 根目录
**技术要点**:
```bash
# 初始化 Go workspace
go work init
go work use ./app/gateway
go work use ./app/user/rpc
go work use ./app/content/rpc
go work use ./app/media/rpc
```

**验收标准**:
- [ ] `go.work` 文件创建成功
- [ ] `go build ./...` 编译通过

---

#### 任务 1.2: 创建公共库 pkg/
**涉及模块**: `pkg/`

**目录结构**:
```
pkg/
├── result/
│   └── response.go        # 统一响应 Result[T]
├── errx/
│   ├── codes.go           # 业务错误码定义
│   └── errors.go          # 错误封装
├── jwtx/
│   └── jwt.go             # JWT 工具
├── interceptor/
│   ├── auth.go            # gRPC 认证拦截器
│   ├── logging.go         # 日志拦截器
│   └── recovery.go        # 异常恢复拦截器
├── middleware/
│   ├── auth.go            # HTTP 认证中间件
│   └── cors.go            # CORS 中间件
├── cachex/
│   └── keys.go            # 缓存 key 前缀定义
├── mqx/
│   ├── producer.go        # RocketMQ 生产者封装
│   ├── consumer.go        # RocketMQ 消费者封装
│   └── topics.go          # Topic 定义
├── esx/
│   └── client.go          # Elasticsearch 客户端封装
├── milvusx/
│   └── client.go          # Milvus 客户端封装
└── util/
    ├── snowflake.go       # 分布式 ID 生成
    ├── hash.go            # 哈希工具
    └── time.go            # 时间工具
```

**验收标准**:
- [ ] 所有公共库可独立编译
- [ ] 单元测试覆盖核心函数

---

#### 任务 1.3: 定义 Proto 文件
**涉及模块**: `proto/`

**Proto 文件清单**:
```
proto/
├── user/
│   └── user.proto
├── content/
│   └── content.proto
├── interaction/
│   └── interaction.proto
├── search/
│   └── search.proto
├── recommend/
│   └── recommend.proto
├── feed/
│   └── feed.proto
├── message/
│   └── message.proto
└── media/
    └── media.proto
```

**user.proto 示例**:
```protobuf
syntax = "proto3";
package user;
option go_package = "./pb";

service UserService {
  rpc GetUser(GetUserReq) returns (GetUserResp);
  rpc UpdateProfile(UpdateProfileReq) returns (UpdateProfileResp);
  rpc Follow(FollowReq) returns (FollowResp);
  rpc Unfollow(UnfollowReq) returns (UnfollowResp);
  rpc GetFollowers(GetFollowersReq) returns (GetFollowersResp);
  rpc GetFollowing(GetFollowingReq) returns (GetFollowingResp);
  rpc GetUserTags(GetUserTagsReq) returns (GetUserTagsResp);
  rpc BatchGetUsers(BatchGetUsersReq) returns (BatchGetUsersResp);
}

message GetUserReq {
  int64 user_id = 1;
}

message UserInfo {
  int64 id = 1;
  string username = 2;
  string nickname = 3;
  string avatar_url = 4;
  string bio = 5;
  int32 level = 6;
  int64 follower_count = 7;
  int64 following_count = 8;
}
```

**验收标准**:
- [ ] 所有 proto 文件通过 `protoc` 校验
- [ ] 生成 Go 代码无错误

---

#### 任务 1.4: 启动中间件
**涉及模块**: `deploy/docker-compose.middleware.yml`

**中间件清单**:

| 服务 | 端口 | 用途 |
|------|------|------|
| MySQL | 3306 | 数据库 |
| Redis | 6379 | 缓存 |
| etcd | 2379 | 服务注册/发现 |
| RocketMQ Namesrv | 9876 | 消息队列 |
| RocketMQ Broker | 10911 | 消息队列 |
| Elasticsearch | 9200 | 搜索引擎 |
| Milvus | 19530 | 向量数据库 |
| MinIO | 9000/9001 | 对象存储 |
| DTM | 36789/36790 | 分布式事务 |
| Jaeger | 16686/4317 | 链路追踪 |
| Prometheus | 9090 | 监控 |
| Grafana | 3000 | 可视化 |
| Loki | 3100 | 日志收集 |

**验收标准**:
- [ ] `docker-compose -f docker-compose.middleware.yml up -d` 成功
- [ ] 所有服务健康检查通过

---

#### 任务 1.5: 创建数据库 Schema
**涉及模块**: `deploy/sql/`

**SQL 文件清单**:
```
deploy/sql/
├── xbh_user.sql
├── xbh_content.sql
├── xbh_interaction.sql
├── xbh_message.sql
├── xbh_feed.sql
└── xbh_media.sql
```

**验收标准**:
- [ ] 所有 SQL 文件可执行
- [ ] 数据库表结构创建成功

---

### W2: Gateway + 认证

#### 任务 2.1: 创建 Gateway 服务
**涉及模块**: `app/gateway/`

**目录结构**:
```
app/gateway/
├── gateway.api              # API 定义
├── gateway.go               # 入口
├── etc/
│   └── gateway.yaml         # 配置
└── internal/
    ├── config/
    │   └── config.go
    ├── handler/             # 路由处理（自动生成）
    ├── logic/               # 业务逻辑
    ├── svc/                 # 服务依赖上下文
    ├── middleware/
    └── types/               # 请求/响应结构体
```

**生成命令**:
```bash
cd app/gateway
goctl api go -api gateway.api -dir . --style go_zero
```

**验收标准**:
- [ ] Gateway 服务可启动
- [ ] 健康检查端点 `/health` 可访问

---

#### 任务 2.2: 实现 JWT 认证
**涉及模块**: `pkg/jwtx/`, `app/gateway/internal/middleware/`

**技术要点**:
```go
// pkg/jwtx/jwt.go
type JwtConfig struct {
    AccessSecret string
    AccessExpire int64
}

func GenerateToken(userId int64, config JwtConfig) (string, error)
func ParseToken(tokenString string, config JwtConfig) (*Claims, error)
```

**Gateway JWT 配置**:
```yaml
# app/gateway/etc/gateway.yaml
Auth:
  AccessSecret: "your-secret-key"
  AccessExpire: 86400  # 24小时
```

**验收标准**:
- [ ] JWT token 生成正常
- [ ] JWT token 解析正常
- [ ] 无效 token 返回 401

---

#### 任务 2.3: 创建 User RPC 服务
**涉及模块**: `app/user/rpc/`

**生成命令**:
```bash
cd app/user/rpc
goctl rpc protoc ../../proto/user/user.proto --go_out=. --go-grpc_out=. --zrpc_out=. --style go_zero
```

**配置示例**:
```yaml
# app/user/rpc/etc/user.yaml
Name: user.rpc
ListenOn: 0.0.0.0:9001
Mode: pro

Etcd:
  Hosts:
    - 127.0.0.1:2379
  Key: user.rpc

MySQL:
  DataSource: "root:123456@tcp(localhost:3306)/xbh_user?parseTime=true"

Cache:
  - Host: localhost:6379
    Type: node

Telemetry:
  Name: user.rpc
  Endpoint: http://localhost:4318
  Sampler: 1.0
  Batcher: otlpgrpc
```

**验收标准**:
- [x] User RPC 服务启动成功
- [x] 服务注册到 etcd

---

#### 任务 2.4: 实现注册/登录
**涉及模块**: `app/user/rpc/internal/logic/`

**技术要点**:
- 手机号验证码登录
- 密码加密存储
- JWT token 返回

**验收标准**:
- [x] 注册接口正常
- [x] 登录接口返回有效 token
- [x] 密码正确加密存储

---

### W3: 内容服务

#### 任务 3.1: 创建 Content RPC 服务
**涉及模块**: `app/content/rpc/`

**生成命令**:
```bash
cd app/content/rpc
goctl rpc protoc ../../proto/content/content.proto --go_out=. --go-grpc_out=. --zrpc_out=. --style go_zero
```

**验收标准**:
- [x] Content RPC 服务启动成功
- [x] 服务注册到 etcd

---

#### 任务 3.2: 生成 Model（含缓存）
**涉及模块**: `app/content/rpc/internal/model/`

**生成命令**:
```bash
goctl model mysql ddl \
  -src deploy/sql/xbh_content.sql \
  -dir app/content/rpc/internal/model \
  -cache --style go_zero
```

**自动生成能力**:

| 方法 | 缓存行为 |
|------|---------|
| `FindOne(ctx, id)` | 自动读缓存 |
| `FindOneByXxx(ctx, val)` | 按唯一索引缓存 |
| `Update(ctx, data)` | 自动失效缓存 |
| `Delete(ctx, id)` | 自动失效缓存 |

**验收标准**:
- [x] Model 代码生成成功
- [ ] CRUD 操作缓存生效

---

#### 任务 3.3: 实现帖子 CRUD
**涉及模块**: `app/content/rpc/internal/logic/`

**功能列表**:
- 创建帖子
- 获取帖子详情
- 更新帖子
- 删除帖子
- 帖子列表（分页）

**验收标准**:
- [x] 所有 CRUD 接口正常
- [ ] 分页查询正常

---

#### 任务 3.4: 实现评论功能
**涉及模块**: `app/content/rpc/internal/logic/`

**功能列表**:
- 发表评论
- 评论回复
- 评论列表
- 删除评论

**验收标准**:
- [ ] 评论 CRUD 正常
- [ ] 嵌套回复正常

---

### W4: 媒体服务

#### 任务 4.1: 创建 Media RPC 服务
**涉及模块**: `app/media/rpc/`

**生成命令**:
```bash
cd app/media/rpc
goctl rpc protoc ../../proto/media/media.proto --go_out=. --go-grpc_out=. --zrpc_out=. --style go_zero
```

**验收标准**:
- [ ] Media RPC 服务启动成功

---

#### 任务 4.2: 集成 MinIO
**涉及模块**: `app/media/rpc/internal/svc/`

**MinIO SDK**:
```go
import "github.com/minio/minio-go/v7"

type ServiceContext struct {
    Config    config.Config
    MinioClient *minio.Client
}
```

**验收标准**:
- [ ] MinIO 连接成功
- [ ] Bucket 创建成功

---

#### 任务 4.3: 实现图片上传
**涉及模块**: `app/media/rpc/internal/logic/`

**功能列表**:
- 图片上传
- 图片压缩
- 图片 URL 生成

**技术要点**:
```go
// 图片压缩
func compressImage(src []byte, quality int) ([]byte, error)
```

**验收标准**:
- [ ] 图片上传成功
- [ ] 压缩后图片质量符合预期
- [ ] 返回可访问 URL

---

#### 任务 4.4: Gateway 聚合接口
**涉及模块**: `app/gateway/internal/logic/`

**聚合示例**:
```go
// 帖子详情聚合
func (l *PostDetailLogic) PostDetail(req *types.PostDetailReq) (*types.PostDetailResp, error) {
    // 并发调用多个 RPC
    g, ctx := errgroup.WithContext(l.ctx)

    var post *content.Post
    var author *user.UserInfo
    var counts *interaction.Counts

    g.Go(func() error {
        resp, err := l.svcCtx.ContentRpc.GetPost(ctx, &content.GetPostReq{Id: req.Id})
        post = resp
        return err
    })

    g.Go(func() error {
        resp, err := l.svcCtx.UserRpc.GetUser(ctx, &user.GetUserReq{UserId: post.AuthorId})
        author = resp
        return err
    })

    g.Go(func() error {
        resp, err := l.svcCtx.InteractionRpc.GetCounts(ctx, &interaction.GetCountsReq{TargetId: req.Id})
        counts = resp
        return err
    })

    if err := g.Wait(); err != nil {
        return nil, err
    }

    return &types.PostDetailResp{
        Id:           post.Id,
        Title:        post.Title,
        AuthorName:   author.Nickname,
        LikeCount:    counts.LikeCount,
        CommentCount: counts.CommentCount,
    }, nil
}
```

**验收标准**:
- [ ] 聚合接口返回完整数据
- [ ] 并发调用正常工作

---

### W5: Flutter 骨架

#### 任务 5.1: 创建 Flutter 项目
**涉及模块**: `xbh-flutter-app/`

**创建命令**:
```bash
flutter create xbh-flutter-app
```

**目录结构**:
```
xbh-flutter-app/
├── lib/
│   ├── main.dart
│   ├── app/
│   │   ├── app.dart
│   │   └── routes.dart
│   ├── pages/
│   │   ├── login/
│   │   ├── home/
│   │   ├── post_detail/
│   │   └── profile/
│   ├── services/
│   │   ├── api_client.dart
│   │   └── auth_service.dart
│   ├── models/
│   └── widgets/
├── pubspec.yaml
└── ...
```

**验收标准**:
- [ ] Flutter 项目创建成功
- [ ] 可在模拟器运行

---

#### 任务 5.2: 实现登录页面
**涉及模块**: `lib/pages/login/`

**功能**:
- 手机号输入
- 验证码输入
- 登录按钮
- Token 存储

**验收标准**:
- [ ] 登录流程完整
- [ ] Token 本地存储

---

#### 任务 5.3: 实现首页列表
**涉及模块**: `lib/pages/home/`

**功能**:
- 帖子列表展示
- 下拉刷新
- 上拉加载更多

**验收标准**:
- [ ] 列表数据正常显示
- [ ] 刷新/加载更多正常

---

#### 任务 5.4: 实现帖子详情
**涉及模块**: `lib/pages/post_detail/`

**功能**:
- 帖子内容展示
- 作者信息
- 互动按钮

**验收标准**:
- [ ] 详情页数据完整
- [ ] 交互按钮可点击

---

### W6: 集成测试

#### 任务 6.1: 编写单元测试
**涉及模块**: 所有服务

**测试覆盖目标**:
- pkg/ 公共库: 80%+
- logic/ 业务逻辑: 80%+

**验收标准**:
- [ ] 所有测试通过
- [ ] 覆盖率达标

---

#### 任务 6.2: 端到端测试
**涉及模块**: 全部

**测试场景**:
1. 注册 → 登录 → 获取用户信息
2. 发布帖子 → 获取帖子列表 → 获取帖子详情
3. 上传图片 → 发布带图帖子

**验收标准**:
- [ ] 所有场景通过
- [ ] Flutter App 流程顺畅

---

#### 任务 6.3: Bug 修复
**涉及模块**: 全部

**验收标准**:
- [ ] 无阻塞性 Bug
- [ ] 核心流程稳定

---

## 技术要点

### go-zero 配置模式

**RPC 服务配置**:
```go
type Config struct {
    zrpc.RpcServerConf  // 必须嵌入

    MySQL struct {
        DataSource string
    }
    Cache cache.CacheConf
}
```

**API 服务配置**:
```go
type Config struct {
    rest.RestConf  // 必须嵌入

    Auth struct {
        AccessSecret string
        AccessExpire int64
    }
    UserRpc zrpc.RpcClientConf
    // ...
}
```

### goctl 代码生成

| 命令 | 用途 |
|------|------|
| `goctl api go -api xxx.api -dir .` | 生成 API 服务 |
| `goctl rpc protoc xxx.proto --go_out=. --go-grpc_out=. --zrpc_out=.` | 生成 RPC 服务 |
| `goctl model mysql ddl -src xxx.sql -dir ./model -cache` | 生成 Model |

### ServiceContext 依赖管理

```go
type ServiceContext struct {
    Config      config.Config
    UserModel   model.UserProfileModel
    Cache       cache.Cache
}

func NewServiceContext(c config.Config) *ServiceContext {
    conn := sqlx.NewMysql(c.MySQL.DataSource)
    return &ServiceContext{
        Config:    c,
        UserModel: model.NewUserProfileModel(conn, c.Cache),
    }
}
```

---

## 依赖与风险

### 外部依赖
| 依赖 | 版本 | 用途 |
|------|------|------|
| Go | 1.22+ | 运行时 |
| goctl | latest | 代码生成 |
| protoc | 3.x | protobuf 编译 |
| Docker Desktop | latest | 容器运行 |

### 潜在风险

| 风险 | 等级 | 缓解措施 |
|------|------|---------|
| goctl 版本兼容性 | MEDIUM | 锁定 goctl 版本 |
| 中间件启动顺序 | LOW | 使用 depends_on 配置 |
| Flutter 环境配置 | MEDIUM | 提供详细环境配置文档 |

---

## 验收标准

### 功能验收
- [ ] Gateway 正常路由所有 API
- [ ] User RPC 注册/登录正常
- [ ] Content RPC 帖子 CRUD 正常
- [ ] Media RPC 图片上传正常
- [ ] Flutter App 核心流程跑通

### 性能验收
- [ ] Gateway 响应时间 < 100ms
- [ ] RPC 调用响应时间 < 50ms

### 测试验收
- [ ] 单元测试覆盖率 > 80%
- [ ] 端到端测试全部通过

### 文档验收
- [ ] API 文档生成成功（Swagger）
- [ ] 项目 README 更新

---

## 交付物清单

| 交付物 | 路径 |
|--------|------|
| 项目骨架 | 根目录 |
| 公共库 | `pkg/` |
| Gateway 服务 | `app/gateway/` |
| User RPC | `app/user/rpc/` |
| Content RPC | `app/content/rpc/` |
| Media RPC | `app/media/rpc/` |
| Flutter App 骨架 | `xbh-flutter-app/` |
| 中间件配置 | `deploy/docker-compose.middleware.yml` |
| 数据库 Schema | `deploy/sql/` |
| Proto 定义 | `proto/` |

---

## 下一步

Phase 1 完成后，进入 [Phase 2: 互动功能](phase-2-interaction.md)。
