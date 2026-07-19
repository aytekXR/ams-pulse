# Pulse load lane — `qa/realams/load/`

Opt-in load-testing lane that answers Ant Media's ask ("verify how your stats hold up under
load"). It proves two things at once: (1) a real AMS carries the media load, and (2) **Pulse's
numbers stay correct and its budgets hold while it does** — convergence, oracle parity, API latency,
alert latency, and ghost-free teardown. Every assertion is a **delta on streams we own** and cannot
false-green. Full design + budgets (L-1..L-9): `docs/testing/full-e2e-validation-run.md` §Rev2.

## ⚠ Dedicated instance only

The generators are **never** pointed at the shared validation VPS (`ams.beyondkaira.com`,
~5–7 concurrent RTMP capacity, shared admin account with the prod poller), prod, or Ant Media's own
hosts. Spin up a throw-away AMS on the pay-as-you-go hourly licence, run the lane, tear it down. The
lane reads **only** `harness/load-env.sh` (never `env.sh`), so it structurally cannot reach the
shared VPS; a `load-env.sh` guard hard-aborts if a forbidden host is detected.

## Run it

```bash
cp qa/realams/harness/load-env.sh.example qa/realams/harness/load-env.sh
$EDITOR qa/realams/harness/load-env.sh          # replace every REPLACE-ME value
bash qa/realams/run-load-suite.sh               # → LOAD-REPORT.md under $EVIDENCE_ROOT
```

It is also phase **45** of `run-full-e2e.sh` (SKIPs 77 when `load-env.sh` is absent).

## Generators

Default is **native** — the repo's own harness primitives (`start_bulk_publishers`,
`ramp_hls_viewers`/`start_hls_viewer`, `start_webrtc_viewer`): zero downloads, synthetic source, and
the HLS viewer sim uses real-player segment-once semantics (no refetch inflation). Set
`LOAD_GENERATOR=official` to wrap Ant Media's own
[Scripts](https://github.com/ant-media/Scripts/tree/master/load-testing) (`rtmp_publisher.sh`) and,
via `WEBRTC_TEST_DIR`, the [WebRTC Load Test Tool](https://docs.antmedia.io/guides/configuration-and-testing/load-testing/webrtc-load-testing/)
— useful when the marketplace submission should speak Ant Media's toolkit vocabulary, or for WebRTC
viewer scale beyond the native Playwright path (which caps at 8 browsers).

## Scenarios (`load/scenarios/`, isolated from `scenarios/`)

Kept out of `qa/realams/scenarios/` on purpose: `make validate-all` and `run-full-e2e.sh` phase 41
sweep `scenarios/TC-*.sh` against the **shared VPS**, so load scenarios must not live there.

| Scenario | Budgets | Proves |
|---|---|---|
| `TC-S-10-publisher-ramp` | L-1, L-2, L-3, L-9 | Pulse converges to +N ≤120 s; AMS↔Pulse delta parity; `/live/overview` p95 <300 ms (N≤50); ghost-free teardown |
| `TC-S-11-viewer-scale` | L-6 | WebRTC viewer count parity ±10% (asserted); HLS recorded only (known inflation, see TC-V-10) |
| `TC-S-12-soak` | L-4, L-5, L-8 | count stable ≥95% of samples over the soak; a mid-soak guaranteed-fire alert reaches `firing` ≤30 s; RSS slope recorded |
| `TC-S-13-churn` | L-9 | after start/stop storms the count returns exactly to baseline (no ghosts); no waves lose streams |
