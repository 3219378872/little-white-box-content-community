#!/usr/bin/env python3
"""Validate agent-facing docs and the layered repository knowledge contract."""

import re
import sys
from pathlib import Path
from typing import Sequence

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

REQUIRED_KNOWLEDGE_FILES = {
    "docs/knowledge/README.md",
    "docs/knowledge/TRANSITION.md",
    "docs/knowledge/intent/README.md",
    "docs/knowledge/spec/README.md",
    "docs/knowledge/design/README.md",
    "docs/knowledge/implementation/README.md",
    "docs/knowledge/implementation/evidence/README.md",
    "docs/knowledge/proposals/README.md",
    "docs/knowledge/templates/intent.md",
    "docs/knowledge/templates/spec.md",
    "docs/knowledge/templates/design.md",
    "docs/knowledge/templates/implementation.md",
    "docs/knowledge/templates/proposal.md",
    "docs/knowledge/templates/evidence.md",
}

REQUIRED_PROTECTED_PATHS = {
    "AGENTS.md",
    "docs/INDEX.md",
    "docs/knowledge/README.md",
    "docs/knowledge/templates/",
    "docs/knowledge/intent/",
    "docs/knowledge/spec/",
}

REQUIRED_AGENT_WRITE_POLICY = "human-authorized"
REQUIRED_AUTHORIZATION_MODE = "conversation"

LAYER_DIRS = {
    "intent": "intent",
    "spec": "spec",
    "design": "design",
    "implementation": "implementation",
}

LAYER_ID_PATTERNS = {
    "intent": re.compile(r"^INT-[a-z0-9]+(?:-[a-z0-9]+)*$"),
    "spec": re.compile(r"^SPEC-[a-z0-9]+(?:-[a-z0-9]+)*$"),
    "design": re.compile(r"^DES-[a-z0-9]+(?:-[a-z0-9]+)*$"),
    "implementation": re.compile(r"^IMP-[a-z0-9]+(?:-[a-z0-9]+)*$"),
    "proposal": re.compile(r"^PROP-[0-9]{8}-[a-z0-9]+(?:-[a-z0-9]+)*$"),
}

LAYER_OWNERS = {
    "intent": "human",
    "spec": "human",
    "design": "agent",
    "implementation": "agent",
    "proposal": "agent",
}

LAYER_STATUSES = {
    "intent": {"draft", "approved", "retired"},
    "spec": {"draft", "approved", "retired"},
    "design": {"draft", "active", "blocked", "superseded"},
    "implementation": {"unknown", "aligned", "diverged", "retired"},
    "proposal": {"open", "closed", "superseded"},
}

_FRONTMATTER_RE = re.compile(r"\A---\n(.*?)\n---(?:\n|\Z)", re.DOTALL)
_DATE_RE = re.compile(r"^[0-9]{4}-[0-9]{2}-[0-9]{2}$")
_COMMIT_RE = re.compile(r"^[0-9a-f]{7,40}$")


class FrontmatterError(ValueError):
    """Raised when a knowledge page has malformed frontmatter."""


def _display_path(path: Path, root: Path) -> str:
    try:
        return str(path.relative_to(root))
    except ValueError:
        return str(path)


def _knowledge_error(path: Path, root: Path, message: str) -> str:
    return f"[KNOWLEDGE] {_display_path(path, root)}: {message}"


def _strip_scalar(raw: str):
    value = raw.strip()
    if value == "[]":
        return []
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
        return value[1:-1]
    return value


def parse_frontmatter(path: Path) -> dict[str, object]:
    """Parse the small YAML subset used by repository knowledge pages."""
    text = path.read_text(encoding="utf-8")
    match = _FRONTMATTER_RE.match(text)
    if match is None:
        raise FrontmatterError("missing or malformed frontmatter")

    data: dict[str, object] = {}
    current_list: list[str] | None = None
    for line_no, line in enumerate(match.group(1).splitlines(), start=2):
        if not line.strip():
            continue
        if line.startswith("  - "):
            if current_list is None:
                raise FrontmatterError(
                    f"line {line_no}: list item without a list-valued key"
                )
            current_list.append(str(_strip_scalar(line[4:])))
            continue
        if ":" not in line:
            raise FrontmatterError(f"line {line_no}: expected 'key: value'")
        key, _, raw = line.partition(":")
        key = key.strip()
        if not key:
            raise FrontmatterError(f"line {line_no}: empty key")
        if key in data:
            raise FrontmatterError(f"line {line_no}: duplicate key: {key}")
        if not raw.strip():
            current_list = []
            data[key] = current_list
        else:
            current_list = None
            data[key] = _strip_scalar(raw)
    return data


