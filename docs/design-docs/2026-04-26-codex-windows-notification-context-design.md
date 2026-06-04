# Codex Windows Notification Context Design

## Goal

Enhance the existing Codex Windows Toast notification script so approval and completion notifications show enough context to identify the active Codex session.

The notification should show:

- the Codex session repository location
- the current Git branch
- a short summary of the relevant Codex output

For this iteration, "output" means the final assistant message excerpt already available in the `Stop` hook payload. The design intentionally does not capture shell command stdout or stderr.

## Current State

The active notification script is:

- `/home/bt/.codex/hooks/codex_windows_notify.py`

The active hook config is:

- `/home/bt/.codex/hooks.json`

Configured events:

- `PermissionRequest`
- `Stop`

The script already reads JSON hook payloads from standard input and sends Windows Toast notifications through Windows PowerShell and the BurntToast module.

## Design

Keep the current hook set and enhance only the script.

Add a repository context helper:

- read `cwd` from the Codex hook payload
- run `git -C <cwd> rev-parse --show-toplevel` to find the repository root
- run `git -C <repo-root> branch --show-current` to find the current branch
- fall back to the payload `cwd` when the session is not inside a Git repository
- display branch as `non-git` when no branch can be resolved

Format notification bodies consistently:

```text
Repo: /absolute/repo/path
Branch: current-branch
Summary: short event-specific text
```

For `PermissionRequest`, the summary is the approval description when available, otherwise the tool name.

For `Stop`, the summary is a compact excerpt of `last_assistant_message`. If the payload does not contain a final message, use `The current turn is complete.`

## Scope

In scope:

- update `/home/bt/.codex/hooks/codex_windows_notify.py`
- keep `/home/bt/.codex/hooks.json` unchanged unless testing proves a schema mismatch
- preserve current Windows Toast delivery through BurntToast
- keep notification text compact enough for Toast display

Out of scope:

- capturing command stdout or stderr
- adding `PostToolUse` hooks
- storing per-session state
- changing repository code
- changing shell startup files or Windows system settings

## Error Handling

Notification failures must not break Codex execution.

The script should:

- return a non-zero status only for direct notification failure, matching current behavior
- log best-effort diagnostic messages to `/home/bt/.codex/log/codex-windows-notify.log`
- tolerate missing or malformed hook payload fields
- tolerate non-Git working directories

## Verification

Verify with:

- parse the script with Python `ast.parse`
- validate `/home/bt/.codex/hooks.json` with `python3 -m json.tool`
- call context-formatting functions with synthetic `PermissionRequest` and `Stop` payloads
- run the script's `--test` path to confirm Windows Toast still displays

Manual end-to-end validation requires a fresh Codex session because hook configuration may not be hot-loaded by the current running session.
