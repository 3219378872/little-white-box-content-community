#!/usr/bin/env python3
"""Validate agent-facing docs and the layered repository knowledge contract."""

import re
import subprocess
import sys
from datetime import date
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
    "docs/knowledge/evidence/README.md",
    "docs/knowledge/guides/README.md",
    "docs/knowledge/implementation/README.md",
    "docs/knowledge/implementation/evidence/README.md",
    "docs/knowledge/proposals/README.md",
    "docs/knowledge/status/README.md",
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
    "evidence": "evidence",
}

LAYER_ID_PATTERNS = {
    "intent": re.compile(r"^INT-[a-z0-9]+(?:-[a-z0-9]+)*$"),
    "spec": re.compile(r"^SPEC-[a-z0-9]+(?:-[a-z0-9]+)*$"),
    "design": re.compile(r"^DES-[a-z0-9]+(?:-[a-z0-9]+)*$"),
    "implementation": re.compile(r"^IMP-[a-z0-9]+(?:-[a-z0-9]+)*$"),
    "evidence": re.compile(r"^EVD-[a-z0-9]+(?:-[a-z0-9]+)*$"),
    "proposal": re.compile(r"^PROP-[0-9]{8}-[a-z0-9]+(?:-[a-z0-9]+)*$"),
}

LAYER_OWNERS = {
    "intent": "human",
    "spec": "human",
    "design": "agent",
    "implementation": "agent",
    "evidence": "agent",
    "proposal": "agent",
}

LAYER_STATUSES = {
    "intent": {"draft", "approved", "retired"},
    "spec": {"draft", "approved", "retired"},
    "design": {"draft", "active", "blocked", "superseded"},
    "implementation": {"unknown", "aligned", "diverged", "retired"},
    "evidence": {"active", "superseded"},
    "proposal": {"open", "closed", "superseded"},
}

_FRONTMATTER_RE = re.compile(r"\A---\n(.*?)\n---(?:\n|\Z)", re.DOTALL)
_DATE_RE = re.compile(r"^[0-9]{4}-[0-9]{2}-[0-9]{2}$")
_FULL_COMMIT_RE = re.compile(r"^[0-9a-f]{40}$")
_REQUIREMENT_ID = r"[A-Z][A-Z0-9]*-(?:A[0-9]{2}|[0-9]{3}(?:-[0-9]{2})?)"
_REQUIREMENT_BULLET = re.compile(
    rf"^ {{0,3}}[-*]\s+`(?P<requirement>{_REQUIREMENT_ID})`[：:]"
)
_REQUIREMENT_TABLE_CELL = re.compile(
    rf"^(?:`(?P<quoted>{_REQUIREMENT_ID})`|(?P<plain>{_REQUIREMENT_ID}))$"
)
_TABLE_SEPARATOR_CELL = re.compile(r"^:?-{3,}:?$")
_REQUIREMENT_TABLE_HEADERS = {"requirement", "条款", "id"}
_INLINE_CODE_MASK = "x"
_AUTHORITY_TABLE_HEADER = "| Requirement | Design | Status | Evidence/Gap |"
_AUTHORITY_TABLE_SEPARATOR = "| --- | --- | --- | --- |"
_TRACKING_ROW = re.compile(
    rf"^\|\s*`?(?P<requirement>{_REQUIREMENT_ID})`?\s*\|\s*"
    r"`?(?P<design>DES-[a-z0-9]+(?:-[a-z0-9]+)*)`?\s*\|\s*"
    r"(?P<status>aligned|diverged|unknown)\s*\|\s*(?P<detail>.+?)\s*\|$"
)
_EXTERNAL_UPSTREAM = re.compile(
    r"^(?P<repo>little-white-box|little-white-box-content-community|little-white-box-front)@"
    r"(?P<commit>[0-9a-f]{40}):"
    rf"(?P<id>(?:(?:INT|SPEC|DES|IMP|EVD)-[a-z0-9]+(?:-[a-z0-9]+)*|{_REQUIREMENT_ID}))$"
)
EVIDENCE_SCOPES = {
    "static",
    "unit",
    "integration",
    "e2e",
    "browser",
    "device",
    "synthetic",
    "human-review",
    "live-provider",
    "production",
}


class FrontmatterError(ValueError):
    """Raised when a knowledge page has malformed frontmatter."""


