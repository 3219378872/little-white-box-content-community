from __future__ import annotations

import dataclasses
import subprocess
from pathlib import Path

from _knowledge_base.model import (
    KnowledgeBaseError,
    Page,
    extract_links,
    parse_page,
)


@dataclasses.dataclass(frozen=True)
class CheckIssue:
    rule_id: str
    path: str
    message: str


def _kb_root(repo_root: Path) -> Path:
    return repo_root / "docs" / "knowledge-base"


def _page_files(kb_root: Path) -> list[Path]:
    files: list[Path] = []
    for sub in ("modules", "flows"):
        directory = kb_root / sub
        if directory.exists():
            files.extend(sorted(directory.glob("*.md")))
    return files


def _load_pages(kb_root: Path) -> tuple[list[Page], list[CheckIssue]]:
    pages: list[Page] = []
    issues: list[CheckIssue] = []
    for path in _page_files(kb_root):
        try:
            pages.append(parse_page(path))
        except KnowledgeBaseError as exc:
            issues.append(CheckIssue("K001", str(path), str(exc)))
    return pages, issues


def _has_go_file(directory: Path) -> bool:
    return next(directory.rglob("*.go"), None) is not None


def _expected_modules(repo_root: Path) -> set[str]:
    """Backend subpackages every module page set must cover.

    For the esx go-zero monorepo that is each service under ``app/`` that owns
    Go code and each shared library under ``pkg/``.
    """
    expected: set[str] = set()
    app = repo_root / "app"
    if app.exists():
        for entry in sorted(app.iterdir()):
            if not entry.is_dir() or entry.name.startswith("."):
                continue
            if _has_go_file(entry):
                expected.add(f"app/{entry.name}/")
    pkg = repo_root / "pkg"
    if pkg.exists():
        for entry in sorted(pkg.iterdir()):
            if not entry.is_dir() or entry.name.startswith("."):
                continue
            if _has_go_file(entry):
                expected.add(f"pkg/{entry.name}/")
    return expected


def _check_tracked_paths(repo_root: Path, pages: list[Page]) -> list[CheckIssue]:
    issues: list[CheckIssue] = []
    for page in pages:
        for track in page.tracks:
            if not (repo_root / track).exists():
                issues.append(
                    CheckIssue(
                        "K003", str(page.path), f"tracked path missing: {track}"
                    )
                )
    return issues


def _check_module_coverage(
    repo_root: Path, kb_root: Path, pages: list[Page]
) -> list[CheckIssue]:
    module_pages = [p for p in pages if p.path.parent.name == "modules"]
    covered: dict[str, list[str]] = {}
    for page in module_pages:
        for track in page.tracks:
            covered.setdefault(track, []).append(str(page.path))
    issues: list[CheckIssue] = []
    expected = _expected_modules(repo_root)
    for module in sorted(expected - set(covered)):
        issues.append(
            CheckIssue(
                "K002", str(kb_root), f"module not covered by any page: {module}"
            )
        )
    for track, owners in sorted(covered.items()):
        if track in expected and len(owners) > 1:
            issues.append(
                CheckIssue(
                    "K002", owners[1], f"module covered by multiple pages: {track}"
                )
            )
    return issues


# Files outside modules/ and flows/ whose markdown links K004 also validates.
_EXTRA_LINK_FILES = (
    "docs/knowledge-base/README.md",
    "docs/knowledge-base/INDEX.md",
)


def _link_issues(repo_root: Path, source: Path, text: str) -> list[CheckIssue]:
    issues: list[CheckIssue] = []
    for link in extract_links(text):
        target = link.split("#", 1)[0].strip()
        if not target or target.startswith(("http://", "https://", "mailto:")):
            continue
        if target.startswith("/"):
            resolved = repo_root / target.lstrip("/")
        else:
            resolved = (source.parent / target).resolve()
        if not resolved.exists():
            issues.append(
                CheckIssue("K004", str(source), f"broken link: {link}")
            )
    return issues


