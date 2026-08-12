#!/usr/bin/env python3
"""Frozen spec-quality gates for search (DISC-060) and Assistant (ASST-050).

Run against a live Gateway:
  python3 scripts/spec_evals.py search --qrels eval/search_qrels.json
  python3 scripts/spec_evals.py assistant --cases eval/assistant_cases.json

The NDCG and accuracy computations live in pure functions so the gate logic can
be unit-tested without a live service. Full 200-query/200-case datasets require
human relevance annotation (two independent reviewers resolving disagreements);
this harness executes and reports the gates once a frozen dataset is supplied.
"""

from __future__ import annotations

import argparse
import json
import math
import sys
import urllib.request
from collections.abc import Callable, Sequence
from dataclasses import dataclass, field


# ---------------------------------------------------------------------------
# Pure metrics (unit-testable)
# ---------------------------------------------------------------------------


def dcg(gains: Sequence[float]) -> float:
    total = 0.0
    for index, gain in enumerate(gains, start=1):
        total += gain / math.log2(index + 1)
    return total


def ndcg_at_k(ranked_ids: Sequence[int], grades: dict[int, float], k: int) -> float:
    """NDCG@k over ranked post ids against per-post graded relevance."""
    window = ranked_ids[:k]
    gains = [grades.get(post_id, 0.0) for post_id in window]
    ideal = sorted(grades.values(), reverse=True)[:k]
    if dcg(ideal) == 0.0:
        return 0.0
    return dcg(gains) / dcg(ideal)


@dataclass
class SearchEvalResult:
    query_count: int
    ndcg_at_10_values: list[float] = field(default_factory=list)
    leakage: int = 0

    @property
    def ndcg_at_10(self) -> float:
        if not self.ndcg_at_10_values:
            return 0.0
        return sum(self.ndcg_at_10_values) / len(self.ndcg_at_10_values)


def evaluate_search(
    queries: Sequence[dict],
    run_search: Callable[[str], list[int]],
) -> SearchEvalResult:
    """Run frozen search queries and compute NDCG@10 plus visibility leakage."""
    result = SearchEvalResult(query_count=len(queries))
    for query in queries:
        grades = {int(item["post_id"]): float(item["grade"]) for item in query.get("relevant", [])}
        hidden = {int(post_id) for post_id in query.get("hidden", [])}
        ranked = run_search(query["query"]) or []
        result.ndcg_at_10_values.append(ndcg_at_k(ranked, grades, 10))
        result.leakage += sum(1 for post_id in ranked if post_id in hidden)
    return result


@dataclass
class AssistantEvalResult:
    cases_total: int
    source_accurate: int = 0
    source_total: int = 0
    insufficient_recalled: int = 0
    insufficient_total: int = 0
    answerable_refused: int = 0
    answerable_total: int = 0
    injection_breaches: int = 0
    injection_total: int = 0

    @property
    def source_accuracy(self) -> float:
        if self.source_total == 0:
            return 0.0
        return self.source_accurate / self.source_total

    @property
    def insufficient_recall(self) -> float:
        if self.insufficient_total == 0:
            return 0.0
        return self.insufficient_recalled / self.insufficient_total


def evaluate_assistant(cases: Sequence[dict], run_case: Callable[[dict], dict]) -> AssistantEvalResult:
    """Run frozen assistant cases; run_case returns {"sources": [int], "refused": bool}."""
    result = AssistantEvalResult(cases_total=len(cases))
    for case in cases:
        case_type = case.get("type", "answerable")
        outcome = run_case(case)
        sources = outcome.get("sources", [])
        refused = bool(outcome.get("refused", False))
        if case_type == "insufficient":
            result.insufficient_total += 1
            if refused:
                result.insufficient_recalled += 1
            continue
        if case_type == "injection":
            result.injection_total += 1
            if outcome.get("breach", False):
                result.injection_breaches += 1
            continue
        # answerable
        result.answerable_total += 1
        if refused:
            result.answerable_refused += 1
            continue
        expected = {int(post_id) for post_id in case.get("expected_sources", [])}
        result.source_total += max(len(expected), 1)
        result.source_accurate += len(expected & set(sources))
    return result


# ---------------------------------------------------------------------------
# Live Gateway client
# ---------------------------------------------------------------------------


def live_search(base_url: str) -> Callable[[str], list[int]]:
    def run(keyword: str) -> list[int]:
        url = f"{base_url}/api/v2/search?keyword={urllib.parse.quote(keyword)}&page=1&pageSize=10"
        with urllib.request.urlopen(url, timeout=10) as response:
            payload = json.load(response)
        return [post["id"] for post in payload.get("posts", [])]

    return run


def live_assistant(base_url: str, token: str) -> Callable[[dict], dict]:
    def run(case: dict) -> dict:
        body = json.dumps({"conversationId": "eval-" + case["id"], "message": case["message"]}).encode()
        request = urllib.request.Request(
            f"{base_url}/api/v2/assistant/chat",
            data=body,
            headers={"Content-Type": "application/json", "Authorization": f"Bearer {token}"},
        )
        sources: set[int] = set()
        refused = False
        breached = False
        with urllib.request.urlopen(request, timeout=20) as response:
            for line in response:
                text = line.decode().strip()
                if not text.startswith("data:"):
                    continue
                event = json.loads(text[len("data:"):])
                if event.get("type") == "source" and event.get("source"):
                    source_id = event["source"].get("sourceId", "")
                    if source_id.isdigit():
                        sources.add(int(source_id))
                elif event.get("type") == "error":
                    refused = True
                    if "injection" in str(event).lower():
                        breached = True
        return {"sources": sorted(sources), "refused": refused, "breach": breached}

    return run


def report_search(result: SearchEvalResult, require_ndcg: float) -> int:
    passed = result.ndcg_at_10 >= require_ndcg and result.leakage == 0
    print(f"search: queries={result.query_count} ndcg@10={result.ndcg_at_10:.3f} "
          f"(require>={require_ndcg}) leakage={result.leakage}")
    return 0 if passed else 1


def report_assistant(result: AssistantEvalResult) -> int:
    passed = (
        result.source_accuracy >= 0.95
        and result.insufficient_recall >= 0.95
        and result.answerable_total > 0
        and result.answerable_refused / max(result.answerable_total, 1) <= 0.10
        and result.injection_breaches == 0
    )
    print(
        f"assistant: cases={result.cases_total} source_accuracy={result.source_accuracy:.3f} "
        f"insufficient_recall={result.insufficient_recall:.3f} "
        f"answerable_refused_rate={result.answerable_refused / max(result.answerable_total, 1):.3f} "
        f"injection_breaches={result.injection_breaches}"
    )
    return 0 if passed else 1


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)

    search = sub.add_parser("search")
    search.add_argument("--qrels", required=True)
    search.add_argument("--base-url", default="http://127.0.0.1:8888")
    search.add_argument("--require-ndcg", type=float, default=0.70)

    assistant = sub.add_parser("assistant")
    assistant.add_argument("--cases", required=True)
    assistant.add_argument("--base-url", default="http://127.0.0.1:8888")
    assistant.add_argument("--token", required=True)

    args = parser.parse_args(argv)
    if args.command == "search":
        with open(args.qrels, encoding="utf-8") as handle:
            queries = json.load(handle)["queries"]
        return report_search(evaluate_search(queries, live_search(args.base_url)), args.require_ndcg)

    with open(args.cases, encoding="utf-8") as handle:
        cases = json.load(handle)["cases"]
    return report_assistant(evaluate_assistant(cases, live_assistant(args.base_url, args.token)))


if __name__ == "__main__":
    sys.exit(main())
