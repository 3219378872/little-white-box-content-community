---
implementation: IMP-content-community-backend
verified_at: 2026-09-05
verified_commit: 66f4406
commands:
  - make check
  - make test
  - env NO_PROXY=localhost,127.0.0.1,172.28.0.1 no_proxy=localhost,127.0.0.1,172.28.0.1 go test -tags=integration ./app/assistant/internal/runtime -run TestResearchSQLTransactions -count=1 -timeout=5m
  - just e2e-agent-research
  - just e2e-agent-reset
  - env ENV_FILE=/tmp/xbh-dev.env just up
  - env ENV_FILE=/tmp/xbh-dev.env just e2e
result: partial
---

# 社区研究闭环工程证据

## 当前已验证

- `make check` 通过：25 项工程文档检查测试、知识校验、vet、golangci-lint 零问题。
- `make test` 全量 race 测试通过，REST 决策表包含新增 answers 路由的成功、参数与认证边界。
- 隔离 MySQL/Redis 的 `TestResearchSQLTransactions` 通过。从旧列/表形态升级并重复应用
  增量补丁，验证长网页 URL 不截断、并发答案一次接受、恢复后真实工具结果、发布失败整笔回滚、
  重试仅一份正文/展示快照/终止事件，以及清历史删除展示记录。
- 单测覆盖未知/跳过不造偏好、跨用户/问题与重复提交、过期和 Stop、不重开旧任务、历史删除后禁止
  事件恢复、请求标识与参数冲突、交互工具独占一轮、伪造/跨 run/失效证据、危险 URL、原文分页与
  不把随机 handle/时间戳视为新的检索进展。

## 整合后的真实栈复验

后端观察提交为 `66f4406`（代码 `0171e64`，已包含独立生命周期修复），前端为 `7ab3de2`
（代码 `0a31c95`）。`just` 命令从联调编排仓主检出运行，不在无子仓的根 task 工作树中运行。

- 严格 fixture 下研究闭环 4 项通过（6.45s），流式 reset/replay 1 项通过（1.23s）；使用真实
  MySQL、RPC、Gateway、持久 SSE 和迁移，模型与外部检索响应为确定性测试输入。
- 随后按人类要求，仅把本地 `ASSISTANT_LLM_ENDPOINT` 改为 `https://api.weblearning.fun/v1`，
  保留 `glm-5.3-flash`、`responses` 与现有凭据；正常 `just up` 成功且 worker 通过启动 canary。
  核对进程环境与正常 env 一致，fallback 未启用，Tavily 使用默认端点，临时 fixture 已停止。
- 正常 provider 下，消息生成与 SSE 重连单项 1 passed（13.28s）；全量 `just e2e` 为
  116 passed、5 skipped（231.37s），跳过项仅为要求专用 fixture 的研究与 reset 测试。
- 真实 Tavily 请求曾返回 HTTP 200 和一条结果；这只证明搜索服务可达，不是完整外部资料闭环验收。

## 真实模型失败记录

正常 provider 下另发起 protocol v2 社区跑步资料整理请求（本地 run `302`），4 次 `search_posts`
与 14 次 `read_source` 成功。第 5 轮模型调用的三次尝试均为 `timeout`，最终在约 353.03s 后以
`LLM_UNAVAILABLE` 终止；持久事件中 `answer_committed` 为零，没有把未完成检索发布为答案。
该观测不能关闭真实模型长工具链或逐项语义支持质量门禁；未通过延长超时、换模型或放松断言掩盖。

## 证据边界

本页针对整合提交 `66f4406`。结构校验仅证明引用指向实际取得的片段及当前可见来源，不能自动证明
每条自然语言表述在语义上被支持。前端三种视口的 Mock 浏览器通过不等于真实模型浏览器闭环。
人类冻结评测集与生产自然月 SLO 没有因此关闭，整体实现保持 diverged。
