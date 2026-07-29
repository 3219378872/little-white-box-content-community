import json
import tempfile
import unittest
from concurrent import futures
from pathlib import Path

try:
    import grpc
except ModuleNotFoundError:  # Runtime integration dependency is intentionally isolated.
    grpc = None


@unittest.skipIf(grpc is None, "grpcio is not installed")
class OnlineInferGRPCTest(unittest.TestCase):
    def setUp(self):
        from algorithm.online_infer.model_manager import LoadedModel, ModelManager
        from algorithm.online_infer.server import (
            OnlineInferService,
            inference_pb2,
            inference_pb2_grpc,
        )

        class InitialModel:
            def predict(self, rows):
                return [row["quality"] + row["coarse_score"] for row in rows]

        self.pb = inference_pb2
        self.manager = ModelManager()
        self.manager.register(
            LoadedModel("rank-v1", InitialModel(), ("quality", "coarse_score")),
            activate=True,
        )
        self.server = grpc.server(futures.ThreadPoolExecutor(max_workers=2))
        inference_pb2_grpc.add_OnlineInferServiceServicer_to_server(
            OnlineInferService(self.manager), self.server
        )
        port = self.server.add_insecure_port("127.0.0.1:0")
        self.server.start()
        self.channel = grpc.insecure_channel(f"127.0.0.1:{port}")
        self.stub = inference_pb2_grpc.OnlineInferServiceStub(self.channel)

    def tearDown(self):
        self.channel.close()
        self.server.stop(grace=0).wait(timeout=2)
        self.manager.close()

    def test_rank_health_reload_and_rollback(self):
        response = self.stub.Rank(
            self.pb.RankReq(
                request_id="request-1",
                model_version="rank-v1",
                candidates=[
                    self.pb.RankCandidate(
                        post_id=9, features={"quality": 0.4}, coarse_score=0.2
                    )
                ],
            )
        )
        self.assertEqual("rank-v1", response.model_version)
        self.assertAlmostEqual(0.6, response.candidates[0].score)
        self.assertTrue(self.stub.Health(self.pb.InferHealthReq()).ready)

        with tempfile.TemporaryDirectory() as directory:
            model_path = Path(directory) / "linear.json"
            model_path.write_text(
                json.dumps({"weights": [2.0, 1.0], "bias": 0.1}), encoding="utf-8"
            )
            Path(str(model_path) + ".meta.json").write_text(
                json.dumps(
                    {
                        "model_type": "linear-json",
                        "feature_names": ["quality", "coarse_score"],
                    }
                ),
                encoding="utf-8",
            )
            loaded = self.stub.ReloadModel(
                self.pb.ReloadModelReq(model_version="rank-v2", model_uri=str(model_path))
            )
            self.assertEqual("rank-v2", loaded.model_version)

        rolled_back = self.stub.ReloadModel(
            self.pb.ReloadModelReq(
                model_version="", model_uri="rollback://previous"
            )
        )
        self.assertEqual("rank-v1", rolled_back.model_version)

    def test_invalid_candidate_returns_invalid_argument(self):
        with self.assertRaises(grpc.RpcError) as raised:
            self.stub.Rank(
                self.pb.RankReq(
                    request_id="request-invalid",
                    candidates=[self.pb.RankCandidate(post_id=0)],
                )
            )
        self.assertEqual(grpc.StatusCode.INVALID_ARGUMENT, raised.exception.code())


if __name__ == "__main__":
    unittest.main()
