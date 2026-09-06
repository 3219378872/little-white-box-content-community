# EVD 证据页面模板

此文件只是 agent 使用的格式模板，不是验证结果。

```yaml
---
id: EVD-example
layer: evidence
title: Replace with an observed verification
status: active
owner: agent
upstream:
  - IMP-example
updated_at: YYYY-MM-DD
covers:
  - EXAMPLE-001
scope:
  - unit
commands:
  - replace-with-actual-command
observed_commit: replace-with-full-40-character-sha
result: partial
---
```

正文记录环境、关键输出、未覆盖边界，以及结果为何是 `passed/failed/partial/blocked`。只记录实际
执行的命令；临时文件不能成为 active/passed 证据的唯一 artifact。EVD 与 IMP 必须双向引用。
upstream IMP 的任一 `code_paths` 变更后必须在新提交上重跑并更新证据，不能沿用旧 passed 结果。
