from __future__ import annotations

import json
import math
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Iterable, Mapping, Sequence


FEATURE_NAMES = (
    "recall_score",
    "quality",
    "ctr",
    "freshness",
    "popularity",
    "coarse_score",
)


@dataclass(frozen=True)
class Sample:
    request_id: str
    event_time_ms: int
    post_id: int
    label: int
    features: Mapping[str, float]
    category: str = ""


@dataclass(frozen=True)
class Evaluation:
    auc: float
    recall_at_k: float
    ndcg_at_k: float
    coverage: float
    diversity: float


def validate_samples(samples: Sequence[Sample]) -> None:
    if len(samples) < 4:
        raise ValueError("at least four exposure samples are required")
    requests = {sample.request_id for sample in samples}
    if len(requests) < 2:
        raise ValueError("at least two request groups are required")
    labels = {sample.label for sample in samples}
    if not labels.issubset({0, 1}) or labels != {0, 1}:
        raise ValueError("training samples must contain binary positive and negative labels")
    candidates: set[tuple[str, int]] = set()
    for sample in samples:
        if not sample.request_id.strip() or sample.post_id <= 0:
            raise ValueError("training samples require request_id and positive post_id")
        candidate = (sample.request_id, sample.post_id)
        if candidate in candidates:
            raise ValueError("training samples contain duplicate request/post candidates")
        candidates.add(candidate)
        missing = [name for name in FEATURE_NAMES if name not in sample.features]
        if missing:
            raise ValueError(f"training sample is missing features: {', '.join(missing)}")
        if any(not math.isfinite(float(sample.features[name])) for name in FEATURE_NAMES):
            raise ValueError("training sample contains a non-finite feature")


def time_split(samples: Sequence[Sample], validation_fraction: float) -> tuple[list[Sample], list[Sample]]:
    if not 0 < validation_fraction < 0.5:
        raise ValueError("validation_fraction must be between 0 and 0.5")
    if len(samples) < 2:
        raise ValueError("at least two samples are required")
    ordered = sorted(samples, key=lambda sample: (sample.event_time_ms, sample.request_id, sample.post_id))
    cutoff_index = max(1, min(len(ordered) - 1, int(len(ordered) * (1 - validation_fraction))))
    cutoff_time = ordered[cutoff_index].event_time_ms
    training = [sample for sample in ordered if sample.event_time_ms < cutoff_time]
    validation = [sample for sample in ordered if sample.event_time_ms >= cutoff_time]
    if not training or not validation:
        training = ordered[:cutoff_index]
        validation = ordered[cutoff_index:]

    validation_requests = {sample.request_id for sample in validation}
    moved = [sample for sample in training if sample.request_id in validation_requests]
    if moved:
        training = [sample for sample in training if sample.request_id not in validation_requests]
        validation = sorted(validation + moved, key=lambda sample: sample.event_time_ms)
    if not training:
        raise ValueError("time split cannot keep request groups disjoint")
    return training, validation


def evaluate(samples: Sequence[Sample], scores: Sequence[float], *, k: int, catalog_size: int) -> Evaluation:
    if len(samples) != len(scores) or not samples:
        raise ValueError("samples and scores must be non-empty and equal in length")
    if k <= 0 or catalog_size <= 0:
        raise ValueError("k and catalog_size must be positive")
    paired = list(zip(samples, (float(score) for score in scores), strict=True))
    auc = binary_auc([sample.label for sample, _ in paired], [score for _, score in paired])
    grouped: dict[str, list[tuple[Sample, float]]] = {}
    for sample, score in paired:
        grouped.setdefault(sample.request_id, []).append((sample, score))

    recalls: list[float] = []
    ndcgs: list[float] = []
    diversity: list[float] = []
    recommended: set[int] = set()
    for rows in grouped.values():
        ranked = sorted(rows, key=lambda item: (-item[1], item[0].post_id))[:k]
        positives = sum(sample.label > 0 for sample, _ in rows)
        hits = sum(sample.label > 0 for sample, _ in ranked)
        if positives:
            recalls.append(hits / positives)
        gains = [sample.label for sample, _ in ranked]
        ideal = sorted((sample.label for sample, _ in rows), reverse=True)[:k]
        ndcgs.append(_dcg(gains) / _dcg(ideal) if _dcg(ideal) else 0.0)
        categories = [sample.category for sample, _ in ranked if sample.category]
        diversity.append(len(set(categories)) / len(categories) if categories else 0.0)
        recommended.update(sample.post_id for sample, _ in ranked)
    return Evaluation(
        auc=auc,
        recall_at_k=_mean(recalls),
        ndcg_at_k=_mean(ndcgs),
        coverage=min(1.0, len(recommended) / catalog_size),
        diversity=_mean(diversity),
    )


