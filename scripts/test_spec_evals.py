import unittest
import json
from pathlib import Path

from spec_evals import (
    DatasetError,
    RecommendationEvalResult,
    SLOReport,
    SLOThreshold,
    evaluate_assistant,
    evaluate_recommendation,
    evaluate_search,
    monthly_slo_report,
    percentile,
    report_assistant,
    report_recommendation,
    report_search,
    report_slo,
    require_official_assistant,
    require_official_search,
    time_ordered_holdout,
)


class SearchEvalTest(unittest.TestCase):
    def test_report_fails_below_200_queries(self):
        # DISC-060：不足 200 条查询时门禁必须失败，即使指标达标。
        result = evaluate_search(
            [{"id": "q1", "query": "go", "relevant": [], "hidden": []}] * 199,
            lambda _query: [1],
        )
        self.assertEqual(1, report_search(result, 0.70))

    def test_ndcg_at_10_ranks_relevant_first(self):
        result = evaluate_search(
            [
                {
                    "id": "q1",
                    "query": "go",
                    "relevant": [{"post_id": 2, "grade": 3}, {"post_id": 1, "grade": 1}],
                    "hidden": [99],
                }
            ],
            lambda _query: [2, 1, 99],
        )
        self.assertEqual(1.0, result.ndcg_at_10)
        self.assertEqual(1, result.leakage)

    def test_ndcg_penalizes_missing_relevant(self):
        result = evaluate_search(
            [
                {
                    "id": "q1",
                    "query": "go",
                    "relevant": [{"post_id": 1, "grade": 3}, {"post_id": 2, "grade": 3}],
                    "hidden": [],
                }
            ],
            lambda _query: [1],
        )
        self.assertLess(result.ndcg_at_10, 1.0)


class AssistantEvalTest(unittest.TestCase):
    def test_report_fails_below_200_cases(self):
        # ASST-050：不足 200 个案例时门禁必须失败，即使指标达标。
        result = evaluate_assistant(
            [{"id": f"a{i}", "type": "answerable", "message": "q", "expected_sources": [1]} for i in range(199)],
            lambda _case: {"sources": [1], "refused": False, "breach": False},
        )
        self.assertEqual(1, report_assistant(result))

    def test_metrics_for_mixed_cases(self):
        result = evaluate_assistant(
            [
                {"id": "a1", "type": "answerable", "message": "q", "expected_sources": [1, 2]},
                {"id": "a2", "type": "answerable", "message": "q", "expected_sources": [1]},
                {"id": "i1", "type": "insufficient", "message": "q"},
                {"id": "j1", "type": "injection", "message": "q"},
            ],
            lambda case: {
                "sources": [1, 2] if case["id"] == "a1" else [1],
                "refused": case["id"] == "i1",
                "breach": False,
            },
        )
        self.assertEqual(1.0, result.source_accuracy)
        self.assertEqual(1.0, result.insufficient_recall)
        self.assertEqual(0.0, result.answerable_refused / result.answerable_total)
        self.assertEqual(0, result.injection_breaches)


class RecommendationEvalTest(unittest.TestCase):
    def test_time_ordered_holdout_preserves_chronology(self):
        samples = [
            {"id": "a", "session_time": 100},
            {"id": "b", "session_time": 50},
            {"id": "c", "session_time": 200},
        ]
        train, holdout = time_ordered_holdout(samples, ratio=0.5)
        self.assertEqual(["b"], [s["id"] for s in train])
        self.assertEqual(["a", "c"], [s["id"] for s in holdout])

    def test_relative_improvement_and_bootstrap_ci(self):
        samples = [
            {"id": str(index), "grades": [{"post_id": 1, "grade": 3}, {"post_id": 2, "grade": 3}]}
            for index in range(50)
        ]
        result = evaluate_recommendation(
            samples,
            lambda _s: ([1, 2], [2, 1]),
        )
        self.assertGreaterEqual(result.relative_improvement, 0.0)
        lower, upper = result.bootstrap_ci(seed=7, samples=200)
        self.assertLessEqual(lower, upper)


class SLOReportTest(unittest.TestCase):
    def test_monthly_slo_availability_and_p95(self):
        requests = []
        for index in range(100):
            requests.append({"latency_ms": 100 if index % 10 else 1000, "unavailable": index == 0})
        report = monthly_slo_report("community_core_read", requests)
        self.assertEqual(100, report.total)
        self.assertEqual(99, report.available)
        self.assertEqual(0.99, report.availability)
        self.assertGreaterEqual(report.p95_ms, 1000)

    def test_degraded_counts_available_and_refusal_excluded(self):
        requests = [
            {"latency_ms": 50, "degraded": True, "unavailable": False},
            {"latency_ms": 50, "correct_refusal": True, "unavailable": False},
            {"latency_ms": 200, "error_success": True, "unavailable": True},
        ]
        report = monthly_slo_report("discovery", requests)
        self.assertEqual(2, report.available)


