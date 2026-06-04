# Codex Command Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Codex the canonical workflow entrypoint for repository task playbooks while keeping `.claude/commands/` as a thin Claude compatibility layer.

**Architecture:** Introduce a repository-level `AGENTS.md` entrypoint, make `.agents/skills/gozero-project-commands/` the single source of truth for workflow playbooks, rewrite the `.agents` references to Codex-native language, and replace the duplicated `.claude/commands/*.md` bodies with shims that forward to the canonical `.agents` references. Keep `.claude/settings*.json` untouched because they are tool-local permissions, not repository workflow logic.

**Tech Stack:** Markdown docs, Codex repository skills, Claude compatibility shims, `rg`, `git`, `goctl`, PowerShell, `apply_patch`

---

## File Structure

**Create**
- `AGENTS.md`
- `docs/superpowers/plans/2026-04-21-codex-command-migration.md`

**Modify**
- `CLAUDE.md`
- `.agents/skills/gozero-project-commands/SKILL.md`
- `.agents/skills/gozero-project-commands/references/commands/zero-init.md`
- `.agents/skills/gozero-project-commands/references/commands/new-api-gozero.md`
- `.agents/skills/gozero-project-commands/references/commands/update-api-gozero.md`
- `.agents/skills/gozero-project-commands/references/commands/gen-api-gozero.md`
- `.agents/skills/gozero-project-commands/references/commands/new-rpc-gozero.md`
- `.agents/skills/gozero-project-commands/references/commands/update-rpc-gozero.md`
- `.agents/skills/gozero-project-commands/references/commands/gen-rpc-gozero.md`
- `.agents/skills/gozero-project-commands/references/commands/new-model-gozero.md`
- `.agents/skills/gozero-project-commands/references/commands/add-logic-gozero.md`
- `.agents/skills/gozero-project-commands/references/commands/tdd-gozero.md`
- `.agents/skills/gozero-project-commands/references/commands/review-gozero.md`
- `.agents/skills/gozero-project-commands/references/commands/review-git-gozero.md`
- `.claude/commands/zero-init.md`
- `.claude/commands/new-api-gozero.md`
- `.claude/commands/update-api-gozero.md`
- `.claude/commands/gen-api-gozero.md`
- `.claude/commands/new-rpc-gozero.md`
- `.claude/commands/update-rpc-gozero.md`
- `.claude/commands/gen-rpc-gozero.md`
- `.claude/commands/new-model-gozero.md`
- `.claude/commands/add-logic-gozero.md`
- `.claude/commands/tdd-gozero.md`
- `.claude/commands/review-gozero.md`
- `.claude/commands/review-git-gozero.md`

**Leave Unchanged**
- `.claude/settings.json`
- `.claude/settings.local.json`
- All generated go-zero service code under `app/` and `proto/`

**Responsibility Split**
- `AGENTS.md`: Codex-facing repository bootstrap and source-of-truth rules
- `CLAUDE.md`: Claude-facing compatibility entrypoint with explicit pointer to `.agents`
- `.agents/skills/gozero-project-commands/SKILL.md`: routing/index for task playbooks
- `.agents/.../references/commands/*.md`: canonical task playbooks
- `.claude/commands/*.md`: compatibility shims only, no duplicated workflow bodies

## Scope Guard

This plan does not migrate personal tool permission files, does not add helper scripts just to rewrite markdown once, and does not change business code under `app/`, `pkg/`, or `proto/`. The end state is a repository-doc migration, not a runtime feature change.

### Task 1: Establish Codex Repository Entry Points

**Files:**
- Create: `AGENTS.md`
- Modify: `CLAUDE.md`
- Test: repository root checks via `Test-Path`, `rg`, and `git diff --stat`

- [ ] **Step 1: Run the failing repository-entrypoint checks**

```powershell
Test-Path AGENTS.md
rg -n "Source of Truth|compatibility shim|\.agents/skills/gozero-project-commands" CLAUDE.md AGENTS.md
```

