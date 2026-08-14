#!/usr/bin/env python3
"""Aggregate Go coverage profiles by architecture layer and enforce thresholds."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Sequence


GENERATED_MARKERS = (
    ".pb.go",
    "_grpc.pb.go",
    "/internal/types/",
    "/internal/handler/routes.go",
    "/contentservice/content_service.go",
    "/feedservice/feed_service.go",
    "/interactionservice/interaction_service.go",
    "/mediaservice/media_service.go",
    "/messageservice/message_service.go",
    "/userservice/user_service.go",
)


def category(path: str) -> str:
    normalized = "/" + path.replace("\\", "/")
    if any(marker in normalized for marker in GENERATED_MARKERS):
        return "generated"
    if "/internal/handler/" in normalized:
        return "handler"
    if "/internal/logic/" in normalized:
        return "logic"
    if "/internal/model/" in normalized:
        return "model"
    if "/internal/mqs/" in normalized:
        return "mq_consumer"
    if "/internal/server/" in normalized or "/internal/svc/" in normalized:
        return "wiring"
    if "/pkg/" in normalized or normalized.count("/") == 2:
        return "shared"
    return "other"


def load_profiles(profile_dir: Path) -> dict[str, dict[str, int]]:
    blocks: dict[str, tuple[str, int, bool]] = {}
    for profile in sorted(profile_dir.glob("*.out")):
        for line in profile.read_text(encoding="utf-8").splitlines():
            if not line or line.startswith("mode:"):
                continue
            location, statements, count = line.rsplit(" ", 2)
            path = location.rsplit(":", 1)[0]
            layer = category(path)
            statement_count = int(statements)
            previous = blocks.get(location)
            covered = int(count) > 0
            if previous is not None:
                if previous[0] != layer or previous[1] != statement_count:
                    raise ValueError(f"inconsistent coverage block: {location}")
                covered = covered or previous[2]
            blocks[location] = (layer, statement_count, covered)

    totals: dict[str, dict[str, int]] = {}
    for layer, statement_count, covered in blocks.values():
        bucket = totals.setdefault(layer, {"covered": 0, "statements": 0})
        bucket["statements"] += statement_count
        if covered:
            bucket["covered"] += statement_count
    return totals


def add_percentages(totals: dict[str, dict[str, int]]) -> None:
    for values in totals.values():
        statements = values["statements"]
        values["coverage"] = round(100 * values["covered"] / statements, 1) if statements else 0.0

    handwritten = [values for layer, values in totals.items() if layer != "generated"]
    covered = sum(values["covered"] for values in handwritten)
    statements = sum(values["statements"] for values in handwritten)
    totals["handwritten"] = {
        "covered": covered,
        "statements": statements,
        "coverage": round(100 * covered / statements, 1) if statements else 0.0,
    }


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("profile_dir", type=Path)
    parser.add_argument("--thresholds", type=Path, required=True)
    parser.add_argument("--gate", choices=("none", "baseline", "target"), default="baseline")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args(argv)

    totals = load_profiles(args.profile_dir)
    add_percentages(totals)
    result = {"gate": args.gate, "layers": dict(sorted(totals.items()))}
    rendered = json.dumps(result, ensure_ascii=True, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered, encoding="utf-8")

    print("layer          covered/statements  coverage")
    for layer, values in sorted(totals.items()):
        print(f"{layer:<14} {values['covered']:>6}/{values['statements']:<10} {values['coverage']:>6.1f}%")

    if args.gate == "none":
        return 0
    threshold_sets = json.loads(args.thresholds.read_text(encoding="utf-8"))
    baseline = threshold_sets["baseline"]
    target = threshold_sets["target"]
    regressions = [
        layer
        for layer, minimum in baseline.items()
        if target.get(layer, -1) < minimum
    ]
    if regressions:
        joined = ", ".join(sorted(regressions))
        raise ValueError(f"target thresholds are below baseline: {joined}")

    thresholds = threshold_sets[args.gate]
    failures = []
    for layer, minimum in thresholds.items():
        actual = totals.get(layer, {}).get("coverage", 0.0)
        if actual < minimum:
            failures.append(f"{layer}: {actual:.1f}% < {minimum:.1f}%")
    if failures:
        print("coverage gate failed: " + "; ".join(failures))
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
