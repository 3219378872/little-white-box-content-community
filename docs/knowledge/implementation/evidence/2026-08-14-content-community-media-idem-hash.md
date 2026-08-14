---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 3d7cb50
commands:
  - go test ./app/media/...
  - go test -tags integration ./app/media/rpc/internal/logic/
  - make check
  - make test
result: passed
---

# 2026-08-14 媒体上传幂等命令哈希纳入文件内容指纹（CORE-050/051）

## 缺陷

`mediaIdempotencyRecord` 的 `CommandHash` 只覆盖 filename/quality/maxWidth/
maxHeight，未覆盖文件内容。同一调用者以相同键、相同元数据上传**不同字节**
会被当作"同一命令"静默返回旧媒体，而不是按 CORE-051 返回可区分的幂等冲突；
这与 CORE-042 消息"异命令冲突"及 2026-08-14 帖子幂等哈希完整性修复
（post-idem-hash）确立的方向不一致。

## 修复

- `app/media/rpc/internal/logic/upload_common.go`：
  - 新增 `sha256File`（流式 sha256，恒定内存）；
  - `mediaIdempotencyRecord(meta, contentHash)` 将文件内容指纹并入
    `CommandHash`（与 filename/quality/maxWidth/maxHeight 一起）。
- `upload_image_logic.go` / `upload_video_logic.go`：接收完流后计算
  `sha256File(sink.Path())`，失败返回 SystemError；再构造幂等记录。

## 测试

- `TestMediaIdempotencyRecord`：同键同内容同参数 → 相同哈希；同键不同内容 /
  不同文件名 → 不同哈希（异命令冲突语义）。
- 新增 `TestSHA256File`：确定性、幂等、缺失文件报错。

## 审查证据

- `go test ./app/media/...`：全部通过。
- media 集成测试（MySQL/Redis testcontainers + SeaweedFS）：全部通过
  （含上传图片/视频真实流 + 幂等路径）。
- `make check`：exit 0；`make test`：85 包 0 失败。

## 结果

- 媒体幂等命令哈希现覆盖文件内容；同键异命令按 CORE-051 返回 409 幂等冲突。

## 未覆盖边界

- 行为变更影响真实重试客户端：同键不同字节的重试将得到 409 而非旧资源，
  符合幂等契约语义；客户端应对重试发送相同字节或更换键。
