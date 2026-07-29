#!/usr/bin/env python3
"""Dependency-free latency gate for a live esx Gateway."""

from __future__ import annotations

import argparse
import concurrent.futures
import dataclasses
import http.client
import json
import math
import os
import statistics
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from collections.abc import Callable, Sequence


DEFAULT_BASE_URL = "http://127.0.0.1:8888"
DEFAULT_SCENARIOS = "behavior,search,feed,gateway,assistant"
MAX_RESPONSE_BYTES = 16 * 1024 * 1024


@dataclasses.dataclass(frozen=True)
class RequestSpec:
    method: str
    url: str
    expected_status: int
    headers: dict[str, str]
    body: bytes | None = None
    first_sse_token: bool = False
    json_validator: Callable[[object], None] | None = None


@dataclasses.dataclass(frozen=True)
class Scenario:
    name: str
    target_ms: float
    request: Callable[[int], RequestSpec]


@dataclasses.dataclass(frozen=True)
class Sample:
    latency_ms: float | None
    error: str | None


@dataclasses.dataclass(frozen=True)
class Summary:
    name: str
    target_ms: float
    total: int
    successful: int
    errors: int
    p50_ms: float | None
    p95_ms: float | None
    p99_ms: float | None
    max_ms: float | None
    error_examples: tuple[str, ...]

    @property
    def passed(self) -> bool:
        return self.errors == 0 and self.p95_ms is not None and self.p95_ms < self.target_ms


def percentile(values: Sequence[float], percentile_value: float) -> float:
    """Return a nearest-rank percentile, matching common latency dashboards."""
    if not values:
        raise ValueError("percentile requires at least one value")
    if percentile_value <= 0 or percentile_value > 100:
        raise ValueError("percentile must be in (0, 100]")
    ordered = sorted(values)
    rank = max(1, math.ceil(percentile_value / 100 * len(ordered)))
    return ordered[rank - 1]


def _authorization_headers(token: str) -> dict[str, str]:
    headers = {"User-Agent": "esx-gateway-performance/1.0"}
    if token:
        value = token if token.lower().startswith("bearer ") else f"Bearer {token}"
        headers["Authorization"] = value
    return headers


def _json_request(
    method: str,
    url: str,
    expected_status: int,
    token: str,
    payload: dict[str, object] | None = None,
    *,
    first_sse_token: bool = False,
    json_validator: Callable[[object], None] | None = None,
) -> RequestSpec:
    headers = _authorization_headers(token)
    headers["Accept"] = "text/event-stream" if first_sse_token else "application/json"
    body = None
    if payload is not None:
        headers["Content-Type"] = "application/json"
        body = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    return RequestSpec(
        method=method,
        url=url,
        expected_status=expected_status,
        headers=headers,
        body=body,
        first_sse_token=first_sse_token,
        json_validator=json_validator,
    )


def build_scenarios(
    base_url: str,
    token: str,
    *,
    search_keyword: str,
    target_id: int,
    assistant_message: str,
    ordinary_path: str,
) -> dict[str, Scenario]:
    base_url = base_url.rstrip("/")
    run_id = uuid.uuid4().hex

    def request_id(name: str, index: int) -> str:
        return f"perf-{name}-{run_id}-{index}-{uuid.uuid4().hex[:12]}"

    def behavior(index: int) -> RequestSpec:
        event_id = request_id("behavior", index)
        payload = {
            "anonymousId": f"perf-device-{run_id}",
            "sessionId": f"perf-session-{run_id}",
            "events": [
                {
                    "clientEventId": event_id,
                    "occurredAt": int(time.time() * 1000),
                    "action": "exposure",
                    "targetId": target_id,
                    "targetType": "post",
                    "scene": "performance",
                    "requestId": request_id("recommend", index),
                    "position": 1,
                }
            ],
        }
        return _json_request(
            "POST",
            f"{base_url}/api/v2/behavior/events",
            202,
            token,
            payload,
            json_validator=_validate_behavior_response,
        )

    def search(_index: int) -> RequestSpec:
        query = urllib.parse.urlencode(
            {"keyword": search_keyword, "page": 1, "pageSize": 20}
        )
        return _json_request(
            "GET",
            f"{base_url}/api/v2/search?{query}",
            200,
            token,
            json_validator=lambda value: _require_list_fields(
                value, "search", ("posts", "users", "tags")
            ),
        )

    def feed(index: int) -> RequestSpec:
        query = urllib.parse.urlencode(
            {
                "anonymousId": f"perf-device-{run_id}",
                "scene": "home",
                "requestId": request_id("feed", index),
                "sessionId": f"perf-session-{run_id}",
                "pageSize": 20,
            }
        )
        return _json_request(
            "GET",
            f"{base_url}/api/v2/feed/recommend?{query}",
            200,
            token,
            json_validator=lambda value: _require_list_fields(value, "feed", ("items",)),
        )

    def gateway(_index: int) -> RequestSpec:
        path = ordinary_path if ordinary_path.startswith("/") else f"/{ordinary_path}"
        return _json_request(
            "GET",
            f"{base_url}{path}",
            200,
            token,
            json_validator=lambda value: _require_list_fields(value, "gateway", ("list",)),
        )

    def assistant(index: int) -> RequestSpec:
        payload = {
            "conversationId": request_id("conversation", index),
            "message": assistant_message,
            "requestId": request_id("assistant", index),
        }
        return _json_request(
            "POST",
            f"{base_url}/api/v2/assistant/chat",
            200,
            token,
            payload,
            first_sse_token=True,
        )

    return {
        "behavior": Scenario("behavior", 100, behavior),
        "search": Scenario("search", 200, search),
        "feed": Scenario("feed", 250, feed),
        "gateway": Scenario("gateway", 400, gateway),
        "assistant": Scenario("assistant", 2000, assistant),
    }


