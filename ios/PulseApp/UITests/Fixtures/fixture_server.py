#!/usr/bin/env python3
"""
Fixture HTTP server for PulseApp UI tests.

Serves realistic JSON responses for all endpoints that PulseAPIClient calls:
  - /healthz (unauthenticated)
  - /auth/me
  - /api/v1/live/overview
  - /api/v1/live/streams
  - /api/v1/alerts/history
  - /api/v1/fleet/nodes
  - /api/v1/anomalies
  - /api/v1/qoe/summary
  - /api/v1/admin/license

AUTHENTICATION:
  - Any non-empty Bearer token is accepted (401 if missing/empty).
  - /healthz is unauthenticated per the OpenAPI spec.

FIXTURE VALIDATION:
  All fixture bodies are derived from contracts/openapi/pulse-api.yaml schemas and
  cross-checked against ios/PulseKit/Sources/PulseKit/Models/*.swift decoders.
  Existing test fixtures in ios/PulseKit/Tests/PulseKitTests/Fixtures/ were used
  as reference for realistic values. Each fixture includes:
  - Required fields per the OpenAPI schema
  - Realistic values (multiple streams, both healthy and degraded nodes, etc.)
  - Varied data to exercise different UI states (firing vs resolved alerts, etc.)

USAGE:
  python3 fixture_server.py [--port 8090]

The server binds to localhost only. Set FIXTURE_SERVER_READY_FILE to a path;
the server will touch that file when it is ready to accept connections — use this
for CI readiness checks instead of sleeping.
"""

import argparse
import json
import os
import sys
from http.server import HTTPServer, BaseHTTPRequestHandler


class FastBindHTTPServer(HTTPServer):
    """HTTPServer that does not stall for ~35 seconds on bind.

    http.server's server_bind() calls socket.getfqdn() to populate server_name.
    On a GitHub macOS runner that reverse lookup blocks until a DNS timeout: the
    server printed "Listening" 36 SECONDS after start, having sailed past CI's
    30-second readiness wait, so the job failed on a server that was in fact
    about to work. Nothing was wrong with the fixtures or the app.

    We know the name — we bound it ourselves — so skip the lookup entirely.
    """

    def server_bind(self):
        # Deliberately NOT calling HTTPServer.server_bind(); that is the method
        # containing the getfqdn() call. Go straight to its parent and set the
        # two attributes it would have derived.
        import socketserver

        socketserver.TCPServer.server_bind(self)
        host, port = self.server_address[:2]
        self.server_name = "localhost"
        self.server_port = port
from pathlib import Path
from urllib.parse import urlparse, parse_qs
import time

# -- Fixture Data ----------------------------------------------------------------
#
# Each fixture is validated against the OpenAPI spec in contracts/openapi/pulse-api.yaml.
# Field names use snake_case per the API contract; the Swift models decode these
# using CodingKeys to camelCase.

# Current timestamp for all responses (Unix epoch ms).
NOW_MS = int(time.time() * 1000)

# /healthz — lines 979-1010 in pulse-api.yaml
# HealthStatus schema: lines 2996-3035
# Status: "ok" or "degraded" for the server, per-component status includes latency.
HEALTHZ = {
    "status": "ok",
    "ams_env_configured": True,
    "components": {
        "clickhouse": {
            "status": "ok",
            "latency_ms": 5
        },
        "meta_store": {
            "status": "ok",
            "latency_ms": 2,
            "message": None
        },
        "collector": {
            "status": "ok",
            "message": None
        },
        "kafka": {
            "status": "ok",
            "latency_ms": 10,
            "lag": 50,
            "parse_errors": 0
        }
    }
}

# /auth/me — lines 1206-1239 in pulse-api.yaml
# Response: name, role, auth_method (bearer/cookie)
AUTH_ME = {
    "name": "operator@example.com",
    "role": "admin",
    "auth_method": "bearer"
}

