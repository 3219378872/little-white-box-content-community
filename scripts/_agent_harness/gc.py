from __future__ import annotations

import dataclasses
import datetime as dt
import re
from pathlib import Path

from _agent_harness.checks import check_task
from _agent_harness.lifecycle import active_root, completed_root
from _agent_harness.model import HarnessError, TaskStatus, read_task_yaml

STALE_WINDOW_DAYS = 14
_DATE_PREFIX_RE = re.compile(r"^(\d{4}-\d{2}-\d{2})")
_COMMAND_ROW_RE = re.compile(r"\|\s*`[^`]+`\s*\|")
_SKIPPED_RE = re.compile(r"\b(skipped|not run)\b", re.IGNORECASE)


@dataclasses.dataclass(frozen=True)
class HarnessSummary:
    active_tasks: list[str]
    completed_count: int
    issue_count: int
    gc_notes: list[str]
    specs_without_plans: list[str]
    plans_without_task_refs: list[str]
    recommended_next_action: str


def _task_dirs(root: Path) -> list[Path]:
    if not root.exists():
        return []
    return sorted(p for p in root.iterdir() if p.is_dir())


def _markdown_files(root: Path) -> list[Path]:
    return sorted(root.glob("*.md")) if root.exists() else []


def _dir_date(name: str) -> dt.date | None:
    match = _DATE_PREFIX_RE.match(name)
    if match is None:
        return None
    try:
        return dt.date.fromisoformat(match.group(1))
    except ValueError:
        return None


def _is_stale(task_dir: Path, today: dt.date) -> bool:
    """A non-terminal active task is stale only when both its directory date is
    older than the window *and* its ``task.yaml`` has not been modified within
    the window — i.e. there is no recent evidence of work."""
    started = _dir_date(task_dir.name)
    if started is None or (today - started).days <= STALE_WINDOW_DAYS:
        return False
    yaml_path = task_dir / "task.yaml"
    if not yaml_path.exists():
        return False
    modified = dt.date.fromtimestamp(yaml_path.stat().st_mtime)
    return (today - modified).days > STALE_WINDOW_DAYS


def _verification_has_evidence(task_dir: Path) -> bool:
    path = task_dir / "verification.md"
    if not path.exists():
        return False
    text = path.read_text(encoding="utf-8")
    return bool(_COMMAND_ROW_RE.search(text) or _SKIPPED_RE.search(text))


def _audit_without_verification(task_dir: Path) -> bool:
    audit = task_dir / "audit.md"
    if not audit.exists():
        return False
    has_audit_rows = "|" in audit.read_text(encoding="utf-8")
    return has_audit_rows and not _verification_has_evidence(task_dir)


def _gc_notes(repo_root: Path, today: dt.date) -> list[str]:
    notes: list[str] = []
    for task_dir in _task_dirs(active_root(repo_root)):
        try:
            record = read_task_yaml(task_dir / "task.yaml")
        except HarnessError:
            continue
        if record.status in {TaskStatus.DONE, TaskStatus.ABANDONED}:
            notes.append(
                f"misplaced: {task_dir.name} is {record.status.value} but under active/"
            )
        elif record.status is TaskStatus.ACTIVE and _is_stale(task_dir, today):
            started = _dir_date(task_dir.name)
            age = (today - started).days if started is not None else 0
            notes.append(
                f"stale: {task_dir.name} is {record.status.value}, {age} days "
                f"old, with no task.yaml update inside the {STALE_WINDOW_DAYS}-day "
                f"window"
            )
        if _audit_without_verification(task_dir):
            notes.append(f"audit-without-verification: {task_dir.name}")
    for task_dir in _task_dirs(completed_root(repo_root)):
        if _audit_without_verification(task_dir):
            notes.append(f"audit-without-verification: {task_dir.name}")
    return notes


def _specs_without_plans(repo_root: Path) -> list[str]:
    specs = _markdown_files(repo_root / "docs" / "design-docs")
    plan_keys = {
        p.stem
        for p in _markdown_files(repo_root / "docs" / "exec-plans" / "completed")
    }
    return [
        str(s.relative_to(repo_root))
        for s in specs
        if s.stem.removesuffix("-design") not in plan_keys
    ]


def _plans_without_task_refs(repo_root: Path) -> list[str]:
    plans = _markdown_files(repo_root / "docs" / "exec-plans" / "completed")
    chunks: list[str] = []
    for root in (active_root(repo_root), completed_root(repo_root)):
        for task_dir in _task_dirs(root):
            for name in ("context.md", "verification.md", "audit.md", "handoff.md"):
                path = task_dir / name
                if path.exists():
                    chunks.append(path.read_text(encoding="utf-8"))
    task_text = "\n".join(chunks)
    return [
        str(p.relative_to(repo_root))
        for p in plans
        if str(p.relative_to(repo_root)) not in task_text
    ]


def build_summary(
    repo_root: Path, *, today: dt.date | None = None
) -> HarnessSummary:
    today = today or dt.date.today()
    active = [p.name for p in _task_dirs(active_root(repo_root))]
    completed = _task_dirs(completed_root(repo_root))
    issues = [i for d in _task_dirs(active_root(repo_root)) for i in check_task(d)]
    gc_notes = _gc_notes(repo_root, today)
    specs_without_plans = _specs_without_plans(repo_root)
    plans_without_task_refs = _plans_without_task_refs(repo_root)
    if issues:
        next_action = "Fix active task check failures."
    elif active:
        next_action = "Continue active task work and keep verification current."
    else:
        next_action = "Create a task record for the next substantial change."
    return HarnessSummary(
        active_tasks=active,
        completed_count=len(completed),
        issue_count=len(issues),
        gc_notes=gc_notes,
        specs_without_plans=specs_without_plans,
        plans_without_task_refs=plans_without_task_refs,
        recommended_next_action=next_action,
    )
