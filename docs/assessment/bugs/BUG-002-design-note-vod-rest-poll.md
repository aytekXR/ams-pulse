# DESIGN NOTE — BUG-002 fix: VoD REST poll fallback for `recording_gb`

**Status:** PROPOSAL — not implemented, not committed.  
**Authored:** S20 (2026-07-12), DOC/BE design author  
**Relates to:** BUG-002 (`docs/assessment/bugs/BUG-002-recording-gb-zero-webhook-blocked.md`),
  D-066 (O3 closed-N/A), D-V2-1 (unsigned-webhook mode, operator-gated),
  PRD F6 (Usage and Billing Reports)  
**Roadmap position:** P0 — `docs/assessment/final-assessment.md` §5 first item  
**Required before implementation:** ORCH-approved work order; INT-01 contract review
  (see §4 for scope)

---

## 1. Problem restated

`GET /api/v1/reports/usage` returns `totals.recording_gb: 0` on every AMS 3.0.3
deployment, even when AMS has real VoD assets.  The structural reason is a chain of
three facts.  First, Pulse ingests VoD recording data exclusively from the `vodReady`
lifecycle webhook (`webhook.go:translateWebhook()` maps it to `domain.EventRecordingReady`
at `server/internal/collector/webhook/webhook.go:243-249`).  Second, AMS 3.0.3 cannot
HMAC-sign outbound lifecycle hooks — verified live in D-066/O3
(`agents/handoffs/decisions.md:2404-2410`).  Third, Pulse's webhook listener is
fail-closed: an absent or invalid `X-Ams-Signature` header causes an unconditional
HTTP 401 rejection (`server/internal/collector/webhook/webhook.go:158-164`).  The
result is that `EventRecordingReady` never fires, no rows reach `server_events` with
`event_type='recording_ready'`, and the billing-path query
(`server/internal/reports/accounting.go:213-223`) reads `sum(recording_bytes)` from
`rollup_usage_1d` which is structurally zero because the only materialized view that
writes to that column (`mv_usage_1d`, `contracts/db/clickhouse/0001_init.sql:448-464`)
hard-codes `toUInt64(0) AS recording_bytes` — there is no MV that rolls
`server_events.recording_size` into `rollup_usage_1d.recording_bytes`.

There is a parallel data path already confirmed working: the AMS REST endpoint
`/{app}/rest/v2/vods/count` returns `{"number": N}` and
`/{app}/rest/v2/vods/list/{offset}/{size}` returns per-file metadata including sizes.
These are never polled by Pulse today
(`docs/assessment/capability-map.md:309-320`, `docs/AMS-INTEGRATION.md:387-402`).

---

## 2. Why the webhook path is dead (and must stay fail-closed)

D-066/O3 is a live-verified, closed decision
(`agents/handoffs/decisions.md:2404-2410`):

> "Authed live GET of AMS 3.0.3 LiveApp settings (182 fields): `listenerHookURL` +
> retry/content-type knobs exist, NO HMAC-secret or signature-header field —
> AMS lifecycle hooks are UNSIGNED; configuring them would only 401 against Pulse's
> fail-closed listener."

"Make AMS sign the webhook" is not an option in this design note.

Pulse's fail-closed posture is intentional and MUST remain:
`server/internal/collector/webhook/webhook.go:85-87` logs a startup error if no
shared secret is configured, and `webhook.go:158-164` rejects every request whose
`X-Ams-Signature` header does not pass `validateHMAC`.  Relaxing this without
additional safeguards would be a security regression.

D-V2-1 (`agents/handoffs/ROADMAP-V2.md:133-146`,
`agents/handoffs/decisions.md:712`) is the operator-gated open decision about
building an optional unsigned-ingest mode gated on a
`PULSE_WEBHOOK_ALLOW_UNSIGNED_SOURCES` IP CIDR allowlist.  That path is a separate
operator product call, not a dependency of this design note.  The VoD REST poll
proposed here works regardless of how or whether D-V2-1 is decided.

