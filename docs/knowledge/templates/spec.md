# 规范页面模板

此文件是规范格式模板，不是正式规范。人类开发者可以亲自填写，也可以通过当前对话中的自然语言
指令授权 agent 填写或维护；不需要额外授权文件。

```yaml
---
id: SPEC-example
layer: spec
title: Replace with a human-defined specification
status: draft
owner: human
upstream:
  - INT-example
---
```

正文至少说明可观察行为、约束、失败行为、兼容边界和验收标准。只有人类明确接受、批准或要求
发布正式规范后，状态才可改为 `approved`；实际由 agent 执笔时 `owner` 仍为 `human`。
