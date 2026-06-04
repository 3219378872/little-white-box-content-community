# 服务架构

## 概览

esx 是一个基于 go-zero 的社交内容平台微服务集群。

```
Client → Gateway (REST :8888) → User RPC (:9090)
                               → Content RPC (:8088)
                               → Media RPC (:9008)
```

## 服务清单

| 服务 | 类型 | 端口 | 入口文件 | 定义文件 |
|------|------|------|---------|---------|
| Gateway | REST API 网关 | :8888 | `app/gateway/gateway.go` | `app/gateway/gateway.api` |
| User | RPC 服务 | :9090 | `app/user/user.go` | `proto/user/user.proto` |
| Content | RPC 服务 | :8088 | `app/content/content.go` | `proto/content/content.proto` |
| Media | RPC 服务 | :9008 | `app/media/media.go` | `proto/media/media.proto` |

## RPC 服务分层

每个 RPC 服务遵循 go-zero 标准分层：

```
internal/config/   → 配置结构体
internal/svc/      → 依赖注入容器（ServiceContext）
internal/server/   → gRPC server 实现
internal/logic/    → 业务逻辑
internal/model/    → 数据访问层
```

## 服务间通信

- **Gateway → RPC**：通过 zrpc 客户端，经 etcd 服务发现
- **RPC → RPC**：Content 聚合 User（作者信息）和 Interaction（点赞/收藏状态）
- **RPC → MQ**：异步事件通过 RocketMQ（media-deleted、post-create 等）
- **DTM 二阶段消息**：Content 发帖使用 DTM 保证写库与 Feed Fanout 的最终一致性

## 详细架构图

参见 [architecture-diagrams.md](design-docs/architecture-diagrams.md) —— 包含 17 个 Mermaid 图覆盖系统全景、请求生命周期、事件总线、部署拓扑等。