---

## 3. Proposed design: VoD REST poll

### 3.1 New amsclient method

The `server/pkg/amsclient/` package (`server/pkg/amsclient/client.go`) currently has
no VoD-related method.  The confirmed AMS route (from
`docs/assessment/capability-map.md:309` and `docs/AMS-INTEGRATION.md:402`) is:

```
GET /{app}/rest/v2/vods/list/{offset}/{size}
```

This returns a JSON array.  The exact field names of the response objects are
**ASSUMPTION-TO-VERIFY** (no fixture exists in the repo; probe the live AMS or the
AMS source/docs to confirm):

```go
// VodDTO is the AMS REST v2 VoD list entry.
// ASSUMPTION-TO-VERIFY: field names inferred from the vodReady webhook payload
// (vodName = path, vodSize = size bytes) and typical AMS REST conventions.
// Confirm against GET /pulse-test/rest/v2/vods/list/0/1 on the live AMS.
type VodDTO struct {
    VodID        string `json:"vodId"`        // ASSUMPTION — unique identifier
    VodName      string `json:"vodName"`      // path/filename, confirmed in webhook payload
    FileSize     int64  `json:"fileSize"`     // ASSUMPTION — may be vodSize or size
    CreationDate int64  `json:"creationDate"` // ASSUMPTION — Unix epoch ms
    StreamName   string `json:"streamName"`   // ASSUMPTION — originating stream
}

// ListVods returns one page of VoD records for an application.
// Path: /{app}/rest/v2/vods/list/{offset}/{size}
func (c *Client) ListVods(ctx context.Context, app string, offset, size int) ([]VodDTO, error) {
    if size <= 0 {
        size = 200
    }
    path := fmt.Sprintf("/%s/rest/v2/vods/list/%d/%d", app, offset, size)
    var result []VodDTO
    if err := c.getJSON(ctx, path, &result); err != nil {
        return nil, err
    }
    return result, nil
}
```

A `ListVodsPaged` wrapper (analogous to `ListBroadcastsPaged` at
`server/pkg/amsclient/client.go:457-479`) fetches all pages.

### 3.2 Poll loop integration

The existing `poll()` function (`server/internal/collector/restpoller/restpoller.go:123-172`)
already iterates per-app via `pollApp()`.  A new `pollVods()` function would be
called from within `pollApp()` at a lower frequency than the main broadcast poll
(default 5 s, `config.go:204`).

Recommended VoD poll interval: **60 s**.  Rationale: VoD creation is a low-frequency
event (a file is muxed once, not continuously updated), so 60 s latency is acceptable.
Polling on every 5 s tick would make 12 unnecessary AMS REST calls per minute per app.

Implementation options:

- **Option A (counter-based):** Add a `vodPollCounter` field to the `Poller` struct
  (`restpoller.go:47-57`).  Increment per tick; call `pollVods` when
  `vodPollCounter % 12 == 0` (every 12th 5 s tick ≈ 60 s).  Simplest change.
- **Option B (separate ticker):** Add a second `time.Ticker` with 60 s period inside
  `Run()` (`restpoller.go:98-119`), driven from the same goroutine via select.
  Cleaner separation; slightly more code.

Either option is acceptable.  Option A is smaller.

### 3.3 Emitting the domain event

For each new VoD discovered, the poller calls `p.sink.WriteServerEvent(ev)` with:

```go
ev := domain.ServerEvent{
    Version:  1,
    Type:     domain.EventRecordingReady,   // "recording_ready"
    TS:       time.Now().UnixMilli(),       // or VoD creationDate if available
    Source:   domain.SourceRestPoll,
    NodeID:   p.cfg.NodeID,
    App:      app,
    StreamID: vod.StreamName,              // ASSUMPTION: field name per 3.1
    Data: map[string]any{
        "path":       vod.VodName,
        "size_bytes": vod.FileSize,
    },
}
```