def _string_list(data: dict[str, object], key: str) -> list[str] | None:
    value = data.get(key)
    if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
        return None
    return value


def _path_is_protected(path: str, protected_paths: set[str]) -> bool:
    return any(
        path == protected.rstrip("/")
        or (protected.endswith("/") and path.startswith(protected))
        for protected in protected_paths
    )


def _heading_exists(path: Path, heading: str) -> bool:
    for line in path.read_text(encoding="utf-8", errors="ignore").splitlines():
        match = re.match(r"^#{1,6}\s+(.+?)\s*#*\s*$", line)
        if match and match.group(1).strip() == heading:
            return True
    return False


def _load_knowledge_governance(root: Path, errors: list[str]) -> set[str]:
    policy_path = root / "docs" / "knowledge" / "README.md"
    try:
        policy = parse_frontmatter(policy_path)
    except (OSError, FrontmatterError) as exc:
        errors.append(_knowledge_error(policy_path, root, str(exc)))
        return set()

    if policy.get("owner") != "human":
        errors.append(_knowledge_error(policy_path, root, "owner must be human"))
    if policy.get("status") != "approved":
        errors.append(_knowledge_error(policy_path, root, "status must be approved"))
    if policy.get("agent_write_policy") != REQUIRED_AGENT_WRITE_POLICY:
        errors.append(
            _knowledge_error(
                policy_path,
                root,
                f"agent_write_policy must be {REQUIRED_AGENT_WRITE_POLICY}",
            )
        )
    if policy.get("authorization_mode") != REQUIRED_AUTHORIZATION_MODE:
        errors.append(
            _knowledge_error(
                policy_path,
                root,
                f"authorization_mode must be {REQUIRED_AUTHORIZATION_MODE}",
            )
        )

    protected = _string_list(policy, "protected_paths")
    if protected is None:
        errors.append(
            _knowledge_error(policy_path, root, "protected_paths must be a list")
        )
        protected_set: set[str] = set()
    else:
        protected_set = set(protected)
        missing = REQUIRED_PROTECTED_PATHS - protected_set
        if missing:
            errors.append(
                _knowledge_error(
                    policy_path,
                    root,
                    "missing protected paths: " + ", ".join(sorted(missing)),
                )
            )
        for rel in protected:
            if not (root / rel).exists():
                errors.append(
                    _knowledge_error(policy_path, root, f"protected path missing: {rel}")
                )

    legacy = _string_list(policy, "legacy_upstream")
    if legacy is None:
        errors.append(
            _knowledge_error(policy_path, root, "legacy_upstream must be a list")
        )
        return set()

    for rel in legacy:
        if not (root / rel).exists():
            errors.append(
                _knowledge_error(policy_path, root, f"legacy upstream missing: {rel}")
            )
        if not _path_is_protected(rel, protected_set):
            errors.append(
                _knowledge_error(
                    policy_path, root, f"legacy upstream is not protected: {rel}"
                )
            )
    return set(legacy)


def _knowledge_page_paths(root: Path) -> list[tuple[str, Path]]:
    knowledge = root / "docs" / "knowledge"
    pages: list[tuple[str, Path]] = []
    for layer, directory in LAYER_DIRS.items():
        layer_dir = knowledge / directory
        if not layer_dir.exists():
            continue
        for path in sorted(layer_dir.rglob("*.md")):
            if path.name == "README.md":
                continue
            if layer == "implementation" and "evidence" in path.relative_to(layer_dir).parts:
                continue
            pages.append((layer, path))
    proposals = knowledge / "proposals"
    if proposals.exists():
        for path in sorted(proposals.rglob("*.md")):
            if path.name != "README.md":
                pages.append(("proposal", path))
    return pages


