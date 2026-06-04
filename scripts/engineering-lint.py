#!/usr/bin/env python3
"""Engineering-lint: validates md references, KB sync, and harness compliance."""

import os
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOCS_DIR = ROOT / "docs"

# Only scan these directories for broken references in active records.
ACTIVE_DIRS = [
    DOCS_DIR / "exec-plans" / "active",
]


def is_checkable_reference(ref: str) -> bool:
    return not Path(ref).is_absolute()


def resolve_reference(md_file: Path, ref: str) -> Path:
    """Resolve a markdown file reference relative to the document or repo root."""
    candidates = [(md_file.parent / ref).resolve(), (ROOT / ref).resolve()]
    parts = Path(ref).parts
    if parts:
        if parts[0] == "flows" or parts[0] == "modules":
            candidates.append((ROOT / "docs" / "generated" / ref).resolve())
        if parts[0] in {"_knowledge_base", "_agent_harness"}:
            candidates.append((ROOT / "scripts" / ref).resolve())
    for candidate in candidates:
        if candidate.exists():
            return candidate
    return candidates[0]


def is_active_file(path: Path) -> bool:
    """Only lint files under active task/harness directories."""
    try:
        path.resolve().relative_to(ROOT)
    except ValueError:
        return False
    for ad in ACTIVE_DIRS:
        try:
            path.resolve().relative_to(ad.resolve())
            return True
        except ValueError:
            continue
    return False


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

    errors.extend(check_md_file_links())

    kb_check = os.system(f"cd {ROOT} && python3 scripts/knowledge_base.py check")

    if errors:
        print("\n".join(errors))
        print(f"\n{len(errors)} engineering-lint error(s) found")

    exit_code = 1 if errors or kb_check != 0 else 0
    if exit_code == 0:
        print("engineering-lint: all checks passed")
    sys.exit(exit_code)


if __name__ == "__main__":
    main()