# /api/v1/live/overview — lines 77-98 in pulse-api.yaml
# LiveOverview schema: lines 1886-1906
# ProtocolMix schema: lines 1908-1926
# AppOverview schema: lines 1928-1939
# NodeHealth schema: lines 1941-1965
LIVE_OVERVIEW = {
    "ts": NOW_MS,
    "total_viewers": 2847,
    "total_publishers": 42,
    "protocol_mix": {
        "webrtc": 1250,
        "hls": 1400,
        "rtmp": 150,
        "dash": 47,
        "other": 0
    },
    "apps": [
        {
            "app": "live",
            "viewers": 2100,
            "publishers": 35,
            "streams": 18
        },
        {
            "app": "webinar",
            "viewers": 580,
            "publishers": 5,
            "streams": 4
        },
        {
            "app": "events",
            "viewers": 167,
            "publishers": 2,
            "streams": 2
        }
    ],
    "nodes": [
        {
            "node_id": "origin-us-east-1",
            "role": "origin",
            "status": "up",
            "last_seen": NOW_MS,
            "cpu_pct": 38.5,
            "mem_pct": 52.0,
            "version": "3.0.3"
        },
        {
            "node_id": "edge-eu-west-1",
            "role": "edge",
            "status": "up",
            "last_seen": NOW_MS - 2000,
            "cpu_pct": 65.2,
            "mem_pct": 71.8,
            "version": "3.0.3"
        },
        {
            "node_id": "edge-ap-south-1",
            "role": "edge",
            "status": "degraded",
            "last_seen": NOW_MS - 45000,
            "cpu_pct": 92.5,
            "mem_pct": 88.0,
            "version": "3.0.2"
        }
    ]
}

# /api/v1/live/streams — lines 100-123 in pulse-api.yaml
# LiveStreamList schema: lines 1967-1976
# LiveStream schema: lines 1978-2024
# PublisherState enum: publishing, idle, offline
LIVE_STREAMS = {
    "items": [
        {
            "stream_id": "keynote-2026-main",
            "app": "live",
            "node_id": "origin-us-east-1",
            "tenant": "acme-corp",
            "viewers": 1250,
            "publisher_state": "publishing",
            "health_score": 98.5,
            "protocol_mix": {
                "webrtc": 800,
                "hls": 400,
                "rtmp": 50,
                "dash": 0,
                "other": 0
            },
            "bitrate_kbps": 4500.0,
            "started_at": NOW_MS - 3600000,
            "viewer_rtt_ms": 32.5,
            "viewer_jitter_ms": 4.2,
            "viewer_loss_pct": 0.1
        },
        {
            "stream_id": "product-launch-backup",
            "app": "live",
            "node_id": "origin-us-east-1",
            "viewers": 450,
            "publisher_state": "publishing",
            "health_score": 95.0,
            "protocol_mix": {
                "webrtc": 250,
                "hls": 200,
                "dash": 0,
                "rtmp": 0,
                "other": 0
            },
            "bitrate_kbps": 3200.0,
            "started_at": NOW_MS - 1800000
        },
        {
            "stream_id": "webinar-ai-trends",
            "app": "webinar",
            "node_id": "edge-eu-west-1",
            "tenant": "techconf",
            "viewers": 320,
            "publisher_state": "publishing",
            "health_score": 88.0,
            "protocol_mix": {
                "webrtc": 100,
                "hls": 220,
                "dash": 0,
                "rtmp": 0,
                "other": 0
            },
            "bitrate_kbps": 2800.0,
            "started_at": NOW_MS - 2400000,
            "viewer_rtt_ms": 85.0,
            "viewer_jitter_ms": 12.5,
            "viewer_loss_pct": 0.8
        },
        {
            "stream_id": "interview-ceo",
            "app": "webinar",
            "viewers": 180,
            "publisher_state": "idle",
            "health_score": 75.0,
            "bitrate_kbps": 0.0,
            "started_at": NOW_MS - 7200000
        },
        {
            "stream_id": "live-concert-stage-b",
            "app": "events",
            "node_id": "edge-ap-south-1",
            "viewers": 95,
            "publisher_state": "publishing",
            "health_score": 62.0,
            "protocol_mix": {
                "webrtc": 40,
                "hls": 55,
                "dash": 0,
                "rtmp": 0,
                "other": 0
            },
            "bitrate_kbps": 1800.0,
            "started_at": NOW_MS - 900000,
            "viewer_rtt_ms": 180.0,
            "viewer_jitter_ms": 25.0,
            "viewer_loss_pct": 2.5
        },
        {
            "stream_id": "test-stream-offline",
            "app": "live",
            "viewers": 0,
            "publisher_state": "offline",
            "health_score": 0.0
        }
    ],
    "meta": {
        "next_cursor": None,
        "total": 6
    }
}

