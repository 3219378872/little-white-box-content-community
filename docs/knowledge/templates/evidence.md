# 实现证据模板

此文件只是 agent 使用的格式模板，不是验证结果。

```yaml
---
implementation: IMP-example
verified_at: YYYY-MM-DD
verified_commit: replace-with-git-sha
commands:
  - replace-with-actual-command
result: partial
---
```

正文记录环境、关键输出、未覆盖边界，以及结果为何是 `passed/failed/partial/blocked`。