def _display_path(path: Path, root: Path) -> str:
    try:
        return str(path.relative_to(root))
    except ValueError:
        return str(path)


def _knowledge_error(path: Path, root: Path, message: str) -> str:
    return f"[KNOWLEDGE] {_display_path(path, root)}: {message}"


def _valid_calendar_date(value: object) -> bool:
    if not isinstance(value, str) or _DATE_RE.fullmatch(value) is None:
        return False
    try:
        date.fromisoformat(value)
    except ValueError:
        return False
    return True


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


def _is_markdown_fence_opener(match: re.Match[str] | None) -> bool:
    if match is None:
        return False
    marker, remainder = match.groups()
    return marker[0] != "`" or "`" not in remainder


def _inline_code_span_block_boundary(line: str) -> bool:
    if not line.strip():
        return True
    if re.match(r"^ {0,3}#{1,6}(?:[ \t]+|$)", line):
        return True
    if re.match(r"^ {0,3}(?:=+|-+)[ \t]*$", line):
        return True
    return _is_markdown_fence_opener(
        re.match(r"^ {0,3}(`{3,}|~{3,})(.*)$", line)
    )


def _matching_inline_code_span_end(
    lines: list[str], start_line: int, start: int, *, allow_multiline: bool
) -> tuple[int, int] | None:
    opener = lines[start_line]
    opener_end = start
    while opener_end < len(opener) and opener[opener_end] == "`":
        opener_end += 1
    opener_length = opener_end - start

    for line_number in range(start_line, len(lines)):
        line = lines[line_number]
        if line_number == start_line:
            cursor = opener_end
        else:
            if not allow_multiline:
                return None
            if _inline_code_span_block_boundary(line):
                return None
            cursor = 0

        while cursor < len(line):
            candidate = line.find("`", cursor)
            if candidate == -1:
                break
            candidate_end = candidate
            while candidate_end < len(line) and line[candidate_end] == "`":
                candidate_end += 1
            if candidate_end - candidate == opener_length:
                return line_number, candidate_end
            cursor = candidate_end
    return None


def _visible_markdown_lines(path: Path) -> list[str]:
    text = path.read_text(encoding="utf-8", errors="ignore")
    frontmatter = _FRONTMATTER_RE.match(text)
    if frontmatter is not None:
        text = text[frontmatter.end() :]

    lines = text.splitlines()
    visible: list[str] = []
    fence_character = ""
    fence_length = 0
    in_html_comment = False
    for line_number, raw_line in enumerate(lines):
        fence = re.match(r"^ {0,3}(`{3,}|~{3,})(.*)$", raw_line)
        if fence_character:
            if (
                fence is not None
                and fence.group(1)[0] == fence_character
                and len(fence.group(1)) >= fence_length
                and not fence.group(2).strip()
            ):
                fence_character = ""
                fence_length = 0
            continue

        # A comment marker in a fence opener's info string is literal too.
        if not in_html_comment and _is_markdown_fence_opener(fence):
            fence_character = fence.group(1)[0]
            fence_length = len(fence.group(1))
            continue

        line_parts: list[str] = []
        cursor = 0
        while cursor < len(raw_line):
            if in_html_comment:
                comment_end = raw_line.find("-->", cursor)
                if comment_end == -1:
                    cursor = len(raw_line)
                    break
                in_html_comment = False
                cursor = comment_end + 3
                continue

            comment_start = raw_line.find("<!--", cursor)
            code_start = raw_line.find("`", cursor)
            if code_start != -1 and (
                comment_start == -1 or code_start < comment_start
            ):
                line_parts.append(raw_line[cursor:code_start])
                span_end = _matching_inline_code_span_end(
                    lines,
                    line_number,
                    code_start,
                    allow_multiline=fence is None,
                )
                if span_end is None:
                    opener_end = code_start + 1
                    while (
                        opener_end < len(raw_line)
                        and raw_line[opener_end] == "`"
                    ):
                        opener_end += 1
                    line_parts.append(raw_line[code_start:opener_end])
                    cursor = opener_end
                    continue

                end_line, code_end = span_end
                if end_line == line_number:
                    line_parts.append(raw_line[code_start:code_end])
                    cursor = code_end
                    continue

                # Non-space masking keeps a closing-line suffix from becoming
                # line-leading Markdown while preserving physical columns.
                line_parts.append(
                    _INLINE_CODE_MASK * (len(raw_line) - code_start)
                )
                for covered_line in range(line_number + 1, end_line):
                    lines[covered_line] = _INLINE_CODE_MASK * len(
                        lines[covered_line]
                    )
                lines[end_line] = (
                    _INLINE_CODE_MASK * code_end + lines[end_line][code_end:]
                )
                cursor = len(raw_line)
                continue
            if comment_start == -1:
                line_parts.append(raw_line[cursor:])
                break
            line_parts.append(raw_line[cursor:comment_start])
            in_html_comment = True
            cursor = comment_start + 4

        line = "".join(line_parts)
        fence = re.match(r"^ {0,3}(`{3,}|~{3,})(.*)$", line)
        if _is_markdown_fence_opener(fence):
            fence_character = fence.group(1)[0]
            fence_length = len(fence.group(1))
            continue
        visible.append(line)
    return visible