# /api/v1/alerts/history — lines 534-564 in pulse-api.yaml
# AlertHistoryList schema: lines 2542-2551
# AlertHistoryEntry schema: lines 2553-2585
# AlertState enum: firing, resolved, delivery_failure
# AlertSeverity enum: info, warning, critical
ALERTS_HISTORY = {
    "items": [
        {
            "id": "alert-f8c2e1a3",
            "rule_id": "rule-rebuffer-threshold",
            "state": "firing",
            "severity": "critical",
            "ts": NOW_MS - 120000,
            "metric": "rebuffer_ratio",
            "value": 0.18,
            "threshold": 0.10,
            "scope": {
                "node_id": "edge-ap-south-1",
                "app": "events",
                "stream_id": "live-concert-stage-b",
                "tenant": None
            },
            "cooldown_until": NOW_MS + 180000,
            "group_key": "live-concert-stage-b"
        },
        {
            "id": "alert-b4d9f0e2",
            "rule_id": "rule-cpu-high",
            "state": "firing",
            "severity": "warning",
            "ts": NOW_MS - 300000,
            "metric": "cpu_pct",
            "value": 92.5,
            "threshold": 85.0,
            "scope": {
                "node_id": "edge-ap-south-1",
                "app": None,
                "stream_id": None,
                "tenant": None
            },
            "cooldown_until": None,
            "group_key": "edge-ap-south-1"
        },
        {
            "id": "alert-9a1b3c5d",
            "rule_id": "rule-viewer-drop",
            "state": "resolved",
            "severity": "warning",
            "ts": NOW_MS - 1800000,
            "metric": "viewer_count",
            "value": 50.0,
            "threshold": 100.0,
            "scope": {
                "app": "webinar",
                "stream_id": "interview-ceo",
                "node_id": None,
                "tenant": None
            },
            "cooldown_until": None,
            "group_key": None
        },
        {
            "id": "alert-2e4f6a8c",
            "rule_id": "rule-bitrate-low",
            "state": "resolved",
            "severity": "info",
            "ts": NOW_MS - 3600000,
            "metric": "bitrate_kbps",
            "value": 800.0,
            "threshold": 1000.0,
            "scope": {
                "app": "live",
                "stream_id": None,
                "node_id": None,
                "tenant": None
            },
            "cooldown_until": None,
            "group_key": None
        },
        {
            "id": "alert-d7e9f1b3",
            "rule_id": "rule-error-rate",
            "state": "delivery_failure",
            "severity": "critical",
            "ts": NOW_MS - 7200000,
            "metric": "error_rate",
            "value": 0.05,
            "threshold": 0.02,
            "scope": {
                "app": None,
                "stream_id": None,
                "node_id": None,
                "tenant": "acme-corp"
            },
            "cooldown_until": None,
            "group_key": None
        }
    ],
    "meta": {
        "next_cursor": None,
        "total": 5
    }
}

