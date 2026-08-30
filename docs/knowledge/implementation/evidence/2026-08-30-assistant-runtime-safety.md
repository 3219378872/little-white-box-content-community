---
implementation: IMP-content-community-backend
verified_at: 2026-08-30
verified_commit: e83e3da6f318
commands:
  - make test
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

验证在 `task/assistant-safety` 工作树执行；frontmatter 的 commit 是验证时工作树基线，安全修复尚未
集成到该 commit。覆盖 Assistant worker、Store、Content update/delete 内部 RPC 幂等、Gateway 撤权
联动和已有 v3 数据卷升级；没有调用 live provider，也没有运行根目录真实浏览器/E2E。

## 通过结果

- `make test`：全仓 race 测试通过；本批新增 generation 抢占、续租丢失取消、pending journal 接管、
  Content RPC 已成功但 journal 未完成、redirect 丢弃旧响应、模型调用中撤权、delete revision 确认、
  Watch terminal bucket/stat 与唯一终态测试。
- `make fmt-check`、`make vet`、`make lint ARGS="--timeout 5m"`：通过，lint 为 `0 issues`。
- `make integration-critical`：通过 interaction logic、user logic、user model 三个隔离 MySQL 包。
- Assistant Store MySQL integration：通过旧 generation fencing、journal takeover，以及 terminal 唯一键
  失败时 run/message/outbox/thread 全事务回滚。
- Content Model MySQL integration：update/delete 各自模拟响应丢失后以同一幂等键重放；revision 只增加
  一次，outbox 只增加一行。
- 使用当前 `HEAD` 的旧 v3 `xbh_assistant.sql` 创建临时数据库，连续执行两次
  `20260830_assistant_run_fencing.sql` 均成功；检查得到 run 新列 3 项、terminal 唯一索引、journal 新列
  3 项和 Memory command 去重索引。临时数据库随后删除并确认不存在。
- `goctl rpc protoc` 分别重新生成 Assistant 与 Content protobuf/zrpc，相关全包测试通过。

## 部分与未覆盖

- `make generate` 未全量执行：环境缺少仓库既有开发依赖 `grpc_tools.protoc`；未安装新依赖。所涉及的
  Assistant/Content proto 已用对应 `goctl rpc protoc` 命令生成并经编译测试。
- `make check` 在本批修改前即存在的知识迁移问题处停止：两份 2026-08-29 evidence 使用非 SHA
  `verified_commit`、实现页缺退休规范标题、历史页面引用已删除路径。fmt/vet/lint 已拆分执行并通过；
  这些知识问题等待并入同步完成度分支后重跑。
- 未验证 live LLM、真实 SSE 代理、根栈浏览器流程、生产 profile 或生产迁移。
