# WO-A1 (S105) — Kafka collector: align to the real AMS Kafka feed

**Review finding (Issue 1, P0, verified locally + against AMS source):** Pulse subscribes to a
topic AMS never publishes to (`ams-server-events`), parses flat fields the official messages
don't carry, and reads node identity from `nodeId` where AMS sends `instanceId`. Even with
correct brokers, Pulse receives nothing; if it did, it would parse zeros. Verified against
`StatsCollector.java` in the AMS GitHub source: AMS publishes to **`ams-instance-stats`** and
**`ams-webrtc-stats`** (a third topic, `kafka-webrtc-tester-stats`, belongs to the WebRTC load
tester tool, not the server); instance-stats messages carry `instanceId` plus **nested objects**
`cpuUsage` (with `processCpuTime`, `systemCpuLoad`, `processCpuLoad`, `systemLoadAverageLastMinute`),
`jvmMemoryUsage`, `systemMemoryInfo` (total/free/inUse + swap fields), `fileSystemInfo`
(`usableSpace`, `totalSpace`, `freeSpace`, `inUseSpace`).

## Scope — you may ONLY edit these files
- `server/internal/collector/kafka/kafka.go`
- `server/internal/collector/kafka/kafka_test.go`
- `server/internal/collector/kafka/testdata/` (new fixtures)
- `server/cmd/pulse/config.go`
- `server/cmd/pulse/serve.go`
- `server/cmd/pulse/*_test.go` (only if a config test needs extending)

Do NOT touch docs (a parallel agent owns them), do NOT run git commands, do NOT edit any other file.
Other agents are editing other files in this same working tree concurrently — that is expected.

## Work items

1. **Correct the default topics.** `DefaultConfig()` (kafka.go ~line 58) and the `New()` fallback
   (~line 100) currently use `[]string{"ams-server-events"}`. Change both to
   `[]string{"ams-instance-stats", "ams-webrtc-stats"}` and fix the struct comment (~line 35).
   Update `TestKafka_DefaultTopic` (kafka_test.go ~line 133) accordingly.

2. **Add `PULSE_KAFKA_TOPICS` env override.** In `server/cmd/pulse/config.go` next to the existing
   `PULSE_KAFKA_BROKERS` parsing (~lines 303-312): comma-separated list, entries trimmed, empty
   entries dropped, stored in a new `EnvConfig.KafkaTopics []string`. In `serve.go` (~line 289),
   pass `Topics: cfg.KafkaTopics` into `kafkasrc.Config` (empty slice → `New()` default applies).
   Follow the exact style of the neighboring env parsing code.

3. **Rewrite `normalizeKafkaMessage` to parse the official shapes, routed by topic.**
   First fetch the authoritative source to derive exact field names — do not guess:
   `https://raw.githubusercontent.com/ant-media/Ant-Media-Server/master/src/main/java/io/antmedia/statistic/StatsCollector.java`
   (also try the `ams-v2.14.0` ref for comparison; if both unreachable, use the field lists in the
   header of this WO, which were extracted from that file today).
   - Change the normalize path so the **topic name** of the consumed message is available for
     routing (thread it from the fetch loop). Route:
     - `ams-instance-stats` → `EventNodeStats`: node identity from `instanceId` (fallbacks:
       existing `nodeId` key, then cfg NodeID — preserve the current precedence rule where the
       configured NodeID wins); `cpu_pct` from nested `cpuUsage.systemCpuLoad` (document the
       choice of systemCpuLoad over processCpuLoad in a comment); `mem_pct` computed from
       `systemMemoryInfo` in-use vs total (guard div-by-zero); `disk_pct` from `fileSystemInfo`
       in-use vs total (guard div-by-zero).
     - `ams-webrtc-stats` → derive the message shape from StatsCollector.java; map to the closest
       existing event type (`EventIngestStats` or `EventStreamStats`) ONLY for fields that
       genuinely map; if the shape doesn't map cleanly, skip the message with a rate-limited/once
       debug log — do NOT fabricate metrics.
     - any other topic → keep the CURRENT legacy field-sniffing behavior as a fallback
       (back-compat for operators feeding a custom bridge topic), unchanged.

4. **Pin fixtures.** Put one realistic `ams-instance-stats` JSON message and one
   `ams-webrtc-stats` JSON message (field names verbatim from StatsCollector.java) in
   `server/internal/collector/kafka/testdata/`. Table-tests must load them and assert: correct
   event type, node id from `instanceId`, cpu/mem/disk all NON-zero, mem/disk percentages
   plausible (0 < pct <= 100).

5. **Env-gated integration test** (new function in kafka_test.go): when `PULSE_TEST_KAFKA_BROKERS`
   is set, produce the instance-stats fixture to a real broker on that address and assert the
   Source consumes + normalizes it end-to-end (use the same kafka-go writer the package already
   depends on). `t.Skip` when the env var is unset so CI is unaffected.

6. Keep the existing supervision/restart behavior and public API of the package intact.

## Definition of done
- `gofmt -l` clean on every touched file (CI hard-fails on this).
- `cd server && go build ./... && go test ./internal/collector/kafka/... ./cmd/pulse/...` green.
- Return (as your final structured output): files changed, test results, the exact fixture field
  names used, any deviation from this WO with reasoning, and anything you found that a
  live-validation run (AV-15) must still confirm.
