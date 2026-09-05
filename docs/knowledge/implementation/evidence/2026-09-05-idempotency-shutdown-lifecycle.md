---
implementation: IMP-content-community-backend
verified_at: 2026-09-05
verified_commit: f5105a3847d8d8ddaa01333780f16eecb864f7aa
commands:
  - make check
  - make engineering-lint
  - make test
  - make coverage
  - go test -race -count=1 ./pkg/outboxx ./app/recommend/mq ./app/pipeline/behaviorlog ./app/content/mq/cleanup
  - go test -race ./pkg/outboxx ./app/content/rpc/... ./app/interaction/rpc/... ./app/user/rpc/... ./app/media/rpc/... ./app/recommend/mq ./app/pipeline/behaviorlog ./app/content/mq/cleanup
  - go test -race -tags=integration -count=1 -run TestResolveIdempotencySessionSeesConcurrentWinnerUnderRepeatableRead ./pkg/idempotencyx
  - go test -tags=integration -count=1 ./pkg/idempotencyx
result: passed
---

# 2026-09-05 幂等并发与停机生命周期修复证据

## 验证范围

- `pkg/idempotencyx` 在 MySQL 默认 REPEATABLE READ 下处理并发同键竞争时，唯一键失败后的回查使用
  `SELECT ... FOR UPDATE` current read；初次查询仍为普通 consistent read。确定性 MySQL 8 集成测试构造
  “胜出事务未提交、失败事务建立旧快照、唯一键等待、胜出事务提交”的窗口，并返回胜出 resource ID。
- `pkg/outboxx.RelayHandle` 对后台 relay 执行 cancel + join。在途 publish 未返回时 `Stop` 不会提前返回；
  context 取消后 relay 不再 Claim 或查询 backlog，正常停机不记 backlog failure。
- Content、Interaction、User、Media RPC 的退出顺序为停止 server、停止并等待 relay、关闭 MQ producer、
  关闭数据库；Media RPC 补齐了 producer 与数据库关闭路径。
- Assistant、Feed、Media、Search、Recommend 和 Content cleanup MQ 入口响应进程结束信号。Content cleanup
  明确先停两个 consumer 再关数据库；recommend 清理与 behaviorlog 聚合继承父 context 并等待在途任务退出。

## 门禁结果

- `make check` 通过：知识策略、格式检查、`go vet` 与 golangci-lint 均通过，lint 为 `0 issues`。
- `make engineering-lint` 通过：25 个治理单测与知识层主检查均通过。
- `make test` 全仓 race-enabled 测试通过。
- `make coverage` 通过既有基线：handwritten 53.6%、handler 88.3%、logic 78.1%、model 12.0%、
  mq_consumer 73.6%、shared 60.7%。
- 两组聚焦 race 命令通过；覆盖 relay cancel/join、停机后零 store 访问、周期任务取消和 cleanup 关闭顺序。
- `pkg/idempotencyx` 聚焦 race + MySQL 8 集成用例通过；完整该包 integration 命令通过（13.965s）。

## 行为与未覆盖边界

- 未修改公开 API、protobuf、数据库 schema、配置格式或正常业务返回；current read 只进入唯一键竞争失败路径。
- 未运行完整 `integration-all`、根仓黑盒 E2E、真实 RocketMQ 进程 SIGTERM 或 production profile。
- 未做正式性能基准。竞争回查会持有胜出幂等行锁直到外层事务结束；这是同键串行语义的一部分，但本证据
  不证明高竞争吞吐。
- relay join 仍依赖数据库和 RocketMQ 调用遵守 context 或配置的发送超时；本地 fake/race 测试不等于真实
  broker 故障下的退出时延证据。
