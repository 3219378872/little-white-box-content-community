#!/usr/bin/env python3
"""Repository knowledge: one authority matrix, scoped evidence, Git snapshots."""

from __future__ import annotations

import argparse
from collections import Counter
from datetime import date
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys

try:
    from markdown_it import MarkdownIt
    import yaml
except ImportError as exc:
    raise SystemExit(
        "Knowledge dependencies missing; run make knowledge-setup"
    ) from exc

ROOT = Path(
    os.environ.get("KNOWLEDGE_ROOT", Path(__file__).resolve().parents[1])
).resolve()
REPOSITORY = "little-white-box-content-community"
REQUIREMENT = re.compile(r"[A-Z][A-Z0-9]*-(?:A\d{2}|\d{3}(?:-\d{2})?)\Z")
REPOSITORIES = {
    "little-white-box",
    "little-white-box-content-community",
    "little-white-box-front",
}
LAYERS = dict(
    intent="INT", spec="SPEC", design="DES", implementation="IMP", evidence="EVD"
)
STATUSES = {
    "intent": {"draft", "approved", "retired"},
    "spec": {"draft", "approved", "retired"},
    "design": {"draft", "active", "blocked", "superseded"},
    "implementation": {"active", "retired"},
    "evidence": {"active", "superseded"},
}
SCOPES = {
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
RESULTS = {"passed", "partial", "failed", "blocked"}
FORMAL_ID = re.compile(r"(?:INT|SPEC|DES|IMP|EVD)-[a-z0-9]+(?:-[a-z0-9]+)*\Z")
SHA = re.compile(r"[0-9a-f]{40}\Z")
MD = MarkdownIt("commonmark").enable("table")
INDEX_START = "<!-- knowledge-index:start -->"
INDEX_END = "<!-- knowledge-index:end -->"
AUTHORITY_HEADERS = {
    ("requirement", "design", "status", "evidence/gap"),
    ("requirement", "design", "state", "evidence or gap"),
}


class KnowledgeError(ValueError):
    pass


class UniqueLoader(yaml.SafeLoader):
    pass


def _mapping(loader, node, deep=False):
    result = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node, deep=deep)
        if not isinstance(key, str) or key in result:
            raise KnowledgeError(f"duplicate or non-text YAML key: {key!r}")
        result[key] = loader.construct_object(value_node, deep=deep)
    return result


UniqueLoader.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, _mapping)


def frontmatter(text: str) -> tuple[dict, str]:
    lines = text.splitlines(keepends=True)
    if not lines or lines[0].strip() != "---":
        raise KnowledgeError("missing YAML frontmatter")
    end = next((i for i in range(1, len(lines)) if lines[i].strip() == "---"), None)
    if end is None:
        raise KnowledgeError("unclosed YAML frontmatter")
    try:
        meta = yaml.load("".join(lines[1:end]), Loader=UniqueLoader)
    except yaml.YAMLError as exc:
        raise KnowledgeError(f"invalid YAML: {exc}") from exc
    if not isinstance(meta, dict):
        raise KnowledgeError("frontmatter must be a mapping")
    return meta, "".join(lines[end + 1 :])


def texts(value, name: str, *, empty=False) -> list[str]:
    if empty and value is None:
        return []
    if not isinstance(value, list) or (not empty and not value):
        raise KnowledgeError(
            f"{name} must be a {'non-empty ' if not empty else ''}list"
        )
    if any(not isinstance(item, str) or not item.strip() for item in value):
        raise KnowledgeError(f"{name} items must be non-blank text")
    if len(set(value)) != len(value):
        raise KnowledgeError(f"{name} has duplicate items")
    return value


def inline_text(token) -> str:
    return "".join(
        (child.attrGet("href") or "")
        if child.type == "link_open"
        else " "
        if child.type in {"softbreak", "hardbreak"}
        else child.content
        if child.type in {"text", "code_inline", "image"}
        else ""
        for child in (token.children or [])
    ).strip()


