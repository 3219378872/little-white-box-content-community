---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 2f29ee3
commands:
  - make check
  - make test
  - go test -tags integration ./app/media/rpc/internal/logic/
result: passed
---

# 2026-08-14 Media SoftDelete RowsAffected 错误不再静默吞掉

## 背景

全仓命令模型扫描：`RowsAffected()` 的错误在所有命令模型中均被传播
（content/user/interaction/message/outboxx），唯一例外是
`app/media/rpc/internal/model/media_command_model.go` 的 `SoftDelete`——
`rowsAffected, _ := result.RowsAffected()` 静默吞错。若 RowsAffected 失败，
rowsAffected 被当作 0，会跳过 outbox 事件投递（S3 清理事件丢失），且调用方
无法感知。

## 本批次改动

- `SoftDelete` 改为传播 `RowsAffected()` 错误：无法确认删除是否发生时事务回滚、
  返回错误，不静默跳过事件投递。

## 审查证据

- 全仓 `RowsAffected()` 使用点逐一核对：其余 15 处均传播错误，本处为唯一吞错点。
- `go build ./app/media/...`、`go test ./app/media/...`（logic/model/mq 等）通过。
- `make check` 通过；`make test` 85 包 0 失败；media 集成测试
  `TestDeleteMedia_Integration` 通过（软删 + event_outbox 行断言）。

## 结果

- `make check`：exit 0。
- `make test`：85 包全部 ok。
- media 集成测试：`ok`。

## 未覆盖边界

- `RowsAffected` 失败路径依赖真实驱动行为，未单独构造故障注入（无 sqlmock
  依赖）；修复与全仓既有模式一致。
