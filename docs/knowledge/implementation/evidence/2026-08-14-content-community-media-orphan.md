---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: a8b3b3a
commands:
  - go test -count=1 ./app/media/...
  - make check
  - make test
result: passed
---

# 2026-08-14 媒体上传幂等重试孤儿对象清理

## 缺陷

`UploadImage`/`UploadVideo` 的流程是「先上传对象 → 再 CreateMedia 事务」。
对象键由 `uuid.NewString()` 每次随机生成，与幂等键无关。当客户端用同一
`idempotency_key` 重试（首响丢失是正常路径）：

1. 本次上传的新对象（objKey，可能含缩略图 thumbKey）已写入对象存储；
2. `CreateMedia` 幂等命中返回已有媒体记录；
3. 代码直接返回已有记录——**本次上传的对象成为无 DB 引用的孤儿**，
   每次重试泄漏一组对象。

## 修复

- `upload_common.go`：新增 `removeOrphanObjects(ctx, logger, storage, keys...)`
  best-effort 删除辅助（CORE-053：删除失败只告警，不影响成功响应）。
- `upload_image_logic.go` / `upload_video_logic.go`：在 `!result.Created`
  （幂等命中）分支调用 `removeOrphanObjects`，删除本次上传的对象
  （image 含缩略图，video 仅原对象）。

## 审查证据（本轮深入扫描）

- schema 一致性：全部 Model 引用表均存在于 `deploy/sql`；`event_outbox`
  （outboxx 字面量 SQL）、`idempotency`、`personalization_preference`
  确认在用；`favorite_folder`/`report`/`view_history` 为上轮保留的预留表。
- 关注流/推荐游标、recommend 游标 codec（HMAC+绑定+过期）、behavior
  事件链路（确定性事件 ID/白名单/时钟窗口）、interaction 幂等事务
  （归属校验/并发重复键）均审查通过，无新增问题。

## 结果

- `go test ./app/media/...` 全过；`make check` 通过；`make test` 全部模块
  通过（含 race）。

## 未覆盖边界

- 上传后 `CreateMedia` 事务失败（非幂等冲突）时的对象残留仍无清理机制
  （重试会覆盖，属已知取舍）；外部输入门禁不变，见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
