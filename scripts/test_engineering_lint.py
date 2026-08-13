from __future__ import annotations

import importlib.util
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

    def _write(self, rel: str, frontmatter: str):
        path = self.root / "docs" / "knowledge" / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(f"---\n{frontmatter.strip()}\n---\n# Page\n", encoding="utf-8")
        return path

    def _add_valid_chain(self, *, evidence: bool = True):
        self._write(
            "intent/product.md",
            """
id: INT-product
layer: intent
title: Product intent
status: approved
owner: human
upstream:
""",
        )
        self._write(
            "spec/behavior.md",
            """
id: SPEC-behavior
layer: spec
title: Behavior specification
status: approved
owner: human
upstream:
  - INT-product
""",
        )
        self._write(
            "design/service.md",
            """
id: DES-service
layer: design
title: Service design
status: active
owner: agent
upstream:
  - SPEC-behavior
""",
        )
        tracked = self.root / "app" / "example"
        tracked.mkdir(parents=True, exist_ok=True)
        self._write(
            "implementation/service.md",
            """
id: IMP-service
layer: implementation
title: Service implementation
status: aligned
owner: agent
upstream:
  - DES-service
tracks:
  - app/example
verified_at: 2026-08-12
verified_commit: 1234567
""",
        )
        if evidence:
            self._write(
                "implementation/evidence/2026-08-12-service.md",
                """
implementation: IMP-service
verified_at: 2026-08-12
verified_commit: 1234567
commands:
  - make check
result: passed
""",
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
            "design/service.md",
            """
id: DES-service
layer: design
title: Service design
status: draft
owner: agent
upstream:
  - PROP-20260812-change
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "proposal cannot be an upstream")

    def test_spec_cannot_reverse_reference_design(self):
        self._write(
            "design/service.md",
            """
id: DES-service
layer: design
title: Service design
status: blocked
owner: agent
upstream:
blocking_reason: Waiting for specification
""",
        )
        self._write(
            "spec/behavior.md",
            """
id: SPEC-behavior
layer: spec
title: Behavior specification
status: draft
owner: human
upstream:
  - DES-service
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "upstream must reference INT-")

    def test_active_design_requires_approved_spec(self):
        self._write(
            "intent/product.md",
            """
id: INT-product
layer: intent
title: Product intent
status: approved
owner: human
upstream:
""",
        )
        self._write(
            "spec/behavior.md",
            """
id: SPEC-behavior
layer: spec
title: Draft behavior
status: draft
owner: human
upstream:
  - INT-product
""",
        )
        self._write(
            "design/service.md",
            """
id: DES-service
layer: design
title: Service design
status: active
owner: agent
upstream:
  - SPEC-behavior
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "active design requires approved spec")

    def test_design_requires_upstream_or_blocking_reason(self):
        self._write(
            "design/service.md",
            """
id: DES-service
layer: design
title: Service design
status: draft
owner: agent
upstream:
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "design requires SPEC/legacy upstream")

    def test_allowlisted_legacy_heading_is_valid(self):
        self._write_policy(["AGENTS.md"])
        self._write(
            "design/service.md",
            """
id: DES-service
layer: design
title: Transitional service design
status: active
owner: agent
upstream:
legacy_upstream:
  - legacy:AGENTS.md#Rule
""",
        )
        self.assertEqual(engineering_lint.check_knowledge_layers(self.root), [])

    def test_unlisted_legacy_path_is_rejected(self):
        self._write(
            "design/service.md",
            """
id: DES-service
layer: design
title: Transitional service design
status: active
owner: agent
upstream:
legacy_upstream:
  - legacy:docs/ARCHITECTURE.md#Rule
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        # 旧 ARCHITECTURE 已迁移删除，legacy_upstream 白名单不再登记它。
        self.assert_error(errors, "legacy path is not allowlisted")

    def test_evidence_requires_commands(self):
        self._add_valid_chain(evidence=False)
        self._write(
            "implementation/evidence/2026-08-12-service.md",
            """
implementation: IMP-service
verified_at: 2026-08-12
verified_commit: 1234567
result: passed
""",
        )
        errors = engineering_lint.check_knowledge_layers(self.root)
        self.assert_error(errors, "missing frontmatter keys: commands")


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



class SpecTrackingLintTest(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = Path(self.tempdir.name)
        self.des = self.root / engineering_lint.SPEC_TRACKING_DES
        self.imp = self.root / engineering_lint.SPEC_TRACKING_IMP
        self.des.parent.mkdir(parents=True, exist_ok=True)
        self.imp.parent.mkdir(parents=True, exist_ok=True)

    def tearDown(self):
        self.tempdir.cleanup()

    def _write_pages(self, *, des_text: str, imp_text: str):
        self.des.write_text(des_text, encoding="utf-8")
        self.imp.write_text(imp_text, encoding="utf-8")

    def test_missing_files_are_skipped(self):
        self.assertEqual(engineering_lint.check_spec_tracking(self.root), [])

    def test_tracking_must_live_in_implementation(self):
        headings = "\n".join(f"## {name}\n" for name in engineering_lint.SPEC_TRACKING_HEADINGS)
        self._write_pages(des_text="# Design\n\n" + headings, imp_text="# Impl\n")
        errors = engineering_lint.check_spec_tracking(self.root)
        self.assertTrue(any("must not contain implementation tracking heading" in error for error in errors), errors)
        self.assertTrue(any("implementation tracking heading" in error and "is required" in error for error in errors), errors)

    def test_forbidden_aligned_rows_fail(self):
        headings = "\n".join(f"## {name}\n" for name in engineering_lint.SPEC_TRACKING_HEADINGS)
        self._write_pages(
            des_text="# Design\n",
            imp_text=(
                "# Impl\n"
                + headings
                + "\n| REL-030~033 SLO | aligned | no data |\n"
                + "| ASST-050~051 eval | aligned | no frozen set |\n"
            ),
        )
        errors = engineering_lint.check_spec_tracking(self.root)
        self.assertTrue(any("REL-030 cannot be marked aligned" in error for error in errors), errors)
        self.assertTrue(any("ASST-050 cannot be marked aligned" in error for error in errors), errors)

    def test_partial_blocked_rows_pass(self):
        headings = "\n".join(f"## {name}\n" for name in engineering_lint.SPEC_TRACKING_HEADINGS)
        self._write_pages(
            des_text="# Design\n",
            imp_text=(
                "# Impl\n"
                + headings
                + "\n| CORE-013 revision | partial | v1 skip |\n"
                + "| CORE-014 read | aligned | status returned |\n"
            ),
        )
        self.assertEqual(engineering_lint.check_spec_tracking(self.root), [])



if __name__ == "__main__":
    unittest.main()
