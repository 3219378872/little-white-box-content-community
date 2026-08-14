---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 900ac7e
commands:
  - make check
  - make test
  - go test -tags integration ./app/media/rpc/internal/logic/
result: passed
---

# 2026-08-14 Media 删除事件接入事务 outbox（架构一致性）

## 背景

`DeleteMedia` 此前在软删提交后调用 `MQProducer.SendOneWay` 投递 `media-deleted`
（触发 S3 清理）。提交与投递之间进程崩溃/重启会导致事件永久丢失，S3 对象成为
孤儿——这是全仓唯一未走 outbox 模式的权威业务事件，与架构约定
「权威业务事务通过 outbox 同事务投递」偏离（此前已登记为观察项）。

## 本批次改动

- `app/media/rpc/internal/model/media_command_model.go`：
  `SoftDelete` 在同一事务内执行 `UPDATE media SET status=0 WHERE id=? AND status=1`
  与 `outboxx.Enqueue`；`rowsAffected=0`（并发重复删除）或 `event.ID==0`
  （无 S3 对象可清理）时不投递事件，保持幂等。
- `app/media/rpc/internal/logic/delete_media_logic.go`：
  改为 `buildMediaDeletedOutboxEvent`（`util.NextID` 事件 ID + 幂等键）+
  `MediaCommandModel.SoftDelete`；删除 SendOneWay；无对象键时仅软删不投递；
  事务提交后 `MediaModel.DelCache` 失效读缓存（保持原 `UpdateStatus` 的
  `ExecCtx` 缓存语义）。
- `app/media/rpc/internal/svc/service_context.go` + `media.go` + `config.go` +
  `app/media/rpc/etc/media.yaml`：outbox store/relay 装配（producer → RocketMQ），relay
  协程随服务启动；`deploy/sql/xbh_media.sql` 新增 `event_outbox` 表。
- `pkg/event/media.go`：共享 `MediaDeletedEvent`（含 JSON 契约单测），
  media rpc 与 media mq 消费端共用，删除重复结构体。

## 审查证据

- 单测：`pkg/event` 载荷契约、`buildMediaDeletedOutboxEvent`（topic/tag/key/
  payload）、owner 软删 + outbox 事件 + DelCache、无对象键仅软删、命令模型
  错误映射 SystemError、已删幂等不投递。
- 集成测试（testcontainers MySQL/Redis + SeaweedFS）：`TestDeleteMedia_Integration`
  全部通过，其中「成功软删并同事务写 outbox」断言 `event_outbox` 中存在
  `media-deleted` 记录；GetMedia 删除后返回 MediaNotFound（缓存已失效）。
- `make check` 通过；`make test` 85 包 0 失败（含 `app/media/...`、
  `pkg/event`、`pkg/outboxx`）。

## 结果

- `make check`：exit 0（fmt/engineering-lint/vet/golangci-lint 0 issues）。
- `make test`：85 包全部 ok，无 FAIL。
- media 集成测试：`ok`（删除 + outbox 行断言）。

## 未覆盖边界

- relay 投递端到端（RocketMQ）未在本批集成环境验证（MQ 未纳入 testcontainers）；
  outbox relay 逻辑复用 `pkg/outboxx`（已有 `outbox_integration_test.go`）。
- S3 对象实际删除仍由 media mq 消费端执行，消费端单测覆盖重试/幂等语义。
