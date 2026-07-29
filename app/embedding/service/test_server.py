import unittest
from concurrent import futures

import grpc

import embedding_pb2
import embedding_pb2_grpc
from server import EmbeddingServicer, ServiceConfig


class AbortError(Exception):
    def __init__(self, code, details):
        super().__init__(details)
        self.code = code
        self.details = details


class FakeContext:
    def abort(self, code, details):
        raise AbortError(code, details)


class FakeBackend:
    dimension = 3

    def __init__(self):
        self.calls = []
        self.error = None
        self.vectors = None

    def encode(self, texts):
        self.calls.append(list(texts))
        if self.error:
            raise self.error
        if self.vectors is not None:
            return self.vectors
        return [[0.1, float(index + 1), 0.3] for index, _ in enumerate(texts)]


def config(**overrides):
    values = dict(
        listen_address="127.0.0.1:50051",
        model_name="test-model",
        model_revision="abc123",
        model_version="test-model@abc123",
        dimension=3,
        max_text_bytes=16,
        max_batch_size=3,
        max_batch_bytes=32,
        inference_batch_size=2,
        workers=2,
        shutdown_grace_seconds=1,
    )
    values.update(overrides)
    return ServiceConfig(**values)


class EmbeddingServicerTest(unittest.TestCase):
    def test_config_requires_artifact_derived_model_version(self):
        config().validate()
        with self.assertRaisesRegex(ValueError, "EMBEDDING_MODEL_VERSION"):
            config(model_version="untraceable-v1").validate()

    def test_batch_uses_one_inference_and_returns_metadata(self):
        backend = FakeBackend()
        service = EmbeddingServicer(backend, config())

        response = service.EmbedBatch(
            embedding_pb2.EmbedBatchReq(texts=["one", "two"]), FakeContext()
        )

        self.assertEqual([["one", "two"]], backend.calls)
        self.assertEqual(2, len(response.items))
        self.assertEqual("test-model@abc123", response.model_version)
        self.assertEqual(3, response.dimension)

    def test_generated_grpc_server_serves_batch_and_health(self):
        backend = FakeBackend()
        grpc_server = grpc.server(futures.ThreadPoolExecutor(max_workers=1))
        embedding_pb2_grpc.add_EmbeddingServiceServicer_to_server(
            EmbeddingServicer(backend, config()), grpc_server
        )
        port = grpc_server.add_insecure_port("127.0.0.1:0")
        self.assertGreater(port, 0)
        grpc_server.start()
        channel = grpc.insecure_channel(f"127.0.0.1:{port}")
        try:
            stub = embedding_pb2_grpc.EmbeddingServiceStub(channel)
            health = stub.Health(embedding_pb2.EmbeddingHealthReq(), timeout=1)
            response = stub.EmbedBatch(
                embedding_pb2.EmbedBatchReq(texts=["one", "two"]), timeout=1
            )
            self.assertTrue(health.ready)
            self.assertEqual(2, len(response.items))
            self.assertEqual([["one", "two"]], backend.calls)
        finally:
            channel.close()
            grpc_server.stop(0).wait(timeout=1)

    def test_health_reports_model_metadata(self):
        service = EmbeddingServicer(FakeBackend(), config())
        response = service.Health(embedding_pb2.EmbeddingHealthReq(), FakeContext())
        self.assertTrue(response.ready)
        self.assertEqual("test-model@abc123", response.model_version)
        self.assertEqual(3, response.dimension)

    def test_rejects_input_limits_before_inference(self):
        backend = FakeBackend()
        service = EmbeddingServicer(backend, config())

        with self.assertRaises(AbortError) as caught:
            service.Embed(embedding_pb2.EmbedReq(text="x" * 17), FakeContext())

        self.assertEqual(grpc.StatusCode.RESOURCE_EXHAUSTED, caught.exception.code)
        self.assertEqual([], backend.calls)

    def test_rejects_zero_and_nonfinite_vectors(self):
        for vector, detail in (([0, 0, 0], "all zero"), ([0.1, float("nan"), 0.3], "non-finite")):
            with self.subTest(detail=detail):
                backend = FakeBackend()
                backend.vectors = [vector]
                service = EmbeddingServicer(backend, config())
                with self.assertRaises(AbortError) as caught:
                    service.Embed(embedding_pb2.EmbedReq(text="ok"), FakeContext())
                self.assertEqual(grpc.StatusCode.INTERNAL, caught.exception.code)
                self.assertIn(detail, caught.exception.details)

    def test_inference_failure_is_retryable_unavailable(self):
        backend = FakeBackend()
        backend.error = RuntimeError("model crashed")
        service = EmbeddingServicer(backend, config())

        with self.assertLogs("embedding-service", level="ERROR"):
            with self.assertRaises(AbortError) as caught:
                service.Embed(embedding_pb2.EmbedReq(text="ok"), FakeContext())

        self.assertEqual(grpc.StatusCode.UNAVAILABLE, caught.exception.code)


if __name__ == "__main__":
    unittest.main()
