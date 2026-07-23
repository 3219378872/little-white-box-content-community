#!/usr/bin/env python3
"""Engineering-lint: validates active docs, links, and repository policy."""

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOCS_DIR = ROOT / "docs"

# Only scan current agent-facing docs for broken references.
ACTIVE_DIRS = [
    DOCS_DIR / "active",
]

ACTIVE_FILES = {
    ROOT / "AGENTS.md",
    ROOT / "CLAUDE.md",
    DOCS_DIR / "INDEX.md",
    DOCS_DIR / "ARCHITECTURE.md",
    DOCS_DIR / "DESIGN.md",
    DOCS_DIR / "SECURITY.md",
    DOCS_DIR / "RELIABILITY.md",
    DOCS_DIR / "QUALITY_SCORE.md",
    DOCS_DIR / "generated" / "INDEX.md",
}

EXPECTED_ACTIVE_DOCS = {
    "api.md",
    "data.md",
    "operations.md",
    "rpc.md",
    "security.md",
    "testing.md",
}

LEGACY_DOC_DIRS = (
    DOCS_DIR / "references",
    DOCS_DIR / "design-docs",
    DOCS_DIR / "exec-plans",
)

LEGACY_TERMS = (
    "zero-powers",
    "zero-skills",
    "superpowers:",
    "read the entire docs",
    "阅读整个 docs",
    "阅读全部 docs",
)


def is_checkable_reference(ref: str) -> bool:
    return not Path(ref).is_absolute()


def resolve_reference(md_file: Path, ref: str) -> Path:
    """Resolve a markdown file reference relative to the document or repo root."""
    candidates = [(md_file.parent / ref).resolve(), (ROOT / ref).resolve()]
    parts = Path(ref).parts
    if parts:
        if parts[0] == "flows" or parts[0] == "modules":
            candidates.append((ROOT / "docs" / "generated" / ref).resolve())
    for candidate in candidates:
        if candidate.exists():
            return candidate
    return candidates[0]


def is_active_file(path: Path) -> bool:
    """Return whether a markdown file is part of the current doc surface."""
    try:
        path.resolve().relative_to(ROOT)
    except ValueError:
        return False
    if path.resolve() in {p.resolve() for p in ACTIVE_FILES}:
        return True
    for ad in ACTIVE_DIRS:
        try:
            path.resolve().relative_to(ad.resolve())
            return True
        except ValueError:
            continue
    return False


def check_doc_policy():
    """Keep the default agent context small and single-sourced."""
    errors = []
    agents = ROOT / "AGENTS.md"
    claude = ROOT / "CLAUDE.md"

    if len(agents.read_text(encoding="utf-8").splitlines()) > 80:
        errors.append("[DOC-POLICY] AGENTS.md must stay within 80 lines")

    claude_lines = claude.read_text(encoding="utf-8").splitlines()
    if len(claude_lines) > 12:
        errors.append("[DOC-POLICY] CLAUDE.md must remain a compatibility pointer")
    if "AGENTS.md" not in claude.read_text(encoding="utf-8"):
        errors.append("[DOC-POLICY] CLAUDE.md must point to AGENTS.md")

    active_dir = DOCS_DIR / "active"
    actual = {path.name for path in active_dir.glob("*.md")}
    missing = EXPECTED_ACTIVE_DOCS - actual
    extra = actual - EXPECTED_ACTIVE_DOCS
    if missing:
        errors.append(f"[DOC-POLICY] missing active docs: {', '.join(sorted(missing))}")
    if extra:
        errors.append(f"[DOC-POLICY] unexpected active docs: {', '.join(sorted(extra))}")

    for path in active_dir.glob("*.md"):
        if len(path.read_text(encoding="utf-8").splitlines()) > 300:
            errors.append(f"[DOC-POLICY] active doc exceeds 300 lines: {path.relative_to(ROOT)}")

    for directory in LEGACY_DOC_DIRS:
        if directory.exists():
            errors.append(f"[DOC-POLICY] legacy docs directory must be absent: {directory.relative_to(ROOT)}")

    scan_files = [agents, claude, DOCS_DIR / "INDEX.md"]
    scan_files.extend(active_dir.glob("*.md"))
    for path in scan_files:
        content = path.read_text(encoding="utf-8", errors="ignore")
        for term in LEGACY_TERMS:
            if term in content:
                errors.append(f"[DOC-POLICY] legacy or broad-load instruction in {path.relative_to(ROOT)}: {term}")

    return errors


def check_md_file_links():
    """Check that markdown file references resolve to existing files."""
    errors = []
    ref_patterns = [
        re.compile(r"\[([^\]]+)\]\(([^)]+)\)"),
        re.compile(
            r"`([a-zA-Z0-9_\-\./]+/(?:[a-zA-Z0-9_\-\.]+\.)"
            r"(?:md|go|py|yaml|yml|json|proto|api))`"
        ),
    ]

    for md_file in DOCS_DIR.rglob("*.md"):
        if not is_active_file(md_file):
            continue
        rel_path = md_file.relative_to(ROOT)
        content = md_file.read_text(encoding="utf-8", errors="ignore")
        lines = content.split("\n")

        for lineno, line in enumerate(lines, 1):
            for pattern in ref_patterns:
                for m in pattern.finditer(line):
                    ref = m.group(2) if m.lastindex and m.lastindex >= 2 else m.group(1)
                    if not ref:
                        continue
                    if ref.startswith(
                        ("http://", "https://", "#", "mailto:")
                    ) or not is_checkable_reference(ref):
                        continue
                    ref_path = resolve_reference(md_file, ref)
                    if not ref_path.exists():
                        errors.append(
                            f"[MD-REF] {rel_path}:{lineno}: "
                            f"referenced path does not exist: {ref}"
                        )

    return errors


def main():
    errors = []

    errors.extend(check_doc_policy())
    errors.extend(check_md_file_links())

    if errors:
        print("\n".join(errors))
        print(f"\n{len(errors)} engineering-lint error(s) found")

    exit_code = 1 if errors else 0
    if exit_code == 0:
        print("engineering-lint: all checks passed")
    sys.exit(exit_code)


if __name__ == "__main__":
    main()
