from __future__ import annotations

import importlib.util
import tempfile
import unittest
from pathlib import Path

import yaml

SCRIPT = Path(__file__).with_name("engineering-lint.py")
SPEC = importlib.util.spec_from_file_location("engineering_lint", SCRIPT)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"cannot load {SCRIPT}")
engineering_lint = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(engineering_lint)


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
        self.assertTrue(
            any("proto/extra/extra.proto" in error for error in errors), errors
        )


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

    def test_governance_keeps_human_authorization_and_proposals_non_authoritative(self):
        protected = [
            "AGENTS.md",
            "docs/INDEX.md",
            "docs/knowledge/README.md",
            "docs/knowledge/templates/",
            "docs/knowledge/intent/",
            "docs/knowledge/spec/",
        ]
        for path in protected:
            if path.endswith("/"):
                (self.root / path).mkdir(parents=True)
        policy = dict(
            owner="human",
            status="approved",
            agent_write_policy="human-authorized",
            authorization_mode="conversation",
            protected_paths=protected,
            legacy_upstream=["AGENTS.md"],
        )
        target = self.root / "docs/knowledge/README.md"

        def write(values):
            target.write_text("---\n" + yaml.safe_dump(values) + "---\n")

        write(policy)
        self.assertEqual(engineering_lint.check_knowledge_governance(self.root), [])
        for field, value in [
            ("owner", "agent"),
            ("status", "draft"),
            ("agent_write_policy", "automatic"),
            ("authorization_mode", "file"),
            ("protected_paths", protected[:-1]),
        ]:
            with self.subTest(field=field):
                write({**policy, field: value})
                self.assertTrue(engineering_lint.check_knowledge_governance(self.root))
        write(policy)
        proposal = self.root / "docs/knowledge/proposals/PROP-20260907-test.md"
        proposal.parent.mkdir()
        meta = dict(
            id=proposal.stem,
            layer="proposal",
            title="Suggestion",
            owner="agent",
            status="open",
            target_layer="spec",
        )
        proposal.write_text("---\n" + yaml.safe_dump(meta) + "---\n")
        self.assertEqual(engineering_lint.check_knowledge_governance(self.root), [])
        proposal.write_text(
            "---\n" + yaml.safe_dump({**meta, "status": "approved"}) + "---\n"
        )
        self.assertTrue(engineering_lint.check_knowledge_governance(self.root))

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
        (self.root / "docs" / "knowledge" / "implementation").mkdir(
            parents=True, exist_ok=True
        )

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
            self.root / "docs/knowledge/implementation/evidence/2026-08-14-snapshot.md"
        )
        legacy.parent.mkdir(parents=True, exist_ok=True)
        legacy.write_text("Historical `eval/removed.json` input.\n", encoding="utf-8")
        self.assertEqual(engineering_lint.check_md_file_links(self.root), [])
