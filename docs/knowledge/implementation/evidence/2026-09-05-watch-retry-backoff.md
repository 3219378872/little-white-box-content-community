---
implementation: IMP-content-community-backend
verified_at: 2026-09-05
verified_commit: a2c2f295d459aaa0650d7cbbbf0ba7ac272e29ce
commands:
  - go test -race -count=1 ./app/assistant/internal/store ./app/assistant/internal/runtime
  - go test -tags=integration -count=1 -timeout=10m -run 'TestSQL(WatchFailure|FinishWatchDelivery|RequeueFailedWatchBucket|RequeueFailedWatchBuckets)' ./app/assistant/internal/store
  - make check
result: passed
---

# 2026-09-05 Watch 失败重试退避证据

## 验证范围

- Watch run 的终态存储接口从布尔 delivered 改为明确 run status。`done` 原子提交 sent/stat；
  `cancelled`、Stop、撤权或用户输入抢占仍立即回 pending；`error` 释放 reservation 后进入延迟重试。
- error 首次退避 1 分钟，之后为 2/4/8/16/30/30 分钟，第 8 次失败 discard。若下一次重试时间达到
  bucket 创建后 90 天边界，同样 discard；discarded bucket 不再出现在 due 列表。
- 历史失败只统计相同 user/source/status 且结构化 `queued_payload.bucket_id` 相同的 run，并明确排除
  当前 run 后再加本次尝试。当前 payload 缺少或错误关联时保守 discard；恢复扫描还会拒绝跨用户或
  非 Watch 的终态 run 关联。
- 正常 error、cancel、恢复扫描、discard，以及 bucket 已归还但 reservation 残留的幂等收尾都会释放
  `watch_send_reservation` 和 `reserved_count`；只有成功投递增加 sent_count。

## 门禁结果

- Store 与 Runtime 两个包的 race-enabled 单测通过：`store` 1.029s，`runtime` 1.432s。
- MySQL 8 + Redis 隔离容器的 5 个定向集成测试通过（20.950s），覆盖 error 延迟且到期前不 due、
  cancel 即时 pending、8 次上限、恢复扫描共用策略、payload/owner/source fail-closed 与残留 reservation
  清理。容器时区为 Asia/Shanghai，测试输入为固定 UTC Unix 毫秒；状态比较不依赖 SQL wall-clock。
- `make check` 通过：25 个知识治理单测、工程知识校验、`go vet` 和 golangci-lint（0 issues）。

## 行为与未覆盖边界

- 未修改公开 API、protobuf、数据库 schema、配置格式或成功投递业务语义。
- 本证据没有运行全仓 `make test`、完整 integration、根仓黑盒 E2E、live provider、生产流量或正式性能
  基准；这些门禁由后续整合验证单独记录。
