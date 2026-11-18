"""Offline HTTP evidence contract tests; no device access."""
import concurrent.futures
import http.client
import json
from pathlib import Path
import tempfile
import threading
import unittest
from unittest.mock import patch

from serve import make_server, MAX_BODY


class Contract(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.output = Path(self.temp.name) / "run"
        self.server = make_server("127.0.0.1", 0, self.output)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.session = self.request("GET", "/session")[1]

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()
        self.temp.cleanup()

    def request(self, method, path, data=None, headers=None):
        conn = http.client.HTTPConnection("127.0.0.1", self.server.server_port, timeout=5)
        body = json.dumps(data).encode() if data is not None else None
        conn.request(method, path, body, headers or ({"Content-Type": "application/json"} if body else {}))
        response = conn.getresponse()
        raw = response.read()
        result = json.loads(raw) if response.getheader("Content-Type", "").startswith("application/json") else raw
        conn.close()
        return response.status, result

    def event(self, **overrides):
        return {**self.session, "seq": 1, "type": "click", "client_time": 1000,
                "monotonic_ms": 20, "event": {"buttons": 0, "isTrusted": True},
                "state": {"clicks": 1, "text": "a_A 1", "scrollY": 0, "mode": "static",
                          "viewport": {"width": 390}, "targets": {"box": {"x": 25}}}, **overrides}

    def heartbeat(self, **overrides):
        event = self.event()
        for key in ("seq", "type", "event"):
            del event[key]
        return {**event, "last_seq": 1, "pending": 0, "lost": 0, **overrides}

    def status(self):
        return self.request("GET", "/status?page_id=" + self.session["page_id"])[1]

    def test_paths_methods_and_assets(self):
        self.assertIn(b"Handsy HID", self.request("GET", "/")[1])
        self.assertIn(b"/heartbeat", self.request("GET", "/app.js")[1])
        self.assertEqual(self.request("GET", "/missing")[0], 404)
        self.assertEqual(self.request("GET", "/../../serve.py")[0], 404)
        self.assertEqual(self.request("POST", "/missing", {})[0], 404)
        self.assertEqual(self.request("PUT", "/event", {})[0], 405)

    def test_durable_ack_duplicate_and_conflict(self):
        event = self.event()
        self.assertEqual(self.request("POST", "/event", event), (200, {"ack": 1, "duplicate": False}))
        self.assertEqual(self.request("POST", "/event", event), (200, {"ack": 1, "duplicate": True}))
        self.assertEqual(self.request("POST", "/event", self.event(type="input"))[0], 409)
        rows = [json.loads(line) for line in (self.output / "events.jsonl").read_text().splitlines()]
        self.assertEqual(len([r for r in rows if r["kind"] == "event"]), 1)
        self.assertEqual(next(r for r in rows if r["kind"] == "event")["state"]["text"], "a_A 1")
        self.assertIn("received_time", rows[-1])

    def test_concurrent_duplicate_is_once(self):
        with concurrent.futures.ThreadPoolExecutor(2) as pool:
            results = list(pool.map(lambda _: self.request("POST", "/event", self.event()), range(2)))
        self.assertEqual(sorted(r[1]["duplicate"] for r in results), [False, True])

    def test_page_isolation_old_run_and_invalid_fields(self):
        second = self.request("GET", "/session")[1]
        self.assertEqual(self.request("POST", "/event", self.event(**second))[0], 200)
        self.assertEqual(self.status()["ack"], 0)
        for changes in ({"run_id": "old"}, {"page_id": []}, {"page_id": "old"}):
            self.assertEqual(self.request("POST", "/event", self.event(**changes))[0], 409)
        for changes in ({"seq": True}, {"seq": 0}, {"seq": 1.2}, {"type": "arbitrary"},
                        {"version": True}, {"state": {}}, {"event": []}, {"kind": "heartbeat"},
                        {"monotonic_ms": "20"}, {"client_time": float("nan")}):
            self.assertEqual(self.request("POST", "/event", self.event(**changes))[0], 400, changes)

    def test_negative_evidence_requires_online_complete_state(self):
        self.assertEqual(self.status()["evidence"], "insufficient")
        self.assertEqual(self.request("POST", "/event", self.event())[0], 200)
        self.assertEqual(self.request("POST", "/heartbeat", self.heartbeat())[0], 200)
        self.assertEqual(self.status()["evidence"], "current")
        self.request("POST", "/heartbeat", self.heartbeat(pending=1, last_seq=2))
        self.assertIn("unacknowledged events", self.status()["reasons"])
        self.request("POST", "/heartbeat", self.heartbeat(lost=1, last_seq=2))
        self.assertIn("client queue overflow", self.status()["reasons"])
        self.server.store.pages[self.session["page_id"]]["last_seen"] -= 5
        self.assertIn("page offline or stale", self.status()["reasons"])

    def test_gap_is_not_erased_by_later_recovery(self):
        self.assertEqual(self.request("POST", "/event", self.event(seq=2))[0], 409)
        self.request("POST", "/event", self.event())
        self.request("POST", "/heartbeat", self.heartbeat())
        self.assertIn("sequence gap observed", self.status()["reasons"])

    def test_storage_failure_never_acknowledges_and_latches(self):
        with patch("serve.os.fsync", side_effect=OSError("disk full")):
            self.assertEqual(self.request("POST", "/event", self.event())[0], 503)
        self.assertEqual(self.status()["ack"], 0)
        self.assertEqual(self.request("POST", "/event", self.event())[0], 503)
        self.assertIn("storage failed", self.status()["reasons"])

    def test_bad_json_body_size_origin_and_recovery(self):
        for raw in (b"{", b"null", b"[]", b'"x"', b'\xff'):
            conn = http.client.HTTPConnection("127.0.0.1", self.server.server_port)
            conn.request("POST", "/event", raw, {"Content-Type": "application/json"})
            result = conn.getresponse(); self.assertEqual(result.status, 400); result.read(); conn.close()
        self.assertEqual(self.request("POST", "/event", self.event(), {"Content-Type": "text/plain"})[0], 415)
        self.assertEqual(self.request("POST", "/event", self.event(), {"Content-Type": "application/json", "Origin": "http://elsewhere"})[0], 403)
        self.assertEqual(self.request("POST", "/event", {"text": "x" * MAX_BODY})[0], 413)
        self.assertEqual(self.request("POST", "/event", self.event())[0], 200)

    def test_existing_run_is_not_overwritten(self):
        before = (self.output / "events.jsonl").read_bytes()
        with self.assertRaises(FileExistsError):
            make_server("127.0.0.1", 0, self.output)
        self.assertEqual((self.output / "events.jsonl").read_bytes(), before)


if __name__ == "__main__":
    unittest.main()
