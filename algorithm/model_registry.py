from __future__ import annotations

import hashlib
import json
import math
import os
import shutil
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from urllib.parse import urlparse


MODEL_STATUSES = frozenset({"candidate", "shadow", "active", "retired", "rolled_back"})
DEPLOYABLE_MODEL_STATUSES = frozenset({"candidate", "shadow", "active"})


@dataclass(frozen=True)
class ModelManifest:
    model_version: str
    model_uri: str
    metadata_uri: str
    sha256: str
    feature_version: str
    training_window: Mapping[str, str]
    metrics: Mapping[str, float]
    status: str
    registered_at: str

    def __post_init__(self) -> None:
        if not self.model_version.strip():
            raise ValueError("manifest model_version is required")
        _validate_uri(self.model_uri, "model_uri")
        _validate_uri(self.metadata_uri, "metadata_uri")
        if len(self.sha256) != 64 or any(character not in "0123456789abcdef" for character in self.sha256):
            raise ValueError("manifest sha256 must be a lowercase hexadecimal SHA-256 digest")
        if not self.feature_version.strip():
            raise ValueError("manifest feature_version is required")
        if not self.training_window:
            raise ValueError("manifest training_window is required")
        if any(not str(key).strip() or not str(value).strip() for key, value in self.training_window.items()):
            raise ValueError("manifest training_window keys and values must be non-empty")
        if not self.metrics:
            raise ValueError("manifest metrics are required")
        if any(not str(key).strip() or not math.isfinite(float(value)) for key, value in self.metrics.items()):
            raise ValueError("manifest metrics must have non-empty names and finite values")
        if self.status not in MODEL_STATUSES:
            raise ValueError(f"unsupported model status: {self.status}")
        if not self.registered_at.strip():
            raise ValueError("manifest registered_at is required")

    @classmethod
    def from_mapping(cls, payload: Mapping[str, Any]) -> "ModelManifest":
        training_window = payload.get("training_window")
        metrics = payload.get("metrics")
        if not isinstance(training_window, Mapping):
            raise ValueError("manifest training_window must be an object")
        if not isinstance(metrics, Mapping):
            raise ValueError("manifest metrics must be an object")
        return cls(
            model_version=str(payload.get("model_version", "")),
            model_uri=str(payload.get("model_uri", "")),
            metadata_uri=str(payload.get("metadata_uri", "")),
            sha256=str(payload.get("sha256", "")),
            feature_version=str(payload.get("feature_version", "")),
            training_window={str(key): str(value) for key, value in training_window.items()},
            metrics={str(key): float(value) for key, value in metrics.items()},
            status=str(payload.get("status", "")),
            registered_at=str(payload.get("registered_at", "")),
        )

    def to_dict(self) -> dict[str, Any]:
        return {
            "model_version": self.model_version,
            "model_uri": self.model_uri,
            "metadata_uri": self.metadata_uri,
            "sha256": self.sha256,
            "feature_version": self.feature_version,
            "training_window": dict(self.training_window),
            "metrics": {key: float(value) for key, value in self.metrics.items()},
            "status": self.status,
            "registered_at": self.registered_at,
        }


def create_s3_client():
    import boto3  # type: ignore[import-not-found]

    options = {
        "endpoint_url": os.getenv("MODEL_S3_ENDPOINT") or None,
        "aws_access_key_id": os.getenv("MODEL_S3_ACCESS_KEY") or None,
        "aws_secret_access_key": os.getenv("MODEL_S3_SECRET_KEY") or None,
        "region_name": os.getenv("MODEL_S3_REGION") or None,
    }
    return boto3.client("s3", **{key: value for key, value in options.items() if value is not None})


def load_manifest(uri: str, *, s3_client=None) -> ModelManifest:
    try:
        payload = json.loads(read_uri(uri, s3_client=s3_client).decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise ValueError(f"model manifest is not valid JSON: {uri}") from exc
    if not isinstance(payload, Mapping):
        raise ValueError("model manifest must be a JSON object")
    return ModelManifest.from_mapping(payload)


def read_uri(uri: str, *, s3_client=None) -> bytes:
    parsed = urlparse(uri)
    if parsed.scheme in ("", "file"):
        path = Path(parsed.path if parsed.scheme else uri).resolve()
        if not path.is_file():
            raise ValueError(f"artifact does not exist: {path}")
        return path.read_bytes()
    bucket, key = _s3_location(uri)
    client = s3_client or create_s3_client()
    response = client.get_object(Bucket=bucket, Key=key)
    body = response.get("Body")
    if body is None or not hasattr(body, "read"):
        raise ValueError(f"S3 object response has no readable body: {uri}")
    return bytes(body.read())


def download_uri(uri: str, destination: Path, *, s3_client=None) -> None:
    parsed = urlparse(uri)
    destination.parent.mkdir(parents=True, exist_ok=True)
    if parsed.scheme in ("", "file"):
        source = Path(parsed.path if parsed.scheme else uri).resolve()
        if not source.is_file():
            raise ValueError(f"artifact does not exist: {source}")
        shutil.copyfile(source, destination)
        return
    bucket, key = _s3_location(uri)
    client = s3_client or create_s3_client()
    client.download_file(bucket, key, str(destination))


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as artifact:
        for block in iter(lambda: artifact.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def _validate_uri(uri: str, field: str) -> None:
    if not uri.strip():
        raise ValueError(f"manifest {field} is required")
    parsed = urlparse(uri)
    if parsed.scheme not in ("", "file", "s3"):
        raise ValueError(f"manifest {field} has unsupported URI scheme: {parsed.scheme}")
    if parsed.scheme == "s3":
        _s3_location(uri)


def _s3_location(uri: str) -> tuple[str, str]:
    parsed = urlparse(uri)
    bucket = parsed.netloc.strip()
    key = parsed.path.lstrip("/")
    if parsed.scheme != "s3" or not bucket or not key:
        raise ValueError(f"invalid S3 URI: {uri}")
    return bucket, key
