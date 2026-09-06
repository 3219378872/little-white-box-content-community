# 证据层

本目录是独立的第五层事实记录。正式知识链为：

```text
INT -> SPEC -> DES -> IMP <-> EVD
```

`EVD-*` 只证明 `observed_commit` 对应提交在列明 `scope` 与命令内的结果。`status: active`
表示可用于当前判断，`superseded` 只保留历史；`result` 为 `passed/partial/failed/blocked`。
代码、契约和运行结果仍是事实权威，证据不能修改上游语义。active/passed EVD 的 upstream IMP
任一 `code_paths` 相对 `observed_commit` 发生已提交或 dirty/untracked 变更时，该证据不再可用于 aligned。

旧 [implementation/evidence](../implementation/evidence/README.md) 保留为 legacy 历史记录，
不自动升级、不参与当前 `aligned` 判定。

## 当前证据

- [EVD-20260906-community-core](EVD-20260906-community-core.md)：社区核心确定性验证。
- [EVD-20260906-content-discovery](EVD-20260906-content-discovery.md)：内容发现确定性验证。
- [EVD-20260906-assistant-agent](EVD-20260906-assistant-agent.md)：Assistant Agent 确定性验证。
- [EVD-20260906-agent-memory](EVD-20260906-agent-memory.md)：Agent Memory 确定性验证。
- [EVD-20260906-agent-watch](EVD-20260906-agent-watch.md)：Agent Watch 确定性验证。
- [EVD-20260906-feedback-reliability](EVD-20260906-feedback-reliability.md)：反馈与可靠性确定性验证。
