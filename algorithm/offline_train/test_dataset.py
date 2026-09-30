from datetime import datetime, timedelta, timezone
from pathlib import Path
import unittest

from algorithm.offline_train.dataset import SAMPLE_QUERY, load_samples


class CanonicalSampleTest(unittest.TestCase):
    def test_every_training_input_uses_durable_canonical_facts(self):
        self.assertNotIn("behavior_events", SAMPLE_QUERY)
        self.assertEqual(SAMPLE_QUERY.count("xbh_analytics.behavior_facts"), 3)
        schema = (Path(__file__).resolve().parents[2] / "deploy/sql/xbh_analytics.sql").read_text()
        self.assertIn("CREATE OR REPLACE VIEW xbh_analytics.behavior_facts", schema)
        self.assertIn("LIMIT 1 BY", schema)
        self.assertIn("ORDER BY event_time, event_id", schema)
        # Request and post are distinct SQL tuple components, not delimiter-joined text.
        self.assertIn("target_id, event_id)", schema)
        self.assertIn("request_id, '')", schema)

    def test_load_samples_preserves_canonical_rows_and_parameters(self):
        class Result:
            column_names = ["request_id", "event_time_ms", "post_id", "label", "recall_score", "quality", "ctr", "freshness", "popularity", "coarse_score", "category"]
            result_rows = [("request", 123, 9, 1, 1.0, 0.5, 0.1, 0.8, 2.0, 0.6, "hot")]

        class Client:
            def query(self, query, parameters):
                self.query_text = query
                self.parameters = parameters
                return Result()

        client = Client()
        end = datetime(2026, 9, 30, tzinfo=timezone.utc)
        rows = load_samples(client, feature_start=end-timedelta(days=30), sample_start=end-timedelta(days=1), sample_end=end)
        self.assertEqual([(row.request_id, row.post_id) for row in rows], [("request", 9)])
        self.assertEqual(rows[0].features["ctr"], 0.1)
        self.assertEqual(client.query_text, SAMPLE_QUERY)
        self.assertEqual(client.parameters["sample_end"], "2026-09-30 00:00:00.000")

    def test_missing_canonical_schema_fails_instead_of_reading_raw_duplicates(self):
        class Client:
            def query(self, query, parameters):
                raise RuntimeError("behavior_facts missing: schema upgrade required")

        end = datetime(2026, 9, 30, tzinfo=timezone.utc)
        with self.assertRaisesRegex(RuntimeError, "schema upgrade required"):
            load_samples(Client(), feature_start=end-timedelta(days=30), sample_start=end-timedelta(days=1), sample_end=end)