def _require_list_fields(value: object, name: str, fields: Sequence[str]) -> None:
    if not isinstance(value, dict):
        raise ValueError(f"{name} response is not a JSON object")
    for field in fields:
        if not isinstance(value.get(field), list):
            raise ValueError(f"{name} response field {field!r} is not a list")


def _validate_behavior_response(value: object) -> None:
    if not isinstance(value, dict):
        raise ValueError("behavior response is not a JSON object")
    if value.get("acceptedCount") != 1 or value.get("rejectedCount") != 0:
        raise ValueError("behavior event was not acknowledged as accepted")


def _read_json_response(response: object) -> object:
    payload = response.read(MAX_RESPONSE_BYTES + 1)
    if len(payload) > MAX_RESPONSE_BYTES:
        raise ValueError("response exceeds 16 MiB")
    if not payload:
        raise ValueError("empty JSON response")
    return json.loads(payload)


def _read_first_sse_token(response: object) -> None:
    content_type = response.headers.get("Content-Type", "")
    if content_type.split(";", 1)[0].strip().lower() != "text/event-stream":
        raise ValueError(f"unexpected SSE content type {content_type!r}")
    while True:
        line = response.readline(64 * 1024 + 1)
        if not line:
            raise ValueError("SSE stream ended before the first token")
        if len(line) > 64 * 1024:
            raise ValueError("SSE event exceeds 64 KiB")
        if not line.startswith(b"data:"):
            continue
        event = json.loads(line[5:].strip())
        event_type = event.get("type")
        if event_type == "token" and event.get("text"):
            return
        if event_type == "error":
            code = event.get("errorCode") or "UNKNOWN"
            raise ValueError(f"assistant returned error event {code}")
        if event_type == "done":
            raise ValueError("assistant completed before the first token")


def execute_request(spec: RequestSpec, timeout_seconds: float) -> Sample:
    request = urllib.request.Request(
        spec.url,
        data=spec.body,
        headers=spec.headers,
        method=spec.method,
    )
    started = time.perf_counter()
    try:
        with urllib.request.urlopen(request, timeout=timeout_seconds) as response:
            if response.status != spec.expected_status:
                return Sample(None, f"HTTP {response.status}, expected {spec.expected_status}")
            if spec.first_sse_token:
                _read_first_sse_token(response)
            else:
                value = _read_json_response(response)
                if spec.json_validator is not None:
                    spec.json_validator(value)
        return Sample((time.perf_counter() - started) * 1000, None)
    except urllib.error.HTTPError as exc:
        return Sample(None, f"HTTP {exc.code}, expected {spec.expected_status}")
    except (
        urllib.error.URLError,
        http.client.HTTPException,
        TimeoutError,
        OSError,
        ValueError,
        json.JSONDecodeError,
    ) as exc:
        return Sample(None, f"{type(exc).__name__}: {exc}")


def run_scenario(
    scenario: Scenario,
    request_count: int,
    concurrency: int,
    timeout_seconds: float,
    warmup_count: int = 0,
) -> Summary:
    for index in range(warmup_count):
        execute_request(scenario.request(-(index + 1)), timeout_seconds)

    samples: list[Sample] = []
    worker_count = min(concurrency, request_count)
    with concurrent.futures.ThreadPoolExecutor(max_workers=worker_count) as executor:
        futures = [
            executor.submit(execute_request, scenario.request(index), timeout_seconds)
            for index in range(request_count)
        ]
        for future in concurrent.futures.as_completed(futures):
            samples.append(future.result())

    latencies = [sample.latency_ms for sample in samples if sample.latency_ms is not None]
    errors = [sample.error for sample in samples if sample.error is not None]
    return Summary(
        name=scenario.name,
        target_ms=scenario.target_ms,
        total=request_count,
        successful=len(latencies),
        errors=len(errors),
        p50_ms=statistics.median(latencies) if latencies else None,
        p95_ms=percentile(latencies, 95) if latencies else None,
        p99_ms=percentile(latencies, 99) if latencies else None,
        max_ms=max(latencies) if latencies else None,
        error_examples=tuple(errors[:3]),
    )


