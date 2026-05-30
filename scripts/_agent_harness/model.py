from __future__ import annotations

import dataclasses
import enum
from pathlib import Path


class HarnessError(RuntimeError):
    pass


class TaskStatus(enum.Enum):
    ACTIVE = "active"
    DONE = "done"
    ABANDONED = "abandoned"


_STATUS_BY_VALUE = {status.value: status for status in TaskStatus}
_LEGACY_STATUS_MAP = {
    "created": TaskStatus.ACTIVE,
    "implementing": TaskStatus.ACTIVE,
    "pr-open": TaskStatus.ACTIVE,
    "merged": TaskStatus.DONE,
}

_FIELDS = (
    "slug",
    "status",
    "task_branch",
    "spec",
    "plan",
    "spec_waiver",
    "plan_waiver",
)
_OPTIONAL = {"spec", "plan", "spec_waiver", "plan_waiver"}


@dataclasses.dataclass(frozen=True)
class TaskRecord:
    slug: str
    status: TaskStatus
    task_branch: str
    spec: str | None
    plan: str | None
    spec_waiver: str | None
    plan_waiver: str | None


def _parse_scalar(raw: str) -> str | None:
    value = raw.strip()
    if value.lower() in {"null", "~", ""}:
        return None
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
        return value[1:-1]
    return value


def _parse_yaml(text: str) -> dict[str, str | None]:
    """Minimal flat ``key: value`` YAML reader. No nesting, no lists."""
    result: dict[str, str | None] = {}
    for line_no, line in enumerate(text.splitlines(), start=1):
        stripped = line.split("#", 1)[0].rstrip()
        if not stripped:
            continue
        if ":" not in stripped:
            raise HarnessError(f"task.yaml line {line_no}: expected 'key: value'")
        key, _, raw = stripped.partition(":")
        result[key.strip()] = _parse_scalar(raw)
    return result


_LEGACY_FIELDS = {
    "base_branch",
    "remote_branch",
    "pull_request",
    "ci_status",
    "codex_review",
    "remote_cleanup",
    "pr_waiver",
}


def read_task_yaml(path: Path) -> TaskRecord:
    if not path.exists():
        raise HarnessError(f"missing task.yaml: {path}")
    data = _parse_yaml(path.read_text(encoding="utf-8"))
    unknown = set(data) - set(_FIELDS) - _LEGACY_FIELDS
    if unknown:
        raise HarnessError(f"task.yaml unknown keys: {sorted(unknown)}")
    missing = [f for f in _FIELDS if f not in data and f not in _OPTIONAL]
    if missing:
        raise HarnessError(f"task.yaml missing keys: {missing}")
    status_value = data["status"]
    if status_value not in _STATUS_BY_VALUE and status_value not in _LEGACY_STATUS_MAP:
        raise HarnessError(f"task.yaml invalid status: {status_value!r}")
    status = _STATUS_BY_VALUE.get(status_value) or _LEGACY_STATUS_MAP[status_value]
    return TaskRecord(
        slug=data["slug"] or "",
        status=status,
        task_branch=data.get("task_branch") or "",
        spec=data.get("spec"),
        plan=data.get("plan"),
        spec_waiver=data.get("spec_waiver"),
        plan_waiver=data.get("plan_waiver"),
    )


def write_task_yaml(path: Path, record: TaskRecord) -> None:
    lines: list[str] = []
    for field in _FIELDS:
        value = getattr(record, field)
        if isinstance(value, TaskStatus):
            value = value.value
        lines.append(f"{field}: {'null' if value is None else value}")
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
