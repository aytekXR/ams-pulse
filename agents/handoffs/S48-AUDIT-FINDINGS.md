# S48 subsystem audit — confirmed findings ledger (D-109 close, 2026-07-16)

> Produced by the SESSION-48 adversarial audit workflow (7 finders + refute-by-default
> verifiers, 27 agents). **16 CONFIRMED, 4 refuted.** These are agent findings — **each MUST be
> re-verified against the code before building** (S38/S43/S46/S47 lesson). Work through them in
> coherent clusters, one scope per PR. Mark each ✅ DONE (D-NNN) as it ships.

## [1] HIGH — Deduplicator key missing App field — cross-app same-StreamID publish_start silently dropped
- **loc:** `server/internal/collector/dedup.go:18`  ·  **lens:** poller-sessions-agg  ·  status: ⏳ TODO
- **behavior:** dedupKey uses {eventType, nodeID, streamID, window} but omits App. When two AMS applications on the same node both have a stream with the same bare StreamID (e.g. LiveApp/test123 and PetarTest2/test123) going live within the same dedup window (default 10 s = 2×PollInterval), IsDuplicate() returns true for the second publish_start, which is silently dropped and never reaches ClickHouse. The TestRestPoller_MultiApp_NoFalseEnd test exercises exactly this scenario (both apps broadcasting in the same poll cycle) but only asserts the absence of a false publish_end and the presence of app-A's publish_start; it does not assert that app-B's publish_start was received, so the drop is undetected.
- **scenario:** AMS node serves LiveApp/test123 and PetarTest2/test123, both first seen as broadcasting in the same poll cycle. pollApp(LiveApp) emits publish_start {nodeID=n, streamID=test123}; dedupKey={publish_start,n,test123,T/10000}; IsDuplicate→false, passes through. pollApp(PetarTest2) emits publish_start {nodeID=n, streamID=test123}; same dedupKey; IsDuplicate→true; event DROPPED. PetarTest2/test123 has no stream_publish_start row in ClickHouse; stream visibility and billing queries are wrong.
- **fix:** Add `app string` to `dedupKey` and populate it in `IsDuplicate`:

In `server/internal/collector/dedup.go`, change the struct at line 18:

```go
type dedupKey struct {
    eventType string
    nodeID    string
    app       string   // add this field
    streamID  string
    window    int64
}
```

And update the key construction at line 62 inside `IsDuplicate`:

```go
key := dedupKey{
    eventType: e.Type,
    nodeID:    e.NodeID,
    app:       e.App,   // add this line
    streamID:  e.StreamID,
    window:    e.TS / d.windowMs,
}
```

Then add a complementary assertion to `TestRestPoller_MultiApp_NoFalseEnd` (Phase 3 or a new Phase 4) that checks app-B's `publish_start` was also received, so the test detects a regression if `App` is dropped from the key again.
- **mutation:** Yes. A mutation that removes `app` from `dedupKey` (reverting the fix) would cause a new test assertion — that `sink.events` contains a `publish_start` with `App == appB` and `StreamID == sharedStreamID` — to fail deterministically, because the second `publish_start` would collide with the first and be dropped. The existing dedup unit test file (`server/internal/collector/dedup_test.go`, if present) can also host a direct unit test: feed two `ServerEvent` values with the same `{eventType, nodeID, streamID, window}` but different `App` values, and assert both return `IsDuplicate == false`. Mutating `app` out of the key makes the second call return `true`, killing the mutant.

## [2] HIGH — snapRemoveStream deletes surviving stream from snapshot.Streams when two apps share a bare StreamID
- **loc:** `server/internal/collector/aggregator/aggregator.go:562`  ·  **lens:** poller-sessions-agg  ·  status: ⏳ TODO
- **behavior:** snapshot.Streams is keyed by bare StreamID (not the compound nodeID/app/streamID key used by a.streams). When two apps both have an active stream with the same StreamID, snapAddStream overwrites snapshot.Streams[streamID] with the last-written pointer (last-write-wins). When one stream ends, snapRemoveStream calls delete(snapshot.Streams, s.StreamID), which removes the entry — but that entry now points to the OTHER app's still-active stream. Result: snapshot.ActiveStreams=1 (correct) but snapshot.Streams is empty (wrong). The inconsistency persists until the surviving stream receives its next stats event, which triggers snapRemoveStream+snapAddStream and re-inserts it. At a 5 s poll interval the window is up to ~5 s. The existing TestAggregator_CrossAppStreamID_NoCollision only tests a publish_end for an app that never had a publish_start, so a.streams never contains both; the two-active-streams path is untested.
- **scenario:** Both LiveApp/test123 and PetarTest2/test123 publish_start, then test123 in LiveApp ends. After onPublishEnd for LiveApp: ActiveStreams=1, TotalViewers=PetarTest2's count (correct), but snapshot.Streams={} (empty). Any CurrentSnapshot() call until PetarTest2's next stats event returns a snapshot where the live stream is invisible to the dashboard — 1 active stream in the counter but 0 in the per-stream detail map.
- **fix:** In snapRemoveStream (aggregator.go line 562), guard the delete with a pointer-equality check so only the owning stream removes its entry:

    if a.snapshot.Streams[s.StreamID] == s {
        delete(a.snapshot.Streams, s.StreamID)
    }

