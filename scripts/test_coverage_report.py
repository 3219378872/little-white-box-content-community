from __future__ import annotations

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).with_name("coverage_report.py")
SPEC = importlib.util.spec_from_file_location("coverage_report", SCRIPT)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError(f"cannot load {SCRIPT}")
coverage_report = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(coverage_report)


class CategoryTest(unittest.TestCase):
    def test_layers(self):
        cases = {
            "esx/app/content/rpc/internal/handler/routes.go": "generated",
            "esx/app/content/rpc/internal/logic/get_post_logic.go": "logic",
            "esx/app/content/rpc/internal/model/post_model.go": "model",
            "esx/app/content/mq/cleanup/internal/mqs/cleanup_consumer.go": "mq_consumer",
            "esx/app/content/rpc/internal/svc/service_context.go": "wiring",
            "esx/app/content/rpc/internal/server/content_service_server.go": "wiring",
            "esx/pkg/errx/codes.go": "shared",
            "errx/errors.go": "shared",
            "esx/app/gateway/internal/logic/posts/get_post_logic.go": "logic",
            "esx/app/content/rpc/pb/xiaobaihe/content/pb/content.pb.go": "generated",
            "esx/app/content/rpc/contentservice/content_service.go": "generated",
            "esx/app/content/rpc/internal/types/types.go": "generated",
        }
        for path, want in cases.items():
            self.assertEqual(coverage_report.category(path), want, path)

    def test_windows_paths_are_normalized(self):
        self.assertEqual(
            coverage_report.category(r"esx\app\content\rpc\internal\logic\get_post_logic.go"),
            "logic",
        )


class LoadProfilesTest(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.dir = Path(self.tempdir.name)

    def tearDown(self):
        self.tempdir.cleanup()

    def _write(self, name: str, lines: list[str]):
        (self.dir / name).write_text("\n".join(lines) + "\n", encoding="utf-8")

    def test_aggregates_by_layer(self):
        self._write("root.out", [
            "mode: set",
            "esx/app/content/rpc/internal/logic/a.go:10.13,15.2 5 1",
            "esx/app/content/rpc/internal/logic/b.go:10.13,15.2 3 0",
            "esx/pkg/errx/codes.go:10.13,15.2 4 1",
        ])
        totals = coverage_report.load_profiles(self.dir)
        self.assertEqual(totals["logic"], {"covered": 5, "statements": 8})
        self.assertEqual(totals["shared"], {"covered": 4, "statements": 4})

    def test_duplicate_block_union_covered(self):
        self._write("dup.out", [
            "mode: set",
            "esx/app/content/rpc/internal/logic/a.go:10.13,15.2 5 0",
            "esx/app/content/rpc/internal/logic/a.go:10.13,15.2 5 1",
        ])
        totals = coverage_report.load_profiles(self.dir)
        self.assertEqual(totals["logic"], {"covered": 5, "statements": 5})

    def test_inconsistent_block_raises(self):
        self._write("bad.out", [
            "mode: set",
            "esx/app/content/rpc/internal/logic/a.go:10.13,15.2 5 1",
            "esx/app/content/rpc/internal/logic/a.go:10.13,15.2 6 1",
        ])
        with self.assertRaises(ValueError):
            coverage_report.load_profiles(self.dir)


class AddPercentagesTest(unittest.TestCase):
    def test_percentages_and_handwritten(self):
        totals = {
            "logic": {"covered": 5, "statements": 8},
            "generated": {"covered": 10, "statements": 20},
        }
        coverage_report.add_percentages(totals)
        self.assertEqual(totals["logic"]["coverage"], 62.5)
        self.assertEqual(totals["handwritten"]["covered"], 5)
        self.assertEqual(totals["handwritten"]["statements"], 8)
        self.assertEqual(totals["handwritten"]["coverage"], 62.5)

    def test_zero_statements(self):
        totals = {"logic": {"covered": 0, "statements": 0}}
        coverage_report.add_percentages(totals)
        self.assertEqual(totals["logic"]["coverage"], 0.0)


class MainGateTest(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.dir = Path(self.tempdir.name)
        self.thresholds = self.dir / "thresholds.json"
        self.thresholds.write_text(
            json.dumps({
                "baseline": {"logic": 50.0, "handler": 80.0},
                "target": {"logic": 90.0, "handler": 95.0},
            }),
            encoding="utf-8",
        )

    def tearDown(self):
        self.tempdir.cleanup()

    def _profile(self, logic_covered: int, logic_total: int):
        # logic 层：covered 与 statements 由参数决定；handler 层始终全覆盖。
        b_stmts = logic_total - logic_covered
        (self.dir / "p.out").write_text(
            "mode: set\n"
            f"esx/app/content/rpc/internal/logic/a.go:1.1,2.1 {logic_covered} 1\n"
            f"esx/app/content/rpc/internal/logic/b.go:1.1,2.1 {b_stmts} 0\n"
            "esx/app/content/rpc/internal/handler/h.go:1.1,2.1 100 1\n",
            encoding="utf-8",
        )

    def test_gate_none_always_passes(self):
        self._profile(0, 220)
        self.assertEqual(
            coverage_report.main([str(self.dir), "--thresholds", str(self.thresholds), "--gate", "none"]),
            0,
        )

    def test_baseline_failure_returns_one(self):
        # logic 45.5% < 50% baseline -> fail。
        self._profile(100, 220)
        self.assertEqual(
            coverage_report.main([str(self.dir), "--thresholds", str(self.thresholds), "--gate", "baseline"]),
            1,
        )

    def test_baseline_pass_returns_zero(self):
        # logic 62.5% >= 50%、handler 100% >= 80% -> pass。
        self._profile(200, 320)
        self.assertEqual(
            coverage_report.main([str(self.dir), "--thresholds", str(self.thresholds), "--gate", "baseline"]),
            0,
        )


if __name__ == "__main__":
    unittest.main()
