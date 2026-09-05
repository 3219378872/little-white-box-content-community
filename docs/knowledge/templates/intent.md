# 意图页面模板

此文件是意图格式模板，不是正式意图。人类开发者可以亲自填写，也可以通过当前对话中的自然语言
指令授权 agent 填写或维护；不需要额外授权文件。

```yaml
---
id: INT-example
layer: intent
title: Replace with a human-defined intent
status: draft
owner: human
upstream:
---
```

正文至少说明产品价值、能力、优先级、边界和非目标。工程指标进入规范，内部机制进入设计。
只有人类明确接受、批准或要求发布正式意图后，
状态才可改为 `approved`；实际由 agent 执笔时 `owner` 仍为 `human`。