def _parse_scenario_names(raw: str, available: dict[str, Scenario]) -> list[str]:
    names = [name.strip().lower() for name in raw.split(",") if name.strip()]
    if names == ["all"]:
        names = list(available)
    unknown = sorted(set(names) - set(available))
    if unknown:
        raise ValueError(f"unknown scenarios: {', '.join(unknown)}")
    if not names:
        raise ValueError("at least one scenario is required")
    return list(dict.fromkeys(names))


def _optional_float(value: float | None) -> str:
    return "-" if value is None else f"{value:.1f}"


def print_human(summaries: Sequence[Summary]) -> None:
    print(
        f"{'scenario':<10} {'ok/total':>10} {'errors':>7} {'p50 ms':>9} "
        f"{'p95 ms':>9} {'p99 ms':>9} {'max ms':>9} {'target':>9} {'result':>7}"
    )
    for summary in summaries:
        result = "PASS" if summary.passed else "FAIL"
        print(
            f"{summary.name:<10} {summary.successful:>4}/{summary.total:<5} "
            f"{summary.errors:>7} {_optional_float(summary.p50_ms):>9} "
            f"{_optional_float(summary.p95_ms):>9} {_optional_float(summary.p99_ms):>9} "
            f"{_optional_float(summary.max_ms):>9} {summary.target_ms:>8.0f}ms {result:>7}"
        )
        for error in summary.error_examples:
            print(f"  error: {error}")


def print_json(summaries: Sequence[Summary]) -> None:
    documents = []
    for summary in summaries:
        document = dataclasses.asdict(summary)
        document["passed"] = summary.passed
        documents.append(document)
    print(json.dumps(documents, indent=2, sort_keys=True))


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description=(
            "Run isolated latency gates against a live Gateway. Request counts are per "
            "scenario; Assistant requests may invoke a paid LLM."
        )
    )
    parser.add_argument(
        "--base-url",
        default=os.environ.get("PERF_GATEWAY_BASE_URL", DEFAULT_BASE_URL),
        help="Gateway or Nginx base URL (env: PERF_GATEWAY_BASE_URL)",
    )
    parser.add_argument(
        "--token",
        default=os.environ.get("PERF_GATEWAY_TOKEN", ""),
        help="JWT; prefer PERF_GATEWAY_TOKEN to avoid shell history",
    )
    parser.add_argument(
        "--scenarios",
        default=os.environ.get("PERF_GATEWAY_SCENARIOS", DEFAULT_SCENARIOS),
        help="comma-separated behavior,search,feed,gateway,assistant or all",
    )
    parser.add_argument("--concurrency", type=int, default=8)
    parser.add_argument("--requests", type=int, default=20)
    parser.add_argument("--warmup", type=int, default=0)
    parser.add_argument("--timeout", type=float, default=10.0)
    parser.add_argument("--search-keyword", default="test")
    parser.add_argument("--target-id", type=int, default=1)
    parser.add_argument(
        "--ordinary-path", default="/api/v1/posts?page=1&pageSize=20&sortBy=1"
    )
    parser.add_argument(
        "--assistant-message", default="Summarize the current feed in one sentence."
    )
    parser.add_argument("--json", action="store_true", help="emit machine-readable output")
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    parsed_base_url = urllib.parse.urlparse(args.base_url)
    if parsed_base_url.scheme not in {"http", "https"} or not parsed_base_url.netloc:
        parser.error("--base-url must be an absolute http(s) URL")
    if args.concurrency <= 0 or args.requests <= 0 or args.warmup < 0 or args.timeout <= 0:
        parser.error("concurrency, requests, and timeout must be positive; warmup cannot be negative")
    if args.target_id <= 0:
        parser.error("--target-id must be positive")

    scenarios = build_scenarios(
        args.base_url,
        args.token.strip(),
        search_keyword=args.search_keyword,
        target_id=args.target_id,
        assistant_message=args.assistant_message,
        ordinary_path=args.ordinary_path,
    )
    try:
        selected_names = _parse_scenario_names(args.scenarios, scenarios)
    except ValueError as exc:
        parser.error(str(exc))
    if "assistant" in selected_names and not args.token.strip():
        parser.error("assistant scenario requires --token or PERF_GATEWAY_TOKEN")

    summaries = [
        run_scenario(
            scenarios[name],
            request_count=args.requests,
            concurrency=args.concurrency,
            timeout_seconds=args.timeout,
            warmup_count=args.warmup,
        )
        for name in selected_names
    ]
    if args.json:
        print_json(summaries)
    else:
        print_human(summaries)
    return 0 if all(summary.passed for summary in summaries) else 1


if __name__ == "__main__":
    sys.exit(main())
