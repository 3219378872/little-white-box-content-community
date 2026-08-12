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
legacy_upstream:
---
```

正文至少说明目标映射、方案、接口与数据流、取舍、失败模式和验证策略。仅使用过渡基线时，
将正式 `upstream` 留空，并写入 `legacy:<path>#<heading>`。