def tables(body: str) -> list[list[list[str]]]:
    result, table, row = [], None, None
    for token in MD.parse(body):
        if token.type == "table_open":
            table = []
        elif token.type == "tr_open" and table is not None:
            row = []
        elif token.type == "inline" and row is not None:
            row.append(inline_text(token))
        elif token.type == "tr_close" and table is not None:
            table.append(row)
            row = None
        elif token.type == "table_close":
            result.append(table)
            table = None
    return result


def definitions(body: str) -> list[tuple[str, str]]:
    result, stack = [], []
    for token in MD.parse(body):
        if token.nesting == 1:
            stack.append(token.type.removesuffix("_open"))
        elif token.nesting == -1:
            stack.pop()
        elif token.type == "inline" and stack[-3:] == [
            "bullet_list",
            "list_item",
            "paragraph",
        ]:
            children = token.children or []
            if (
                children
                and children[0].type == "code_inline"
                and REQUIREMENT.fullmatch(children[0].content)
            ):
                rest = "".join(
                    "\n"
                    if c.type in {"softbreak", "hardbreak"}
                    else c.content
                    if c.type in {"text", "code_inline", "image"}
                    else ""
                    for c in children[1:]
                )
                if re.match(r"[：:]\s*\S", rest.splitlines()[0]):
                    result.append((children[0].content, inline_text(token)))
    for table in tables(body):
        if len(table[0]) >= 2 and table[0][0].lower() in {"id", "requirement", "条款"}:
            for row in table[1:]:
                if REQUIREMENT.fullmatch(row[0]) and any(
                    cell.strip() for cell in row[1:]
                ):
                    result.append((row[0], " ".join(row)))
    return [(key, " ".join(value.split())) for key, value in result]


def authority(body: str) -> list[tuple[str, str, str, str]]:
    found = [
        t for t in tables(body) if tuple(c.lower() for c in t[0]) in AUTHORITY_HEADERS
    ]
    if len(found) != 1:
        raise KnowledgeError("requires exactly one authoritative requirement table")
    rows = []
    for row in found[0][1:]:
        if (
            len(row) != 4
            or not REQUIREMENT.fullmatch(row[0])
            or not FORMAL_ID.fullmatch(row[1])
            or not row[1].startswith("DES-")
        ):
            raise KnowledgeError("invalid authority row: " + " | ".join(row))
        if row[2] not in {"aligned", "unknown", "diverged"}:
            raise KnowledgeError(f"invalid requirement state: {row[2]}")
        if row[2] != "aligned" and not re.fullmatch(r"gap:\s+\S.*", row[3], re.DOTALL):
            raise KnowledgeError(f"{row[0]} requires a non-empty gap: explanation")
        rows.append(tuple(row))
    if not rows or len({row[0] for row in rows}) != len(rows):
        raise KnowledgeError("authority rows must be non-empty and unique")
    return rows


def aggregate(rows) -> str:
    states = {row[2] for row in rows}
    return (
        "diverged"
        if "diverged" in states
        else "unknown"
        if "unknown" in states
        else "aligned"
    )


def git(root: Path, *args: str, optional=False) -> bytes:
    env = dict(os.environ, GIT_LITERAL_PATHSPECS="1")
    process = subprocess.run(
        ["git", "-C", str(root), *args], capture_output=True, env=env
    )
    if process.returncode and not optional:
        raise KnowledgeError(
            process.stderr.decode(errors="replace").strip() or f"git {args[0]} failed"
        )
    return process.stdout if not process.returncode else b""


def safe_path(raw: str) -> str:
    path = PurePosixPath(raw)
    if (
        not raw.strip()
        or path.is_absolute()
        or ".." in path.parts
        or path.as_posix() != raw
        or raw == "."
    ):
        raise KnowledgeError(f"unsafe repository path: {raw!r}")
    return raw


def external(meta: dict, source: str) -> list[dict]:
    if "external_upstream" not in meta:
        return []
    result = []
    for ref in texts(meta["external_upstream"], "external_upstream"):
        match = re.fullmatch(r"([^@]+)@([0-9a-f]{40}):(.+)", ref)
        if not match or match[1] not in REPOSITORIES:
            raise KnowledgeError(f"invalid external_upstream: {ref}")
        target = match[3]
        grammar = (
            r"(?:FX|FQ)-\d{3}"
            if match[1] == "little-white-box-front"
            else r"[A-Z][A-Z0-9]*-(?:A\d{2}|\d{3}(?:-\d{2})?)"
        )
        if not FORMAL_ID.fullmatch(target) and not re.fullmatch(grammar, target):
            raise KnowledgeError(f"invalid external target: {target}")
        result.append(
            dict(
                source_id=source,
                repository=match[1],
                revision=match[2],
                target_id=target,
            )
        )
    return result


