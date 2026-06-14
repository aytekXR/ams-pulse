# Pulse Prometheus / Grafana Integration Guide

**PRD ref:** F8 (Data API + Prometheus) · **Status: Shipped (Wave 2)**

---

## Overview

Pulse exposes a Prometheus-compatible `/metrics` endpoint. This lets you scrape
Pulse metrics into any Prometheus-compatible backend and build Grafana panels,
PagerDuty/Alertmanager rules, or custom dashboards on top.

The endpoint is bounded cardinality by design: only `node=` labels are used
(no `stream_id` or `session_id`). This keeps the metric count predictable
even on installations with thousands of concurrent streams.

---

## Enabling the scrape endpoint

By default `/metrics` returns `401 Unauthorized`. Set `PULSE_METRICS_TOKEN`
to enable scraping:

```sh
export PULSE_METRICS_TOKEN=my-secret-scrape-token
```

Restart Pulse. The token can be any string; it is compared with a constant-time
string comparison. Store it in a Kubernetes Secret (see Helm guide below).

To disable the scrape token check (allow unauthenticated scraping on a closed
network), leave `PULSE_METRICS_TOKEN` unset and send no `Authorization` header.
In that case the endpoint returns 401 to prevent accidental exposure.

> **Security note:** The `/metrics` endpoint is not blocked by the admin Bearer token.
> It uses its own token to allow Prometheus to scrape without an admin credential.

---

## Prometheus scrape config

Add a scrape job to your `prometheus.yml`:

### Without authentication

If Pulse is on a private network with no scrape token:
```yaml
scrape_configs:
  - job_name: pulse
    static_configs:
      - targets: ['pulse.internal:8090']
    metrics_path: /metrics
    scrape_interval: 15s
```

### With scrape token (recommended)

```yaml
scrape_configs:
  - job_name: pulse
    static_configs:
      - targets: ['pulse.internal:8090']
    metrics_path: /metrics
    scrape_interval: 15s
    authorization:
      credentials: my-secret-scrape-token
```

### Kubernetes / Prometheus Operator

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: pulse
  namespace: monitoring
spec:
  selector:
    matchLabels:
      app: pulse
  endpoints:
    - port: http
      path: /metrics
      interval: 15s
      authorization:
        credentials:
          name: pulse-scrape-secret
          key: token
