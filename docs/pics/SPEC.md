# esx (little-white-box) 架构与流程规格说明

> 社交内容平台 · go-zero v1.10.1 · Go 1.26.1
>
> 本文以 **Mermaid** 图形化方式描述系统的主要流程图、时序图、数据流转图与协作图。
> 所有图均依据仓库实际代码（`app/`、`proto/`、`pkg/`、`deploy/`）绘制。

## 目录

1. [系统架构总览（组件图）](#1-系统架构总览组件图)
2. [服务协作图（Gateway → RPC）](#2-服务协作图gateway--rpc)
3. [统一请求生命周期（流程图）](#3-统一请求生命周期流程图)
4. [鉴权与 JWT（时序图 + 流程图）](#4-鉴权与-jwt时序图--流程图)
5. [用户注册 / 登录（时序图）](#5-用户注册--登录时序图)
6. [发帖流程 — DTM 二阶段消息 + Fanout（时序图）](#6-发帖流程--dtm-二阶段消息--fanout时序图)
7. [Feed 读写扩散（数据流转图）](#7-feed-读写扩散数据流转图)
8. [评论 / 点赞 / 收藏（时序图）](#8-评论--点赞--收藏时序图)
9. [媒体上传 — gRPC 流式 + S3（时序图）](#9-媒体上传--grpc-流式--s3时序图)
10. [媒体删除事件 — MQ 异步清理（数据流转图）](#10-媒体删除事件--mq-异步清理数据流转图)
11. [搜索索引（数据流转图）](#11-搜索索引数据流转图)
12. [推荐与行为日志管线（数据流转图）](#12-推荐与行为日志管线数据流转图)
13. [私信与通知（时序图）](#13-私信与通知时序图)
14. [事件总线全景（MQ 协作图）](#14-事件总线全景mq-协作图)
15. [错误码与统一响应（流程图）](#15-错误码与统一响应流程图)
16. [部署拓扑（组件图）](#16-部署拓扑组件图)
17. [整体数据流转全景图](#17-整体数据流转全景图)

---

## 1. 系统架构总览（组件图）

```mermaid
graph TB
    Client["客户端<br/>(Web / App)"]

    subgraph Edge["接入层"]
        GW["Gateway<br/>REST :8888<br/>app/gateway"]
    end

    subgraph RPC["RPC 服务层 (zrpc + etcd)"]
        USER["User RPC :9090<br/>用户/关注/认证"]
        CONTENT["Content RPC :8088<br/>帖子/评论/标签"]
        MEDIA["Media RPC :9008<br/>图片/视频流式上传"]
        INTER["Interaction RPC<br/>点赞/收藏/计数"]
        FEED["Feed RPC<br/>关注流/推荐流/Fanout"]
        MSG["Message RPC<br/>私信/通知"]
        SEARCH["Search RPC<br/>帖子/用户/标签搜索"]
        RECO["Recommend RPC<br/>推荐/相似/行为"]
    end

    subgraph MQConsumers["MQ 消费者 (RocketMQ)"]
        FEEDMQ["feed/mq<br/>post-create → fanout"]
        MEDIAMQ["media/mq<br/>media-deleted → S3 清理"]
        SEARCHMQ["search/mq<br/>search-index → ES"]
        RECOMQ["recommend/mq<br/>user-behavior → 画像"]
        MSGMQ["message/mq<br/>message-push → 通知"]
        BLOG["pipeline/behaviorlog<br/>行为聚合 → ClickHouse"]
    end

    subgraph Infra["基础设施"]
        MYSQL[("MySQL 8.0")]
        REDIS[("Redis 7")]
        ES[("Elasticsearch 8.8")]
        MILVUS[("Milvus")]
        S3[("MinIO / SeaweedFS")]
        CH[("ClickHouse")]
        DTM["DTM<br/>分布式事务"]
        ROCKET["RocketMQ 5.1"]
        ETCD["etcd 注册中心"]
    end

    Client -->|HTTPS REST| GW
    GW -->|gRPC| USER & CONTENT & MEDIA & INTER & FEED & MSG & SEARCH & RECO

    CONTENT -.->|2-phase msg| DTM
    DTM -.->|FanoutPost| FEED
    MEDIA -->|SendOneWay| ROCKET
    ROCKET --> FEEDMQ & MEDIAMQ & SEARCHMQ & RECOMQ & MSGMQ & BLOG

    USER & CONTENT & INTER & FEED & MSG --> MYSQL
    USER & CONTENT & INTER & MSG --> REDIS
    MEDIA --> S3
    MEDIAMQ --> S3
    SEARCHMQ --> ES
    RECOMQ --> MILVUS
    BLOG --> CH
    RPC -. 服务发现 .-> ETCD
```

---

## 2. 服务协作图（Gateway → RPC）

Gateway 在 `app/gateway/internal/svc/service_context.go` 中注入 5 个 zrpc 客户端（均带 `bizErrInterceptor` 业务错误拦截器），按业务域路由到各 RPC。

```mermaid
graph LR
    subgraph GWlogic["Gateway Logic 分组 (gateway.api)"]
        L1["login<br/>/auth/*"]
        L2["user<br/>/user/* /users/*"]
        L3["posts<br/>/post /posts"]
        L4["comment<br/>/comment /comments"]
        L5["like_favorite<br/>/like /favorite"]
        L6["image<br/>/media/image"]
    end

    USER["UserService"]
    CONTENT["ContentService"]
    INTER["InteractionService"]
    MEDIA["MediaService"]

    L1 -->|Register/Login/SendVerifyCode| USER
    L2 -->|GetUser/Follow/GetFollowers| USER
    L3 -->|CreatePost/GetPostList/GetPost| CONTENT
    L4 -->|CreateComment/GetCommentList| CONTENT
    L5 -->|Like/Favorite/GetCounts| INTER
    L6 -->|UploadImage stream| MEDIA

    CONTENT -.->|聚合作者信息| USER
    CONTENT -.->|聚合点赞/收藏状态| INTER
```

> 协作约定（CLAUDE.md 硬性规则）：Handler 仅做参数绑定/调用 Logic；Logic 经 `svc.ServiceContext` 取资源并透传 `ctx`；跨服务一律走 zrpc 且透传入参 `ctx`。

---

## 3. 统一请求生命周期（流程图）

```mermaid
flowchart TD
    A["HTTP 请求"] --> B["go-zero Router<br/>prefix /api/v1 + group"]
    B --> C{"该 group 是否<br/>jwt: Auth?"}
    C -->|是| D["JWT 鉴权中间件<br/>校验 Authorization"]
    C -->|否, 含 OptionalAuth| E["可选鉴权中间件<br/>有 token 则解析"]
    D --> F{"token 合法?"}
    F -->|否| G["401 / errx 统一响应"]
    F -->|是| H["注入 Claims 到 ctx"]
    E --> H
    H --> I["Handler: 绑定 + 校验入参"]
    I --> J["Logic: 业务编排<br/>logx.WithContext(ctx)"]
    J --> K["zrpc 调用 RPC 服务"]
    K --> L{"RPC 返回 BizError?"}
    L -->|是| M["bizErrInterceptor 还原 errx"]
    L -->|否| N["组装 Result[T]"]
    M --> O["errx → HTTP 状态码映射"]
    N --> P["200 + JSON 响应"]
    O --> P
    G --> Q["客户端"]
    P --> Q
```

---

## 4. 鉴权与 JWT（时序图 + 流程图）

`pkg/jwtx/jwt.go`：`GenerateToken` 签发、`ParseToken` 校验（已防 `alg=none` / 算法混淆攻击），`WithClaimsContext` / `GetUserIdFromContext` 完成上下文透传。

```mermaid
sequenceDiagram
    autonumber
    participant C as 客户端
    participant MW as JWT 中间件
    participant GW as Gateway Handler/Logic
    participant U as User RPC

    Note over C,U: 受保护接口 (group jwt: Auth)
    C->>MW: 请求 + Authorization: Bearer <token>
    MW->>MW: ParseToken(token, secret)
    alt 校验失败 (过期/伪造/alg none)
        MW-->>C: errx 鉴权失败 → 401
    else 校验成功
        MW->>MW: WithClaimsContext(ctx, claims)
        MW->>GW: 透传 ctx(含 userId)
        GW->>GW: GetUserIdFromContext(ctx)
        GW->>U: RPC 调用(ctx, req)
        U-->>GW: resp
        GW-->>C: Result[T]
    end
```

JWT 校验内部判定：

```mermaid
flowchart LR
    A["token 字符串"] --> B{"签名算法 == HS256?"}
    B -->|否| X["拒绝: 算法攻击"]
    B -->|是| C{"签名有效?"}
    C -->|否| X
    C -->|是| D{"未过期?"}
    D -->|否| Y["拒绝: 已过期"]
    D -->|是| E["返回 Claims{userId, username}"]
```

---

## 5. 用户注册 / 登录（时序图）

```mermaid
sequenceDiagram
    autonumber
    participant C as 客户端
    participant GW as Gateway (login)
    participant U as User RPC
    participant DB as MySQL
    participant R as Redis

    rect rgb(235,245,255)
    Note over C,R: 发送验证码
    C->>GW: POST /auth/verify-code {phone}
    GW->>U: SendVerifyCode(ctx, req)
    U->>U: validator 校验手机号
    U->>R: SET code:phone (TTL)
    U-->>GW: ok
    GW-->>C: Result(已发送)
    end

    rect rgb(235,255,235)
    Note over C,R: 注册
    C->>GW: POST /auth/register {phone, code, pwd}
    GW->>U: Register(ctx, req)
    U->>R: GET code:phone 校验
    U->>U: 密码哈希 + 雪花ID
    U->>DB: INSERT user (CachedConn)
    U->>U: GenerateToken(userId)
    U-->>GW: {token, user}
    GW-->>C: Result(token)
    end

    rect rgb(255,245,235)
    Note over C,R: 登录
    C->>GW: POST /auth/login {phone, pwd}
    GW->>U: Login(ctx, req)
    U->>DB: 查用户 + 校验密码哈希
    U->>U: GenerateToken(userId)
    U-->>GW: {token}
    GW-->>C: Result(token)
    end
```

---

## 6. 发帖流程 — DTM 二阶段消息 + Fanout（时序图）

`app/content/rpc/internal/logic/create_post_logic.go`：使用 **DTM 二阶段消息（2-phase message）** 保证「写库（帖子 + 标签，同一事务）」与「触发 Feed Fanout」的最终一致性。屏障表通过 `Content.QueryPrepared` 实现。

```mermaid
sequenceDiagram
    autonumber
    participant C as 客户端
    participant GW as Gateway (posts)
    participant CT as Content RPC
    participant DTM as DTM Server
    participant DB as MySQL
    participant FD as Feed RPC

    C->>GW: POST /post {title, content, images, tags}
    GW->>CT: CreatePost(ctx, req)
    CT->>CT: 校验字段 + 雪花ID(post/tag)<br/>images → JSON
    CT->>CT: factory.NewPostCreateMsg(gid)<br/>msg.Add(FanoutPost, {authorId, postId, createdAt})

    Note over CT,FD: DoAndSubmitDB(QueryPrepared 屏障)
    CT->>DTM: Prepare(msg)
    CT->>DB: BEGIN tx
    CT->>DB: InsertPostTx(post)
    CT->>DB: BatchInsertTagsByPostIdTx(tags)
    CT->>DB: COMMIT
    CT->>DTM: Submit(msg)
    DTM->>CT: QueryPrepared 屏障校验(幂等)
    DTM->>FD: FanoutPost(authorId, postId, createdAt)
    FD-->>DTM: ok
    CT-->>GW: {postId}
    GW-->>C: Result(postId)
```

> 若任一分支失败，DTM 依据屏障表自动重试 / 回滚，避免「写库成功但 Fanout 丢失」或重复投递。

---

## 7. Feed 读写扩散（数据流转图）

Feed 采用 **推拉结合（写扩散 + 读扩散）**：

- **普通作者**：写扩散 —— Fanout 时分页拉取粉丝（`GetFollowers`），批量写入每个粉丝的 `feed_inbox`（收件箱）。
- **大 V**（`FollowerCount >= BigVThreshold`）：只写作者 `feed_outbox`（发件箱），**不**写扩散；读取时由消费方按需拉取，避免热点风暴。

逻辑见 `app/feed/mq/internal/logic/fanout.go`。

```mermaid
flowchart TD
    Start["FanoutPost 事件<br/>{authorId, postId, createdAt}"] --> GU["UserSvc.GetUser(authorId)"]
    GU --> OUT["InsertIgnore feed_outbox<br/>(作者发件箱, 幂等)"]
    OUT --> J{"FollowerCount >=<br/>BigVThreshold?"}
    J -->|是 (大V)| BIGV["仅保留 outbox<br/>读扩散: 读时拉取<br/>结束"]
    J -->|否 (普通)| PAGE["分页 GetFollowers<br/>pageSize = FanoutBatchSize"]
    PAGE --> BUILD["构建 FeedInbox 行<br/>{userId, authorId, postId, createdAt}"]
    BUILD --> MORE{"还有下一页?"}
    MORE -->|是| PAGE
    MORE -->|否| INS["BatchInsertIgnore feed_inbox<br/>(粉丝收件箱, 幂等去重)"]
    INS --> Done["完成"]

    BIGV --> Done
```

读取关注流时合并两个来源：

```mermaid
graph LR
    Reader["GetFollowFeed(userId)"] --> INBOX["读 feed_inbox<br/>(普通作者已写扩散)"]
    Reader --> PULL["对关注的大V<br/>实时拉 feed_outbox"]
    INBOX --> MERGE["按 createdAt 归并排序"]
    PULL --> MERGE
    MERGE --> Resp["关注流时间线"]
```

---

## 8. 评论 / 点赞 / 收藏（时序图）

帖子详情页通常需要 Content（帖子/评论）与 Interaction（点赞/收藏状态与计数）协同。

```mermaid
sequenceDiagram
    autonumber
    participant C as 客户端
    participant GW as Gateway
    participant CT as Content RPC
    participant IN as Interaction RPC
    participant DB as MySQL
    participant R as Redis

    rect rgb(235,245,255)
    Note over C,R: 发表评论
    C->>GW: POST /comment {postId, content}
    GW->>CT: CreateComment(ctx)
    CT->>DB: INSERT comment
    CT-->>GW: {commentId}
    GW-->>C: Result
    end

    rect rgb(255,240,240)
    Note over C,R: 点赞
    C->>GW: POST /like {targetId}
    GW->>IN: Like(ctx)
    IN->>DB: UPSERT like 记录
    IN->>R: INCR like_count 缓存
    IN-->>GW: ok
    GW-->>C: Result
    end

    rect rgb(240,255,240)
    Note over C,R: 帖子详情聚合
    C->>GW: GET /post/:postId
    GW->>CT: GetPost(ctx)
    CT->>IN: GetCounts / BatchCheckLiked(ctx)
    IN->>R: 读计数缓存(miss→DB)
    IN-->>CT: {likeCount, liked, favorited}
    CT-->>GW: 帖子 + 互动状态
    GW-->>C: Result
    end
```

> 互动写操作（Like/Favorite/Comment/Follow）同时作为 **行为事件源**，进入推荐画像与行为日志管线（见 §12、§14）。

---

## 9. 媒体上传 — gRPC 流式 + S3（时序图）

`media.proto` 中 `UploadImage` / `UploadVideo` 为 **客户端流式（stream）** RPC。Gateway 将 HTTP 上传体分块通过 `stream.Send` 转发给 Media RPC，落地 MinIO / SeaweedFS（S3 兼容）。

```mermaid
sequenceDiagram
    autonumber
    participant C as 客户端
    participant GW as Gateway (image)
    participant M as Media RPC
    participant S3 as MinIO / SeaweedFS
    participant DB as MySQL

    C->>GW: POST /media/image (multipart)
    GW->>M: stream = UploadImage(ctx)
    loop 分块
        GW->>M: stream.Send(chunk)
        M->>S3: 分片写入对象
    end
    GW->>M: CloseAndRecv()
    M->>S3: 完成对象 (objectKey)
    M->>DB: INSERT media 元数据(雪花ID, objectKey)
    M-->>GW: UploadImageResp{mediaId, url}
    GW-->>C: Result(url)
```

---

## 10. 媒体删除事件 — MQ 异步清理（数据流转图）

`app/media/rpc/internal/logic/delete_media_logic.go` 删除元数据后，向 `media-deleted` topic 发 **SendOneWay** 消息；`app/media/mq` 的 `media_cleanup_consumer` 消费并删除 S3 对象，实现「元数据即时删除 + 存储异步回收」解耦。

```mermaid
flowchart LR
    A["DeleteMedia RPC"] --> B["删除 DB 元数据"]
    B --> C["MQProducer.SendOneWay<br/>topic: media-deleted<br/>{mediaId, s3ObjectKey, deletedAt}"]
    C --> MQ(("RocketMQ<br/>media-deleted"))
    MQ --> D["media/mq<br/>media_cleanup_consumer"]
    D --> E{"s3ObjectKey 非空?"}
    E -->|否| Skip["跳过"]
    E -->|是| F["Storage.Delete(objectKey)"]
    F --> G{"删除成功?"}
    G -->|是| H["ConsumeSuccess"]
    G -->|否| I["重试 (ConsumeRetryLater)"]
    F --> S3[("S3 对象存储")]
```

---

## 11. 搜索索引（数据流转图）

`app/search/mq` 的 `search_consumer` 订阅 `search-index` topic，按事件类型对 Elasticsearch 做 `Index` 或 `Delete`；查询路径由 Search RPC 直接读 ES。

```mermaid
flowchart TD
    subgraph Write["写入路径 (异步索引)"]
        EV["内容变更事件<br/>topic: search-index"] --> MQ(("RocketMQ"))
        MQ --> SC["search/mq consumer"]
        SC --> T{"事件类型"}
        T -->|index| IDX["Indexer.Index(IndexDoc)"]
        T -->|delete| DEL["Indexer.Delete(docID)"]
        IDX --> ES[("Elasticsearch")]
        DEL --> ES
    end

    subgraph Read["查询路径 (同步)"]
        Q["客户端搜索"] --> SR["Search RPC<br/>SearchPosts/Users/Tags"]
        SR --> ES
        SR --> HOT["GetHotSearches<br/>(热搜榜)"]
    end
```

---

## 12. 推荐与行为日志管线（数据流转图）

用户互动行为（点赞/收藏/评论/关注等）作为统一事件源，**双路消费**：

- **Recommend**：`recommend/mq` 订阅 `user-behavior` → 更新用户画像 → 写入 Milvus 向量，支撑 `GetRecommendPosts` / `GetSimilarPosts`。
- **Behavior Log**：`pipeline/behaviorlog` 订阅多种行为 topic（`like/unlike/favorite/unfavorite/comment-create/user-follow/user-unfollow`），经 **布隆过滤器去重** 后批量写入 ClickHouse 供离线分析。

```mermaid
flowchart TD
    SRC["互动/关注行为<br/>like, unlike, favorite, unfavorite,<br/>comment-create, user-follow, user-unfollow,<br/>user-behavior"] --> MQ(("RocketMQ<br/>事件总线"))

    MQ --> RECO["recommend/mq<br/>recommend_consumer<br/>(topic: user-behavior)"]
    RECO --> PROFILE["UpdateUserProfile<br/>画像/向量"]
    PROFILE --> MILVUS[("Milvus 向量库")]
    MILVUS --> SERVE["Recommend RPC<br/>GetRecommendPosts / GetSimilarPosts"]

    MQ --> BL["pipeline/behaviorlog<br/>(多 topic 订阅)"]
    BL --> REC["Recorder.Process"]
    REC --> DEDUP{"Bloom 去重<br/>(BloomBits)"}
    DEDUP -->|已存在| Drop["丢弃重复"]
    DEDUP -->|新行为| CH[("ClickHouse<br/>clickhouse_store")]
```

---

## 13. 私信与通知（时序图）

`message.proto` 提供私信（会话/消息/已读/未读数）与通知；`message/mq` 订阅 `message-push` 渲染并落地通知、清理未读缓存。

```mermaid
sequenceDiagram
    autonumber
    participant C as 客户端
    participant GW as Gateway
    participant MSG as Message RPC
    participant DB as MySQL
    participant MQ as RocketMQ (message-push)
    participant MM as message/mq

    C->>GW: 发送私信
    GW->>MSG: SendMessage(ctx)
    MSG->>DB: INSERT message + 更新会话
    MSG-->>GW: ok
    GW-->>C: Result

    Note over MSG,MM: 通知异步渲染
    MSG-->>MQ: message-push 事件
    MQ->>MM: notification consumer
    MM->>MM: RenderNotification(UserActionEvent)
    MM->>DB: NotificationModel.Insert
    MM->>MM: UnreadStore.DeleteUserUnread(targetUser)

    C->>GW: GetUnreadCount / GetNotifications
    GW->>MSG: RPC
    MSG-->>GW: 未读数 / 列表
    GW-->>C: Result
```

---

## 14. 事件总线全景（MQ 协作图）

RocketMQ topic 与消费者组定义见 `pkg/mqx/topics.go`。下图汇总「生产者 → topic → 消费者组」的事件协作关系。

```mermaid
graph LR
    subgraph Producers["生产者"]
        P_MEDIA["Media RPC<br/>(已接线 SendOneWay)"]
        P_CONTENT["Content RPC"]
        P_INTER["Interaction RPC"]
        P_USER["User RPC"]
        P_MSG["Message RPC"]
    end

    subgraph Topics["Topic (pkg/mqx/topics.go)"]
        T1(["media-deleted"])
        T2(["post-create"])
        T3(["search-index / search-delete"])
        T4(["user-behavior"])
        T5(["like / unlike / favorite / unfavorite"])
        T6(["comment-create / comment-delete"])
        T7(["user-follow / user-unfollow"])
        T8(["message-push"])
    end

    subgraph Consumers["消费者组"]
        C_MEDIA["media-service-group<br/>→ S3 清理"]
        C_FEED["feed-service-group<br/>→ fanout 收/发件箱"]
        C_SEARCH["search-service-group<br/>→ ES 索引"]
        C_RECO["recommend-service-group<br/>→ Milvus 画像"]
        C_BLOG["behavior-log-service-group<br/>→ ClickHouse"]
        C_MSG["message-service-group<br/>→ 通知"]
    end

    P_MEDIA --> T1 --> C_MEDIA
    P_CONTENT --> T2 --> C_FEED
    P_CONTENT --> T3 --> C_SEARCH
    P_INTER --> T4 --> C_RECO
    P_INTER --> T5 --> C_BLOG
    P_CONTENT --> T6 --> C_BLOG
    P_USER --> T7 --> C_BLOG
    P_MSG --> T8 --> C_MSG
```

> 实现状态：`media-deleted` 为当前已接线的 MQ 生产者（`SendOneWay`）；发帖一致性走 **DTM 二阶段消息**（§6）直接驱动 Feed Fanout。其余 topic / 消费者已在 `pkg/mqx` 与各 `*/mq` 模块定义就绪，作为事件总线的统一契约。

---

## 15. 错误码与统一响应（流程图）

错误码集中于 `pkg/errx/codes.go`（通用 1-999 / 用户 1000-1999 / 内容 2000-2999 / 交互 3000-3999 / 媒体 4000-4999 / 搜索 5000-5999）。Logic 层统一返回 `errx.New(code, msg)`，跨进程经 gRPC status 传播，Gateway 侧由 `bizErrInterceptor` 还原后映射 HTTP 状态码。

```mermaid
flowchart TD
    L["RPC Logic 层<br/>errx.NewWithCode(code)"] --> G["resolve_grpc: 编码为<br/>gRPC status + details"]
    G --> NET(("gRPC 传输"))
    NET --> I["Gateway bizErrInterceptor<br/>解码还原 BizError"]
    I --> H["resolve_http: code → HTTP 状态"]
    H --> R["统一响应体 Result[T]<br/>{code, msg, data}"]
    R --> Client["客户端"]

    subgraph Rule["硬性约定"]
        X1["禁止裸 errors.New"]
        X2["禁止 Handler 手动 httpx.Error"]
        X3["状态码映射由 errx 中间件统一处理"]
    end
```

---

## 16. 部署拓扑（组件图）

`deploy/docker-compose.middleware.yml` 提供完整本地依赖栈，统一接入 `xbh-network`。

```mermaid
graph TB
    subgraph Net["docker network: xbh-network"]
        subgraph Data["数据/缓存"]
            MYSQL[("mysql:3306")]
            REDIS[("redis:6379")]
            ES[("elasticsearch:9200")]
            CH[("clickhouse")]
        end
        subgraph MQReg["消息/注册/事务"]
            NS["rocketmq-namesrv"]
            BR["rocketmq-broker"]
            CONSOLE["rocketmq-console"]
            ETCD["etcd"]
            DTM["dtm"]
        end
        subgraph Vector["向量检索 (Milvus 栈)"]
            MILVUS[("milvus")]
            METCD["etcd-milvus"]
            MMINIO["minio-milvus"]
        end
        subgraph Object["对象存储"]
            MINIO[("minio")]
            SEAWEED[("seaweedfs")]
        end
        subgraph Obs["可观测性"]
            JAEGER["jaeger"]
            PROM["prometheus"]
            GRAFANA["grafana"]
            LOKI["loki"]
        end
    end

    NS --- BR
    BR --- CONSOLE
    MILVUS --- METCD
    MILVUS --- MMINIO
    PROM --> GRAFANA
    LOKI --> GRAFANA
```

---

## 17. 整体数据流转全景图

```mermaid
flowchart TB
    Client(["客户端"])

    Client -->|REST| GW["Gateway :8888"]

    GW -->|gRPC 同步| SVC["RPC 服务层<br/>User/Content/Media<br/>Interaction/Feed<br/>Message/Search/Recommend"]

    SVC -->|读写| DB[("MySQL + Redis")]
    SVC -->|2-phase msg| DTM["DTM"]
    DTM -->|FanoutPost| FEED["Feed: inbox/outbox"]
    FEED --> DB

    SVC -->|流式| OBJ[("S3: MinIO/SeaweedFS")]

    SVC ==>|事件| BUS(("RocketMQ 事件总线"))
    BUS -->|media-deleted| CLEAN["S3 异步清理"] --> OBJ
    BUS -->|search-index| ESIDX["ES 索引"] --> ES[("Elasticsearch")]
    BUS -->|user-behavior| RECO["画像更新"] --> MILVUS[("Milvus")]
    BUS -->|行为多 topic| BLOG["布隆去重 → 聚合"] --> CH[("ClickHouse")]
    BUS -->|message-push| NOTIF["通知渲染"] --> DB

    ES -.->|搜索结果| GW
    MILVUS -.->|推荐结果| GW
    FEED -.->|时间线| GW

    SVC -.->|trace/metric/log| OBS["Jaeger / Prometheus<br/>Grafana / Loki"]
```

---

> 维护说明：本规格随 `proto/`、`app/*/internal/logic`、`pkg/mqx/topics.go` 变更同步更新。
> 图中「DTM 二阶段消息」与「media-deleted 事件」为当前已接线路径，其余 topic 为事件总线统一契约（消费者已就绪，生产者陆续接入）。