Expected:
- `Test-Path AGENTS.md` returns `False`
- `rg` returns no matches for `AGENTS.md`
- `CLAUDE.md` does not yet declare `.agents` as the canonical source

- [ ] **Step 2: Create `AGENTS.md` and add a source-of-truth block to `CLAUDE.md`**

```diff
*** Begin Patch
*** Add File: AGENTS.md
+# Repository Agent Guide
+
+This repository uses `.agents/skills/gozero-project-commands/` as the canonical source for task playbooks.
+
+## Startup
+
+- When a request touches `app/`, `proto/`, `go.work`, or workflow migration, load `zero-skills` first.
+- Then load `.agents/skills/gozero-project-commands/SKILL.md` and open only the matching reference file.
+- Prefer `superpowers:brainstorming`, `superpowers:writing-plans`, `superpowers:test-driven-development`, `superpowers:systematic-debugging`, and `superpowers:verification-before-completion` as the process layer.
+
+## Repository Rules
+
+- Prefer tool `workdir`, `go -C`, and `git -C` over `cd ... && ...`.
+- `.claude/commands/*.md` are compatibility-only wrappers and must not contain canonical workflow logic.
+- Do not edit goctl-generated files directly; regenerate from `.api` or `.proto`.
+- Keep repository workflow guidance in `.agents/skills/gozero-project-commands/`.
+
*** Update File: CLAUDE.md
@@
-## MUST do at the start of any conversation
-- run /init
+## MUST do at the start of any conversation
+- run /init
+
+## Source of Truth
+- Canonical repository workflow docs live under `.agents/skills/gozero-project-commands/`
+- `.claude/commands/*.md` are compatibility shims and should forward to the `.agents` source
+- Do not maintain duplicated workflow bodies in both `.claude/commands/` and `.agents/skills/`
*** End Patch
```

- [ ] **Step 3: Run the passing repository-entrypoint checks**

```powershell
Test-Path AGENTS.md
rg -n "canonical source|compatibility-only|\.agents/skills/gozero-project-commands" AGENTS.md CLAUDE.md
git diff --stat -- AGENTS.md CLAUDE.md
```

Expected:
- `Test-Path AGENTS.md` returns `True`
- `rg` finds the new source-of-truth lines in both files
- `git diff --stat` shows exactly `AGENTS.md` plus `CLAUDE.md`

- [ ] **Step 4: Commit the entrypoint changes**

```bash
git add AGENTS.md CLAUDE.md
git commit -m "docs: add codex repository entrypoint"
```

### Task 2: Make the Repository Skill Codex-Native

**Files:**
- Modify: `.agents/skills/gozero-project-commands/SKILL.md`
- Modify: `.agents/skills/gozero-project-commands/references/commands/zero-init.md`
- Test: `rg` checks for Claude slash-command language in the canonical skill

- [ ] **Step 1: Run the failing skill-language checks**

```powershell
rg -n "/zero-skills|former project-level \.claude/commands prompts|Use Skill tool" .agents/skills/gozero-project-commands/SKILL.md .agents/skills/gozero-project-commands/references/commands/zero-init.md
```

Expected:
- Matches are returned from the current `SKILL.md`
- `zero-init.md` still uses Claude-oriented "Use Skill tool" wording

- [ ] **Step 2: Rewrite `SKILL.md` and `zero-init.md` to Codex-native wording**

```diff
*** Begin Patch
*** Update File: .agents/skills/gozero-project-commands/SKILL.md
@@
-Repository-local Codex skill that replaces the former project `.claude/commands/*.md`
-prompts with a skill + references workflow.
-
-Load `zero-skills` first for framework knowledge. When a reference below says
-"use /zero-skills", interpret that as "load the `zero-skills` skill".
+Repository-local Codex skill and canonical source for repository task playbooks.
+
+Load `zero-skills` first for framework knowledge, then open only the matching
+reference file below. Do not preserve Claude slash-command syntax inside the
+canonical `.agents` references.
@@
-## References
+## References
@@
-- Session bootstrap: `references/commands/zero-init.md`
+- Session bootstrap: `references/commands/zero-init.md`
+
+## Compatibility
+
+- `.claude/commands/*.md` must forward to these references instead of duplicating them
+- Prefer tool `workdir`, `go -C`, and `git -C` over `cd ... && ...`
+- Keep new workflow logic in `.agents/skills/gozero-project-commands/`

*** Update File: .agents/skills/gozero-project-commands/references/commands/zero-init.md
@@
-Use Skill tool and load the following skills in order:
-1. `zero-skills`
-2. `using-superpowers`
+Use this bootstrap when a session needs repository conventions before opening a task-specific reference.
+
+1. Load `zero-skills`
+2. Load `superpowers:using-superpowers`
+3. Read `AGENTS.md`
+4. If the task touches go-zero workflows, open `.agents/skills/gozero-project-commands/SKILL.md`
*** End Patch
```

- [ ] **Step 3: Run the passing skill-language checks**

```powershell
rg -n "/zero-skills|Use Skill tool" .agents/skills/gozero-project-commands/SKILL.md .agents/skills/gozero-project-commands/references/commands/zero-init.md
rg -n "canonical source|Compatibility|Load `zero-skills`|Read `AGENTS.md`" .agents/skills/gozero-project-commands/SKILL.md .agents/skills/gozero-project-commands/references/commands/zero-init.md
```

Expected:
- The first `rg` returns no matches
- The second `rg` returns the new Codex-native wording

- [ ] **Step 4: Commit the canonical skill rewrite**

```bash
git add .agents/skills/gozero-project-commands/SKILL.md .agents/skills/gozero-project-commands/references/commands/zero-init.md
git commit -m "docs: make gozero codex skill canonical"
```

### Task 3: Rewrite API Playbooks as Canonical Codex References

**Files:**
- Modify: `.agents/skills/gozero-project-commands/references/commands/new-api-gozero.md`
- Modify: `.agents/skills/gozero-project-commands/references/commands/update-api-gozero.md`
- Modify: `.agents/skills/gozero-project-commands/references/commands/gen-api-gozero.md`
- Test: `rg` checks for `/zero-skills` and `cd <service-dir>` removal

- [ ] **Step 1: Run the failing API-playbook checks**

```powershell
rg -n "/zero-skills|cd <service-dir>|cd app/<service>" .agents/skills/gozero-project-commands/references/commands/new-api-gozero.md .agents/skills/gozero-project-commands/references/commands/update-api-gozero.md .agents/skills/gozero-project-commands/references/commands/gen-api-gozero.md
```

Expected:
- Matches are returned in all three files
- At least one match includes `cd <service-dir>` or `cd app/<service>`

- [ ] **Step 2: Replace the API-playbook context and execution wording**

```diff
*** Begin Patch
*** Update File: .agents/skills/gozero-project-commands/references/commands/new-api-gozero.md
@@
-使用 /zero-skills 加载以下参考：
+先加载 `zero-skills`，再阅读以下参考：
 - references/rest-api-patterns.md — 三层架构、Handler/Logic/Model 模式
 - references/goctl-commands.md — goctl 命令和 API Spec Patterns
@@
-```bash
-cd app/<service>
-goctl api go -api <service>.api -dir . --style go_zero
-```
+在 `app/<service>/` 作为 `workdir` 执行：
+
+```bash
+goctl api go -api <service>.api -dir . --style go_zero
+```
@@
-```bash
-# 初始化模块（如果是新服务）
-[ ! -f go.mod ] && go mod init esx/app/<service>
-
-# 整理依赖
-go mod tidy
-
-# 验证构建
-go build ./...
-```
+```bash
+go -C app/<service> mod tidy
+go -C app/<service> build ./...
+```

*** Update File: .agents/skills/gozero-project-commands/references/commands/update-api-gozero.md
@@
-使用 /zero-skills 加载：
+先加载 `zero-skills`，再阅读：
 - references/rest-api-patterns.md — 三层架构、API Spec 规范
 - references/goctl-commands.md — goctl 命令和 Post-Generation Pipeline
@@
-```bash
-cd <service-dir>
-goctl api go -api <file>.api -dir . --style go_zero
-```
+在目标服务目录作为 `workdir` 执行：
+
+```bash
+goctl api go -api <file>.api -dir . --style go_zero
+```
@@
-```bash
-go mod tidy
-go build ./...
-```
+```bash
+go -C <service-dir> mod tidy
+go -C <service-dir> build ./...
+```

*** Update File: .agents/skills/gozero-project-commands/references/commands/gen-api-gozero.md
@@
-使用 /zero-skills 加载：
+先加载 `zero-skills`，再阅读：
 - references/goctl-commands.md — goctl 命令和 Post-Generation Pipeline
@@
-```bash
-cd <service-dir>
-goctl api go -api <file>.api -dir . --style go_zero
-```
+在目标服务目录作为 `workdir` 执行：
+
+```bash
+goctl api go -api <file>.api -dir . --style go_zero
+```
@@
-```bash
-go mod tidy
-go build ./...
-```
+```bash
+go -C <service-dir> mod tidy
+go -C <service-dir> build ./...
+```
*** End Patch
```

- [ ] **Step 3: Run the passing API-playbook checks**

```powershell
rg -n "/zero-skills|cd <service-dir>|cd app/<service>" .agents/skills/gozero-project-commands/references/commands/new-api-gozero.md .agents/skills/gozero-project-commands/references/commands/update-api-gozero.md .agents/skills/gozero-project-commands/references/commands/gen-api-gozero.md
rg -n "先加载 `zero-skills`|workdir|go -C" .agents/skills/gozero-project-commands/references/commands/new-api-gozero.md .agents/skills/gozero-project-commands/references/commands/update-api-gozero.md .agents/skills/gozero-project-commands/references/commands/gen-api-gozero.md
```

Expected:
- The first `rg` returns no matches
- The second `rg` finds the new context wording and `go -C` examples in all three files

- [ ] **Step 4: Commit the API-playbook rewrite**

```bash
git add .agents/skills/gozero-project-commands/references/commands/new-api-gozero.md .agents/skills/gozero-project-commands/references/commands/update-api-gozero.md .agents/skills/gozero-project-commands/references/commands/gen-api-gozero.md
git commit -m "docs: rewrite codex api playbooks"
```

### Task 4: Rewrite RPC, Model, and Logic Playbooks

**Files:**
- Modify: `.agents/skills/gozero-project-commands/references/commands/new-rpc-gozero.md`
- Modify: `.agents/skills/gozero-project-commands/references/commands/update-rpc-gozero.md`
- Modify: `.agents/skills/gozero-project-commands/references/commands/gen-rpc-gozero.md`
- Modify: `.agents/skills/gozero-project-commands/references/commands/new-model-gozero.md`
- Modify: `.agents/skills/gozero-project-commands/references/commands/add-logic-gozero.md`
- Test: `rg` checks for slash-command removal and `workdir`-based execution guidance

- [ ] **Step 1: Run the failing RPC/model/logic checks**

```powershell
rg -n "/zero-skills|cd <service-dir>|cd app/<service>" .agents/skills/gozero-project-commands/references/commands/new-rpc-gozero.md .agents/skills/gozero-project-commands/references/commands/update-rpc-gozero.md .agents/skills/gozero-project-commands/references/commands/gen-rpc-gozero.md .agents/skills/gozero-project-commands/references/commands/new-model-gozero.md .agents/skills/gozero-project-commands/references/commands/add-logic-gozero.md
```

Expected:
- Matches are returned from several files
- Canonical `.agents` docs still mention `/zero-skills`

- [ ] **Step 2: Replace the shared context and execution sections**

```diff
*** Begin Patch
*** Update File: .agents/skills/gozero-project-commands/references/commands/new-rpc-gozero.md
@@
-使用 /zero-skills 加载以下参考：
+先加载 `zero-skills`，再阅读以下参考：
 - references/rpc-patterns.md — gRPC/zrpc 服务模式
 - references/goctl-commands.md — proto 和 zrpc 生成命令

*** Update File: .agents/skills/gozero-project-commands/references/commands/update-rpc-gozero.md
@@
-使用 /zero-skills 加载：
+先加载 `zero-skills`，再阅读：
 - references/rpc-patterns.md — RPC 设计和服务边界
 - references/goctl-commands.md — 生成命令和收尾流水线

*** Update File: .agents/skills/gozero-project-commands/references/commands/gen-rpc-gozero.md
@@
-使用 /zero-skills 加载：
+先加载 `zero-skills`，再阅读：
 - references/goctl-commands.md — proto 生成命令
@@
-```bash
-cd <service-dir>
-goctl rpc protoc <proto>.proto --go_out=. --go-grpc_out=. --zrpc_out=.
-```
+在目标服务目录作为 `workdir` 执行：
+
+```bash
+goctl rpc protoc <proto>.proto --go_out=. --go-grpc_out=. --zrpc_out=.
+```

*** Update File: .agents/skills/gozero-project-commands/references/commands/new-model-gozero.md
@@
-使用 /zero-skills 加载以下参考：
+先加载 `zero-skills`，再阅读以下参考：
 - references/database-patterns.md — sqlx、Redis、模型生成
 - references/goctl-commands.md — model 生成命令

*** Update File: .agents/skills/gozero-project-commands/references/commands/add-logic-gozero.md
@@
-使用 /zero-skills 加载以下参考：
+先加载 `zero-skills`，再阅读以下参考：
 - references/rest-api-patterns.md — REST Logic 规范
 - references/rpc-patterns.md — RPC Logic 规范
 - references/database-patterns.md — Model 和 SQL 约定
*** End Patch
```

- [ ] **Step 3: Normalize verification/build commands in the same five files**

```diff
*** Begin Patch
*** Update File: .agents/skills/gozero-project-commands/references/commands/new-rpc-gozero.md
@@
-```bash
-go mod tidy
-go build ./...
-```
+```bash
+go -C app/<service> mod tidy
+go -C app/<service> build ./...
+```

*** Update File: .agents/skills/gozero-project-commands/references/commands/update-rpc-gozero.md
@@
-```bash
-go mod tidy
-go build ./...
-```
+```bash
+go -C <service-dir> mod tidy
+go -C <service-dir> build ./...
+```

*** Update File: .agents/skills/gozero-project-commands/references/commands/gen-rpc-gozero.md
@@
-```bash
-go mod tidy
-go build ./...
-```
+```bash
+go -C <service-dir> mod tidy
+go -C <service-dir> build ./...
+```

*** Update File: .agents/skills/gozero-project-commands/references/commands/new-model-gozero.md
@@
-```bash
-go test ./...
-go build ./...
-```
+```bash
+go -C app/<service> test ./...
+go -C app/<service> build ./...
+```

*** Update File: .agents/skills/gozero-project-commands/references/commands/add-logic-gozero.md
@@
-```bash
-go test ./...
-go build ./...
-```
+```bash
+go -C app/<service> test ./...
+go -C app/<service> build ./...
+```
*** End Patch
```

- [ ] **Step 4: Run the passing RPC/model/logic checks**

```powershell
rg -n "/zero-skills|cd <service-dir>|cd app/<service>" .agents/skills/gozero-project-commands/references/commands/new-rpc-gozero.md .agents/skills/gozero-project-commands/references/commands/update-rpc-gozero.md .agents/skills/gozero-project-commands/references/commands/gen-rpc-gozero.md .agents/skills/gozero-project-commands/references/commands/new-model-gozero.md .agents/skills/gozero-project-commands/references/commands/add-logic-gozero.md
rg -n "先加载 `zero-skills`|workdir|go -C" .agents/skills/gozero-project-commands/references/commands/new-rpc-gozero.md .agents/skills/gozero-project-commands/references/commands/update-rpc-gozero.md .agents/skills/gozero-project-commands/references/commands/gen-rpc-gozero.md .agents/skills/gozero-project-commands/references/commands/new-model-gozero.md .agents/skills/gozero-project-commands/references/commands/add-logic-gozero.md
```

Expected:
- The first `rg` returns no matches
- The second `rg` finds Codex-native wording and `go -C` usage across all five files

- [ ] **Step 5: Commit the RPC/model/logic rewrite**

```bash
git add .agents/skills/gozero-project-commands/references/commands/new-rpc-gozero.md .agents/skills/gozero-project-commands/references/commands/update-rpc-gozero.md .agents/skills/gozero-project-commands/references/commands/gen-rpc-gozero.md .agents/skills/gozero-project-commands/references/commands/new-model-gozero.md .agents/skills/gozero-project-commands/references/commands/add-logic-gozero.md
git commit -m "docs: rewrite codex rpc model logic playbooks"
```

### Task 5: Rewrite TDD and Review Playbooks for Codex Output Style

**Files:**
- Modify: `.agents/skills/gozero-project-commands/references/commands/tdd-gozero.md`
- Modify: `.agents/skills/gozero-project-commands/references/commands/review-gozero.md`
- Modify: `.agents/skills/gozero-project-commands/references/commands/review-git-gozero.md`
- Test: `rg` checks for slash-command removal and review-output guidance

- [ ] **Step 1: Run the failing TDD/review checks**

```powershell
rg -n "/zero-skills|cd app/<service>|输出顺序|Findings" .agents/skills/gozero-project-commands/references/commands/tdd-gozero.md .agents/skills/gozero-project-commands/references/commands/review-gozero.md .agents/skills/gozero-project-commands/references/commands/review-git-gozero.md
```

Expected:
- `/zero-skills` matches are present
- No explicit Codex review output order is present yet

- [ ] **Step 2: Rewrite the context and command sections in `tdd-gozero.md`**

```diff
*** Begin Patch
*** Update File: .agents/skills/gozero-project-commands/references/commands/tdd-gozero.md
@@
-使用 /zero-skills 加载以下参考：
+先加载 `zero-skills`，再阅读以下参考：
 - references/testing-patterns.md — 测试金字塔、Mock 模式、集成测试
 - references/rest-api-patterns.md — 三层架构中 Logic 层规范
 - references/rpc-patterns.md — RPC Logic 层规范
 - references/database-patterns.md — 数据操作模式
@@
-```bash
-cd app/<service>
-go test -race -v ./internal/logic/... -run TestXxxLogic
-```
+```bash
+go -C app/<service> test -race -v ./internal/logic/... -run TestXxxLogic
+```
@@
-```bash
-cd app/<service>
-go test -race -v ./internal/logic/... -run TestXxxLogic
-```
+```bash
+go -C app/<service> test -race -v ./internal/logic/... -run TestXxxLogic
+```
*** End Patch
```

- [ ] **Step 3: Rewrite review output requirements in the two review playbooks**

```diff
*** Begin Patch
*** Update File: .agents/skills/gozero-project-commands/references/commands/review-gozero.md
@@
-使用 /zero-skills 加载所有相关规范：
+先加载 `zero-skills`，再阅读所有相关规范：
 - references/rest-api-patterns.md
 - references/rpc-patterns.md
 - references/database-patterns.md
 - references/concurrency-patterns.md
 - references/security-patterns.md
 - references/observability-patterns.md
@@
+## 输出顺序
+
+- Findings first, ordered by severity
+- Each finding must include file and line references
+- Then list open questions or assumptions
+- Keep change summary as a short trailing section only when useful

*** Update File: .agents/skills/gozero-project-commands/references/commands/review-git-gozero.md
@@
-使用 /zero-skills 加载所有相关规范：
+先加载 `zero-skills`，再阅读所有相关规范：
 - references/rest-api-patterns.md
 - references/rpc-patterns.md
 - references/database-patterns.md
 - references/concurrency-patterns.md
 - references/security-patterns.md
 - references/observability-patterns.md
@@
+## 输出顺序
+
+- Findings first, ordered by severity
+- Each finding must include file and line references from the git diff
+- Then list open questions or assumptions
+- Keep change summary as a short trailing section only when useful
*** End Patch
```

- [ ] **Step 4: Run the passing TDD/review checks**

```powershell
rg -n "/zero-skills|cd app/<service>" .agents/skills/gozero-project-commands/references/commands/tdd-gozero.md .agents/skills/gozero-project-commands/references/commands/review-gozero.md .agents/skills/gozero-project-commands/references/commands/review-git-gozero.md
rg -n "先加载 `zero-skills`|Findings first|输出顺序|go -C app/<service> test" .agents/skills/gozero-project-commands/references/commands/tdd-gozero.md .agents/skills/gozero-project-commands/references/commands/review-gozero.md .agents/skills/gozero-project-commands/references/commands/review-git-gozero.md
```

Expected:
- The first `rg` returns no matches
- The second `rg` finds the new review-order requirements and `go -C` test commands

- [ ] **Step 5: Commit the TDD/review rewrite**

```bash
git add .agents/skills/gozero-project-commands/references/commands/tdd-gozero.md .agents/skills/gozero-project-commands/references/commands/review-gozero.md .agents/skills/gozero-project-commands/references/commands/review-git-gozero.md
git commit -m "docs: align codex tdd and review playbooks"
```

### Task 6: Replace `.claude/commands` with Compatibility Shims

**Files:**
- Modify: `.claude/commands/zero-init.md`
- Modify: `.claude/commands/new-api-gozero.md`
- Modify: `.claude/commands/update-api-gozero.md`
- Modify: `.claude/commands/gen-api-gozero.md`
- Modify: `.claude/commands/new-rpc-gozero.md`
- Modify: `.claude/commands/update-rpc-gozero.md`
- Modify: `.claude/commands/gen-rpc-gozero.md`
- Modify: `.claude/commands/new-model-gozero.md`
- Modify: `.claude/commands/add-logic-gozero.md`
- Modify: `.claude/commands/tdd-gozero.md`
- Modify: `.claude/commands/review-gozero.md`
- Modify: `.claude/commands/review-git-gozero.md`
- Test: `rg` checks for duplicated canonical bodies and compatibility markers

- [ ] **Step 1: Run the failing compatibility-layer checks**

```powershell
rg -n "使用 /zero-skills|执行步骤|检查清单" .claude/commands
rg -n "compatibility-only|Canonical reference" .claude/commands
```

Expected:
- The first `rg` returns many matches because old full bodies are still present
- The second `rg` returns no matches

- [ ] **Step 2: Replace the old Claude command bodies with thin forwarding shims**

```powershell
$map = @{
  ".claude/commands/zero-init.md" = ".agents/skills/gozero-project-commands/references/commands/zero-init.md"
  ".claude/commands/new-api-gozero.md" = ".agents/skills/gozero-project-commands/references/commands/new-api-gozero.md"
  ".claude/commands/update-api-gozero.md" = ".agents/skills/gozero-project-commands/references/commands/update-api-gozero.md"
  ".claude/commands/gen-api-gozero.md" = ".agents/skills/gozero-project-commands/references/commands/gen-api-gozero.md"
  ".claude/commands/new-rpc-gozero.md" = ".agents/skills/gozero-project-commands/references/commands/new-rpc-gozero.md"
  ".claude/commands/update-rpc-gozero.md" = ".agents/skills/gozero-project-commands/references/commands/update-rpc-gozero.md"
  ".claude/commands/gen-rpc-gozero.md" = ".agents/skills/gozero-project-commands/references/commands/gen-rpc-gozero.md"
  ".claude/commands/new-model-gozero.md" = ".agents/skills/gozero-project-commands/references/commands/new-model-gozero.md"
  ".claude/commands/add-logic-gozero.md" = ".agents/skills/gozero-project-commands/references/commands/add-logic-gozero.md"
  ".claude/commands/tdd-gozero.md" = ".agents/skills/gozero-project-commands/references/commands/tdd-gozero.md"
  ".claude/commands/review-gozero.md" = ".agents/skills/gozero-project-commands/references/commands/review-gozero.md"
  ".claude/commands/review-git-gozero.md" = ".agents/skills/gozero-project-commands/references/commands/review-git-gozero.md"
}

foreach ($item in $map.GetEnumerator()) {
  @"
本命令保留给 Claude 兼容层使用。

Canonical reference: `$($item.Value)`

执行要求：
1. 先加载 `zero-skills`
2. 打开 canonical reference
3. 严格按 canonical reference 执行
4. 不要在 `.claude/commands/` 中维护重复正文
"@ | Set-Content -Encoding UTF8 $item.Key
}
```

- [ ] **Step 3: Run the passing compatibility-layer checks**

```powershell
rg -n "使用 /zero-skills|执行步骤|检查清单" .claude/commands
rg -n "compatibility|Canonical reference|不要在 `.claude/commands/` 中维护重复正文" .claude/commands
git diff --stat -- .claude/commands
```

Expected:
- The first `rg` returns no matches
- The second `rg` matches all 12 wrapper files
- `git diff --stat` shows only `.claude/commands/*.md` changed in this task

- [ ] **Step 4: Commit the compatibility shims**

```bash
git add .claude/commands
git commit -m "docs: convert claude commands to compatibility shims"
```

### Task 7: Verify Canonical State and Repository Cleanliness

**Files:**
- Modify: none
- Test: `rg`, `git status`, and a manual diff review over the migration files

- [ ] **Step 1: Run the final canonical-state checks**

```powershell
rg -n "/zero-skills|Use Skill tool|cd <service-dir>|cd app/<service>" .agents/skills/gozero-project-commands
rg -n "canonical source|compatibility-only|Canonical reference|Read `AGENTS.md`" AGENTS.md CLAUDE.md .agents/skills/gozero-project-commands .claude/commands
git status --short
```

Expected:
- The first `rg` returns no matches
- The second `rg` returns matches in `AGENTS.md`, `CLAUDE.md`, `.agents/...`, and `.claude/commands/...`
- `git status --short` shows tracked modifications or staged commits, but no `?? .agents/`

- [ ] **Step 2: Review the final diff before closing the branch**

```bash
git diff --stat HEAD~6..HEAD
git diff -- AGENTS.md CLAUDE.md .agents/skills/gozero-project-commands .claude/commands
```

Expected:
- Diff stats show only repository workflow docs
- No business-code or generated-code changes appear

- [ ] **Step 3: Run the final documentation-only commit if verification fixes were needed**

```bash
git add AGENTS.md CLAUDE.md .agents/skills/gozero-project-commands .claude/commands
git commit -m "docs: finish codex workflow migration"
```

Expected:
- If no fixes were needed, skip this commit
- If fixes were needed, commit contains only documentation migrations

## Self-Review

### Spec Coverage

- Codex-first entrypoint: covered by Task 1
- Canonical `.agents` source of truth: covered by Tasks 2 through 5
- Claude compatibility shim strategy: covered by Task 6
- Verification and cleanup: covered by Task 7
- Leaving `.claude/settings*.json` untouched: captured in File Structure and Scope Guard

### Placeholder Scan

- No `TODO`, `TBD`, or "implement later" placeholders remain
- Every change step includes either an exact patch block or an exact command block
- Every verification step includes a concrete command and expected outcome

### Type and Naming Consistency

- Canonical source terminology is consistently `AGENTS.md` + `.agents/skills/gozero-project-commands/`
- Compatibility terminology is consistently `.claude/commands/*.md`
- Codex-native wording consistently uses `load \`zero-skills\``, `workdir`, `go -C`, and `git -C`
