---
implementation: IMP-content-community-backend
verified_at: 2026-09-05
verified_commit: cf644b1
commands:
  - make check
  - make test
  - env NO_PROXY=localhost,127.0.0.1,172.28.0.1 no_proxy=localhost,127.0.0.1,172.28.0.1 go test -tags=integration ./app/assistant/internal/runtime -run TestResearchSQLTransactions -count=1 -timeout=5m
result: partial
---

# 社区研究闭环工程证据

## 当前已验证

- `make check` 通过：25 项工程文档检查测试、知识校验、vet、golangci-lint 零问题。
- `make test` 全量 race 测试通过，REST 决策表包含新增 answers 路由的成功、参数与认证边界。
- 隔离 MySQL/Redis 的 `TestResearchSQLTransactions` 通过（15.233s）。从旧列/表形态升级并重复应用
  增量补丁，验证长网页 URL 不截断、并发答案一次接受、恢复后真实工具结果、发布失败整笔回滚、
  重试仅一份正文/展示快照/终止事件，以及清历史删除展示记录。
- 单测覆盖未知/跳过不造偏好、跨用户/问题与重复提交、过期和 Stop、不重开旧任务、历史删除后禁止
  事件恢复、请求标识与参数冲突、交互工具独占一轮、伪造/跨 run/失效证据、危险 URL、原文分页与
  不把随机 handle/时间戳视为新的检索进展。

## 证据边界

本页针对代码提交 cf644b1。结构校验仅证明引用指向实际取得的片段及当前可见来源，不能自动证明
每条自然语言表述在语义上被支持。真实联调、浏览器、真实模型和互联网搜索的结果须另行补录。
人类冻结评测集与生产自然月 SLO 没有因此关闭，整体实现保持 diverged。
