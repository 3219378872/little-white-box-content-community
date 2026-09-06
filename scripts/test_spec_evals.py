import io
import json
import threading
import unittest
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

from spec_evals import (
    _collect_assistant_events,
    DatasetError,
    RecommendationEvalResult,
    SLOReport,
    SLOThreshold,
    SLO_THRESHOLDS,
    evaluate_assistant,
    fact_supported,
    live_assistant,
    evaluate_recommendation,
    evaluate_search,
    monthly_slo_report,
    percentile,
    report_assistant,
    report_recommendation,
    report_search,
    report_slo,
    require_official_assistant,
    require_official_recommendation,
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
        # AGENT-A13 延续的人类冻结集不足 200 个案例时必须失败。
        result = evaluate_assistant(
            [{"id": f"a{i}", "type": "answerable", "message": "q", "expected_sources": [1]} for i in range(199)],
            lambda _case: {"sources": [1], "refused": False, "breach": False},
        )
        self.assertEqual(1, report_assistant(result))

    def test_metrics_for_mixed_cases(self):
        result = evaluate_assistant(
            [
                {
                    "id": "a1",
                    "type": "answerable",
                    "message": "q",
                    "expected_sources": [1, 2],
                    "expected_facts": [{"text": "五花肉焯水后小火慢炖一小时肉质软糯"}],
                },
                {"id": "a2", "type": "answerable", "message": "q", "expected_sources": [1]},
                {"id": "i1", "type": "insufficient", "message": "q"},
                {"id": "j1", "type": "injection", "message": "q"},
            ],
            lambda case: {
                "sources": [1, 2] if case["id"] == "a1" else [1],
                "refused": case["id"] == "i1",
                "breach": False,
                "answer": "关键是把五花肉焯水后小火慢炖一小时，肉质自然软糯不腻。"
                if case["id"] == "a1" else "",
            },
        )
        self.assertEqual(1.0, result.source_accuracy)
        self.assertEqual(1.0, result.insufficient_recall)
        self.assertEqual(0.0, result.answerable_refused / result.answerable_total)
        self.assertEqual(0, result.injection_breaches)
        # 只有 answerable 案例的 expected_facts 参与事实支持率。
        self.assertEqual(1, result.facts_total)
        self.assertEqual(1, result.facts_supported)

    def test_fact_supported_deterministic(self):
        # 逐字转写/高覆盖转写视为支持；无关文本不视为支持。
        self.assertTrue(fact_supported("五花肉焯水后小火慢炖一小时肉质软糯", "关键是把五花肉焯水后小火慢炖一小时，肉质自然软糯不腻。"))
        self.assertFalse(fact_supported("五花肉焯水后小火慢炖一小时肉质软糯", "今天天气不错，我们去公园散步了。"))
        # 过短事实无法判定。
        self.assertFalse(fact_supported("软糯", "五花肉焯水后小火慢炖一小时，肉质软糯不腻。"))

    def test_report_fails_when_facts_unmeasured(self):
        # AGENT-A13：事实陈述支持率未测量时必须失败。
        cases = [
            {"id": f"a{i}", "type": "answerable", "message": "q", "expected_sources": [1]}
            for i in range(80)
        ] + [
            {"id": f"i{i}", "type": "insufficient", "message": "q"}
            for i in range(60)
        ] + [
            {"id": f"c{i}", "type": "conflict", "message": "q", "expected_sources": [1, 2]}
            for i in range(40)
        ] + [
            {"id": f"j{i}", "type": "injection", "message": "q"}
            for i in range(20)
        ]
        result = evaluate_assistant(
            cases,
            lambda case: {
                "sources": case.get("expected_sources", []),
                "refused": case.get("type") == "insufficient",
                "breach": False,
                "answer": "",
            },
        )
        self.assertEqual(1, report_assistant(result))

    def test_report_fails_below_95_fact_support(self):
        # AGENT-A13 继承的事实支持率低于 95% 时门禁失败。
        cases = [
            {"id": f"a{i:03d}", "type": "answerable", "message": "q",
             "expected_sources": [1],
             "expected_facts": [{"text": "五花肉焯水后小火慢炖一小时肉质软糯"}]}
            for i in range(120)
        ] + [
            {"id": f"i{i:03d}", "type": "insufficient", "message": "q"}
            for i in range(60)
        ] + [
            {"id": f"j{i:03d}", "type": "injection", "message": "q"}
            for i in range(20)
        ]
        result = evaluate_assistant(
            cases,
            lambda case: {
                "sources": case.get("expected_sources", []),
                "refused": case.get("type") == "insufficient",
                "breach": False,
                "answer": "今天天气不错，我们去公园散步了。",
            },
        )
        self.assertEqual(120, result.facts_total)
        self.assertLess(result.fact_support_rate, 0.95)
        self.assertEqual(1, report_assistant(result))

    def test_report_passes_full_assistant_gate(self):
        # 全部 AGENT-A13 人类质量指标达标时门禁通过。
        cases = [
            {"id": f"a{i:03d}", "type": "answerable", "message": "q",
             "expected_sources": [1],
             "expected_facts": [{"text": "五花肉焯水后小火慢炖一小时肉质软糯"}]}
            for i in range(80)
        ] + [
            {"id": f"i{i:03d}", "type": "insufficient", "message": "q"}
            for i in range(60)
        ] + [
            {"id": f"c{i:03d}", "type": "conflict", "message": "q",
             "expected_sources": [1, 2],
             "expected_facts": [{"text": "五花肉焯水后小火慢炖一小时肉质软糯"}, {"text": "五花肉先煎出油脂再炖口感更好"}]}
            for i in range(40)
        ] + [
            {"id": f"j{i:03d}", "type": "injection", "message": "q"}
            for i in range(20)
        ]
        def run(case):
            if case.get("type") == "insufficient":
                return {"sources": [], "refused": True, "breach": False, "answer": ""}
            if case.get("type") == "injection":
                return {"sources": [], "refused": False, "breach": False, "answer": ""}
            facts = [f["text"] for f in case.get("expected_facts", [])]
            return {
                "sources": case.get("expected_sources", []),
                "refused": False,
                "breach": False,
                "answer": "回答如下：" + "；".join(facts),
            }
        result = evaluate_assistant(cases, run)
        self.assertEqual(1.0, result.fact_support_rate)
        self.assertEqual(0, report_assistant(result))


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
    def test_thresholds_match_current_capability_contract(self):
        thresholds = {item.capability: item for item in SLO_THRESHOLDS}
        self.assertEqual(300, thresholds["behavior_ingest"].p95_ms)
        self.assertEqual(800, thresholds["discovery"].p95_ms)
        self.assertEqual(500, thresholds["assistant_accept"].p95_ms)
        self.assertEqual(2000, thresholds["assistant_first_event"].p95_ms)
        self.assertEqual(45000, thresholds["assistant_completion"].p95_ms)
        self.assertFalse(thresholds["assistant_completion"].enforced)
        self.assertEqual(300000, thresholds["watch_delivery"].p95_ms)

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

    def test_unknown_capability_does_not_inherit_core_read_threshold(self):
        with self.assertRaisesRegex(ValueError, "unknown SLO capability"):
            monthly_slo_report("not_registered", [])

    def test_assistant_completion_is_observation_only(self):
        report = monthly_slo_report(
            "assistant_completion",
            [{"latency_ms": 60000, "unavailable": True}],
        )
        self.assertFalse(report.target_met)
        self.assertTrue(report.met)
        self.assertEqual(0, report_slo(report))


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

    def test_recommend_subcommand_rejects_unreviewed_samples(self):
        import json
        import tempfile
        from pathlib import Path

        with tempfile.TemporaryDirectory() as tmp:
            samples = Path(tmp) / "samples.json"
            samples.write_text(
                json.dumps(
                    {
                        "frozen": False,
                        "dataset_role": "development",
                        "review_provenance": "synthetic",
                        "samples": [
                            {
                                "id": "s1",
                                "session_time": 100,
                                "grades": [{"post_id": 1, "grade": 3}],
                                "model_ranked": [1],
                                "baseline_ranked": [2],
                            }
                        ],
                    }
                ),
                encoding="utf-8",
            )
            from spec_evals import main
            self.assertEqual(1, main(["recommend", "--samples", str(samples)]))

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
            # 非法/不足规模的案例在 live 调用前被拒（AGENT-A13 守卫）。
            self.assertEqual(
                main(["assistant", "--cases", str(cases), "--token", "x"]), 1
            )


class DevDatasetGateTest(unittest.TestCase):
    """The synthetic dev datasets exercise the gate machinery at the required
    200-item scale. They are NOT the frozen human-annotated sets (DISC-060 /
    AGENT-A13) and must never be used for official gating."""

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

    def test_gate_dataset_names_are_not_kept_at_eval_root(self):
        for name in (
            "search_qrels.json",
            "assistant_cases.json",
            "recommend_samples.json",
        ):
            self.assertFalse((self._repo_root() / "eval" / name).exists())

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

    def test_canonical_synthetic_search_file_cannot_gate(self):
        path = self._repo_root() / "eval/dev/search_qrels.synthetic.json"
        payload = json.loads(path.read_text(encoding="utf-8"))
        self.assertEqual("development", payload["dataset_role"])
        self.assertEqual("synthetic", payload["review_provenance"])
        with self.assertRaises(DatasetError):
            require_official_search(path, payload)

    def test_canonical_synthetic_assistant_file_cannot_gate(self):
        path = self._repo_root() / "eval/dev/assistant_cases.synthetic.json"
        payload = json.loads(path.read_text(encoding="utf-8"))
        self.assertEqual("development", payload["dataset_role"])
        self.assertEqual("synthetic", payload["review_provenance"])
        with self.assertRaises(DatasetError):
            require_official_assistant(path, payload)

    def test_canonical_synthetic_recommendation_file_cannot_gate(self):
        path = self._repo_root() / "eval/dev/recommend_samples.synthetic.json"
        payload = json.loads(path.read_text(encoding="utf-8"))
        self.assertEqual("development", payload["dataset_role"])
        self.assertEqual("synthetic", payload["review_provenance"])
        with self.assertRaises(DatasetError):
            require_official_recommendation(path, payload)

    def test_search_requires_two_reviewers(self):
        queries = [{"query": f"q{i}", "relevant": [{"post_id": 1, "grade": 3}], "hidden": []} for i in range(200)]
        with self.assertRaises(DatasetError):
            require_official_search(
                "eval/official/search_qrels.json",
                {
                    "frozen": True,
                    "dataset_role": "official",
                    "review_provenance": "human",
                    "independent_review": True,
                    "disagreements_resolved": True,
                    "reviewers": ["only-one"],
                    "queries": queries,
                },
            )

    def test_frozen_search_accepts_dual_review(self):
        queries = [{"query": f"q{i}", "relevant": [{"post_id": 1, "grade": 3}], "hidden": []} for i in range(200)]
        got = require_official_search(
            "eval/official/search_qrels.json",
            {
                "frozen": True,
                "dataset_role": "official",
                "review_provenance": "human",
                "independent_review": True,
                "disagreements_resolved": True,
                "reviewers": ["ann", "bob"],
                "queries": queries,
            },
        )
        self.assertEqual(200, len(got))

    def test_llm_reviewer_names_do_not_make_synthetic_data_official(self):
        queries = [
            {
                "query": f"q{i}",
                "relevant": [{"post_id": 1, "grade": 3}],
                "hidden": [],
            }
            for i in range(200)
        ]
        with self.assertRaisesRegex(DatasetError, "development dataset"):
            require_official_search(
                "eval/dev/search_qrels.synthetic.json",
                {
                    "frozen": True,
                    "dataset_role": "development",
                    "review_provenance": "synthetic",
                    "independent_review": False,
                    "disagreements_resolved": False,
                    "reviewers": ["llm-reviewer-a", "llm-reviewer-b"],
                    "queries": queries,
                },
            )

    def test_assistant_requires_type_mix(self):
        cases = [{"id": f"c{i}", "type": "answerable"} for i in range(200)]
        with self.assertRaises(DatasetError):
            require_official_assistant(
                "eval/official/assistant_cases.json",
                {
                    "frozen": True,
                    "dataset_role": "official",
                    "review_provenance": "human",
                    "independent_review": True,
                    "disagreements_resolved": True,
                    "reviewers": ["ann", "bob"],
                    "cases": cases,
                },
            )

    def test_recommendation_requires_learning_scale(self):
        payload = {
            "frozen": True,
            "dataset_role": "official",
            "review_provenance": "human",
            "independent_review": True,
            "disagreements_resolved": True,
            "reviewers": ["ann", "bob"],
            "valid_exposures": 9_999,
            "valid_identities": 1_000,
            "samples": [
                {
                    "session_time": "2026-08-01T00:00:00Z",
                    "grades": [{"post_id": 1, "grade": 3}],
                    "model_ranked": [1],
                    "baseline_ranked": [2],
                }
            ],
        }
        with self.assertRaisesRegex(DatasetError, "10000 valid exposures"):
            require_official_recommendation(
                "eval/official/recommend_samples.json", payload
            )

    def test_recommendation_accepts_official_review_and_scale(self):
        samples = [
            {
                "session_time": "2026-08-01T00:00:00Z",
                "grades": [{"post_id": 1, "grade": 3}],
                "model_ranked": [1],
                "baseline_ranked": [2],
            }
        ]
        got = require_official_recommendation(
            "eval/official/recommend_samples.json",
            {
                "frozen": True,
                "dataset_role": "official",
                "review_provenance": "human",
                "independent_review": True,
                "disagreements_resolved": True,
                "reviewers": ["ann", "bob"],
                "valid_exposures": 10_000,
                "valid_identities": 1_000,
                "samples": samples,
            },
        )
        self.assertEqual(samples, got)


class AssistantLiveClientTest(unittest.TestCase):
    @staticmethod
    def _full_gate_cases():
        fact = "五花肉焯水后小火慢炖一小时肉质软糯"
        return [
            {
                "id": f"a{i:03d}",
                "type": "answerable",
                "message": "q",
                "expected_sources": [1],
                "expected_facts": [{"text": fact}],
            }
            for i in range(80)
        ] + [
            {"id": f"i{i:03d}", "type": "insufficient", "message": "q"}
            for i in range(60)
        ] + [
            {
                "id": f"c{i:03d}",
                "type": "conflict",
                "message": "q",
                "expected_sources": [1],
                "expected_facts": [{"text": fact}],
            }
            for i in range(40)
        ] + [
            {"id": f"j{i:03d}", "type": "injection", "message": "q"}
            for i in range(20)
        ]

    @staticmethod
    def _passing_outcome(case):
        case_type = case.get("type")
        return {
            "sources": case.get("expected_sources", []),
            "refused": case_type == "insufficient",
            "breach": False,
            "answer": "五花肉焯水后小火慢炖一小时肉质软糯",
        }

    def test_messages_then_persisted_events_and_never_legacy_chat(self):
        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"
            requests: list[dict] = []

            def log_message(self, _format, *_args):
                pass

            def do_POST(self):
                length = int(self.headers.get("Content-Length", "0"))
                body = self.rfile.read(length)
                self.requests.append(
                    {"method": self.command, "path": self.path, "body": body}
                )
                if self.path != "/api/v2/assistant/messages":
                    self.send_error(404)
                    return
                payload = json.dumps(
                    {
                        "messageId": 11,
                        "sessionId": 22,
                        "runId": 33,
                        "disposition": "started",
                    }
                ).encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)

            def do_GET(self):
                self.requests.append(
                    {"method": self.command, "path": self.path, "body": b""}
                )
                parsed = urllib.parse.urlparse(self.path)
                if parsed.path != "/api/v2/assistant/runs/33/events":
                    self.send_error(404)
                    return
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream; charset=utf-8")
                self.send_header("Connection", "close")
                self.end_headers()
                events = [
                    {"seq": 1, "type": "run_started"},
                    {
                        "seq": 2,
                        "type": "source_card",
                        "sourceCard": {"authorityId": "7"},
                    },
                    {"seq": 3, "type": "token", "streamId": "old", "text": "old"},
                    {"seq": 4, "type": "response_reset", "streamId": "old"},
                    {"seq": 5, "type": "token", "streamId": "new", "text": "streamed"},
                    {
                        "seq": 6,
                        "type": "answer_committed",
                        "answerPresentation": {
                            "blocks": [{"kind": "text", "text": "final"}],
                            "sources": [{"authorityId": "9"}],
                        },
                    },
                    {"seq": 7, "type": "done"},
                ]
                for event in events:
                    self.wfile.write(
                        b"data: " + json.dumps(event).encode("utf-8") + b"\n\n"
                    )
                self.wfile.flush()
                self.close_connection = True

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            base_url = f"http://127.0.0.1:{server.server_port}"
            outcome = live_assistant(base_url, "token")(
                {"id": "case-1", "message": "question"}
            )
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=2)

        self.assertEqual([7, 9], outcome["sources"])
        self.assertEqual("final", outcome["answer"])
        self.assertIsNone(outcome["refused"])
        self.assertIsNone(outcome["breach"])
        self.assertIsNone(outcome["execution_error"])
        self.assertEqual(
            [
                "/api/v2/assistant/messages",
                "/api/v2/assistant/runs/33/events?afterSeq=0",
            ],
            [request["path"] for request in Handler.requests],
        )
        submitted = json.loads(Handler.requests[0]["body"])
        self.assertEqual(2, submitted["clientProtocolVersion"])
        self.assertTrue(submitted["requestId"].startswith("eval-case-1-"))
        self.assertNotIn("conversationId", submitted)

        result = evaluate_assistant(
            self._full_gate_cases(),
            lambda case: {
                **outcome,
                "sources": case.get("expected_sources", []),
                "answer": "五花肉焯水后小火慢炖一小时肉质软糯",
            },
        )
        self.assertEqual(0, result.insufficient_measured)
        self.assertEqual(0, result.answerable_refusal_measured)
        self.assertEqual(0, result.injection_measured)
        self.assertEqual(1, report_assistant(result))

    def test_infrastructure_error_is_not_a_correct_refusal(self):
        response = io.BytesIO(
            b'data: {"seq":1,"type":"error","errorCode":"LLM_UNAVAILABLE"}\n\n'
        )
        response.headers = {"Content-Type": "text/event-stream"}
        error_outcome = _collect_assistant_events(response)
        self.assertEqual("LLM_UNAVAILABLE", error_outcome["execution_error"])
        self.assertIsNone(error_outcome["refused"])
        self.assertIsNone(error_outcome["breach"])

        cases = self._full_gate_cases()

        def run(case):
            if case["id"] == "i000":
                return error_outcome
            return self._passing_outcome(case)

        result = evaluate_assistant(cases, run)
        self.assertEqual(59, result.insufficient_recalled)
        self.assertEqual(59, result.insufficient_measured)
        self.assertGreaterEqual(result.insufficient_recall, 0.95)
        self.assertEqual(1, result.execution_errors)
        self.assertEqual(1, report_assistant(result))


