# Codex Windows Notification Context Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show repository location, current Git branch, and a compact Codex output summary in Windows Toast notifications for approval and completion events.

**Architecture:** Keep the existing `PermissionRequest` and `Stop` hooks. Enhance `/home/bt/.codex/hooks/codex_windows_notify.py` with pure helper functions for repository context and body formatting, then keep BurntToast delivery unchanged.

**Tech Stack:** Python 3 standard library, Git CLI, Windows PowerShell 5.1, BurntToast, Codex hooks.

---

## File Structure

- Modify: `/home/bt/.codex/hooks/codex_windows_notify.py`
  - Add `run_git()`, `repo_context()`, and `format_body()` helpers.
  - Update `title_body()` to use the new body format.
  - Keep notification delivery and hook JSON contract unchanged.
- Read-only verification: `/home/bt/.codex/hooks.json`
  - Validate JSON syntax only.
- No repository source files are changed.
- No Git commit is made because the target script is outside this repository and `docs/` is ignored by project policy.

---

### Task 1: Add Repository Context Helpers

**Files:**
- Modify: `/home/bt/.codex/hooks/codex_windows_notify.py`

- [ ] **Step 1: Verify the current helper behavior before editing**

Run:

```bash
python3 - <<'PY'
import importlib.util
spec = importlib.util.spec_from_file_location("notify", "/home/bt/.codex/hooks/codex_windows_notify.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
print(module.project_label("/home/bt/projects/backend/little-white-box-content-community"))
PY
```

Expected output:

```text
little-white-box-content-community
```

- [ ] **Step 2: Add Git context helpers**

Insert these functions after `project_label()`:

```python
def run_git(cwd: str, *args: str) -> str:
    try:
        completed = subprocess.run(
            ["git", "-C", cwd, *args],
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            text=True,
            timeout=2,
            check=False,
        )
    except Exception as exc:  # noqa: BLE001 - context lookup must not break notifications.
        log(f"git context lookup failed in {cwd}: {exc}")
        return ""
    if completed.returncode != 0:
        return ""
    return completed.stdout.strip()


def repo_context(cwd: str | None) -> tuple[str, str]:
    if not cwd:
        return "unknown", "unknown"

    repo = run_git(cwd, "rev-parse", "--show-toplevel")
    if not repo:
        return cwd, "non-git"

    branch = run_git(repo, "branch", "--show-current")
    if not branch:
        branch = run_git(repo, "rev-parse", "--short", "HEAD")
    if not branch:
        branch = "detached"

    return repo, branch
```

- [ ] **Step 3: Verify helper behavior in this repository**

Run:

```bash
python3 - <<'PY'
import importlib.util
spec = importlib.util.spec_from_file_location("notify", "/home/bt/.codex/hooks/codex_windows_notify.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
print(module.repo_context("/home/bt/projects/backend/little-white-box-content-community"))
PY
```

Expected output includes:

```text
('/home/bt/projects/backend/little-white-box-content-community', 'main')
```

- [ ] **Step 4: Verify non-Git fallback**

Run:

```bash
python3 - <<'PY'
import importlib.util
spec = importlib.util.spec_from_file_location("notify", "/home/bt/.codex/hooks/codex_windows_notify.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
print(module.repo_context("/tmp"))
PY
```

Expected output:

```text
('/tmp', 'non-git')
```

---

### Task 2: Format Approval and Completion Bodies

**Files:**
- Modify: `/home/bt/.codex/hooks/codex_windows_notify.py`

- [ ] **Step 1: Add a shared body formatter**

Insert this function after `repo_context()`:

```python
def format_body(cwd: str | None, summary: str) -> str:
    repo, branch = repo_context(cwd)
    return "\n".join(
        [
            f"Repo: {repo}",
            f"Branch: {branch}",
            f"Summary: {compact(summary, 220)}",
        ]
    )
```

- [ ] **Step 2: Replace `title_body()` with the context-aware version**

Replace the existing `title_body()` function with:

