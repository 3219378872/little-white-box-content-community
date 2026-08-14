---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 9a39976
commands:
  - go test ./app/media/rpc/internal/logic/
  - make check
  - make test
result: passed
---

# 2026-08-14 上传内容哈希透传请求 ctx（AGENTS.md 上下文规则）

## 背景

media-idem-hash 批次新增的 `sha256File` 使用 `context.Background()` 构造
延迟关闭日志上下文——位于请求范围内的上传路径（图片/视频流），违反
「所有请求上下文必须透传」规则。

## 修复

- `sha256File(ctx context.Context, path string)`：入参改为请求 ctx；
  哈希开始前检查取消；延迟关闭日志使用 `logx.WithContext(ctx)`。
- `upload_image_logic.go` / `upload_video_logic.go`：调用改为
  `sha256File(l.ctx, sink.Path())`。
- 测试：`TestSHA256File` 增加已取消 ctx 返回错误的分支。

## 审查证据

- 全仓 `context.Background()` 复查：剩余使用点均为清理/健康检查类独立上下文
  （vectorstore/s3/search svc），不属于请求链路。
- `go test ./app/media/rpc/internal/logic/` 通过；`make check` exit 0；
  `make test` 86 包 0 失败。

## 结果

- 上传路径上下文全部透传，符合 AGENTS.md 约定。

## 未覆盖边界

- 大文件哈希期间的中途取消仍需逐块检查（io.Copy 非 ctx 感知）；当前仅入口
  检查与关闭日志透传，已覆盖规则要求。