def _validate_legacy_references(
    path: Path,
    root: Path,
    references: list[str],
    allowed_paths: set[str],
) -> list[str]:
    errors: list[str] = []
    for reference in references:
        if not reference.startswith("legacy:"):
            errors.append(
                _knowledge_error(
                    path, root, f"invalid legacy reference syntax: {reference}"
                )
            )
            continue
        target, separator, heading = reference.removeprefix("legacy:").partition("#")
        if target not in allowed_paths:
            errors.append(
                _knowledge_error(path, root, f"legacy path is not allowlisted: {target}")
            )
            continue
        target_path = root / target
        if not separator or not heading:
            errors.append(
                _knowledge_error(path, root, f"legacy reference needs a heading: {reference}")
            )
        elif target_path.exists() and not _heading_exists(target_path, heading):
            errors.append(
                _knowledge_error(path, root, f"legacy heading does not exist: {reference}")
            )
    return errors


def _validate_typed_upstream(
    path: Path,
    root: Path,
    upstream: list[str],
    expected_layer: str,
    documents: dict[str, tuple[str, Path, dict[str, object]]],
) -> tuple[list[str], list[tuple[str, Path, dict[str, object]]]]:
    errors: list[str] = []
    targets: list[tuple[str, Path, dict[str, object]]] = []
    pattern = LAYER_ID_PATTERNS[expected_layer]
    expected_prefix = {
        "intent": "INT-",
        "spec": "SPEC-",
        "design": "DES-",
    }[expected_layer]
    for reference in upstream:
        if reference.startswith("PROP-"):
            errors.append(
                _knowledge_error(
                    path, root, f"proposal cannot be an upstream: {reference}"
                )
            )
            continue
        if not pattern.fullmatch(reference):
            errors.append(
                _knowledge_error(
                    path, root, f"upstream must reference {expected_prefix}: {reference}"
                )
            )
            continue
        target = documents.get(reference)
        if target is None:
            errors.append(
                _knowledge_error(path, root, f"upstream document does not exist: {reference}")
            )
            continue
        if target[0] != expected_layer:
            errors.append(
                _knowledge_error(path, root, f"upstream has wrong layer: {reference}")
            )
            continue
        targets.append(target)
    return errors, targets


def _validate_implementation_fields(
    path: Path, root: Path, data: dict[str, object]
) -> list[str]:
    errors: list[str] = []
    tracks = _string_list(data, "tracks")
    if not tracks:
        errors.append(_knowledge_error(path, root, "tracks must be a non-empty list"))
    else:
        for track in tracks:
            if not (root / track).exists():
                errors.append(
                    _knowledge_error(path, root, f"tracked path does not exist: {track}")
                )

    if data.get("status") != "retired":
        verified_at = data.get("verified_at")
        verified_commit = data.get("verified_commit")
        if not isinstance(verified_at, str) or not _DATE_RE.fullmatch(verified_at):
            errors.append(
                _knowledge_error(path, root, "verified_at must use YYYY-MM-DD")
            )
        if not isinstance(verified_commit, str) or not _COMMIT_RE.fullmatch(
            verified_commit
        ):
            errors.append(
                _knowledge_error(path, root, "verified_commit must be a 7-40 character hex SHA")
            )
    return errors


