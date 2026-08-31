---
implementation: IMP-content-community-backend
verified_at: 2026-08-31
verified_commit: f21b68795233b34ee07ffa5c574bb8f06add9ac6
commands:
  - make check
  - make test
  - make coverage
  - PATH=/tmp/xbh-generate-venv.AS6R5I/venv/bin:$PATH make generate
  - make integration-critical
  - INTEGRATION_S3_PORT=18333 NO_PROXY=127.0.0.1,localhost,172.28.0.1 no_proxy=127.0.0.1,localhost,172.28.0.1 make integration-all
  - go test ./deploy -run TestProductionMigrationScriptRequiresPreparedBackupBeforeConfirmation -count=1 -v
result: partial
---

# 2026-08-31 审查整改证据

## 已验证

- `make check` 在实现提交 rebase 到最新 `main` 后通过：engineering policy、文档策略、`go vet` 与
  golangci-lint 均无问题。
- `make test` 在 rebase 前后均通过；refresh token 32 并发只允许一次消费，内部 gRPC unary/stream
  HMAC、Watch stale-version 冲突、图片头预算与容量 2 解码信号量、上传补偿和保留期失败回滚均有回归测试。
- `make coverage` 通过当前门禁：Handler 88.3%、Logic 78.4%、MQ consumer 74.6%、handwritten 53.1%。
- `make generate` 使用隔离 venv 中与仓库兼容的 `grpcio-tools==1.71.0`、`grpcio==1.71.0`、
  `protobuf==5.29.4` 完成；生成差异只包含本次 Watch version 契约。
- `make integration-critical` 通过。隔离的 `make integration-all` 全部通过，包含 Assistant SQL retention
  事务回滚、Watch CAS、ES/Behavior 链路与 15 项模型管线测试，结束后清理 `xbh-test` 环境。
- fake-Docker 迁移行为测试证明：target UUID 不符时在备份前拒绝；准备阶段只生成并验证 Assistant 与
  consent 两份 gzip 备份；未提供 manifest-bound 精确确认值时既不创建 ledger 也不执行补丁；正确确认后
  才应用并登记 checksum。
- production Compose 契约测试证明只有 Nginx 发布 host ports，media-rpc 限制为 512 MiB；MySQL init
  只挂载白名单 schema，认证 healthcheck、CSP/HSTS/no-cache/immutable 头配置均由测试覆盖。

## 部分与未覆盖

- 未启动真实 production profile，也未执行真实生产迁移；备份与确认流程仅在确定性 fake-Docker 行为测试
  中验证，不能替代操作员对真实目标 UUID、备份可恢复性与生产变更窗口的验收。
- 未调用外部 live provider，不证明供应商网关的真实流式、限流或计费行为。
- 本证据不把后续根编排真实栈 E2E 或浏览器验收提前计入；这些结果应由对应提交后的独立证据记录。
