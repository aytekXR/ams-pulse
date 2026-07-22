# Pulse — Known Limitations

**Product:** Pulse v0.4.0 (last refreshed D-161, S97, 2026-07-22)  
**Source:** `docs/assessment/documentation-gaps.md` (DG-01 through DG-18),
`docs/assessment/final-assessment.md` §1 and Appendix B,
`docs/assessment/capability-map.md`

This document lists every known operator-facing limitation of Pulse v0.4.0 in
priority order. Each entry states what the limitation means for you, and what
workaround or roadmap path exists.

---

## Priority 1 — Affects the default installation experience

### LIM-01: Fleet resource gauges (CPU, memory, disk) are blank for standalone AMS

**What it means for you:** The Pulse Fleet page shows your AMS node with OS, JVM,
and version info, but CPU %, memory %, and disk % gauges are empty. You cannot
set CPU or memory alert rules for a standalone AMS deployment.

**Root cause:** AMS 3.x `GET /rest/v2/system-status` for standalone nodes returns
only `{osName, osArch, javaVersion, processorCount}` — no CPU, memory, or disk
fields (AV-06; `docs/assessment/capability-map.md` §5). These metrics are
available via Kafka (topic `ams-server-events` — name is code-derived,
`server/internal/collector/kafka/kafka.go:35,58,100`; not wire-validated against
a live AMS broker, AV-15 BLOCKED — see LIM-19).

**Workaround:** Set `PULSE_KAFKA_BROKERS=<your-kafka>` to activate the Kafka
consumer (`server/internal/collector/kafka/`). **Read LIM-19 first** — the
Kafka integration has never been live-validated against a real AMS broker; gauges
may stay empty if the topic or field names do not match your AMS version. If you
run AMS in cluster mode, cluster-node REST responses include `cpuUsage` and
`memoryUsage` fields (no Kafka needed).

**Roadmap:** DG-05; roadmap item P1 "Standalone CPU/mem/disk via Kafka"
(`docs/assessment/final-assessment.md` §5).

---

### LIM-02: HLS viewer count is approximately 9× higher than the real session count

**What it means for you:** `hlsViewerCount` in Pulse mirrors the AMS value
exactly. With 5 real HLS viewers, Pulse showed a count of 45; the count did not
drop below 38 for 90 seconds after 3 of 5 viewers stopped.

**Root cause:** AMS `hlsViewerCount` is a sliding segment-request window, not a
true session count. Each HLS player requests multiple segments per second; each
request extends the window. The window expiry is longer than most session
durations. This is AMS platform behavior, not a Pulse defect (TC-V-06, S18;
`docs/assessment/final-assessment.md` Appendix B; DG-01).

**Workaround:** Treat `hlsViewerCount` as a request-rate proxy, not a unique
session count. Q2 in `docs/assessment/final-assessment.md` §6 asks the AMS team
whether a session-accurate count is available.

**Roadmap:** Awaiting AMS team input (Q2). If a session-accurate HLS endpoint is
found, Pulse will expose it.

---

### LIM-03: Webhook path is non-functional on AMS 3.0.3; VoD recording has ≤60 s visibility latency

**What it means for you:** AMS 3.0.3 exposes `listenerHookURL` but provides no
HMAC secret field. Pulse's webhook listener is fail-closed (rejects all unsigned
deliveries). Do not point `listenerHookURL` at Pulse — you will see only 401
errors and no benefit.

VoD recording (`recording_gb` in usage reports) is populated via a REST poll of
`/{app}/rest/v2/vods/list` that runs every ~60 s (12th REST tick at default 5 s
interval). New VoD recordings may appear in reports up to 60 s after AMS registers
them. Stream lifecycle detection (start/stop) is not affected — REST polling covers
that within the ≤10 s PRD budget (4 s confirmed, TC-WH-02).

**Root cause:** AMS 3.0.3 cannot sign webhooks (O3 decision, `decisions.md`;
AV-08 confirmed). BUG-002 was fixed in S23/D-085 by the VoD REST poll fallback;
the 60 s polling lag is a residual of that fix (DG-04).

