# 小白盒内容社区 — Go 微服务技术方案

## 一、项目总览

### 1.1 项目定位

与 Java 方案业务需求完全一致：类似"小黑盒"的游戏内容社区平台。本文档聚焦 Go 语言微服务技术栈的选型、架构设计和 Go 特有的技术亮点。

搜索（多路召回+精排）、推荐（多阶段漏斗）、Feed 流（推拉结合）、评论模型、数据库表结构等业务设计与 Java 方案一致，详见 [java-microservices-plan.md](./java-microservices-plan.md)。

### 1.2 微服务框架选型分析

| 框架 | Stars | 特点 | 适用场景 | 学习曲线 |
|------|-------|------|---------|---------|
| **go-zero** | 29k+ | 内置代码生成(goctl)、自带API/RPC框架、内置熔断限流缓存、一站式解决方案 | 快速开发、中小团队、全功能需求 | 中等 |
| **Kratos** | 23k+ | B站开源、插件化设计、Wire依赖注入、protobuf驱动 | 大型项目、灵活定制、微服务治理 | 较高 |
| **go-micro** | 21k+ | 插件化、多种注册中心、多种传输协议 | 灵活组合、协议多样 | 较高 |
| **自建 (Gin+gRPC)** | - | 完全控制、灵活组合 | 经验丰富团队 | 高 |

**推荐选型：go-zero**

理由：
1. **代码生成效率高**：goctl 从 API/Proto 文件自动生成 handler/logic/types/client，个人项目开发效率关键
2. **内置完整解决方案**：限流、熔断、缓存、负载均衡开箱即用，不需要额外集成 Sentinel 等
3. **自带 API 网关能力**：API 服务天然充当网关，无需单独部署 Gateway
4. **社区活跃**：中文文档完善，对实习面试友好
5. **性能出色**：底层基于 fasthttp 思想优化，QPS 远高于 Spring Cloud

### 1.3 完整技术栈

| 层级 | 技术选型 | 版本 | 说明 |
|------|---------|------|------|
| **语言** | Go | 1.22+ | 最新稳定版 |
| **微服务框架** | go-zero | 1.6+ | API + RPC 一体化框架 |
| **前端** | Flutter 3.x + Dart | - | 跨平台（与 Java 方案共用） |
| **RPC** | gRPC + protobuf | v3 | 服务间同步通信 |
| **API 协议** | RESTful (go-zero API) | - | 面向客户端 |
| **ORM** | GORM 2.x | - | 成熟稳定，生态丰富 |
| **服务注册/发现** | etcd | 3.5+ | go-zero 原生支持，Go 生态首选 |
| **配置中心** | etcd + go-zero 内置 | - | 配置热更新 |
| **消息队列** | RocketMQ 5.x | - | Go SDK (rocketmq-client-go) |
| **数据库** | MySQL 8.0 | - | 每个服务独立 Schema |
| **缓存** | Redis 7.x | - | go-zero 内置 cache 组件 |
| **搜索引擎** | Elasticsearch 8.x | - | olivere/elastic 或 official go-elasticsearch |
| **向量数据库** | Milvus 2.x | - | milvus-sdk-go |
| **对象存储** | MinIO / 阿里云 OSS | - | minio-go SDK |
| **分布式事务** | DTM | 1.17+ | Go 原生分布式事务框架 |
| **链路追踪** | Jaeger + OpenTelemetry | - | go-zero 原生集成 OTEL |
| **日志** | zap + Loki + Grafana | - | 结构化日志 + 集中收集 |
| **监控** | Prometheus + Grafana | - | go-zero 内置 metrics 暴露 |
| **依赖注入** | Wire | - | Google 编译期依赖注入 |
| **任务调度** | go-zero 内置 cron / XXL-JOB | - | 定时任务 |
| **接口文档** | goctl 生成 Swagger | - | 自动生成 |
| **容器化** | Docker + Docker Compose | - | 容器编排 |
| **代码生成** | goctl | - | 从 .api/.proto 生成代码骨架 |

### 1.4 关键选型对比（与 Java 方案差异）

| 维度 | Java 方案 | Go 方案 | 选型理由 |
|------|----------|---------|---------|
| 服务注册 | Nacos | **etcd** | go-zero 原生支持，Go 生态标准（K8s 也用 etcd） |
| API 网关 | Spring Cloud Gateway | **go-zero API 服务** | go-zero API 天然支持路由、中间件、限流，无需单独网关 |
| 同步通信 | OpenFeign (HTTP) | **gRPC (protobuf)** | 二进制协议，性能高 10x，强类型契约 |
| ORM | MyBatis-Plus | **GORM** | Go 生态最成熟的 ORM |
| 分布式事务 | Seata | **DTM** | Go 原生，支持 Saga/TCC/XA，与 gRPC 集成好 |
| 链路追踪 | SkyWalking | **Jaeger + OTEL** | go-zero 内置 OpenTelemetry，Jaeger 是 CNCF 标准 |
| 日志收集 | ELK | **Loki + Grafana** | 更轻量，与 Prometheus/Grafana 统一监控栈 |
| 依赖注入 | Spring IoC | **Wire** | 编译期注入，零运行时开销 |
| 代码生成 | 手写 | **goctl** | 从 API/Proto 文件自动生成 80% 样板代码 |

