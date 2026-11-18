"""Local HID observation server. Never opens serial or sends phone input.

Based on ESP32 iPhone Agent by Henry Yuan, MIT (repository LICENSE).
"""
import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlsplit, parse_qs
import uuid

VERSION = 1
MAX_BODY = 65536
EVENT_TYPES = {"loaded", "click", "pointerdown", "pointerup", "pointercancel",
               "pointermove", "wheel", "scroll", "input", "keydown", "keyup", "mode"}


class Invalid(Exception):
    def __init__(self, status, message):
        self.status, self.message = status, message


def integer(value, low=0, high=1000000):
    return type(value) is int and low <= value <= high


def finite(value):
    return type(value) in (int, float) and math.isfinite(value)


class Store:
    def __init__(self, output):
        self.output = Path(output)
        self.output.mkdir(parents=True, exist_ok=False)
        self.run_id = uuid.uuid4().hex
        self.pages = {}
        self.lock = threading.Lock()
        self.failed = False
        self.append({"kind": "run", "version": VERSION, "run_id": self.run_id})

    def append(self, record):
        if self.failed:
            raise Invalid(503, "recording failed; start a new run")
        try:
            with (self.output / "events.jsonl").open("a", encoding="utf-8") as f:
                f.write(json.dumps({**record, "received_time": time.time()}, ensure_ascii=False,
                                   allow_nan=False) + "\n")
                f.flush()
                os.fsync(f.fileno())
        except OSError:
            self.failed = True
            raise Invalid(503, "recording failed; evidence incomplete")

    def session(self):
        with self.lock:
            if len(self.pages) >= 100:
                raise Invalid(503, "page limit reached; start a new run")
            page = uuid.uuid4().hex
            self.append({"kind": "page", "run_id": self.run_id, "page_id": page})
            self.pages[page] = {"ack": 0, "hashes": {}, "last_seen": time.monotonic(),
                                "pending": 0, "lost": 0, "gap": False, "state": None}
            return {"version": VERSION, "run_id": self.run_id, "page_id": page}

    def identify(self, data):
        if not isinstance(data, dict) or type(data.get("version")) is not int or data["version"] != VERSION:
            raise Invalid(400, "invalid schema version")
        if data.get("run_id") != self.run_id or not isinstance(data.get("page_id"), str) or data["page_id"] not in self.pages:
            raise Invalid(409, "unknown run or page; reload page")
        return self.pages[data["page_id"]]

    @staticmethod
    def state(data):
        state = data.get("state")
        if not isinstance(state, dict):
            raise Invalid(400, "state required")
        if not integer(state.get("clicks")) or not isinstance(state.get("text"), str) or len(state["text"]) > 8192:
            raise Invalid(400, "invalid text or click state")
        if not finite(state.get("scrollY")) or state.get("mode") not in ("action", "static"):
            raise Invalid(400, "invalid scroll or mode")
        for key in ("viewport", "targets"):
            if not isinstance(state.get(key), dict):
                raise Invalid(400, "geometry required")
        return state

    def event(self, data):
        with self.lock:
            p = self.identify(data)
            if set(data) != {"version", "run_id", "page_id", "seq", "type", "client_time", "monotonic_ms", "event", "state"} or not isinstance(data["event"], dict):
                raise Invalid(400, "invalid event fields")
            state = self.state(data)
            seq = data.get("seq")
            if not integer(seq, 1, 100000) or data.get("type") not in EVENT_TYPES:
                raise Invalid(400, "invalid event type or sequence")
            if not finite(data.get("client_time")) or not finite(data.get("monotonic_ms")):
                raise Invalid(400, "client timestamps required")
            digest = hashlib.sha256(json.dumps(data, sort_keys=True, ensure_ascii=False,
                                               allow_nan=False).encode()).hexdigest()
            if seq <= p["ack"]:
                if p["hashes"].get(seq) != digest:
                    self.append({"kind": "conflict", "page_id": data["page_id"], "seq": seq})
                    p["gap"] = True
                    raise Invalid(409, "conflicting duplicate event")
                return {"ack": p["ack"], "duplicate": True}
            if seq != p["ack"] + 1:
                self.append({"kind": "gap", "page_id": data["page_id"], "expected": p["ack"] + 1, "got": seq})
                p["gap"] = True
                raise Invalid(409, "sequence gap; evidence incomplete")
            self.append({"kind": "event", **data})
            p["hashes"][seq] = digest
            p.update(ack=seq, last_seen=time.monotonic(), state=state)
            return {"ack": seq, "duplicate": False}

    def heartbeat(self, data):
        with self.lock:
            p = self.identify(data)
            if set(data) != {"version", "run_id", "page_id", "last_seq", "pending", "lost", "client_time", "monotonic_ms", "state"}:
                raise Invalid(400, "invalid heartbeat fields")
            state = self.state(data)
            for key in ("pending", "lost", "last_seq"):
                if not integer(data.get(key)):
                    raise Invalid(400, "invalid heartbeat counters")
            if not finite(data["client_time"]) or not finite(data["monotonic_ms"]):
                raise Invalid(400, "client timestamps required")
            if data["last_seq"] - p["ack"] > data["pending"] + data["lost"]:
                raise Invalid(409, "heartbeat counters do not match acknowledged events")
            self.append({"kind": "heartbeat", **data})
            p.update(last_seen=time.monotonic(), pending=data["pending"],
                     lost=max(p["lost"], data["lost"]), state=state)
            return {"ack": p["ack"]}

    def status(self, page):
        with self.lock:
            if page not in self.pages:
                raise Invalid(404, "unknown page")
            p = self.pages[page]
            age = time.monotonic() - p["last_seen"]
            reasons = []
            if age > 4: reasons.append("page offline or stale")
            if p["pending"]: reasons.append("unacknowledged events")
            if p["lost"]: reasons.append("client queue overflow")
            if p["gap"]: reasons.append("sequence gap observed")
            if self.failed: reasons.append("storage failed")
            if p["state"] is None: reasons.append("no observed state")
            return {"run_id": self.run_id, "page_id": page, "ack": p["ack"],
                    "age_seconds": age, "reasons": reasons, "evidence": "insufficient" if reasons else "current",
                    "state": p["state"], "note": "current is not proof of a complete past observation window"}


