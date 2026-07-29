from __future__ import annotations

import hashlib
import logging
import math
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from typing import Mapping, Protocol, Sequence


class RankingModel(Protocol):
    def predict(self, rows: Sequence[Mapping[str, float]]) -> Sequence[float]: ...


@dataclass(frozen=True)
class Candidate:
    post_id: int
    features: Mapping[str, float]
    coarse_score: float


@dataclass(frozen=True)
class RankResult:
    model_version: str
    scores: tuple[tuple[int, float], ...]


@dataclass(frozen=True)
class LoadedModel:
    version: str
    model: RankingModel
    feature_names: tuple[str, ...]


class ModelManager:
    def __init__(self, *, shadow_workers: int = 2) -> None:
        self._lock = threading.RLock()
        self._models: dict[str, LoadedModel] = {}
        self._active_version = ""
        self._activation_history: list[str] = []
        self._traffic: tuple[tuple[str, float], ...] = ()
        self._shadow_versions: tuple[str, ...] = ()
        self._shadow_pool = ThreadPoolExecutor(
            max_workers=max(1, shadow_workers), thread_name_prefix="rank-shadow"
        )

    def close(self) -> None:
        self._shadow_pool.shutdown(wait=True, cancel_futures=True)

    def register(self, loaded: LoadedModel, *, activate: bool = False) -> None:
        if not loaded.version.strip():
            raise ValueError("model version is required")
        if not loaded.feature_names:
            raise ValueError("model feature_names are required")
        with self._lock:
            self._models[loaded.version] = loaded
            if activate or not self._active_version:
                self._activate_locked(loaded.version)

    def activate(self, version: str) -> None:
        with self._lock:
            if version not in self._models:
                raise KeyError(f"model version is not loaded: {version}")
            self._activate_locked(version)

    def rollback(self) -> str:
        with self._lock:
            if len(self._activation_history) < 2:
                raise RuntimeError("no previous model version is available")
            current = self._activation_history.pop()
            previous = self._activation_history[-1]
            self._active_version = previous
            self._traffic = ()
            logging.warning("rolled back online model from %s to %s", current, previous)
            return previous

    def configure_traffic(self, weights: Mapping[str, float]) -> None:
        positive = [(version, float(weight)) for version, weight in weights.items() if weight > 0]
        if not positive:
            raise ValueError("traffic configuration requires at least one positive weight")
        with self._lock:
            missing = [version for version, _ in positive if version not in self._models]
            if missing:
                raise KeyError(f"traffic references unloaded models: {', '.join(sorted(missing))}")
            total = sum(weight for _, weight in positive)
            cumulative = 0.0
            normalized: list[tuple[str, float]] = []
            for version, weight in sorted(positive):
                cumulative += weight / total
                normalized.append((version, cumulative))
            normalized[-1] = (normalized[-1][0], 1.0)
            self._traffic = tuple(normalized)

    def configure_shadow(self, versions: Sequence[str]) -> None:
        with self._lock:
            missing = [version for version in versions if version not in self._models]
            if missing:
                raise KeyError(f"shadow references unloaded models: {', '.join(sorted(missing))}")
            self._shadow_versions = tuple(dict.fromkeys(versions))

    def rank(
        self,
        request_id: str,
        requested_version: str,
        candidates: Sequence[Candidate],
    ) -> RankResult:
        if not request_id.strip():
            raise ValueError("request_id is required")
        if not candidates:
            raise ValueError("at least one candidate is required")
        post_ids = [candidate.post_id for candidate in candidates]
        if any(post_id <= 0 for post_id in post_ids) or len(set(post_ids)) != len(post_ids):
            raise ValueError("candidate post_ids must be positive and unique")

        with self._lock:
            version = self._select_version_locked(request_id, requested_version)
            loaded = self._models[version]
            shadows = tuple(
                self._models[item]
                for item in self._shadow_versions
                if item != version and item in self._models
            )

        rows = tuple(self._row(candidate, loaded.feature_names) for candidate in candidates)
        scores = self._predict(loaded, rows, len(candidates))
        for shadow in shadows:
            shadow_rows = tuple(self._row(candidate, shadow.feature_names) for candidate in candidates)
            self._shadow_pool.submit(
                self._run_shadow, request_id, shadow, shadow_rows, scores
            )
        return RankResult(
            model_version=version,
            scores=tuple(zip(post_ids, scores, strict=True)),
        )

    def health(self) -> tuple[bool, tuple[str, ...], str]:
        with self._lock:
            versions = tuple(sorted(self._models))
            return bool(self._active_version), versions, self._active_version

    def _activate_locked(self, version: str) -> None:
        if not self._activation_history or self._activation_history[-1] != version:
            self._activation_history.append(version)
        self._active_version = version
        self._traffic = ()

    def _select_version_locked(self, request_id: str, requested_version: str) -> str:
        requested = requested_version.strip()
        if requested and requested != "auto":
            if requested not in self._models:
                raise KeyError(f"requested model version is not loaded: {requested}")
            return requested
        if self._traffic:
            bucket = int.from_bytes(
                hashlib.sha256(request_id.encode("utf-8")).digest()[:8], "big"
            ) / float(2**64)
            for version, cumulative in self._traffic:
                if bucket < cumulative:
                    return version
        if not self._active_version:
            raise RuntimeError("no active model is loaded")
        return self._active_version

    @staticmethod
    def _row(candidate: Candidate, feature_names: Sequence[str]) -> dict[str, float]:
        row = {name: float(candidate.features.get(name, 0.0)) for name in feature_names}
        if "coarse_score" in row:
            row["coarse_score"] = float(candidate.coarse_score)
        return row

    @staticmethod
    def _predict(
        loaded: LoadedModel,
        rows: Sequence[Mapping[str, float]],
        expected: int,
    ) -> tuple[float, ...]:
        raw_scores = loaded.model.predict(rows)
        scores = tuple(float(score) for score in raw_scores)
        if len(scores) != expected:
            raise ValueError(
                f"model {loaded.version} returned {len(scores)} scores for {expected} candidates"
            )
        if any(not math.isfinite(score) for score in scores):
            raise ValueError(f"model {loaded.version} returned a non-finite score")
        return scores

    @staticmethod
    def _run_shadow(
        request_id: str,
        shadow: LoadedModel,
        rows: Sequence[Mapping[str, float]],
        primary_scores: Sequence[float],
    ) -> None:
        started = time.monotonic()
        try:
            shadow_scores = ModelManager._predict(shadow, rows, len(primary_scores))
            mean_delta = sum(
                abs(primary - candidate)
                for primary, candidate in zip(primary_scores, shadow_scores, strict=True)
            ) / len(primary_scores)
            logging.info(
                "shadow rank request_id=%s model=%s latency_ms=%.2f mean_abs_delta=%.6f",
                request_id,
                shadow.version,
                (time.monotonic() - started) * 1000,
                mean_delta,
            )
        except Exception:
            logging.exception(
                "shadow rank failed request_id=%s model=%s", request_id, shadow.version
            )
