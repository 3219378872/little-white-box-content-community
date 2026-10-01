from __future__ import annotations

import logging
import os
import signal
import sys
from concurrent import futures
from pathlib import Path

import grpc

GENERATED_DIR = Path(__file__).with_name("generated")
sys.path.insert(0, str(GENERATED_DIR))
import moderation_pb2  # type: ignore[import-not-found]  # noqa: E402
import moderation_pb2_grpc  # type: ignore[import-not-found]  # noqa: E402

from algorithm.moderation_infer.scorer import score  # noqa: E402

MAX_TEXT_BYTES = 16384
MAX_ISSUES = 64


class ModerationInferService(moderation_pb2_grpc.ModerationInferServiceServicer):
    def __init__(self, fixture_enabled: bool) -> None:
        self._fixture_enabled = fixture_enabled

    def Score(self, request, context):
        if len(request.text.encode("utf-8")) > MAX_TEXT_BYTES or len(request.issues) > MAX_ISSUES:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, "request too large")
        try:
            version, scores = score(request.text, list(request.issues), self._fixture_enabled)
        except ValueError as exc:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(exc))
        # Never log ad text (REL-022); task id is enough to correlate.
        logging.info("scored task_id=%s issues=%d", request.task_id, len(scores))
        return moderation_pb2.ScoreResp(
            scores=[moderation_pb2.IssueScore(issue=s.issue, p_yes=s.p_yes, p_no=s.p_no) for s in scores],
            model_version=version,
        )

    def Health(self, request, context):
        version, _ = score("", [], self._fixture_enabled)
        return moderation_pb2.ModerationHealthResp(ready=True, model_version=version)


def serve() -> None:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    # Bind loopback by default: the sidecar has no authentication (DES-review-platform).
    address = os.environ.get("MODERATION_INFER_LISTEN", "127.0.0.1:9026")
    fixture_enabled = os.environ.get("MODERATION_FIXTURE_ENABLED", "") == "1"
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=int(os.environ.get("GRPC_WORKERS", "4"))))
    moderation_pb2_grpc.add_ModerationInferServiceServicer_to_server(ModerationInferService(fixture_enabled), server)
    server.add_insecure_port(address)
    server.start()
    logging.info("moderation-infer listening on %s fixture=%s", address, fixture_enabled)

    def stop(*_args) -> None:
        server.stop(grace=5)

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    server.wait_for_termination()


if __name__ == "__main__":
    serve()