class ReportFunctionTest(unittest.TestCase):
    """Direct coverage for percentile and the report return-code helpers."""

    def test_percentile_empty_returns_zero(self):
        self.assertEqual(0.0, percentile([], 95))

    def test_percentile_single_value(self):
        self.assertEqual(42.0, percentile([42.0], 95))

    def test_percentile_p95_index(self):
        values = [float(index) for index in range(100)]  # 0..99
        # p95 的 ceil(0.95*100)-1 = 94
        self.assertEqual(94.0, percentile(values, 95))

    def test_percentile_clamps_small_lists(self):
        self.assertEqual(3.0, percentile([1.0, 2.0, 3.0], 95))

    def test_report_slo_returns_zero_when_met(self):
        report = SLOReport(
            capability="community_core_read",
            total=100,
            available=100,
            p95_ms=50,
            threshold=SLOThreshold("community_core_read", 0.999, 300),
        )
        self.assertEqual(0, report_slo(report))

    def test_report_slo_returns_one_when_not_met(self):
        report = SLOReport(
            capability="community_core_read",
            total=100,
            available=99,
            p95_ms=50,
            threshold=SLOThreshold("community_core_read", 0.999, 300),
        )
        self.assertEqual(1, report_slo(report))

    def test_report_recommendation_returns_zero_when_passing(self):
        result = RecommendationEvalResult(
            case_count=40,
            model_ndcg_at_20_values=[0.2] * 40,
            baseline_ndcg_at_20_values=[0.1] * 40,
        )
        self.assertEqual(0, report_recommendation(result))

    def test_report_recommendation_returns_one_when_failing(self):
        result = RecommendationEvalResult(
            case_count=40,
            model_ndcg_at_20_values=[0.05] * 40,
            baseline_ndcg_at_20_values=[0.1] * 40,
        )
        self.assertEqual(1, report_recommendation(result))


class CLIDispatchTest(unittest.TestCase):
    """recommend/slo subcommands must dispatch to their own file inputs."""

    def test_recommend_subcommand_dispatches_to_samples(self):
        import json
        import tempfile
        from pathlib import Path

        with tempfile.TemporaryDirectory() as tmp:
            samples = Path(tmp) / "samples.json"
            samples.write_text(
                json.dumps({"samples": [{"id": "s1", "session_time": 100, "grades": []}]}),
                encoding="utf-8",
            )
            from spec_evals import main
            code = main(["recommend", "--samples", str(samples)])
            self.assertIsInstance(code, int)

    def test_slo_subcommand_dispatches_to_requests(self):
        import json
        import tempfile
        from pathlib import Path

        with tempfile.TemporaryDirectory() as tmp:
            requests = Path(tmp) / "requests.json"
            requests.write_text(
                json.dumps({"requests": [{"latency_ms": 100, "unavailable": False}]}),
                encoding="utf-8",
            )
            from spec_evals import main
            code = main(["slo", "--requests", str(requests), "--capability", "community_core_read"])
            self.assertIsInstance(code, int)

    def test_search_subcommand_rejects_invalid_qrels(self):
        import json
        import tempfile
        from pathlib import Path

        with tempfile.TemporaryDirectory() as tmp:
            qrels = Path(tmp) / "qrels.json"
            qrels.write_text(json.dumps({"queries": []}), encoding="utf-8")
            from spec_evals import main
            # 非法/不足规模的数据集在 live 调用前被拒（DISC-060 守卫）。
            self.assertEqual(main(["search", "--qrels", str(qrels)]), 1)

    def test_assistant_subcommand_rejects_invalid_cases(self):
        import json
        import tempfile
        from pathlib import Path

        with tempfile.TemporaryDirectory() as tmp:
            cases = Path(tmp) / "cases.json"
            cases.write_text(json.dumps({"cases": []}), encoding="utf-8")
            from spec_evals import main
            # 非法/不足规模的案例在 live 调用前被拒（ASST-050 守卫）。
            self.assertEqual(
                main(["assistant", "--cases", str(cases), "--token", "x"]), 1
            )


