from __future__ import annotations

import logging
import math
import os
import signal
import threading
from concurrent import futures
from dataclasses import dataclass
from typing import Protocol, Sequence

import grpc

import embedding_pb2
import embedding_pb2_grpc


LOGGER = logging.getLogger("embedding-service")


def _required_env(name: str) -> str:
    value = os.getenv(name, "").strip()
    if not value:
        raise ValueError(f"{name} is required")
    return value


def _positive_int_env(name: str, default: int) -> int:
    raw = os.getenv(name, str(default))
    try:
        value = int(raw)
    except ValueError as exc:
        raise ValueError(f"{name} must be an integer") from exc
    if value <= 0:
        raise ValueError(f"{name} must be positive")
    return value


@dataclass(frozen=True)
class ServiceConfig:
    listen_address: str
    model_name: str
    model_revision: str
    model_version: str
    dimension: int
    max_text_bytes: int
    max_batch_size: int
    max_batch_bytes: int
    inference_batch_size: int
    workers: int
    shutdown_grace_seconds: int

    @classmethod
    def from_env(cls) -> "ServiceConfig":
        config = cls(
            listen_address=os.getenv("EMBEDDING_LISTEN_ADDRESS", "[::]:50051"),
            model_name=_required_env("EMBEDDING_MODEL_NAME"),
            model_revision=_required_env("EMBEDDING_MODEL_REVISION"),
            model_version=_required_env("EMBEDDING_MODEL_VERSION"),
            dimension=_positive_int_env("EMBEDDING_DIMENSION", 384),
            max_text_bytes=_positive_int_env("EMBEDDING_MAX_TEXT_BYTES", 16384),
            max_batch_size=_positive_int_env("EMBEDDING_MAX_BATCH_SIZE", 64),
            max_batch_bytes=_positive_int_env("EMBEDDING_MAX_BATCH_BYTES", 262144),
            inference_batch_size=_positive_int_env("EMBEDDING_INFERENCE_BATCH_SIZE", 32),
            workers=_positive_int_env("EMBEDDING_WORKERS", 8),
            shutdown_grace_seconds=_positive_int_env("EMBEDDING_SHUTDOWN_GRACE_SECONDS", 30),
        )
        config.validate()
        return config

    def validate(self) -> None:
        if not self.listen_address.strip():
            raise ValueError("EMBEDDING_LISTEN_ADDRESS is required")
        if self.max_batch_bytes < self.max_text_bytes:
            raise ValueError("EMBEDDING_MAX_BATCH_BYTES must be at least EMBEDDING_MAX_TEXT_BYTES")
        if self.inference_batch_size > self.max_batch_size:
            raise ValueError("EMBEDDING_INFERENCE_BATCH_SIZE cannot exceed EMBEDDING_MAX_BATCH_SIZE")
        expected_version = f"{self.model_name}@{self.model_revision}"
        if self.model_version != expected_version:
            raise ValueError(
                f"EMBEDDING_MODEL_VERSION must be {expected_version!r} for the configured artifact"
            )
        if len(self.model_version.encode("utf-8")) > 256:
            raise ValueError("EMBEDDING_MODEL_VERSION must not exceed 256 UTF-8 bytes")


class EmbeddingBackend(Protocol):
    @property
    def dimension(self) -> int: ...

    def encode(self, texts: Sequence[str]) -> Sequence[Sequence[float]]: ...


class SentenceTransformerBackend:
    def __init__(self, config: ServiceConfig) -> None:
        from sentence_transformers import SentenceTransformer

        self._batch_size = config.inference_batch_size
        self._model = SentenceTransformer(
            config.model_name,
            revision=config.model_revision,
            trust_remote_code=False,
        )
        dimension = self._model.get_sentence_embedding_dimension()
        if dimension is None:
            raise RuntimeError("model did not report an embedding dimension")
        self._dimension = int(dimension)
        if self._dimension != config.dimension:
            raise RuntimeError(
                f"model dimension mismatch: got {self._dimension}, expected {config.dimension}"
            )

    @property
    def dimension(self) -> int:
        return self._dimension

    def encode(self, texts: Sequence[str]) -> Sequence[Sequence[float]]:
        vectors = self._model.encode(
            list(texts),
            batch_size=min(self._batch_size, len(texts)),
            convert_to_numpy=True,
            normalize_embeddings=True,
            show_progress_bar=False,
        )
        return vectors.tolist()