```python
def title_body(payload: dict[str, Any], source: str) -> tuple[str, str]:
    event = payload.get("hook_event_name") or payload.get("type") or source
    cwd_value = payload.get("cwd")
    cwd = cwd_value if isinstance(cwd_value, str) else None

    if event == "PermissionRequest":
        tool_name = payload.get("tool_name")
        tool_input = payload.get("tool_input")
        description = ""
        if isinstance(tool_input, dict):
            maybe_description = tool_input.get("description")
            if isinstance(maybe_description, str):
                description = maybe_description
        detail = description or f"{tool_name or 'tool'} is waiting for approval"
        return "Codex needs approval", format_body(cwd, detail)

    if event == "Stop":
        message = payload.get("last_assistant_message")
        detail = compact(message if isinstance(message, str) else None)
        if not detail:
            detail = "The current turn is complete."
        return "Codex task complete", format_body(cwd, detail)

    if isinstance(payload.get("title"), str):
        title = compact(payload.get("title"), 80) or "Codex"
        message = payload.get("message") or payload.get("body")
        body = compact(message if isinstance(message, str) else None) or compact(str(event))
        return title, format_body(cwd, body)

    return "Codex notification", format_body(cwd, compact(str(event)))
```

- [ ] **Step 3: Verify approval body formatting**

Run:

```bash
python3 - <<'PY'
import importlib.util
spec = importlib.util.spec_from_file_location("notify", "/home/bt/.codex/hooks/codex_windows_notify.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
title, body = module.title_body({
    "hook_event_name": "PermissionRequest",
    "cwd": "/home/bt/projects/backend/little-white-box-content-community",
    "tool_name": "exec_command",
    "tool_input": {"description": "需要执行提权命令"}
}, "hook")
print(title)
print(body)
PY
```

Expected output includes:

```text
Codex needs approval
Repo: /home/bt/projects/backend/little-white-box-content-community
Branch: main
Summary: 需要执行提权命令
```

- [ ] **Step 4: Verify completion body formatting**

Run:

```bash
python3 - <<'PY'
import importlib.util
spec = importlib.util.spec_from_file_location("notify", "/home/bt/.codex/hooks/codex_windows_notify.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
title, body = module.title_body({
    "hook_event_name": "Stop",
    "cwd": "/home/bt/projects/backend/little-white-box-content-community",
    "last_assistant_message": "脚本已更新并通过验证。"
}, "hook")
print(title)
print(body)
PY
```

Expected output includes:

```text
Codex task complete
Repo: /home/bt/projects/backend/little-white-box-content-community
Branch: main
Summary: 脚本已更新并通过验证。
```

---

### Task 3: Verify Delivery and Configuration

**Files:**
- Modify: `/home/bt/.codex/hooks/codex_windows_notify.py`
- Read: `/home/bt/.codex/hooks.json`

- [ ] **Step 1: Check Python syntax without writing `__pycache__`**

Run:

```bash
python3 - <<'PY'
import ast
from pathlib import Path
ast.parse(Path("/home/bt/.codex/hooks/codex_windows_notify.py").read_text(encoding="utf-8"))
print("syntax-ok")
PY
```

Expected output:

```text
syntax-ok
```

- [ ] **Step 2: Validate hook config JSON**

Run:

```bash
python3 -m json.tool /home/bt/.codex/hooks.json >/tmp/codex-hooks-json.out
test -s /tmp/codex-hooks-json.out
```

Expected result: exit code `0`.

- [ ] **Step 3: Send a Windows Toast test notification**

Run:

```bash
python3 /home/bt/.codex/hooks/codex_windows_notify.py --test
```

Expected result: exit code `0` and a Windows Toast notification appears.

- [ ] **Step 4: Check notification log for new errors**

Run:

```bash
test ! -f /home/bt/.codex/log/codex-windows-notify.log || tail -n 20 /home/bt/.codex/log/codex-windows-notify.log
```

Expected result: no new PowerShell or Git context errors from this implementation.

- [ ] **Step 5: Confirm repository status**

Run:

```bash
git status --short
```

Expected result: no tracked repository files changed. The ignored plan/spec documents may remain hidden from normal status output.

---

## Self-Review

- Spec coverage: Task 1 covers repository path and branch. Task 2 covers approval and completion summaries. Task 3 covers syntax, JSON, Toast delivery, and log checks.
- Placeholder scan: no placeholders are present.
- Type consistency: helper signatures use `str | None` and return `tuple[str, str]`; `title_body()` continues returning `tuple[str, str]`.
