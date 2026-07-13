# Pulse — Kafka Integration Guide

> **Audience:** operators who want Fleet resource gauges (CPU, memory, disk) on
> standalone AMS deployments, or who want higher-frequency per-stream ingest
> metrics not available from AMS's REST API.
>
> **Accuracy note:** every file reference, endpoint path, field name, and code
> fact below was read directly from the source files cited at the exact line
> numbers shown. Nothing is inferred from planning documents or memory.

---

> **⚠️ AV-15 BLOCKED — not live-validated against a real AMS Kafka producer.**
> The Kafka consumer is code-complete and covered by 8 contract tests that run
> against an in-process fake broker (see `server/internal/collector/kafka/kafka_test.go`
> and `docs/adr/0006-kafka-client-kafka-go.md`). It has **never** been connected
> to a real AMS Kafka producer. No Kafka broker has been deployed in any Pulse
> validation environment as of S27/D-089. Field names, topic names, and message
> shapes documented here are derived from Pulse source code and comments; they
> are unconfirmed as actual AMS wire values until you validate them against your
> AMS version. Treat this guide as a configuration reference, not a
> live-validated deployment recipe.

---

## 1. Why Kafka?

### 1.1 The REST limitation for standalone AMS

AMS 3.x exposes system status via `GET /rest/v2/system-status`. On a standalone
(non-cluster) node, that response contains only operating-system metadata:
`osName`, `osArch`, `javaVersion`, `processorCount`. No CPU utilisation, memory
usage, or disk usage fields are present in this response
(`docs/assessment/prd-validation-matrix.md`, AV-06;
`docs/assessment/capability-map.md` §5).

Consequences for Pulse without Kafka:

- The Fleet page shows OS/JVM metadata only; the CPU%, Memory%, and Disk%
  gauges remain empty for all standalone nodes (DG-05,
  `docs/known-limitations.md` LIM-01).
- Alert rules that condition on `cpu_pct`, `mem_pct`, or `disk_pct` cannot
  fire for standalone AMS because those fields never arrive
  (`docs/assessment/prd-validation-matrix.md` line 189).
- Real-time ingest FPS (`fps`), keyframe interval, jitter, and packet-loss
  fields are also absent from standalone REST; they appear only in the Kafka
  ingest-stats message stream.

### 1.2 Cluster mode: an alternative path

AMS cluster-node REST responses (`GET /rest/v2/cluster/nodes`) include
`cpuUsage` and `memoryUsage` per node. The Pulse cluster-discovery source
consumes those automatically; no Kafka configuration is needed for cluster
deployments.

**Standalone deployments have no cluster REST endpoint.** Kafka is the only
supported path to resource metrics for standalone AMS.

### 1.3 What Kafka adds beyond REST

| Metric | REST (standalone) | Kafka |
|---|---|---|
| CPU utilisation (`cpu_pct`) | Absent | Present |
| Memory utilisation (`mem_pct`) | Absent | Present |
| Disk utilisation (`disk_pct`) | Absent | Present |
| Ingest FPS (`fps`) | Absent (AMS 3.0.3) | Present (field name unconfirmed — see §4.4) |
| Keyframe interval | Absent | Present |
| Per-stream jitter | Absent | Present |
| Per-stream packet loss | Absent | Present |
| Per-protocol viewer counts | Present | Present (higher-frequency path) |

---

## 2. Prerequisites

### 2.1 Infrastructure decision

AMS's Kafka producer is optional and off by default. Enabling it requires:

1. A running Kafka broker (or cluster) reachable from both AMS and Pulse on the
   configured port (typically 9092).
2. AMS configured to publish events to that broker.
3. Pulse configured to subscribe to that broker.

Running and operating a Kafka broker is an operator infrastructure decision
(`docs/operator-expected.md`). Pulse does not include or manage a Kafka broker.

If you run AMS in cluster mode and do not need the additional Kafka metrics, set
`PULSE_KAFKA_BROKERS` to empty (the default) and Pulse reads CPU/mem from the
cluster REST endpoints instead.

