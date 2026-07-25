# Pulse — Kafka Integration Guide

> **Audience:** operators who want Fleet resource gauges (CPU, memory, disk) on
> standalone AMS deployments, or who want higher-frequency per-stream ingest
> metrics not available from AMS's REST API.
>
> **Accuracy note:** every file reference, endpoint path, field name, and code
> fact below was read directly from the source files cited at the exact line
> numbers shown. Nothing is inferred from planning documents or memory.

---

> **⚠️ EXPERIMENTAL / PREVIEW — AV-15 pending live validation against a real AMS Kafka producer.**
> The Kafka consumer is now aligned to the **official AMS topic names**
> (`ams-instance-stats`, `ams-webrtc-stats`, verified from AMS `StatsCollector.java`)
> and official nested message shapes. It is covered by contract tests against an
> in-process fake broker **and** broker-integration tests. However, it has **never**
> been connected to a real AMS Kafka producer. Topic names, field names, and message
> shapes documented here are source-derived and unconfirmed as live AMS wire values
> until AV-15 is resolved. Treat this guide as a pre-validated configuration reference.

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
property in AMS's **`conf/red5.properties`** file. Set it to your broker
address(es):

```properties
# AMS conf/red5.properties — enable the Kafka producer:
server.kafka_brokers=kafka1:9092,kafka2:9092
```

**For exact syntax, supported versions, and publish-interval settings, consult
the official AMS documentation at docs.antmedia.io** — no AMS `red5.properties`
file is included in this repository, and AMS-side configuration is outside
Pulse's scope.

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
| `PULSE_KAFKA_TOPICS` | No | `ams-instance-stats,ams-webrtc-stats` | Comma-separated list of Kafka topics to subscribe to. Default subscribes to both official AMS stats topics. Override when your AMS deployment uses non-standard topic names (see §4.1). |
| `PULSE_AMS_NODE_ID` | Recommended | `standalone` | Identifier stamped on every event emitted from Kafka messages. Set to a unique value per AMS node in multi-node environments (see §3.4). |

Sources: `server/cmd/pulse/config.go` (Kafka fields in `EnvConfig`; env parsing
in `loadEnvConfig` — search for `PULSE_KAFKA_`).

### 3.2 Docker Compose snippet