def _check_evidence_pages(
    root: Path,
    documents: dict[str, tuple[str, Path, dict[str, object]]],
) -> list[str]:
    errors: list[str] = []
    evidence_dir = root / "docs" / "knowledge" / "implementation" / "evidence"
    if not evidence_dir.exists():
        return errors
    readme = evidence_dir / "README.md"
    registered = set()
    if readme.is_file():
        readme_text = readme.read_text(encoding="utf-8", errors="ignore")
        for match in re.finditer(r"\]\(([^)#]+)(?:#[^)]*)?\)", readme_text):
            target = match.group(1)
            if target.endswith(".md"):
                registered.add(Path(target).name)
        for name in sorted(registered):
            if not (evidence_dir / name).is_file():
                errors.append(
                    _knowledge_error(
                        readme, root, f"evidence README links missing file: {name}"
                    )
                )
    else:
        errors.append(
            _knowledge_error(
                readme, root, "evidence directory must contain a README.md index"
            )
        )
    for path in sorted(evidence_dir.rglob("*.md")):
        if path.name == "README.md":
            continue
        if readme.is_file() and path.name not in registered:
            errors.append(
                _knowledge_error(
                    path, root, "evidence file is not registered in evidence/README.md"
                )
            )
        try:
            data = parse_frontmatter(path)
        except FrontmatterError as exc:
            errors.append(_knowledge_error(path, root, str(exc)))
            continue
        required = {"implementation", "verified_at", "verified_commit", "commands", "result"}
        missing = required - set(data)
        if missing:
            errors.append(
                _knowledge_error(
                    path, root, "missing frontmatter keys: " + ", ".join(sorted(missing))
                )
            )
            continue
        implementation = data.get("implementation")
        target = documents.get(str(implementation))
        if target is None or target[0] != "implementation":
            errors.append(
                _knowledge_error(
                    path, root, f"implementation does not exist: {implementation}"
                )
            )
        verified_at = data.get("verified_at")
        if not isinstance(verified_at, str) or not _DATE_RE.fullmatch(verified_at):
            errors.append(
                _knowledge_error(path, root, "verified_at must use YYYY-MM-DD")
            )
        elif not path.name.startswith(verified_at + "-"):
            errors.append(
                _knowledge_error(path, root, "filename must start with verified_at")
            )
        verified_commit = data.get("verified_commit")
        if not isinstance(verified_commit, str) or not _COMMIT_RE.fullmatch(
            verified_commit
        ):
            errors.append(
                _knowledge_error(path, root, "verified_commit must be a 7-40 character hex SHA")
            )
        commands = _string_list(data, "commands")
        if not commands:
            errors.append(_knowledge_error(path, root, "commands must be a non-empty list"))
        if data.get("result") not in {"passed", "failed", "partial", "blocked"}:
            errors.append(
                _knowledge_error(
                    path, root, "result must be passed, failed, partial, or blocked"
                )
            )
    return errors


