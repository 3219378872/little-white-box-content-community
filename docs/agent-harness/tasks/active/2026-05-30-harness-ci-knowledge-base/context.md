# Context: Harness Ci Knowledge Base

## Objective
Stand up three agent-development pillars on esx — an agent harness, CI gating,
and a code knowledge base — modeled on the reference repo `../bidking-controller`.

## Scope
- In scope:
  - Agent harness: `scripts/agent_harness.py` + `scripts/_agent_harness/` package,
    `docs/agent-harness/` (README, quality-rules, garbage-collection, templates).
  - Knowledge base: `scripts/knowledge_base.py` + `scripts/_knowledge_base/`,
    `docs/knowledge-base/` (README, INDEX, one page per `app/`+`pkg/` module, flows).
  - CI gating: `.github/workflows/` (ci, knowledge-base-sync, pr-cleanup),
    `.pre-commit-config.yaml`, `scripts/engineering-lint.py`, `scripts/test.sh`,
    `scripts/vet.sh`, `scripts/lint.sh`, `.golangci.yml` exclusions.
  - Behavior-neutral compliance fixes required to make the now-repo-wide gofmt
    and golangci-lint gates green on arrival (see below). No logic/behavior
    changes.
- Out of scope:
  - Any change to Go behavior / business logic under `app/` or `pkg/`.
  - Editing goctl-generated files (`internal/handler/*_handler.go`, `routes.go`,
    `internal/types/*.go`, `*.pb.go`) — these are excluded from the lint
    gate in `.golangci.yml` instead.
  - Wiring the Codex review secrets (OPENAI_API_KEY / endpoint) — repo settings.

## Compliance Fixes (in scope, behavior-neutral)
Turning the lint gate from root-only to repo-wide surfaced pre-existing issues
in previously-unlinted modules. Fixed without behavior change:
- gofmt: `pkg/middleware/cors.go`, `app/user/rpc/internal/model/user_follow_model.go`,
  `app/content/rpc/internal/logic/integration_test.go`.
- goimports: `app/gateway/internal/logic/login/*_test.go`,
  `app/user/rpc/internal/config/config.go`.
- staticcheck/errcheck: `pkg/errx/errors.go` (tagged switch), `pkg/util/hash.go`
  (`return err == nil`), `pkg/middleware/auth.go` (`_, _ = w.Write`).
Generated handler files were NOT edited; they are excluded from the gate.

## Related Artifacts
- Spec: none — see `spec_waiver`. The design is established in
  `../bidking-controller` (harness-refactor, harness-ci-workflow, knowledge-base,
  agent-first-harness design docs).
- Plan: none — see `plan_waiver`.

## Likely Files
- `scripts/_agent_harness/`, `scripts/_knowledge_base/`
- `docs/agent-harness/`, `docs/knowledge-base/`
- `.github/workflows/`, `.pre-commit-config.yaml`

## Adaptation Decisions
- Tooling stays Python (language-agnostic meta-tooling); python3.12 is available.
- `_expected_modules` (K002) scans `app/<service>/` (Go-owning) and `pkg/<lib>/`
  instead of the reference's `src/bidking_controller/` layout.
- CI runs Go gates (gofmt, `go vet`, golangci-lint, `go test -race -cover` across
  all go.work modules) in place of the reference's Python/Node gates.
- The reference's strict `diff AGENTS.md CLAUDE.md` mirror gate is dropped: in
  this repo the two files intentionally differ (Codex vs Claude header, different
  skill/subagent names). Drift is a manual GC checklist item instead.

## Safety Constraints
- Logic 层只返回 `errx.New(code, msg)`，禁止裸 `errors.New()`。
- 所有日志用 `logx.WithContext(ctx)`，禁止 `logx.Info/Error` 不带 ctx。
- zrpc 调用透传入参 ctx；goroutine 内使用 ctx 拷贝。
- 不手改 goctl 生成文件（`internal/handler/*`、`internal/types/*.go`、`*.pb.go`）。
- 配置不硬编码，敏感值走环境变量占位。

## Open Questions
- none
