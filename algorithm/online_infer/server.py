from __future__ import annotations

import json
import logging
import os
import signal
import sys
from concurrent import futures
from pathlib import Path

import grpc

GENERATED_DIR = Path(__file__).with_name("generated")
sys.path.insert(0, str(GENERATED_DIR))
import inference_pb2  # type: ignore[import-not-found]  # noqa: E402
import inference_pb2_grpc  # type: ignore[import-not-found]  # noqa: E402

from algorithm.online_infer.model_loader import load_model_reference  # noqa: E402
from algorithm.online_infer.model_manager import Candidate, ModelManager  # noqa: E402


class OnlineInferService(inference_pb2_grpc.OnlineInferServiceServicer):
    def __init__(self, manager: ModelManager) -> None:
        self._manager = manager

    def Rank(self, request, context):
        try:
            result = self._manager.rank(
                request.request_id,
                request.model_version,
                [
                    Candidate(
                        post_id=item.post_id,
                        features=dict(item.features),
                        coarse_score=item.coarse_score,
                    )
                    for item in request.candidates
                ],
            )
        except (KeyError, RuntimeError) as exc:
            context.abort(grpc.StatusCode.FAILED_PRECONDITION, str(exc))
        except ValueError as exc:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(exc))
        except Exception:
            logging.exception("online rank failed request_id=%s", request.request_id)
            context.abort(grpc.StatusCode.INTERNAL, "online inference failed")
        return inference_pb2.RankResp(
            candidates=[
                inference_pb2.RankedCandidate(post_id=post_id, score=score)
                for post_id, score in result.scores
            ],
            model_version=result.model_version,
        )

    def ReloadModel(self, request, context):
        try:
            if request.model_uri in ("rollback", "rollback://previous"):
                version = self._manager.rollback()
            elif request.model_uri in ("activate", "activate://loaded"):
                self._manager.activate(request.model_version)
                version = request.model_version
            else:
                loaded = load_model_reference(request.model_version, request.model_uri)
                self._manager.register(loaded, activate=True)
                version = loaded.version
        except (KeyError, RuntimeError, ValueError) as exc:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(exc))
        except Exception:
            logging.exception("model reload failed version=%s", request.model_version)
            context.abort(grpc.StatusCode.INTERNAL, "model reload failed")
        return inference_pb2.ReloadModelResp(loaded=True, model_version=version)

    def Health(self, request, context):
        ready, versions, _ = self._manager.health()
        return inference_pb2.InferHealthResp(
            ready=ready, loaded_versions=list(versions)
        )


def configure_manager() -> ModelManager:
    manager = ModelManager(shadow_workers=int(os.getenv("SHADOW_WORKERS", "2")))
    manifest = json.loads(os.getenv("MODEL_MANIFEST_JSON", "[]"))
    if not isinstance(manifest, list):
        raise ValueError("MODEL_MANIFEST_JSON must be a JSON array")
    for item in manifest:
        if not isinstance(item, dict):
            raise ValueError("MODEL_MANIFEST_JSON entries must be objects")
        loaded = load_model_reference(
            str(item.get("version", "")),
            str(item.get("manifest_uri") or item.get("uri") or ""),
        )
        manager.register(loaded, activate=bool(item.get("active", False)))
    registry_manifest_uri = os.getenv("MODEL_REGISTRY_MANIFEST_URI", "").strip()
    if registry_manifest_uri:
        loaded = load_model_reference(
            os.getenv("MODEL_REGISTRY_VERSION", "").strip(),
            registry_manifest_uri,
        )
        manager.register(loaded, activate=True)
    traffic = json.loads(os.getenv("MODEL_TRAFFIC_JSON", "{}"))
    if traffic:
        manager.configure_traffic({str(key): float(value) for key, value in traffic.items()})
    shadows = [item.strip() for item in os.getenv("SHADOW_MODEL_VERSIONS", "").split(",") if item.strip()]
    if shadows:
        manager.configure_shadow(shadows)
    return manager


def serve() -> None:
    logging.basicConfig(
        level=os.getenv("LOG_LEVEL", "INFO"),
        format="%(asctime)s %(levelname)s %(message)s",
    )
    manager = configure_manager()
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=int(os.getenv("GRPC_WORKERS", "16"))),
        options=[
            ("grpc.max_receive_message_length", 4 * 1024 * 1024),
            ("grpc.max_send_message_length", 4 * 1024 * 1024),
        ],
    )
    inference_pb2_grpc.add_OnlineInferServiceServicer_to_server(
        OnlineInferService(manager), server
    )
    address = os.getenv("ONLINE_INFER_LISTEN", "0.0.0.0:9025")
    server.add_insecure_port(address)
    server.start()
    logging.info("online inference listening on %s", address)

    def stop(_signum, _frame):
        logging.info("online inference stopping")
        server.stop(grace=10)

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    try:
        server.wait_for_termination()
    finally:
        manager.close()


if __name__ == "__main__":
    serve()
