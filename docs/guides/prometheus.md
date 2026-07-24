# Pulse Prometheus / Grafana Integration Guide

**PRD ref:** F8 (Data API + Prometheus) · **Status: Shipped (Wave 2)**

---

## Overview

Pulse exposes a Prometheus-compatible `/metrics` endpoint. This lets you scrape
Pulse metrics into any Prometheus-compatible backend and build Grafana panels,
PagerDuty/Alertmanager rules, or custom dashboards on top.

The endpoint is bounded cardinality by design: the five aggregate metrics carry
no labels; `node=` labels appear only on the per-node CPU and memory metrics
(no `stream_id` or `session_id` anywhere). This keeps the metric count
predictable even on installations with thousands of concurrent streams.

---

## Enabling the scrape endpoint

The `/metrics` endpoint is available without a scrape token on Business or
Enterprise tier (see [Known limitations](#known-limitations)). When
`PULSE_METRICS_TOKEN` is set, every scrape request must supply it as a Bearer
token:

```sh
export PULSE_METRICS_TOKEN=my-secret-scrape-token
```

Restart Pulse. The token can be any string; it is compared with a constant-time
string comparison. Store it in a Kubernetes Secret (see Helm guide below).

If `PULSE_METRICS_TOKEN` is unset, the endpoint serves metrics without any
token check — appropriate for a private network where the port is not exposed
publicly.

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

All metrics use the `pulse_` prefix. The five aggregate metrics carry no labels.
The two per-node metrics carry a `node` label with one series per AMS node;
when no nodes have reported yet the `HELP` and `TYPE` lines are emitted but no
sample lines appear.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `pulse_live_viewers` | gauge | — | Total current viewer count across all active streams |
| `pulse_live_streams` | gauge | — | Number of currently active streams (publisher connected) |
| `pulse_live_publishers` | gauge | — | Number of active publisher connections (ingest) |
| `pulse_ingest_bitrate_kbps` | gauge | — | Aggregate ingest bitrate in Kbps across all active streams |
| `pulse_node_cpu_pct` | gauge | `node` | CPU utilization percent for each AMS node |
| `pulse_node_mem_pct` | gauge | `node` | Memory utilization percent for each AMS node |
| `pulse_alerts_firing` | gauge | — | Count of `alert_history` rows in the `firing` state — see the caveat below |
| `pulse_collector_last_success_timestamp` | gauge | — | Unix time of Pulse's most recent successful AMS poll; `0` if none since boot |
| `pulse_collector_up` | gauge | — | `1` when the last successful poll is within the staleness window, else `0` |

> **`pulse_alerts_firing` does not mean "rules firing right now."** It counts rows
> in the alert history whose state is `firing`, over all time and capped at the
> 1000 most recent. One rule that has fired on ten occasions contributes ten, and
> a rule that has since resolved still contributes its historical firings. Treat it
> as a cumulative event count, not a live gauge of current incidents — alerting on
> `pulse_alerts_firing > 0` will page you for something that resolved last month.

> **★ Alert on Pulse's own blindness — `pulse_collector_up` / `pulse_collector_last_success_timestamp`.**
> Pulse's own alert rules evaluate metrics *derived from* the collector, so if the
> collector stops reaching your AMS there is nothing left to evaluate and no rule
> fires — a monitor that cannot notice its own blindness (this is exactly how a
> 7 h 46 m collection outage once went unpaged). These two gauges let **your**
> Prometheus catch it independently. The `_last_success_timestamp` gauge keeps
> reporting the last (old) success while blind, so `time() - …` is the outage age;
> `pulse_collector_up` is the ready-made boolean, matching `/healthz`'s collector
> component (fresh unless the last poll is older than the staleness window; both are
> absent on a pure-beacon deployment with no AMS collector). A minimal rule:
>
> ```yaml
> - alert: PulseCollectorBlind
>   expr: pulse_collector_up == 0
>   for: 2m
>   annotations:
>     summary: "Pulse has not polled AMS successfully in over its staleness window"
> ```

**Sample output** (Business tier, idle instance with no AMS nodes connected):

```text
# HELP pulse_live_viewers Current live viewer count
# TYPE pulse_live_viewers gauge
pulse_live_viewers 0
# HELP pulse_live_streams Current active stream count
# TYPE pulse_live_streams gauge
pulse_live_streams 0
# HELP pulse_live_publishers Current publishing stream count
# TYPE pulse_live_publishers gauge
pulse_live_publishers 0
# HELP pulse_ingest_bitrate_kbps Aggregate ingest bitrate kbps
# TYPE pulse_ingest_bitrate_kbps gauge
pulse_ingest_bitrate_kbps 0
# HELP pulse_node_cpu_pct Node CPU utilization percent
# TYPE pulse_node_cpu_pct gauge
# HELP pulse_node_mem_pct Node memory utilization percent
# TYPE pulse_node_mem_pct gauge
# HELP pulse_alerts_firing Total firing alert count
# TYPE pulse_alerts_firing gauge
pulse_alerts_firing 0
# HELP pulse_collector_last_success_timestamp Unix time of the most recent successful AMS poll (0 = none since boot)
# TYPE pulse_collector_last_success_timestamp gauge
pulse_collector_last_success_timestamp 1784908800
# HELP pulse_collector_up 1 when the collector's last successful poll is within its staleness window, else 0
# TYPE pulse_collector_up gauge
pulse_collector_up 1
```

With one AMS node connected the two per-node metrics gain sample lines:

```text
pulse_node_cpu_pct{node="standalone"} 15
pulse_node_mem_pct{node="standalone"} 40
```

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
          "expr": "pulse_live_viewers",
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
          "expr": "pulse_live_streams",
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
          "expr": "pulse_live_publishers",
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
      "title": "Ingest Bitrate (Kbps)",
      "type": "timeseries",
      "gridPos": {"h": 8, "w": 12, "x": 0, "y": 4},
      "targets": [
        {
          "datasource": "$datasource",
          "expr": "pulse_ingest_bitrate_kbps",
          "legendFormat": "ingest bitrate"
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
          "expr": "pulse_live_viewers",
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

**Total live viewers:**
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
pulse_ingest_bitrate_kbps / 1000
```

**CPU by node:**
```promql
pulse_node_cpu_pct
```

**Seconds since Pulse last polled AMS (its own blindness):**
```promql
time() - pulse_collector_last_success_timestamp
```
(High and climbing = Pulse is not collecting. Use `pulse_collector_up == 0` for a ready-made boolean.)

**Alert for streams going down:**
```yaml
# prometheus alerting rule (optional — Pulse also has its own alert evaluator)
groups:
  - name: pulse
    rules:
      - alert: PulseNoStreams
        expr: pulse_live_streams == 0
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
- **Business tier gate.** The `/metrics` endpoint requires Business or Enterprise
  tier (`CheckPrometheus` license check). Free and Pro tiers receive
  `403 LICENSE_REQUIRED` with body
  `{"code":"LICENSE_REQUIRED","message":"Prometheus endpoint (F8) requires Business tier or higher (current: \"<tier>\")"}`.
  Set `PULSE_LICENSE_KEY` to upgrade, or use the Pulse UI directly (always available).

**Phase-3 roadmap:** Additional QoE metrics (startup p50/p95, rebuffer ratio,
error rate) and per-application aggregates are planned for Wave 3.
