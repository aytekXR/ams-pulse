# Pulse Monitoring Runbook

**Target:** `pulse-prod` stack on `beyondkaira.com` (VPS `161.97.172.146`).
**Authored:** 2026-07-09 (D-062 SESSION-06).
**Scope:** operational health signals; what to watch and when to act.

---

## Backup daemon health

The backup sidecar runs `pulse-backup.sh` in daemon mode: one cycle immediately on start,
then every 24 hours. Retention: **7 most-recent artifacts per type** (CH zip + SQLite
db/wal/shm triplet) in the `pulse-prod_pulse-backups` Docker volume.

**Confirming the last cycle succeeded:**

```sh
# View last few cycle results:
sg docker -c "docker compose -p pulse-prod \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.backup.yml \
  logs backup | grep 'Backup cycle' | tail -5"
```

| Log line | Meaning |
|---|---|
| `[pulse-backup] HH:MM:SS Backup cycle complete (ts=<ts>)` | Cycle fully succeeded |
| `[pulse-backup] ERROR: Backup cycle COMPLETED WITH ERRORS (ts=<ts>) — check logs above` | At least one store failed; read the preceding ERROR lines (the error path uses the `ERROR:` prefix via `err()`, not a timestamp) |

A failed cycle does NOT crash the daemon or the stack. Fix the root cause (disk full,
ClickHouse unreachable) then run a manual one-shot:

```sh
sg docker -c "docker compose -p pulse-prod \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.backup.yml \
  exec backup /scripts/pulse-backup.sh once"
```

**Confirming artifacts exist:**

```sh
sg docker -c "docker run --rm \
  -v pulse-prod_pulse-backups:/backups:ro \
  busybox ls -lhR /backups"
# Expect ≤7 .zip files under /backups/ch/ and ≤7 .db files under /backups/meta/
```

---

## alert_history growth

`alert_history` is **self-capped at 1000 rows per `rule_id`** by `AlertHistoryDefaultKeep = 1000`
(`server/internal/store/meta/meta.go:45`). `CreateAlertHistory` calls `PruneAlertHistory` after
every insert; excess rows (oldest `ts ASC`, then `rowid ASC`) are deleted in one `DELETE`.
The design is O(excess), not O(n²). **No operator action is needed for normal operation.**

**What to check if the table appears to grow unboundedly:**

```sh
# Count rows per rule (run from the VPS with the meta store accessible):
sg docker -c "docker run --rm \
  -v pulse-prod_pulse-data:/data:ro \
  alpine sh -c \"apk add -q sqlite && sqlite3 /data/pulse_meta.db \
    \\\"SELECT rule_id, COUNT(*) AS n FROM alert_history GROUP BY rule_id ORDER BY n DESC LIMIT 20;\\\"\""
```

If any rule_id has n > 1000, `PruneAlertHistory` is failing. Check for:
1. SQLite write errors in pulse logs: `sg docker -c "docker compose ... logs pulse | grep -i 'prune\|alert_history'"`.
2. Disk full on the `pulse-data` volume (see Disk section below).
3. Burst of evaluator ticks faster than prune can keep up — transient; resolves on its own.

---

## Disk watch (ClickHouse volume)

ClickHouse stores all time-series metrics in the `pulse-prod_clickhouse-data` volume.
Raw event data is retained for `PULSE_RETENTION_DAYS=90` days; rollups for 395 days.

**Check volume size:**

```sh
sg docker -c "docker run --rm \
  -v pulse-prod_clickhouse-data:/data:ro \
  busybox du -sh /data"
```

Expected on a single-node AMS with modest stream counts: well under 10 GB after 90 days.
Alert when the VPS disk usage (including OS + compose layers) exceeds 80%.

```sh
df -h /    # host filesystem
```

---

## Prometheus /metrics

`/metrics` requires **Business+ tier** (returns `403 LICENSE_REQUIRED` for Free/Pro).
If `PULSE_METRICS_TOKEN` is set, scrapers must send `Authorization: Bearer <token>`;
comparison is constant-time. Rate-limited at 10 rps / burst 20 per IP before token check.

Verified: `server/internal/api/server.go:688-696`; `server/internal/license/license.go:351-357`.

**The 7 registered metrics (all gauges, text/plain; version=0.0.4):**

| Metric name | Labels | Description |
|---|---|---|
| `pulse_live_viewers` | — | Current live viewer count |
| `pulse_live_streams` | — | Current active stream count |
| `pulse_live_publishers` | — | Current publishing stream count |
| `pulse_ingest_bitrate_kbps` | — | Aggregate ingest bitrate (kbps) |
| `pulse_node_cpu_pct` | `node` | Node CPU utilization percent |
| `pulse_node_mem_pct` | `node` | Node memory utilization percent |
| `pulse_alerts_firing` | — | Total firing alert count |

Verified: `server/internal/api/server.go:725-749`. No `collector_errors_total` metric exists
in this codebase.

**Example Prometheus scrape config:**

```yaml
scrape_configs:
  - job_name: pulse
    scheme: https
    tls_config:
      insecure_skip_verify: false   # Let's Encrypt cert; leave false
    authorization:
      credentials: "<PULSE_METRICS_TOKEN value>"
    static_configs:
      - targets: ["beyondkaira.com"]
```

**Key alerting signals:**

- `pulse_alerts_firing > 0` — at least one rule is firing; check the alert history UI.
- `pulse_live_publishers == 0` for > 5 min during expected broadcast hours — AMS source may be down.
- `pulse_node_cpu_pct{node=...} > 90` — investigate AMS node overload.

---

