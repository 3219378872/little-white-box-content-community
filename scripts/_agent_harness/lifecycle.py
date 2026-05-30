from __future__ import annotations

import dataclasses
import datetime as dt
import re
import shutil
from pathlib import Path

from _agent_harness import templates
from _agent_harness.checks import REQUIRED_MD, check_task
from _agent_harness.model import (
    HarnessError,
    TaskRecord,
    TaskStatus,
    read_task_yaml,
    write_task_yaml,
)


def _harness_root(repo_root: Path) -> Path:
    return repo_root / "docs" / "agent-harness"


def active_root(repo_root: Path) -> Path:
    return _harness_root(repo_root) / "tasks" / "active"


def completed_root(repo_root: Path) -> Path:
    return _harness_root(repo_root) / "tasks" / "completed"


def _slugify(value: str) -> str:
    slug = re.sub(r"[^a-z0-9]+", "-", value.strip().lower()).strip("-")
    if not slug:
        raise HarnessError("slug cannot be empty")
    return slug


def _find_task(repo_root: Path, slug: str) -> Path:
    for root in (active_root(repo_root), completed_root(repo_root)):
        candidate = root / slug
        if candidate.is_dir():
            return candidate
    raise HarnessError(f"task not found: {slug}")


def new_task(repo_root: Path, slug: str, *, date: str | None = None) -> Path:
    day = date or dt.date.today().isoformat()
    normalized = _slugify(slug)
    full_slug = f"{day}-{normalized}"
    target = active_root(repo_root) / full_slug
    if target.exists():
        raise HarnessError(f"task already exists: {target}")
    target.mkdir(parents=True)
    for name in REQUIRED_MD:
        (target / name).write_text(
            templates.render(repo_root, name, slug=normalized, day=day),
            encoding="utf-8",
        )
    task_branch = f"task/{full_slug}"
    write_task_yaml(
        target / "task.yaml",
        TaskRecord(
            slug=full_slug,
            status=TaskStatus.ACTIVE,
            task_branch=task_branch,
            spec=None,
            plan=None,
            spec_waiver=None,
            plan_waiver=None,
        ),
    )
    return target


def _verification_has_pass(task_dir: Path) -> bool:
    text = (task_dir / "verification.md").read_text(encoding="utf-8")
    return (
        re.search(r"\|\s*`[^`]+`\s*\|\s*pass(ed)?\b", text, re.IGNORECASE) is not None
    )


def _audit_has_rows(task_dir: Path) -> bool:
    text = (task_dir / "audit.md").read_text(encoding="utf-8")
    rows = [line for line in text.splitlines() if line.strip().startswith("|")]
    return len(rows) >= 3


def complete_task(repo_root: Path, slug: str) -> Path:
    task_dir = active_root(repo_root) / slug
    if not task_dir.is_dir():
        raise HarnessError(f"active task not found: {slug}")
    record = read_task_yaml(task_dir / "task.yaml")
    if record.status is not TaskStatus.ACTIVE:
        raise HarnessError(
            f"task {slug} is not active (status={record.status.value})"
        )
    issues = check_task(task_dir)
    if issues:
        raise HarnessError(
            f"task {slug} has {len(issues)} unresolved check issue(s)"
        )
    if not _verification_has_pass(task_dir):
        raise HarnessError(
            f"task {slug} verification.md has no passing command row"
        )
    if not _audit_has_rows(task_dir):
        raise HarnessError(
            f"task {slug} audit.md has no requirement rows"
        )
    write_task_yaml(
        task_dir / "task.yaml",
        dataclasses.replace(record, status=TaskStatus.DONE),
    )
    destination = completed_root(repo_root) / slug
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.move(str(task_dir), str(destination))
    return destination


def abandon_task(repo_root: Path, slug: str, *, reason: str) -> Path:
    task_dir = _find_task(repo_root, slug)
    record = read_task_yaml(task_dir / "task.yaml")
    write_task_yaml(
        task_dir / "task.yaml",
        dataclasses.replace(record, status=TaskStatus.ABANDONED),
    )
    handoff = task_dir / "handoff.md"
    handoff.write_text(
        handoff.read_text(encoding="utf-8") + f"\n## Abandoned\n{reason}\n",
        encoding="utf-8",
    )
    destination = completed_root(repo_root) / slug
    if task_dir.parent != completed_root(repo_root):
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.move(str(task_dir), str(destination))
    return destination