### 2.2 AMS-side Kafka producer configuration

AMS has a built-in Kafka producer controlled by the `server.kafka_brokers`
property in AMS's `application.properties` file. Set it to your broker
address(es):

```
# AMS application.properties
server.kafka_brokers=kafka1:9092,kafka2:9092
```

This property name is referenced in `docs/prd-report.md` line 338 and
`agents/handoffs/wave-2/WO-202.md` line 26. **For exact syntax, supported
versions, and publish-interval settings, consult the official AMS documentation
at docs.antmedia.io** — no AMS `application.properties` file is included in
this repository, and AMS-side configuration is outside Pulse's scope.

According to `docs/adr/0006-kafka-client-kafka-go.md`, AMS publishes
approximately one message per 5 seconds per active stream when the producer is
enabled.

### 2.3 Network connectivity

The Pulse container must reach every configured Kafka broker on the configured
port. Check firewall rules, security groups, and VPC routing before starting.

> **⚠️ Plaintext only:** the current Pulse Kafka consumer does not support TLS
> or SASL authentication. The `kafkago.ReaderConfig` in
> `server/internal/collector/kafka/kafka.go` lines 130–138 has no `Dialer`,
> `TLS`, or `SASL` fields configured. All Kafka traffic is unencrypted. For
> production deployments on networks you do not fully control, restrict access
> to the Kafka broker port via network-layer controls (VPC ACLs, host firewall).

---

## 3. Pulse Configuration

### 3.1 Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `PULSE_KAFKA_BROKERS` | Yes (to enable) | *(empty — disabled)* | Comma-separated broker addresses, e.g. `kafka1:9092,kafka2:9092`. Whitespace around commas is stripped. Empty string means the Kafka source is not started. |
| `PULSE_KAFKA_GROUP_ID` | No | `pulse-collector` | Kafka consumer group ID. Change this if you run multiple Pulse instances against the same broker and want independent consumption. |
| `PULSE_AMS_NODE_ID` | Recommended | `standalone` | Identifier stamped on every event emitted from Kafka messages. Set to a unique value per AMS node in multi-node environments (see §3.4). |

Sources: `server/cmd/pulse/config.go` lines 212, 270–278.

> **No `PULSE_KAFKA_TOPIC` env var exists.** The topic Pulse subscribes to is
> fixed in source code and cannot be changed via environment variable in this
> release (see §4.1 for the topic name and the important operator caveat).

### 3.2 Docker Compose snippet

```yaml
services:
  pulse:
    image: pulse:latest
    environment:
      PULSE_KAFKA_BROKERS: "kafka1:9092,kafka2:9092"
      PULSE_KAFKA_GROUP_ID: "pulse-collector"
      PULSE_AMS_NODE_ID: "ams-node-01"
      # … other required vars …
```

An example with the Kafka line commented out is at
`deploy/config/pulse.example.yaml` line 16.

### 3.3 When Kafka is disabled

If `PULSE_KAFKA_BROKERS` is empty (the default), the Kafka source is not
constructed and not started (`server/cmd/pulse/serve.go` lines 279–290). Pulse
operates on REST polling only. No warning or error is logged for the absent
Kafka source; the Fleet resource gauges will simply remain empty for standalone
nodes.

### 3.4 Node identity mapping

`serve.go` line 283 sets `Config.NodeID = cfg.AMSNodeID` from
`PULSE_AMS_NODE_ID` (default `"standalone"`).

In `normalizeKafkaMessage` (`kafka.go` lines 228–233):

- If `PULSE_AMS_NODE_ID` is **non-empty** (including the `"standalone"`
  default), that value is stamped on every event emitted from Kafka messages.
  The `nodeId` field inside the Kafka message is ignored.
- If `PULSE_AMS_NODE_ID` is explicitly set to an empty string, the `nodeId`
  field from the Kafka message is used instead.

