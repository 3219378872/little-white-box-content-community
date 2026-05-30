# Agent Harness Quality Rules

These rules make agent work easier to resume, review, and trust.

## Rule IDs

`scripts/agent_harness.py check` reports each failure with a stable rule ID:

| ID | Rule |
| --- | --- |
| H001 | `task.yaml` exists and parses |
| H002 | `spec` is a `docs/superpowers/specs/` path or null with a `spec_waiver` |
| H003 | `plan` is a `docs/superpowers/plans/` path or null with a `plan_waiver` |
| H004 | the four narrative files exist and are non-empty |
| H005 | `task_branch` matches `task/YYYY-MM-DD-<slug>` |
| H020 | no unresolved placeholder (`TODO`, `TBD`, `fill in`) |
| H021 | no unresolved template guidance (`explain why`, `replace with`) |
| H030 | a completed task record has a terminal status (`done` or `abandoned`) |

`H030` is only emitted by the lighter historical pass
(`check --include-completed`); the other rules apply to active task records.

## What task records capture

Task records are a knowledge-capture system, not a status dashboard. Record
information that is hard to reconstruct later:

- **context.md**: why the task exists, what alternatives were considered, key
  constraints and decisions.
- **audit.md**: risk reasoning, spec-compliance evidence.
- **handoff.md**: key decisions, known risks, next actions for resuming work.
- **verification.md**: which checks were run and their outcomes.

Do not duplicate information that is easily queryable from git or GitHub: PR
URLs, CI status, branch cleanup state, exact test counts, and file lists belong
in git history and `gh pr view`, not in task records.

## Spec First

Complex behavior changes need a design under `docs/superpowers/specs/` before
implementation. If a separate spec is unnecessary, `context.md` must say why.

## Plan First

Multi-step implementation needs a plan under `docs/superpowers/plans/`. If a
separate plan is unnecessary, `context.md` must say why.

## Branch First

Substantial tasks must not be implemented directly on `main`. Use a task branch
named `task/YYYY-MM-DD-short-slug`.

## Re-Audit Before Completion

After implementation, audit the current files and command output against the
spec. Record every relevant requirement in `audit.md`. Do not treat a green
focused test subset as proof that all spec semantics are complete.

## Backend Verification

Default backend verification for the esx Go services is:

```bash
go test ./... -race -cover
go vet ./...
golangci-lint run
```

Record unrelated environment blockers (missing Docker, no testcontainers
runtime) separately from feature results. Integration tests that touch MySQL /
Redis / RPC use testcontainers; record skipped containers with a reason.

## go-zero Compliance (audited by the Codex review job)

Harness task records and their diffs must respect the project's hard rules in
`CLAUDE.md` / `AGENTS.md`:

- Logic 层只返回 `errx.New(code, msg)`；错误码集中在 `pkg/errx/codes.go`。
- 日志一律 `logx.WithContext(ctx)`；zrpc 调用透传 ctx；goroutine 用 ctx 拷贝。
- 分层：Handler 只做参数绑定/调用 Logic/返回；业务逻辑在 Logic；数据访问在 Model。
- 不手改 goctl 生成文件；改 `.api` / `.proto` 后重新生成。
- 配置走 `etc/*.yaml` → `config.Config`；敏感值用 `${ENV_VAR}` 占位，不硬编码。
- 测试覆盖率 ≥ 80%；每个 Logic 至少一条失败路径测试。