This is exactly the shape that `webhook.go:translateWebhook()` produces for
`vodReady` events (`webhook.go:243-249`).  The ClickHouse store already handles
it: `clickhouse.go:519-522` unpacks `path`, `size_bytes`, and `duration_s` into
`server_events.recording_path`, `recording_size`, and `recording_dur_s`.

The live aggregator (`aggregator.go:139-161`) does not need to be changed —
it has no `EventRecordingReady` case and recording data is ClickHouse-only.

### 3.4 Deduplication — the core design challenge

**A VoD polled twice must not double-count.**

The `rollup_usage_1d` table uses `SummingMergeTree` summing over `recording_bytes`
(`contracts/db/clickhouse/0001_init.sql:358`).  If the same VoD's `recording_ready`
event reaches ClickHouse more than once (two poll cycles, or a restart triggering a
full backfill), the sizes will be summed — doubling or tripling the true figure.

The existing broadcast deduplicator (`collector.NewDeduplicator` in
`restpoller.go:85`) is event-type-keyed with a TTL window.  It is designed for
frequent stream-stat events, not for once-per-lifetime VoD events.  It MUST NOT be
reused for recording deduplication without modification.

**Proposed deduplication mechanism: per-app high-water mark.**

Maintain a `vodHWM map[string]int64` in the `Poller` struct (key = app, value =
latest VoD creation timestamp seen).  On each poll cycle:

1. Fetch all VoDs for the app via `ListVodsPaged`.
2. Filter to VoDs whose `creationDate > vodHWM[app]`.
3. Emit `EventRecordingReady` for each new VoD (in creation-date order).
4. Advance `vodHWM[app]` to the maximum `creationDate` seen this cycle.

This guarantees each VoD fires exactly once per Pulse process lifetime.

**Cold-start / backfill behaviour:**

On a fresh Pulse start (no HWM state), `vodHWM[app] = 0` → all VoDs in AMS are
treated as new → one `EventRecordingReady` per VoD is emitted in the first poll cycle.
This is the correct backfill behaviour: it catches all existing VoDs (including the
S17 `pulse-test` fixture, D-079).

**Restart risk:**

Because HWM is in-memory only, a process restart resets it to 0 and triggers another
full backfill.  Each VoD would produce a second `EventRecordingReady` → duplicate row
in `server_events` → duplicate `recording_bytes` contribution if the MV path is used
(see §3.5).

Two mitigation options:
- **Mitigate via persistent HWM:** Store `vodHWM` in the meta store (SQLite) as a
  `vod_poll_hwm` key-value table.  This requires a new meta migration (see §4 on
  contract impact).
- **Mitigate via ClickHouse dedup table:** Replace the event-sourcing path with a
  separate `vod_snapshots` table using `ReplacingMergeTree(last_updated)` keyed on
  `(app, vod_id)`, populated directly by the VoD poll.  A new MV feeds
  `rollup_usage_1d.recording_bytes` from this table.  This requires a new ClickHouse
  migration (see §4).

**Recommended choice:** persistent HWM in the meta store.  It is the smallest
schema surface and the most operationally predictable.  The VoD poll emits events
into the existing `server_events` pipeline, maintaining the event-sourcing design.

**OPEN QUESTION OQ-1:** Does AMS 3.0.3 `vods/list` expose a reliable, stable
`vodId` field?  The HWM-by-creationDate approach can produce near-collisions if two
VoDs are created within the same millisecond.  If `vodId` is available and stable,
a seen-set approach (persisted in the meta store) is safer.

### 3.5 The missing materialized view — critical gap

The `mv_usage_1d` MV (`contracts/db/clickhouse/0001_init.sql:449-464`) hard-codes
`toUInt64(0) AS recording_bytes`.  There is no MV that reads
`server_events WHERE event_type='recording_ready'` and populates
`rollup_usage_1d.recording_bytes`.