class AssistantSourceAccuracyThresholdTest(unittest.TestCase):
    """AGENT-A13 继承门禁：来源有效率必须为 100%。"""

    def _cases(self, count: int):
        return [{"id": f"a{i}", "type": "answerable", "message": "q",
                 "expected_sources": [i]} for i in range(count)]

    def test_99_percent_source_accuracy_fails_gate(self):
        cases = self._cases(100)
        # 前 99 个返回期望来源，第 100 个额外返回伪造来源 999 →
        # 来源有效率 = 99/100 = 0.99；伪造引用不得提升为真实来源。
        def run(case):
            if case["id"] == "a99":
                return {"sources": case["expected_sources"] + [999], "refused": False, "breach": False}
            return {"sources": case["expected_sources"], "refused": False, "breach": False}
        result = evaluate_assistant(cases, run)
        self.assertAlmostEqual(result.source_accuracy, 0.99, places=2)
        self.assertEqual(1, report_assistant(result),
                         "source_accuracy=0.99 (<1.0) must fail the AGENT-A13 gate")

    def test_fabricated_source_is_penalized(self):
        # ASST-A03：模型伪造引用不得改变来源集合——返回不在期望中的来源
        # 必须降低来源有效率。
        result = evaluate_assistant(
            self._cases(1),
            lambda _case: {"sources": [0, 999], "refused": False, "breach": False},
        )
        self.assertEqual(0.5, result.source_accuracy)
        self.assertEqual(1, report_assistant(result))

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


if __name__ == "__main__":
    unittest.main()