---

## 二、项目结构（Monorepo）

### 2.1 整体目录布局

```
xbh-community-go/
├── go.work                              (Go workspace)
├── go.work.sum
├── deploy/
│   ├── docker-compose.yml               (应用服务)
│   ├── docker-compose.middleware.yml     (中间件)
│   ├── k8s/
│   └── sql/
│       ├── xbh_user.sql
│       ├── xbh_content.sql
│       ├── xbh_interaction.sql
│       ├── xbh_message.sql
│       ├── xbh_feed.sql
│       └── xbh_media.sql
├── docs/
├── proto/                               (protobuf 定义，所有服务共享)
│   ├── user/
│   │   └── user.proto
│   ├── content/
│   │   └── content.proto
│   ├── interaction/
│   │   └── interaction.proto
│   ├── search/
│   │   └── search.proto
│   ├── recommend/
│   │   └── recommend.proto
│   ├── feed/
│   │   └── feed.proto
│   ├── message/
│   │   └── message.proto
│   └── media/
│       └── media.proto
├── pkg/                                 (公共库，所有服务共享)
│   ├── result/                          (统一响应 Result[T])
│   │   └── response.go
│   ├── errx/                            (业务错误码定义)
│   │   ├── codes.go
│   │   └── errors.go
│   ├── jwtx/                            (JWT 工具)
│   │   └── jwt.go
│   ├── interceptor/                     (gRPC 拦截器)
│   │   ├── auth.go
│   │   ├── logging.go
│   │   └── recovery.go
│   ├── middleware/                       (HTTP 中间件)
│   │   ├── auth.go
│   │   └── cors.go
│   ├── cachex/                          (缓存工具)
│   │   └── keys.go
│   ├── mqx/                             (RocketMQ 封装)
│   │   ├── producer.go
│   │   ├── consumer.go
│   │   └── topics.go
│   ├── esx/                             (Elasticsearch 封装)
│   │   └── client.go
│   ├── milvusx/                         (Milvus 封装)
│   │   └── client.go
│   └── util/                            (通用工具)
│       ├── snowflake.go                 (分布式 ID)
│       ├── hash.go
│       └── time.go
├── app/                                 (微服务目录)
│   ├── gateway/                         (API 网关/BFF)
│   │   ├── gateway.api                  (go-zero API 定义)
│   │   ├── gateway.go                   (入口)
│   │   ├── etc/
│   │   │   └── gateway.yaml             (配置)
│   │   └── internal/
│   │       ├── config/
│   │       ├── handler/                 (路由处理)
│   │       ├── logic/                   (业务逻辑，聚合 RPC 调用)
│   │       ├── svc/                     (服务依赖上下文)
│   │       ├── middleware/
│   │       └── types/                   (请求/响应结构体)
│   ├── user/
│   │   ├── rpc/                         (gRPC 服务)
│   │   │   ├── user.go
│   │   │   ├── etc/user.yaml
│   │   │   ├── internal/
│   │   │   │   ├── config/
│   │   │   │   ├── server/              (gRPC server 实现)
│   │   │   │   ├── logic/               (业务逻辑)
│   │   │   │   ├── svc/                 (ServiceContext)
│   │   │   │   ├── model/              (GORM 模型)
│   │   │   │   └── repo/               (数据访问层)
│   │   │   └── pb/                      (生成的 protobuf Go 代码)
│   │   └── api/                         (可选：独立 HTTP 接口)
│   ├── content/
│   │   └── rpc/
│   ├── interaction/
│   │   └── rpc/
│   ├── search/
│   │   └── rpc/
│   ├── recommend/
│   │   └── rpc/
│   ├── feed/
│   │   └── rpc/
│   ├── message/
│   │   └── rpc/
│   ├── media/
│   │   └── rpc/
│   └── mq/                             (MQ 消费者，独立进程)
│       ├── search-consumer/             (搜索索引同步)
│       ├── recommend-consumer/          (行为事件消费)
│       ├── feed-consumer/               (Feed 写扩散)
│       ├── message-consumer/            (通知生成)
│       └── media-consumer/              (媒体处理任务)
└── xbh-flutter-app/                     (Flutter 客户端)
```

### 2.2 单个微服务内部结构（以 user 为例）