Without a new MV, `EventRecordingReady` events would be stored in `server_events`
(auditable, searchable) but would never flow into the billing rollup.  The
`ComputeUsage` function (`server/internal/reports/accounting.go:199-223`) queries
only `rollup_usage_1d`, not `server_events` directly.

**Required new MV (ClickHouse migration — needs INT-01 CR):**

```sql
-- mv_recording_1d: populate rollup_usage_1d.recording_bytes from recording_ready events
-- Companion to mv_usage_1d. SummingMergeTree will merge viewer and recording rows.
CREATE MATERIALIZED VIEW IF NOT EXISTS {db}.mv_recording_1d
TO {db}.rollup_usage_1d AS
SELECT
    toDate(ts)          AS bucket,
    app,
    stream_id,
    node_id,
    tenant,             -- '' (recording events have no tenant attribution yet)
    geo_country,        -- '' (not applicable to VoDs)
    client_device,      -- '' (not applicable to VoDs)
    protocol,           -- '' (not applicable to VoDs)
    toFloat64(0)        AS viewer_minutes,
    toUInt32(0)         AS peak_concurrency,
    toUInt64(0)         AS egress_bytes,
    recording_size      AS recording_bytes
FROM {db}.server_events
WHERE event_type = 'recording_ready';
```

**Note on the "no schema change required" claim:**

`docs/assessment/final-assessment.md:321` characterises the fix effort as "Low —
incremental REST poll, no schema change required."  Code inspection reveals this is
overoptimistic: without the above MV, `recording_bytes` in `rollup_usage_1d` will
remain 0 even after the poller emits events, because no data path connects
`server_events.recording_size` to `rollup_usage_1d.recording_bytes`.  The MV above
is a narrow, additive migration — the lowest-cost schema change possible — but it
IS a contract change and therefore requires an ORCH-approved CR applied by INT-01
(D-004 channel).

---

## 4. Contract impact

### 4.1 ClickHouse schema (`contracts/db/clickhouse/`)

**Required:** New migration file (e.g. `0009_recording_mv.sql`) containing the
`mv_recording_1d` materialized view (see §3.5).  No changes to existing tables.
The `recording_bytes` column in `rollup_usage_1d` already exists
(`contracts/db/clickhouse/0001_init.sql:356`); no ALTER needed.

**ORCH approval required.**  INT-01 must author and apply the migration via the
D-004 CR channel.

### 4.2 Meta store schema (`contracts/db/meta/`)

**Required if persistent HWM is chosen (recommended):** New migration file (e.g.
`0003_vod_poll_state.sql`) adding a `vod_poll_state` table:

```sql
CREATE TABLE IF NOT EXISTS vod_poll_state (
    app      TEXT NOT NULL,
    hwm_ms   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (app)
);
```

**ORCH approval required.**  INT-01 applies.

### 4.3 OpenAPI spec (`contracts/openapi/pulse-api.yaml`)

**No change required.**  The `recording_gb` field is already present in
`UsageRow` and `UsageTotals` in the OpenAPI spec and in the Go struct
(`server/internal/reports/accounting.go:58,68`).  Once the MV is in place and
events flow, the field will self-populate with correct values.

### 4.4 Event schema (`contracts/events/ams-server-event.schema.json`)

**No change required.**  `domain.EventRecordingReady` is already a valid event
type; its `path` + `size_bytes` payload is already accepted by the ClickHouse
insert path (`clickhouse.go:519-522`).

### 4.5 Summary

| Artifact | Change? | CR needed? |
|---|---|---|
| ClickHouse migration (new `mv_recording_1d`) | YES | Yes — INT-01 CR via D-004 |
| Meta store migration (new `vod_poll_state`) | YES (if persistent HWM) | Yes — INT-01 CR via D-004 |
| OpenAPI spec | No | No |
| Event schema | No | No |
| Beacon schema | No | No |