class Snapshot:
    def __init__(self, root: Path, ref: str | None = None):
        self.root = root.resolve()
        self.revision = (
            git(root, "rev-parse", "--verify", f"{ref}^{{commit}}").decode().strip()
            if ref
            else None
        )
        self.documents = {}
        self.requirements = {}
        self.references = []
        if self.revision:
            paths = (
                git(
                    root,
                    "ls-tree",
                    "-r",
                    "--name-only",
                    "-z",
                    self.revision,
                    "--",
                    "docs/knowledge",
                )
                .decode()
                .split("\0")
            )
        else:
            paths = [
                p.relative_to(root).as_posix()
                for layer in LAYERS
                for p in (root / "docs/knowledge" / layer).glob("*.md")
            ]
            for layer in LAYERS:
                base = root / "docs/knowledge" / layer
                for candidate in base.rglob("*.md"):
                    relative = candidate.relative_to(base)
                    if len(relative.parts) > 1 and not (
                        layer == "implementation" and relative.parts[0] == "evidence"
                    ):
                        raise KnowledgeError(
                            f"formal pages must be direct layer children: {candidate}"
                        )
        for raw in sorted(paths):
            path = PurePosixPath(raw)
            if (
                len(path.parts) > 4
                and path.parts[:2] == ("docs", "knowledge")
                and path.parts[2] in LAYERS
                and path.suffix == ".md"
                and path.parts[2:4] != ("implementation", "evidence")
            ):
                raise KnowledgeError(
                    f"formal pages must be direct layer children: {raw}"
                )
            if (
                len(path.parts) != 4
                or path.parts[:2] != ("docs", "knowledge")
                or path.parts[2] not in LAYERS
                or path.suffix != ".md"
                or path.name == "README.md"
            ):
                continue
            meta, body = frontmatter(self.read(raw))
            identity = meta.get("id", "")
            layer = path.parts[2]
            if (
                not isinstance(identity, str)
                or not FORMAL_ID.fullmatch(identity)
                or not identity.startswith(LAYERS[layer] + "-")
                or path.stem != identity
                or meta.get("layer") != layer
            ):
                raise KnowledgeError(f"{raw}: filename, id and layer must agree")
            if identity in self.documents:
                raise KnowledgeError(f"duplicate formal id: {identity}")
            statuses = STATUSES[layer] | (
                {"aligned", "unknown", "diverged"}
                if layer == "implementation"
                else set()
            )
            if (
                not isinstance(meta.get("status"), str)
                or meta["status"] not in statuses
            ):
                raise KnowledgeError(f"{raw}: invalid lifecycle")
            self.documents[identity] = dict(
                id=identity, layer=layer, path=raw, meta=meta, body=body
            )
            self.references.extend(external(meta, identity))
            if layer == "spec" and meta.get("status") == "approved":
                for requirement, definition in definitions(body):
                    if requirement in self.requirements:
                        raise KnowledgeError(
                            f"duplicate approved requirement: {requirement}"
                        )
                    self.requirements[requirement] = dict(
                        id=requirement,
                        spec_id=identity,
                        path=raw,
                        definition=definition,
                        text_sha256=hashlib.sha256(definition.encode()).hexdigest(),
                    )

    def read(self, path: str) -> str:
        if self.revision:
            return git(self.root, "show", f"{self.revision}:{path}").decode()
        local = (self.root / safe_path(path)).resolve()
        if not local.is_relative_to(self.root):
            raise KnowledgeError(f"document escapes repository: {path}")
        return local.read_text(encoding="utf-8")

    def export(self) -> dict:
        return dict(
            schema_version=1,
            repository=REPOSITORY,
            revision=self.revision
            or git(self.root, "rev-parse", "HEAD").decode().strip(),
            documents=[
                dict(
                    id=d["id"],
                    layer=d["layer"],
                    path=d["path"],
                    status=d["meta"].get("status"),
                )
                for d in self.documents.values()
            ],
            requirements=list(self.requirements.values()),
            external_upstream=self.references,
        )