```yaml
services:
  pulse:
    image: pulse:latest
    environment:
      PULSE_KAFKA_BROKERS: "kafka1:9092,kafka2:9092"
      PULSE_KAFKA_GROUP_ID: "pulse-collector"
      # PULSE_KAFKA_TOPICS: "ams-instance-stats,ams-webrtc-stats"  # optional override
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

### 4.1 Topics — official AMS topic names

AMS publishes Kafka messages to two distinct topics. Pulse subscribes to both by
default:

| Topic | Content | AMS source |
|---|---|---|
| `ams-instance-stats` | Node-level CPU, memory, disk utilisation per AMS instance | AMS `StatsCollector.java` |
| `ams-webrtc-stats` | Per-stream WebRTC and ingest metrics (FPS, bitrate, jitter, packet loss) | AMS `StatsCollector.java` |

Both topic names are verified from AMS source code (`StatsCollector.java`) and
are the defaults for `PULSE_KAFKA_TOPICS`. To subscribe to only one topic, or
to override the names for a non-standard AMS configuration, set:

```env
PULSE_KAFKA_TOPICS=ams-instance-stats,ams-webrtc-stats
```

> **AV-15 — not yet live-validated.** The topic names above are derived from AMS
> source (`StatsCollector.java`). They have not been confirmed against a live AMS
> Kafka broker. If AMS on your version publishes to different topics, `GET /healthz`
> will show `lag = 0` indefinitely — a silent failure with no parse errors (see §6.2).
>
> **Historical note:** earlier Pulse releases subscribed to `ams-server-events`, a
> name derived from Pulse's own source comments rather than AMS source. The consumer
> is now aligned to the official AMS topic names above.

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

### 4.3 Message routing (topic-based)

Pulse routes each Kafka message by the **topic** it was received on:

| Topic | Parsed as | Section |
|---|---|---|
| `ams-instance-stats` | Node-stats — Fleet CPU/memory/disk gauges | §4.4 |
| `ams-webrtc-stats` | **Subscribed but currently SKIPPED** (no domain mapping yet) | §4.5 |
| any other configured topic | Legacy flat-field parsing (custom bridge feeds) | §4.6 |

Routing is implemented in `normalizeKafkaMessage` at
`server/internal/collector/kafka/kafka.go`.

### 4.4 Node-stats message — Fleet CPU/memory/disk gauges

**Topic:** `ams-instance-stats`. This topic populates the Fleet resource gauges.
Without messages on this topic, Fleet CPU%, Memory%, and Disk% remain empty for
standalone nodes.

**Official AMS message shape** (source-derived from AMS `StatsCollector.java`,
pinned as the test fixture `server/internal/collector/kafka/testdata/ams-instance-stats.json`;
unconfirmed against a live broker until AV-15):

```json
{
  "instanceId": "<node-id-string>",
  "cpuUsage": {
    "processCPUTime": 123456789,
    "systemCPULoad": 42,
    "processCPULoad": 15,
    "systemLoadAverageLastMinute": 1.8
  },
  "systemMemoryInfo": {
    "totalMemory": 8589934592,
    "freeMemory": 1073741824,
    "inUseMemory": 7516192768
  },
  "fileSystemInfo": {
    "usableSpace": 107374182400,
    "totalSpace": 214748364800,
    "inUseSpace": 107374182400
  }
}
```

| AMS wire field | Pulse output field | Type | Notes |
|---|---|---|---|
| `instanceId` | node identity | string | Used when `PULSE_AMS_NODE_ID` is empty (see §3.4) |
| `cpuUsage.systemCPULoad` (nested) | `data["cpu_pct"]` | float64, % | **Already an integer percent on the wire** — AMS's `SystemUtils` converts `[0,1]→0-100` before publishing; Pulse does NOT rescale |
| `systemMemoryInfo.inUseMemory / .totalMemory` (nested) | `data["mem_pct"]` | float64, % | Computed ratio ×100, div-by-zero guarded |
| `fileSystemInfo.inUseSpace / .totalSpace` (nested) | `data["disk_pct"]` | float64, % | Computed ratio ×100, div-by-zero guarded |

Source: `server/internal/collector/kafka/kafka.go` (`normalizeKafkaMessage`,
`ams-instance-stats` case).

### 4.5 `ams-webrtc-stats` — subscribed but currently SKIPPED

Pulse subscribes to `ams-webrtc-stats` (it is in the default topic set) but
**does not currently emit any events from it**. The official message shape
(`StatsCollector.java` `sendWebRTCClientStats2Kafka`) is a **per-viewer WebRTC
client record** — `webrtcClientId`, `measured_bitrate`, `send_bitrate`,
`audioFrameSendPeriod`, `videoFrameSendPeriod`, `ipAddress`, `hostAddress`,
`time` — which does not map cleanly onto Pulse's per-stream ingest-stats or
viewer-count event types. Rather than fabricate metrics from a mismatched
shape, the consumer skips these messages (a once-per-session DEBUG log notes
the skip; the `parse_errors` counter is NOT incremented). A future release may
map this feed to a dedicated per-viewer event type once AV-15 live validation
confirms the wire shape. **No FPS/bitrate/viewer-count data flows from this
topic today** — those metrics come from the REST poller (and the beacon SDK on
the viewer side).

### 4.6 Custom bridge topics — legacy flat-field parsing

Messages on any **other** topic configured via `PULSE_KAFKA_TOPICS` (i.e. a
custom feed you bridge yourself, not an official AMS topic) keep the original
field-sniffing behavior, unchanged for back-compat:

| Trigger | Parsed as | Fields read (flat) |
|---|---|---|
| `cpuUsage` present (flat number) | node-stats | `cpuUsage`, `memoryUsage`, `diskUsage`, `nodeId` |
| `fps` + `bitrate` both present | ingest-stats | `bitrate`, `fps`, `keyFrameInterval`, `packetLost`, `jitter` |
| otherwise | stream-stats | `hlsViewerCount`, `webRTCViewerCount`, `rtmpViewerCount`, `dashViewerCount`, `bitrate` |

Source: `kafka.go` (`normalizeKafkaMessage`, `default` case).

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
| Are the topic names correct? | Confirm the topics your AMS version publishes to. Pulse subscribes to `ams-instance-stats,ams-webrtc-stats` by default (`PULSE_KAFKA_TOPICS` override available); a mismatch produces zero messages and zero `lag` movement. |

---

## 7. Limitations

| Limitation | Detail |
|---|---|
| **AV-15 PENDING — live validation not yet run** | The Kafka consumer is aligned to official AMS topics and shapes (source-derived, fixture-tested + broker-integration-tested) but has never been connected to a real AMS Kafka producer. Topic names and field names are unconfirmed as live wire values. |
| **Plaintext only** | No TLS or SASL authentication. Restrict broker port access via network controls on untrusted networks. |
| **Topic list configurable via `PULSE_KAFKA_TOPICS`** | Default `ams-instance-stats,ams-webrtc-stats`; comma-separated override available to target a subset or non-standard topic names. |
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