**The Go server code** (`amsclient`, `restpoller`, `domain`) is inside the server
package, not a frozen contract.  It can be changed by BE-01/BE-02 under a normal
work order.

---

## 5. Alternatives considered

### 5.1 Unsigned-webhook ingest mode (D-V2-1)

**What it is:** Accept AMS lifecycle events from a configured CIDR allowlist without
HMAC validation (`agents/handoffs/ROADMAP-V2.md:133-146`).  This would allow
`vodReady` events to reach Pulse from the AMS IP without a signed delivery.

**Why not chosen here:** This is an open operator decision (D-V2-1, status OPEN as
of S19).  No code exists for it.  Implementing it requires its own work order,
operator sign-off, and a security documentation update.  More importantly, the
VoD REST poll proposed above (§3) is independent of D-V2-1: the poll works now,
with zero operator configuration change, against any AMS 3.0.3 deployment.  D-V2-1
and the VoD poll are complementary, not mutually exclusive; the poll provides
latency-tolerant accounting while an unsigned webhook (if ever implemented) would
provide near-real-time event delivery.  This design note does NOT depend on D-V2-1.

### 5.2 Filesystem / volume inspection of the AMS recording directory

**What it is:** Mount the AMS recording directory (e.g. `/usr/local/antmedia/webapps/`)
as a read-only volume into the Pulse container, and walk the filesystem to size VoD files.

**Why rejected:** (a) Requires operator changes to the deployment topology (bind
mount or volume share between AMS and Pulse containers) — a significant ops burden.
(b) Not applicable when AMS stores VoDs on object storage (S3, GCS, Azure Blob).
(c) This couples Pulse to AMS filesystem layout, which has changed between AMS
versions.  The REST API is the documented stable interface.  (d) Walking a
filesystem tree with potentially thousands of VoDs has O(n) I/O cost per scan.

### 5.3 Leave `recording_gb` as an honest null / absent field

**What it is:** Remove `recording_gb` from the usage report response (or return
`null`) when no VoD data is available, rather than returning 0.

**Why rejected:** (a) `recording_gb: 0` is an OpenAPI contract commitment that
downstream callers (including the web UI at
`web/src/features/reports/ReportsPage.tsx` and PDF report generation) may rely
on being a number.  A null would break existing callers without a contract version
bump.  (b) The 0 is itself an honest value right now — it is structurally 0, not
"unknown".  Removing it would make the field absence indistinguishable from an AMS
with genuinely zero recordings.  (c) The PRD F6 requirement explicitly includes
recording storage accounting; omitting the field would make F6 MISSING rather than
PARTIALLY.

**What would make sense instead:** Add an `egress_method`-style `recording_method`
field to the response documenting how recording data was (or was not) obtained.
This is a minor enhancement that could accompany the BUG-002 fix, not a reason to
omit the fix.

### 5.4 Direct INSERT into `rollup_usage_1d` from the VoD poll

**What it is:** Skip the `server_events` path entirely.  The poller computes the
total recording size per app per day and INSERTs directly into `rollup_usage_1d`
with `recording_bytes = sum(VoD sizes for that day)`.

**Why partially attractive:** No new ClickHouse MV is needed (the `recording_bytes`
column already exists).  The `docs/assessment/final-assessment.md:321` "no schema
change required" claim was likely describing this path.

**Why problematic:** (a) The `SummingMergeTree` sums rows with identical sort keys.
Two INSERT rounds with the same `(bucket, app, stream_id, ...)` values would double
the `recording_bytes` count.  Correct operation requires exactly-once insert per
VoD per day, enforced by the persistent HWM described in §3.4.  If the HWM is also
persisted (meta migration required), the meta migration cost is the same as the
`vod_poll_state` approach in §4.2.  (b) This path bypasses `server_events`,
losing the audit trail (there is no raw recording event row for drill-down or
replay).  (c) The `stream_id` dimension in `rollup_usage_1d` would have to be
set to the VoD file name (or empty), which is disjoint from the stream IDs in the
viewer-session rows — making the per-stream breakdown in usage reports misleading.

