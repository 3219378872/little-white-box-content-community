from __future__ import annotations

from pathlib import Path

from _agent_harness.model import HarnessError

_FILE_TO_TEMPLATE = {
    "context.md": "task-context.md",
    "verification.md": "verification.md",
    "audit.md": "audit.md",
    "handoff.md": "handoff.md",
}


def render(repo_root: Path, file_name: str, *, slug: str, day: str) -> str:
    template_name = _FILE_TO_TEMPLATE.get(file_name)
    if template_name is None:
        raise HarnessError(f"unknown template file: {file_name}")
    path = repo_root / "docs" / "exec-plans" / "templates" / template_name
    if not path.exists():
        raise HarnessError(f"missing template: {path}")
    task_branch = f"task/{day}-{slug}"
    return path.read_text(encoding="utf-8").format(
        slug=f"{day}-{slug}",
        title=slug.replace("-", " ").title(),
        day=day,
        task_branch=task_branch,
    )
