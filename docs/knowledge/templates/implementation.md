# 实现页面模板

此文件只是 agent 使用的格式模板，不是正式实现记录。

```yaml
---
id: IMP-example
layer: implementation
title: Replace with a current implementation mapping
status: unknown
owner: agent
upstream:
  - DES-example
updated_at: YYYY-MM-DD
tracks:
  - EXAMPLE-001
code_paths:
  - app/example
evidence:
  - EVD-example
---
```

正文使用权威逐条表，每行只含一个 requirement、一个 current DES、状态和证据/缺口。`aligned` 行
必须引用该 IMP 双向登记的 active/passed EVD；`unknown/diverged` 行使用 `gap: ...`。页头按行聚合，
提交和验证日期只写 EVD。

| Requirement | Design | Status | Evidence/Gap |
| --- | --- | --- | --- |
| EXAMPLE-001 | DES-example | unknown | gap: replace with a concrete validation gap. |
