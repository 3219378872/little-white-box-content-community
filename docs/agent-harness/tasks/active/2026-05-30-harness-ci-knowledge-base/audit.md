# Audit

No human-approved spec exists for this task (spec_waiver); the requirements
below are derived from the goal "参考 ../bidking-controller，完成 harness 改造、CI
门控和知识库建设" and the reference repo's three-pillar design.

| Requirement | Status | Evidence |
| --- | --- | --- |
| Harness pillar: lifecycle CLI (new/check/summary/complete/abandon) | met | `scripts/agent_harness.py` + `scripts/_agent_harness/`; `check` passes, this record created via `new` |
| Harness pillar: docs + templates + rule IDs (H001–H030) | met | `docs/agent-harness/{README,quality-rules,garbage-collection}.md` + `templates/` |
| Harness adapted to go-zero (errx/ctx/layering/goctl/config rules) | met | template Safety Constraints + quality-rules go-zero Compliance section |
| KB pillar: checker with K001–K006 + co-change + sync-report | met | `scripts/knowledge_base.py` + `scripts/_knowledge_base/`; `check` passes |
| KB pillar: one page per backend module, all covered (K002) | met | 22 module pages (10 `app/` services + 12 `pkg/` libs); coverage clean |
| KB pillar: flows + INDEX + frontmatter | met | 4 flow pages, INDEX links all 26 pages, frontmatter validated by K001 |
| CI gating: Go lint gate (gofmt, vet, golangci-lint) | met | `.github/workflows/ci.yml` lint job + `scripts/vet.sh`, `scripts/lint.sh` |
| CI gating: cross-module test gate (race + cover) | met | `scripts/test.sh` over all go.work modules; ci.yml test job |
| CI gating: harness + KB + engineering-lint as blocking lint | met | ci.yml lint job runs all three; co-change K005 on PRs |
| CI gating: weekly KB sync + PR branch cleanup | met | `knowledge-base-sync.yml`, `pr-cleanup.yml` |
| CI gating: Codex completion/compliance review (go-zero prompt) | met | ci.yml codex-review job; fails closed without secrets |
| pre-commit parity | met | `.pre-commit-config.yaml` (gofmt, golangci-lint, python-compile, harness, KB, engineering-lint) |
| No production Go behavior changed | met | only gofmt/goimports + staticcheck/errcheck compliance fixes on pre-existing issues (see context.md); no logic/behavior change; generated handlers untouched |

## Deviations From Reference
- Dropped the strict `diff AGENTS.md CLAUDE.md` mirror gate: the two files
  intentionally differ here (Codex vs Claude header, different skill/subagent
  names). Recorded as a manual GC checklist item instead.
- Module coverage `_expected_modules` scans `app/` + `pkg/` (Go monorepo) rather
  than the reference's single `src/bidking_controller/` package tree.
- goctl-generated handler files (`internal/handler/*_handler.go`) are excluded
  from the lint gate in `.golangci.yml` (alongside `routes.go` / `types/` /
  `*.pb.go`) rather than edited, honoring the "do not hand-edit generated files"
  rule while keeping the repo-wide lint gate green.