For multi-node deployments, set a unique `PULSE_AMS_NODE_ID` per Pulse
instance so events from different nodes can be distinguished in the Fleet view.

---

## 4. AMS Topic and Message Schema

> **⚠️ Unconfirmed against a live AMS broker (AV-15 BLOCKED).** The topic name
> and field names below are derived from Pulse source code and comments. They
> have not been validated against a real AMS Kafka producer. Confirm the actual
> topic name and field names against your AMS version before relying on this
> guide in production.

### 4.1 Topic name — operator caveat

Pulse subscribes to the topic **`ams-server-events`**.

This name appears as the hard-coded default in:

- `server/internal/collector/kafka/kafka.go` line 35 (comment), line 58
  (`DefaultConfig`), line 100 (`New()` fallback)
- `server/internal/collector/kafka/kafka_test.go` lines 135, 317, 336

`serve.go` lines 280–286 do not set `cfg.Topics`, so the default
`"ams-server-events"` is always active. The topic is **not configurable via
environment variable** in this release.

> **Important discrepancy:** planning and assessment documents in this
> repository (`docs/assessment/capability-map.md` line 210,
> `docs/known-limitations.md` lines 25 and 92,
> `docs/assessment/prd-validation-matrix.md` line 96,
> `docs/assessment/final-assessment.md` lines 262, 271, 335) consistently use
> the name **`ams-instance-stats`**. Those documents were written before the
> Kafka consumer code was committed and the topic name was never corrected in
> them. **The code is authoritative: Pulse subscribes to `ams-server-events`.**
>
> Because AV-15 is blocked, neither name has been confirmed against what a real
> AMS broker actually publishes. **Before deploying, verify the topic name your
> AMS version publishes to.** A mismatch means Pulse receives zero messages and
> consumer lag will not move — a silent failure with no parse errors.

### 4.2 Message format

All messages are UTF-8-encoded JSON objects. AMS publishes stats as JSON blobs;
there is no Avro or schema-registry dependency (per `docs/adr/0006-kafka-client-kafka-go.md`).

#### Common fields (present in all message types)

| AMS field | Pulse field | Notes |
|---|---|---|
| `streamId` | `ServerEvent.StreamID` | String; may be empty for node-level stats |
| `app` | `ServerEvent.App` | String; defaults to `"live"` if absent (`kafka.go` line 224) |
| `nodeId` | `ServerEvent.NodeID` | Used only when `PULSE_AMS_NODE_ID` is empty (`kafka.go` lines 228–233) |
| `timestamp` | `ServerEvent.TS` | Float64, epoch milliseconds; falls back to server time if absent or zero (`kafka.go` lines 235–238) |

`ServerEvent.Source` is always `domain.SourceKafka` for Kafka-sourced events.

### 4.3 Message routing (field-presence based)

Pulse routes each message to one of three event types by inspecting which fields
are present. Routing is implemented in `normalizeKafkaMessage` at
`server/internal/collector/kafka/kafka.go` lines 244–283.

The three cases are evaluated in priority order:

1. If the message contains `cpuUsage` → **node-stats** (`node_stats`)
2. Else if the message contains both `fps` **and** `bitrate` → **ingest-stats** (`ingest_stats`)
3. Otherwise → **stream-stats** (`stream_stats`)

### 4.4 Node-stats message — Fleet CPU/memory/disk gauges

**Routing trigger:** message contains the `cpuUsage` key (`kafka.go` line 245).

This is the message type that populates the Fleet resource gauges. Without
messages of this type, Fleet CPU%, Memory%, and Disk% remain empty for
standalone nodes.

| AMS field | Pulse output field | Type | Notes |
|---|---|---|---|
| `cpuUsage` | `data["cpu_pct"]` | float64, % | |
| `memoryUsage` | `data["mem_pct"]` | float64, % | |
| `diskUsage` | `data["disk_pct"]` | float64, % | |

Source: `kafka.go` lines 247–252.

### 4.5 Ingest-stats message — per-stream encoder metrics

