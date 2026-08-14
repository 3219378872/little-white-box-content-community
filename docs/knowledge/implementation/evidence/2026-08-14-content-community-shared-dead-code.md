---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: fa72c9e
commands:
  - go build ./...
  - make check
  - make test
  - go test -count=1 ./pkg/util/... ./pkg/middleware/... ./pkg/validator/... ./pkg/interceptor/...
result: passed
---

# 2026-08-14 共享库死代码清理

## 本批次改动

- 删除 `pkg/cachex/` 整个模块（`keys.go` 全部缓存 key 常量与 `BuildKey`/
  `toString`/`intToStr`，`go.mod`/`go.sum`）：全仓（含测试）零 import，
  仅 `go.work use` 注册与 IMP-architecture 清单提及。同步移除
  `go.work` 中的 `./pkg/cachex`，并更新
  `IMP-architecture.md` 共享库清单。
- 删除 `pkg/util` 中 `time.go` 文件（14 个时间工具导出函数：`FormatTime`、
  `ParseDateTime`、`NowUnix`、`UnixToTime`、`IsToday`、`StartOfDay` 等）
  与 `pkg/util/hash.go` 的 `SHA256`：全部零引用（含测试）。
- 删除 `pkg/middleware/auth.go` 的 `AuthMiddleware` 与 `writeUnauthorized`：
  全仓零引用（gateway 使用 `NewOptionalAuthMiddleware` 替代）。
- 删除 `pkg/validator/validator.go` 的 `IsUsernameValid`/`IsPasswordValid`
  与 `usernameRegex`：零引用（注册路径使用 `CheckPasswordStrength`/
  `ValidateUserName`）。

## 保留项（测试引用）

- `pkg/middleware.CORSMiddleware`：`cors_test.go` 三个测试直接使用，保留。
- `pkg/interceptor.GetTraceID`：`trace_middleware_test.go` 引用，保留。
- `pkg/mqx.GroupBehaviorLogService`（上轮保留），`pkg/errx` 错误码（对外
  契约，观察不改）。

## 结果

- `go build ./...` 通过；`make check` 通过（fmt/engineering-lint/vet/
  golangci-lint 0 issues）；`make test` 全部模块通过（含 race，cachex
  模块随删除退出 workspace 遍历）。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
