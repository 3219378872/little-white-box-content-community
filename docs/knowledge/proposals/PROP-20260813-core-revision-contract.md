---
id: PROP-20260813-core-revision-contract
layer: proposal
title: CORE-013 乐观锁与 CORE-062 向后兼容的契约收敛选项
status: closed
owner: agent
target_layer: spec
decision: 2026-08-13 人类采纳选项 B（/api/v2 强制 expected_revision，/api/v1 迁移期+废弃计划）
upstream:
  - SPEC-community-core
---

# 观察到的冲突

- `CORE-013`：发布、取消发布、编辑和删除必须携带调用者最后读取的预期 `revision`；
  版本冲突返回 `409 Conflict`，不得覆盖并发更新。
- `CORE-062`：已发布的 `/api/v1` 社区核心契约在同一主版本内保持字段和语义向后兼容；
  破坏性变化必须使用新版本或明确迁移期。
- 现状（v1）：`expected_revision=0` 表示旧客户端未携带，跳过乐观锁检查
  （`app/content/rpc/internal/logic/update_post_logic.go` / `delete_post_logic.go`）。
  新客户端携带预期 revision 时正常冲突检测。`IMP-content-community-backend` 因此将
  CORE-013 记为 `partial`，DES 已登记“未获人类决定前不改契约”。

# 建议决策选项（请人类选择其一）

- **选项 A（维持现状 + 迁移期限）**：保留 `expected_revision=0` 跳过语义，但设定明确的
  迁移截止日期与客户端升级要求；到期后 CORE-013 强制。影响：兼容性最好，但截止日前
  规范仍为 partial。
- **选项 B（新版本严格契约）**：`/api/v2/post` 写接口强制 `expected_revision`（缺失按
  参数错误拒绝），`/api/v1` 维持现状并声明废弃计划；CORE-062 以“新版本”满足，CORE-013
  在 v2 上完全满足。影响：需要新增/调整 `/api/v2` 路由、生成与测试；旧客户端不受影响。
- **选项 C（服务端默认取当前 revision）**：旧客户端缺失时由服务端读取当前 revision 作为
  期望值再执行更新（等效无检测），语义上与现状相同但把行为显式化。影响：不推荐——与
  CORE-013 的并发保护意图冲突，只是把隐藏行为文档化。

# 推荐

**选项 B**：契约层面显式区分“旧兼容”与“严格并发控制”，CORE-013/CORE-062 可同时闭环；
实现范围清晰（gateway `.api`、content proto/Logic、幂等与冲突测试），可独立批次验证。
若短期资源受限，可先取**选项 A** 并登记截止日期。

# 决策结果（2026-08-13，人类采纳）

采纳**选项 B**：新增 `/api/v2/post` 写接口（create/update/delete），Update/Delete 强制
`expectedRevision`（缺失或 0 → `ParamError`，版本冲突 → `409 ContentVersionConflict`）；
`/api/v1` 维持现状并登记迁移期与废弃计划。`SPEC-community-core` 文本无需修改（CORE-013
本就要求携带 revision，CORE-062 允许新版本）。

# 影响

- 代码：gateway/content 契约与逻辑、测试。
- 文档：`SPEC-community-core`（需人类批准后修改）、DES、IMP 状态行。
- 不改动：读取路径、幂等语义、outbox 可靠写入。