**Workaround:** No action needed for stream lifecycle. For VoD recording accuracy,
rely on the REST poll (already active).

**Roadmap:** D-V2-1 (OPEN) — unsigned-webhook ingest mode gated on a
`PULSE_WEBHOOK_ALLOW_UNSIGNED_SOURCES` CIDR allowlist (operator decision pending;
`agents/handoffs/ROADMAP-V2.md` §2.6). A future AMS version with HMAC signing
would cut recording visibility latency to near-real-time.

---

### LIM-04: FPS is always 0 for streams polled from AMS 3.x REST

**What it means for you:** The Pulse ingest health view shows `fps = 0` for every
stream, regardless of actual frame rate. The health score formula redistributes
FPS weight so a healthy 2 Mbps stream still scores above 80 (confirmed TC-I-06).

**Root cause:** `currentFPS` is absent from the AMS 3.0.3 BroadcastDTO REST
response (AV-04; `server/pkg/amsclient/client.go:97` comment). FPS data is
expected on the `ams-server-events` Kafka topic (name code-derived,
`server/internal/collector/kafka/kafka.go:35,58,100`; not wire-validated,
AV-15 BLOCKED — see LIM-19); the Kafka FPS field name is also unconfirmed
(Q4 in `docs/assessment/final-assessment.md` §6; DG-03).

**Workaround:** Use the health score as the primary ingest-health signal rather
than raw FPS. Enable Kafka (`PULSE_KAFKA_BROKERS`) if your deployment has a broker.

**Roadmap:** Awaiting AMS team confirmation of the Kafka FPS field name (Q4).

---

### LIM-05: Geo country analytics require a MaxMind GeoLite2 database that is not bundled

**What it means for you:** `GET /api/v1/analytics/geo` returns rows with
`country: ""` (blank country) until you configure a GeoLite2-City mmdb file.
The geo analytics feature is structurally functional — the ClickHouse query runs
and returns real rows — but country enrichment is inactive without the database
(AV-10, AV-11; DG-17).

**Workaround:** Obtain GeoLite2-City.mmdb from MaxMind (free account at
maxmind.com), mount it into the Pulse container, and set
`PULSE_GEO_MMDB_PATH=/path/to/GeoLite2-City.mmdb` in `deploy/.env`.

**Roadmap:** DG-17; PRD §7.9 F2; roadmap item P2 "GeoLite2 mmdb bundling or
setup guide" (`docs/assessment/final-assessment.md` §5). Licensing (OFL vs MaxMind
EULA) determines whether the mmdb can be bundled in the Docker image.

---

## Priority 2 — Affects specific metrics or configurations

### LIM-06: Error-rate and rebuffer-ratio anomaly signals are not implemented (F9 sparsity gate)

**What it means for you:** The PRD F9 anomaly detection covers `viewers` and
`bitrate` deviations (implemented, 0.259 false alarms/node-week). It does NOT
cover `error_rate` or `rebuffer_ratio`, which the PRD also lists. TC-AN-05 confirms
these signals are absent from the anomaly evaluator.

**Why the delay:** In the production beacon deployment, `beacon_events` = 2 rows
over 1 stream; in the `realams` environment = 0 rows. Welford baselines built on
all-zero data make the first real rebuffer event an instant false alarm — a PRD
violation. Additionally, `rollup_qoe_1h` accumulates within the hour, so up to 30
Welford ticks read non-independent samples; sub-hour windowing is needed before
this is safe to enable (S25/D-087 assessment; `docs/assessment/prd-validation-matrix.md`
F9 MISSING row).

**Workaround:** None currently. Monitor rebuffer ratios via `GET /api/v1/qoe/summary`.

**Roadmap:** P1 "error\_rate + rebuffer\_ratio anomaly signals" pending windowing
redesign (`docs/assessment/final-assessment.md` §5).

---

### LIM-07: Egress GB is a bitrate×watch-time estimate, not measured delivered bytes