# /api/v1/fleet/nodes — lines 728-750 in pulse-api.yaml
# FleetNodeList schema: lines 2707-2716
# FleetNode schema: lines 2718-2760
# NodeRole enum: origin, edge, standalone
# NodeStatus enum: up, degraded (D-090: "down" removed)
FLEET_NODES = {
    "items": [
        {
            "node_id": "origin-us-east-1",
            "role": "origin",
            "status": "up",
            "last_seen": NOW_MS,
            "version": "3.0.3",
            "cpu_pct": 38.5,
            "mem_pct": 52.0,
            "net_in_mbps": 250.5,
            "net_out_mbps": 1200.0,
            "os_name": "Ubuntu 22.04.4 LTS",
            "os_arch": "amd64",
            "java_version": "17.0.10",
            "processor_count": 16
        },
        {
            "node_id": "edge-eu-west-1",
            "role": "edge",
            "status": "up",
            "last_seen": NOW_MS - 2000,
            "version": "3.0.3",
            "cpu_pct": 65.2,
            "mem_pct": 71.8,
            "net_in_mbps": 180.0,
            "net_out_mbps": 850.0,
            "os_name": "Debian 12",
            "os_arch": "amd64",
            "java_version": "17.0.10",
            "processor_count": 8
        },
        {
            "node_id": "edge-ap-south-1",
            "role": "edge",
            "status": "degraded",
            "last_seen": NOW_MS - 45000,
            "version": "3.0.2",
            "cpu_pct": 92.5,
            "mem_pct": 88.0,
            "net_in_mbps": 45.0,
            "net_out_mbps": 320.0,
            "os_name": "Ubuntu 22.04.3 LTS",
            "os_arch": "arm64",
            "java_version": "17.0.9",
            "processor_count": 4
        },
        {
            "node_id": "standalone-dev-local",
            "role": "standalone",
            "status": "up",
            "last_seen": NOW_MS - 5000,
            "version": None
        }
    ],
    "meta": {
        "next_cursor": None,
        "total": 4
    }
}

# /api/v1/anomalies — lines 756-802 in pulse-api.yaml
# AnomalyList schema: lines 2764-2773
# AnomalyFlag schema: lines 2775-2800
ANOMALIES = {
    "items": [
        {
            "id": "anomaly-a1b2c3d4",
            "metric": "viewer_count",
            "scope": {
                "app": "live",
                "stream_id": "keynote-2026-main",
                "node_id": None,
                "tenant": None
            },
            "observed": 1250.0,
            "expected": 800.0,
            "sigma": 3.8,
            "ts": NOW_MS - 600000
        },
        {
            "id": "anomaly-e5f6g7h8",
            "metric": "ingest_bitrate_kbps",
            "scope": {
                "app": "events",
                "stream_id": "live-concert-stage-b",
                "node_id": "edge-ap-south-1",
                "tenant": None
            },
            "observed": 1800.0,
            "expected": 3500.0,
            "sigma": -4.2,
            "ts": NOW_MS - 180000
        },
        {
            "id": "anomaly-i9j0k1l2",
            "metric": "cpu_pct",
            "scope": {
                "app": None,
                "stream_id": None,
                "node_id": "edge-ap-south-1",
                "tenant": None
            },
            "observed": 92.5,
            "expected": 55.0,
            "sigma": 5.1,
            "ts": NOW_MS - 300000
        }
    ],
    "meta": {
        "next_cursor": None,
        "total": 3
    }
}

# /api/v1/qoe/summary — lines 289-325 in pulse-api.yaml
# QoeSummaryResponse schema: lines 2147-2156
# QoeTotals schema: lines 2158-2175
# BitrateBucket schema: lines 2177-2189
QOE_SUMMARY = {
    "totals": {
        "startup_p50_ms": 720.0,
        "startup_p95_ms": 1850.0,
        "rebuffer_ratio": 0.012,
        "error_rate": 0.003
    },
    "bitrate_timeline": [
        {
            "ts": NOW_MS - 7200000,
            "bitrate_kbps_p50": 2800.0,
            "bitrate_kbps_p95": 4200.0
        },
        {
            "ts": NOW_MS - 3600000,
            "bitrate_kbps_p50": 3100.0,
            "bitrate_kbps_p95": 4500.0
        },
        {
            "ts": NOW_MS,
            "bitrate_kbps_p50": 3400.0,
            "bitrate_kbps_p95": 4800.0
        }
    ]
}