def _check_links(repo_root: Path, pages: list[Page]) -> list[CheckIssue]:
    issues: list[CheckIssue] = []
    for page in pages:
        issues.extend(_link_issues(repo_root, page.path, page.body))
    for rel in _EXTRA_LINK_FILES:
        path = repo_root / rel
        if path.exists():
            issues.extend(
                _link_issues(repo_root, path, path.read_text(encoding="utf-8"))
            )
    return issues


def _check_index(kb_root: Path, pages: list[Page]) -> list[CheckIssue]:
    index_path = kb_root / "INDEX.md"
    if not index_path.exists():
        return [CheckIssue("K006", str(kb_root), "INDEX.md is missing")]
    index_text = index_path.read_text(encoding="utf-8")
    issues: list[CheckIssue] = []
    for page in pages:
        ref = f"{page.path.parent.name}/{page.path.name}"
        if ref not in index_text:
            issues.append(
                CheckIssue(
                    "K006", str(index_path), f"page not linked in INDEX.md: {ref}"
                )
            )
    return issues


def check_co_change(
    pages: list[Page], changed_files: set[str]
) -> list[CheckIssue]:
    issues: list[CheckIssue] = []
    for page in pages:
        rel_suffix = f"knowledge-base/{page.path.parent.name}/{page.path.name}"
        page_changed = any(
            c == str(page.path) or c.endswith(rel_suffix) for c in changed_files
        )
        if page_changed:
            continue
        for track in page.tracks:
            track_norm = track.rstrip("/")
            if any(
                c == track_norm or c.startswith(track_norm + "/")
                for c in changed_files
            ):
                issues.append(
                    CheckIssue(
                        "K005",
                        str(page.path),
                        f"tracked path changed but page not updated: {track}",
                    )
                )
                break
    return issues


def _git_changed_files(repo_root: Path, base_ref: str) -> set[str]:
    result = subprocess.run(
        ["git", "-C", str(repo_root), "diff", "--name-only", f"{base_ref}...HEAD"],
        capture_output=True,
        text=True,
        check=True,
    )
    return {line.strip() for line in result.stdout.splitlines() if line.strip()}


def stale_pages(repo_root: Path) -> list[tuple[Page, str]]:
    """Return ``(page, track)`` for pages whose tracked code drifted.

    A page is stale when ``git log last_synced_commit..HEAD`` over one of its
    tracked paths is non-empty.
    """
    pages, _ = _load_pages(_kb_root(repo_root))
    stale: list[tuple[Page, str]] = []
    for page in pages:
        for track in page.tracks:
            result = subprocess.run(
                [
                    "git",
                    "-C",
                    str(repo_root),
                    "log",
                    "--oneline",
                    f"{page.last_synced_commit}..HEAD",
                    "--",
                    track,
                ],
                capture_output=True,
                text=True,
            )
            if result.returncode == 0 and result.stdout.strip():
                stale.append((page, track))
                break
    return stale


def render_sync_report(stale: list[tuple[Page, str]]) -> str:
    """Render the knowledge base staleness report as markdown."""
    lines = ["# Knowledge Base Sync Status", ""]
    if not stale:
        lines.append("All knowledge base pages are in sync with their tracked code.")
    else:
        lines.append(
            "These pages track code that changed since `last_synced_commit` "
            "and need a content review:"
        )
        lines.append("")
        for page, track in stale:
            name = f"{page.path.parent.name}/{page.path.name}"
            lines.append(f"- `{name}` — tracked path `{track}` has drifted")
    return "\n".join(lines) + "\n"


def check_knowledge_base(
    repo_root: Path, *, base_ref: str | None = None
) -> list[CheckIssue]:
    kb_root = _kb_root(repo_root)
    issues: list[CheckIssue] = []
    pages, parse_issues = _load_pages(kb_root)
    issues.extend(parse_issues)
    issues.extend(_check_tracked_paths(repo_root, pages))
    issues.extend(_check_module_coverage(repo_root, kb_root, pages))
    issues.extend(_check_links(repo_root, pages))
    issues.extend(_check_index(kb_root, pages))
    if base_ref:
        changed = _git_changed_files(repo_root, base_ref)
        issues.extend(check_co_change(pages, changed))
    return issues
