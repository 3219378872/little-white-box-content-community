# Context: {title}

## Objective
Describe the task outcome in one sentence.

## Scope
- In scope:
- Out of scope:

## Related Artifacts
- Spec: none - state why a separate spec is unnecessary.
- Plan: none - state why a separate plan is unnecessary.

## Likely Files
- `path/to/file`

## Safety Constraints
- Logic 层只返回 `errx.New(code, msg)`，禁止裸 `errors.New()`。
- 所有日志用 `logx.WithContext(ctx)`，禁止 `logx.Info/Error` 不带 ctx。
- zrpc 调用透传入参 ctx；goroutine 内使用 ctx 拷贝。
- 不手改 goctl 生成文件（`internal/handler/*`、`internal/types/*.go`、`*.pb.go`）。
- 配置不硬编码，敏感值走环境变量占位。

## Open Questions
- none