class DevDatasetGateTest(unittest.TestCase):
    """The synthetic dev datasets exercise the gate machinery at the required
    200-item scale. They are NOT the frozen human-annotated sets (DISC-060 /
    ASST-050) and must never be used for official gating."""

    def _repo_root(self):
        return Path(__file__).resolve().parent.parent

    def test_search_dev_dataset_reaches_required_scale(self):
        path = self._repo_root() / "eval/dev/search_qrels.dev.json"
        queries = json.loads(
            path.read_text(encoding="utf-8")
        )["queries"]
        self.assertGreaterEqual(len(queries), 200)
        by_query = {q["query"]: q for q in queries}
        result = evaluate_search(
            queries,
            lambda query: [r["post_id"] for r in by_query[query]["relevant"]]
            + by_query[query]["hidden"],
        )
        # 200 条查询满足规模门禁；故意注入 hidden 泄漏应使门禁失败。
        self.assertGreaterEqual(result.query_count, 200)
        self.assertGreater(result.leakage, 0)
        self.assertEqual(1, report_search(result, 0.70))

    def test_assistant_dev_dataset_reaches_required_scale(self):
        path = self._repo_root() / "eval/dev/assistant_cases.dev.json"
        cases = json.loads(
            path.read_text(encoding="utf-8")
        )["cases"]
        self.assertGreaterEqual(len(cases), 200)
        result = evaluate_assistant(
            cases,
            lambda case: {
                "sources": case.get("expected_sources", []),
                "refused": case["type"] == "insufficient",
                "breach": False,
            },
        )
        self.assertGreaterEqual(result.cases_total, 200)
        self.assertEqual(1.0, result.source_accuracy)
        self.assertEqual(1.0, result.insufficient_recall)
        self.assertEqual(0, result.injection_breaches)



class OfficialDatasetContractTest(unittest.TestCase):
    def _repo_root(self):
        return Path(__file__).resolve().parent.parent

    def test_dev_search_file_cannot_gate(self):
        path = self._repo_root() / "eval/dev/search_qrels.dev.json"
        payload = json.loads(path.read_text(encoding="utf-8"))
        with self.assertRaises(DatasetError):
            require_official_search(path, payload)

    def test_dev_assistant_file_cannot_gate(self):
        path = self._repo_root() / "eval/dev/assistant_cases.dev.json"
        payload = json.loads(path.read_text(encoding="utf-8"))
        with self.assertRaises(DatasetError):
            require_official_assistant(path, payload)

    def test_search_requires_two_reviewers(self):
        queries = [{"query": f"q{i}", "relevant": [{"post_id": 1, "grade": 3}], "hidden": []} for i in range(200)]
        with self.assertRaises(DatasetError):
            require_official_search(
                "eval/search_qrels.json",
                {"frozen": True, "reviewers": ["only-one"], "queries": queries},
            )

    def test_frozen_search_accepts_dual_review(self):
        queries = [{"query": f"q{i}", "relevant": [{"post_id": 1, "grade": 3}], "hidden": []} for i in range(200)]
        got = require_official_search(
            "eval/search_qrels.json",
            {"frozen": True, "reviewers": ["ann", "bob"], "queries": queries},
        )
        self.assertEqual(200, len(got))

    def test_assistant_requires_type_mix(self):
        cases = [{"id": f"c{i}", "type": "answerable"} for i in range(200)]
        with self.assertRaises(DatasetError):
            require_official_assistant(
                "eval/assistant_cases.json",
                {"frozen": True, "reviewers": ["ann", "bob"], "cases": cases},
            )



if __name__ == "__main__":
    unittest.main()


class AssistantSourceAccuracyThresholdTest(unittest.TestCase):
    """ASST-051：来源有效率必须为 100%（99% 也视为未达标）。"""

    def _cases(self, count: int):
        return [{"id": f"a{i}", "type": "answerable", "message": "q",
                 "expected_sources": [i]} for i in range(count)]

    def test_99_percent_source_accuracy_fails_gate(self):
        cases = self._cases(100)
        # 前 99 个命中 expected source，第 100 个 sources 为空 → accuracy=0.99。
        def run(case):
            if case["id"] == "a99":
                return {"sources": [], "refused": False, "breach": False}
            return {"sources": case["expected_sources"], "refused": False, "breach": False}
        result = evaluate_assistant(cases, run)
        self.assertAlmostEqual(result.source_accuracy, 0.99, places=2)
        self.assertEqual(1, report_assistant(result),
                         "source_accuracy=0.99 (<1.0) must fail the gate under ASST-051")

    def test_100_percent_source_accuracy_with_bad_recall_still_fails(self):
        cases = self._cases(100) + [
            {"id": "i0", "type": "insufficient", "message": "q", "expected_sources": []},
        ]
        def run(case):
            if case["type"] == "insufficient":
                return {"sources": [], "refused": False, "breach": False}  # 未拒答
            return {"sources": case["expected_sources"], "refused": False, "breach": False}
        result = evaluate_assistant(cases, run)
        self.assertEqual(1, report_assistant(result),
                         "insufficient recall <95% must fail even with 100% source accuracy")