This is safe because Go map equality on pointers is reference equality. When two apps share a bare StreamID, snapshot.Streams holds the last-written pointer (s2 from PetarTest2). When LiveApp's stream (s1) ends, s1 != s2, so the delete is skipped and PetarTest2's entry survives. When PetarTest2 itself ends, snapshot.Streams["test123"] == s2, so the delete proceeds normally. A corresponding test should send publish_start for both apps (so a.streams contains both keys), then publish_end for one, and assert that snapshot.Streams still contains the surviving app's stream.
- **mutation:** Yes. A mutation that removes the pointer-equality guard (reverting to the bare delete) would leave the existing TestAggregator_CrossAppStreamID_NoCollision passing (it never puts both streams in a.streams simultaneously), but would cause a new test covering the two-active-streams path to fail: after the first app's publish_end, snapshot.Streams would be empty while snapshot.ActiveStreams = 1. This makes the fix non-vacuous and the mutation detectable.

## [3] HIGH — streamID not URL-path-escaped in WebRTCClientStats — wrong AMS endpoint called silently
- **loc:** `server/pkg/amsclient/client.go:475`  ·  **lens:** amsclient  ·  status: ⏳ TODO
- **behavior:** Path is built with fmt.Sprintf("/%s/rest/v2/broadcasts/%s/webrtc-client-stats/0/100", app, streamID) and then passed to http.NewRequestWithContext via plain string concatenation with baseURL. No url.PathEscape is applied to streamID. AMS stream names are publisher-chosen and can contain any character the publisher sets via RTMP or WebRTC publish; the AMS wire returns them verbatim in the streamId JSON field.
- **scenario:** Stream published with name "test#peer" → ListBroadcasts returns BroadcastDTO{StreamID:"test#peer"} → pollApp at restpoller.go:420 calls WebRTCClientStats(ctx, app, "test#peer") → fmt.Sprintf produces path "/LiveApp/rest/v2/broadcasts/test#peer/webrtc-client-stats/0/100" → url.Parse (invoked inside http.NewRequestWithContext) silently splits on '#', yielding Path="/LiveApp/rest/v2/broadcasts/test" and Fragment="peer/webrtc-client-stats/0/100" → GET goes to the single-broadcast-detail AMS endpoint, not the WebRTC-stats endpoint. If AMS returns null (broadcast "test" not found), json.Decode(null, &[]WebRTCClientStatsDTO{}) returns (nil-slice, nil-error) — confirmed by running url.Parse and json.Unmarshal("null") above. restpoller.go:420 gates on err==nil: no error, no stats, no log — viewer-side QoE data is silently dropped for every broadcasting stream whose name contains '#'. The '?' case is identical: url.Parse treats everything after '?' as query string, path becomes the bare broadcast endpoint, AMS returns a BroadcastDTO JSON object, decoder fails with a type-mismatch error — that error is also silently swallowed by the err==nil gate in restpoller.
- **fix:** Apply url.PathEscape to both streamID and app inside WebRTCClientStats (client.go:475) and to app in all other fmt.Sprintf path-builders (lines 436, 455, 576, 589):

  path := fmt.Sprintf("/%s/rest/v2/broadcasts/%s/webrtc-client-stats/0/100",
      url.PathEscape(app), url.PathEscape(streamID))

url is already imported. The same one-line change applies to ListBroadcasts, ListBroadcastsPaged, ListVods, and ListVodsPaged — replace bare app/streamID with url.PathEscape(app)/url.PathEscape(streamID) in every fmt.Sprintf that builds an AMS path.
- **mutation:** Yes. Add a TestWebRTCClientStats_UsesPathParams test that captures r.URL.Path in the httptest server handler and asserts it equals "/LiveApp/rest/v2/broadcasts/test%23peer/webrtc-client-stats/0/100" when called with streamID "test#peer". Removing url.PathEscape causes the server to receive "/LiveApp/rest/v2/broadcasts/test", failing the assertion. This is a direct, non-vacuous mutation following the existing TestListBroadcasts_UsesPerAppPathParams pattern in client_test.go.

## [4] HIGH — Off-by-one in scheduled report period: first day of current month included in previous-month report
- **loc:** `server/internal/reports/scheduler.go:169`  ·  **lens:** reports  ·  status: ⏳ TODO
- **behavior:** to is set to time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC) — the first day of the CURRENT month (e.g., 2026-07-01). The ClickHouse queries use an inclusive bound: WHERE bucket >= ? AND bucket <= ?, so bucket=2026-07-01 rows satisfy the condition and are returned. The same to value flows into StatementOptions and appears in the output filename and header as the period end date.
- **scenario:** Scheduler fires on 2026-07-15 to generate the June monthly report. from=2026-06-01, to=2026-07-01. The rollup_usage_1d query (bucket Date column) returns all rows where bucket >= 2026-06-01 AND bucket <= 2026-07-01, which includes July 1 data. Any streams with activity on July 1 are added to the June billing statement, overstating viewer-minutes, egress, and peak concurrency for the reporting period.
- **fix:** In server/internal/reports/scheduler.go, change the time-range calculation from:

    now := time.Now().UTC()
    to := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
    from := to.AddDate(0, -1, 0)

