---
implementation: IMP-content-community-backend
verified_at: 2026-08-19
verified_commit: 49fb5d4
commands:
  - go test -count=1 ./deploy/ -run TestRocketMQBrokerDisablesContainerSupport
result: passed
---

# 2026-08-19 RocketMQ cgroup v2 StoreUtil 修复

## 缺陷

`apache/rocketmq:5.1.3`（Temurin 8u372）在宿主机 cgroup v2 上，首次 Pull 触发
`StoreUtil.<clinit>` → `OperatingSystemMXBean.getTotalPhysicalMemorySize` →
`CgroupV2Subsystem.getInstance` NPE。类初始化失败后，该 JVM 内所有 Pull 回
`NoClassDefFoundError`，count-sync / search / feed / behavior-log 全部积压。

## 改动

`deploy/docker-compose.middleware.yml` 的 broker `JAVA_OPT_EXT` 增加
`-XX:-UseContainerSupport`。

## 结果

- `go test ./deploy/ -run TestRocketMQBrokerDisablesContainerSupport` 通过。
- 本机 recreate broker 后消费组 Diff 归零；新点赞 1 秒内 `count applied`，
  `GET /api/v1/post/1006` 为 `likeCount=1, isLiked=true`。
