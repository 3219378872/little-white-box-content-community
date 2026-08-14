from __future__ import annotations

import json
import math
import os
import tempfile
import time
import unittest
from concurrent import futures
from datetime import datetime, timedelta, timezone
from pathlib import Path

try:
    import boto3
    import clickhouse_connect
    import grpc

    from algorithm.model_registry import load_manifest
    from algorithm.offline_train.registry import S3ModelRegistry
    from algorithm.offline_train.run import train_and_register
    from algorithm.online_infer.server import (
        OnlineInferService,
        configure_manager,
        inference_pb2,
        inference_pb2_grpc,
    )

    INTEGRATION_IMPORT_ERROR = None
except ModuleNotFoundError as exc:
    INTEGRATION_IMPORT_ERROR = exc


@unittest.skipIf(
    INTEGRATION_IMPORT_ERROR is not None,
    f"model pipeline dependencies are not installed: {INTEGRATION_IMPORT_ERROR}",
)
class ModelPipelineIntegrationTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.clickhouse = cls._connect_clickhouse()
        cls.s3 = boto3.client(
            "s3",
            endpoint_url=os.environ["MODEL_S3_ENDPOINT"],
            aws_access_key_id=os.environ["MODEL_S3_ACCESS_KEY"],
            aws_secret_access_key=os.environ["MODEL_S3_SECRET_KEY"],
            region_name=os.environ.get("MODEL_S3_REGION", "us-east-1"),
        )
        cls._create_bucket()

    @classmethod
    def tearDownClass(cls) -> None:
        cls.clickhouse.close()

    @classmethod
    def _connect_clickhouse(cls):
        deadline = time.monotonic() + 90
        last_error = None
        while time.monotonic() < deadline:
            try:
                client = clickhouse_connect.get_client(dsn=os.environ["CLICKHOUSE_DSN"])
                client.command("SELECT 1")
                return client
            except Exception as exc:
                last_error = exc
                time.sleep(1)
        raise RuntimeError("ClickHouse did not become ready") from last_error

    @classmethod
    def _create_bucket(cls) -> None:
        deadline = time.monotonic() + 90
        last_error = None
        while time.monotonic() < deadline:
            try:
                cls.s3.create_bucket(Bucket=os.environ["MODEL_REGISTRY_BUCKET"])
                return
            except cls.s3.exceptions.BucketAlreadyOwnedByYou:
                return
            except Exception as exc:
                last_error = exc
                time.sleep(1)
        raise RuntimeError("MinIO did not become ready") from last_error

    def test_clickhouse_training_registry_inference_reload_and_rollback(self):
        # 窗口使用相对当前时间：behavior_events 的 TTL 是 received_at+90 天，
        # 固定历史时间戳（如 2026-01）会在插入后立即过期并被后台任务删除，
        # 导致第二次训练查询无样本（两次查询间 TTL 删除已触发）。
        now = datetime.now(timezone.utc)
        feature_start = now - timedelta(days=14)
        sample_start = now - timedelta(days=7)
        sample_end = now - timedelta(days=1)
        self._seed_behavior(feature_start, sample_start)
        registry = S3ModelRegistry(
            bucket=os.environ["MODEL_REGISTRY_BUCKET"],
            prefix=os.environ["MODEL_REGISTRY_PREFIX"],
            client=self.s3,
        )

        with tempfile.TemporaryDirectory() as directory:
            first = train_and_register(
                client=self.clickhouse,
                registry=registry,
                version="rank-pipeline-v1",
                feature_start=feature_start,
                sample_start=sample_start,
                sample_end=sample_end,
                feature_version="v2",
                validation_fraction=0.2,
                output_dir=Path(directory),
                registry_status="active",
            )
            second = train_and_register(
                client=self.clickhouse,
                registry=registry,
                version="rank-pipeline-v2",
                feature_start=feature_start,
                sample_start=sample_start,
                sample_end=sample_end,
                feature_version="v2",
                validation_fraction=0.2,
                output_dir=Path(directory),
                registry_status="candidate",
            )

        self.assertGreater(first.training_samples, first.validation_samples)
        self.assertTrue(math.isfinite(first.metrics.ndcg_at_k))
        manifest = load_manifest(first.registered_model.manifest_uri, s3_client=self.s3)
        self.assertEqual("active", manifest.status)
        self.assertEqual("lightgbm", self._metadata_type(manifest.metadata_uri))

        os.environ["MODEL_MANIFEST_JSON"] = "[]"
        os.environ["MODEL_REGISTRY_MANIFEST_URI"] = first.registered_model.manifest_uri
        os.environ.pop("MODEL_REGISTRY_VERSION", None)
        os.environ.pop("MODEL_TRAFFIC_JSON", None)
        manager = configure_manager()
        server = grpc.server(futures.ThreadPoolExecutor(max_workers=2))
        inference_pb2_grpc.add_OnlineInferServiceServicer_to_server(
            OnlineInferService(manager), server
        )
        port = server.add_insecure_port("127.0.0.1:0")
        server.start()
        channel = grpc.insecure_channel(f"127.0.0.1:{port}")
        stub = inference_pb2_grpc.OnlineInferServiceStub(channel)
        try:
            grpc.channel_ready_future(channel).result(timeout=10)
            self.assertEqual("rank-pipeline-v1", self._rank(stub).model_version)

            loaded = stub.ReloadModel(
                inference_pb2.ReloadModelReq(
                    model_uri=second.registered_model.manifest_uri
                ),
                timeout=30,
            )
            self.assertTrue(loaded.loaded)
            self.assertEqual("rank-pipeline-v2", loaded.model_version)
            self.assertEqual("rank-pipeline-v2", self._rank(stub).model_version)

            rolled_back = stub.ReloadModel(
                inference_pb2.ReloadModelReq(model_uri="rollback://previous"),
                timeout=10,
            )
            self.assertEqual("rank-pipeline-v1", rolled_back.model_version)
            self.assertEqual("rank-pipeline-v1", self._rank(stub).model_version)
            self.assertEqual(
                {"rank-pipeline-v1", "rank-pipeline-v2"},
                set(stub.Health(inference_pb2.InferHealthReq(), timeout=10).loaded_versions),
            )
        finally:
            channel.close()
            server.stop(grace=0).wait(timeout=5)
            manager.close()

    def _metadata_type(self, metadata_uri: str) -> str:
        bucket_and_key = metadata_uri.removeprefix("s3://")
        bucket, key = bucket_and_key.split("/", 1)
        response = self.s3.get_object(Bucket=bucket, Key=key)
        return json.loads(response["Body"].read().decode("utf-8"))["model_type"]

    @staticmethod
    def _rank(stub):
        response = stub.Rank(
            inference_pb2.RankReq(
                request_id="pipeline-recommend-request",
                model_version="auto",
                candidates=[
                    inference_pb2.RankCandidate(
                        post_id=1,
                        features={
                            "recall_score": 1.0,
                            "quality": 0.7,
                            "ctr": 0.6,
                            "freshness": 0.5,
                            "popularity": 2.0,
                        },
                        coarse_score=0.8,
                    ),
                    inference_pb2.RankCandidate(
                        post_id=2,
                        features={
                            "recall_score": 0.5,
                            "quality": 0.2,
                            "ctr": 0.1,
                            "freshness": 0.9,
                            "popularity": 1.0,
                        },
                        coarse_score=0.3,
                    ),
                ],
            ),
            timeout=10,
        )
        if len(response.candidates) != 2 or any(
            not math.isfinite(candidate.score) for candidate in response.candidates
        ):
            raise AssertionError("online inference returned invalid ranking scores")
        return response

    def _seed_behavior(self, feature_start: datetime, sample_start: datetime) -> None:
        columns = [
            "event_id",
            "client_event_id",
            "schema_version",
            "event_time",
            "received_at",
            "user_id",
            "request_id",
            "action",
            "target_id",
            "target_type",
            "scene",
            "position",
            "recall_source",
            "model_version",
            "producer",
        ]
        rows = []
        event_id = 1

        def append_event(event_time, request_id, action, post_id, position, recall_source):
            nonlocal event_id
            rows.append(
                [
                    event_id,
                    f"pipeline-{event_id}",
                    2,
                    event_time,
                    event_time + timedelta(milliseconds=10),
                    1000 + event_id % 17,
                    request_id,
                    action,
                    post_id,
                    "post",
                    "home",
                    position,
                    recall_source,
                    "rules-v2",
                    "model-pipeline-integration",
                ]
            )
            event_id += 1

        for post_id in range(1, 9):
            for index in range(12):
                timestamp = feature_start + timedelta(hours=post_id * 12 + index)
                append_event(timestamp, f"feature-{post_id}-{index}", "exposure", post_id, index + 1, "hot")
                if index < post_id:
                    append_event(
                        timestamp + timedelta(minutes=1),
                        f"feature-{post_id}-{index}",
                        "click",
                        post_id,
                        None,
                        "hot",
                    )

        recall_sources = ("itemcf", "vector", "hot")
        for request_index in range(40):
            timestamp = sample_start + timedelta(hours=request_index * 2)
            request_id = f"recommend-{request_index:03d}"
            positive_post = request_index % 8 + 1
            for position, post_id in enumerate(range(1, 9), start=1):
                append_event(
                    timestamp,
                    request_id,
                    "exposure",
                    post_id,
                    position,
                    recall_sources[position % len(recall_sources)],
                )
            append_event(
                timestamp + timedelta(minutes=2),
                request_id,
                "click",
                positive_post,
                None,
                recall_sources[positive_post % len(recall_sources)],
            )

        self.clickhouse.insert(
            "xbh_analytics.behavior_events", rows, column_names=columns
        )


if __name__ == "__main__":
    unittest.main()