to:

    now := time.Now().UTC()
    firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
    to   := firstOfThisMonth.AddDate(0, 0, -1) // last day of previous month (e.g. 2026-06-30)
    from := firstOfThisMonth.AddDate(0, -1, 0)  // first day of previous month (e.g. 2026-06-01)

This makes the period [2026-06-01, 2026-06-30] so `bucket <= Date('2026-06-30')` no longer matches July 1 rows. It also fixes the StatementOptions.To field (used in the filename and report header), which currently shows '2026-07-01' as the end-of-period date for the June statement. The same `to` value flows into fetchConcurrencyPeaks and Reconcile via their callers, so no changes are needed in those functions once the scheduler passes the corrected bound.
- **mutation:** Yes. A table-driven unit test can capture the SQL arguments passed to the fake conn (add a field to acctFakeConn that records the args slice for each Query call), then assert that when runSchedule is invoked with now = 2026-07-15, the upper bound argument is Date 2026-06-30 and not 2026-07-01. If the fix is reverted the assertion fails immediately, making the test non-vacuous. No real ClickHouse connection is needed.

## [5] HIGH — IsEdgeStream ignores node Status — downed edge node permanently suppresses origin viewer counts
- **loc:** `server/internal/cluster/discovery.go:264`  ·  **lens:** cluster  ·  status: ⏳ TODO
- **behavior:** IsEdgeStream iterates d.nodes and returns true for any node where Role=="edge" && ActiveStreams>0, with no check on Status. When an edge node disappears from the AMS API, poll() marks it Status="down" (line 209) but never clears ActiveStreams. The stale non-zero ActiveStreams value survives indefinitely in the map.
- **scenario:** Cluster starts with origin + edge node (edge-1, ActiveStreams=5). Edge node crashes. After 3×PollInterval (StaleTimeout) edge-1 is marked Status="down" with ActiveStreams=5 still set. IsEdgeStream("any-stream") still returns true. The aggregator at aggregator.go:344 sees IsEdgeStream=true + NodeRole=="origin" → sets skipViewerCount=true for every subsequent origin node_stats event → s.ViewerCount is never updated → dashboard permanently reports 0 viewers (or a frozen stale count) for all streams even though origin is the only active node serving traffic.
- **fix:** In `server/internal/cluster/discovery.go`, add a `Status != "down"` guard to the loop inside `IsEdgeStream` (line 264):

```go
func (d *Discovery) IsEdgeStream(streamID string) bool {
    d.mu.RLock()
    defer d.mu.RUnlock()
    for _, n := range d.nodes {
        if n.Role == "edge" && n.Status != "down" && n.ActiveStreams > 0 {
            return true
        }
    }
    return false
}
```

This is a one-character-class change: a downed edge node is excluded from the edge-serving signal. The alternative — resetting `ActiveStreams = 0` inside the stale-mark block at poll() line 209 — also works but places the fix far from the predicate that relies on the invariant, making future readers less likely to notice the coupling. The `IsEdgeStream` fix is self-documenting at the point of use.
- **mutation:** Yes. Add a test that (1) sets up a Discovery with an origin node and an edge node with `ActiveStreamCount=5`, (2) lets the first poll run so `IsEdgeStream` returns `true`, (3) removes the edge node from the mock client and waits for `StaleTimeout` to elapse, then (4) asserts `IsEdgeStream` returns `false`. A mutation that deletes `n.Status != "down"` from the loop condition causes the post-crash assertion to fail because the downed edge node still has `ActiveStreams=5`. The test is non-vacuous: without the fix the assertion at step 4 fails; with it, it passes. The `mockClusterClient.setNodes` helper already exists in `discovery_test.go` and can be reused directly.