**Routing trigger:** message contains both `fps` **and** `bitrate`
(`kafka.go` line 254).

| AMS field | Pulse output field | Type | Notes |
|---|---|---|---|
| `bitrate` | `data["bitrate_kbps"]` | float64 | |
| `fps` | `data["fps"]` | float64 | Field name unconfirmed — see note below |
| `keyFrameInterval` | `data["keyframe_interval_s"]` | float64 | |
| `packetLost` | `data["packet_loss_pct"]` | float64 | |
| `jitter` | `data["jitter_ms"]` | float64 | |

Source: `kafka.go` lines 257–263.

> **Note:** `docs/assessment/final-assessment.md` lines 409–413 flags the AMS
> FPS field name as an open question. The consumer maps the key `"fps"`; if AMS
> publishes under a different key name, the `fps` output field will be 0 and
> this message type will not be routed as ingest-stats (because the trigger
> requires both `fps` and `bitrate` to be present). Confirm the field name
> against your AMS version.

### 4.6 Stream-stats message — viewer counts (default/fallback)

**Routing trigger:** all messages not matched by §4.4 or §4.5 (`kafka.go`
line 265 `default` case).

| AMS field | Pulse output field | Type | Notes |
|---|---|---|---|
| `hlsViewerCount` | `data["viewer_count_by_protocol"]["hls"]` | int | |
| `webRTCViewerCount` | `data["viewer_count_by_protocol"]["webrtc"]` | int | |
| `rtmpViewerCount` | `data["viewer_count_by_protocol"]["rtmp"]` | int | |
| `dashViewerCount` | `data["viewer_count_by_protocol"]["dash"]` | int | |
| *(sum of all four)* | `data["viewer_count"]` | int | |
| `bitrate` | `data["bitrate_kbps"]` | float64 | |

Source: `kafka.go` lines 269–283.

---

## 5. Error Handling and Reliability

### 5.1 Malformed messages

Invalid JSON is skipped: the `parseErrors` counter is incremented, a `DEBUG`-level
log line is emitted (`kafka.go` lines 183–190), and processing continues. Pulse
never crashes on a malformed Kafka message.

Unknown fields inside an otherwise valid JSON object are silently ignored.
`floatField` (`kafka.go` lines 303–313) returns `0` for any key that is absent
or of an unexpected type; there is no error and no log output for missing fields.

### 5.2 Broker failure and reconnect

If the broker is unreachable or returns an error, `Run()` returns an error. The
parent `collector.Collector` supervisor (`server/internal/collector/collector.go`
lines 62–109) restarts the source with exponential backoff:

- Initial delay: 100 ms
- Cap: 60 s
- Each failed restart doubles the delay up to the cap

A clean shutdown via SIGTERM cancels the context, causing `Run()` to exit
cleanly with no restart.

### 5.3 Delivery guarantee

At-least-once delivery: Pulse commits the Kafka offset after
`processMessage` returns (`kafka.go` lines 163–169). A process crash between
message processing and offset commit will cause that message to be redelivered
on the next restart. Duplicate delivery of a message may result in a duplicate
event in ClickHouse.

### 5.4 Offset start position

On the FIRST start with a consumer group that has no committed offset, the
consumer begins at the EARLIEST retained message (`FirstOffset` — Pulse does
not set `StartOffset`, and kafka-go defaults the zero value to `FirstOffset`,
`consumergroup.go:243`). This means a fresh Pulse install pointed at a topic
with retained history will replay and ingest that history once. Subsequent
restarts resume from the group's committed offsets (no re-replay). There is
no configurable start-position override in this release.

---

## 6. Verification

### 6.1 Startup log

When `PULSE_KAFKA_BROKERS` is set and the Kafka source is constructed, Pulse
emits:

```
INFO pulse: kafka source configured brokers=[kafka1:9092,kafka2:9092]
```

Source: `server/cmd/pulse/serve.go` line 289.