**What it means for you:** `egress_gb` in `GET /api/v1/reports/usage` is computed
as `bitrate × watch_time` stored in `mv_usage_1d`. It is a heuristic proxy for
network egress, not a measurement from a CDN or network switch (AV-09; DG-06).

**Example:** One beacon session of a 2 Mbps stream for 20 s produced
`egress_gb = 0.0025` — this is the model estimate, not a counter of actual bytes
delivered to the viewer's NIC.

**Workaround:** Use CDN access logs or network flow data for billing-grade egress
accounting. Pulse's `egress_gb` is useful for directional estimates and relative
comparisons, not for invoicing.

**Roadmap:** DG-06; PRD §7.9 F6 notes this limitation in the product spec
("Egress estimated from delivered-bytes events where available, else
bitrate×watch-time model with method disclosed on the report").

---

### LIM-08: Packet loss (`packetLostRatio`) is always 0 for RTMP ingest

**What it means for you:** AMS `packetLostRatio` and `packetsLost` are populated
only by UDP-based ingest paths (WebRTC, SRT). For RTMP ingest, TCP retransmission
repairs packet loss below the application layer before AMS observes the stream.
Even with 10% netem loss injected on the publisher's NIC, AMS reported
`packetLostRatio=0.0` and `bitrate≈2 Mbps` (TC-I-05, S18; DG-18).

**What this means:** Monitoring `packetLostRatio` is only meaningful for WebRTC
and SRT ingest paths. RTMP ingest always shows 0 — this is correct for RTMP, not
a Pulse defect.

**Workaround:** If packet loss monitoring is critical, switch to WebRTC or SRT
ingest.

---

### LIM-09: RTMP-pull viewer count shows 0

**What it means for you:** For RTMP pull streams (where AMS pulls from a source
rather than receiving a push), `rtmpViewerCount` inline in BroadcastDTO is 0.
The `broadcast-statistics` endpoint (now dead code, removed in S26/D-088 BUG-001
FIXED) returned `-1` as an "untracked" sentinel for pull viewers — Pulse was
never calling it. The inline RTMP count is the actual value for push
streams and is correct there (AV-02, AV-16; DG-02).

**Workaround:** RTMP pull viewer tracking is a roadmap P2 item that would require
polling the per-connection `/{app}/connections` endpoint.

**Roadmap:** P2 "RTMP pull viewer count via `/{app}/connections`"
(`docs/assessment/final-assessment.md` §5).

---

### LIM-10: Cluster mode is not live-validated against a real multi-node AMS

**What it means for you:** Edge/origin viewer dedup (`IsEdgeStream()`), cluster
node auto-discovery, and aggregate fleet metrics are implemented and unit-tested.
None of these were validated against a real multi-node AMS cluster — the validation
VPS ran single-node AMS only.

**What is proven:** Standalone mode is fully live-validated (46/50 scenarios PASS);
`IsEdgeStream()` dedup logic is unit-tested; node discovery completes in 24.4 ms
in CI.

**Workaround:** If you run a multi-node AMS cluster, treat cluster-specific behavior
(edge role labeling, dedup) as provisionally implemented, not live-proven. Report
any discrepancies as issues.

**Roadmap:** `docs/assessment/final-assessment.md` §1 "Honest limitations";
N3 PARTIALLY in prd-validation-matrix.md.

---

### LIM-11: WebRTC remote viewer QoE stats (RTT/jitter/loss) validated at 0 only

**What it means for you:** The ×1000 unit conversion for WebRTC viewer stats
(ms to µs, `normalize.go:185`) is code-verified, but was exercised only with
same-host loopback WebRTC viewers that return all-zero AMS stats. Non-zero RTT,
jitter, and loss values from a geographically remote WebRTC viewer have not been
validated against real AMS.

**What is proven:** The conversion formula is correct at line 185. RTT/jitter/loss
keys are present in probe results (TC-P-01, S17). A flip to ×0.001 would produce
µs-range values silently — the unit conversion direction has not been validated
at non-zero values.

**Workaround:** If your viewers are geographically distant from AMS, treat WebRTC
RTT/jitter/loss values as unvalidated. Report suspicious readings.

**Roadmap:** P1 "Remote-viewer WebRTC QoE parity" (`docs/assessment/final-assessment.md` §5).

---

### LIM-19: Kafka consumer never live-validated against a real AMS broker (AV-15 BLOCKED)

**What it means for you:** Setting `PULSE_KAFKA_BROKERS` activates the Kafka
consumer path — the only route to Fleet CPU/memory/disk gauges on standalone AMS
(LIM-01) and to FPS data (LIM-04). The consumer is code-complete and covered by
8 contract tests against an in-process fake broker. However, it has **never** been
connected to a real AMS Kafka producer. The topic name (`ams-server-events`,
code-derived — `server/internal/collector/kafka/kafka.go` lines 35, 58, 100), field
names, and message shapes are unconfirmed against any AMS version. Gauges may stay
permanently empty if the topic or field names do not match what your AMS version
publishes.

**Root cause:** AV-15 BLOCKED — no Kafka broker was deployed in any Pulse
validation environment as of S27/D-089. The `kafka.go` package comment (lines 10–11)
states this explicitly. Neither topic name (`ams-server-events`) nor any field name
has been verified against a live AMS broker.

**Workaround:** Before enabling the Kafka path in production, confirm the actual
topic name and field names against your AMS version and `docs.antmedia.io`.
Diagnostic steps: `docs/kafka-integration.md` §6.3. If `GET /healthz` shows
`lag = 0` more than 30 seconds after connecting a broker, AMS is likely not
publishing to `ams-server-events`, or the topic name differs — a silent failure
with no parse errors in `/healthz` output.

**Roadmap:** AV-15 validation requires a live Kafka broker connected to a real AMS
instance. Until resolved, treat the Kafka ingest path as provisionally implemented
(`docs/assessment/final-assessment.md` §5 P1 "Standalone CPU/mem/disk via Kafka").

---

### LIM-20: Kafka transport is plaintext-only — no TLS or SASL

**What it means for you:** All Kafka traffic between Pulse and your broker is
unencrypted and unauthenticated. There is no TLS transport encryption or SASL
credential authentication support in this release. Any host that can reach your
Kafka broker port can read or write messages without authentication.

**Root cause:** The `kafkago.ReaderConfig` constructed in
`server/internal/collector/kafka/kafka.go` lines 130–138 sets only `Brokers`,
`GroupID`, `GroupTopics`, `StartOffset`, `MaxWait`, `MinBytes`, and `MaxBytes`.
No `Dialer`, `TLS`, or `SASL` field is present (verified against current source;
`docs/kafka-integration.md` §2.3 documents this constraint).

**Workaround:** Restrict access to your Kafka broker port via network-layer controls
(VPC ACLs, security groups, host firewall rules) so that only the Pulse container
and AMS can reach it. Do not expose the Kafka broker port on an untrusted or
public-facing network segment.

**Roadmap:** TLS + SASL authentication is a P2 roadmap item for the Kafka source.

---

### LIM-21: Kafka at-least-once delivery and first-start history replay

**What it means for you:** Two related behaviors affect Kafka event ingestion:

1. **At-least-once delivery.** If the Pulse process crashes after processing a
   message but before committing the Kafka offset, that message is redelivered on
   the next startup. A duplicate metric row may appear in ClickHouse within the
   crash window.

2. **First-start history replay.** On the first start with a fresh `pulse-collector`
   consumer group (no committed offsets on the broker), Pulse begins consuming from
   the **earliest retained message** in the topic, not from the current position. All
   history retained by your broker is ingested once. Subsequent restarts resume from
   the committed offset — no re-replay.

**Root cause:**

1. The commit follows the write: `server/internal/collector/kafka/kafka.go`
   lines 163–169 call `r.CommitMessages` after `processMessage` returns. A process
   crash between the two leaves the offset uncommitted.
2. `server/cmd/pulse/serve.go` lines 280–287 construct `kafkasrc.Config` without
   setting `StartOffset`; the Go zero-value (`0`) flows into
   `kafkago.ReaderConfig.StartOffset`. kafka-go treats `0` as `FirstOffset` for
   consumer groups with no committed offsets (`consumergroup.go:243`; documented in
   `docs/kafka-integration.md` §5.4). `DefaultConfig` (kafka.go:60) sets
   `kafkago.LastOffset` but is not called by `serve.go`.

**Workaround:** ClickHouse deduplication is not implemented for Kafka events; a
crash-recovery replay may produce duplicate metric rows in the crash window. For
the history replay: if your broker retains a large backlog (days or weeks), the
first-start ingest may take time to process. A short broker retention policy
(e.g., 1–2 hours) limits the replay volume on first connect.

---

## Priority 3 — Configuration and semantic gotchas

### LIM-12: AMS VPS capacity limits stream count (AMS / OS constraint, not Pulse)

**What it means for you:** On a standard VPS running AMS, the practical concurrent
RTMP stream limit is approximately 5–7 publishers. All additional publishers are
rejected by AMS with "current system resources not enough." This is an AMS / OS
resource limit, not a Pulse scalability limit.

**What Pulse supports:** All scale claims at N = 500 concurrent streams and
N = 3,000 concurrent viewers are CI-verified with mock-ams (SESSION-07, D-064;
`docs/assessment/final-assessment.md` §1). Pulse uses 18.6 MiB peak memory at
that scale. The stream limit you hit in production is AMS's, not Pulse's.

**Workaround:** Size your AMS VPS (RAM, CPU) for your expected concurrent stream
count. Pulse imposes no practical stream limit.

---

### LIM-13: `speed_read_kbps` stores the AMS real-time ratio (~1.0), not a bitrate

**What it means for you:** In the Pulse ingest event schema and API, the column
`speed_read_kbps` stores the AMS `speed` ratio (approximately 1.0 for a healthy
ingest, <0.8 indicates ingestion backpressure). It is NOT a bitrate in kbps.
A healthy 2 Mbps stream shows `speed_read_kbps ≈ 1.02` — not 2000 (DG-16;
`docs/assessment/capability-map.md` §4 "MISLEADING: this is the AMS realtime ratio").

**Workaround:** Use `bitrate_kbps` for the actual ingest bitrate. Treat
`speed_read_kbps > 0.8` as healthy, `< 0.8` as under backpressure.

---

### LIM-14: Each AMS application must have `remoteAllowedCIDR` opened for Pulse

**What it means for you:** AMS controls REST API access per-application via
`remoteAllowedCIDR`. If an app is set to `remoteAllowedCIDR=127.0.0.1` only,
Pulse receives HTTP 403 and logs a warning every poll cycle. Apps with restricted
CIDR are silently excluded from monitoring.

**Workaround:** For each AMS application Pulse should monitor, add the Pulse
container's IP to `remoteAllowedCIDR` in the AMS console. TC-APP-02 was SKIP
in the S17 validation because all test apps were already CIDR-open (DG-08).

---

### LIM-15: AMS locks a login account after 2 failed attempts for 5 minutes

**What it means for you:** AMS enforces a per-email (not per-IP) brute-force
lockout: 2 failed login attempts on any account lock that email for 5 minutes,
returning "User is blocked" even for correct passwords. If you use the same account
for `PULSE_AMS_LOGIN_EMAIL` as for human console sessions, a mistyped console
password will disrupt Pulse polling for 5 minutes (memory: `ams-brute-force-lockout.md`; DG-09).

**Workaround:** Use a dedicated AMS account (e.g., `pulse-service@`) for
`PULSE_AMS_LOGIN_EMAIL` and a separate account for human console logins.

---

### LIM-16: HLS probe URL must use the flat path form `/{app}/streams/{id}.m3u8`

**What it means for you:** AMS 3.0.3 serves HLS at `/{app}/streams/{id}.m3u8`.
The nested form `/{app}/streams/{id}/playlist.m3u8` returns 404 on AMS 3.0.3
build 20260504\_1443. If you create HLS probes with the nested path, they will
always report `success=false` with `error_code=http_4xx` (DG-10; S17 corrections;
TC-P-04).

**Workaround:** Use the flat form: `http://<ams-host>:5080/<app>/streams/<id>.m3u8`.

---

### LIM-17: SRT ingest packet loss before AMS ARQ correction is not instrumented

**What it means for you:** `packetLostRatio` for SRT ingest reflects the
BroadcastDTO value, which is the post-ARQ packet loss seen by AMS after SRT
error-correction. Transport-layer packet loss that SRT's ARQ mechanism repaired
before delivering to AMS is invisible to Pulse. You may see `packetLostRatio = 0`
even when the SRT link has meaningful transport-level loss (DG-18;
`docs/assessment/final-assessment.md` §4.2).

**Workaround:** Monitor SRT link health at the network level (e.g., sFlow, netflow,
SRT socket statistics from the publisher side) for pre-ARQ loss.

**Validation status:** TC-I-05-SRT PASS (2/2 assertions, S31/D-093, 2026-07-14T02:29:45Z).
`packetLostRatio=0.0` on a clean loss run confirmed correct post-ARQ semantics.
See `docs/assessment/final-assessment.md` §5 and `docs/AMS-INTEGRATION.md` §1.1.

---

### LIM-18: WHEP viewer counts are not tracked

**What it means for you:** WHEP (WebRTC HTTP Egress Protocol) viewer counts are
not separately exposed in the AMS 3.0.3 BroadcastDTO inline fields observed during
validation. WHIP (WebRTC HTTP Ingest Protocol) publisher counts are visible as
WebRTC publishers. WHEP viewer counts, if accessible via a separate endpoint, are
not consumed by Pulse (`docs/assessment/final-assessment.md` §4.5; Q3).

**Workaround:** None currently. The total `viewer_count` field sums
`hlsViewerCount + webRTCViewerCount + rtmpViewerCount + dashViewerCount`
(`normalize.go:83`); any WHEP-specific count not in those fields is not included.

**Roadmap:** Q3 in `docs/assessment/final-assessment.md` §6 asks the AMS team
whether WHEP viewer counts are exposed separately.

---

### LIM-22: First viewer on a zero-viewer-history stream fires an anomaly z-spike (intentional)

**What it means for you:** If a stream has been live with zero viewers for more
than approximately 30 minutes (the `MinSamples = 30` Welford warmup gate, one tick
per minute), the anomaly detector builds a `mean = 0, stddev = 0` viewer baseline.
When the first viewer connects, the detector fires a very high-sigma anomaly flag
(approximately 10⁹ σ). This is logged and queryable via `GET /api/v1/anomalies`.

This is **not a false alarm in the statistical sense** — "audience appeared after a
long quiet period" is a genuine deviation from baseline behavior, and the ruling
at S28/D-090 is to keep it (`docs/guides/anomaly-detection.md` §2 "Zero-viewer
baselines and the first-viewer spike"; `docs/operator-expected.md` §2.17.1).

**Root cause:** The viewer count is fed to the Welford accumulator unconditionally
for every active stream tick — `0` is a real measurement, not a sentinel.
After a zero-viewer history the effective-stddev floor at detection time collapses
to `StddevAbsEpsilon = 1e-9`, producing
`z = |1 − 0| / 1e-9 ≈ 10⁹` on first-viewer arrival. Spike mechanics verified in
`docs/guides/anomaly-detection.md` §2. Contrast: `cpu_pct`/`mem_pct`/`disk_pct`
use `CPUPCTReported`/`MemPCTReported`/`DiskPCTReported` presence flags to suppress
false-zero feeding when the metric is absent (standalone REST — LIM-01); viewer
count has no such guard because 0 is always a real value.

**Workaround:** If the spike is noise for your deployment, raise `min_sigma` on
viewer-metric queries:

```
GET /api/v1/anomalies?metric=viewers&min_sigma=10
```

A `min_sigma` of 10 suppresses the first-viewer spike while still surfacing
sustained high-anomaly events.

**Note — Enterprise tier required:** Anomaly detection is gated to Enterprise tier.
`GET /api/v1/anomalies` returns `403 LICENSE_REQUIRED` on Free or Pro licenses
(`server/internal/api/wave3.go` lines 4–5, 45–48;
`server/internal/license/license.go` lines 356–363).

**Roadmap:** An observation-side skip (analogous to the `APILatencyMS > 0`
presence guard — a ~2-line change) remains an open follow-up option if operators
determine this spike is systematically unwanted
(`docs/guides/anomaly-detection.md` §2).

---

### LIM-23: SRT streams are attributed as RTMP in Pulse's protocol breakdown

**What it means for you:** When a publisher connects via SRT, Pulse reports that
stream as `protocol=RTMP` in protocol breakdown charts (ProtocolDonut) and protocol
filter dropdowns. A deployment that mixes SRT and RTMP ingest will overcount RTMP
publishers and show zero SRT publishers.

**Root cause:** AMS 3.0.3 EE's `BroadcastDTO` reports `publishType="RTMP"` for
SRT-ingested streams. This was live-observed during the first successful SRT ingest
validation run (S31/D-093, 2026-07-14T02:29:45Z; evidence:
`qa/realams/evidence/TC-I-05-SRT-20260714T022945Z/`). Pulse reads `publishType`
verbatim at `server/pkg/amsclient/client.go:88` (field `PublishType`; the inline
comment at that line documents the known value set as `"webrtc|rtmp|hls"` — `"srt"`
is absent because AMS 3.0.3 does not emit it). This value is stored and forwarded
to the UI without transformation. Distinguishing SRT from RTMP would require an
out-of-band heuristic (e.g., matching the publisher's source port to AMS's SRT
listen port 4200); no such heuristic is implemented.

**Workaround:** Cross-reference SRT publisher counts using the AMS Management Console
directly. As a supplementary signal: RTMP ingest always shows `packetLostRatio=0`
(LIM-08, TCP absorbs loss); SRT may show non-zero post-ARQ `packetLostRatio` on a
degraded link (LIM-17). On a clean link both protocols show 0, so the signal only
helps on impaired links.

**Roadmap:** Accurate SRT attribution requires AMS to emit `publishType="SRT"` in
BroadcastDTO, or a port-based heuristic added to the collector. No change is planned
for this release.

---

### LIM-24: Interactive PDF export is not implemented (scheduled PDF reports ARE)

**What it means for you:** The Reports page's on-demand export offers only CSV
(`GET /api/v1/reports/export?format=csv`); requesting `format=pdf` there returns
`501 NOT_IMPLEMENTED`, and the "Export PDF" button has been removed. **Scheduled
reports are a separate, implemented path:** a report schedule with `format: pdf`
generates a real PDF statement each run (Business+ tier), with the logo set by
`PULSE_REPORT_LOGO_PATH`; the custom white-label header (your company name and
address on the statement) additionally requires an Enterprise license with the
`white_label` claim. (Verified D-161: `server/internal/api/export.go:40-44`;
`server/internal/reports/scheduler.go:255-277`; `statement.go:194,265`.)

**Root cause:** The interactive export endpoint predates the scheduled-statement
renderer and was shipped CSV-first; wiring the on-demand path to the PDF renderer
is pending. Shipping a button that errors is worse than not shipping the button.

**Workaround:** For a one-off PDF, create a schedule with `format: pdf` for the
period you need (then delete it), use CSV export in a spreadsheet, or use your
browser's Print → Save as PDF. (The CSV is formula-injection-safe as of D-106 —
publisher-controlled cells that begin with `= + - @` are neutralized, so opening
the export in a spreadsheet cannot execute an injected formula.)

**Roadmap:** Wire `GET /reports/export?format=pdf` to the existing statement
renderer. No ETA. File a feature request if this is blocking your use case.

---

### LIM-25: User management has no web UI yet (API only)

**What it means for you:** Settings → Users shows "User management — coming in a
future update." Creating, updating, and deleting Pulse users works today only via
the API: `GET/POST /api/v1/admin/users`, `PUT/DELETE /api/v1/admin/users/{userId}`
(admin-scoped token required; every change is recorded in the audit log). SSO/OIDC
user provisioning (Enterprise) is unaffected — first-login provisioning works end
to end.

**Root cause:** The users API shipped with full test coverage (D-100); the
management UI tab was deferred behind higher-priority dashboard work.

**Workaround:** Use the API directly (see `docs/api-guide.md`), or manage access
via API tokens (Settings → API Tokens, which has a full UI) instead of named users.

**Roadmap:** UI tab planned; no ETA.

---

### LIM-26: A firing alert can outlive a source that disappears entirely (non-"Stream offline" rules)

**What it means for you:** If a node/QoE/threshold alert (anything except
"Stream offline") is currently **firing** and the stream or node it watches then
vanishes completely from monitoring (deleted, renamed, decommissioned), the alert
stays "firing" — there is no rule that says "the thing I was watching is gone,
so resolve." "Stream offline" rules are unaffected (absence *is* their signal,
and they auto-resolve correctly, D-157/D-159). This only occurs when a source
disappears *while* its alert is firing — rare outside high-churn labs.

