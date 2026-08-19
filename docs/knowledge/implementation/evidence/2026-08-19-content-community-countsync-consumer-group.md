---
implementation: IMP-content-community-backend
verified_at: 2026-08-19
verified_commit: 8e9c867
commands:
  - go test -count=1 ./app/content/mq/cleanup/... ./deploy/... ./pkg/mqx/...
  - python3 scripts/engineering-lint.py
result: passed
---

# 2026-08-19 count-sync 独立消费组（CORE-032）

## 缺陷

`content-cleanup` 在同一进程里先 `Start()` cleanup 消费者，再 `Start()`
count-sync 消费者，两者共用 `content-cleanup-service-group`。RocketMQ 客户端
第二次 `Start()` fatal：`the consumer group has been created`。公开
`post.like_count` 永不回写，Gateway 又只回填 `isLiked`，表现为实心赞 + 数字 0。

## 改动

- `CountSync` 独立配置，组名 `content-count-sync-service-group`，与 cleanup
  组必须不同（缺省或同组直接报错）。
- `init-topics.sh` 预创建该组；yaml/部署测试断言组名分离且重试上限为 8。

## 结果

- `go test ./app/content/mq/cleanup/... ./deploy/... ./pkg/mqx/...` 通过。
- `engineering-lint` 通过。
- 现场：进程保持存活，两个组均已订阅；从 `action_count` 回填后
  `GET /api/v1/post/1300` 与推荐流为 `likeCount=1, isLiked=true`。

## 未覆盖

- 本机 broker 拉消息报 `StoreUtil` `NoClassDefFoundError`，新点赞的 30s
  回写未在本次观测到；历史计数以权威表回填，消费位移已拨到队尾避免重放双计。
