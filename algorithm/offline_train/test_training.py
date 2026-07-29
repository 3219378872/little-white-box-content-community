import json
import tempfile
import unittest
from pathlib import Path

from algorithm.offline_train.training import (
    FEATURE_NAMES,
    Evaluation,
    Sample,
    binary_auc,
    evaluate,
    time_split,
    validate_samples,
    write_model_metadata,
)


def sample(request_id, timestamp, post_id, label, category=""):
    return Sample(
        request_id=request_id,
        event_time_ms=timestamp,
        post_id=post_id,
        label=label,
        features={name: 0.5 for name in FEATURE_NAMES},
        category=category,
    )


class TrainingTest(unittest.TestCase):
    def test_training_data_quality_rejects_duplicate_candidate(self):
        samples = [
            sample("r1", 1, 1, 1),
            sample("r1", 2, 1, 0),
            sample("r2", 3, 2, 1),
            sample("r2", 4, 3, 0),
        ]
        with self.assertRaisesRegex(ValueError, "duplicate request/post"):
            validate_samples(samples)

    def test_training_data_quality_accepts_grouped_binary_labels(self):
        validate_samples(
            [
                sample("r1", 1, 1, 1),
                sample("r1", 2, 2, 0),
                sample("r2", 3, 3, 1),
                sample("r2", 4, 4, 0),
            ]
        )

    def test_time_split_is_chronological_and_request_disjoint(self):
        samples = [
            sample("r1", 1, 1, 1),
            sample("r1", 2, 2, 0),
            sample("r2", 3, 3, 1),
            sample("r2", 4, 4, 0),
            sample("r3", 5, 5, 1),
        ]
        training, validation = time_split(samples, 0.3)
        self.assertLess(max(item.event_time_ms for item in training), min(item.event_time_ms for item in validation))
        self.assertTrue({item.request_id for item in training}.isdisjoint(item.request_id for item in validation))

    def test_metrics_cover_auc_ranking_coverage_and_diversity(self):
        samples = [
            sample("r1", 1, 1, 1, "go"),
            sample("r1", 1, 2, 0, "rust"),
            sample("r2", 2, 3, 1, "go"),
            sample("r2", 2, 4, 0, "go"),
        ]
        metrics = evaluate(samples, [0.9, 0.1, 0.8, 0.2], k=2, catalog_size=8)
        self.assertEqual(1.0, metrics.auc)
        self.assertEqual(1.0, metrics.recall_at_k)
        self.assertEqual(1.0, metrics.ndcg_at_k)
        self.assertEqual(0.5, metrics.coverage)
        self.assertEqual(0.75, metrics.diversity)
        self.assertEqual(0.5, binary_auc([1, 1], [0.2, 0.3]))
        self.assertEqual(0.875, binary_auc([0, 1, 0, 1], [0.1, 0.9, 0.5, 0.5]))
        with self.assertRaisesRegex(ValueError, "equal in length"):
            binary_auc([0, 1], [0.5])

    def test_metadata_is_traceable_and_compatible_with_online_loader(self):
        with tempfile.TemporaryDirectory() as directory:
            model_path = Path(directory) / "ranker.txt"
            model_path.write_text("model", encoding="utf-8")
            metadata_path = write_model_metadata(
                model_path,
                version="rank-v1",
                feature_version="v2",
                training_window={"sample_start": "2026-01-01", "sample_end": "2026-01-02"},
                metrics=Evaluation(0.8, 0.7, 0.6, 0.5, 0.4),
            )
            metadata = json.loads(metadata_path.read_text(encoding="utf-8"))
            self.assertEqual("lightgbm", metadata["model_type"])
            self.assertEqual("rank-v1", metadata["model_version"])
            self.assertEqual("v2", metadata["feature_version"])
            self.assertIn("feature_names", metadata)


if __name__ == "__main__":
    unittest.main()
