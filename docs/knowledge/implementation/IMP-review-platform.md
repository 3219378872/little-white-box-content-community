---
id: IMP-review-platform
layer: implementation
title: 审核平台实现映射
status: active
owner: agent
code_paths:
- pkg/outboxx
- pkg/idempotencyx
- pkg/mqx
- pkg/errx
- app/gateway
- deploy/sql
updated_at: 2026-10-01
---

# 审核平台实现映射

审核平台尚无实现。`code_paths` 暂列将被复用或修改的既有入口；review-rpc、review-worker、
moderation-infer 与 `xbh_review` schema 落地后补充对应路径。全部条款在取得当前提交上的有效证据前保持
`unknown`，计划周次见 [DES-review-platform](../design/DES-review-platform.md) 的分期表。

| requirement | design | state | evidence or gap |
| --- | --- | --- | --- |
| RVW-001 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-002 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-003 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-004 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-010 | DES-review-platform | unknown | gap: 尚未实现（计划 W2～W3）。 |
| RVW-011 | DES-review-platform | unknown | gap: 尚未实现（计划 W2～W3）。 |
| RVW-012 | DES-review-platform | unknown | gap: 尚未实现（计划 W2）。 |
| RVW-013 | DES-review-platform | unknown | gap: 尚未实现（计划 W2）。 |
| RVW-014 | DES-review-platform | unknown | gap: 尚未实现（计划 W2）。 |
| RVW-015 | DES-review-platform | unknown | gap: 尚未实现（计划 W2）。 |
| RVW-016 | DES-review-platform | unknown | gap: 尚未实现（计划 W3）。 |
| RVW-017 | DES-review-platform | unknown | gap: 尚未实现（计划 W2）。 |
| RVW-018 | DES-review-platform | unknown | gap: 尚未实现（计划 W3）。 |
| RVW-020 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-021 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-022 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-023 | DES-review-platform | unknown | gap: 尚未实现（计划 W1，机审证据随 W3 补齐）。 |
| RVW-024 | DES-review-platform | unknown | gap: 尚未实现（计划 W1，申诉随 W6 补齐）。 |
| RVW-025 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-030 | DES-review-platform | unknown | gap: 尚未实现（计划 W3）。 |
| RVW-040 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-041 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-050 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-051 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-060 | DES-review-platform | unknown | gap: 尚未实现（计划 W2）。 |
| RVW-061 | DES-review-platform | unknown | gap: 尚未实现（计划 W2）；占位模型与演示流量不能代表真实时效。 |
| RVW-A01 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-A02 | DES-review-platform | unknown | gap: 尚未实现（计划 W3）。 |
| RVW-A03 | DES-review-platform | unknown | gap: 尚未实现（计划 W1）。 |
| RVW-A04 | DES-review-platform | unknown | gap: 尚未实现（计划 W3）。 |
| RVW-A05 | DES-review-platform | unknown | gap: 尚未实现（计划 W1，申诉随 W6 补齐）。 |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出，路径均相对仓库根目录。新服务落地时同步更新本页与
`docs/knowledge/guides/architecture.md`。