def _path_is_protected(path: str, protected_paths: set[str]) -> bool:
    return any(
        path == protected.rstrip("/")
        or (protected.endswith("/") and path.startswith(protected))
        for protected in protected_paths
    )


def _heading_exists(path: Path, heading: str) -> bool:
    for line in _visible_markdown_lines(path):
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

    list_errors, protected = _validate_string_list(
        policy_path, root, policy, "protected_paths", allow_empty=False
    )
    errors.extend(list_errors)
    protected_set = set(protected)
    if protected:
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

    list_errors, legacy = _validate_string_list(
        policy_path, root, policy, "legacy_upstream", allow_empty=True
    )
    errors.extend(list_errors)

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
            if path == layer_dir / "README.md":
                continue
            if layer == "implementation" and "evidence" in path.relative_to(layer_dir).parts:
                continue
            pages.append((layer, path))
    proposals = knowledge / "proposals"
    if proposals.exists():
        for path in sorted(proposals.rglob("*.md")):
            if path != proposals / "README.md":
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
        "implementation": "IMP-",
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


def _validate_external_upstream(path: Path, root: Path, data: dict[str, object]) -> list[str]:
    if "external_upstream" not in data:
        return []
    errors, references = _validate_string_list(
        path, root, data, "external_upstream", allow_empty=True
    )
    if _string_list(data, "external_upstream") is None:
        return errors
    if not references:
        errors.append(
            _knowledge_error(
                path,
                root,
                "external_upstream must be omitted or contain at least one reference",
            )
        )
    errors.extend(
        _knowledge_error(
            path,
            root,
            "invalid external_upstream; expected repo@<40sha>:<ID>: " + reference,
        )
        for reference in references
        if reference.strip() and _EXTERNAL_UPSTREAM.fullmatch(reference) is None
    )
    return errors


def _validate_string_list(
    path: Path,
    root: Path,
    data: dict[str, object],
    key: str,
    *,
    allow_empty: bool,
    duplicate_label: str = "items",
) -> tuple[list[str], list[str]]:
    values = _string_list(data, key)
    if values is None:
        return [_knowledge_error(path, root, f"{key} must be a list")], []
    errors: list[str] = []
    if not allow_empty and not values:
        errors.append(_knowledge_error(path, root, f"{key} must be a non-empty list"))
    blank_positions = [str(index) for index, item in enumerate(values, start=1) if not item.strip()]
    if blank_positions:
        errors.append(
            _knowledge_error(
                path,
                root,
                f"{key} contains blank items at positions: " + ", ".join(blank_positions),
            )
        )
    duplicates = sorted(
        {item for item in values if item.strip() and values.count(item) > 1}
    )
    if duplicates:
        errors.append(
            _knowledge_error(
                path,
                root,
                f"{key} contains duplicate {duplicate_label}: " + ", ".join(duplicates),
            )
        )
    return errors, values


def _validate_requirement_list(
    path: Path,
    root: Path,
    data: dict[str, object],
    key: str,
    *,
    allow_empty: bool,
) -> tuple[list[str], list[str]]:
    errors, values = _validate_string_list(
        path,
        root,
        data,
        key,
        allow_empty=allow_empty,
        duplicate_label="IDs",
    )
    return errors, values


