# Agent Harness

This directory is the repo-local record system for agent-first development on
the **esx** (little-white-box) platform. It does not replace
`docs/superpowers/specs/` or `docs/superpowers/plans/`. Instead, it records the
execution lifecycle around those artifacts so a later agent or human can
understand current work without reading the chat transcript.

## When To Use It

Create or update a harness task record for substantial work:

- behavior changes (new Handler / Logic / Model)
- multi-file refactors
- new `.api` / `.proto` and goctl generation
- middleware / JWT / context-propagation changes
- database / cache / RPC / message-queue changes
- runtime debugging
- safety-sensitive changes (error-code mapping, rate limiting, auth)

Tiny documentation-only edits may skip a task record when the reason is obvious.

## Task Records

Active task records live under:

```text
docs/agent-harness/tasks/active/
```

Completed task records move to:

```text
docs/agent-harness/tasks/completed/
```

Each task record contains four narrative files plus one structured file:

- `task.yaml`: machine-readable task metadata (see Task Metadata below).
- `context.md`: objective, scope, related spec/plan, relevant files, evidence to
  inspect first, safety constraints, and open questions.
- `verification.md`: commands run, results, evidence, skipped commands, and
  environment blockers.
- `audit.md`: requirement-by-requirement review against the relevant spec or
  explicit no-spec reason.
- `handoff.md`: current status, key decisions, known risks, and next action.

## Task Metadata

`task.yaml` holds the machine-readable fields so validation never parses prose
out of the narrative files. Fields:

- `slug`: full task slug (`YYYY-MM-DD-short-slug`).
- `status`: `active`, `done`, or `abandoned`.
- `task_branch`: must match `task/YYYY-MM-DD-<slug>`.
- `spec`, `plan`: paths under `docs/superpowers/`, or `null` with a
  `spec_waiver` / `plan_waiver` reason.

The four `.md` files stay purely narrative; the checker reads `task.yaml` for
all structured rules. PR URL, CI status, and branch cleanup state belong in git
history and `gh pr view`, not in the task record.

## Delivery Workflow

Every substantial task must follow the branch-to-PR loop:

1. Create a task branch from `main` named `task/YYYY-MM-DD-short-slug`.
2. Create or update the active harness task record.
3. Implement in the task branch (TDD: RED → GREEN → REFACTOR).
4. Run local acceptance and record exact commands in `verification.md`:
   - `go test ./... -race -cover`
   - `go vet ./...`
   - `golangci-lint run`
5. Push `origin/task/YYYY-MM-DD-short-slug`.
6. Open a pull request and record the PR URL in `handoff.md`.
7. Pass CI, including the Codex completion-and-compliance review.
8. After merge, the remote branch is auto-deleted by `pr-cleanup.yml`; move the
   task to `completed/`.

## CLI

```bash
# Create an active task record
python3 scripts/agent_harness.py new harness-refactor

# Validate task records (read-only; CI lint gate)
python3 scripts/agent_harness.py check
python3 scripts/agent_harness.py check --include-completed

# Advisory state summary (always exits 0; emits GC notes)
python3 scripts/agent_harness.py summary

# Complete a finished task / abandon a dropped one
python3 scripts/agent_harness.py complete 2026-05-30-harness-refactor
python3 scripts/agent_harness.py abandon 2026-05-30-short-slug --reason "superseded"
```

`check` is intentionally read-only. By default it validates only active task
records; `--include-completed` adds a lighter historical pass. It reports
missing files, placeholders, missing spec/plan references, and invalid
`task.yaml` fields without rewriting anything. Each issue carries a stable rule
ID (see [quality-rules.md](quality-rules.md)). CI runs it as part of the `lint`
job, so harness record failures are treated as workflow lint failures.

`complete` validates that a task passes `check`, has a passing verification row,
and has audit rows before moving it to `completed/` with `status: done`.
`abandon` records a reason and moves the task to `completed/` with
`status: abandoned`.

## Relationship To Superpowers Docs

Use `docs/superpowers/specs/` for approved designs and
`docs/superpowers/plans/` for multi-step implementation plans. A harness task
record links those files from `context.md` and uses `audit.md` to prove the
implemented state satisfies the spec.

## Completion Rule

Do not move a task from `active/` to `completed/` until verification evidence
and spec audit evidence are current. A passing focused test is useful evidence,
but it does not replace a requirement-by-requirement audit when a spec exists.