```

---

## Metric reference

All metrics use the `pulse_` prefix and carry a `node` label where applicable.
High-cardinality labels (`stream_id`, `session_id`) are deliberately absent.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `pulse_live_viewers` | gauge | `node` | Total current viewer count across all active streams on this node |
| `pulse_live_streams` | gauge | `node` | Number of currently active streams (publisher connected) |
| `pulse_live_publishers` | gauge | `node` | Number of active publisher connections (ingest) |
| `pulse_ingest_bitrate_kbps` | gauge | `node` | Aggregate ingest bitrate in Kbps across all active streams |
| `pulse_alerts_firing` | gauge | — | Number of alert rules currently in the `firing` state |

**C-W2-09 verified (QA gate 2026-06-14):**
- All 5 metrics present in exposition
- 22 total metric lines (bounded)
- Zero `stream_id` or `session_id` labels

---

## Grafana starter panels

The following panel JSON uses Grafana's dashboard JSON model format.
Import it via Dashboard → Import → Paste JSON.

> **Note:** Replace `$datasource` with your Prometheus datasource name.

```json
{
  "title": "Pulse — AMS Real-Time Overview",
  "uid": "pulse-ams-overview",
  "panels": [
    {
      "id": 1,
      "title": "Live Viewers",
      "type": "stat",
      "gridPos": {"h": 4, "w": 6, "x": 0, "y": 0},
      "targets": [
        {
          "datasource": "$datasource",
          "expr": "sum(pulse_live_viewers)",
          "legendFormat": "viewers"
        }
      ],
      "options": {"colorMode": "background", "graphMode": "area"},
      "fieldConfig": {"defaults": {"color": {"mode": "thresholds"},
        "thresholds": {"steps": [
          {"color": "green", "value": null},
          {"color": "yellow", "value": 1000},
          {"color": "red", "value": 5000}
        ]}}}
    },
    {
      "id": 2,
      "title": "Active Streams",
      "type": "stat",
      "gridPos": {"h": 4, "w": 6, "x": 6, "y": 0},
      "targets": [
        {
          "datasource": "$datasource",
          "expr": "sum(pulse_live_streams)",
          "legendFormat": "streams"
        }
      ],
      "options": {"colorMode": "background", "graphMode": "area"}
    },
    {
      "id": 3,
      "title": "Active Publishers (ingest)",
      "type": "stat",
      "gridPos": {"h": 4, "w": 6, "x": 12, "y": 0},
      "targets": [
        {
          "datasource": "$datasource",
          "expr": "sum(pulse_live_publishers)",
          "legendFormat": "publishers"
        }
      ],
      "options": {"colorMode": "background", "graphMode": "area"}
    },
    {
      "id": 4,
      "title": "Alerts Firing",
      "type": "stat",
      "gridPos": {"h": 4, "w": 6, "x": 18, "y": 0},
      "targets": [
        {
          "datasource": "$datasource",
          "expr": "pulse_alerts_firing",
          "legendFormat": "firing"
        }
      ],
      "options": {"colorMode": "background", "graphMode": "none"},
      "fieldConfig": {"defaults": {"color": {"mode": "thresholds"},
        "thresholds": {"steps": [
          {"color": "green", "value": null},
          {"color": "red", "value": 1}
        ]}}}
    },
    {
      "id": 5,
      "title": "Ingest Bitrate (Kbps) by Node",
      "type": "timeseries",
      "gridPos": {"h": 8, "w": 12, "x": 0, "y": 4},
      "targets": [
        {
          "datasource": "$datasource",
          "expr": "pulse_ingest_bitrate_kbps",
          "legendFormat": "{{node}}"
        }
      ],
      "fieldConfig": {"defaults": {"unit": "kbps", "min": 0}}
    },
    {
      "id": 6,
      "title": "Live Viewers over Time",
      "type": "timeseries",
      "gridPos": {"h": 8, "w": 12, "x": 12, "y": 4},
      "targets": [
        {
          "datasource": "$datasource",
          "expr": "sum(pulse_live_viewers)",
          "legendFormat": "viewers"
        }
      ],
      "fieldConfig": {"defaults": {"unit": "short", "min": 0}}
    }
  ],
  "time": {"from": "now-1h", "to": "now"},
  "refresh": "30s",
  "schemaVersion": 36
}
```

### Useful PromQL expressions

**Viewers per node:**
```promql
pulse_live_viewers
```

**Total viewer-hour estimate (rough):**
```promql
sum_over_time(pulse_live_viewers[1h]) * 15 / 3600
```
(Multiply by scrape interval in seconds, divide by 3600 to get viewer-hours.)

**Ingest total Mbps:**
```promql
sum(pulse_ingest_bitrate_kbps) / 1000
```

**Alert for streams going down:**
```yaml
# prometheus alerting rule (optional — Pulse also has its own alert evaluator)
groups:
  - name: pulse
    rules:
      - alert: PulseNoStreams
        expr: sum(pulse_live_streams) == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Pulse reports zero active streams for 5 minutes"
```

---

## Helm installation with scrape token

```yaml
# my-values.yaml
pulse:
  secretRef:
    name: pulse-secrets  # must include PULSE_METRICS_TOKEN

# Create the secret before installing:
# kubectl create secret generic pulse-secrets \
#   --from-literal=PULSE_AMS_AUTH_TOKEN=<ams-token> \
#   --from-literal=PULSE_SECRET_KEY=<32-byte-hex> \
#   --from-literal=PULSE_METRICS_TOKEN=<scrape-token>
```

The Helm chart mounts `pulse-secrets` as environment variables via `envFrom.secretRef`.
`PULSE_METRICS_TOKEN` is never written to `values.yaml` — it is always secret-sourced.

---

## Known limitations

- **Bounded metrics only.** Per-stream or per-viewer cardinality metrics are not
  exposed via `/metrics`. Use the REST API (`/api/v1/live/streams`,
  `/api/v1/analytics/*`) or the Pulse UI for stream-level detail.
- **Data API tier gate.** The `/metrics` endpoint is behind the Pro tier
  (`CheckDataAPI` license check). Free tier returns `403 LICENSE_REQUIRED`.
  Set `PULSE_LICENSE_KEY` to upgrade, or use the Pulse UI directly (always available).

**Phase-3 roadmap:** Additional QoE metrics (startup p50/p95, rebuffer ratio,
error rate) and per-application aggregates are planned for Wave 3.