class EmbeddingServicer(embedding_pb2_grpc.EmbeddingServiceServicer):
    def __init__(self, backend: EmbeddingBackend, config: ServiceConfig) -> None:
        if backend.dimension != config.dimension:
            raise ValueError(
                f"backend dimension mismatch: got {backend.dimension}, expected {config.dimension}"
            )
        self._backend = backend
        self._config = config
        self._inference_lock = threading.Lock()

    def Embed(self, request, context):
        vectors = self._encode([request.text], context)
        return embedding_pb2.EmbedResp(
            vector=vectors[0],
            model_version=self._config.model_version,
            dimension=self._config.dimension,
        )

    def EmbedBatch(self, request, context):
        vectors = self._encode(list(request.texts), context)
        return embedding_pb2.EmbedBatchResp(
            items=[embedding_pb2.EmbedBatchItem(vector=vector) for vector in vectors],
            model_version=self._config.model_version,
            dimension=self._config.dimension,
        )

    def Health(self, _request, _context):
        return embedding_pb2.EmbeddingHealthResp(
            ready=True,
            model_version=self._config.model_version,
            dimension=self._config.dimension,
        )

    def _encode(self, texts: Sequence[str], context) -> list[list[float]]:
        self._validate_inputs(texts, context)
        try:
            with self._inference_lock:
                raw_vectors = self._backend.encode(texts)
        except Exception as exc:
            LOGGER.exception("embedding inference failed")
            context.abort(grpc.StatusCode.UNAVAILABLE, f"embedding inference failed: {exc}")
            raise AssertionError("context.abort must not return")

        if len(raw_vectors) != len(texts):
            context.abort(
                grpc.StatusCode.INTERNAL,
                f"model returned {len(raw_vectors)} vectors for {len(texts)} inputs",
            )
        vectors: list[list[float]] = []
        for index, raw_vector in enumerate(raw_vectors):
            vector = [float(value) for value in raw_vector]
            if len(vector) != self._config.dimension:
                context.abort(
                    grpc.StatusCode.INTERNAL,
                    f"vector {index} has dimension {len(vector)}, expected {self._config.dimension}",
                )
            if any(not math.isfinite(value) for value in vector):
                context.abort(grpc.StatusCode.INTERNAL, f"vector {index} contains non-finite values")
            if not any(value != 0.0 for value in vector):
                context.abort(grpc.StatusCode.INTERNAL, f"vector {index} is all zero")
            vectors.append(vector)
        return vectors

    def _validate_inputs(self, texts: Sequence[str], context) -> None:
        if not texts:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, "at least one input is required")
        if len(texts) > self._config.max_batch_size:
            context.abort(
                grpc.StatusCode.RESOURCE_EXHAUSTED,
                f"batch has {len(texts)} inputs, maximum is {self._config.max_batch_size}",
            )
        total_bytes = 0
        for index, text in enumerate(texts):
            if not text.strip():
                context.abort(grpc.StatusCode.INVALID_ARGUMENT, f"input {index} is blank")
            text_bytes = len(text.encode("utf-8"))
            if text_bytes > self._config.max_text_bytes:
                context.abort(
                    grpc.StatusCode.RESOURCE_EXHAUSTED,
                    f"input {index} is {text_bytes} bytes, maximum is {self._config.max_text_bytes}",
                )
            total_bytes += text_bytes
        if total_bytes > self._config.max_batch_bytes:
            context.abort(
                grpc.StatusCode.RESOURCE_EXHAUSTED,
                f"batch is {total_bytes} bytes, maximum is {self._config.max_batch_bytes}",
            )


def serve(config: ServiceConfig) -> None:
    backend = SentenceTransformerBackend(config)
    max_response_bytes = config.max_batch_size * config.dimension * 4 + 65536
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=config.workers),
        options=(
            ("grpc.max_receive_message_length", config.max_batch_bytes + 65536),
            ("grpc.max_send_message_length", max_response_bytes),
        ),
    )
    embedding_pb2_grpc.add_EmbeddingServiceServicer_to_server(
        EmbeddingServicer(backend, config), server
    )
    if server.add_insecure_port(config.listen_address) == 0:
        raise RuntimeError(f"could not bind {config.listen_address}")

    stopped = threading.Event()

    def request_stop(signum, _frame) -> None:
        LOGGER.info("received signal %s, stopping", signum)
        server.stop(config.shutdown_grace_seconds)
        stopped.set()

    signal.signal(signal.SIGTERM, request_stop)
    signal.signal(signal.SIGINT, request_stop)
    server.start()
    LOGGER.info(
        "embedding service ready address=%s model=%s dimension=%d",
        config.listen_address,
        config.model_version,
        config.dimension,
    )
    while not stopped.wait(timeout=1):
        pass
    server.wait_for_termination(timeout=config.shutdown_grace_seconds)


def main() -> None:
    logging.basicConfig(
        level=os.getenv("LOG_LEVEL", "INFO").upper(),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    serve(ServiceConfig.from_env())


if __name__ == "__main__":
    main()