# /api/v1/admin/license — lines 1365-1404 in pulse-api.yaml
# LicenseInfo schema: lines 3069-3117
LICENSE = {
    "tier": "enterprise",
    "valid": True,
    "limits": {
        "max_streams": 10000,
        "max_nodes": 50,
        "retention_days": 395,
        "data_api": True,
        "white_label": True
    },
    "expires_at": NOW_MS + 86400000 * 365,
    "offline_file": False
}

# Error response for 401 — Error schema: lines 1856-1869
ERROR_401 = {
    "code": "UNAUTHORIZED",
    "message": "Missing or invalid authentication token"
}

# -- HTTP Handler ----------------------------------------------------------------

class FixtureHandler(BaseHTTPRequestHandler):
    """HTTP request handler serving fixture JSON."""

    def log_message(self, fmt, *args):
        """Override to print to stderr with a prefix."""
        print(f"[fixture-server] {args[0]}", file=sys.stderr)

    def send_json(self, data, status=200):
        """Send a JSON response."""
        body = json.dumps(data, separators=(",", ":")).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def check_auth(self):
        """Check for a valid Authorization header. Returns True if valid."""
        auth = self.headers.get("Authorization", "")
        if not auth.startswith("Bearer ") or len(auth) <= 7:
            self.send_json(ERROR_401, 401)
            return False
        return True

    def do_GET(self):
        """Handle GET requests."""
        parsed = urlparse(self.path)
        path = parsed.path

        # Unauthenticated endpoints.
        if path == "/healthz":
            self.send_json(HEALTHZ)
            return

        # All other endpoints require authentication.
        if not self.check_auth():
            return

        # Route to fixture data.
        if path == "/auth/me":
            self.send_json(AUTH_ME)
        elif path == "/api/v1/live/overview":
            self.send_json(LIVE_OVERVIEW)
        elif path == "/api/v1/live/streams":
            self.send_json(LIVE_STREAMS)
        elif path == "/api/v1/alerts/history":
            self.send_json(ALERTS_HISTORY)
        elif path == "/api/v1/fleet/nodes":
            self.send_json(FLEET_NODES)
        elif path == "/api/v1/anomalies":
            self.send_json(ANOMALIES)
        elif path == "/api/v1/qoe/summary":
            self.send_json(QOE_SUMMARY)
        elif path == "/api/v1/admin/license":
            self.send_json(LICENSE)
        else:
            self.send_json({"code": "NOT_FOUND", "message": f"Unknown path: {path}"}, 404)


def main():
    parser = argparse.ArgumentParser(description="Pulse fixture server for UI tests")
    parser.add_argument("--port", type=int, default=8090, help="Port to listen on")
    args = parser.parse_args()

    # Bind only to localhost — this is a test fixture, not a production server.
    server = FastBindHTTPServer(("127.0.0.1", args.port), FixtureHandler)

    # Signal readiness by touching a file if FIXTURE_SERVER_READY_FILE is set.
    # This is more reliable than sleeping in CI.
    ready_file = os.environ.get("FIXTURE_SERVER_READY_FILE")
    if ready_file:
        Path(ready_file).touch()
        print(f"[fixture-server] Wrote ready file: {ready_file}", file=sys.stderr)

    print(f"[fixture-server] Listening on http://127.0.0.1:{args.port}", file=sys.stderr)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\n[fixture-server] Shutting down", file=sys.stderr)
        server.shutdown()


if __name__ == "__main__":
    main()
