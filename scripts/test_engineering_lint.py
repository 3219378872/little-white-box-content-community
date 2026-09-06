from __future__ import annotations

import importlib.util
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("engineering-lint.py")
SPEC = importlib.util.spec_from_file_location("engineering_lint", SCRIPT)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"cannot load {SCRIPT}")
engineering_lint = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(engineering_lint)


class KnowledgeLayerLintTest(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        for rel in engineering_lint.REQUIRED_KNOWLEDGE_FILES:
            path = self.root / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("# Placeholder\n", encoding="utf-8")
        for rel in engineering_lint.REQUIRED_PROTECTED_PATHS:
            path = self.root / rel
            if rel.endswith("/"):
                path.mkdir(parents=True, exist_ok=True)
            else:
                path.parent.mkdir(parents=True, exist_ok=True)
                if not path.exists():
                    path.write_text("# Rule\n", encoding="utf-8")
        self._write_policy([])
        subprocess.run(["git", "init", "-q", str(self.root)], check=True)
        subprocess.run(
            ["git", "-C", str(self.root), "config", "user.email", "lint@example.invalid"],
            check=True,
        )
        subprocess.run(
            ["git", "-C", str(self.root), "config", "user.name", "Engineering Lint"],
            check=True,
        )
        subprocess.run(["git", "-C", str(self.root), "add", "."], check=True)
        subprocess.run(
            ["git", "-C", str(self.root), "commit", "-qm", "fixture"], check=True
        )
        self.observed_commit = subprocess.run(
            ["git", "-C", str(self.root), "rev-parse", "HEAD"],
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()

    def tearDown(self):
        self.tempdir.cleanup()

    def _write_policy(self, legacy: list[str]):
        protected = "\n".join(
            f"  - {path}" for path in sorted(engineering_lint.REQUIRED_PROTECTED_PATHS)
        )
        legacy_values = "\n".join(f"  - {path}" for path in legacy)
        (self.root / "docs" / "knowledge" / "README.md").write_text(
            "---\n"
            "title: Test policy\n"
            "owner: human\n"
            "status: approved\n"
            "agent_write_policy: human-authorized\n"
            "authorization_mode: conversation\n"
            "protected_paths:\n"
            f"{protected}\n"
            "legacy_upstream:\n"
            f"{legacy_values}\n"
            "---\n"
            "# Policy\n",
            encoding="utf-8",
        )

    def _write(self, rel: str, frontmatter: str, body: str = "# Page\n"):
        path = self.root / "docs" / "knowledge" / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(f"---\n{frontmatter.strip()}\n---\n{body}", encoding="utf-8")
        return path

    def _commit_fixture(self, message: str) -> str:
        subprocess.run(["git", "-C", str(self.root), "add", "."], check=True)
        subprocess.run(
            ["git", "-C", str(self.root), "commit", "-qm", message], check=True
        )
        return subprocess.run(
            ["git", "-C", str(self.root), "rev-parse", "HEAD"],
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()

    def _record_evidence_for_current_source(self) -> Path:
        self._add_valid_chain()
        source = self.root / "app" / "example" / "service.go"
        source.write_text("package example\n", encoding="utf-8")
        observed_commit = self._commit_fixture("validated source")
        evidence = self.root / "docs/knowledge/evidence/EVD-service.md"
        evidence.write_text(
            evidence.read_text(encoding="utf-8").replace(
                self.observed_commit, observed_commit
            ),
            encoding="utf-8",
        )
        self._commit_fixture("record evidence")
        return source

    def _add_valid_chain(self, *, evidence: bool = True):
        self._write(
            "intent/INT-product.md",
            """
id: INT-product
layer: intent
title: Product intent
status: approved
owner: human
upstream:
updated_at: 2026-09-06
""",
        )
        self._write(
            "spec/SPEC-behavior.md",
            """
id: SPEC-behavior
layer: spec
title: Behavior specification
status: approved
owner: human
upstream:
  - INT-product
updated_at: 2026-09-06
""",
            "# Page\n\n- `TEST-001`：Test requirement.\n",
        )
        self._write(
            "design/DES-service.md",
            """
id: DES-service
layer: design
title: Service design
status: active
owner: agent
upstream:
  - SPEC-behavior
updated_at: 2026-09-06
tracks:
  - TEST-001
""",
        )
        tracked = self.root / "app" / "example"
        tracked.mkdir(parents=True, exist_ok=True)
        self._write(
            "implementation/IMP-service.md",
            """
id: IMP-service
layer: implementation
title: Service implementation
status: aligned
owner: agent
upstream:
  - DES-service
tracks:
  - TEST-001
updated_at: 2026-09-06
code_paths:
  - app/example
evidence:
  - EVD-service
""",
            "# Page\n\n"
            "| Requirement | Design | Status | Evidence/Gap |\n"
            "| --- | --- | --- | --- |\n"
            "| TEST-001 | DES-service | aligned | EVD-service |\n",
        )
        indexes = {
            "intent": "INT-product.md",
            "spec": "SPEC-behavior.md",
            "design": "DES-service.md",
            "implementation": "IMP-service.md",
        }
        for layer, filename in indexes.items():
            (self.root / "docs" / "knowledge" / layer / "README.md").write_text(
                f"# Index\n\n- [{filename}]({filename})\n", encoding="utf-8"
            )
        if evidence:
            self._write(
                "evidence/EVD-service.md",
                f"""
id: EVD-service
layer: evidence
title: Service evidence
status: active
owner: agent
upstream:
  - IMP-service
updated_at: 2026-09-06
covers:
  - TEST-001
scope:
  - unit
commands:
  - make check
observed_commit: {self.observed_commit}
result: passed
""",
            )
            readme = self.root / "docs" / "knowledge" / "evidence" / "README.md"
            readme.write_text(
                "# 实现证据\n\n- [EVD-service.md](EVD-service.md)：样例证据。\n",
                encoding="utf-8",
            )

    def assert_error(self, errors: list[str], expected: str):
        self.assertTrue(
            any(expected in error for error in errors),
            msg=f"expected {expected!r} in {errors!r}",
        )

    def test_valid_chain_and_evidence(self):
        self._add_valid_chain()
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_governance_requires_conversation_authorization_policy(self):
        policy = self.root / "docs" / "knowledge" / "README.md"
        policy.write_text(
            policy.read_text(encoding="utf-8").replace(
                "authorization_mode: conversation",
                "authorization_mode: repository-file",
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "authorization_mode must be conversation")

    def test_duplicate_id_wrong_owner_and_status(self):
        self._write(
            "intent/one.md",
            """
id: INT-duplicate
layer: intent
title: First
status: approved
owner: human
upstream:
updated_at: 2026-09-06
""",
        )
        self._write(
            "intent/two.md",
            """
id: INT-duplicate
layer: intent
title: Second
status: active
owner: agent
upstream:
updated_at: 2026-09-06
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "duplicate id INT-duplicate")
        self.assert_error(errors, "owner must be human")
        self.assert_error(errors, "invalid status for intent")

    def test_proposal_cannot_be_design_upstream(self):
        self._write(
            "proposals/change.md",
            """
id: PROP-20260812-change
layer: proposal
title: Proposed change
status: open
owner: agent
target_layer: spec
""",
        )
        self._write(
            "design/DES-service.md",
            """
id: DES-service
layer: design
title: Service design
status: draft
owner: agent
upstream:
  - PROP-20260812-change
updated_at: 2026-09-06
tracks: []
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "proposal cannot be an upstream")

    def test_spec_cannot_reverse_reference_design(self):
        self._write(
            "design/DES-service.md",
            """
id: DES-service
layer: design
title: Service design
status: blocked
owner: agent
upstream:
updated_at: 2026-09-06
tracks:
  - TEST-001
blocking_reason: Waiting for specification
""",
        )
        self._write(
            "spec/SPEC-behavior.md",
            """
id: SPEC-behavior
layer: spec
title: Behavior specification
status: draft
owner: human
upstream:
  - DES-service
updated_at: 2026-09-06
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "upstream must reference INT-")

    def test_active_design_requires_approved_spec(self):
        self._write(
            "intent/INT-product.md",
            """
id: INT-product
layer: intent
title: Product intent
status: approved
owner: human
upstream:
updated_at: 2026-09-06
""",
        )
        self._write(
            "spec/SPEC-behavior.md",
            """
id: SPEC-behavior
layer: spec
title: Draft behavior
status: draft
owner: human
upstream:
  - INT-product
updated_at: 2026-09-06
""",
        )
        self._write(
            "design/DES-service.md",
            """
id: DES-service
layer: design
title: Service design
status: active
owner: agent
upstream:
  - SPEC-behavior
updated_at: 2026-09-06
tracks:
  - TEST-001
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "current design requires approved spec")

    def test_design_requires_upstream_or_blocking_reason(self):
        self._write(
            "design/DES-service.md",
            """
id: DES-service
layer: design
title: Service design
status: draft
owner: agent
upstream:
updated_at: 2026-09-06
tracks: []
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "design requires SPEC/legacy upstream")

    def test_allowlisted_legacy_heading_is_valid(self):
        self._write_policy(["AGENTS.md"])
        self._write(
            "design/DES-service.md",
            """
id: DES-service
layer: design
title: Transitional service design
status: superseded
owner: agent
upstream:
updated_at: 2026-09-06
tracks: []
legacy_upstream:
  - legacy:AGENTS.md#Rule
""",
        )
        (self.root / "docs/knowledge/design/README.md").write_text(
            "# Index\n\n- [DES-service.md](DES-service.md)\n", encoding="utf-8"
        )
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_unlisted_legacy_path_is_rejected(self):
        self._write(
            "design/DES-service.md",
            """
id: DES-service
layer: design
title: Transitional service design
status: superseded
owner: agent
upstream:
updated_at: 2026-09-06
tracks: []
legacy_upstream:
  - legacy:docs/ARCHITECTURE.md#Rule
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        # 旧 ARCHITECTURE 已迁移删除，legacy_upstream 白名单不再登记它。
        self.assert_error(errors, "legacy path is not allowlisted")

    def test_evidence_must_be_registered_in_readme(self):
        self._add_valid_chain(evidence=False)
        self._write(
            "evidence/EVD-service.md",
            f"""
id: EVD-service
layer: evidence
title: Service evidence
status: active
owner: agent
upstream:
  - IMP-service
updated_at: 2026-09-06
covers:
  - TEST-001
scope:
  - unit
commands:
  - make check
observed_commit: {self.observed_commit}
result: passed
""",
        )
        readme = self.root / "docs" / "knowledge" / "evidence" / "README.md"
        readme.write_text("# 实现证据\n", encoding="utf-8")
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(
            errors, "evidence file is not registered in evidence/README.md"
        )

    def test_evidence_readme_dead_link_fails(self):
        self._add_valid_chain()
        readme = self.root / "docs" / "knowledge" / "evidence" / "README.md"
        readme.write_text(
            "# 实现证据\n\n- [EVD-service.md](EVD-service.md)\n"
            "- [missing-file.md](missing-file.md)\n",
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "evidence README links missing file: missing-file.md")

    def test_evidence_requires_commands(self):
        self._add_valid_chain(evidence=False)
        self._write(
            "evidence/EVD-service.md",
            f"""
id: EVD-service
layer: evidence
title: Service evidence
status: active
owner: agent
upstream:
  - IMP-service
updated_at: 2026-09-06
covers:
  - TEST-001
scope:
  - unit
observed_commit: {self.observed_commit}
result: passed
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "missing frontmatter keys: commands")

    def test_grouped_requirement_row_is_rejected(self):
        self._add_valid_chain()
        implementation = self.root / "docs/knowledge/implementation/IMP-service.md"
        implementation.write_text(
            implementation.read_text(encoding="utf-8").replace(
                "| TEST-001 | DES-service | aligned | EVD-service |",
                "| TEST-001~002 | DES-service | aligned | EVD-service |",
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "tracking rows require one exact requirement ID")

    def test_authoritative_row_rejects_unpaired_backticks(self):
        self._add_valid_chain()
        implementation = self.root / "docs/knowledge/implementation/IMP-service.md"
        original = implementation.read_text(encoding="utf-8")
        valid = "| TEST-001 | DES-service | aligned | EVD-service |"
        malformed_rows = (
            "| `TEST-001 | DES-service | aligned | EVD-service |",
            "| TEST-001` | DES-service | aligned | EVD-service |",
            "| TEST-001 | `DES-service | aligned | EVD-service |",
            "| TEST-001 | DES-service` | aligned | EVD-service |",
            "| ``TEST-001`` | DES-service | aligned | EVD-service |",
            "| TEST-001 | ``DES-service`` | aligned | EVD-service |",
        )
        for row in malformed_rows:
            with self.subTest(row=row):
                implementation.write_text(
                    original.replace(valid, row), encoding="utf-8"
                )
                errors = engineering_lint.check_knowledge_layers(self.root)
                self.assert_error(
                    errors, "tracking rows require one exact requirement ID"
                )

    def test_requirement_definition_inside_fence_is_ignored(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8").replace(
                "- `TEST-001`：Test requirement.",
                "```text\n- `TEST-001`：Test requirement.\n```",
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "tracks unknown active requirement: TEST-001")

    def test_requirement_inside_list_container_fence_is_ignored(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        original = specification.read_text(encoding="utf-8")
        cases = {
            "unordered tilde": (
                "- ~~~markdown\n"
                "  - `TEST-001`: Hidden duplicate requirement.\n"
                "  ~~~\n"
            ),
            "ordered backtick": (
                "1. ```markdown\n"
                "   - `TEST-001`: Hidden duplicate requirement.\n"
                "   ```\n"
            ),
        }
        for name, fenced_example in cases.items():
            with self.subTest(container=name):
                specification.write_text(
                    original + "\n" + fenced_example,
                    encoding="utf-8",
                )
                self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_requirement_inside_list_continuation_fence_is_ignored(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        original = specification.read_text(encoding="utf-8")
        cases = {
            "unordered tilde": (
                "- Context\n"
                "  ~~~markdown\n"
                "  - `TEST-001`: Hidden duplicate requirement.\n"
                "  ~~~\n"
            ),
            "ordered backtick": (
                "1. Context\n"
                "   ```markdown\n"
                "   - `TEST-001`: Hidden duplicate requirement.\n"
                "   ```\n"
            ),
        }
        for name, fenced_example in cases.items():
            with self.subTest(container=name):
                specification.write_text(
                    original + "\n" + fenced_example,
                    encoding="utf-8",
                )
                self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_list_container_fence_content_cannot_close_an_inline_span(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8")
            + "\n- ````markdown\n"
            + "  prose contains the same-length ```` run.\n"
            + "  - `TEST-001`: Hidden duplicate requirement.\n"
            + "  ````\n",
            encoding="utf-8",
        )

        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_unclosed_list_container_fence_stops_at_sibling_item(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        original = specification.read_text(encoding="utf-8")
        for opener, content_indent in (
            ("- ~~~markdown", "  "),
            ("1. ```markdown", "   "),
        ):
            with self.subTest(opener=opener):
                specification.write_text(
                    original.replace(
                        "- `TEST-001`：Test requirement.",
                        f"{opener}\n"
                        f"{content_indent}- `FAKE-001`: Fenced example only.\n"
                        "- `TEST-001`: Visible sibling requirement.",
                    ),
                    encoding="utf-8",
                )
                self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_unclosed_list_continuation_fence_stops_at_sibling_item(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        original = specification.read_text(encoding="utf-8")
        cases = (
            (
                "- Context\n"
                "  ~~~markdown\n"
                "  - `FAKE-001`: Fenced example only.\n"
                "- `TEST-001`: Visible sibling requirement."
            ),
            (
                "- Outer item\n"
                "  1. Nested context\n"
                "     ```markdown\n"
                "     - `FAKE-001`: Fenced example only.\n"
                "  - `TEST-001`: Visible nested sibling requirement."
            ),
        )
        for replacement in cases:
            with self.subTest(replacement=replacement.splitlines()[0:2]):
                specification.write_text(
                    original.replace(
                        "- `TEST-001`：Test requirement.", replacement
                    ),
                    encoding="utf-8",
                )
                self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_bare_list_item_owns_a_continuation_fence(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8").replace(
                "- `TEST-001`：Test requirement.",
                "-\n"
                "  ```markdown\n"
                "  - `FAKE-001`: Fenced example only.\n"
                "- `TEST-001`: Visible sibling requirement.",
            ),
            encoding="utf-8",
        )

        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_requirement_definition_inside_frontmatter_is_ignored(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8").replace(
                "updated_at: 2026-09-06\n---",
                "updated_at: 2026-09-06\nexamples:\n"
                "- `TEST-001`: Metadata example only.\n---",
            ),
            encoding="utf-8",
        )
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_prose_requirement_reference_is_not_a_definition(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8")
            + "\nThis prose only refers to `TEST-001`: it is not a clause.\n",
            encoding="utf-8",
        )
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_orphan_table_row_is_not_a_requirement_definition(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8")
            + "\n| `TEST-001` | Incidental table-shaped reference |\n",
            encoding="utf-8",
        )
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_requirement_table_header_aliases_are_recognized(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        original = specification.read_text(encoding="utf-8")
        for header in ("Requirement", "条款", "ID"):
            with self.subTest(header=header):
                table = (
                    f"| {header} | Definition |\n"
                    "| --- | :--- |\n"
                    "| `TEST-001` | Test requirement. |"
                )
                specification.write_text(
                    original.replace("- `TEST-001`：Test requirement.", table),
                    encoding="utf-8",
                )
                self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_requirement_table_without_outer_pipes_is_recognized(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        table = (
            "   Requirement | Definition\n"
            "   --- | ---:\n"
            "   TEST-001 | Test requirement."
        )
        specification.write_text(
            specification.read_text(encoding="utf-8").replace(
                "- `TEST-001`：Test requirement.", table
            ),
            encoding="utf-8",
        )
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_requirement_table_requires_matching_valid_separator(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        original = specification.read_text(encoding="utf-8")
        invalid_separators = ("| --- |", "| --- | not-a-separator |")
        for separator in invalid_separators:
            with self.subTest(separator=separator):
                specification.write_text(
                    original
                    + "\n| Requirement | Definition |\n"
                    + separator
                    + "\n| `TEST-001` | Incidental reference |\n",
                    encoding="utf-8",
                )
                self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_requirement_table_rejects_data_row_column_mismatch(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        original = specification.read_text(encoding="utf-8")
        invalid_rows = (
            "| `TEST-001` |",
            "| `TEST-001` | Incidental reference | Unexpected cell |",
        )
        for row in invalid_rows:
            with self.subTest(row=row):
                specification.write_text(
                    original
                    + "\n| Requirement | Definition |\n"
                    + "| --- | --- |\n"
                    + row
                    + "\n",
                    encoding="utf-8",
                )
                self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_requirement_table_requires_structural_definition_content(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        original = specification.read_text(encoding="utf-8")
        invalid_tables = {
            "one column": (
                "| requirement |\n| --- |\n| `TEST-001` |"
            ),
            "blank header": (
                "| requirement | |\n| --- | --- |\n| `TEST-001` | Works. |"
            ),
            "blank definition": (
                "| requirement | definition |\n"
                "| --- | --- |\n"
                "| `TEST-001` | |"
            ),
            "unpaired backtick": (
                "| requirement | definition |\n"
                "| --- | --- |\n"
                "| `TEST-001 | Works. |"
            ),
            "double backticks": (
                "| requirement | definition |\n"
                "| --- | --- |\n"
                "| ``TEST-001`` | Works. |"
            ),
        }
        for name, table in invalid_tables.items():
            with self.subTest(table=name):
                specification.write_text(
                    original.replace("- `TEST-001`：Test requirement.", table),
                    encoding="utf-8",
                )
                errors = engineering_lint.check_knowledge_layers(self.root)
                self.assert_error(errors, "tracks unknown active requirement: TEST-001")

    def test_requirement_table_uses_backslash_parity_for_escaped_pipes(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        original = specification.read_text(encoding="utf-8")
        valid_table = (
            "| requirement | definition |\n"
            "| --- | --- |\n"
            r"| `TEST-001` | Literal \| pipe. |"
        )
        specification.write_text(
            original.replace("- `TEST-001`：Test requirement.", valid_table),
            encoding="utf-8",
        )
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

        invalid_table = (
            "| requirement | definition |\n"
            "| --- | --- |\n"
            r"| `TEST-001` | Even slash pair \\| starts another cell. |"
        )
        specification.write_text(
            original.replace("- `TEST-001`：Test requirement.", invalid_table),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "tracks unknown active requirement: TEST-001")

    def test_star_requirement_bullet_is_recognized(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8").replace(
                "- `TEST-001`：Test requirement.",
                "   * `TEST-001`: Test requirement.",
            ),
            encoding="utf-8",
        )
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_requirement_bullet_requires_same_line_definition(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8").replace(
                "- `TEST-001`：Test requirement.",
                "- `TEST-001`:\n  Definition only appears on a continuation line.",
            ),
            encoding="utf-8",
        )

        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "tracks unknown active requirement: TEST-001")

    def test_indented_code_bullet_is_not_a_requirement_definition(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8")
            + "\n    - `TEST-001`: Indented code example.\n",
            encoding="utf-8",
        )
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_indented_code_table_is_not_a_requirement_definition(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        original = specification.read_text(encoding="utf-8")
        for indentation in ("    ", "\t", "  \t"):
            with self.subTest(indentation=repr(indentation)):
                specification.write_text(
                    original
                    + f"\n{indentation}| Requirement | Definition |\n"
                    + f"{indentation}| --- | --- |\n"
                    + f"{indentation}| `TEST-001` | Indented code example |\n",
                    encoding="utf-8",
                )
                self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_inline_code_comment_opener_does_not_hide_visible_requirement(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8")
            + "\nThe literal inline marker is `<!--`.\n"
            + "- `TEST-001`：Duplicate visible requirement.\n",
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "duplicate approved requirement TEST-001")

    def test_multiline_inline_code_hides_only_its_exact_delimited_span(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        original = specification.read_text(encoding="utf-8")
        for delimiter_length in (1, 2, 3):
            with self.subTest(delimiter_length=delimiter_length):
                delimiter = "`" * delimiter_length
                nonmatching_runs = " / ".join(
                    "`" * run_length
                    for run_length in range(1, 5)
                    if run_length != delimiter_length
                )
                bullet_quotes = "``" if delimiter_length == 1 else "`"
                specification.write_text(
                    original
                    + f"\nProse {delimiter}multiline code begins\n"
                    + "| Requirement | Definition |\n"
                    + "| --- | --- |\n"
                    + "| FAKE-002 | Hidden table row. |\n"
                    + f"Nonmatching runs {nonmatching_runs} and <!-- stay literal.\n"
                    + f"{delimiter}- `FAKE-003`: Closing-line suffix is not a bullet.\n"
                    + "- `TEST-001`: Duplicate visible requirement.\n",
                    encoding="utf-8",
                )
                errors = engineering_lint.check_knowledge_layers(self.root)
                self.assert_error(
                    errors, "duplicate approved requirement TEST-001"
                )
                self.assertFalse(
                    any("FAKE-" in error for error in errors), errors
                )

    def test_unclosed_inline_code_stops_at_top_level_list_item(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8").replace(
                "- `TEST-001`：Test requirement.",
                "Paragraph ``has no closing delimiter\n"
                "- `TEST-001`: Visible list requirement.\n"
                "A later `` run cannot close the previous block.",
            ),
            encoding="utf-8",
        )

        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_unclosed_inline_code_in_list_item_stops_at_sibling_item(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8").replace(
                "- `TEST-001`：Test requirement.",
                "- Context ``has no closing delimiter\n"
                "- `TEST-001`: Visible sibling requirement.\n"
                "  A later `` run cannot close the previous item.",
            ),
            encoding="utf-8",
        )

        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_multiline_inline_code_can_close_in_same_list_item(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8")
            + "\n- Context ``starts a code span\n"
            + "  <!-- remains literal on a continuation line\n"
            + "  and closes here`` outside.\n",
            encoding="utf-8",
        )

        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_unclosed_inline_code_stops_at_nested_list_item(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8").replace(
                "- `TEST-001`：Test requirement.",
                "- Context ``has no closing delimiter\n"
                "  - `TEST-001`: Visible nested requirement.\n"
                "  A later `` run cannot close the parent item.",
            ),
            encoding="utf-8",
        )

        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_unmatched_inline_code_stops_at_blank_line(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8")
            + "\nUnmatched `` opener <!-- closed comment -->\n"
            + "\n"
            + "- `TEST-001`: Duplicate visible requirement.\n"
            + "Later `` same-length run.\n",
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "duplicate approved requirement TEST-001")

    def test_unmatched_inline_code_stops_at_markdown_block_boundary(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        original = specification.read_text(encoding="utf-8")
        cases = {
            "atx": "# Boundary\n",
            "setext": "Boundary\n===\n",
            "fence": "```text\ninside fence\n```\n",
        }
        for name, boundary in cases.items():
            with self.subTest(name=name):
                specification.write_text(
                    original
                    + "\nUnmatched `` opener\n"
                    + boundary
                    + "- `TEST-001`: Duplicate visible requirement.\n"
                    + "Later `` same-length run.\n",
                    encoding="utf-8",
                )
                errors = engineering_lint.check_knowledge_layers(self.root)
                self.assert_error(
                    errors, "duplicate approved requirement TEST-001"
                )

    def test_fenced_comment_opener_does_not_hide_visible_requirement(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8")
            + "\n```text\n<!--\n```\n"
            + "- `TEST-001`：Duplicate visible requirement.\n-->\n",
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "duplicate approved requirement TEST-001")

    def test_unclosed_html_comment_hides_to_eof(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8")
            + "\n<!--\n- `TEST-001`：Hidden duplicate requirement.\n",
            encoding="utf-8",
        )
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_backtick_in_backtick_fence_info_does_not_open_fence(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8")
            + "\n```text`invalid\n"
            + "- `TEST-001`：Duplicate visible requirement.\n```\n",
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "duplicate approved requirement TEST-001")

    def test_backtick_in_tilde_fence_info_is_allowed(self):
        self._add_valid_chain()
        specification = self.root / "docs/knowledge/spec/SPEC-behavior.md"
        specification.write_text(
            specification.read_text(encoding="utf-8")
            + "\n~~~text`allowed\n"
            + "- `TEST-001`：Hidden duplicate requirement.\n~~~\n",
            encoding="utf-8",
        )
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_authoritative_rows_inside_fence_are_ignored(self):
        self._add_valid_chain()
        implementation = self.root / "docs/knowledge/implementation/IMP-service.md"
        table = (
            "| Requirement | Design | Status | Evidence/Gap |\n"
            "| --- | --- | --- | --- |\n"
            "| TEST-001 | DES-service | aligned | EVD-service |"
        )
        implementation.write_text(
            implementation.read_text(encoding="utf-8").replace(
                table, f"```text\n{table}\n```"
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(
            errors, "requires exactly one authoritative Requirement table header"
        )

    def test_authoritative_rows_require_prescribed_table_header(self):
        self._add_valid_chain()
        implementation = self.root / "docs/knowledge/implementation/IMP-service.md"
        implementation.write_text(
            implementation.read_text(encoding="utf-8").replace(
                "| Requirement | Design | Status | Evidence/Gap |",
                "| Requirement ID | Design | Status | Evidence/Gap |",
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(
            errors, "requires exactly one authoritative Requirement table header"
        )

    def test_authoritative_rows_require_prescribed_separator(self):
        self._add_valid_chain()
        implementation = self.root / "docs/knowledge/implementation/IMP-service.md"
        implementation.write_text(
            implementation.read_text(encoding="utf-8").replace(
                "| --- | --- | --- | --- |",
                "| :--- | --- | --- | --- |",
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(
            errors, "authoritative Requirement table requires the exact separator row"
        )

    def test_approved_requirement_requires_exactly_one_current_imp(self):
        self._add_valid_chain()
        source = self.root / "docs/knowledge/implementation/IMP-service.md"
        duplicate = self.root / "docs/knowledge/implementation/IMP-duplicate.md"
        duplicate.write_text(
            source.read_text(encoding="utf-8").replace("IMP-service", "IMP-duplicate"),
            encoding="utf-8",
        )
        index = duplicate.parent / "README.md"
        index.write_text(index.read_text(encoding="utf-8") + "- [duplicate.md](duplicate.md)\n")
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "multiple current IMP owners: TEST-001")

    def test_current_design_requires_current_imp(self):
        self._add_valid_chain()
        implementation = self.root / "docs/knowledge/implementation/IMP-service.md"
        implementation.unlink()
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "current design has no current IMP")

    def test_approved_requirement_requires_exactly_one_current_design(self):
        self._add_valid_chain()
        source = self.root / "docs/knowledge/design/DES-service.md"
        duplicate = self.root / "docs/knowledge/design/DES-duplicate.md"
        duplicate.write_text(
            source.read_text(encoding="utf-8").replace(
                "DES-service", "DES-duplicate"
            ),
            encoding="utf-8",
        )
        index = duplicate.parent / "README.md"
        index.write_text(
            index.read_text(encoding="utf-8")
            + "- [duplicate.md](duplicate.md)\n",
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "multiple current DES owners: TEST-001")

    def test_aligned_row_requires_active_passed_evidence(self):
        self._add_valid_chain()
        evidence = self.root / "docs/knowledge/evidence/EVD-service.md"
        evidence.write_text(
            evidence.read_text(encoding="utf-8").replace(
                "status: active", "status: superseded"
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "aligned row lacks active passed EVD coverage: TEST-001")

    def test_non_aligned_row_requires_gap_prefix(self):
        self._add_valid_chain()
        implementation = self.root / "docs/knowledge/implementation/IMP-service.md"
        implementation.write_text(
            implementation.read_text(encoding="utf-8").replace(
                "| TEST-001 | DES-service | aligned | EVD-service |",
                "| TEST-001 | DES-service | unknown | evidence pending |",
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "unknown row requires detail in 'gap: ...' form")

    def test_implementation_status_must_match_row_aggregate(self):
        self._add_valid_chain()
        implementation = self.root / "docs/knowledge/implementation/IMP-service.md"
        implementation.write_text(
            implementation.read_text(encoding="utf-8")
            .replace("status: aligned", "status: unknown")
            .replace(
                "| TEST-001 | DES-service | aligned | EVD-service |",
                "| TEST-001 | DES-service | unknown | gap: validation pending |",
            )
            .replace("status: unknown", "status: aligned", 1),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(
            errors,
            "implementation status must equal authoritative row aggregate: "
            "expected unknown, got aligned",
        )

    def test_imp_and_evidence_require_bidirectional_links(self):
        self._add_valid_chain()
        implementation = self.root / "docs/knowledge/implementation/IMP-service.md"
        implementation.write_text(
            implementation.read_text(encoding="utf-8").replace(
                "evidence:\n  - EVD-service", "evidence: []"
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "upstream IMP does not point back to EVD-service")

    def test_evidence_requires_reachable_full_commit(self):
        self._add_valid_chain()
        evidence = self.root / "docs/knowledge/evidence/EVD-service.md"
        evidence.write_text(
            evidence.read_text(encoding="utf-8").replace(
                self.observed_commit, "1234567"
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "observed_commit must be a full 40-character SHA")

    def test_evidence_rejects_unreachable_full_commit(self):
        self._add_valid_chain()
        evidence = self.root / "docs/knowledge/evidence/EVD-service.md"
        evidence.write_text(
            evidence.read_text(encoding="utf-8").replace(
                self.observed_commit, "0" * 40
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "observed_commit is not an ancestor of HEAD")

    def test_passed_evidence_cannot_only_use_tmp_artifacts(self):
        self._add_valid_chain()
        evidence = self.root / "docs/knowledge/evidence/EVD-service.md"
        evidence.write_text(
            evidence.read_text(encoding="utf-8").replace(
                "result: passed", "result: passed\nartifacts:\n  - /tmp/result.txt"
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "active passed evidence cannot rely only on /tmp artifacts")

    def test_evidence_scope_uses_controlled_values(self):
        self._add_valid_chain()
        evidence = self.root / "docs/knowledge/evidence/EVD-service.md"
        evidence.write_text(
            evidence.read_text(encoding="utf-8").replace("  - unit", "  - guess"),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "invalid evidence scope: guess")

    def test_required_frontmatter_lists_reject_blank_items(self):
        self._add_valid_chain()
        cases = (
            ("design/DES-service.md", "tracks:\n  - TEST-001", "tracks"),
            ("implementation/IMP-service.md", "tracks:\n  - TEST-001", "tracks"),
            (
                "implementation/IMP-service.md",
                "code_paths:\n  - app/example",
                "code_paths",
            ),
            (
                "implementation/IMP-service.md",
                "evidence:\n  - EVD-service",
                "evidence",
            ),
            ("evidence/EVD-service.md", "covers:\n  - TEST-001", "covers"),
            ("evidence/EVD-service.md", "scope:\n  - unit", "scope"),
            ("evidence/EVD-service.md", "commands:\n  - make check", "commands"),
        )
        for relative, original_value, key in cases:
            with self.subTest(relative=relative, key=key):
                page = self.root / "docs" / "knowledge" / relative
                original = page.read_text(encoding="utf-8")
                page.write_text(
                    original.replace(original_value, f'{key}:\n  - "   "'),
                    encoding="utf-8",
                )
                errors = engineering_lint.check_knowledge_layers(self.root)
                self.assert_error(errors, f"{key} contains blank items at positions: 1")
                page.write_text(original, encoding="utf-8")

    def test_critical_frontmatter_lists_reject_duplicates(self):
        self._add_valid_chain()
        cases = (
            ("design/DES-service.md", "tracks:\n  - TEST-001", "tracks", "IDs"),
            (
                "implementation/IMP-service.md",
                "code_paths:\n  - app/example",
                "code_paths",
                "items",
            ),
            (
                "evidence/EVD-service.md",
                "upstream:\n  - IMP-service",
                "upstream",
                "items",
            ),
        )
        for relative, original_value, key, label in cases:
            with self.subTest(relative=relative, key=key):
                page = self.root / "docs" / "knowledge" / relative
                original = page.read_text(encoding="utf-8")
                item = original_value.splitlines()[-1]
                page.write_text(
                    original.replace(original_value, f"{original_value}\n{item}"),
                    encoding="utf-8",
                )
                errors = engineering_lint.check_knowledge_layers(self.root)
                self.assert_error(errors, f"{key} contains duplicate {label}")
                page.write_text(original, encoding="utf-8")

    def test_formal_page_title_must_not_be_blank(self):
        self._add_valid_chain()
        design = self.root / "docs/knowledge/design/DES-service.md"
        design.write_text(
            design.read_text(encoding="utf-8").replace(
                "title: Service design", 'title: "   "'
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "title must be non-empty")

    def test_updated_at_must_be_a_real_calendar_date(self):
        self._add_valid_chain()
        design = self.root / "docs/knowledge/design/DES-service.md"
        design.write_text(
            design.read_text(encoding="utf-8").replace(
                "updated_at: 2026-09-06", "updated_at: 2026-99-99"
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "updated_at must be a valid YYYY-MM-DD date")

    def test_formal_page_filename_must_match_id(self):
        self._add_valid_chain()
        self._write(
            "design/foo.md",
            """
id: DES-other
layer: design
title: Other design
status: superseded
owner: agent
upstream:
  - SPEC-behavior
updated_at: 2026-09-06
tracks: []
""",
        )
        index = self.root / "docs/knowledge/design/README.md"
        index.write_text(
            index.read_text(encoding="utf-8") + "- [foo.md](foo.md)\n",
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "filename must match id: DES-other.md")

    def test_nested_readme_is_not_exempt_from_formal_page_schema(self):
        self._add_valid_chain()
        self._write(
            "design/nested/README.md",
            """
id: DES-hidden
layer: design
title: Hidden design
status: superseded
owner: agent
upstream:
  - SPEC-behavior
updated_at: 2026-09-06
tracks: []
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "design pages must be direct children")
        self.assert_error(errors, "filename must match id: DES-hidden.md")

    def test_active_passed_evidence_rejects_changed_upstream_code_paths(self):
        source = self._record_evidence_for_current_source()
        source.write_text("package example\n\nconst changed = true\n", encoding="utf-8")
        self._commit_fixture("change validated source")
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(
            errors,
            "active passed evidence is stale; upstream code_paths changed "
            "after observed_commit: IMP-service",
        )

    def test_active_passed_evidence_rejects_dirty_upstream_code_paths(self):
        source = self._record_evidence_for_current_source()
        source.write_text("package example\n\nconst dirty = true\n", encoding="utf-8")
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(
            errors,
            "active passed evidence is stale; upstream code_paths changed "
            "after observed_commit: IMP-service",
        )

    def test_active_passed_evidence_rejects_untracked_upstream_source(self):
        source = self._record_evidence_for_current_source()
        (source.parent / "new.go").write_text("package example\n", encoding="utf-8")
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(
            errors,
            "active passed evidence is stale; upstream code_paths changed "
            "after observed_commit: IMP-service",
        )

    def test_active_passed_evidence_allows_knowledge_only_follow_up(self):
        self._record_evidence_for_current_source()
        implementation = self.root / "docs/knowledge/implementation/IMP-service.md"
        implementation.write_text(
            implementation.read_text(encoding="utf-8") + "\nValidation notes updated.\n",
            encoding="utf-8",
        )
        self._commit_fixture("update implementation notes")
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_external_upstream_requires_canonical_wire_format(self):
        self._add_valid_chain()
        intent = self.root / "docs/knowledge/intent/INT-product.md"
        intent.write_text(
            intent.read_text(encoding="utf-8").replace(
                "updated_at: 2026-09-06",
                "updated_at: 2026-09-06\nexternal_upstream:\n  - backend@123:SPEC-other",
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "invalid external_upstream")

    def test_external_upstream_accepts_requirement_target(self):
        self._add_valid_chain()
        intent = self.root / "docs/knowledge/intent/INT-product.md"
        intent.write_text(
            intent.read_text(encoding="utf-8").replace(
                "updated_at: 2026-09-06",
                "updated_at: 2026-09-06\nexternal_upstream:\n"
                f"  - little-white-box-front@{self.observed_commit}:FRONT-001",
            ),
            encoding="utf-8",
        )
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_external_upstream_rejects_explicit_empty_list(self):
        self._add_valid_chain()
        intent = self.root / "docs/knowledge/intent/INT-product.md"
        intent.write_text(
            intent.read_text(encoding="utf-8").replace(
                "updated_at: 2026-09-06",
                "updated_at: 2026-09-06\nexternal_upstream: []",
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(
            errors,
            "external_upstream must be omitted or contain at least one reference",
        )

    def test_code_paths_reject_parent_traversal(self):
        self._add_valid_chain()
        implementation = self.root / "docs/knowledge/implementation/IMP-service.md"
        implementation.write_text(
            implementation.read_text(encoding="utf-8").replace(
                "  - app/example", "  - ../outside"
            ),
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "code path does not exist: ../outside")

    def test_all_formal_layers_require_index_registration(self):
        self._add_valid_chain()
        (self.root / "docs/knowledge/spec/README.md").write_text("# Index\n")
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "spec file is not registered in spec/README.md")

    def test_readme_link_inside_html_comment_does_not_register_page(self):
        self._add_valid_chain()
        readme = self.root / "docs/knowledge/spec/README.md"
        readme.write_text(
            "# Index\n\n<!-- [behavior.md](behavior.md) -->\n",
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "spec file is not registered in spec/README.md")

    def test_formal_page_must_be_registered_exactly_once(self):
        self._add_valid_chain()
        index = self.root / "docs/knowledge/design/README.md"
        index.write_text(
            index.read_text(encoding="utf-8")
            + "- [service again](DES-service.md)\n",
            encoding="utf-8",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(
            errors, "design file is registered multiple times: DES-service.md"
        )

    def test_blocked_design_is_current(self):
        self._add_valid_chain()
        design = self.root / "docs/knowledge/design/DES-service.md"
        design.write_text(
            design.read_text(encoding="utf-8").replace("status: active", "status: blocked"),
            encoding="utf-8",
        )
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])


class ProtoGenerationLintTest(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        (self.root / "scripts").mkdir(parents=True)
        (self.root / "proto" / "search").mkdir(parents=True)
        (self.root / "proto" / "content").mkdir(parents=True)

    def tearDown(self):
        self.tempdir.cleanup()

    def test_allowlisted_ungenerated_proto_is_ok(self):
        (self.root / "scripts" / "generate.sh").write_text(
            "goctl rpc protoc proto/search/search.proto\n", encoding="utf-8"
        )
        (self.root / "proto" / "search" / "search.proto").write_text(
            "service Search { rpc Search(Req) returns (Resp); }\n", encoding="utf-8"
        )
        for rel in engineering_lint.UNGENERATED_RPC_PROTOS:
            path = self.root / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(
                "service X { rpc Ping(Req) returns (Resp); }\n", encoding="utf-8"
            )
        self.assertEqual(engineering_lint.check_proto_generation(self.root), [])

    def test_new_rpc_proto_must_be_generated(self):
        (self.root / "scripts" / "generate.sh").write_text(
            "goctl rpc protoc proto/search/search.proto\n", encoding="utf-8"
        )
        (self.root / "proto" / "search" / "search.proto").write_text(
            "service Search { rpc Search(Req) returns (Resp); }\n", encoding="utf-8"
        )
        for rel in engineering_lint.UNGENERATED_RPC_PROTOS:
            path = self.root / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(
                "service X { rpc Ping(Req) returns (Resp); }\n", encoding="utf-8"
            )
        extra = self.root / "proto" / "extra"
        extra.mkdir()
        (extra / "extra.proto").write_text(
            "service Extra { rpc Ping(Req) returns (Resp); }\n", encoding="utf-8"
        )
        errors = engineering_lint.check_proto_generation(self.root)
        self.assertTrue(any("proto/extra/extra.proto" in error for error in errors), errors)



if __name__ == "__main__":
    unittest.main()


class DocPolicyLintTest(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        (self.root / "docs" / "knowledge").mkdir(parents=True, exist_ok=True)
        self.agents = self.root / "AGENTS.md"
        self.claude = self.root / "CLAUDE.md"
        (self.root / "docs" / "INDEX.md").write_text("# Index\n", encoding="utf-8")
        (self.root / "docs" / "knowledge" / "README.md").write_text(
            "# Policy\n", encoding="utf-8"
        )
        self.agents.write_text(
            "# Rules\n\nSee docs/knowledge/README.md for routing.\n", encoding="utf-8"
        )
        self.claude.write_text("# CLAUDE\n\nSee AGENTS.md.\n", encoding="utf-8")

    def tearDown(self):
        self.tempdir.cleanup()

    def test_valid_docs_pass(self):
        self.assertEqual(engineering_lint.check_doc_policy(self.root), [])

    def test_agents_must_route_through_knowledge_readme(self):
        self.agents.write_text("# Rules without routing\n", encoding="utf-8")
        errors = engineering_lint.check_doc_policy(self.root)
        self.assertTrue(any("route through" in e for e in errors))

    def test_legacy_terms_are_rejected(self):
        self.agents.write_text(
            "# Rules\n\nSee docs/knowledge/README.md. zero-powers is forbidden.\n",
            encoding="utf-8",
        )
        errors = engineering_lint.check_doc_policy(self.root)
        self.assertTrue(any("zero-powers" in e for e in errors))

    def test_legacy_docs_directory_is_rejected(self):
        (self.root / "docs" / "references").mkdir(parents=True, exist_ok=True)
        errors = engineering_lint.check_doc_policy(self.root)
        self.assertTrue(any("legacy docs directory" in e for e in errors))

    def test_claude_must_point_to_agents(self):
        self.claude.write_text("# CLAUDE without pointer\n", encoding="utf-8")
        errors = engineering_lint.check_doc_policy(self.root)
        self.assertTrue(any("CLAUDE.md must point to AGENTS.md" in e for e in errors))


class MdFileLinkLintTest(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        (self.root / "docs" / "knowledge" / "implementation").mkdir(parents=True, exist_ok=True)

    def tearDown(self):
        self.tempdir.cleanup()

    def test_resolved_link_passes(self):
        (self.root / "docs" / "knowledge" / "implementation" / "target.md").write_text(
            "# T\n", encoding="utf-8"
        )
        (self.root / "docs" / "knowledge" / "implementation" / "source.md").write_text(
            "[t](target.md)\n", encoding="utf-8"
        )
        self.assertEqual(engineering_lint.check_md_file_links(self.root), [])

    def test_broken_link_is_reported(self):
        # 链接检查仅覆盖 docs/knowledge/ 下的活动文档（ACTIVE_DIRS）。
        (self.root / "docs" / "knowledge" / "implementation" / "source.md").write_text(
            "[missing](../missing.md)\n", encoding="utf-8"
        )
        errors = engineering_lint.check_md_file_links(self.root)
        self.assertTrue(any("missing.md" in e for e in errors), errors)

    def test_legacy_implementation_evidence_is_not_link_checked(self):
        legacy = (
            self.root
            / "docs/knowledge/implementation/evidence/2026-08-14-snapshot.md"
        )
        legacy.parent.mkdir(parents=True, exist_ok=True)
        legacy.write_text("Historical `eval/removed.json` input.\n", encoding="utf-8")
        self.assertEqual(engineering_lint.check_md_file_links(self.root), [])
