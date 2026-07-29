from __future__ import annotations

import json
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Mapping

from algorithm.model_registry import ModelManifest, create_s3_client, sha256_file


@dataclass(frozen=True)
class RegisteredModel:
    version: str
    model_uri: str
    metadata_uri: str
    manifest_uri: str


class S3ModelRegistry:
    def __init__(self, *, bucket: str, prefix: str = "recommend-models", client=None) -> None:
        if not bucket:
            raise ValueError("model registry bucket is required")
        self._bucket = bucket
        self._prefix = prefix.strip("/")
        self._client = client or create_s3_client()

    def register(
        self,
        *,
        version: str,
        model_path: Path,
        metadata_path: Path,
        metrics: Mapping[str, float],
        training_window: Mapping[str, str],
        feature_version: str,
        status: str = "candidate",
    ) -> RegisteredModel:
        if not model_path.is_file():
            raise ValueError(f"model artifact does not exist: {model_path}")
        if not metadata_path.is_file():
            raise ValueError(f"model metadata does not exist: {metadata_path}")
        metadata = json.loads(metadata_path.read_text(encoding="utf-8"))
        if metadata.get("model_version") != version:
            raise ValueError("model metadata version does not match registry version")
        if metadata.get("feature_version") != feature_version:
            raise ValueError("model metadata feature_version does not match registry feature_version")
        model_key = f"{self._prefix}/{version}/{model_path.name}"
        metadata_key = f"{self._prefix}/{version}/{metadata_path.name}"
        manifest_key = f"{self._prefix}/{version}/manifest.json"
        model_uri = f"s3://{self._bucket}/{model_key}"
        metadata_uri = f"s3://{self._bucket}/{metadata_key}"
        manifest = ModelManifest(
            model_version=version,
            model_uri=model_uri,
            metadata_uri=metadata_uri,
            sha256=sha256_file(model_path),
            feature_version=feature_version,
            training_window=dict(training_window),
            metrics={key: float(value) for key, value in metrics.items()},
            status=status,
            registered_at=datetime.now(timezone.utc).isoformat(),
        )
        self._client.upload_file(str(model_path), self._bucket, model_key)
        self._client.upload_file(str(metadata_path), self._bucket, metadata_key)
        self._client.put_object(
            Bucket=self._bucket,
            Key=manifest_key,
            Body=(json.dumps(manifest.to_dict(), sort_keys=True, indent=2) + "\n").encode("utf-8"),
            ContentType="application/json",
        )
        return RegisteredModel(
            version=version,
            model_uri=model_uri,
            metadata_uri=metadata_uri,
            manifest_uri=f"s3://{self._bucket}/{manifest_key}",
        )
