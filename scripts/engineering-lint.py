#!/usr/bin/env python3
"""Validate agent-facing docs and the layered repository knowledge contract."""

import re
import sys
from pathlib import Path
from typing import Sequence

from knowledge import Knowledge, KnowledgeError, frontmatter, safe_path, texts

ROOT = Path(__file__).resolve().parent.parent
DOCS_DIR = ROOT / "docs"
KNOWLEDGE_DIR = DOCS_DIR / "knowledge"

# Only scan current agent-facing docs for broken references.
ACTIVE_DIRS = [
    KNOWLEDGE_DIR,
]

ACTIVE_FILES = {
    ROOT / "AGENTS.md",
    ROOT / "CLAUDE.md",
    DOCS_DIR / "INDEX.md",
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


def check_knowledge_governance(root: Path = ROOT) -> list[str]:
    try:
        policy, _ = frontmatter((root / "docs/knowledge/README.md").read_text())
        required = dict(
            owner="human",
            status="approved",
            agent_write_policy="human-authorized",
            authorization_mode="conversation",
        )
        for key, value in required.items():
            if policy.get(key) != value:
                raise KnowledgeError(f"governance {key} must be {value}")
        protected = texts(policy.get("protected_paths"), "protected_paths")
        minimum = {
            "AGENTS.md",
            "docs/INDEX.md",
            "docs/knowledge/README.md",
            "docs/knowledge/templates/",
            "docs/knowledge/intent/",
            "docs/knowledge/spec/",
        }
        if not minimum <= set(protected):
            raise KnowledgeError("governance is missing protected paths")
        for path in protected:
            local = (root / safe_path(path.rstrip("/"))).resolve()
            if not local.is_relative_to(root.resolve()) or not local.exists():
                raise KnowledgeError(
                    f"protected path missing or outside repository: {path}"
                )
        for path in texts(policy.get("legacy_upstream"), "legacy_upstream", empty=True):
            if not (root / safe_path(path)).exists() or not any(
                path == item.rstrip("/")
                or (item.endswith("/") and path.startswith(item))
                for item in protected
            ):
                raise KnowledgeError(
                    f"legacy upstream must exist and be protected: {path}"
                )
        for path in (root / "docs/knowledge/proposals").rglob("*.md"):
            if path.name == "README.md":
                continue
            meta, _ = frontmatter(path.read_text())
            if (
                not re.fullmatch(r"PROP-\d{8}-[a-z0-9]+(?:-[a-z0-9]+)*", path.stem)
                or meta.get("id") != path.stem
                or meta.get("layer") != "proposal"
            ):
                raise KnowledgeError(f"invalid proposal identity: {path.name}")
            if (
                meta.get("owner") != "agent"
                or meta.get("status") not in {"open", "closed", "superseded"}
                or meta.get("target_layer") not in {"intent", "spec"}
                or not isinstance(meta.get("title"), str)
                or not meta["title"].strip()
            ):
                raise KnowledgeError(
                    f"invalid proposal ownership, state or target: {path.name}"
                )
    except (KnowledgeError, OSError) as exc:
        return [f"[KNOWLEDGE] {exc}"]
    return []


def check_knowledge_layers(root: Path = ROOT) -> list[str]:
    try:
        Knowledge(root).validate()
    except (KnowledgeError, OSError) as exc:
        return [f"[KNOWLEDGE] {exc}"]
    return []


def is_checkable_reference(ref: str) -> bool:
    return not Path(ref).is_absolute()


def resolve_reference(md_file: Path, ref: str) -> Path:
    """Resolve a markdown file reference relative to the document or repo root."""
    candidates = [(md_file.parent / ref).resolve(), (ROOT / ref).resolve()]
    for candidate in candidates:
        if candidate.exists():
            return candidate
    return candidates[0]


def is_active_file(
    path: Path,
    root: Path = ROOT,
    active_files: Sequence[Path] | None = None,
    active_dirs: Sequence[Path] | None = None,
) -> bool:
    """Return whether a markdown file is part of the current doc surface."""
    files = ACTIVE_FILES if active_files is None else active_files
    dirs = ACTIVE_DIRS if active_dirs is None else active_dirs
    try:
        path.resolve().relative_to(root.resolve())
    except ValueError:
        return False
    if path.resolve() in {p.resolve() for p in files}:
        return True
    for active_dir in dirs:
        try:
            path.resolve().relative_to(active_dir.resolve())
            return True
        except ValueError:
            continue
    return False


def check_doc_policy(root: Path = ROOT):
    """Keep the default agent context small and single-sourced."""
    errors = []
    agents = root / "AGENTS.md"
    claude = root / "CLAUDE.md"

    agents_text = agents.read_text(encoding="utf-8")
    if len(agents_text.splitlines()) > 80:
        errors.append("[DOC-POLICY] AGENTS.md must stay within 80 lines")
    if "docs/knowledge/README.md" not in agents_text:
        errors.append(
            "[DOC-POLICY] AGENTS.md must route through docs/knowledge/README.md"
        )

    claude_lines = claude.read_text(encoding="utf-8").splitlines()
    if len(claude_lines) > 12:
        errors.append("[DOC-POLICY] CLAUDE.md must remain a compatibility pointer")
    if "AGENTS.md" not in claude.read_text(encoding="utf-8"):
        errors.append("[DOC-POLICY] CLAUDE.md must point to AGENTS.md")

    for directory in LEGACY_DOC_DIRS:
        if (root / directory.relative_to(ROOT)).exists():
            errors.append(
                f"[DOC-POLICY] legacy docs directory must be absent: {directory.relative_to(ROOT)}"
            )

    scan_files = [
        agents,
        claude,
        root / "docs" / "INDEX.md",
        root / "docs" / "knowledge" / "README.md",
    ]
    for path in scan_files:
        content = path.read_text(encoding="utf-8", errors="ignore")
        for term in LEGACY_TERMS:
            if term in content:
                errors.append(
                    f"[DOC-POLICY] legacy or broad-load instruction in {path.relative_to(root)}: {term}"
                )

    return errors


def check_md_file_links(root: Path = ROOT):
    """Check that Markdown file references resolve to existing files."""
    errors = []
    docs_dir = root / "docs"
    if not docs_dir.exists():
        return errors
    active_files = [
        root / "AGENTS.md",
        root / "CLAUDE.md",
        root / "docs" / "INDEX.md",
        root / "docs" / "knowledge" / "README.md",
    ]
    active_dirs = [root / "docs" / "knowledge"]
    legacy_evidence = root / "docs" / "knowledge" / "implementation" / "evidence"
    ref_patterns = [
        re.compile(r"\[([^\]]+)\]\(([^)]+)\)"),
        re.compile(
            r"`([a-zA-Z0-9_\-\./]+/(?:[a-zA-Z0-9_\-\.]+\.)"
            r"(?:md|go|py|yaml|yml|json|proto|api))`"
        ),
    ]

    for md_file in docs_dir.rglob("*.md"):
        if legacy_evidence in md_file.parents:
            continue
        if not is_active_file(md_file, root, active_files, active_dirs):
            continue
        rel_path = md_file.relative_to(root)
        content = md_file.read_text(encoding="utf-8", errors="ignore")
        lines = content.split("\n")

        for lineno, line in enumerate(lines, 1):
            for pattern in ref_patterns:
                for match in pattern.finditer(line):
                    ref = (
                        match.group(2)
                        if match.lastindex and match.lastindex >= 2
                        else match.group(1)
                    )
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


UNGENERATED_RPC_PROTOS = {}


def check_proto_generation(root: Path = ROOT) -> list[str]:
    generate_path = root / "scripts" / "generate.sh"
    proto_root = root / "proto"
    if not generate_path.exists() or not proto_root.exists():
        return []
    generate = generate_path.read_text(encoding="utf-8")
    errors: list[str] = []
    found_allowlist: set[str] = set()
    for proto in sorted(proto_root.rglob("*.proto")):
        rel = proto.relative_to(root).as_posix()
        text = proto.read_text(encoding="utf-8")
        if re.search(r"\brpc\s+\w+", text) is None:
            continue
        referenced = rel in generate or proto.name in generate
        if rel in UNGENERATED_RPC_PROTOS:
            found_allowlist.add(rel)
            if referenced:
                errors.append(
                    f"{rel}: listed as ungenerated but referenced by scripts/generate.sh"
                )
            continue
        if not referenced:
            errors.append(f"{rel}: RPC proto is not generated by scripts/generate.sh")
    stale = set(UNGENERATED_RPC_PROTOS) - found_allowlist
    if stale:
        errors.append("stale ungenerated proto allowlist: " + ", ".join(sorted(stale)))
    return errors


def main():
    errors = []
    errors.extend(check_doc_policy())
    errors.extend(check_knowledge_governance())
    errors.extend(check_knowledge_layers())
    errors.extend(check_md_file_links())
    errors.extend(check_proto_generation())

    if errors:
        print("\n".join(errors))
        print(f"\n{len(errors)} engineering-lint error(s) found")

    exit_code = 1 if errors else 0
    if exit_code == 0:
        print("engineering-lint: all checks passed")
    sys.exit(exit_code)


if __name__ == "__main__":
    main()