def _validate_implementation_fields(
    path: Path, root: Path, data: dict[str, object]
) -> list[str]:
    retired = data.get("status") == "retired"
    errors, _ = _validate_requirement_list(
        path, root, data, "tracks", allow_empty=retired
    )
    list_errors, code_paths = _validate_string_list(
        path, root, data, "code_paths", allow_empty=retired
    )
    errors.extend(list_errors)
    for code_path in code_paths:
        if not code_path.strip():
            continue
        candidate = Path(code_path)
        if (
            candidate.is_absolute()
            or ".." in candidate.parts
            or not (root / candidate).exists()
        ):
            errors.append(
                _knowledge_error(path, root, f"code path does not exist: {code_path}")
            )
    list_errors, _ = _validate_string_list(
        path, root, data, "evidence", allow_empty=retired
    )
    errors.extend(list_errors)
    for obsolete in ("verified_at", "verified_commit"):
        if obsolete in data:
            errors.append(
                _knowledge_error(path, root, f"{obsolete} belongs in EVD, not IMP")
            )
    return errors


def _commit_is_ancestor(root: Path, commit: str) -> bool:
    try:
        completed = subprocess.run(
            ["git", "-C", str(root), "merge-base", "--is-ancestor", commit, "HEAD"],
            check=False,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    except OSError:
        return False
    return completed.returncode == 0


def _paths_changed_since(root: Path, commit: str, paths: list[str]) -> bool | None:
    try:
        completed = subprocess.run(
            ["git", "-C", str(root), "diff", "--quiet", commit, "--", *paths],
            check=False,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    except OSError:
        return None
    if completed.returncode == 0:
        try:
            untracked = subprocess.run(
                [
                    "git",
                    "-C",
                    str(root),
                    "ls-files",
                    "--others",
                    "--exclude-standard",
                    "--",
                    *paths,
                ],
                check=False,
                capture_output=True,
                text=True,
            )
        except OSError:
            return None
        if untracked.returncode != 0:
            return None
        return bool(untracked.stdout.strip())
    if completed.returncode == 1:
        return True
    return None


def _registered_pages(directory: Path) -> tuple[set[str], list[str], list[str]]:
    readme = directory / "README.md"
    if not readme.is_file():
        return set(), [f"{readme}: missing README.md index"], []
    registration_counts: dict[str, int] = {}
    for match in re.finditer(
        r"\]\(([^)#]+\.md)(?:#[^)]*)?\)",
        "\n".join(_visible_markdown_lines(readme)),
    ):
        target = Path(match.group(1))
        if len(target.parts) == 1:
            registration_counts[target.name] = registration_counts.get(target.name, 0) + 1
    registered = set(registration_counts)
    missing = [name for name in sorted(registered) if not (directory / name).is_file()]
    duplicates = sorted(
        name for name, count in registration_counts.items() if count > 1
    )
    return registered, missing, duplicates


def _markdown_table_cells(line: str) -> list[str] | None:
    if len(line) - len(line.lstrip(" ")) > 3 or re.match(r"^ {0,3}\t", line):
        return None
    value = line.strip()
    if "|" not in value:
        return None
    if value.startswith("|"):
        value = value[1:]
    if value.endswith("|"):
        value = value[:-1]
    return [cell.strip() for cell in re.split(r"(?<!\\)\|", value)]


def _formal_requirement_ids(lines: list[str]) -> list[str]:
    requirements: list[str] = []
    index = 0
    while index < len(lines):
        bullet = _REQUIREMENT_BULLET.match(lines[index])
        if bullet is not None:
            requirements.append(bullet.group("requirement"))

        header = _markdown_table_cells(lines[index])
        separator = (
            _markdown_table_cells(lines[index + 1])
            if index + 1 < len(lines)
            else None
        )
        if (
            header is None
            or separator is None
            or header[0].strip("`").strip().casefold()
            not in _REQUIREMENT_TABLE_HEADERS
            or len(header) != len(separator)
            or not all(_TABLE_SEPARATOR_CELL.fullmatch(cell) for cell in separator)
        ):
            index += 1
            continue

        index += 2
        while index < len(lines):
            row = _markdown_table_cells(lines[index])
            if row is None or len(row) != len(header):
                break
            requirement = _REQUIREMENT_TABLE_CELL.fullmatch(row[0])
            if requirement is not None:
                requirements.append(
                    requirement.group("quoted") or requirement.group("plain")
                )
            index += 1
    return requirements


def _extract_requirement_definitions(
    records: list[tuple[str, Path, dict[str, object]]], root: Path
) -> tuple[dict[str, tuple[str, Path]], list[str]]:
    requirements: dict[str, tuple[str, Path]] = {}
    errors: list[str] = []
    for layer, path, data in records:
        if layer != "spec" or data.get("status") != "approved":
            continue
        spec_id = str(data["id"])
        for requirement in _formal_requirement_ids(_visible_markdown_lines(path)):
            if requirement in requirements:
                first = _display_path(requirements[requirement][1], root)
                errors.append(
                    _knowledge_error(
                        path,
                        root,
                        f"duplicate approved requirement {requirement}; first seen in {first}",
                    )
                )
            else:
                requirements[requirement] = (spec_id, path)
    return requirements, errors


def _parse_tracking_rows(
    path: Path,
) -> tuple[list[tuple[str, str, str, str]], list[str], list[str]]:
    rows: list[tuple[str, str, str, str]] = []
    malformed: list[str] = []
    errors: list[str] = []
    lines = _visible_markdown_lines(path)
    header_indexes = [
        index for index, line in enumerate(lines) if line == _AUTHORITY_TABLE_HEADER
    ]
    if len(header_indexes) != 1:
        errors.append("requires exactly one authoritative Requirement table header")
        return rows, malformed, errors
    header_index = header_indexes[0]
    if (
        header_index + 1 >= len(lines)
        or lines[header_index + 1] != _AUTHORITY_TABLE_SEPARATOR
    ):
        errors.append("authoritative Requirement table requires the exact separator row")
        return rows, malformed, errors
    for line in lines[header_index + 2 :]:
        if not line.startswith("|"):
            break
        match = _TRACKING_ROW.match(line)
        if match is not None:
            rows.append(
                (
                    match.group("requirement"),
                    match.group("design"),
                    match.group("status"),
                    match.group("detail").strip(),
                )
            )
            continue
        first_cell = line.split("|", 2)[1]
        malformed.append(first_cell.strip() or "<empty>")
    return rows, malformed, errors


def _check_current_knowledge_graph(
    root: Path,
    records: list[tuple[str, Path, dict[str, object]]],
    documents: dict[str, tuple[str, Path, dict[str, object]]],
) -> list[str]:
    errors: list[str] = []
    requirements, definition_errors = _extract_requirement_definitions(records, root)
    errors.extend(definition_errors)

    current_designs: dict[str, tuple[Path, dict[str, object], set[str]]] = {}
    for layer, path, data in records:
        if layer != "design":
            continue
        list_errors, tracks = _validate_requirement_list(
            path,
            root,
            data,
            "tracks",
            allow_empty=data.get("status") not in {"active", "blocked"},
        )
        errors.extend(list_errors)
        if data.get("status") not in {"active", "blocked"}:
            continue
        design_id = str(data["id"])
        upstream = set(_string_list(data, "upstream") or [])
        for requirement in tracks:
            owner = requirements.get(requirement)
            if owner is None:
                errors.append(
                    _knowledge_error(path, root, f"tracks unknown active requirement: {requirement}")
                )
            elif owner[0] not in upstream:
                errors.append(
                    _knowledge_error(
                        path,
                        root,
                        f"{requirement} belongs to {owner[0]}, which is not an upstream",
                    )
                )
        current_designs[design_id] = (path, data, set(tracks))

    for requirement in sorted(requirements):
        design_owners = [
            design_id
            for design_id, (_, _, tracks) in current_designs.items()
            if requirement in tracks
        ]
        if not design_owners:
            errors.append(
                _knowledge_error(
                    requirements[requirement][1],
                    root,
                    f"approved requirement has no current DES track: {requirement}",
                )
            )
        elif len(design_owners) > 1:
            errors.append(
                _knowledge_error(
                    requirements[requirement][1],
                    root,
                    "approved requirement has multiple current DES owners: "
                    f"{requirement}: {', '.join(sorted(design_owners))}",
                )
            )

    evidence_records = {
        str(data["id"]): (path, data)
        for layer, path, data in records
        if layer == "evidence"
    }
    implementation_tracks: dict[str, list[Path]] = {}
    design_consumers: dict[str, list[str]] = {design_id: [] for design_id in current_designs}
    for layer, path, data in records:
        if layer != "implementation" or data.get("status") == "retired":
            continue
        implementation_id = str(data["id"])
        tracks = _string_list(data, "tracks") or []
        declared_evidence = set(_string_list(data, "evidence") or [])
        upstream = set(_string_list(data, "upstream") or [])
        for evidence_id in sorted(declared_evidence):
            evidence = evidence_records.get(evidence_id)
            if evidence is None:
                errors.append(
                    _knowledge_error(path, root, f"evidence document does not exist: {evidence_id}")
                )
            elif implementation_id not in set(_string_list(evidence[1], "upstream") or []):
                errors.append(
                    _knowledge_error(
                        path,
                        root,
                        f"evidence does not point back to {implementation_id}: {evidence_id}",
                    )
                )
        for design_id in upstream & set(current_designs):
            design_consumers[design_id].append(implementation_id)
        for requirement in tracks:
            implementation_tracks.setdefault(requirement, []).append(path)
            if requirement not in requirements:
                errors.append(
                    _knowledge_error(path, root, f"tracks unknown active requirement: {requirement}")
                )

        rows, malformed, table_errors = _parse_tracking_rows(path)
        for table_error in table_errors:
            errors.append(_knowledge_error(path, root, table_error))
        for value in malformed:
            errors.append(
                _knowledge_error(
                    path,
                    root,
                    f"tracking rows require one exact requirement ID, design, status, and evidence/gap: {value}",
                )
            )
        row_ids = [row[0] for row in rows]
        for requirement in sorted(set(tracks) - set(row_ids)):
            errors.append(_knowledge_error(path, root, f"missing authoritative row: {requirement}"))
        for requirement in sorted(set(row_ids) - set(tracks)):
            errors.append(_knowledge_error(path, root, f"row is not declared in tracks: {requirement}"))
        for requirement in sorted({item for item in row_ids if row_ids.count(item) > 1}):
            errors.append(_knowledge_error(path, root, f"duplicate authoritative row: {requirement}"))
        if rows:
            row_statuses = {row[2] for row in rows}
            aggregate_status = (
                "diverged"
                if "diverged" in row_statuses
                else "unknown"
                if "unknown" in row_statuses
                else "aligned"
            )
            if data.get("status") != aggregate_status:
                errors.append(
                    _knowledge_error(
                        path,
                        root,
                        "implementation status must equal authoritative row aggregate: "
                        f"expected {aggregate_status}, got {data.get('status')}",
                    )
                )
        for requirement, design_id, status, detail in rows:
            design = current_designs.get(design_id)
            if design_id not in upstream:
                errors.append(
                    _knowledge_error(path, root, f"row design is not an IMP upstream: {design_id}")
                )
            if design is None or requirement not in design[2]:
                errors.append(
                    _knowledge_error(
                        path, root, f"row design does not track {requirement}: {design_id}"
                    )
                )
            if status == "aligned":
                evidence_ids = set(re.findall(r"\bEVD-[a-z0-9]+(?:-[a-z0-9]+)*\b", detail))
                suitable = False
                for evidence_id in evidence_ids & declared_evidence:
                    evidence = evidence_records.get(evidence_id)
                    if evidence is None:
                        continue
                    evidence_data = evidence[1]
                    covers = set(_string_list(evidence_data, "covers") or [])
                    if (
                        evidence_data.get("status") == "active"
                        and evidence_data.get("result") == "passed"
                        and requirement in covers
                        and implementation_id
                        in set(_string_list(evidence_data, "upstream") or [])
                    ):
                        suitable = True
                        break
                if not suitable:
                    errors.append(
                        _knowledge_error(
                            path,
                            root,
                            f"aligned row lacks active passed EVD coverage: {requirement}",
                        )
                    )
            elif not detail.startswith("gap: ") or not detail.removeprefix("gap: ").strip():
                errors.append(
                    _knowledge_error(
                        path, root, f"{status} row requires detail in 'gap: ...' form: {requirement}"
                    )
                )

    for requirement, (_, spec_path) in sorted(requirements.items()):
        owners = implementation_tracks.get(requirement, [])
        if not owners:
            errors.append(
                _knowledge_error(spec_path, root, f"approved requirement has no current IMP row: {requirement}")
            )
        elif len(owners) > 1:
            pages = ", ".join(_display_path(path, root) for path in owners)
            errors.append(
                _knowledge_error(
                    spec_path, root, f"approved requirement has multiple current IMP owners: {requirement}: {pages}"
                )
            )
    for design_id, consumers in design_consumers.items():
        if not consumers:
            errors.append(
                _knowledge_error(
                    current_designs[design_id][0], root, f"current design has no current IMP: {design_id}"
                )
            )

    for evidence_id, (path, data) in evidence_records.items():
        covers = _string_list(data, "covers") or []
        upstream = _string_list(data, "upstream") or []
        upstream_tracks: set[str] = set()
        for implementation_id in upstream:
            target = documents.get(implementation_id)
            if target is not None:
                upstream_tracks.update(_string_list(target[2], "tracks") or [])
                if evidence_id not in set(_string_list(target[2], "evidence") or []):
                    errors.append(
                        _knowledge_error(
                            path,
                            root,
                            f"upstream IMP does not point back to {evidence_id}: {implementation_id}",
                        )
                    )
        for requirement in covers:
            if requirement not in requirements:
                errors.append(
                    _knowledge_error(path, root, f"covers unknown active requirement: {requirement}")
                )
            elif requirement not in upstream_tracks:
                errors.append(
                    _knowledge_error(
                        path,
                        root,
                        f"covers requirement outside upstream IMP tracks: {requirement}",
                    )
                )
        if data.get("status") == "active":
            for implementation_id in upstream:
                target = documents.get(implementation_id)
                if target is not None and target[2].get("status") == "retired":
                    errors.append(
                        _knowledge_error(
                            path,
                            root,
                            f"active evidence cannot use retired IMP upstream: {implementation_id}",
                        )
                    )
        observed_commit = data.get("observed_commit")
        if not isinstance(observed_commit, str) or not _FULL_COMMIT_RE.fullmatch(observed_commit):
            errors.append(
                _knowledge_error(path, root, "observed_commit must be a full 40-character SHA")
            )
        elif not _commit_is_ancestor(root, observed_commit):
            errors.append(
                _knowledge_error(path, root, f"observed_commit is not an ancestor of HEAD: {observed_commit}")
            )
        elif data.get("status") == "active" and data.get("result") == "passed":
            for implementation_id in upstream:
                target = documents.get(implementation_id)
                if target is None or target[0] != "implementation":
                    continue
                code_paths = [
                    code_path
                    for code_path in (_string_list(target[2], "code_paths") or [])
                    if code_path.strip()
                ]
                if not code_paths:
                    continue
                changed = _paths_changed_since(root, observed_commit, code_paths)
                if changed is True:
                    errors.append(
                        _knowledge_error(
                            path,
                            root,
                            "active passed evidence is stale; upstream code_paths changed "
                            f"after observed_commit: {implementation_id}",
                        )
                    )
                elif changed is None:
                    errors.append(
                        _knowledge_error(
                            path,
                            root,
                            "cannot compare upstream code_paths with observed_commit: "
                            f"{implementation_id}",
                        )
                    )
        if "artifacts" in data:
            list_errors, artifacts = _validate_string_list(
                path, root, data, "artifacts", allow_empty=True
            )
            errors.extend(list_errors)
        else:
            artifacts = []
        if artifacts:
            for artifact in artifacts:
                candidate = Path(artifact)
                stable_uri = artifact.startswith(("https://", "http://"))
                durable_file = (
                    not candidate.is_absolute()
                    and ".." not in candidate.parts
                    and (root / candidate).is_file()
                )
                if not stable_uri and not durable_file and not artifact.startswith("/tmp/"):
                    errors.append(
                        _knowledge_error(
                            path,
                            root,
                            f"artifact must be a stable URI or repository file: {artifact}",
                        )
                    )
            if (
                data.get("status") == "active"
                and data.get("result") == "passed"
                and all(item == "/tmp" or item.startswith("/tmp/") for item in artifacts)
            ):
                errors.append(
                    _knowledge_error(path, root, "active passed evidence cannot rely only on /tmp artifacts")
                )
        if evidence_id not in documents:
            errors.append(_knowledge_error(path, root, f"unregistered evidence ID: {evidence_id}"))
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
        required = required_common | (
            {"target_layer"}
            if expected_layer == "proposal"
            else {"upstream", "updated_at"}
        )
        if expected_layer == "evidence":
            required |= {"covers", "scope", "commands", "observed_commit", "result"}
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
        layer_directory = (
            root / "docs" / "knowledge" / "proposals"
            if expected_layer == "proposal"
            else root / "docs" / "knowledge" / LAYER_DIRS[expected_layer]
        )
        if path.parent != layer_directory:
            errors.append(
                _knowledge_error(
                    path,
                    root,
                    f"{expected_layer} pages must be direct children of "
                    f"{_display_path(layer_directory, root)}",
                )
            )
        if path.name != f"{document_id}.md":
            errors.append(
                _knowledge_error(
                    path,
                    root,
                    f"filename must match id: {document_id}.md",
                )
            )
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
        if expected_layer != "proposal":
            list_errors, _ = _validate_string_list(
                path, root, data, "upstream", allow_empty=True
            )
            errors.extend(list_errors)
            updated_at = data.get("updated_at")
            if not _valid_calendar_date(updated_at):
                errors.append(
                    _knowledge_error(
                        path, root, "updated_at must be a valid YYYY-MM-DD date"
                    )
                )
            if "role" in data and data.get("role") != "baseline":
                errors.append(_knowledge_error(path, root, "role, when present, must be baseline"))
            errors.extend(_validate_external_upstream(path, root, data))
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
            if "legacy_upstream" in data:
                list_errors, legacy = _validate_string_list(
                    path, root, data, "legacy_upstream", allow_empty=True
                )
                errors.extend(list_errors)
            else:
                legacy = []
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
            if data.get("status") in {"active", "blocked"}:
                for _, _, target_data in targets:
                    if target_data.get("status") != "approved":
                        errors.append(
                            _knowledge_error(
                                path, root, "current design requires approved spec"
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
                    if target_data.get("status") not in {"active", "blocked"}:
                        errors.append(
                            _knowledge_error(
                                path,
                                root,
                                "aligned/diverged implementation requires current design",
                            )
                        )
            errors.extend(_validate_implementation_fields(path, root, data))
            continue
        if layer == "evidence":
            if not upstream:
                errors.append(_knowledge_error(path, root, "evidence requires an IMP upstream"))
            ref_errors, _ = _validate_typed_upstream(
                path, root, upstream, "implementation", documents
            )
            errors.extend(ref_errors)
            list_errors, _ = _validate_requirement_list(
                path, root, data, "covers", allow_empty=False
            )
            errors.extend(list_errors)
            list_errors, _ = _validate_string_list(
                path, root, data, "commands", allow_empty=False
            )
            errors.extend(list_errors)
            list_errors, scopes = _validate_string_list(
                path, root, data, "scope", allow_empty=False
            )
            errors.extend(list_errors)
            if invalid_scopes := sorted(
                scope for scope in set(scopes) if scope.strip() and scope not in EVIDENCE_SCOPES
            ):
                errors.append(
                    _knowledge_error(
                        path, root, "invalid evidence scope: " + ", ".join(invalid_scopes)
                    )
                )
            if data.get("result") not in {"passed", "partial", "failed", "blocked"}:
                errors.append(
                    _knowledge_error(
                        path, root, "result must be passed, partial, failed, or blocked"
                    )
                )

    for indexed_layer in ("intent", "spec", "design", "implementation", "evidence"):
        layer_dir = root / "docs" / "knowledge" / LAYER_DIRS[indexed_layer]
        registered, missing, duplicates = _registered_pages(layer_dir)
        for name in missing:
            errors.append(
                _knowledge_error(
                    layer_dir / "README.md",
                    root,
                    f"{indexed_layer} README links missing file: {name}",
                )
            )
        for name in duplicates:
            errors.append(
                _knowledge_error(
                    layer_dir / "README.md",
                    root,
                    f"{indexed_layer} file is registered multiple times: {name}",
                )
            )
        for layer, path, _ in records:
            if layer == indexed_layer and path.name not in registered:
                errors.append(
                    _knowledge_error(
                        path,
                        root,
                        f"{indexed_layer} file is not registered in {indexed_layer}/README.md",
                    )
                )
    errors.extend(_check_current_knowledge_graph(root, records, documents))
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
        errors.append(
            "stale ungenerated proto allowlist: " + ", ".join(sorted(stale))
        )
    return errors



def main():
    errors = []
    errors.extend(check_doc_policy())
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