**Root cause:** Resolving on disappearance is ambiguous for value-threshold
metrics (gone ≠ recovered). The correct behavior — auto-resolve after a grace
window vs. stay firing until acknowledged — is an open product decision
(`agents/handoffs/ROADMAP-V2.md` §2.44, `[FO-1]`), deliberately not guessed.

**Workaround:** Delete or disable the rule to clear the stuck alert (re-enabling
a rule is safe — its evaluation state is rebuilt from live data on the next
tick), or let the source return and recover normally.

**Roadmap:** Pending the §2.44 product ruling; the fix ships once the semantics
are chosen.

---

## Changelog

| Version | Change |
|---------|--------|
| D-089 (S27, 2026-07-13) | Initial document — 18 limitations from DG-01–DG-18 + final-assessment §1 + §4 |
| D-091 (S29, 2026-07-13) | Added LIM-19..LIM-22: Kafka live-validation gap (AV-15), Kafka plaintext-only transport, at-least-once delivery + first-start history replay, first-viewer z-spike intentional ruling; corrected topic name ams-instance-stats → ams-server-events in LIM-01 + LIM-04 with code-derived caveat and AV-15 forward pointer; count 18 → 22 |
| D-093 (S31, 2026-07-14) | LIM-17 roadmap → Validation status: TC-I-05-SRT PASS (F3, first live SRT run, 2/2 assertions); added LIM-23: SRT streams attributed as RTMP in protocol breakdown (AMS-side publishType="RTMP" fact, F5, live-observed S31); updated product version header; count 22 → 23 |
| D-094 (S32, 2026-07-14) | Added LIM-24: PDF export not implemented (Phase 3); removed Export PDF button from Reports page; implemented GET /api/v1/reports/export?format=csv; count 23 → 24 |
| D-106 (S44, 2026-07-15) | Security hardening (not a new limitation): CSV export/statements now neutralize formula-injection cells (LIM-24 workaround note updated); email/SMTP channel creds encrypted at rest; OIDC state cookie Secure on HTTPS. No count change |
| D-161 (S97, 2026-07-22) | Marketplace-docs fact sweep: header → v0.4.0; LIM-24 corrected (scheduled PDF reports ARE implemented, Business+; only interactive export is CSV-only); added LIM-25 (user management API-only, no UI tab yet) and LIM-26 (firing alert can outlive a vanished source, pending §2.44 `[FO-1]` ruling); count 24 → 26 |

---

*All claims in this document are traceable to primary evidence cited inline.
See `docs/assessment/documentation-gaps.md` for the full gap detail and
authoring history. See `docs/assessment/final-assessment.md` for the validation
program results this document summarizes.*