If this line does not appear, `PULSE_KAFKA_BROKERS` was empty or not set in the
process environment.

### 6.2 /healthz Kafka component

When the Kafka source is active, `GET /healthz` includes a `"kafka"` component:

```json
{
  "kafka": {
    "status": "ok",
    "lag": 0,
    "parse_errors": 0
  }
}
```

| Field | Meaning |
|---|---|
| `status` | `"ok"`, or `"degraded"` when `parse_errors > 0` **or** `lag > 10000` (`server.go:803`) |
| `lag` | Consumer lag (messages behind) observed at the last fetch |
| `parse_errors` | Count of malformed messages since process start |

Source: `server/internal/api/server.go` lines 104–112, 797–820.

This component is CI-validated by `TestAPI_Healthz_KafkaStats` against an
in-process fake broker (not a real AMS broker — AV-15 blocked;
`docs/assessment/prd-validation-matrix.md` line 350, N25).

### 6.3 Confirming CPU/memory data flows

After connecting:

1. Navigate to the Pulse Fleet page.
2. CPU%, Memory%, and Disk% gauges should populate within approximately one AMS
   publish interval (~5 s per stream, per `docs/adr/0006-kafka-client-kafka-go.md`).

If the gauges remain empty after 30 seconds:

| Check | How |
|---|---|
| Are messages arriving? | `GET /healthz` — if `lag` is not moving, AMS is not publishing to the topic, or the topic name does not match (see §4.1). |
| Are messages being parsed? | `GET /healthz` — if `parse_errors > 0`, inspect Pulse logs for `kafka: malformed JSON, skipping` at DEBUG level. |
| Does the message contain `cpuUsage`? | Enable `PULSE_LOG_LEVEL=debug` and inspect log output. A node-stats message must contain the `cpuUsage` key to populate Fleet gauges. |
| Is the topic name correct? | Confirm the topic your AMS version publishes to. Pulse subscribes to `ams-server-events`; a mismatch produces zero messages and zero `lag` movement. |

---

## 7. Limitations

| Limitation | Detail |
|---|---|
| **AV-15 BLOCKED — no live validation** | The Kafka consumer has never been connected to a real AMS Kafka producer. Field names and topic names are code-derived and unconfirmed. |
| **Plaintext only** | No TLS or SASL authentication. Restrict broker port access via network controls on untrusted networks. |
| **Topic not configurable** | `PULSE_KAFKA_TOPIC` does not exist. The topic is fixed at `"ams-server-events"` in source code for this release. |
| **First-start history replay** | With a fresh (uncommitted) consumer group the consumer starts at the earliest retained message and ingests topic history once; later restarts resume from committed offsets. |
| **At-least-once delivery** | A crash between process and commit causes redelivery and may produce a duplicate event in ClickHouse. |
| **FPS field name unconfirmed** | `docs/assessment/final-assessment.md` lines 409–413 flags this as an open question. If AMS uses a different key name, `fps` output will be 0. |
| **No CPU/mem alerts without Kafka** | Alert rules on `cpu_pct`, `mem_pct`, `disk_pct` cannot fire for standalone AMS without an active Kafka connection and matching AMS node-stats messages. |

---

## 8. Relationship to Other Docs

| Document | Relationship |
|---|---|
| `docs/AMS-INTEGRATION.md` §1.3 | Two-line stub describing Kafka activation; this document is the complete reference for Kafka operators. |
| `docs/known-limitations.md` LIM-01 | References `PULSE_KAFKA_BROKERS` as the resolution path for absent Fleet resource gauges. |
| `docs/adr/0006-kafka-client-kafka-go.md` | ADR for the `github.com/segmentio/kafka-go` library choice, message format expectations, and estimated publish interval. |
| `docs/assessment/final-assessment.md` §5 P1 | Roadmap item: "Standalone CPU/mem/disk via Kafka" — blocked pending AV-15 live validation. |
| `docs/assessment/prd-validation-matrix.md` AV-15 | Validation status: BLOCKED. |
