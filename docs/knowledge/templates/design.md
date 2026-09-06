# 设计页面模板

此文件只是 agent 使用的格式模板，不是正式设计。

```yaml
---
id: DES-example
layer: design
title: Replace with an agent-designed solution
status: draft
owner: agent
upstream:
  - SPEC-example
updated_at: YYYY-MM-DD
tracks:
  - EXAMPLE-001
---
```

正文至少说明精确 requirement 映射、方案、接口与数据流、取舍、失败模式和验证策略。仅使用过渡
基线时，将正式 `upstream` 留空，并另写白名单中的 `legacy:<path>#<heading>`；current DES 的
`tracks` 不得使用范围或合并 ID。
