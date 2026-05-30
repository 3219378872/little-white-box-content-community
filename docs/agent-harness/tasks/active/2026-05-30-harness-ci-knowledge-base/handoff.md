# Handoff: Harness Ci Knowledge Base

## Status
Active — implementation complete on `task/2026-05-30-harness-ci-knowledge-base`,
pending PR + CI.

## Key Decisions
- Ported the three pillars from `../bidking-controller`; kept tooling in Python.
- Dropped the strict AGENTS.md/CLAUDE.md mirror gate (files differ by design here).
- KB covers all 22 backend modules (10 `app/` services + 12 `pkg/` libs) + 4 flows.
- Cross-module `go test` driven by `scripts/test.sh` because the repo is a
  multi-module go.work workspace (root `./...` would miss `app/gateway`,
  `app/user/rpc`, and per-package `pkg/*` modules).

## Known Risks
- Codex review job is inert until `OPENAI_API_KEY` / `OPENAI_RESPONSES_API_ENDPOINT`
  repository secrets are configured; the step fails closed if they are absent.
- `golangci-lint` / `go test` were not run locally if the Go toolchain or network
  is unavailable in this environment — see verification.md for what actually ran.

## Next Action
Open the PR, let CI run the Go + harness + KB gates, then move this task to
`completed/` via `python3 scripts/agent_harness.py complete`.
