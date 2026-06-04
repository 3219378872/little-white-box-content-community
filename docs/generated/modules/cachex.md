---
title: cachex
tracks:
  - pkg/cachex/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# cachex

## 职责
集中定义缓存键前缀与键构造，避免各服务自行拼接键导致命名冲突或前缀漂移。

## 公开接口与契约
- `BuildKey(...)` — 按统一规则拼接带业务前缀的缓存键。

## 上游
各服务 Logic / Model 在读写 Redis 或 go-zero `CachedConn` 时构造键。

## 下游
Redis。

## 关键文件
- `keys.go` — 前缀常量与 `BuildKey`。

## 注意事项与陷阱
- 新增缓存键必须经 `BuildKey`，禁止裸字符串拼接。
- 键前缀变更属于破坏性操作，需考虑灰度与旧键清理。
