---
implementation: IMP-content-community-backend
verified_at: 2026-08-30
verified_commit: 1992b06c955a812f25b0cad8ec096ca1a883f564
commands:
  - make check
  - make test
  - make coverage
  - PATH=/tmp/xbh-assistant-generate-venv/bin:$PATH make generate
  - make integration-critical
  - INTEGRATION_S3_PORT=18333 make integration-all
  - NO_PROXY="$NO_PROXY,172.28.0.1" no_proxy="$no_proxy,172.28.0.1" go test -tags=integration -count=1 -run '^TestESIndexer_Delete_RemovesDoc$' -timeout=10m ./app/search/mq/internal/indexer
  - NO_PROXY="$NO_PROXY,172.28.0.1" no_proxy="$no_proxy,172.28.0.1" go test -tags=integration -count=1 -run '^TestBehaviorRPCFanoutPersistsCorrelatedRawEventAndFeaturesExactlyOnce$' -timeout=12m ./integration
  - go test -race ./app/assistant/internal/canonical ./app/assistant/internal/llm ./app/assistant/internal/prompt ./app/assistant/internal/runtime ./app/assistant/internal/store ./app/assistant/internal/tool ./app/assistant/worker/internal/config ./app/assistant/worker/internal/svc ./app/gateway/internal/logic/assistant ./deploy
result: partial
---

# Agent provider reliability

## 范围

验证 Hermes 对比后选择引入 Little 的可靠性设计：版本化 system prompt、不可信 Memory/summary sidecar、
冻结 capability/tool/provider snapshot、统一工具 metadata 与严格 JSON Schema、通用 no-progress guard、
Chat Completions/Responses 流式聚合、typed retry/Retry-After/fallback、启动 canary、usage 分桶、compact
锚点，以及 `streamId`/`response_reset`/lease fencing。代码提交同时保留 Little 的用户/数据库边界，未引入
Hermes profile、外部 Memory、cron/subagent/MoA、terminal/browser/code execution 或动态插件。

## 已验证

- `make check`：engineering lint、vet、golangci-lint 通过，`0 issues`。
- `make test`：全仓测试通过；新增 provider、prompt、runtime、tool、worker config 与生产 Compose 测试通过。
- `make coverage`：基线通过，Handler 88.3%、handwritten 53.1%、Logic 78.4%、MQ consumer 74.6%。
- 定向 `go test -race` 覆盖 canonical、provider、prompt、runtime、store、tool、worker config/svc、
  Gateway Assistant logic 与 deploy，全部通过。
- `INTEGRATION_S3_PORT=18333 make integration-all` 完整通过并清理 `xbh-test`：包含 Assistant SQL、
  ES indexer、Behavior 相关链路，以及离线训练、模型 registry、在线热加载和回滚测试。
- `make generate`：Assistant proto/gateway 生成同步通过；生成差异仅为新增 `stream_id`。
- Assistant SQL integration 覆盖 cache-write/reasoning/last-prompt/usage-estimated 字段往返；新增 patch 为
  `information_schema` 守卫的幂等加列，不回写历史 usage。
- provider fixture 覆盖两种 WireAPI 的非流式/流式 tool call、跨 chunk 参数与 sidecar scrub、truncated
  stream retry、typed errors、Retry-After、兼容 fallback、启动 canary tool-result replay 和冻结 route。
- runtime/tool 测试覆盖首 delta 持久化、retry/reset、redirect/lease fencing、恢复重放、严格 schema、
  Snowflake canonical digest、结构化结果截断，以及 user/Watch/review 的 no-progress guard。
- ES 删除与 Behavior 全链路两个既有集成夹具已按 revision 与内部 HMAC 契约修正，定向复跑通过。

## 部分与未覆盖

- 合并后三仓根真实栈的 `just up`、`just status`、确定性 reset gate 与 Assistant E2E 尚待执行。
- 未调用外部 live provider，不证明特定供应商网关的真实流式兼容、限流头或计费字段。
- 未执行 production profile、真实生产迁移或生产流量；fallback 默认关闭，启用时必须显式配置同
  boundary route，并由启动 canary 验证。
