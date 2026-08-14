---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 468da66
commands:
  - go test -count=1 ./app/content/mq/cleanup/...
  - make check
  - make test
result: passed
---

# 2026-08-14 count-sync 去重占位失败重试缺陷修复（CORE-032）

## 缺陷

`CountSyncStore.ApplyBehaviorCount` 先 `SetnxEx` 占用去重键，再应用
计数增量。若 `applyDelta` 失败（DB 故障），消费者返回
`ConsumeRetryLater`，MQ 重投后 `SetnxEx` 因键仍存在返回
`first=false` → 直接跳过 → **该事件的计数永久丢失**（CORE-032
30 秒收敛被破坏）。

## 修复

`applyDelta` 失败时 best-effort 删除去重键（`DelCtx`），允许重投后
重新占位并应用；删除失败则保留占位——宁可漏一次也不对同一事件
重复计数。日志记录删除失败。

## 测试

- 新增 `TestCountSyncStoreDBFailureRemovesDedupForRetry`：DB 失败 →
  断言去重键被释放；模拟重投（DB 恢复）→ 占位重新成功、计数正常应用。
- 既有 `TestCountSyncStoreDBFailurePropagates`（仅断言错误）不受影响。

## 审查证据（本轮深入扫描）

- search 深分页防护（MaxResultWindow）、embedding 批处理/rebuild、
  评论可见性纵深防御、pipeline IP 匿名化、assistant LLM 生成器、
  推荐 fallback 交错、gateway/RPC 参数校验一致性均审查通过。

## 结果

- `go test ./app/content/mq/cleanup/...` 全过；`make check` 通过；
  `make test` 全部模块通过（含 race）。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
