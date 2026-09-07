# 证据页面模板

```yaml
---
id: EVD-example
layer: evidence
title: Example verification
status: active
owner: agent
updated_at: YYYY-MM-DD
observed_commit: <40-character-observed-commit>
scope:
  - unit
commands:
  - <actually-executed-command>
result: passed
coverage:
  - requirements:
      - EXAMPLE-001
    paths:
      - app/example
---
```

## 实际结果与未覆盖边界

保留观察时输入，包含相关测试、配置与共享依赖；禁止缩小旧快照以绕过重验。
公共命令与结果只写一次，独立范围分组；upstream、covers 与反向引用由工具推导。
