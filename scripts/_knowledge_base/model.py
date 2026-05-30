from __future__ import annotations

import dataclasses
import re
from pathlib import Path

_FRONTMATTER_RE = re.compile(r"^---\n(.*?)\n---\n(.*)$", re.DOTALL)
_LINK_RE = re.compile(r"\[[^\]]*\]\(([^)]+)\)")
_REQUIRED_KEYS = ("title", "tracks", "last_synced_commit", "last_synced_date")


class KnowledgeBaseError(RuntimeError):
    pass


@dataclasses.dataclass(frozen=True)
class Page:
    path: Path
    title: str
    tracks: tuple[str, ...]
    last_synced_commit: str
    last_synced_date: str
    sync_note: str
    body: str

    @property
    def links(self) -> list[str]:
        return extract_links(self.body)


def extract_links(text: str) -> list[str]:
    """Return every markdown link target found in ``text``."""
    return _LINK_RE.findall(text)


def _strip_scalar(raw: str) -> str:
    value = raw.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
        return value[1:-1]
    return value


def _parse_frontmatter(text: str, source: Path) -> dict[str, object]:
    data: dict[str, object] = {}
    current_list: list[str] | None = None
    for line_no, line in enumerate(text.splitlines(), start=1):
        if not line.strip():
            continue
        if line.startswith("  - "):
            if current_list is None:
                raise KnowledgeBaseError(
                    f"{source}: line {line_no}: list item without a key"
                )
            current_list.append(_strip_scalar(line[4:]))
            continue
        if ":" not in line:
            raise KnowledgeBaseError(
                f"{source}: line {line_no}: expected 'key: value'"
            )
        key, _, raw = line.partition(":")
        key = key.strip()
        if not raw.strip():
            current_list = []
            data[key] = current_list
        else:
            current_list = None
            data[key] = _strip_scalar(raw)
    return data


def parse_page(path: Path) -> Page:
    text = path.read_text(encoding="utf-8")
    match = _FRONTMATTER_RE.match(text)
    if match is None:
        raise KnowledgeBaseError(f"{path}: missing or malformed frontmatter")
    data = _parse_frontmatter(match.group(1), path)
    missing = [k for k in _REQUIRED_KEYS if k not in data]
    if missing:
        raise KnowledgeBaseError(f"{path}: missing frontmatter keys: {missing}")
    tracks = data["tracks"]
    if not isinstance(tracks, list) or not tracks:
        raise KnowledgeBaseError(f"{path}: 'tracks' must be a non-empty list")
    return Page(
        path=path,
        title=str(data["title"]),
        tracks=tuple(tracks),
        last_synced_commit=str(data["last_synced_commit"]),
        last_synced_date=str(data["last_synced_date"]),
        sync_note=str(data.get("sync_note", "")),
        body=match.group(2),
    )
