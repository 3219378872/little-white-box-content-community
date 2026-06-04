# Context: golangci-lint Fixes

## Objective
Fix all 17 golangci-lint violations so `golangci-lint run` exits cleanly across the workspace.

## Scope
- In scope: errcheck, gofmt, staticcheck SA5008 issues in non-generated source
- Out of scope: generated code (`.pb.go`, `types.go`, `routes.go`), CI workflow changes

## Related Artifacts
- Spec: none — straightforward lint fix, no design decisions
- Plan: none — single-pass fix-and-verify

## Likely Files
- `app/search/mq/internal/indexer/es_indexer.go`
- `app/content/mq/cleanup/internal/mqs/cleanup_consumer_test.go`
- `pkg/event/post_test.go`
- `app/embedding/mq/internal/config/config.go`
- `app/search/mq/internal/config/config.go`
- `.golangci.yml`

## Safety Constraints
- 不手改 goctl 生成文件（`internal/handler/*`、`internal/types/*.go`、`*.pb.go`）。
- 不修改生产逻辑，仅修正 lint 问题。
- staticcheck SA5008 排除规则仅针对 go-zero 自定义 JSON tag 误报。

## Open Questions
- none