**Conclusion:** Direct INSERT is viable but less clean than the event-sourcing path
(§3).  It saves one ClickHouse migration at the cost of losing the raw event record
and complicating per-stream attribution.  Recommend the event-sourcing path (§3 +
§4.1) unless the operator prefers to avoid the ClickHouse migration entirely.

---

## 6. Test plan

### 6.1 Unit tests — fixture replay

Add a test in `server/pkg/amsclient/client_test.go` that:
- Serves a mock `/{app}/rest/v2/vods/list/0/200` returning a JSON fixture array
  with 2 VoD objects (with known `vodId`, `vodName`, `fileSize`, `creationDate`).
- Asserts that `client.ListVods(ctx, app, 0, 200)` returns the correct
  `[]VodDTO` slice, with all fields populated.

Add a test in `server/internal/collector/restpoller/restpoller_test.go` that:
- Configures a Poller against a mock AMS returning 1 app with 2 VoDs.
- Calls `pollVods` directly.
- Asserts that 2 `EventRecordingReady` events are written to the sink.
- Calls `pollVods` again (same VoDs, no new ones since HWM was advanced).
- Asserts 0 new events (deduplication working).
- Adds a 3rd VoD with a newer `creationDate`.
- Calls `pollVods` again.
- Asserts 1 new event (only the new VoD, not the two already-seen ones).

### 6.2 Integration tests

Add a test in `server/internal/store/clickhouse/` (analogous to `drain_test.go`)
that:
- Inserts two `EventRecordingReady` events (app="pulse-test", size_bytes=12345678)
  into `server_events` via `OnServerEvent`.
- Waits for the ClickHouse flush interval.
- Queries `rollup_usage_1d` (after the `mv_recording_1d` MV is applied) and
  asserts `sum(recording_bytes) = 12345678` (not doubled).

### 6.3 Live validation — scenario TC-REC-01

Add a new QA realams scenario `qa/realams/scenarios/TC-REC-01-vod-rest-poll.sh`:

```
Scenario ID: TC-REC-01
Name:        VoD REST poll populates recording_gb
Assertion:   POST-FIX — after BUG-002 fix is deployed:
             1. Confirm pulse-test app has >= 1 VoD via /{app}/rest/v2/vods/count.
             2. Wait <= 70 s (one VoD poll cycle + flush window).
             3. GET /api/v1/reports/usage.
             4. Assert totals.recording_gb > 0.
             5. Assert totals.recording_gb approximately matches AMS VoD sizes.
Ground truth: S17 VoD on pulse-test (D-079: mp4 muxing enabled → 20 s publish →
             mp4 muxing restored OFF; VoD left as standing fixture).
             AMS endpoint: GET /pulse-test/rest/v2/vods/count → {"number": 1}.
```

This scenario must SKIP (exit 77) rather than FAIL if the Pulse version under test
predates the fix (i.e. if the feature flag or module is absent).  It SHOULD be run
against the live AMS at `161.97.172.146:5080` to validate against the real pulse-test
VoD.

The S17 pulse-test VoD is the natural fixture: it was created during the BUG-002
evidence run and is specifically noted as "kept as standing ground truth" in D-079
(`agents/handoffs/decisions.md:3737-3738`).

---

## 7. Effort estimate, risk, and OPEN QUESTIONS

### 7.1 Effort estimate

