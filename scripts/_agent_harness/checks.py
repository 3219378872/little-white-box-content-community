from __future__ import annotations

import dataclasses
import re
from pathlib import Path

from _agent_harness.model import (
    HarnessError,
    TaskRecord,
    TaskStatus,
    read_task_yaml,
)

REQUIRED_MD = ("context.md", "verification.md", "audit.md", "handoff.md")
PLACEHOLDER_RE = re.compile(r"\b(TODO|TBD|fill in)\b", re.IGNORECASE)
GUIDANCE_RE = re.compile(r"\b(explain why|replace with)\b", re.IGNORECASE)
TASK_BRANCH_RE = re.compile(r"^task/\d{4}-\d{2}-\d{2}-[a-z0-9-]+$")


@dataclasses.dataclass(frozen=True)
class CheckIssue:
    task: str
    file: str
    rule_id: str
    message: str


def _record_issues(task: str, record: TaskRecord) -> list[CheckIssue]:
    issues: list[CheckIssue] = []

    def add(rule_id: str, message: str) -> None:
        issues.append(CheckIssue(task, "task.yaml", rule_id, message))

    if not TASK_BRANCH_RE.match(record.task_branch):
        add("H005", "task_branch must match task/YYYY-MM-DD-<slug>")
    if record.spec is None and not record.spec_waiver:
        add("H002", "spec must be a path or null with a spec_waiver reason")
    if record.spec and not record.spec.startswith("docs/design-docs/"):
        add("H002", "spec must live under docs/design-docs/")
    if record.plan is None and not record.plan_waiver:
        add("H003", "plan must be a path or null with a plan_waiver reason")
    if record.plan and not record.plan.startswith("docs/exec-plans/"):
        add("H003", "plan must live under docs/exec-plans/")
    return issues


def _md_issues(
    task: str, task_dir: Path, *, check_prose: bool = True
) -> list[CheckIssue]:
    issues: list[CheckIssue] = []
    for name in REQUIRED_MD:
        path = task_dir / name
        if not path.exists():
            issues.append(CheckIssue(task, name, "H004", f"missing {name}"))
            continue
        text = path.read_text(encoding="utf-8")
        if not text.strip():
            issues.append(CheckIssue(task, name, "H004", f"{name} is empty"))
        if not check_prose:
            continue
        if PLACEHOLDER_RE.search(text):
            issues.append(
                CheckIssue(task, name, "H020", f"{name} has an unresolved placeholder")
            )
        if GUIDANCE_RE.search(text):
            issues.append(
                CheckIssue(
                    task, name, "H021", f"{name} has unresolved template guidance"
                )
            )
    return issues


def check_task(task_dir: Path, *, historical: bool = False) -> list[CheckIssue]:
    """Validate one task record.

    With ``historical=True`` only the structural rules that always applied are
    enforced (``task.yaml`` parses, the four narrative files exist, and the
    task has a terminal status). This is the lighter pass for completed task
    records so tightening active-task rules cannot retroactively fail history.
    """
    task = task_dir.name
    issues = _md_issues(task, task_dir, check_prose=not historical)
    try:
        record = read_task_yaml(task_dir / "task.yaml")
    except HarnessError as exc:
        issues.append(CheckIssue(task, "task.yaml", "H001", str(exc)))
        return issues
    if historical:
        if record.status not in {TaskStatus.DONE, TaskStatus.ABANDONED}:
            issues.append(
                CheckIssue(
                    task,
                    "task.yaml",
                    "H030",
                    "completed task must have status done or abandoned",
                )
            )
        return issues
    issues.extend(_record_issues(task, record))
    return issues