## D-062 ClickHouse memory-pressure WATCH

On 2026-07-09 (pre-swap prod logs) an intermittent ClickHouse error was observed:

```
Memory limit (total) exceeded 1.80 GiB
```

This occurred on `server_events` inserts. It did NOT recur in the post-swap window.
The hardened overlay sets a 2 GiB / 1.0 CPU limit on the `clickhouse` service
(`deploy/docker-compose.hardened.yml:93-97`), leaving ~200 MiB headroom above the
observed threshold — narrow, especially under burst ingestion.

**To check if it has recurred:**

```sh
sg docker -c "docker compose -p pulse-prod logs clickhouse | grep -i 'memory limit'" | tail -10
```

**If it recurs:** do not attempt to tune ClickHouse live. Open a work order for:
- Memory-tune the ClickHouse `max_memory_usage` config.
- Implement insert batching in `server/internal/store/clickhouse/clickhouse.go` to reduce
  peak memory per insert.
- Consider raising the compose limit beyond 2 GiB if the VPS has headroom.

Source: `agents/handoffs/decisions.md` D-062 WATCH note.

---

## Known benign ClickHouse startup messages

ClickHouse 24.8 may log `CANNOT_PARSE_INPUT` (Code 27) entries during first-boot
startup under concurrent-connection load (27 were observed in the D-064 A10 load
test). These are internal wire-protocol format probes from connections arriving
before the server is fully initialized — they do NOT indicate Pulse DDL failures
(the migration runner returns an error on any failed SQL statement, and CI's
integration harness fails loudly on migration errors).

To distinguish from real problems:

```sh
sg docker -c "docker compose -p pulse-prod logs clickhouse | grep -c CANNOT_PARSE_INPUT"
# Startup-window-only occurrences with no matching pulse-migrate error = cosmetic.
```

Any `SYNTAX_ERROR` (Code 62) in ClickHouse startup logs is NOT benign: it means
the `/docker-entrypoint-initdb.d/` anti-pattern was used (raw DDL with `{db}`
placeholders fed to ClickHouse directly) — ClickHouse aborts on the first
statement and NO tables are created. Schema must only be applied via the
`pulse-migrate` container. (D-065 WO-D root-cause.)

---

## WARN log taxonomy

### Expected — transient, self-recovering

These WARNs appear in normal operation. No operator action is required unless they are
sustained for more than a few minutes or paired with service-level errors.

| WARN text (prefix match) | Source file | Meaning |
|---|---|---|
| `clickhouse: connect failed, retrying` | `store/clickhouse/clickhouse.go:118` | CH starting up or briefly unreachable (open failed); resolves on its own |
| `clickhouse: ping failed, retrying` | `store/clickhouse/clickhouse.go:134` | CH opened but ping timed out; resolves on its own |
| `collector: source exited, restarting` | `collector/collector.go:87` | Supervisor restarted a poller after a crash; self-healing |
| `restpoller: poll error` / `app poll error` | `collector/restpoller/restpoller.go:115,164` | Transient AMS REST call failure; next poll will retry |
| `alert evaluator: list rules failed` | `alert/evaluator.go:266` | Transient SQLite read error listing rules; next tick retries |
| `alert evaluator: list channels failed — registry not updated` | `alert/evaluator.go:305` | Transient SQLite read error listing channels; registry unchanged this tick, next tick retries |
| `alert: qoe_reader not configured — rebuffer_ratio/error_rate rules skipped this tick` | `alert/wave2.go:86` | QoEReader not wired (Free tier, or QoE collector not running); fires at most once per tick by design (D-062 G6) |
| `alert: qoe_reader error — stream skipped for this tick` | `alert/wave2.go:94` | Transient QoE DB/CH error; stream skips one evaluation tick |
| `clickhouse: server event channel full, dropping event` | `store/clickhouse/clickhouse.go:232` | Backpressure from bursty AMS; individual event dropped, no data loss beyond that event |
| `kafka: commit offset failed` | `kafka.go:168` | Transient Kafka commit; will retry on next record |

Verified: `grep -rn 'slog.Warn' server/ --include='*.go'` (excluding test files).

### Actionable — requires operator investigation

These WARNs indicate misconfiguration, a missing asset, or a potential security event.

| WARN text (prefix match) | Source file | Required action |
|---|---|---|
| `pulse: AMS bearer token will travel in cleartext` | `cmd/pulse/main.go:134` | Set `PULSE_AMS_URL` to `https://` for remote AMS hosts |
| `geo: cannot open mmdb, geo enrichment disabled` | `collector/enrichment.go:121` | Mount a MaxMind GeoLite2-City.mmdb (see `docs/runbooks/install.md`) |
| `pulse: webhook: could not load per-source secrets` / `pulse: webhook: decrypt per-source secret failed, skipping` | `cmd/pulse/serve.go:288,294` | Key mismatch or corrupt `PULSE_SECRET_KEY`; check that the key has not been rotated (rotating HMAC key invalidates stored secrets); restart may not fix without key restore |
| `webhook: invalid signature` | `collector/webhook/webhook.go:161` | AMS webhook secret misconfiguration or replay attack; verify the AMS-side `X-Ams-Signature` secret matches `PULSE_WEBHOOK_SECRET` / per-source secret |
| `api: web UI assets not found; static serving disabled` | `api/server.go:430` | Deploy misconfiguration; the pulse binary must be built with web assets embedded |
| `license: init failed, using free tier` | `cmd/pulse/serve.go:238` | Bad, expired, or malformed `PULSE_LICENSE_KEY`; check the value and re-deploy |
