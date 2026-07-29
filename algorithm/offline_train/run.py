from __future__ import annotations

import argparse
import json
import os
from dataclasses import asdict, dataclass
from datetime import datetime
from pathlib import Path

from algorithm.offline_train.dataset import load_samples
from algorithm.offline_train.registry import RegisteredModel, S3ModelRegistry
from algorithm.offline_train.training import (
    Evaluation,
    time_split,
    train_lightgbm,
    validate_samples,
    write_model_metadata,
)


@dataclass(frozen=True)
class TrainingRun:
    registered_model: RegisteredModel
    metrics: Evaluation
    training_samples: int
    validation_samples: int


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Train and register the recommendation ranker")
    parser.add_argument("--version", required=True)
    parser.add_argument("--feature-start", required=True)
    parser.add_argument("--sample-start", required=True)
    parser.add_argument("--sample-end", required=True)
    parser.add_argument("--feature-version", default="v2")
    parser.add_argument("--validation-fraction", type=float, default=0.2)
    parser.add_argument("--output-dir", type=Path, default=Path("artifacts"))
    parser.add_argument("--registry-status", default="candidate")
    return parser.parse_args()


def train_and_register(
    *,
    client,
    registry: S3ModelRegistry,
    version: str,
    feature_start: datetime,
    sample_start: datetime,
    sample_end: datetime,
    feature_version: str,
    validation_fraction: float,
    output_dir: Path,
    registry_status: str,
) -> TrainingRun:
    samples = load_samples(
        client,
        feature_start=feature_start,
        sample_start=sample_start,
        sample_end=sample_end,
    )
    if not samples:
        raise RuntimeError("training query returned no exposure samples")
    validate_samples(samples)
    training, validation = time_split(samples, validation_fraction)
    validate_samples(training)
    validate_samples(validation)
    model_path = output_dir / version / "ranker.txt"
    _, metrics = train_lightgbm(training, validation, model_path)
    window = {
        "feature_start": feature_start.isoformat(),
        "sample_start": sample_start.isoformat(),
        "sample_end": sample_end.isoformat(),
    }
    metadata_path = write_model_metadata(
        model_path,
        version=version,
        feature_version=feature_version,
        training_window=window,
        metrics=metrics,
    )
    registered = registry.register(
        version=version,
        model_path=model_path,
        metadata_path=metadata_path,
        metrics=asdict(metrics),
        training_window=window,
        feature_version=feature_version,
        status=registry_status,
    )
    return TrainingRun(
        registered_model=registered,
        metrics=metrics,
        training_samples=len(training),
        validation_samples=len(validation),
    )


def main() -> None:
    import clickhouse_connect  # type: ignore[import-not-found]

    args = parse_args()
    feature_start = _parse_window(args.feature_start)
    sample_start = _parse_window(args.sample_start)
    sample_end = _parse_window(args.sample_end)
    client = clickhouse_connect.get_client(dsn=os.environ["CLICKHOUSE_DSN"])
    registry = S3ModelRegistry(
        bucket=os.environ["MODEL_REGISTRY_BUCKET"],
        prefix=os.getenv("MODEL_REGISTRY_PREFIX", "recommend-models"),
    )
    try:
        result = train_and_register(
            client=client,
            registry=registry,
            version=args.version,
            feature_start=feature_start,
            sample_start=sample_start,
            sample_end=sample_end,
            feature_version=args.feature_version,
            validation_fraction=args.validation_fraction,
            output_dir=args.output_dir,
            registry_status=args.registry_status,
        )
    finally:
        client.close()
    print(json.dumps(asdict(result), sort_keys=True))


def _parse_window(value: str) -> datetime:
    parsed = datetime.fromisoformat(value)
    if parsed.tzinfo is None or parsed.utcoffset() is None:
        raise ValueError("training window timestamps must include an explicit timezone offset")
    return parsed


if __name__ == "__main__":
    main()
