from pathlib import Path
import subprocess
import tempfile
import unittest

import knowledge as k


class MarkdownTest(unittest.TestCase):
    def test_ast_definitions_and_tables(self):
        cases = [
            ("- `FX-001`: Defined.", ["FX-001"]),
            ("* `FX-001`：Defined.", ["FX-001"]),
            ("id | definition\n--- | ---\nFX-001 | Defined.", ["FX-001"]),
            (
                "| `requirement` | definition |\n| --- | --- |\n| `FX-001` | Defined. |",
                ["FX-001"],
            ),
            ("A reference to `FX-001`.", []),
            ("| FX-001 | orphan |", []),
            ("- `FX-001`:", []),
            ("- `FX-001`： <!-- only a comment -->", []),
            ("    - `FX-001`: Indented code.", []),
            ("```md\n- `FX-001`: example\n```", []),
            ("- ```md\n  - `FX-001`: example\n  ```", []),
            ("> ```md\n> - `FX-001`: example\n> ```", []),
            ("<!--\n- `FX-001`: comment\n-->", []),
            ("<!-- unclosed\n- `FX-001`: comment", []),
            ("`<!--`\n\n- `FX-001`: visible", ["FX-001"]),
            ("```\n<!--\n```\n\n- `FX-001`: visible", ["FX-001"]),
            (
                "- Example\n\n  ~~~\n  - `FX-001`: hidden\n  ~~~\n\n- `FX-002`: visible",
                ["FX-002"],
            ),
            ("- `example\n  FX-001: hidden`\n- `FX-002`: visible", ["FX-002"]),
        ]
        for body, expected in cases:
            with self.subTest(body=body):
                self.assertEqual([key for key, _ in k.definitions(body)], expected)

    def test_duplicate_yaml_keys_are_rejected_at_any_depth(self):
        for text in [
            "id: a\nid: b",
            "coverage:\n  - paths: []\n    paths: []",
            "? [a, b]\n: value",
        ]:
            with self.subTest(text=text), self.assertRaises(k.KnowledgeError):
                k.frontmatter("---\n" + text + "\n---\n")

    def test_yaml_cannot_construct_python_objects(self):
        with self.assertRaises(k.KnowledgeError):
            k.frontmatter("---\na: !!python/object:object {}\n---\n")

    def test_authority_uses_ast_not_examples(self):
        table = "| requirement | design | state | evidence or gap |\n| --- | --- | --- | --- |\n| FX-001 | DES-a | unknown | gap: missing |"
        self.assertEqual(
            k.authority("```\n" + table + "\n```\n\n" + table)[0][0], "FX-001"
        )
        for bad in [
            table + "\n\n" + table,
            table.replace("gap: missing", ""),
            table.replace("FX-001", "FX-001/FX-002"),
            table.replace("DES-a", "`DES-a"),
        ]:
            with self.subTest(bad=bad), self.assertRaises(k.KnowledgeError):
                k.authority(bad)


class KnowledgeTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.run_git("init", "-q")
        self.run_git("config", "user.name", "Knowledge tests")
        self.run_git("config", "user.email", "knowledge@example.invalid")
        for layer in k.LAYERS:
            self.write(
                "docs/knowledge/" + layer + "/README.md",
                "# Index\n\n" + k.INDEX_START + "\n" + k.INDEX_END + "\n",
            )
        self.write("src/a.py", "a = 1\n")
        self.write("src/b.py", "b = 1\n")
        self.write("src/shared.py", "shared = 1\n")
        self.document("INT-product", "intent", "approved", upstream=[])
        self.document(
            "SPEC-product",
            "spec",
            "approved",
            upstream=["INT-product"],
            body="- `FX-001`: Alpha.\n- `FX-002`: Beta.\n",
        )
        for name, requirement in [("a", "FX-001"), ("b", "FX-002")]:
            self.document("DES-" + name, "design", "active", tracks=[requirement])
            self.document(
                "IMP-" + name,
                "implementation",
                "active",
                code_paths=["src/" + name + ".py", "src/shared.py"],
                body=self.row(requirement, "DES-" + name),
            )
        self.observed = self.commit("source and mappings")
        self.document(
            "EVD-proof",
            "evidence",
            "active",
            observed_commit=self.observed,
            result="passed",
            scope=["unit"],
            commands=["test fixture"],
            coverage=[
                dict(requirements=["FX-001"], paths=["src/a.py", "src/shared.py"]),
                dict(requirements=["FX-002"], paths=["src/b.py", "src/shared.py"]),
            ],
        )
        self.index()
        self.commit("record result")

    def write(self, path, text):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(text)
        return target

    def run_git(self, *args):
        return subprocess.run(
            ["git", "-C", str(self.root), *args],
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()

    def commit(self, message):
        self.run_git("add", ".")
        self.run_git("commit", "-qm", message)
        return self.run_git("rev-parse", "HEAD")

    def document(self, identity, layer, status, body="# Page\n", **fields):
        meta = dict(
            id=identity,
            layer=layer,
            title=identity,
            status=status,
            owner="human" if layer in {"intent", "spec"} else "agent",
            updated_at="2026-09-07",
            **fields,
        )
        return self.write(
            "docs/knowledge/" + layer + "/" + identity + ".md",
            "---\n" + k.yaml.safe_dump(meta, sort_keys=False) + "---\n" + body,
        )

    def change(self, identity, update):
        doc = k.Snapshot(self.root).documents[identity]
        meta = doc["meta"]
        update(meta)
        self.write(
            doc["path"],
            "---\n" + k.yaml.safe_dump(meta, sort_keys=False) + "---\n" + doc["body"],
        )

    def row(self, requirement, design, state="aligned"):
        detail = "EVD-proof" if state == "aligned" else "gap: validation pending"
        return (
            "| requirement | design | state | evidence or gap |\n| --- | --- | --- | --- |\n"
            + f"| {requirement} | {design} | {state} | {detail} |\n"
        )

    def state(self, name, state):
        doc = k.Snapshot(self.root).documents["IMP-" + name]
        meta = doc["meta"]
        self.write(
            doc["path"],
            "---\n"
            + k.yaml.safe_dump(meta, sort_keys=False)
            + "---\n"
            + self.row("FX-001" if name == "a" else "FX-002", "DES-" + name, state),
        )
        self.index()

    def index(self):
        graph = k.Knowledge(self.root)
        graph.rows = {
            d["id"]: k.authority(d["body"])
            for d in graph.snapshot.documents.values()
            if d["layer"] == "implementation" and d["meta"]["status"] == "active"
        }
        graph.indexes(write=True)

    def check(self):
        k.Knowledge(self.root).validate()

    def test_valid_graph_and_derived_indexes(self):
        self.check()
        self.assertIn(
            "active / aligned",
            (self.root / "docs/knowledge/implementation/README.md").read_text(),
        )

    def test_multiple_designs_can_track_one_requirement(self):
        self.document("DES-other", "design", "active", tracks=["FX-001"])
        self.index()
        self.check()

    def test_duplicate_implementation_owner_is_rejected(self):
        self.document(
            "IMP-other",
            "implementation",
            "active",
            code_paths=["src/a.py"],
            body=self.row("FX-001", "DES-a"),
        )
        with self.assertRaisesRegex(k.KnowledgeError, "duplicate IMP owner"):
            self.check()

    def test_missing_implementation_owner_is_rejected(self):
        self.change("IMP-a", lambda meta: meta.update(status="retired"))
        with self.assertRaisesRegex(k.KnowledgeError, "no current IMP owner"):
            self.check()

    def test_design_must_track_approved_requirement(self):
        self.change("DES-a", lambda meta: meta.update(tracks=["FX-999"]))
        with self.assertRaisesRegex(k.KnowledgeError, "unknown approved requirement"):
            self.check()

    def test_spec_cannot_reference_design(self):
        self.change("SPEC-product", lambda meta: meta.update(upstream=["DES-a"]))
        with self.assertRaisesRegex(k.KnowledgeError, "INT upstream"):
            self.check()

    def test_approved_spec_cannot_use_draft_intent(self):
        self.change("INT-product", lambda meta: meta.update(status="draft"))
        with self.assertRaisesRegex(k.KnowledgeError, "approved INT"):
            self.check()

    def test_derived_fields_are_not_writable(self):
        for key in ["tracks", "upstream", "evidence", "observed_commit"]:
            with self.subTest(key=key):
                self.change("IMP-a", lambda meta: meta.update({key: []}))
                with self.assertRaisesRegex(k.KnowledgeError, "derived IMP"):
                    self.check()
                self.change("IMP-a", lambda meta: meta.pop(key))

    def test_changed_group_does_not_invalidate_other_group(self):
        self.write("src/a.py", "a = 2\n")
        self.state("a", "unknown")
        self.check()
        self.state("a", "aligned")
        with self.assertRaisesRegex(k.KnowledgeError, "FX-001.*stale input"):
            self.check()

    def test_unused_stale_evidence_does_not_block(self):
        self.write("src/shared.py", "shared = 2\n")
        self.state("a", "unknown")
        self.state("b", "diverged")
        self.check()

    def test_shared_input_invalidates_each_consumer(self):
        self.write("src/shared.py", "shared = 2\n")
        self.state("a", "unknown")
        with self.assertRaisesRegex(k.KnowledgeError, "FX-002.*stale input"):
            self.check()

    def test_committed_staged_and_deleted_inputs_invalidate(self):
        self.write("src/a.py", "a = 2\n")
        self.run_git("add", "src/a.py")
        with self.assertRaisesRegex(k.KnowledgeError, "stale input"):
            self.check()
        self.commit("changed input")
        with self.assertRaisesRegex(k.KnowledgeError, "stale input"):
            self.check()
        (self.root / "src/a.py").unlink()
        with self.assertRaisesRegex(k.KnowledgeError, "code path missing"):
            self.check()

    def test_untracked_input_under_observed_directory_invalidates(self):
        for identity in ["IMP-a", "IMP-b"]:
            self.change(identity, lambda meta: meta.update(code_paths=["src"]))
        self.observed = self.commit("directory inputs")
        self.change(
            "EVD-proof",
            lambda meta: meta.update(
                observed_commit=self.observed,
                coverage=[dict(requirements=["FX-001", "FX-002"], paths=["src"])],
            ),
        )
        self.index()
        self.check()
        self.write("src/untracked.py", "new = True\n")
        with self.assertRaisesRegex(k.KnowledgeError, "stale input"):
            self.check()

    def test_staged_only_input_cannot_hide_behind_restored_worktree(self):
        self.write("src/a.py", "a = 2\n")
        self.run_git("add", "src/a.py")
        self.write("src/a.py", "a = 1\n")
        with self.assertRaisesRegex(k.KnowledgeError, "stale input"):
            self.check()

    def test_staged_only_requirement_change_invalidates(self):
        path = self.root / "docs/knowledge/spec/SPEC-product.md"
        original = path.read_text()
        path.write_text(original.replace("Alpha.", "Changed Alpha."))
        self.run_git("add", str(path))
        path.write_text(original)
        with self.assertRaisesRegex(k.KnowledgeError, "stale staged requirement"):
            self.check()

    def test_requirement_change_invalidates_only_its_group(self):
        path = self.root / "docs/knowledge/spec/SPEC-product.md"
        path.write_text(path.read_text().replace("Alpha.", "Changed Alpha."))
        self.state("a", "unknown")
        self.check()
        self.state("a", "aligned")
        with self.assertRaisesRegex(k.KnowledgeError, "stale requirement definition"):
            self.check()

    def test_metadata_only_changes_do_not_invalidate(self):
        self.change("IMP-a", lambda meta: meta.update(title="Better title"))
        self.write("docs/knowledge/README.md", "# Router\n")
        self.check()

    def test_narrowing_both_current_and_evidence_paths_is_rejected(self):
        self.change("IMP-a", lambda meta: meta.update(code_paths=["src/a.py"]))
        self.change(
            "EVD-proof", lambda meta: meta["coverage"][0].update(paths=["src/a.py"])
        )
        with self.assertRaisesRegex(k.KnowledgeError, "paths narrowed"):
            self.check()

    def test_new_input_requires_new_evidence(self):
        self.write("src/new.py", "new = True\n")
        self.change("IMP-a", lambda meta: meta["code_paths"].append("src/new.py"))
        with self.assertRaisesRegex(k.KnowledgeError, "new implementation input"):
            self.check()

    def test_partial_evidence_cannot_align_and_may_have_unknown_paths(self):
        self.change(
            "EVD-proof",
            lambda meta: meta.update(
                result="partial",
                coverage=[dict(requirements=["FX-001", "FX-002"], paths=[])],
            ),
        )
        with self.assertRaisesRegex(k.KnowledgeError, "not active/passed"):
            self.check()
        self.state("a", "unknown")
        self.state("b", "unknown")
        self.check()

    def test_evidence_metadata_is_strict(self):
        cases = [
            ("commands", []),
            ("scope", ["imaginary"]),
            ("observed_commit", "HEAD"),
            ("observed_commit", "f" * 40),
            ("coverage", []),
            ("artifacts", ["/tmp/result.png"]),
        ]
        original = k.Snapshot(self.root).documents["EVD-proof"]["meta"]
        for key, value in cases:
            with self.subTest(key=key, value=value):
                self.change("EVD-proof", lambda meta: meta.update({key: value}))
                with self.assertRaises(k.KnowledgeError):
                    self.check()
                self.change(
                    "EVD-proof", lambda meta: (meta.clear(), meta.update(original))
                )

    def test_unreachable_evidence_commit_is_rejected(self):
        blob = self.run_git("rev-parse", "HEAD^{tree}")
        unreachable = self.run_git("commit-tree", blob, "-m", "unreachable")
        self.change("EVD-proof", lambda meta: meta.update(observed_commit=unreachable))
        with self.assertRaisesRegex(k.KnowledgeError, "ancestor"):
            self.check()

    def test_invalid_metadata_and_paths(self):
        original = k.Snapshot(self.root).documents["IMP-a"]["meta"]
        for values in [
            dict(title=" "),
            dict(owner="human"),
            dict(updated_at="2026-02-30"),
            dict(status="aligned"),
            dict(code_paths=["../outside"]),
            dict(code_paths=["src/a.py", "src/a.py"]),
        ]:
            with self.subTest(values=values):
                self.change("IMP-a", lambda meta: meta.update(values))
                with self.assertRaises(k.KnowledgeError):
                    self.check()
                self.change("IMP-a", lambda meta: (meta.clear(), meta.update(original)))

    def test_missing_observed_input_is_rejected(self):
        self.change(
            "EVD-proof",
            lambda meta: meta["coverage"][0].update(paths=["src/nonexistent.py"]),
        )
        with self.assertRaisesRegex(k.KnowledgeError, "did not exist"):
            self.check()

    def test_index_check_never_rewrites_files(self):
        path = self.root / "docs/knowledge/implementation/README.md"
        path.write_text(path.read_text().replace("active / aligned", "wrong"))
        before = path.read_bytes()
        with self.assertRaisesRegex(k.KnowledgeError, "index drift"):
            self.check()
        self.assertEqual(before, path.read_bytes())
        self.index()
        self.check()

    def test_hidden_links_do_not_need_to_exist(self):
        self.write(
            "docs/knowledge/README.md",
            "# Router\n\n```\n[x](missing.md)\n```\n<!-- [x](missing.md) -->\n",
        )
        self.check()
        self.write("docs/knowledge/README.md", "[x](missing.md)\n")
        with self.assertRaisesRegex(k.KnowledgeError, "broken local link"):
            self.check()

    def test_snapshot_export_is_stable_and_ignores_worktree_changes(self):
        first = k.Snapshot(self.root, self.observed).export()
        self.write("docs/knowledge/spec/SPEC-product.md", "not valid current YAML\n")
        second = k.Snapshot(self.root, self.observed).export()
        self.assertEqual(first, second)
        self.assertEqual(first["schema_version"], 1)
        self.assertEqual(len(first["requirements"]), 2)

    def test_nested_formal_pages_fail_in_working_and_historical_snapshots(self):
        self.write("docs/knowledge/spec/nested/SPEC-hidden.md", "# hidden\n")
        with self.assertRaisesRegex(k.KnowledgeError, "direct layer children"):
            k.Snapshot(self.root)
        commit = self.commit("nested page")
        with self.assertRaisesRegex(k.KnowledgeError, "direct layer children"):
            k.Snapshot(self.root, commit)

    def test_legacy_nested_evidence_is_not_exported(self):
        self.write("docs/knowledge/implementation/evidence/EVD-legacy.md", "# legacy\n")
        commit = self.commit("legacy evidence")
        self.assertNotIn("EVD-legacy", k.Snapshot(self.root, commit).documents)

    def test_historical_inputs_unknown_cannot_support_aligned(self):
        self.change("IMP-a", lambda meta: meta.pop("code_paths"))
        observed = self.commit("historical inputs unavailable")
        self.change(
            "IMP-a", lambda meta: meta.update(code_paths=["src/a.py", "src/shared.py"])
        )
        self.change("EVD-proof", lambda meta: meta.update(observed_commit=observed))
        with self.assertRaisesRegex(
            k.KnowledgeError, "historical implementation inputs unknown"
        ):
            self.check()

    def test_check_is_idempotent_and_read_only(self):
        before = {
            p.relative_to(self.root): p.read_bytes()
            for p in (self.root / "docs").rglob("*.md")
        }
        self.check()
        self.check()
        after = {
            p.relative_to(self.root): p.read_bytes()
            for p in (self.root / "docs").rglob("*.md")
        }
        self.assertEqual(before, after)
        self.assertEqual(self.run_git("status", "--porcelain"), "")

    def test_source_and_formal_page_cannot_escape_through_symlink(self):
        with tempfile.TemporaryDirectory() as outside:
            external = Path(outside) / "file.md"
            external.write_text("# outside\n")
            (self.root / "src/escape").symlink_to(external)
            self.change("IMP-a", lambda meta: meta.update(code_paths=["src/escape"]))
            with self.assertRaisesRegex(k.KnowledgeError, "outside repository"):
                self.check()
            (self.root / "docs/knowledge/spec/SPEC-escape.md").symlink_to(external)
            with self.assertRaisesRegex(k.KnowledgeError, "escapes repository"):
                k.Snapshot(self.root)

    def test_export_rejects_invalid_snapshot_metadata(self):
        self.change("SPEC-product", lambda meta: meta.update(status=["approved"]))
        observed = self.commit("malformed metadata")
        with self.assertRaisesRegex(k.KnowledgeError, "invalid lifecycle"):
            k.Snapshot(self.root, observed).export()

    def test_generated_indexes_expose_both_directions(self):
        implementation = (
            self.root / "docs/knowledge/implementation/README.md"
        ).read_text()
        evidence = (self.root / "docs/knowledge/evidence/README.md").read_text()
        design = (self.root / "docs/knowledge/design/README.md").read_text()
        self.assertIn("[EVD-proof](../evidence/EVD-proof.md)", implementation)
        self.assertIn("[IMP-a](../implementation/IMP-a.md)", evidence)
        self.assertIn("[SPEC-product](../spec/SPEC-product.md)", design)

    def test_historical_metadata_requires_no_current_schema_migration(self):
        self.change(
            "IMP-a",
            lambda meta: meta.update(
                status="aligned",
                tracks=["FX-001"],
                upstream=["DES-a"],
                evidence=["EVD-proof"],
            ),
        )
        historical = self.commit("old shape")
        self.assertIn(
            "IMP-a",
            {d["id"] for d in k.Snapshot(self.root, historical).export()["documents"]},
        )

    def test_external_references_are_versioned_and_repository_typed(self):
        valid = f"little-white-box-front@{self.observed}:FX-001"
        self.assertEqual(
            k.external(dict(external_upstream=[valid]), "DES-a")[0]["target_id"],
            "FX-001",
        )
        for value in [
            [],
            [valid, valid],
            ["repo@HEAD:FX-001"],
            [valid.replace("FX-001", "CORE-001")],
        ]:
            with self.subTest(value=value), self.assertRaises(k.KnowledgeError):
                k.external(dict(external_upstream=value), "DES-a")


if __name__ == "__main__":
    unittest.main()