```
app/user/rpc/
├── user.go                    (main 入口)
├── etc/
│   └── user.yaml              (配置：MySQL/Redis/etcd 地址)
├── internal/
│   ├── config/
│   │   └── config.go          (配置结构体)
│   ├── server/
│   │   └── user_server.go     (gRPC server，调用 logic)
│   ├── logic/                 (核心业务逻辑)
│   │   ├── get_user_logic.go
│   │   ├── update_profile_logic.go
│   │   ├── follow_logic.go
│   │   └── get_followers_logic.go
│   ├── svc/
│   │   └── service_context.go (依赖注入：DB/Redis/MQ/RPC客户端)
│   ├── model/                 (GORM 模型定义)
│   │   ├── user_profile.go
│   │   ├── user_follow.go
│   │   └── user_tag.go
│   └── repo/                  (数据访问层，隔离 DB 操作)
│       ├── user_repo.go
│       ├── follow_repo.go
│       └── tag_repo.go
└── pb/                        (protobuf 生成代码)
    ├── user.pb.go
    └── user_grpc.pb.go
```

### 2.3 架构分层

```
┌─────────────────────────────────────────────┐
│          Gateway (go-zero API)               │
│    HTTP 路由 → 聚合多个 RPC → 返回 JSON       │
├─────────────────────────────────────────────┤
│          Handler 层 (自动生成)                │
│    参数绑定、校验、调用 Logic                  │
├─────────────────────────────────────────────┤
│          Logic 层 (核心业务)                  │
│    业务编排、缓存操作、MQ 发送                 │
├─────────────────────────────────────────────┤
│          Repo 层 (数据访问)                   │
│    GORM 操作、SQL 查询、数据转换              │
├─────────────────────────────────────────────┤
│          Infrastructure                      │
│    MySQL / Redis / ES / Milvus / RocketMQ    │
└─────────────────────────────────────────────┘
```

**与 Java 分层的映射：**

