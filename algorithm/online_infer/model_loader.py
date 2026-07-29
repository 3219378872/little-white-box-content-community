from __future__ import annotations

import json
import hmac
import tempfile
from pathlib import Path
from typing import Callable, Mapping, Sequence
from urllib.parse import urlparse

from algorithm.model_registry import (
    DEPLOYABLE_MODEL_STATUSES,
    create_s3_client,
    download_uri,
    load_manifest,
    sha256_file,
)
from algorithm.online_infer.model_manager import LoadedModel


class LinearModel:
    def __init__(self, feature_names: Sequence[str], weights: Sequence[float], bias: float) -> None:
        if len(feature_names) != len(weights):
            raise ValueError("linear model features and weights differ in length")
        self._feature_names = tuple(feature_names)
        self._weights = tuple(float(weight) for weight in weights)
        self._bias = float(bias)

    def predict(self, rows: Sequence[Mapping[str, float]]) -> list[float]:
        return [
            self._bias
            + sum(float(row.get(name, 0.0)) * weight for name, weight in zip(
                self._feature_names, self._weights, strict=True
            ))
            for row in rows
        ]


class LightGBMModel:
    def __init__(self, model_path: Path, feature_names: Sequence[str]) -> None:
        import lightgbm  # type: ignore[import-not-found]

        self._booster = lightgbm.Booster(model_file=str(model_path))
        self._feature_names = tuple(feature_names)

    def predict(self, rows: Sequence[Mapping[str, float]]) -> list[float]:
        matrix = [[float(row.get(name, 0.0)) for name in self._feature_names] for row in rows]
        return [float(value) for value in self._booster.predict(matrix)]


class ONNXModel:
    def __init__(self, model_path: Path, feature_names: Sequence[str]) -> None:
        import onnxruntime  # type: ignore[import-not-found]

        self._session = onnxruntime.InferenceSession(
            str(model_path), providers=["CPUExecutionProvider"]
        )
        self._input_name = self._session.get_inputs()[0].name
        self._feature_names = tuple(feature_names)

    def predict(self, rows: Sequence[Mapping[str, float]]) -> list[float]:
        import numpy  # type: ignore[import-not-found]

        matrix = numpy.asarray(
            [[float(row.get(name, 0.0)) for name in self._feature_names] for row in rows],
            dtype=numpy.float32,
        )
        output = self._session.run(None, {self._input_name: matrix})[0]
        return [float(value) for value in numpy.asarray(output).reshape(-1)]


def load_model(version: str, uri: str) -> LoadedModel:
    if not version.strip():
        raise ValueError("model version is required")
    path, cleanup = _materialize(uri)
    try:
        metadata_path = Path(str(path) + ".meta.json")
        return _load_local_model(version, path, metadata_path)
    finally:
        cleanup()


def load_model_reference(version: str, uri: str, *, s3_client=None) -> LoadedModel:
    if Path(urlparse(uri).path).name == "manifest.json":
        return load_registered_model(version, uri, s3_client=s3_client)
    return load_model(version, uri)


def load_registered_model(
    expected_version: str,
    manifest_uri: str,
    *,
    s3_client=None,
) -> LoadedModel:
    client = s3_client
    if urlparse(manifest_uri).scheme == "s3" and client is None:
        client = create_s3_client()
    manifest = load_manifest(manifest_uri, s3_client=client)
    if expected_version.strip() and expected_version != manifest.model_version:
        raise ValueError(
            f"manifest model version {manifest.model_version} does not match requested version {expected_version}"
        )
    if manifest.status not in DEPLOYABLE_MODEL_STATUSES:
        raise ValueError(
            f"model {manifest.model_version} has non-deployable registry status: {manifest.status}"
        )

    with tempfile.TemporaryDirectory(prefix="online-infer-registry-") as directory:
        model_path = Path(directory) / "model.artifact"
        metadata_path = Path(directory) / "model.metadata.json"
        download_uri(manifest.model_uri, model_path, s3_client=client)
        download_uri(manifest.metadata_uri, metadata_path, s3_client=client)
        actual_sha256 = sha256_file(model_path)
        if not hmac.compare_digest(actual_sha256, manifest.sha256):
            raise ValueError(
                f"model artifact SHA-256 mismatch for version {manifest.model_version}"
            )
        loaded = _load_local_model(manifest.model_version, model_path, metadata_path)
        metadata = _read_metadata(metadata_path)
        if metadata.get("model_version") != manifest.model_version:
            raise ValueError("model metadata version does not match registry manifest")
        if metadata.get("feature_version") != manifest.feature_version:
            raise ValueError("model metadata feature_version does not match registry manifest")
        return loaded


def _load_local_model(version: str, path: Path, metadata_path: Path) -> LoadedModel:
    metadata = _read_metadata(metadata_path)
    feature_names = tuple(str(item).strip() for item in metadata.get("feature_names", ()))
    if not feature_names or any(not item for item in feature_names):
        raise ValueError("model metadata feature_names are required")
    if len(set(feature_names)) != len(feature_names):
        raise ValueError("model metadata feature_names must be unique")
    metadata_version = str(metadata.get("model_version", "")).strip()
    if metadata_version and metadata_version != version:
        raise ValueError(
            f"model metadata version {metadata_version} does not match requested version {version}"
        )
    model_type = str(metadata.get("model_type", "")).lower()
    if model_type == "linear-json":
        payload = json.loads(path.read_text(encoding="utf-8"))
        model = LinearModel(feature_names, payload["weights"], payload.get("bias", 0.0))
    elif model_type == "lightgbm":
        model = LightGBMModel(path, feature_names)
    elif model_type == "onnx":
        model = ONNXModel(path, feature_names)
    else:
        raise ValueError(f"unsupported model_type: {model_type}")
    return LoadedModel(version=version, model=model, feature_names=feature_names)


def _read_metadata(path: Path) -> Mapping[str, object]:
    if not path.is_file():
        raise ValueError(f"model metadata does not exist: {path}")
    try:
        metadata = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise ValueError(f"model metadata is not valid JSON: {path}") from exc
    if not isinstance(metadata, Mapping):
        raise ValueError("model metadata must be a JSON object")
    return metadata


def _materialize(uri: str) -> tuple[Path, Callable[[], None]]:
    parsed = urlparse(uri)
    if parsed.scheme in ("", "file"):
        path = Path(parsed.path if parsed.scheme else uri).resolve()
        if not path.is_file():
            raise ValueError(f"model file does not exist: {path}")
        return path, lambda: None
    if parsed.scheme != "s3":
        raise ValueError(f"unsupported model URI scheme: {parsed.scheme}")

    temporary = tempfile.TemporaryDirectory(prefix="online-infer-")
    local = Path(temporary.name) / Path(parsed.path).name
    client = create_s3_client()
    key = parsed.path.lstrip("/")
    client.download_file(parsed.netloc, key, str(local))
    client.download_file(parsed.netloc, key + ".meta.json", str(local) + ".meta.json")
    return local, temporary.cleanup