| Component | Owner | Complexity | Estimated |
|---|---|---|---|
| `amsclient.VodDTO` + `ListVods` + `ListVodsPaged` | BE-01 | XS | 1–2 h |
| `restpoller.pollVods` + HWM dedup (in-memory) | BE-01 | S | 2–3 h |
| Meta store `vod_poll_state` migration + read/write | BE-02 | S | 2–3 h |
| ClickHouse `mv_recording_1d` migration | INT-01 | XS | 1 h |
| Unit tests (amsclient + restpoller) | BE-01 | S | 2–3 h |
| Integration test (ClickHouse MV) | BE-01 | S | 1–2 h |
| QA realams scenario TC-REC-01 | QA-01 | S | 1–2 h |
| **Total** | | | **~10–16 h** |

The "Low" complexity estimate in `docs/assessment/final-assessment.md:321` is
broadly correct for the Go implementation.  The main uncertainty is the AMS VoD
response shape (OQ-1 below).

### 7.2 Risk

| Risk | Likelihood | Mitigation |
|---|---|---|
| AMS `vods/list` response fields differ from assumptions in §3.1 | Medium | Probe the live AMS (`GET /pulse-test/rest/v2/vods/list/0/1`) before writing the DTO; the S17 pulse-test VoD is available as a known fixture |
| `creationDate` field absent or in a different format | Medium | Fall back to polling-order as HWM; or use `vodName` as dedup key if unique |
| SummingMergeTree double-count on Pulse restart (before persistent HWM lands) | Low-Medium | Ship persistent HWM in the same PR as the poller; do not ship the poller without it |
| `mv_recording_1d` creates retroactive double-count if VoD events already exist in server_events (from a prior webhook that was somehow delivered) | Very Low | AMS 3.0.3 never delivers signed webhooks; `server_events` has zero `recording_ready` rows on any live deployment. Risk is negligible. |
| AMS `vods/list` paginates differently than broadcasts (`list/{offset}/{size}`) | Low | TC-REC-01 exercises the full paginated fetch; unit test covers multi-page scenario |

### 7.3 OPEN QUESTIONS

**OQ-1 (must resolve before coding):** What are the actual field names and types in
the AMS 3.0.3 `/{app}/rest/v2/vods/list/{offset}/{size}` response?  Specifically:
the unique VoD identifier field name, the file size field name (bytes), and the VoD
creation timestamp field name and epoch unit.  Probe the live AMS at
`161.97.172.146:5080` against the pulse-test app.  Run:
`curl -s http://161.97.172.146:5080/pulse-test/rest/v2/vods/list/0/1 | jq .`
(no auth required for the default AMS open-read policy — ASSUMPTION-TO-VERIFY).

**OQ-2 (architecture preference):** Should `recording_gb` be attributed to the
originating live stream (if AMS provides the `streamName` in the VoD list), or to
the app as a whole (stream_id = '')?  Per-stream attribution makes the billing
breakdown more useful but depends on AMS providing `streamName` in the VoD response.

**OQ-3 (backfill window):** Should the VoD poll on first run backfill ALL VoDs
ever created in AMS, or only those created since Pulse first started?  Full
backfill is the honest accounting choice for F6 (billing), but it can inflate
`recording_gb` for a historical period that predates Pulse's deployment.  The
operator should decide the intended backfill scope.

**OQ-4 (AMS license expiry):** The AMS trial license expired 2026-07-12T12:09Z
(`docs/assessment/final-assessment.md:113-115`).  TC-REC-01 requires the live AMS
to respond to the `vods/list` endpoint.  Confirm with the operator that the license
has been renewed or replaced before scheduling a live validation run.

**OQ-5 (VoD size unit in `vods/list`):** The webhook payload uses `vodSize` as the
field name (`webhook.go:247: "size_bytes": jsonInt64(raw["vodSize"])`).  Whether the
REST list endpoint uses `vodSize` or `fileSize` (or another name) is unconfirmed.
This is a critical AMS API detail (a field name mismatch would result in all
`recording_bytes` being 0 even after the fix is deployed).

---

*This design note was authored at S20 open (2026-07-12) and is a proposal only.
It must be reviewed by ORCH-00 before a work order is issued.  No Go, schema, or
contract files have been created or modified by this note.*