def check_knowledge_layers(root: Path = ROOT) -> list[str]:
    """Validate ownership metadata and one-way knowledge references."""
    errors: list[str] = []
    for rel in sorted(REQUIRED_KNOWLEDGE_FILES):
        if not (root / rel).is_file():
            errors.append(
                _knowledge_error(root / rel, root, "required knowledge file is missing")
            )

    policy_path = root / "docs" / "knowledge" / "README.md"
    if not policy_path.is_file():
        return errors
    legacy_paths = _load_knowledge_governance(root, errors)

    records: list[tuple[str, Path, dict[str, object]]] = []
    documents: dict[str, tuple[str, Path, dict[str, object]]] = {}
    required_common = {"id", "layer", "title", "status", "owner"}
    for expected_layer, path in _knowledge_page_paths(root):
        try:
            data = parse_frontmatter(path)
        except FrontmatterError as exc:
            errors.append(_knowledge_error(path, root, str(exc)))
            continue
        required = required_common | ({"target_layer"} if expected_layer == "proposal" else {"upstream"})
        missing = required - set(data)
        if missing:
            errors.append(
                _knowledge_error(
                    path, root, "missing frontmatter keys: " + ", ".join(sorted(missing))
                )
            )
            continue

        document_id = data.get("id")
        if not isinstance(document_id, str) or not LAYER_ID_PATTERNS[
            expected_layer
        ].fullmatch(document_id):
            errors.append(
                _knowledge_error(path, root, f"invalid {expected_layer} id: {document_id}")
            )
            continue
        if data.get("layer") != expected_layer:
            errors.append(
                _knowledge_error(path, root, f"layer must be {expected_layer}")
            )
        if data.get("owner") != LAYER_OWNERS[expected_layer]:
            errors.append(
                _knowledge_error(
                    path, root, f"owner must be {LAYER_OWNERS[expected_layer]}"
                )
            )
        if data.get("status") not in LAYER_STATUSES[expected_layer]:
            errors.append(
                _knowledge_error(
                    path,
                    root,
                    "invalid status for " + expected_layer + f": {data.get('status')}",
                )
            )
        if not isinstance(data.get("title"), str) or not str(data.get("title")).strip():
            errors.append(_knowledge_error(path, root, "title must be non-empty"))
        if expected_layer != "proposal" and _string_list(data, "upstream") is None:
            errors.append(_knowledge_error(path, root, "upstream must be a list"))
        if expected_layer == "proposal" and data.get("target_layer") not in {
            "intent",
            "spec",
        }:
            errors.append(
                _knowledge_error(path, root, "target_layer must be intent or spec")
            )

        record = (expected_layer, path, data)
        records.append(record)
        if document_id in documents:
            first_path = _display_path(documents[document_id][1], root)
            errors.append(
                _knowledge_error(path, root, f"duplicate id {document_id}; first seen in {first_path}")
            )
        else:
            documents[document_id] = record

    for layer, path, data in records:
        if layer == "proposal":
            continue
        upstream = _string_list(data, "upstream")
        if upstream is None:
            continue
        if layer == "intent":
            if upstream:
                errors.append(_knowledge_error(path, root, "intent upstream must be empty"))
            continue
        if layer == "spec":
            if not upstream:
                errors.append(_knowledge_error(path, root, "spec requires an INT upstream"))
                continue
            ref_errors, targets = _validate_typed_upstream(
                path, root, upstream, "intent", documents
            )
            errors.extend(ref_errors)
            if data.get("status") == "approved":
                for _, _, target_data in targets:
                    if target_data.get("status") != "approved":
                        errors.append(
                            _knowledge_error(
                                path, root, "approved spec requires approved intent"
                            )
                        )
            continue
        if layer == "design":
            legacy = _string_list(data, "legacy_upstream")
            if legacy is None and "legacy_upstream" in data:
                errors.append(
                    _knowledge_error(path, root, "legacy_upstream must be a list")
                )
                legacy = []
            legacy = legacy or []
            if not upstream and not legacy:
                if data.get("status") != "blocked" or not data.get("blocking_reason"):
                    errors.append(
                        _knowledge_error(
                            path,
                            root,
                            "design requires SPEC/legacy upstream or a blocked reason",
                        )
                    )
            ref_errors, targets = _validate_typed_upstream(
                path, root, upstream, "spec", documents
            )
            errors.extend(ref_errors)
            errors.extend(
                _validate_legacy_references(path, root, legacy, legacy_paths)
            )
            if data.get("status") == "active":
                for _, _, target_data in targets:
                    if target_data.get("status") != "approved":
                        errors.append(
                            _knowledge_error(
                                path, root, "active design requires approved spec"
                            )
                        )
            continue
        if layer == "implementation":
            if not upstream:
                errors.append(
                    _knowledge_error(path, root, "implementation requires a DES upstream")
                )
            ref_errors, targets = _validate_typed_upstream(
                path, root, upstream, "design", documents
            )
            errors.extend(ref_errors)
            if data.get("status") in {"aligned", "diverged"}:
                for _, _, target_data in targets:
                    if target_data.get("status") != "active":
                        errors.append(
                            _knowledge_error(
                                path,
                                root,
                                "aligned/diverged implementation requires active design",
                            )
                        )
            errors.extend(_validate_implementation_fields(path, root, data))

    errors.extend(_check_evidence_pages(root, documents))
    return errors


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
        errors.append("[DOC-POLICY] AGENTS.md must route through docs/knowledge/README.md")

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
        agents, claude, root / "docs" / "INDEX.md", root / "docs" / "knowledge" / "README.md"
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
        root / "AGENTS.md", root / "CLAUDE.md", root / "docs" / "INDEX.md",
        root / "docs" / "knowledge" / "README.md",
    ]
    active_dirs = [root / "docs" / "knowledge"]
    ref_patterns = [
        re.compile(r"\[([^\]]+)\]\(([^)]+)\)"),
        re.compile(
            r"`([a-zA-Z0-9_\-\./]+/(?:[a-zA-Z0-9_\-\.]+\.)"
            r"(?:md|go|py|yaml|yml|json|proto|api))`"
        ),
    ]

    for md_file in docs_dir.rglob("*.md"):
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
        errors.append(
            "stale ungenerated proto allowlist: " + ", ".join(sorted(stale))
        )
    return errors



