"""Webhook sink for the marketplace demo stack.

Records every POST body Pulse's webhook alert channel delivers, so the demo can
show (and the capture script can assert) that a fired alert actually left Pulse.

GET  /received  -> JSON array of everything received so far (newest last)
POST /<any>     -> 200, body + headers appended to the log

Stdlib only; runs in python:3.12-slim. Local demo use only — not a product component.
"""

import json
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

LOG_PATH = "/var/log/sink/received.jsonl"
_lock = threading.Lock()
_received = []


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(min(length, 1_000_000))
        try:
            body = json.loads(raw.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError):
            body = raw.decode("utf-8", errors="replace")
        entry = {
            "received_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "path": self.path,
            "content_type": self.headers.get("Content-Type"),
            "signature_present": bool(self.headers.get("X-Pulse-Signature")),
            "body": body,
        }
        with _lock:
            _received.append(entry)
            with open(LOG_PATH, "a", encoding="utf-8") as fh:
                fh.write(json.dumps(entry) + "\n")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"ok":true}')

    def do_GET(self):
        if self.path != "/received":
            self.send_response(404)
            self.end_headers()
            return
        with _lock:
            payload = json.dumps(_received).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, fmt, *args):  # keep container logs to one line per hit
        print("webhook-sink:", self.command, self.path, flush=True)


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
