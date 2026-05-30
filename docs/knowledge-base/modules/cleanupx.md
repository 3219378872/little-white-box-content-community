---
title: cleanupx
tracks:
  - pkg/cleanupx/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# cleanupx

## 职责
资源关闭与优雅停机辅助，安全地调用 `io.Closer` 与清理函数，吸收 nil 与错误不致 panic。

## 公开接口与契约
- `Close(...)` — 安全关闭 closer（nil 安全，错误不 panic）。
- `Remove(...)` — 注册/执行资源清理。
- `Shutdown(...)` — 优雅停机时统一触发清理。

## 上游
各服务 `main` / svc 装配与停机路径。

## 下游
被托管的 `io.Closer` 与清理回调。

## 关键文件
- `cleanup.go` — 关闭/清理/停机辅助。

## 注意事项与陷阱
- 关闭错误默认吞掉以保证停机健壮（见 `cleanup_test.go`）；需要观测时应在调用点记日志。
- 不要把业务错误处理塞进 cleanup 路径。