SPEC_TRACKING_IMP = Path("docs/knowledge/implementation/IMP-content-community-backend.md")
SPEC_TRACKING_DES = Path("docs/knowledge/design/DES-content-community-backend.md")
SPEC_TRACKING_HEADINGS = (
    "SPEC-community-core 追踪",
    "SPEC-content-discovery 追踪",
    "SPEC-grounded-assistant 追踪",
    "SPEC-feedback-reliability 追踪",
)
FORBIDDEN_ALIGNED_REQUIREMENTS = {
    # CORE-013 已于 2026-08-13 经人类采纳选项 B（/api/v2 强制 expected_revision，
    # v1 迁移期）后关闭，不再禁止 aligned；见 proposals/PROP-20260813-core-revision-contract。
    "CORE-032",
    "DISC-060",
    "DISC-063",
    "ASST-013",
    "ASST-014",
    "ASST-050",
    "ASST-051",
    "REL-030",
    "REL-031",
    "REL-032",
    "REL-033",
    "REL-040",
    "REL-041",
    "REL-042",
    "REL-043",
}
_TRACK_ROW = re.compile(
    r"^\|\s*(?P<req>(?:CORE|DISC|ASST|REL)-\d+(?:~\d+)?)\b[^|]*\|\s*(?P<status>aligned|partial|missing|n/a)\s\|",
    re.IGNORECASE,
)


def _expand_requirement_id(raw: str) -> list[str]:
    match = re.fullmatch(r"(CORE|DISC|ASST|REL)-(\d+)(?:~(\d+))?", raw)
    if not match:
        return [raw]
    prefix, start_raw, end_raw = match.group(1), match.group(2), match.group(3)
    start = int(start_raw)
    end = int(end_raw) if end_raw is not None else start
    width = len(start_raw)
    return [f"{prefix}-{value:0{width}d}" for value in range(start, end + 1)]


def _parse_tracking_rows(text: str) -> list[tuple[str, str]]:
    rows: list[tuple[str, str]] = []
    for line in text.splitlines():
        match = _TRACK_ROW.match(line)
        if not match:
            continue
        status = match.group("status").lower()
        for req_id in _expand_requirement_id(match.group("req")):
            rows.append((req_id, status))
    return rows


def check_spec_tracking(root: Path = ROOT) -> list[str]:
    """Keep requirement status in IMP and stop blocked items from being marked aligned."""
    errors: list[str] = []
    imp = root / SPEC_TRACKING_IMP
    des = root / SPEC_TRACKING_DES
    if not imp.is_file() or not des.is_file():
        return errors
    des_text = des.read_text(encoding="utf-8")
    for heading in SPEC_TRACKING_HEADINGS:
        if re.search(r"^## " + re.escape(heading) + r"\s*$", des_text, re.MULTILINE):
            errors.append(
                _knowledge_error(
                    des,
                    root,
                    f"design must not contain implementation tracking heading {heading!r}",
                )
            )
    imp_text = imp.read_text(encoding="utf-8")
    for heading in SPEC_TRACKING_HEADINGS:
        if not re.search(r"^## " + re.escape(heading) + r"\s*$", imp_text, re.MULTILINE):
            errors.append(
                _knowledge_error(
                    imp,
                    root,
                    f"implementation tracking heading {heading!r} is required",
                )
            )
    for req_id, status in _parse_tracking_rows(imp_text):
        if status == "aligned" and req_id in FORBIDDEN_ALIGNED_REQUIREMENTS:
            errors.append(
                _knowledge_error(
                    imp,
                    root,
                    f"{req_id} cannot be marked aligned until the human-owned gap is closed",
                )
            )
    return errors


def main():
    errors = []
    errors.extend(check_doc_policy())
    errors.extend(check_knowledge_layers())
    errors.extend(check_spec_tracking())
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