def make_server(host, port, output):
    # Bind before creating the run directory: a busy port leaves no fake run.
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def respond(self, code, body, content_type="application/json"):
            if not isinstance(body, bytes):
                body = json.dumps(body, ensure_ascii=False, allow_nan=False).encode()
            self.send_response(code)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Cache-Control", "no-store")
            self.send_header("X-Content-Type-Options", "nosniff")
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            path = urlsplit(self.path)
            try:
                if path.path in ("/", "/app.js"):
                    name = "index.html" if path.path == "/" else "app.js"
                    mime = "text/html; charset=utf-8" if name.endswith("html") else "text/javascript; charset=utf-8"
                    self.respond(200, (Path(__file__).parent / "web" / name).read_bytes(), mime)
                elif path.path == "/session":
                    self.respond(200, self.server.store.session())
                elif path.path == "/status":
                    self.respond(200, self.server.store.status(parse_qs(path.query).get("page_id", [""])[0]))
                elif path.path == "/events":
                    with self.server.store.lock:
                        raw = (self.server.store.output / "events.jsonl").read_bytes()
                    self.respond(200, raw, "application/x-ndjson")
                else:
                    self.respond(404, {"error": "unknown path"})
            except Invalid as e:
                self.respond(e.status, {"error": e.message})

        def do_POST(self):
            try:
                if self.path not in ("/event", "/heartbeat"):
                    raise Invalid(404, "unknown path")
                if self.headers.get_content_type() != "application/json":
                    raise Invalid(415, "application/json required")
                origin = self.headers.get("Origin")
                if origin and urlsplit(origin).netloc != self.headers.get("Host"):
                    raise Invalid(403, "cross-origin request rejected")
                try:
                    size = int(self.headers.get("Content-Length", "0"))
                except ValueError:
                    raise Invalid(400, "invalid Content-Length")
                if not 0 < size <= MAX_BODY or self.headers.get("Transfer-Encoding"):
                    raise Invalid(413, "invalid body length")
                self.connection.settimeout(3)
                raw = self.rfile.read(size)
                if len(raw) != size:
                    raise Invalid(400, "truncated body")
                def reject_constant(_):
                    raise ValueError("non-finite number")
                try:
                    data = json.loads(raw, parse_constant=reject_constant)
                    json.dumps(data, allow_nan=False)
                except (ValueError, UnicodeError):
                    raise Invalid(400, "invalid JSON")
                method = self.server.store.event if self.path == "/event" else self.server.store.heartbeat
                self.respond(200, method(data))
            except Invalid as e:
                self.respond(e.status, {"error": e.message})
            except (TimeoutError, OSError):
                self.close_connection = True

        def do_PUT(self): self.respond(405, {"error": "method not allowed"})
        do_DELETE = do_PUT

    server = ThreadingHTTPServer((host, port), Handler)
    server.daemon_threads = True
    try:
        server.store = Store(output)
    except Exception:
        server.server_close()
        raise
    return server


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", required=True, help="PC LAN address reachable from the phone")
    parser.add_argument("--port", type=int, default=8905)
    parser.add_argument("--output", type=Path, required=True, help="new directory for this run")
    args = parser.parse_args()
    with make_server(args.host, args.port, args.output) as server:
        print(json.dumps({"url": f"http://{args.host}:{server.server_port}/", "run_id": server.store.run_id,
                          "output": str(args.output.resolve())}), flush=True)
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            pass


if __name__ == "__main__":
    main()