## [6] HIGH — AudienceAnalytics silently drops p.Tenant filter — all tenants' data returned
- **loc:** `server/internal/query/query.go:263`  ·  **lens:** clickhouse  ·  status: ✅ DONE (D-110, PR #93, prod v0.4.0-37-g5e822e7)
- **behavior:** AudienceAnalytics builds its WHERE clause applying p.App and p.Stream but never p.Tenant. The rollup tables (rollup_audience_1h / rollup_audience_1d) include tenant in their ORDER BY key and the MV populates the tenant column, so tenant-segregated rows exist in storage but are never filtered at query time.
- **scenario:** Multi-tenant AMS deployment (multiple customers sharing one Pulse instance). Caller invokes AudienceAnalytics with AudienceParams{Tenant: "customer-A", From: ..., To: ...}. The WHERE clause has no tenant predicate, so the query returns countMerge/uniqMerge totals across ALL tenants. Customer A sees combined audience data for all other tenants.
- **fix:** Add the tenant guard immediately after the p.Stream block in AudienceAnalytics, matching the pattern used in the sibling functions:

In server/internal/query/query.go, after line 263 (the closing brace of the p.Stream block), insert:

    if p.Tenant != "" {
        where += " AND tenant = ?"
        args = append(args, p.Tenant)
    }

This is lines 260-263 context for the edit anchor:
    if p.Stream != "" {
        where += " AND stream_id = ?"
        args = append(args, p.Stream)
    }
    // ADD HERE:
    if p.Tenant != "" {
        where += " AND tenant = ?"
        args = append(args, p.Tenant)
    }
- **mutation:** Yes. Seed two tenants ("A" and "B") with distinct view counts into rollup_audience_1h in a test ClickHouse instance. Call AudienceAnalytics with AudienceParams{Tenant: "A", ...} and assert Totals.Views equals only tenant-A's count. A mutation that removes the tenant predicate (or the entire if-block) will cause the assertion to fail because the returned total will include both tenants' data — proving the guard is non-vacuous.

## [7] MEDIUM — time.IsZero() guard silently fails for ev.TS == 0
- **loc:** `server/internal/collector/ingest/health.go:172`  ·  **lens:** beacon-ingest  ·  status: ⏳ TODO
- **behavior:** time.UnixMilli(0).UTC().IsZero() returns false because time.UnixMilli(0) is 1970-01-01 00:00:00 UTC, not the Go zero time (January 1, year 1). When ev.TS == 0, the fallback to time.Now() is never taken, and pub.LastSeen is written as 1970-01-01 UTC.
- **scenario:** Any call to OnServerEvent with TS=0 (a domain.ServerEvent zero-value, a future collector path that omits TS, or a direct test call) causes pub.LastSeen = 1970-01-01. The next SweepStale call (which runs every 5 s in production) sees time.Now().Sub(1970) ≈ 56 years >> sourceGoneTimeout (15 s), evicts the publisher, logs a false 'ingest: source gone' warning, and removes the stream from all tracking. The false eviction also hides legitimate upstream health state from the API.
- **fix:** Replace line 172 in `/home/aytek/repo/ams-pulse/server/internal/collector/ingest/health.go`:

  Old: `if now.IsZero() {`
  New: `if ev.TS <= 0 {`

This correctly guards against the zero value of the `int64` field (and also against any future negative sentinel that might be passed), and removes the dead `time.IsZero()` branch that can never fire via `time.UnixMilli`.
- **mutation:** Yes. A mutation test is straightforward and non-vacuous: call `OnServerEvent` (or the internal `onIngestStats`) with a `domain.ServerEvent{TS: 0, NodeID: "n", App: "a", StreamID: "s", Type: domain.EventIngestStats}`, then immediately call `SweepStale` with `sourceGoneTimeout` set to `time.Second`. On the original code the publisher is evicted (SweepStale returns 1); with the fix `pub.LastSeen` is near `time.Now()` and the publisher survives (SweepStale returns 0). The two outcomes are distinct and verifiable, proving the fix is non-vacuous.

## [8] MEDIUM — No replay protection — any captured valid signed webhook can be replayed indefinitely
- **loc:** `server/internal/collector/webhook/webhook.go:160`  ·  **lens:** webhook-hmac  ·  status: ⏳ TODO
- **behavior:** validateHMAC (called at line 160) is the sole authentication gate. It proves that the request body was signed with the correct HMAC key at some point, but performs no freshness check: no timestamp window, no nonce, no tracking of previously-seen HMAC values. Every request that carries a valid signature is accepted regardless of how long ago it was issued or whether an identical request has already been processed.
- **scenario:** An observer on the internal HTTP path between the signing proxy and Pulse (or any party who obtains a copy of a prior webhook delivery) captures a valid POST to /webhook/ams or /webhook/ams/{name} with its X-Ams-Signature header intact. Replaying the request — hours or days later, arbitrarily many times — passes validateHMAC and causes sink.WriteServerEvent to be called, injecting spurious stream-start, stream-end, or recording-ready events into the analytics pipeline. Concrete inputs: captured body {"action":"liveStreamStarted","streamId":"s1","app":"live"} with matching sha256=<valid-hmac> header; replayed POST returns HTTP 200 and produces a duplicate event in the sink on every replay.
- **fix:** Add a timestamp-window check alongside the HMAC check. Standard approach (used by GitHub, Stripe, Svix):

1. Require a second header, e.g. X-Ams-Timestamp, containing the Unix epoch second at which the sender signed the request.
2. In handleWebhookWithSecret, before or after validateHMAC, read that header, parse it as int64, and reject (401) any request whose timestamp is outside ±5 minutes of time.Now().
3. Include the timestamp in the HMAC input (e.g. sign "timestamp.body" or a canonical concatenation) so an attacker cannot forge a fresh timestamp for a captured body without knowing the secret.

Minimal concrete change in webhook.go:

In handleWebhookWithSecret, after reading the body and before passing to validateHMAC:
  tsHeader := r.Header.Get("X-Ams-Timestamp")
  ts, err := strconv.ParseInt(tsHeader, 10, 64)
  if err != nil || abs(time.Now().Unix()-ts) > 300 {
      http.Error(w, "timestamp missing or stale", http.StatusUnauthorized)
      return
  }

Update validateHMAC to sign the concatenation of timestamp and body (matching whatever AMS/signing-proxy convention is chosen), so the timestamp cannot be stripped and replaced by an attacker.

No persistent state is required; the 5-minute window alone eliminates all replays older than the window and, combined with idempotency logic in the sink (stream-start dedup by streamId), reduces the practical replay surface to the narrow window.
- **mutation:** Yes. A mutation that deletes or skips the timestamp-window check (e.g. removes the abs(time.Now().Unix()-ts) > 300 guard) would cause the following test to fail: construct a validly-signed request with a timestamp 10 minutes in the past, replay it, and assert HTTP 401. Without the fix the mutant returns 200; with the fix it returns 401. This makes the test non-vacuous and directly proves the fix closes the gap.

## [9] MEDIUM — detectEnded leaks p.prevStatus entries for non-broadcasting streams that disappear from AMS
- **loc:** `server/internal/collector/restpoller/restpoller.go:455`  ·  **lens:** poller-sessions-agg  ·  status: ⏳ TODO
- **behavior:** pollApp records every broadcast's status in p.prevStatus regardless of its status value (idle, created, broadcasting, etc.). detectEnded is the only path that deletes entries from p.prevStatus, and it only adds entries to the deletion set when status == "broadcasting". When a non-broadcasting stream (status "idle" or "created") disappears from the AMS broadcast response (e.g. deleted via the management UI), its key is never removed from p.prevStatus. The map grows without bound over the lifetime of the process.
- **scenario:** An operator registers 500 IP-camera streams as "idle" inputs in AMS, then later deletes them via the management API. After each poll cycle where they are present, their keys are recorded in p.prevStatus. Once deleted from AMS they stop appearing in ListBroadcastsPaged, but detectEnded's 'status == broadcasting' guard prevents their removal. After 1 hour of 5-second polling with constant turnover of idle streams, p.prevStatus grows proportionally to the total number of ever-seen non-broadcasting streams with no upper bound.
- **fix:** Decouple map eviction from event emission in detectEnded. Collect all disappeared entries (prefix-matching, absent from currentIDs) into a stale slice for deletion; keep the status == "broadcasting" guard only for the ended slice that drives event emission:

  p.mu.Lock()
  var ended []string
  var stale []string
  for key, status := range p.prevStatus {
      if !strings.HasPrefix(key, prefix) || currentIDs[key] {
          continue
      }
      stale = append(stale, key)      // ALL disappeared keys → evict
      if status == "broadcasting" {
          ended = append(ended, key)  // only broadcasting → emit event
      }
  }
  for _, key := range stale {
      delete(p.prevStatus, key)
  }
  p.mu.Unlock()

File: server/internal/collector/restpoller/restpoller.go, replacing the existing p.mu.Lock() … p.mu.Unlock() block inside detectEnded (lines 451-461).
- **mutation:** Yes. Seed p.prevStatus with one "idle" entry under the tested app prefix. Call detectEnded with an empty current slice. Assert (a) the key is no longer in p.prevStatus and (b) no publish_end event was emitted. This test fails against the original code (the idle entry survives) and passes with the fix. Mutating the fix back to gate deletion on status == "broadcasting" makes the test fail again, proving the fix is non-vacuous.

## [10] MEDIUM — UsageReport.EgressMethod hardcoded to bitrate_x_watch_time even when ams_rest_stats_byte_counter is used
- **loc:** `server/internal/reports/accounting.go:350`  ·  **lens:** reports  ·  status: ⏳ TODO
- **behavior:** ComputeUsage returns UsageReport with EgressMethod: EgressMethodBitrateXWatchTime unconditionally. Per-row EgressMethod is correctly set to EgressMethodAMSRestStatsByteCounter when !isHour && v.egressBytes > 0 (line 302), but the report-level field is never updated. The CSV and PDF statement generators both read report.EgressMethod for the disclosure header ('Egress method: …'), so they always emit 'bitrate_x_watch_time' regardless of which method was actually used.
- **scenario:** rollup_usage_1d.egress_bytes is non-zero for any stream (populated by AMS REST /getStats byte counters). ComputeUsage takes the egressBytes > 0 branch (line 299–302), sets per-row EgressMethod = 'ams_rest_stats_byte_counter', but the returned UsageReport.EgressMethod = 'bitrate_x_watch_time'. The generated CSV/PDF header prints '# Egress method: bitrate_x_watch_time', falsely claiming the bitrate model was used when byte-counter data drove the actual egress figures. This violates the PRD F6 egress methodology disclosure requirement.
- **fix:** In accounting.go, introduce a `reportEgressMethod` variable initialized to `EgressMethodBitrateXWatchTime` before the loop, set it to `EgressMethodAMSRestStatsByteCounter` on the first iteration that takes the bytes branch (alongside the existing per-row assignment), and use it instead of the hardcoded constant at line 350:

Before the loop (~line 283): `reportEgressMethod := EgressMethodBitrateXWatchTime`

Inside the bytes branch (~line 302), add: `reportEgressMethod = EgressMethodAMSRestStatsByteCounter`

At line 350, change: `EgressMethod: EgressMethodBitrateXWatchTime` → `EgressMethod: reportEgressMethod`
- **mutation:** Yes. A table-driven test can construct a UsageReport input with at least one row having egressBytes > 0 (!isHour path), call ComputeUsage (or the synthetic helper extended to support bytes), and assert report.EgressMethod == "ams_rest_stats_byte_counter". Reverting line 350 to the hardcoded constant makes that assertion fail, demonstrating the fix is non-vacuous. statement.go's generateCSV/generatePDF can also be exercised: pass such a report to GenerateStatement and assert the output bytes contain "Egress method: ams_rest_stats_byte_counter".

## [11] MEDIUM — Wrong column names in AnomalyBaselineForMetric viewer_count case silently return zeros
- **loc:** `server/internal/query/query.go:1084`  ·  **lens:** clickhouse  ·  status: ⏳ TODO
- **behavior:** The viewer_count case queries `SELECT avg(viewers) ... FROM server_events WHERE event_time >= ...`. Neither column exists: the table column is viewer_count (not viewers) and ts (not event_time), per the DDL at 0001_init.sql lines 47 and 96. ClickHouse returns an 'Unknown identifier' error; clickhouse-go v2 surfaces it through row.Scan, which is caught and causes the function to return (0, 0, 0, nil) — silently zeroing baseline statistics.
- **scenario:** Call AnomalyBaselineForMetric(ctx, "viewer_count", "", 3600) against a live ClickHouse with data in server_events. Expected: (mean=N, stddev=M, n>0, nil). Actual: (0, 0, 0, nil). Any anomaly detector wired to this function would treat every window as having zero data, preventing baseline-driven alerting from functioning.
- **fix:** In server/internal/query/query.go at line 1084, replace `avg(viewers)` with `avg(viewer_count)`, `stddevPop(viewers)` with `stddevPop(viewer_count)`, and `event_time` with `ts`: `SELECT avg(viewer_count) AS mean, stddevPop(viewer_count) AS stddev, count() AS n FROM server_events WHERE ts >= now() - INTERVAL ? SECOND`
- **mutation:** Yes. A mutation test can change `viewer_count` back to `viewers` in the query string and verify that an integration test against a real ClickHouse instance with data in `server_events` returns (0, 0, 0, nil) instead of real statistics — proving the fix is non-vacuous. The existing unit tests cannot catch this because they use a fake conn configured to return fixed values regardless of the SQL text.

## [12] MEDIUM — peak_concurrency excluded from SummingMergeTree column list — underreported after background merges
- **loc:** `contracts/db/clickhouse/0001_init.sql:358`  ·  **lens:** clickhouse  ·  status: ⏳ TODO
- **behavior:** rollup_usage_1d is defined as SummingMergeTree((viewer_minutes, egress_bytes, recording_bytes)). The column peak_concurrency is numeric (UInt32) but absent from the explicit sum-columns list. SummingMergeTree documentation states: for numeric columns NOT in the explicit list, the engine keeps the value from one of the merged rows (first encountered) rather than summing them. The mv_usage_1d MV inserts toUInt32(1) AS peak_concurrency per session with a comment 'summed per key', confirming the intent is summation.
- **scenario:** Insert 200 viewer sessions into viewer_sessions in a single hour bucket. The MV fires, writing 200 rows each with peak_concurrency=1 into rollup_usage_1d. ClickHouse performs a background merge of those 200 parts. After the merge, SELECT sum(peak_concurrency) FROM rollup_usage_1d returns 1 (one surviving row with value 1) instead of 200. Billing reports show near-zero peak_concurrency regardless of actual session volume.
- **fix:** In contracts/db/clickhouse/0001_init.sql line 358, add peak_concurrency to the SummingMergeTree column list:

Before:
ENGINE = SummingMergeTree((viewer_minutes, egress_bytes, recording_bytes))

After:
ENGINE = SummingMergeTree((viewer_minutes, peak_concurrency, egress_bytes, recording_bytes))

For the already-deployed table, apply via a migration: ALTER TABLE {db}.rollup_usage_1d MODIFY ENGINE = SummingMergeTree((viewer_minutes, peak_concurrency, egress_bytes, recording_bytes)) (supported since ClickHouse 22.6). If on an older version, a drop/recreate with data backfill is required.
- **mutation:** Yes. A mutation test can prove the fix non-vacuously: (1) insert N rows (e.g. 200) with peak_concurrency=1 into rollup_usage_1d for the same ORDER BY key, (2) run OPTIMIZE TABLE rollup_usage_1d FINAL to force a merge, (3) assert sum(peak_concurrency) = N. Before the fix this assertion fails (result is 1); after the fix it passes (result is N). The test distinguishes broken from fixed behavior with no ambiguity.

## [13] MEDIUM — insertBeaconEvents calls PrepareBatch per item — partial commit on error with misleading metrics
- **loc:** `server/internal/store/clickhouse/clickhouse.go:550`  ·  **lens:** clickhouse  ·  status: ⏳ TODO
- **behavior:** insertBeaconEvents opens a new PrepareBatch + Append + Send cycle for every BeaconItem inside the double loop (outer: []BeaconEvent, inner: []BeaconItem). If b.Send() fails at item M, items 0..M-1 have already been committed to ClickHouse, but the error is returned to the flusher goroutine. The flusher logs the error for the entire batch and does not increment s.inserted — making monitoring report a full-batch drop even though up to M-1 items were actually stored. insertServerEvents (line 460) correctly issues one PrepareBatch for the entire []ServerEvent slice.
- **scenario:** A beacon flush contains 500 BeaconItems across 50 BeaconEvents. A transient network error causes b.Send() to fail on item 300. Items 0–299 are in ClickHouse. The flusher logs 'insert beacon_events failed, count=50' and s.dropped is NOT incremented (events were never re-queued), s.inserted is NOT incremented. Metrics().inserted under-counts by 299; Metrics().dropped is also wrong. The 200 remaining items are silently lost with no retry.
- **fix:** Hoist PrepareBatch out of the double loop and call b.Send() once after all items are appended, matching the insertServerEvents / insertViewerSessions pattern. Concrete change to server/internal/store/clickhouse/clickhouse.go:

Replace the body of insertBeaconEvents so that:
1. `b, err := s.conn.PrepareBatch(...)` is called once before `for _, ev := range batch`.
2. All Append calls remain inside the double loop.
3. `b.Send()` is called once after both loops complete.

This makes the entire batch atomic: if Send() fails, nothing is committed and the flusher's error path (which correctly skips s.inserted and does not re-queue) is consistent with reality — zero items stored, zero items credited.
- **mutation:** Yes. A unit test can inject a mock conn that returns an error on the Nth Send() call (using a counter in mockConn). Before the fix: assert that beaconRows == N-1 after the error (partial commit). After the fix: assert that beaconRows == 0 (no commit on error). Reverting the fix (mutating back to per-item PrepareBatch) causes the after-fix assertion to fail because N-1 rows will have been committed. This proves the fix is non-vacuous. The existing mockConn/mockBatch infrastructure in drain_test.go already provides the scaffolding needed; adding a sendFailOnCall int field and checking it inside mockBatch.Send() is sufficient.

## [14] LOW — 413 detection uses byte-count heuristic instead of *http.MaxBytesError
- **loc:** `server/internal/collector/beacon/beacon.go:352`  ·  **lens:** beacon-ingest  ·  status: ⏳ TODO
- **behavior:** After io.ReadAll returns (body, err) with err != nil, the code branches on `len(body) >= maxBodyBytes-1` (i.e., len(body) >= 65535) to decide between 413 and 400. This heuristic is incorrect: it returns 413 any time the accumulated byte count is at or above the threshold, regardless of the actual error type.
- **scenario:** A client sends a body of exactly maxBodyBytes-1 = 65535 bytes. The underlying TCP connection resets (ECONNRESET, load-balancer timeout, TLS alert) after all 65535 bytes are buffered by io.ReadAll but before EOF is signalled. io.ReadAll returns ([]byte{65535 bytes}, syscall.ECONNRESET). The check `len(body) = 65535 >= 65535` is true, so the handler returns 413 REQUEST_TOO_LARGE when it should return 400 READ_ERROR. The body was within the 64 KB limit. Fix: use `var maxErr *http.MaxBytesError; if errors.As(err, &maxErr)` to detect the limit breach by error type, not byte count.
- **fix:** Add "errors" to the import block. Replace lines 351-358 with: if err != nil { var maxErr *http.MaxBytesError; if errors.As(err, &maxErr) { writeBeaconError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", fmt.Sprintf("body exceeds %d KB limit", maxBodyBytes/1024)); return }; writeBeaconError(w, http.StatusBadRequest, "READ_ERROR", "failed to read request body"); return }. Also remove the now-unreachable post-read size check at lines 360-364, which can never trigger because http.MaxBytesReader returns *http.MaxBytesError during io.ReadAll before it completes.
- **mutation:** Yes. Inject a custom io.Reader that returns exactly 65535 bytes then syscall.ECONNRESET as the request body. Assert the response status is 400 (READ_ERROR), not 413. Before the fix this assertion fails; after the fix it passes. This directly proves the corrected branch is exercised and non-vacuous.

## [15] LOW — nextCronTime called with time.Now() (local timezone) while the rest of runSchedule uses time.Now().UTC()
- **loc:** `server/internal/reports/scheduler.go:233`  ·  **lens:** reports  ·  status: ⏳ TODO
- **behavior:** nextRun := nextCronTime(sched.Cron, time.Now()) passes a time.Time in the system's local Location. nextCronTime evaluates t.Hour() and t.Minute() (and t.Day() for DOM matching) using that Location. One line earlier (line 168) now := time.Now().UTC() and the surrounding period math all use UTC explicitly. The next_run_at timestamp is stored as UnixMilli (timezone-agnostic), but its calculation is in local time.
- **scenario:** Server deployed with TZ=America/New_York (UTC-5). Operator creates schedule '0 6 1 * *' intending a 06:00 UTC run. nextCronTime interprets hour=6 against local time, resolves to 06:00 EST = 11:00 UTC, and stores that as next_run_at. Every subsequent next-run is also calculated in local time. The schedule fires 5 hours late relative to UTC expectations, and the off-by-TZ error accumulates with each recalculation.
- **fix:** Three callsites need changing:

1. `server/internal/reports/scheduler.go:233`
   Before: `nextRun := nextCronTime(sched.Cron, time.Now())`
   After:  `nextRun := nextCronTime(sched.Cron, time.Now().UTC())`

2. `server/internal/api/reports_wave2.go:130`
   Before: `now := time.Now()`
   After:  `now := time.Now().UTC()`
   (the call on line 131 then inherits UTC — no separate change needed there)

3. `server/internal/api/reports_wave2.go:183`
   Before: `nextRun := reports.NextCronTime(row.Cron, time.Now())`
   After:  `nextRun := reports.NextCronTime(row.Cron, time.Now().UTC())`

No change to `nextCronTime` itself is needed; it is timezone-agnostic as long as the caller normalises the seed time.
- **mutation:** Yes. Add a test case to `cron_dom_test.go` (or a new table-driven test) that passes a `from` time tagged with a non-UTC fixed zone, e.g. `time.Date(2026, 6, 14, 3, 0, 0, 0, time.FixedZone("EST", -5*3600))` (which is 08:00 UTC) for cron `"0 6 1 * *"`, and assert that the returned next-run equals `time.Date(2026, 7, 1, 6, 0, 0, 0, time.UTC)`. With the unfixed code (`time.Now()` passes local-tz time), the search finds 06:00 EST (= 11:00 UTC) on 2026-07-01, not 06:00 UTC — the assertion fails (RED). After the fix (`time.Now().UTC()` normalises the seed), the function operates entirely in UTC and returns the correct value (GREEN). The test is non-vacuous because it exercises the exact condition (non-UTC Location in the seed) that the production bug depends on.

## [16] LOW — Duplicate node_stats events emitted when two ClusterNodeDTOs share the same resolved key
- **loc:** `server/internal/cluster/discovery.go:145`  ·  **lens:** cluster  ·  status: ⏳ TODO
- **behavior:** When n.NodeID == "" the code falls back to n.IP as the map key (line 146-148). The seen map deduplicates for the stale-check step but the preceding for loop still processes every DTO independently: d.nodes[nodeID] is overwritten and a new node_stats event is appended to pending for each DTO. Two DTOs that resolve to the same key (e.g. both have empty NodeID and empty IP, colliding on "") each produce a separate node_stats event emitted after the unlock.
- **scenario:** AMS cluster endpoint returns two entries with NodeID="" and IP="" (possible with certain AMS versions or misconfigured nodes). Both iterate the for loop: first creates d.nodes[""], second overwrites it. Two node_stats events are appended to pending and both are emitted to the sink with NodeID="". The ClickHouse writer receives two inserts for the same logical node per poll cycle, inflating all node-level metrics by 2×. The fleet page shows a phantom second node with an empty ID.
- **fix:** Add a duplicate-key guard at the top of the for loop in poll(), before any processing. In server/internal/cluster/discovery.go, change the loop preamble so seen is checked before it is set:

    for _, n := range nodes {
        nodeID := n.NodeID
        if nodeID == "" {
            nodeID = n.IP
        }
        // Deduplicate within this poll cycle: skip any key already seen.
        if _, dup := seen[nodeID]; dup {
            d.logger.Warn("cluster: duplicate node key in poll response, skipping",
                "node_id", nodeID)
            continue
        }
        seen[nodeID] = struct{}{}
        // ... rest of the loop body unchanged

This causes the second (and any further) DTO with the same resolved key to be skipped entirely: no d.nodes overwrite, no pending append, no duplicate event emitted. The seen map then continues to serve both purposes — intra-poll deduplication and post-loop stale detection — without any additional data structure.
- **mutation:** Yes. A unit test can inject a ClusterClient stub returning []amsclient.ClusterNodeDTO{{NodeID: "", IP: ""}, {NodeID: "", IP: ""}} (or any two DTOs with identical non-empty NodeIDs), capture sink.WriteServerEvent calls via a recorder mock, and assert the call count is exactly 1. Removing the continue from the fix causes the count to be 2, so the mutation is detected and the test is non-vacuous.

## Refuted (do not build)
- ~~[beacon-ingest] Goroutine-per-accepted-request with no backpressure in beacon handler~~
- ~~[amsclient] app parameter not URL-path-escaped in ListBroadcasts, ListBroadcastsPaged, ListVods, ListVodsPaged~~
- ~~[amsclient] nodeID not URL-path-escaped in NodeInfo~~
- ~~[cluster] d.nodes map is never pruned — NodeCount() and Snapshot() include permanently-downed nodes~~