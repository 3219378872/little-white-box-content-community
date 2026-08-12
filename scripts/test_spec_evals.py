import unittest

from spec_evals import evaluate_assistant, evaluate_search


class SearchEvalTest(unittest.TestCase):
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


if __name__ == "__main__":
    unittest.main()
