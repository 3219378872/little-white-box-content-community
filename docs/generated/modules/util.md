---
title: util
tracks:
  - pkg/util/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# util

## 职责
跨服务基础工具：雪花 ID 生成、密码哈希、时间格式化、JSON 字段辅助。

## 公开接口与契约
- `InitSnowflake(...)` / `NextID()` — 分布式唯一 ID。
- `HashPassword` / `ComparePassword` / `IsDefaultPassword` — 密码哈希与校验。
- `SHA256(...)` — 通用摘要。
- `FormatTime` / `FormatDateTime` — 时间格式化。
- `jsonField.go` — JSON 字段读写辅助。

## 上游
各服务 Logic（ID 生成、密码、时间序列化）。

## 下游
无（基础库）。

## 关键文件
- `snowflake.go` — 雪花 ID。
- `hash.go` — 密码/摘要。
- `time.go` — 时间格式化。
- `jsonField.go` — JSON 字段辅助。

## 注意事项与陷阱
- 雪花 ID 需在进程启动时 `InitSnowflake` 后再 `NextID`，且 workerId 需全局唯一。
- 密码只存哈希；禁止记录明文或把哈希写入日志。
