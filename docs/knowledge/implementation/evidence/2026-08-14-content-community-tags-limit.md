---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: cf0f53e
commands:
  - go test -count=1 ./app/content/rpc/internal/logic/
  - make check
  - make test
result: passed
---

# 2026-08-14 标签列表 limit 防御性上限 + vet copylocks 修复

## 改动

- `app/content/rpc/internal/logic/get_tags_logic.go`：`GetTags` 的
  `limit` 增加 `maxTagListLimit=100` 上限（防御性：当前 RPC 无外部入口，
  防止未来接入时超大 LIMIT 全表排序查询）。
- `page.go`：新增 `maxTagListLimit` 常量。
- `tag_logic_test.go`：`TestGetTagsCapsLimit` 验证超大 limit 被截断。
- `create_post_event_test.go`：上一轮幂等哈希测试中的 proto 结构体
  `*draft = *base` 拷贝触发 `go vet` copylocks（proto message 含 mutex），
  改为显式构造各变体。

## 审查发现（本轮深入扫描）

- update（PATCH/revision/防 Lost Update）、delete（软删除/版本检查）、
  search consumer（非 published 索引移除/失败重试）、recommend
  recall/enrich/visible（并发召回/回源验证/降级）、feed fanout
  （BigV 阈值）、assistant safety/state（归一化/Lua 原子性）、
  behavior publisher、pipeline ClickHouse、SSE 均审查通过。
- **观察项**：`content.GetTags` RPC 无任何调用方（gateway 走 search RPC
  的 `/search/tags`）；proto 契约删除需 goctl 重新生成且属对外契约，
  本轮仅加固 limit 上限并记录，不删除契约。

## 结果

- `go test ./app/content/rpc/internal/logic/` 全过；`make check` 通过
  （含 `go vet` copylocks）；`make test` 全部模块通过（含 race）。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
