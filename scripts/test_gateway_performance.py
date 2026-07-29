#!/usr/bin/env python3

import dataclasses
import json
import threading
import time
import unittest
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from scripts import gateway_performance


class _GatewayHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    requests = []
    lock = threading.Lock()

    def log_message(self, _format, *_args):
        pass

    def _record(self, body=b""):
        with self.lock:
            self.requests.append(
                {
                    "method": self.command,
                    "path": self.path,
                    "authorization": self.headers.get("Authorization"),
                    "body": body,
                }
            )

    def _json(self, status, payload):
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        self._record()
        parsed = urllib.parse.urlparse(self.path)
        if parsed.path == "/api/v2/search":
            self._json(200, {"posts": [], "users": [], "tags": []})
        elif parsed.path == "/api/v2/feed/recommend":
            self._json(200, {"items": [], "hasMore": False, "requestId": "test"})
        elif parsed.path == "/api/v1/posts":
            self._json(200, {"list": [], "total": 0, "page": 1, "pageSize": 20})
        else:
            self._json(404, {"error": "not found"})

    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        self._record(body)
        if self.path == "/api/v2/behavior/events":
            self._json(202, {"results": [], "acceptedCount": 1, "rejectedCount": 0})
            return
        if self.path != "/api/v2/assistant/chat":
            self._json(404, {"error": "not found"})
            return

        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream; charset=utf-8")
        self.send_header("Cache-Control", "no-cache")
        self.end_headers()
        self.wfile.write(b'data: {"type":"source","source":{"sourceId":"1"}}\n\n')
        self.wfile.flush()
        time.sleep(0.025)
        self.wfile.write(b'data: {"type":"token","text":"ready"}\n\n')
        self.wfile.flush()
        self.close_connection = True


class GatewayPerformanceTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        _GatewayHandler.requests = []
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), _GatewayHandler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.base_url = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join(timeout=2)

    def setUp(self):
        with _GatewayHandler.lock:
            _GatewayHandler.requests.clear()

    def test_percentile_uses_nearest_rank(self):
        self.assertEqual(gateway_performance.percentile([1, 2, 3, 4], 95), 4)
        self.assertEqual(gateway_performance.percentile([3, 1, 2], 50), 2)
        with self.assertRaises(ValueError):
            gateway_performance.percentile([], 95)

    def test_live_scenarios_send_valid_requests_and_measure_first_token(self):
        scenarios = gateway_performance.build_scenarios(
            self.base_url,
            "test-token",
            search_keyword="little box",
            target_id=42,
            assistant_message="status",
            ordinary_path="/api/v1/posts?page=1&pageSize=20&sortBy=1",
        )
        summaries = {
            name: gateway_performance.run_scenario(
                scenarios[name], request_count=2, concurrency=2, timeout_seconds=2
            )
            for name in scenarios
        }

        self.assertTrue(all(summary.passed for summary in summaries.values()))
        self.assertGreaterEqual(summaries["assistant"].p50_ms, 20)
        with _GatewayHandler.lock:
            requests = list(_GatewayHandler.requests)
        self.assertEqual(len(requests), 10)
        self.assertTrue(all(item["authorization"] == "Bearer test-token" for item in requests))

        behavior_bodies = [
            json.loads(item["body"])
            for item in requests
            if item["path"] == "/api/v2/behavior/events"
        ]
        self.assertEqual(len(behavior_bodies), 2)
        event_ids = {body["events"][0]["clientEventId"] for body in behavior_bodies}
        self.assertEqual(len(event_ids), 2)
        self.assertTrue(
            all(body["events"][0]["requestId"] for body in behavior_bodies)
        )

    def test_threshold_or_request_error_fails_summary(self):
        slow = gateway_performance.Scenario(
            name="slow",
            target_ms=0.01,
            request=lambda _index: gateway_performance.RequestSpec(
                method="GET",
                url=f"{self.base_url}/api/v1/posts",
                expected_status=200,
                headers={"Accept": "application/json"},
            ),
        )
        summary = gateway_performance.run_scenario(
            slow, request_count=2, concurrency=1, timeout_seconds=2
        )
        self.assertFalse(summary.passed)
        self.assertEqual(summary.errors, 0)

        missing = dataclasses.replace(
            slow,
            name="missing",
            target_ms=1000,
            request=lambda _index: dataclasses.replace(
                slow.request(0), url=f"{self.base_url}/missing"
            ),
        )
        failed = gateway_performance.run_scenario(
            missing, request_count=1, concurrency=1, timeout_seconds=2
        )
        self.assertFalse(failed.passed)
        self.assertEqual(failed.errors, 1)


if __name__ == "__main__":
    unittest.main()
