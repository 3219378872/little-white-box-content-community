# RocketMQ Topic 与 ConsumerGroup 预创建

## 问题

Feed 服务（及其他 consumer 服务）启动时，尝试订阅 `post-create` 等 topic，但 RocketMQ broker 中尚未创建这些 topic。尽管 `autoCreateTopicEnable = true`，topic 只在 **producer 首次发送消息时**自动创建，consumer 订阅不存在的 topic 会导致 `logx.Must` 触发 fatal 崩溃。

## 方案

在 broker 容器内挂载初始化脚本，broker 启动后自动通过 `mqadmin` 预创建全部 topic 和 consumerGroup。

### 选型理由

- 不增加额外容器，初始化与 broker 生命周期绑定
- `updateTopic` / `updateSubGroup` 命令是幂等操作，broker 重启时重跑无副作用
- 脚本失败不影响 broker 主进程

## 变更范围

### 1. 新增文件：`deploy/rocketmq/init-topics.sh`

初始化脚本，逻辑如下：

1. 轮询 `localhost:10911`，等待 broker 就绪（最多 60 秒，超时则跳过不阻塞）
2. 使用 `mqadmin updateTopic` 创建全部 17 个 topic，统一 `readQueueNums=4, writeQueueNums=4`
3. 使用 `mqadmin updateSubGroup` 创建全部 7 个 consumerGroup

**Topic 列表**（与 `pkg/mqx/topics.go` 保持一致）：

| 业务域 | Topic |
|--------|-------|
| 用户 | `user-register`, `user-follow`, `user-unfollow` |
| 内容 | `post-create`, `post-update`, `post-delete`, `comment-create`, `comment-delete` |
| 互动 | `like`, `unlike`, `favorite`, `unfavorite` |
| 搜索 | `search-index`, `search-delete` |
| 推荐 | `user-behavior` |
| Feed | `feed-generate` |
| 消息 | `message-push` |
| 媒体 | `media-deleted` |

**ConsumerGroup 列表**（与 `pkg/mqx/topics.go` 保持一致）：

| 服务 | ConsumerGroup |
|------|---------------|
| User | `user-service-group` |
| Content | `content-service-group` |
| Search | `search-service-group` |
| Feed | `feed-service-group` |
| Message | `message-service-group` |
| Recommend | `recommend-service-group` |
| Media | `media-service-group` |

### 2. 修改文件：`deploy/docker-compose.middleware.yml`

**broker 服务 `command` 改动**：

- 将 broker 进程后台启动（`&`），获取 PID
- 执行 `init-topics.sh` 初始化脚本
- `wait $BROKER_PID` 保持容器前台进程存活

**broker 服务 `volumes` 新增**：

```yaml
- ./rocketmq/init-topics.sh:/home/rocketmq/scripts/init-topics.sh:ro
```

### 3. 不变更：`deploy/rocketmq/broker.conf`

`autoCreateTopicEnable` 保持 `true`，不在此次变更中调整。

## 维护约定

当 `pkg/mqx/topics.go` 新增 Topic 或 ConsumerGroup 时，需同步更新 `deploy/rocketmq/init-topics.sh`。