class Knowledge:
    def __init__(self, root: Path = ROOT):
        self.root = root.resolve()
        self.snapshot = Snapshot(self.root)
        self.rows = {}
        self.owners = {}
        self.snapshots = {}
        self.freshness = {}

    def historical(self, commit: str) -> Snapshot:
        if commit not in self.snapshots:
            self.snapshots[commit] = Snapshot(self.root, commit)
        return self.snapshots[commit]

    def validate(self, *, indexes=True, evidence=True) -> None:
        self.rows, self.owners, self.freshness = {}, {}, {}
        docs, requirements = self.snapshot.documents, self.snapshot.requirements
        for doc in docs.values():
            meta, layer = doc["meta"], doc["layer"]
            label = doc["id"]
            if meta.get("status") not in STATUSES[layer]:
                raise KnowledgeError(
                    f"{label}: invalid {layer} lifecycle {meta.get('status')}"
                )
            if meta.get("owner") != (
                "human" if layer in {"intent", "spec"} else "agent"
            ):
                raise KnowledgeError(f"{label}: invalid semantic owner")
            if not isinstance(meta.get("title"), str) or not meta["title"].strip():
                raise KnowledgeError(f"{label}: title must be non-blank text")
            updated = str(meta.get("updated_at", ""))
            if not re.fullmatch(r"\d{4}-\d{2}-\d{2}", updated):
                raise KnowledgeError(
                    f"{label}: updated_at must be an ISO calendar date"
                )
            try:
                date.fromisoformat(updated)
            except ValueError as exc:
                raise KnowledgeError(f"{label}: invalid calendar date") from exc
            if "role" in meta and meta["role"] != "baseline":
                raise KnowledgeError(f"{label}: unsupported role")
            status = meta["status"]
            if layer == "intent" and texts(
                meta.get("upstream"), "upstream", empty=True
            ):
                raise KnowledgeError(f"{label}: intent upstream must be empty")
            if layer == "spec":
                for upstream in texts(meta.get("upstream"), "spec upstream"):
                    target = docs.get(upstream)
                    if (
                        not target
                        or target["layer"] != "intent"
                        or (
                            status == "approved"
                            and target["meta"]["status"] != "approved"
                        )
                    ):
                        raise KnowledgeError(
                            f"{label}: spec requires an approved INT upstream"
                        )
            if layer == "design" and status in {"active", "blocked"}:
                if "upstream" in meta:
                    raise KnowledgeError(
                        f"{label}: design upstream is derived from tracks"
                    )
                for requirement in texts(meta.get("tracks"), "tracks"):
                    if requirement not in requirements:
                        raise KnowledgeError(
                            f"{label}: tracks unknown approved requirement {requirement}"
                        )
            if layer == "implementation" and status == "active":
                if {
                    "tracks",
                    "upstream",
                    "evidence",
                    "verified_commit",
                    "verified_at",
                    "observed_commit",
                } & meta.keys():
                    raise KnowledgeError(
                        f"{label}: derived IMP fields must not be stored"
                    )
                for path in texts(meta.get("code_paths"), "code_paths"):
                    local = (self.root / safe_path(path)).resolve()
                    if not local.is_relative_to(self.root) or not local.exists():
                        raise KnowledgeError(
                            f"{label}: code path missing or outside repository: {path}"
                        )
                self.rows[label] = authority(doc["body"])
                for requirement, design, state, detail in self.rows[label]:
                    if requirement not in requirements or requirement in self.owners:
                        raise KnowledgeError(
                            f"{label}: unknown requirement or duplicate IMP owner: {requirement}"
                        )
                    target = docs.get(design)
                    if (
                        not target
                        or target["layer"] != "design"
                        or target["meta"]["status"] not in {"active", "blocked"}
                        or requirement not in target["meta"].get("tracks", [])
                    ):
                        raise KnowledgeError(
                            f"{label}: row design does not track {requirement}"
                        )
                    self.owners[requirement] = label
            if layer == "evidence":
                if (
                    meta.get("result") not in RESULTS
                    or not set(texts(meta.get("scope"), "scope")) <= SCOPES
                ):
                    raise KnowledgeError(f"{label}: invalid evidence result or scope")
                texts(meta.get("commands"), "commands")
                commit = meta.get("observed_commit", "")
                if not isinstance(commit, str) or not SHA.fullmatch(commit):
                    raise KnowledgeError(f"{label}: observed_commit must be a full SHA")
                if not git(
                    self.root,
                    "rev-parse",
                    "--verify",
                    f"{commit}^{{commit}}",
                    optional=True,
                ):
                    raise KnowledgeError(f"{label}: observed_commit is unavailable")
                process = subprocess.run(
                    [
                        "git",
                        "-C",
                        str(self.root),
                        "merge-base",
                        "--is-ancestor",
                        commit,
                        "HEAD",
                    ],
                    capture_output=True,
                )
                if process.returncode:
                    raise KnowledgeError(
                        f"{label}: observed_commit is not an ancestor of HEAD"
                    )
                if status == "active":
                    if {"upstream", "covers"} & meta.keys():
                        raise KnowledgeError(
                            f"{label}: EVD upstream/covers are derived from coverage"
                        )
                    groups = meta.get("coverage")
                    if not isinstance(groups, list) or not groups:
                        raise KnowledgeError(
                            f"{label}: coverage must be a non-empty list"
                        )
                    seen = set()
                    for group in groups:
                        if not isinstance(group, dict) or set(group) != {
                            "requirements",
                            "paths",
                        }:
                            raise KnowledgeError(
                                f"{label}: coverage group requires requirements and paths"
                            )
                        for requirement in texts(
                            group["requirements"], "coverage requirements"
                        ):
                            if requirement not in requirements or requirement in seen:
                                raise KnowledgeError(
                                    f"{label}: unknown or duplicate covered requirement: {requirement}"
                                )
                            seen.add(requirement)
                        for path in texts(
                            group["paths"],
                            "coverage paths",
                            empty=meta["result"] != "passed",
                        ):
                            safe_path(path)
                            exists = subprocess.run(
                                [
                                    "git",
                                    "-C",
                                    str(self.root),
                                    "cat-file",
                                    "-e",
                                    f"{commit}:{path}",
                                ],
                                capture_output=True,
                            )
                            if exists.returncode:
                                raise KnowledgeError(
                                    f"{label}: input did not exist at observation: {path}"
                                )
                artifacts = texts(meta.get("artifacts"), "artifacts", empty=True)
                for path in artifacts:
                    if (
                        not path.startswith(("https://", "http://", "/tmp/"))
                        and not (self.root / safe_path(path)).is_file()
                    ):
                        raise KnowledgeError(
                            f"{label}: artifact is not durable: {path}"
                        )
                if (
                    status == "active"
                    and meta["result"] == "passed"
                    and artifacts
                    and all(p.startswith("/tmp/") for p in artifacts)
                ):
                    raise KnowledgeError(
                        f"{label}: passed evidence cannot use only /tmp artifacts"
                    )
        for requirement in requirements:
            if requirement not in self.owners:
                raise KnowledgeError(
                    f"approved requirement has no current IMP owner: {requirement}"
                )
        if evidence:
            for rows in self.rows.values():
                for requirement, design, state, detail in rows:
                    if state != "aligned":
                        continue
                    candidates = re.findall(r"EVD-[a-z0-9]+(?:-[a-z0-9]+)*", detail)
                    failures = [
                        self.support(requirement, candidate) for candidate in candidates
                    ]
                    if not failures or all(failures):
                        raise KnowledgeError(
                            f"{requirement}: aligned requires current passed coverage; {'; '.join(failures) or 'no EVD reference'}"
                        )
        if indexes:
            self.indexes(write=False)
        self.links()

    def support(self, requirement: str, evidence_id: str) -> str:
        evidence = self.snapshot.documents.get(evidence_id)
        if not evidence or evidence["layer"] != "evidence":
            return f"missing EVD {evidence_id}"
        meta = evidence["meta"]
        if meta["status"] != "active" or meta["result"] != "passed":
            return f"{evidence_id} is not active/passed"
        for index, group in enumerate(meta.get("coverage", [])):
            if requirement not in group["requirements"]:
                continue
            key = (evidence_id, index)
            if key not in self.freshness:
                self.freshness[key] = self.group_changes(meta, group)
            return self.freshness[key]
        return f"{evidence_id} does not cover {requirement}"

    def group_changes(self, meta: dict, group: dict) -> str:
        commit, paths = meta["observed_commit"], group["paths"]
        previous = self.historical(commit)
        for requirement in group["requirements"]:
            before = previous.requirements.get(requirement)
            now = self.snapshot.requirements.get(requirement)
            if (
                not before
                or not now
                or before["text_sha256"] != now["text_sha256"]
                or before["spec_id"] != now["spec_id"]
            ):
                return f"stale requirement definition: {requirement}"
            staged = git(self.root, "show", f":{now['path']}", optional=True)
            if not staged:
                return f"staged requirement unavailable: {requirement}"
            staged_meta, staged_body = frontmatter(staged.decode())
            staged_definitions = dict(definitions(staged_body))
            if (
                staged_meta.get("status") != "approved"
                or staged_meta.get("id") != now["spec_id"]
                or staged_definitions.get(requirement) != now["definition"]
            ):
                return f"stale staged requirement definition: {requirement}"
            implementation = self.snapshot.documents[self.owners[requirement]]
            needed = list(implementation["meta"]["code_paths"])
            historical_inputs = False
            for doc in previous.documents.values():
                if (
                    doc["layer"] != "implementation"
                    or doc["meta"].get("status") == "retired"
                ):
                    continue
                try:
                    tracked = [r[0] for r in authority(doc["body"])]
                except KnowledgeError:
                    tracked = doc["meta"].get("tracks", [])
                if requirement in tracked:
                    needed.extend(doc["meta"].get("code_paths", []))
                    historical_inputs |= bool(doc["meta"].get("code_paths"))
            if not historical_inputs:
                return f"historical implementation inputs unknown: {requirement}"
            if any(
                not any(PurePosixPath(path).is_relative_to(prefix) for prefix in paths)
                for path in needed
            ):
                return f"coverage paths narrowed or new implementation input: {requirement}"
        changed = git(self.root, "diff", "--name-only", "-z", commit, "--", *paths)
        changed += git(
            self.root, "diff", "--cached", "--name-only", "-z", commit, "--", *paths
        )
        changed += git(
            self.root, "ls-files", "--others", "--exclude-standard", "-z", "--", *paths
        )
        return (
            f"stale input: {changed.split(bytes([0]))[0].decode()}" if changed else ""
        )

    def indexes(self, *, write: bool) -> None:
        docs_by_id = self.snapshot.documents
        owners = {
            row[0]: identity for identity, rows in self.rows.items() for row in rows
        }
        evidence_owners = {
            doc["id"]: {
                owners[r]
                for group in doc["meta"].get("coverage", [])
                for r in group["requirements"]
                if r in owners
            }
            for doc in docs_by_id.values()
            if doc["layer"] == "evidence"
        }
        for layer in LAYERS:
            path = self.root / "docs/knowledge" / layer / "README.md"
            if not path.is_file():
                raise KnowledgeError(f"missing layer index: {path}")
            original = path.read_text(encoding="utf-8")
            generated = [
                INDEX_START,
                "",
                "| Page | State | Related |",
                "| --- | --- | --- |",
            ]
            docs = [d for d in self.snapshot.documents.values() if d["layer"] == layer]
            current = [
                d
                for d in docs
                if d["meta"].get("status") not in {"retired", "superseded"}
            ]
            history = [d for d in docs if d not in current]
            for doc in current:
                state = doc["meta"]["status"]
                if doc["id"] in self.rows:
                    state += " / " + aggregate(self.rows[doc["id"]])
                if layer == "evidence":
                    state += " / " + doc["meta"]["result"]
                related = set()
                if layer == "spec":
                    related.update(doc["meta"].get("upstream", []))
                elif layer == "design":
                    related.update(
                        self.snapshot.requirements[r]["spec_id"]
                        for r in doc["meta"].get("tracks", [])
                        if r in self.snapshot.requirements
                    )
                elif layer == "implementation":
                    related.update(row[1] for row in self.rows.get(doc["id"], []))
                    related.update(
                        identity
                        for identity, consumers in evidence_owners.items()
                        if doc["id"] in consumers
                    )
                elif layer == "evidence":
                    related.update(evidence_owners[doc["id"]])
                links = (
                    ", ".join(
                        f"[{identity}](../{docs_by_id[identity]['layer']}/{identity}.md)"
                        for identity in sorted(related)
                        if identity in docs_by_id
                    )
                    or "-"
                )
                generated.append(
                    f"| [{doc['id']}]({doc['id']}.md) | {state} | {links} |"
                )
            if history:
                generated += ["", "### History", ""]
                generated += [f"- [{d['id']}]({d['id']}.md)" for d in history]
            generated += ["", INDEX_END]
            block = "\n".join(generated)
            if original.count(INDEX_START) != 1 or original.count(INDEX_END) != 1:
                raise KnowledgeError(f"{path}: requires one generated index block")
            start, end = (
                original.index(INDEX_START),
                original.index(INDEX_END) + len(INDEX_END),
            )
            expected = original[:start] + block + original[end:]
            if write:
                path.write_text(expected, encoding="utf-8")
            elif expected != original:
                raise KnowledgeError(
                    f"{path}: generated index drift; run make knowledge-index"
                )

    def links(self) -> None:
        base = self.root / "docs/knowledge"
        for path in base.rglob("*.md"):
            relative = path.relative_to(base)
            if relative.parts[0] == "archive" or relative.parts[:2] == (
                "implementation",
                "evidence",
            ):
                continue
            if not path.resolve().is_relative_to(self.root):
                raise KnowledgeError(f"document escapes repository: {path}")
            text = path.read_text(encoding="utf-8")
            body = frontmatter(text)[1] if text.startswith("---\n") else text
            for token in MD.parse(body):
                for child in token.children or []:
                    if child.type not in {"link_open", "image"}:
                        continue
                    link = (
                        child.attrGet("href" if child.type == "link_open" else "src")
                        or ""
                    )
                    if not link or link.startswith(
                        ("#", "https://", "http://", "mailto:", "/")
                    ):
                        continue
                    from urllib.parse import unquote

                    target = unquote(link.split("#", 1)[0])
                    if (
                        not (path.parent / target).exists()
                        and not (self.root / target).exists()
                    ):
                        raise KnowledgeError(f"{path}: broken local link: {link}")


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["check", "index", "export", "baseline"])
    parser.add_argument("--root", type=Path, default=ROOT)
    parser.add_argument("--ref", default="HEAD")
    args = parser.parse_args(argv)
    try:
        if args.command in {"export", "baseline"}:
            snapshot = Snapshot(args.root.resolve(), args.ref)
            result = snapshot.export()
            if args.command == "baseline":
                result["implementation_rows"] = {
                    d["id"]: authority(d["body"])
                    for d in snapshot.documents.values()
                    if d["layer"] == "implementation"
                    and d["meta"].get("status") != "retired"
                }
            print(json.dumps(result, ensure_ascii=False, sort_keys=True, indent=2))
        else:
            knowledge = Knowledge(args.root.resolve())
            if args.command == "index":
                knowledge.rows = {
                    d["id"]: authority(d["body"])
                    for d in knowledge.snapshot.documents.values()
                    if d["layer"] == "implementation"
                    and d["meta"]["status"] == "active"
                }
                knowledge.indexes(write=True)
            else:
                knowledge.validate()
                counts = Counter(
                    row[2] for rows in knowledge.rows.values() for row in rows
                )
                print(
                    f"knowledge-check: OK ({len(knowledge.snapshot.documents)} documents, {len(knowledge.owners)} requirements; {dict(counts)})"
                )
        return 0
    except (KnowledgeError, OSError) as exc:
        print(f"knowledge-check: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
