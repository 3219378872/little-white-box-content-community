---
implementation: IMP-content-community-backend
verified_at: 2026-09-01
verified_commit: 35051186a75015f7dfc5a41e0a8f6bc398f62398
commands:
  - make check
  - make test
  - make coverage
  - go test -race ./app/assistant/internal/runtime ./app/assistant/internal/store
  - go test -tags=integration ./app/assistant/internal/store
  - go test -tags=integration ./app/assistant/internal/store -run 'TestSQL(InputAcceptanceLocksOpenRunBeforeThread|MemoryMutationsSerializeCapacityDedupeAndUndo|WatchQuotaReservationSerializesWorkers|RequeueFailedWatchBucketReleasesReservation)$' -count=5
  - INTEGRATION_S3_PORT=18333 make integration-all
result: partial
---

# 2026-09-01 Assistant Agent 审查修复证据

## 已验证行为

- SSE 订阅始终按 cursor 轮询 MySQL；阻塞 Redis wake token 的 fake 不会阻止已持久事件和终止事件被读出。
- Responses stream 的错误正文与状态统一映射到 typed provider error；确定性 4xx 不重试，暂态错误仍按
  有界 retry/fallback 策略处理。
- 搜索、推荐和 web executor 只有在全部 source handle 成功写入本 run ledger 后才向模型返回结果；
  ledger 缺失或写失败明确失败，不再返回无法用于 `present_sources` 的裸来源。
- `search_history` 实现 keywords/around/session/recent 四种 shape。ES 只提供候选 id，MySQL 二次读取后
  重新校验 user、visible、role、kind、365 天与 live message 排除；compacted 历史仍可返回。
- Memory 按 `(user_id, target)` 排他串行容量、规范化去重、replace/remove 和 undo。MySQL 并发测试证明
  重复 add 返回同一 id、超容量竞争只有一笔成功、两个 undo 只有一个通过 CAS。
- Watch matcher 校验当前 post revision；调度、恢复、每个模型轮和最终提交前重新回源全部精确 hit。
  最终检查失效时，流式正文先产生 `response_reset`，bucket 进入 discarded，且不写主动消息。
- Watch 小时/日额度在调度事务中写 reservation；四 worker 并发不会超发，成功才转 sent，失败、抢占、
  requeue 和 discard 均释放额度。`RequeueFailedBuckets` 不再用 join `FOR UPDATE` 形成反向锁序。
- 输入接收先按 id 锁该用户全部开放 run，再锁 thread 和 bucket/quota。定向 MySQL 测试构造 worker
  `run -> thread` 与 redirect 竞争，连续 5 轮均无死锁或丢失 redirect。
- Watch update 在恢复时只有当前 version 已等于期望后继且目标状态一致才重放成功，避免把并发修改误认
  为本次调用结果。

## 门禁结果

- `make check` 通过 engineering policy、知识策略、`go vet` 和 golangci-lint，lint 为 `0 issues`。
- `make test` 全仓通过；Assistant runtime 覆盖率 73.5%。聚焦 runtime/store race 测试通过。
- `make coverage` 通过：Handler 88.3%、Logic 78.4%、MQ consumer 73.6%、handwritten 53.4%。第一次
  post-lock-order 重跑在无关 recommend gRPC deadline fixture 上一次得到 `infer-unavailable` 而非
  `infer-timeout`；该 case 定向 10 轮通过，随后完整 coverage 重跑通过，没有修改该产品逻辑或测试。
- 当前提交的 Assistant Store MySQL integration 全部通过；Memory、Watch reservation/requeue 与输入锁序
  组合连续 5 轮通过。
- 代码主提交 `d9d17ffb82a601362036dde1022770fe974c1989` 上的完整 `integration-all` 全部通过，
  包含 `esx/integration`、15 项模型流水线并输出 `Integration services cleared: xbh-test`。其后的
  `35051186a75015f7dfc5a41e0a8f6bc398f62398` 只收紧输入锁序并新增 MySQL 回归；当前提交已重跑
  `make check`、`make test`、`make coverage`、race 和完整 Assistant Store integration。

## 部分与未覆盖

- 本证据尚未计入根编排仓合并后的 `just up`、真实 patch 重放和黑盒 Agent E2E；这些必须在三仓提交
  合并后单独执行。
- 未调用外部 live provider，不证明供应商网关的真实限流、计费或跨网络流式行为。
- 未启动 production profile，也未对生产数据执行迁移；本轮只增加非破坏性、幂等 patch，并在隔离
  MySQL schema 与本地 integration 环境验证。