| Java | Go (go-zero) | 说明 |
|------|-------------|------|
| Controller | Handler (自动生成) | 参数绑定、路由 |
| Service | Logic | 业务逻辑 |
| Service Interface | Proto 定义 | 接口契约 |
| Mapper/Repository | Repo + GORM Model | 数据访问 |
| DTO/VO | Types (自动生成) + PB | 数据传输对象 |
| Config | etc/*.yaml + config.go | 配置 |
| Spring IoC | ServiceContext (svc/) | 依赖注入容器 |

---

## 三、微服务清单

### 3.1 服务列表

| 服务 | 类型 | 端口 | 职责 |
|------|------|------|------|
| **gateway** | API (HTTP) | 8080 | 路由聚合、JWT 校验、限流 |
| **user-rpc** | RPC (gRPC) | 9001 | 用户资料、关注/粉丝、画像 |
| **content-rpc** | RPC (gRPC) | 9002 | 帖子 CRUD、评论、标签 |
| **interaction-rpc** | RPC (gRPC) | 9003 | 点赞、收藏、浏览记录 |
| **search-rpc** | RPC (gRPC) | 9004 | 多路召回、精排、热搜 |
| **recommend-rpc** | RPC (gRPC) | 9005 | 漏斗推荐、冷启动 |
| **feed-rpc** | RPC (gRPC) | 9006 | 关注流推拉结合 |
| **message-rpc** | RPC (gRPC) | 9007 | 通知、未读计数、WebSocket |
| **media-rpc** | RPC (gRPC) | 9008 | 上传、压缩、转码 |
| **mq-consumers** | Worker | - | 5 个独立 MQ 消费者进程 |

### 3.2 Gateway 设计（BFF 模式）

Go 方案中 Gateway 不是简单的反向代理，而是 **BFF (Backend For Frontend)** 层：

```
Flutter App
     │ HTTPS (JSON)
     ▼
┌─────────────────────────────────────┐
│          Gateway (go-zero API)       │
│                                     │
│  /api/v1/post/{id}                  │
│    → content-rpc.GetPost()          │
│    → user-rpc.GetUser()             │
│    → interaction-rpc.GetCounts()    │
│    → 聚合返回 JSON                   │
│                                     │
│  中间件链：                           │
│    CORS → RateLimit → JWT → Handler │
└─────────────────────────────────────┘
     │ gRPC (protobuf)
     ▼
  各 RPC 微服务
```

优势：
- 客户端只调一次 HTTP，Gateway 并发调多个 RPC 聚合
- 利用 goroutine 并发调用，延迟取决于最慢的 RPC
- 避免客户端多次请求

---

## 四、服务间通信

### 4.1 同步通信（gRPC）

**Proto 定义示例（user.proto）：**

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

**调用关系（与 Java 方案一致）：**

| 调用方 | 被调用方 | 协议 |
|--------|---------|------|
| Gateway | 所有 RPC | gRPC |
| Content | User | gRPC |
| Feed | User | gRPC |
| Search | User | gRPC |
| Interaction | Content | gRPC |

### 4.2 异步通信（RocketMQ）

Go SDK：`apache/rocketmq-client-go/v2`

**Topic 定义（与 Java 方案一致）：**

```go
// pkg/mqx/topics.go
const (
    TopicPostPublish   = "POST_PUBLISH"    // 帖子发布/更新/删除
    TopicUserAction    = "USER_ACTION"     // 点赞/收藏/浏览
    TopicUserFollow    = "USER_FOLLOW"     // 关注/取关
    TopicCommentCreate = "COMMENT_CREATE"  // 新评论
    TopicMediaProcess  = "MEDIA_PROCESS"   // 媒体处理完成
)

const (
    TagPostCreate = "create"
    TagPostUpdate = "update"
    TagPostDelete = "delete"
)
```

**MQ 消费者独立部署**（区别于 Java 方案）：

Go 方案将 MQ 消费者从 RPC 服务中分离为独立进程（`app/mq/`），原因：
1. 消费者可独立扩缩容，不影响 RPC 服务
2. 避免 RPC 服务内部逻辑复杂
3. 消费者崩溃不影响在线服务

### 4.3 分布式事务（DTM）

**选 DTM 而非 Seata-Go 的理由：**
- DTM 是 Go 原生项目，与 gRPC 天然集成
- 支持 Saga / TCC / XA / 二阶段消息
- 内置 RocketMQ 事务消息的替代方案（二阶段消息）
- Seata-Go 仍不成熟，生态远不如 Java 版

**事务方案（与 Java 映射）：**

| 场景 | Java 方案 | Go 方案 |
|------|----------|---------|
| 帖子发布 + 索引同步 | RocketMQ 事务消息 | **DTM 二阶段消息** |
| 点赞 + 计数 | RocketMQ 事务消息 | **DTM 二阶段消息** |
| 注册 + 初始化资料 | Seata AT | **DTM Saga** |

**DTM 二阶段消息示例：**

```go
// 帖子发布：本地写库 + 异步同步搜索索引
msg := dtmgrpc.NewMsgGrpc(dtmServer, gid).
    Add(searchRpcBusi+"/SyncPostIndex", &pb.SyncPostReq{PostId: postId}).
    Add(feedRpcBusi+"/FanoutPost", &pb.FanoutReq{PostId: postId, AuthorId: authorId})

msg.DoAndSubmitDB(dtmServer, func(tx *sql.Tx) error {
    // 本地事务：写入帖子表
    return contentRepo.CreatePostTx(tx, post)
})
```

---

## 五、Go 特有技术亮点

### 5.1 goroutine 并发 — 搜索多路召回

搜索多路召回是 Go 并发模型的最佳展示场景：

```go
func (l *SearchLogic) MultiRecall(ctx context.Context, query string, userId int64) ([]*RecallItem, error) {
    g, ctx := errgroup.WithContext(ctx)

    var (
        esItems      []*RecallItem
        vectorItems  []*RecallItem
        hotItems     []*RecallItem
        tagItems     []*RecallItem
        personalItems []*RecallItem
    )

    // 5 路召回并行执行，每个 goroutine 独立超时控制
    g.Go(func() error {
        var err error
        esItems, err = l.esRecall(ctx, query, 200)
        return err
    })
    g.Go(func() error {
        var err error
        vectorItems, err = l.vectorRecall(ctx, query, 200)
        return err
    })
    g.Go(func() error {
        var err error
        hotItems, err = l.hotRecall(ctx, 50)
        return err
    })
    g.Go(func() error {
        var err error
        tagItems, err = l.tagRecall(ctx, query, 100)
        return err
    })
    g.Go(func() error {
        if userId == 0 {
            return nil // 未登录跳过个性化召回
        }
        var err error
        personalItems, err = l.personalRecall(ctx, userId, 100)
        return err
    })

    if err := g.Wait(); err != nil {
        // 部分召回失败时降级：用已成功的结果继续
        logx.Errorf("partial recall failed: %v", err)
    }

    return l.mergeAndDedup(esItems, vectorItems, hotItems, tagItems, personalItems), nil
}
```

**与 Java 对比：**
- Java 用 `CompletableFuture` + 线程池，需要管理线程数量
- Go 用 `errgroup` + goroutine，零成本创建，无需线程池
- goroutine 初始栈仅 2KB（vs Java 线程 1MB），可轻松创建数万并发

### 5.2 channel Pipeline — 推荐漏斗

推荐系统的四层漏斗天然适合 Go 的 Pipeline 模式：

```go
func (l *RecommendLogic) GetFeed(ctx context.Context, userId int64, size int) ([]*FeedItem, error) {
    // Stage 1: 多路召回 → channel
    recallCh := l.recall(ctx, userId)        // 输出 ~5000 候选

    // Stage 2: 粗排 → channel
    roughCh := l.roughRank(ctx, recallCh)    // 筛选 ~500

    // Stage 3: 精排 → channel
    fineCh := l.fineRank(ctx, userId, roughCh)  // 筛选 ~50

    // Stage 4: 重排（消费最终结果）
    return l.rerank(ctx, userId, fineCh, size)   // 输出 20
}

// 召回层：多路并行，结果汇入同一个 channel
func (l *RecommendLogic) recall(ctx context.Context, userId int64) <-chan *RecallItem {
    out := make(chan *RecallItem, 1000)

    go func() {
        defer close(out)
        g, ctx := errgroup.WithContext(ctx)

        channels := make([]<-chan *RecallItem, 0, 5)
        // 每路召回输出到独立 channel
        channels = append(channels, l.cfRecall(ctx, userId))
        channels = append(channels, l.contentRecall(ctx, userId))
        channels = append(channels, l.hotRecall(ctx))
        channels = append(channels, l.followChainRecall(ctx, userId))
        channels = append(channels, l.newContentPool(ctx))

        // Fan-in: 合并所有 channel 到 out
        for _, ch := range channels {
            ch := ch
            g.Go(func() error {
                for item := range ch {
                    select {
                    case out <- item:
                    case <-ctx.Done():
                        return ctx.Err()
                    }
                }
                return nil
            })
        }
        g.Wait()
    }()

    return out
}
```

### 5.3 context 贯穿链路追踪与超时

```go
// 每个请求入口设置全局超时
func (l *SearchLogic) Search(ctx context.Context, req *pb.SearchReq) (*pb.SearchResp, error) {
    // Gateway 已注入 trace span，这里自动传播
    ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
    defer cancel()

    // 召回阶段限时 100ms
    recallCtx, recallCancel := context.WithTimeout(ctx, 100*time.Millisecond)
    defer recallCancel()
    candidates := l.multiRecall(recallCtx, req.Query, req.UserId)

    // 精排阶段用剩余时间
    ranked := l.rank(ctx, candidates)

    return l.buildResp(ranked), nil
}
```

**context 在 Go 微服务中的作用：**
1. **超时控制**：每层漏斗独立超时，避免一层慢拖垮整个请求
2. **链路追踪**：OpenTelemetry span 通过 context 自动传播
3. **取消传播**：上游取消自动级联到所有下游 goroutine
4. **元数据传递**：用户 ID、TraceID 等通过 context 传递

### 5.4 Go interface 组合设计

```go
// 各召回通道实现统一接口
type Recaller interface {
    Recall(ctx context.Context, query string, limit int) ([]*RecallItem, error)
    Name() string
}

// 各精排策略实现统一接口
type Ranker interface {
    Score(ctx context.Context, item *RankItem) float64
}

// 组合多个 Ranker 为加权融合
type WeightedRanker struct {
    rankers []struct {
        ranker Ranker
        weight float64
    }
}

func (w *WeightedRanker) Score(ctx context.Context, item *RankItem) float64 {
    var score float64
    for _, r := range w.rankers {
        score += r.weight * r.ranker.Score(ctx, item)
    }
    return score
}
```

### 5.5 错误处理策略

```go
// pkg/errx/errors.go — 业务错误码
type CodeError struct {
    Code int    `json:"code"`
    Msg  string `json:"msg"`
}

func (e *CodeError) Error() string { return e.Msg }

// 预定义错误
var (
    ErrUserNotFound    = NewCodeError(10001, "用户不存在")
    ErrPostNotFound    = NewCodeError(20001, "帖子不存在")
    ErrAlreadyLiked    = NewCodeError(30001, "已经点赞过了")
    ErrUnauthorized    = NewCodeError(40001, "未登录")
    ErrRateLimited     = NewCodeError(40002, "操作过于频繁")
)

// gRPC 拦截器统一转换错误
func ErrorInterceptor(ctx context.Context, req interface{},
    info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {

    resp, err := handler(ctx, req)
    if err != nil {
        var codeErr *errx.CodeError
        if errors.As(err, &codeErr) {
            // 业务错误 → gRPC status with details
            return nil, status.Error(codes.Code(codeErr.Code), codeErr.Msg)
        }
        // 未知错误 → 记日志，返回通用错误
        logx.WithContext(ctx).Errorf("unexpected: %+v", err)
        return nil, status.Error(codes.Internal, "服务内部错误")
    }
    return resp, nil
}
```

### 5.6 Wire 依赖注入

```go
// wire.go — 编译期依赖注入
//go:build wireinject

func InitServiceContext(c config.Config) *svc.ServiceContext {
    wire.Build(
        repo.NewUserRepo,
        repo.NewFollowRepo,
        repo.NewTagRepo,
        svc.NewServiceContext,
        // 数据库、Redis、MQ 客户端由 config 驱动
        providerSet,
    )
    return nil
}
```

---

## 六、代码生成工作流

go-zero 的 goctl 是个人开发效率的关键：

### 6.1 API → Handler/Logic/Types

```bash
# 从 .api 文件生成 HTTP 服务代码
goctl api go -api gateway.api -dir . -style goZero
```

自动生成：`handler/`（路由绑定）、`logic/`（业务骨架）、`types/`（请求/响应结构体）

### 6.2 Proto → gRPC Server/Client

```bash
# 从 .proto 生成 gRPC 服务代码
goctl rpc protoc user.proto --go_out=./pb --go-grpc_out=./pb --zrpc_out=. -style goZero
```

自动生成：`pb/`（protobuf Go 代码）、`server/`（gRPC 实现骨架）、`logic/`（业务骨架）

### 6.3 SQL → GORM Model

```bash
# 使用 GORM gen 从数据库表生成模型
# 或手动定义 GORM 模型（推荐，更可控）
```

### 6.4 开发流程

```
1. 定义 .proto 文件（接口契约）
2. goctl 生成代码骨架
3. 填写 logic 层业务逻辑（只需写这一层）
4. 填写 repo 层数据访问
5. 单元测试
6. 重复
```

**代码量估算**：goctl 自动生成约 60-70% 的样板代码，开发者只需专注 logic 和 repo。

---

## 七、基础设施

### 7.1 Docker Compose 中间件

```yaml
# deploy/docker-compose.middleware.yml
services:
  mysql:
    image: mysql:8.0
    ports: ["3306:3306"]

  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]

  etcd:
    image: bitnami/etcd:3.5
    ports: ["2379:2379"]
    environment:
      ALLOW_NONE_AUTHENTICATION: "yes"

  rocketmq-namesrv:
    image: apache/rocketmq:5.1.3
    command: sh mqnamesrv
    ports: ["9876:9876"]

  rocketmq-broker:
    image: apache/rocketmq:5.1.3
    command: sh mqbroker -n namesrv:9876
    ports: ["10911:10911"]

  elasticsearch:
    image: elasticsearch:8.11.0
    ports: ["9200:9200"]
    environment:
      discovery.type: single-node
      xpack.security.enabled: "false"

  milvus:
    image: milvusdb/milvus:v2.3.3
    ports: ["19530:19530"]

  minio:
    image: minio/minio
    ports: ["9000:9000", "9001:9001"]
    command: server /data --console-address ":9001"

  dtm:
    image: yedf/dtm:latest
    ports: ["36789:36789", "36790:36790"]

  jaeger:
    image: jaegertracing/all-in-one:1.51
    ports: ["16686:16686", "4317:4317"]

  prometheus:
    image: prom/prometheus
    ports: ["9090:9090"]

  grafana:
    image: grafana/grafana
    ports: ["3000:3000"]

  loki:
    image: grafana/loki
    ports: ["3100:3100"]
```

### 7.2 可观测性对比

| 维度 | Java 方案 | Go 方案 |
|------|----------|---------|
| 链路追踪 | SkyWalking | **Jaeger + OpenTelemetry** (go-zero 内置) |
| 日志 | ELK (重量级) | **Loki + Grafana** (轻量级) |
| 指标 | Prometheus + Grafana | Prometheus + Grafana (一致) |
| 统一面板 | 三套独立 UI | **Grafana 统一** (Jaeger/Loki/Prometheus 都接入) |

Go 方案的监控栈更统一：Grafana 作为唯一 Dashboard，集成 Jaeger（追踪）+ Loki（日志）+ Prometheus（指标）。

---

## 八、与 Java 方案对比

### 8.1 性能对比

| 指标 | Java (Spring Cloud) | Go (go-zero) | 差距 |
|------|-------------------|-------------|------|
| 单服务启动时间 | 8-15 秒 | **< 1 秒** | 10x+ |
| 内存占用（空载） | 200-400 MB | **10-30 MB** | 10x+ |
| 内存占用（带负载） | 500-1000 MB | **50-100 MB** | 5-10x |
| HTTP QPS (单核) | ~8,000 | **~30,000** | 3-4x |
| gRPC QPS (单核) | ~15,000 | **~50,000** | 3x |
| Docker 镜像大小 | 200-500 MB (JRE) | **10-20 MB** (静态编译) | 20x |
| 10 个服务总内存 | 4-8 GB | **0.5-1 GB** | 5-8x |
| 并发模型 | 线程池 (1MB/线程) | **goroutine (2KB/协程)** | 500x 单位成本 |

### 8.2 开发效率对比

| 维度 | Java | Go | 说明 |
|------|------|-----|------|
| 样板代码 | 多（注解驱动但模板代码多） | **少（goctl 生成 70%）** | Go 代码生成更直接 |
| 编译速度 | 慢（Maven 全量编译） | **快（增量编译秒级）** | Go 编译器设计目标就是快 |
| 调试体验 | 优秀（IDE 支持完善） | 良好（Delve 调试器） | Java IDE 生态更成熟 |
| 框架文档 | 丰富（Spring 生态多年积累） | 良好（go-zero 中文文档完善） | Java 略胜 |
| 第三方库 | 极丰富 | 够用 | Java 生态更丰富 |
| 错误处理 | try-catch（一次捕获多异常） | **if err != nil（显式处理）** | Go 更啰嗦但更明确 |
| 泛型支持 | 成熟 | 基本可用 (Go 1.18+) | Java 更成熟 |
| ORM 体验 | MyBatis-Plus (丝滑) | GORM (够用) | Java 更好 |

### 8.3 生态对比

| 维度 | Java | Go | 胜出 |
|------|------|-----|------|
| 微服务框架 | Spring Cloud (一统天下) | go-zero/Kratos (群雄并起) | Java |
| 中间件 SDK | 几乎所有中间件官方首先支持 Java | 主流中间件有 Go SDK | Java |
| 监控工具 | SkyWalking、Sentinel 等原生支持 | OTEL 生态、go-zero 内置 | 平手 |
| 云原生 | 支持良好 | **天然适配 K8s (Docker/etcd)** | Go |
| 容器化 | JRE 镜像大 | **静态编译、镜像极小** | Go |
| 社区求助 | 中文资料极丰富 | 中文资料增长快 | Java |

### 8.4 适用场景对比

| 场景 | 推荐 | 理由 |
|------|------|------|
| Java 后端实习面试 | **Java** | 面试题库围绕 JVM/Spring/MyBatis |
| Go 后端面试 | **Go** | 展示 goroutine/channel/云原生能力 |
| 个人项目（资源有限） | **Go** | 10 个服务只需 1GB 内存，笔记本能跑 |
| 追求开发效率 | **Go** | goctl 代码生成 + 秒级编译 + 快速启动 |
| 追求生态完整性 | **Java** | Spring Cloud 全家桶最完善 |
| 高并发搜索/推荐 | **Go** | goroutine 并发模型更高效 |
| 云原生 / K8s 部署 | **Go** | 天然适配，镜像小、启动快 |

### 8.5 各自核心优势总结

**Java 方案优势：**
1. Spring Cloud 全家桶成熟稳定，生态最完善
2. 面试资料丰富，JVM/并发/Spring 是高频考点
3. MyBatis-Plus ORM 体验丝滑
4. SkyWalking/Sentinel 等国产框架原生支持
5. 企业级项目实战经验更易迁移

**Go 方案优势：**
1. 性能碾压：内存占用 1/10，QPS 3-4 倍，启动 1 秒
2. goroutine 并发模型天然适合搜索/推荐的并行计算
3. goctl 代码生成大幅减少样板代码
4. 云原生亲和：etcd/Docker/K8s 是 Go 生态核心
5. 单机可跑完整微服务集群（10 服务仅 1GB 内存）
6. 编译快、部署镜像小（20MB vs 500MB）

---

## 九、实施阶段划分

### 9.1 时间线

Go 方案因 goctl 代码生成和快速编译，开发效率更高，预计 **26 周（约 6.5 个月）**。

| 阶段 | 周期 | 内容 |
|------|------|------|
| **Phase 1** | 6 周 | 基座搭建 + Gateway/User/Content/Media + Flutter 骨架 |
| **Phase 2** | 5 周 | Interaction/Feed/Message + DTM 分布式事务 + 链路追踪 |
| **Phase 3** | 6 周 | 搜索系统（多路召回 goroutine 并行 + 精排 + 重排） |
| **Phase 4** | 5 周 | 推荐系统（channel Pipeline 漏斗 + 特征工程 + 冷启动） |
| **Phase 5** | 4 周 | 监控/部署/性能优化/安全加固 |

### 9.2 Phase 1 详细（6 周）

| 周次 | 任务 | 交付物 |
|------|------|--------|
| W1 | Go workspace 初始化、pkg 公共库、Docker Compose 中间件、Proto 定义 | 项目骨架可编译 |
| W2 | Gateway API 定义 + JWT 中间件 + 限流；User RPC（注册/登录/资料） | 认证流程跑通 |
| W3 | Content RPC（帖子 CRUD + 评论 + 标签）、goctl 生成代码 | 内容核心可用 |
| W4 | Media RPC（MinIO 上传 + 图片压缩）、Gateway 聚合接口 | 帖子可带图 |
| W5 | Flutter App 骨架 + 登录 + 首页列表 + 帖子详情 | App 核心流程 |
| W6 | 联调 + 单元测试 + Bug 修复 | Phase 1 交付 |

### 9.3 与 Java 方案工时对比

| 阶段 | Java | Go | 节省 | 原因 |
|------|------|-----|------|------|
| Phase 1 基座 | 8 周 | 6 周 | 2 周 | goctl 生成代码、无 XML 配置 |
| Phase 2 互动 | 6 周 | 5 周 | 1 周 | DTM 比 Seata 集成更简单 |
| Phase 3 搜索 | 8 周 | 6 周 | 2 周 | goroutine 并发代码更简洁 |
| Phase 4 推荐 | 6 周 | 5 周 | 1 周 | channel Pipeline 更自然 |
| Phase 5 运维 | 4 周 | 4 周 | 0 | 工作量相同 |
| **总计** | **32 周** | **26 周** | **6 周** | |

---

## 十、面试亮点（Go 特有）

### 10.1 Go 并发模型

| 亮点 | 面试话术 |
|------|---------|
| **goroutine 多路召回** | "搜索系统 5 路召回使用 errgroup 并行执行，每个 goroutine 独立超时控制。goroutine 栈仅 2KB，可轻松创建上万并发，无需像 Java 那样管理线程池大小" |
| **channel Pipeline 推荐漏斗** | "推荐系统四层漏斗用 channel 串联，每层通过 channel 流式传递数据，天然支持背压控制。Fan-in 模式合并多路召回结果" |
| **context 超时级联** | "每个 gRPC 请求设置 500ms 总超时，召回层 100ms，精排层 200ms，通过 context.WithTimeout 自动级联取消下游 goroutine" |

### 10.2 框架选型

| 亮点 | 面试话术 |
|------|---------|
| **go-zero 选型** | "选 go-zero 而非 Kratos：goctl 代码生成减少 70% 样板代码，内置限流/熔断/缓存开箱即用，API 服务天然充当 BFF 网关无需单独部署 Gateway" |
| **etcd vs Nacos** | "选 etcd 因为它是 Go 生态标准（K8s 底层就用 etcd），go-zero 原生支持，且 Raft 协议保证强一致性" |
| **DTM vs Seata** | "选 DTM：Go 原生项目，与 gRPC 天然集成，支持二阶段消息替代 RocketMQ 事务消息，API 更简洁" |

### 10.3 性能优势

| 亮点 | 面试话术 |
|------|---------|
| **资源效率** | "10 个微服务总内存占用约 500MB-1GB，而 Java 方案需要 4-8GB。个人开发在笔记本上就能跑完整集群" |
| **启动速度** | "Go 服务启动 < 1 秒，Java Spring Boot 需要 8-15 秒。在 K8s 弹性扩缩容场景下，Go 的快速启动意味着更好的自动伸缩响应" |
| **Docker 镜像** | "Go 静态编译后镜像仅 10-20MB（FROM scratch），Java 需要 JRE 基础镜像 200-500MB，部署拉取速度差 20 倍" |

### 10.4 工程实践

| 亮点 | 面试话术 |
|------|---------|
| **BFF Gateway** | "Gateway 不是简单反向代理，而是 BFF 层。一个 HTTP 请求进来，Gateway 用 goroutine 并发调用 3-4 个 RPC 聚合数据返回，减少客户端请求次数" |
| **MQ 消费者分离** | "MQ 消费者从 RPC 服务分离为独立进程，消费者可独立扩缩容，崩溃不影响在线服务 SLA" |
| **Proto 驱动开发** | "所有服务先定义 .proto 接口契约，goctl 生成代码骨架，开发者只填 logic 层。接口变更通过 proto 文件 diff 就能 review" |
| **Wire 编译期 DI** | "依赖注入用 Wire 在编译期生成代码，零运行时反射开销（对比 Spring 的运行时 IoC），且类型安全，缺少依赖编译期就报错" |

---

## 十一、风险评估

| 风险 | 等级 | Go 特有影响 | 缓解措施 |
|------|------|-----------|---------|
| RocketMQ Go SDK 成熟度不如 Java | HIGH | 部分高级特性(事务消息)可能有坑 | DTM 二阶段消息替代；降级为本地消息表 |
| Go ORM (GORM) 不如 MyBatis-Plus 灵活 | MEDIUM | 复杂查询需要写原生 SQL | 混用 GORM + sqlx 处理复杂查询 |
| Milvus Go SDK 文档少于 Python/Java | MEDIUM | 调试向量检索可能困难 | 参考官方 example，Python 原型验证后再 Go 实现 |
| Go 泛型不如 Java 成熟 | LOW | 统一响应封装稍繁琐 | 用 any + 类型断言，或 1.18+ 泛型 |
| Go 面试资料不如 Java 丰富 | MEDIUM | 准备面试需要自己总结 | 本方案已覆盖核心亮点 |

---

## 十二、成功标准

- [ ] 10 个微服务通过 etcd 注册发现，Gateway 正常路由
- [ ] 核心流程：注册 → 登录 → 发帖 → 评论 → 点赞 → 收藏
- [ ] 搜索多路召回（goroutine 并行）P99 < 100ms
- [ ] 推荐漏斗（channel Pipeline）输出个性化结果
- [ ] DTM 分布式事务正常工作
- [ ] Jaeger 链路追踪覆盖所有 gRPC 调用
- [ ] Flutter App 核心页面流畅
- [ ] 单元测试覆盖率 > 80%
- [ ] 10 个服务总内存 < 1GB
