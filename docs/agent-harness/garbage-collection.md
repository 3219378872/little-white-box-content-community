# Agent Harness Garbage Collection

Garbage collection keeps the agent record system useful instead of letting it
become another stale documentation tree.

Run this review manually when finishing a task, after long interruptions, or
before a larger merge.

## Checks

- Active task older than the expected work window with no recent evidence.
- Completed task still under `docs/agent-harness/tasks/active/`.
- Spec without a plan when the scope implies implementation.
- Plan without any active or completed task reference.
- Audit without verification evidence.
- Verification without concrete command output or an explicit skipped-command
  reason.
- `AGENTS.md` and `CLAUDE.md` drift (the two harness instruction files are kept
  in sync apart from their tool-specific header and skill names).
- Stale placeholder text in harness documents.

## Commands

```bash
python3 scripts/agent_harness.py summary
python3 scripts/agent_harness.py check
```

`summary` is advisory and exits 0. Its `GC notes` section automates most of the
checklist above: stale non-terminal active tasks, `done`/`abandoned` tasks
misplaced under `active/`, audits missing verification evidence, and spec/plan
drift. The remaining checklist items are not part of `summary`: stale
placeholder text in task records is flagged by `check` (rules `H020`/`H021`),
and `AGENTS.md`/`CLAUDE.md` drift is reviewed manually because the two files
intentionally differ in their header line and harness/skill names. `check` is
stricter for active task records and exits non-zero when a record is
incomplete.

## Cleanup Policy

Prefer fixing the task record over deleting it. Move a task to `completed/` only
after verification and audit are current. If a task is abandoned, leave a short
handoff note explaining why, then move it to `completed/` so future agents do
not treat it as active work.