def binary_auc(labels: Sequence[int], scores: Sequence[float]) -> float:
    if len(labels) != len(scores):
        raise ValueError("labels and scores must be equal in length")
    ordered = sorted(
        ((float(score), int(label) > 0) for label, score in zip(labels, scores, strict=True)),
        key=lambda item: item[0],
    )
    positive_count = sum(is_positive for _, is_positive in ordered)
    negative_count = len(ordered) - positive_count
    if not positive_count or not negative_count:
        return 0.5

    wins = 0.0
    negatives_before = 0
    index = 0
    while index < len(ordered):
        group_end = index + 1
        while group_end < len(ordered) and ordered[group_end][0] == ordered[index][0]:
            group_end += 1
        group = ordered[index:group_end]
        group_positives = sum(is_positive for _, is_positive in group)
        group_negatives = len(group) - group_positives
        wins += group_positives * (negatives_before + group_negatives * 0.5)
        negatives_before += group_negatives
        index = group_end
    return wins / (positive_count * negative_count)


def train_lightgbm(
    training: Sequence[Sample],
    validation: Sequence[Sample],
    output_path: Path,
) -> tuple[object, Evaluation]:
    import lightgbm  # type: ignore[import-not-found]
    import numpy  # type: ignore[import-not-found]

    train_rows, train_labels, train_groups = _ranking_dataset(training)
    valid_rows, valid_labels, valid_groups = _ranking_dataset(validation)
    train_matrix = numpy.asarray(train_rows, dtype=numpy.float64)
    valid_matrix = numpy.asarray(valid_rows, dtype=numpy.float64)
    train_data = lightgbm.Dataset(
        train_matrix,
        label=numpy.asarray(train_labels, dtype=numpy.int32),
        group=numpy.asarray(train_groups, dtype=numpy.int32),
        feature_name=list(FEATURE_NAMES),
    )
    valid_data = lightgbm.Dataset(
        valid_matrix,
        label=numpy.asarray(valid_labels, dtype=numpy.int32),
        group=numpy.asarray(valid_groups, dtype=numpy.int32),
        feature_name=list(FEATURE_NAMES),
    )
    booster = lightgbm.train(
        {
            "objective": "lambdarank",
            "metric": "ndcg",
            "learning_rate": 0.05,
            "num_leaves": 31,
            "min_data_in_leaf": 20,
            "feature_fraction": 0.9,
            "bagging_fraction": 0.9,
            "bagging_freq": 1,
            "verbosity": -1,
            "seed": 20260729,
        },
        train_data,
        num_boost_round=500,
        valid_sets=[valid_data],
        callbacks=[lightgbm.early_stopping(40, verbose=False)],
    )
    output_path.parent.mkdir(parents=True, exist_ok=True)
    booster.save_model(str(output_path), num_iteration=booster.best_iteration)
    valid_scores = [float(value) for value in booster.predict(valid_matrix)]
    catalog_size = max(1, len({sample.post_id for sample in list(training) + list(validation)}))
    metrics = evaluate(validation, valid_scores, k=20, catalog_size=catalog_size)
    return booster, metrics


def write_model_metadata(
    model_path: Path,
    *,
    version: str,
    feature_version: str,
    training_window: Mapping[str, str],
    metrics: Evaluation,
) -> Path:
    metadata = {
        "model_type": "lightgbm",
        "model_version": version,
        "feature_names": list(FEATURE_NAMES),
        "feature_version": feature_version,
        "training_window": dict(training_window),
        "metrics": asdict(metrics),
        "created_at": datetime.now(timezone.utc).isoformat(),
    }
    path = Path(str(model_path) + ".meta.json")
    path.write_text(json.dumps(metadata, sort_keys=True, indent=2) + "\n", encoding="utf-8")
    return path


def _ranking_dataset(samples: Sequence[Sample]) -> tuple[list[list[float]], list[int], list[int]]:
    ordered = sorted(samples, key=lambda sample: (sample.request_id, sample.event_time_ms, sample.post_id))
    rows = [[float(sample.features.get(name, 0.0)) for name in FEATURE_NAMES] for sample in ordered]
    labels = [int(sample.label) for sample in ordered]
    groups: list[int] = []
    current = ""
    count = 0
    for sample in ordered:
        if current and sample.request_id != current:
            groups.append(count)
            count = 0
        current = sample.request_id
        count += 1
    if count:
        groups.append(count)
    return rows, labels, groups


def _dcg(labels: Iterable[int]) -> float:
    return sum((2**label - 1) / math.log2(index + 2) for index, label in enumerate(labels))


def _mean(values: Sequence[float]) -> float:
    return sum(values) / len(values) if values else 0.0
