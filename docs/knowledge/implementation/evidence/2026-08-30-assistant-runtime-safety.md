---
implementation: IMP-content-community-backend
verified_at: 2026-08-30
verified_commit: 397b9eb630b9ea5626d24483fdf9f79ae9c1678f
commands:
  - PATH=/tmp/xbh-assistant-generate-venv/bin:$PATH make generate
  - make check
  - make test
  - make coverage
  - make fmt-check
  - make vet
  - make lint ARGS="--timeout 5m"
  - make integration-critical
  - go test -tags=integration ./app/assistant/internal/store -run 'TestSQL(LeaseGenerationAndJournalTakeover|TerminalFailureRollsBackRunMessageOutboxAndThread)' -count=1
  - go test -tags=integration ./app/content/rpc/internal/model -run 'TestPostCommandModel(UpdatePostIdempotency|DeletePostIdempotency)SurvivesLostResponse' -count=1
result: partial
---

# Assistant runtime safety

## 环境与范围

验证在 `task/assistant-safety` 工作树执行；frontmatter 的 commit 包含完整性分支 rebase 后的安全修复
和输入接收幂等收尾。覆盖 Assistant worker、Store、Content update/delete 内部 RPC 幂等、Gateway 撤权
联动和已有 v3 数据卷升级；没有调用 live provider，也没有运行根目录真实浏览器/E2E。

## 通过结果

- `make test`：全仓 race 测试通过；本批新增 generation 抢占、续租丢失取消、pending journal 接管、
  Content RPC 已成功但 journal 未完成、redirect 丢弃旧响应、模型调用中撤权、delete revision 确认、
  Watch terminal bucket/stat 与唯一终态测试。
- `make fmt-check`、`make vet`、`make lint ARGS="--timeout 5m"`：通过，lint 为 `0 issues`。
- `make generate`、`make check`：完整生成、知识策略、vet 和 lint 通过。
- `make coverage`：通过；Handler 88.4%、Logic 78.2%、MQ consumer 74.6%。覆盖运行前修复了本地
  gRPC fault fixture 未等待 READY 导致的 deadline/unavailable 抖动，不修改推荐降级产品语义。
- `make integration-critical`：通过 interaction logic、user logic、user model 三个隔离 MySQL 包。
- Assistant Store MySQL integration：通过旧 generation fencing、journal takeover，以及 terminal 唯一键
  失败时 run/message/outbox/thread 全事务回滚。
- Content Model MySQL integration：update/delete 各自模拟响应丢失后以同一幂等键重放；revision 只增加
  一次，outbox 只增加一行。
- 使用当前 `HEAD` 的旧 v3 `xbh_assistant.sql` 创建临时数据库，连续执行两次
  `20260830_assistant_run_fencing.sql` 均成功；检查得到 run 新列 3 项、terminal 唯一索引、journal 新列
  3 项和 Memory command 去重索引。临时数据库随后删除并确认不存在。
- `goctl rpc protoc` 分别重新生成 Assistant 与 Content protobuf/zrpc，相关全包测试通过。
- `assistant_input_command` 保存每个用户 requestId 的 message/run/disposition；redirect/steer/queued
  重试返回同一接受结果，不重复消息或再次改写 input version。FIFO 只删除本次已读取到的队列范围，
  不误删并发新输入，已消费条目不永久占用 32 条上限。

## 部分与未覆盖

- 宿主环境原本缺 `grpc_tools.protoc`；复用完整性分支在 `/tmp` 创建的一次性 venv 后完整
  `make generate` 已通过，未修改仓库依赖。
- 未验证 live LLM、真实 SSE 代理、根栈浏览器流程、生产 profile 或生产迁移。
