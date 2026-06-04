# RocketMQ Topic 预创建 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 broker 容器启动时自动预创建全部 17 个 Topic 和 7 个 ConsumerGroup，解决 consumer 服务因 topic 不存在而 fatal 崩溃的问题。

**Architecture:** 在 broker 容器内挂载一个 init 脚本，broker 进程后台启动后运行脚本通过 `mqadmin` 批量创建资源，然后 `wait` broker 进程保持容器存活。

**Tech Stack:** RocketMQ 5.1.3 mqadmin CLI, Docker Compose, Bash

**Spec:** `docs/superpowers/specs/2026-04-27-rocketmq-init-topics-design.md`

---

### Task 1: 创建初始化脚本

**Files:**
- Create: `deploy/rocketmq/init-topics.sh`

- [ ] **Step 1: 创建 init-topics.sh**

```bash
#!/bin/bash
MQADMIN="/home/rocketmq/rocketmq-5.1.3/bin/mqadmin"
NAMESRV="rocketmq-namesrv:9876"
MAX_WAIT=60
waited=0

while ! bash -c "echo > /dev/tcp/localhost/10911" 2>/dev/null; do
  sleep 2
  waited=$((waited + 2))
  if [ $waited -ge $MAX_WAIT ]; then
    echo "[init] broker not ready after ${MAX_WAIT}s, skip init"
    exit 0
  fi
done

echo "[init] broker ready, creating topics and consumer groups..."

TOPICS=(
  user-register user-follow user-unfollow
  post-create post-update post-delete comment-create comment-delete
  like unlike favorite unfavorite
  search-index search-delete
  user-behavior
  feed-generate
  message-push
  media-deleted
)

for t in "${TOPICS[@]}"; do
  $MQADMIN updateTopic -n "$NAMESRV" -b localhost:10911 \
    -t "$t" -r 4 -w 4 2>/dev/null && \
    echo "[init] topic created: $t" || \
    echo "[init] topic already exists or failed: $t"
done

GROUPS=(
  user-service-group
  content-service-group
  search-service-group
  feed-service-group
  message-service-group
  recommend-service-group
  media-service-group
)

for g in "${GROUPS[@]}"; do
  $MQADMIN updateSubGroup -n "$NAMESRV" -b localhost:10911 \
    -g "$g" 2>/dev/null && \
    echo "[init] group created: $g" || \
    echo "[init] group already exists or failed: $g"
done

echo "[init] done."
```

- [ ] **Step 2: 设置可执行权限**

Run: `chmod +x deploy/rocketmq/init-topics.sh`

- [ ] **Step 3: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add deploy/rocketmq/init-topics.sh
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "feat: add RocketMQ topic/consumerGroup init script"
```

---

### Task 2: 修改 docker-compose broker 服务

**Files:**
- Modify: `deploy/docker-compose.middleware.yml:99-119` (rocketmq-broker service)

- [ ] **Step 1: 修改 broker command**

将第 109 行：

```yaml
    command: bash -c "chown -R rocketmq:rocketmq /home/rocketmq/store /home/rocketmq/logs && su rocketmq -s /bin/bash -c 'sh mqbroker -c /home/rocketmq/rocketmq-5.1.3/conf/broker.conf -n rocketmq-namesrv:9876'"
```

替换为：

```yaml
    command: >
      bash -c "
        chown -R rocketmq:rocketmq /home/rocketmq/store /home/rocketmq/logs &&
        su rocketmq -s /bin/bash -c '
          sh mqbroker -c /home/rocketmq/rocketmq-5.1.3/conf/broker.conf -n rocketmq-namesrv:9876 &
          BROKER_PID=\$$! &&
          sh /home/rocketmq/scripts/init-topics.sh &&
          wait \$$BROKER_PID
        '
      "
```

- [ ] **Step 2: 新增 volume 挂载**

在第 114 行 `./rocketmq/broker.conf` 挂载之后添加：

```yaml
      - ./rocketmq/init-topics.sh:/home/rocketmq/scripts/init-topics.sh:ro
```

最终 volumes 部分：

```yaml
    volumes:
      - rocketmq_broker_logs:/home/rocketmq/logs
      - rocketmq_broker_store:/home/rocketmq/store
      - ./rocketmq/broker.conf:/home/rocketmq/rocketmq-5.1.3/conf/broker.conf
      - ./rocketmq/init-topics.sh:/home/rocketmq/scripts/init-topics.sh:ro
```

- [ ] **Step 3: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add deploy/docker-compose.middleware.yml
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "feat: mount init script in broker, background start + wait pattern"
```

---

### Task 3: 验证

- [ ] **Step 1: 重启 broker 容器**

Run: `docker compose -f deploy/docker-compose.middleware.yml up -d rocketmq-broker`

- [ ] **Step 2: 查看 broker 日志确认初始化输出**

Run: `docker logs xbh-rocketmq-broker 2>&1 | grep "\[init\]"`

Expected: 看到 17 行 `topic created:` 和 7 行 `group created:` 输出。

- [ ] **Step 3: 验证 topic 已创建**

Run: `docker exec xbh-rocketmq-broker /home/rocketmq/rocketmq-5.1.3/bin/mqadmin topicList -n rocketmq-namesrv:9876 2>/dev/null | grep -E "post-create|feed-generate|media-deleted"`

Expected: 三个 topic 名称均出现在输出中。

- [ ] **Step 4: 验证 consumerGroup 已创建**

Run: `docker exec xbh-rocketmq-broker /home/rocketmq/rocketmq-5.1.3/bin/mqadmin consumerProgress -n rocketmq-namesrv:9876 -g feed-service-group 2>/dev/null`

Expected: 输出 consumerGroup 信息而非 "not found" 错误。

- [ ] **Step 5: 重启 feed 服务验证不再崩溃**

重新启动 feed 服务，确认启动日志中不再出现 `topic not exist` 错误和 fatal 崩溃。

- [ ] **Step 6: Commit spec 和 plan 文档**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add docs/superpowers/specs/2026-04-27-rocketmq-init-topics-design.md docs/superpowers/plans/2026-04-27-rocketmq-init-topics.md
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "docs: add RocketMQ init topics design spec and plan"
```
