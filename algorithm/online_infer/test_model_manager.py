import threading
import unittest

from algorithm.online_infer.model_manager import Candidate, LoadedModel, ModelManager


class FakeModel:
    def __init__(self, multiplier: float) -> None:
        self.multiplier = multiplier

    def predict(self, rows):
        return [row["quality"] * self.multiplier + row["coarse_score"] for row in rows]


class ModelManagerTest(unittest.TestCase):
    def setUp(self) -> None:
        self.manager = ModelManager()
        self.manager.register(
            LoadedModel("rank-v1", FakeModel(1.0), ("quality", "coarse_score")),
            activate=True,
        )

    def tearDown(self) -> None:
        self.manager.close()

    def candidates(self):
        return [Candidate(1, {"quality": 0.2}, 0.1), Candidate(2, {"quality": 0.5}, 0.2)]

    def test_rank_uses_requested_model_and_preserves_ids(self):
        result = self.manager.rank("request-1", "rank-v1", self.candidates())
        self.assertEqual("rank-v1", result.model_version)
        self.assertEqual(((1, 0.30000000000000004), (2, 0.7)), result.scores)

    def test_hot_load_traffic_and_rollback(self):
        self.manager.register(
            LoadedModel("rank-v2", FakeModel(2.0), ("quality", "coarse_score")),
            activate=True,
        )
        self.manager.configure_traffic({"rank-v1": 1, "rank-v2": 1})
        first = self.manager.rank("stable-request", "auto", self.candidates()).model_version
        second = self.manager.rank("stable-request", "auto", self.candidates()).model_version
        self.assertEqual(first, second)
        self.assertEqual("rank-v1", self.manager.rollback())
        self.assertEqual("rank-v1", self.manager.health()[2])
        self.assertEqual(
            "rank-v1",
            self.manager.rank("stable-request", "auto", self.candidates()).model_version,
        )

    def test_shadow_does_not_change_primary_response(self):
        shadow_called = threading.Event()

        class ShadowModel(FakeModel):
            def predict(self, rows):
                shadow_called.set()
                return super().predict(rows)

        self.manager.register(
            LoadedModel("shadow-v1", ShadowModel(5.0), ("quality", "coarse_score"))
        )
        self.manager.configure_shadow(["shadow-v1"])
        result = self.manager.rank("request-shadow", "rank-v1", self.candidates())
        self.assertEqual("rank-v1", result.model_version)
        self.assertTrue(shadow_called.wait(timeout=1))

    def test_invalid_model_output_is_rejected(self):
        class InvalidModel:
            def predict(self, rows):
                return [float("nan") for _ in rows]

        self.manager.register(LoadedModel("bad", InvalidModel(), ("quality",)))
        with self.assertRaisesRegex(ValueError, "non-finite"):
            self.manager.rank("request-bad", "bad", self.candidates())


if __name__ == "__main__":
    unittest.main()
