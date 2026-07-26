# Marketplace Compliance Review — Round 3 (consolidated, 2026-07-26)

*Operator-supplied third-party review, recorded verbatim in substance. Findings
R1–R15 were raised against the v0.4.2 tree. This file is the record; the
disposition of each finding is in the table at the end and in `decisions.md` D-175.*

> **⚠ One premise of this review is stale.** The review states "v0.4.2 cut locally,
> **tag not yet pushed** — GitHub Latest is still v0.4.1" and makes "push the tag"
> its P0 #2. That was true when the review was authored. It is not true now:
> **v0.4.2 was tagged and released on 2026-07-26 at 17:44 UTC** — `0.4.2` and
> `latest` resolve to the same digest and are anonymously pullable, cosign-signed,
> with binaries + SHA256SUMS + SDK tarball + Helm chart attached, and the Helm
> chart OCI package is anonymously pullable too. Consequently R1/R2 could **not**
> "land before tagging"; v0.4.2 shipped with them, and they land in the next
> release. R10's stale doc stamps were likewise already public.

---

## Executive verdict (as submitted)

**Conditionally ready.** Across three passes, 36 distinct findings were raised; as
of the v0.4.2 tree, every Round-1 and Round-2 finding is fixed or explicitly
dispositioned, all three documented install paths work (two of them now
CI-gated), the listing pack is submission-grade, and the AMS integration matches
the official 3.0.3 surface — which is still the latest AMS release.

Round 3 raised 15 residual findings (R1–R15) of strictly decreasing severity,
none release-blocking, two (R1, R2) worth catching before the tag.

---

## Round 3 findings — R1–R15

**R1 (P1) — Failure-streak events use the configured node ID, so on a cluster the
degraded-node alert ladder is dead during AMS API outages.** `failureStreakEvent()`
emits `NodeID: p.cfg.NodeID` while, post-N2, cluster success events carry real
node IDs. The aggregator treats `api_unreachable` events for unknown node keys as
no-ops (D-087: "failure events create nothing"), so `ConsecAPIErrors` stays 0 for
every real node and `node_degraded` never fires for the whole outage. Existing
tests miss it because both sides of the test use the same ID.

**R2 (P1) — `cluster.Discovery` fabricates the zero-fields the restpoller just
stopped fabricating — now on the same node keys.** Discovery unconditionally emits
`disk_pct`, `net_in_mbps`, `net_out_mbps`, `jvm_heap_used_mb`, `version` — fields
real AMS 3.x never sends. Post-N2 the two emitters collide on `PrimaryID()`: the
clean restpoller event is overwritten by discovery's zeros every 30 s, presence
flags flap, the Fleet card shows "Disk 0%" as a measurement, and zeros feed the
Welford anomaly baselines.

**R3 (P2) — `AMS-INTEGRATION.md` line-citations are partially wrong; one got worse
this wave.** `validateHMAC` cited at `webhook.go:361` (actually `:372`);
`client.go:88` cited for `publishType` (wrong file entirely); pre-rework
`restpoller.go` ranges; the env table omits ~22 live `PULSE_*` vars.

**R4 (P2) — The low-footprint scoping fix reached the compose XML but not the Helm
configmap, and the parity claim is false.** `configmap-clickhouse.yaml` keeps
`<max_threads>` at server scope (silently ignored), keeps the deprecated
`max_memory_usage_for_all_queries`, and its `max_server_memory_usage` does not
match the XML's 768 MB.

**R5 (P2) — NaN/Inf rejection degrades to a fabricated 0 with the presence flag
set.** `CPUPct`/`MemPct` reject non-finite strings but fall back to the alias field
(0 on real AMS), and `normalize.go` emits `cpu_pct`/`mem_pct` unconditionally — so
a `"cpu":"NaN"` JMX hiccup records CPU 0% as a measurement.

**R6 (P2) — AMS's own liveness signals are decoded and ignored: a dead-but-listed
cluster node reports "ok" forever.** Status is derived purely from cpu/mem > 90;
the staleness sweep fires only when a node vanishes from the response — but AMS
keeps dead members listed with a frozen `lastUpdateTime`.

**R7 (P2) — A failed release leaves the vulnerable image publicly pullable under
`candidate-<sha>` forever.** The quarantine flow never deletes candidates.

**R8 (P2) — Chart `version: 0.3.0` unchanged while chart content changed;
`helm push` will overwrite the published 0.3.0 artifact with different content.**
Guard check #2 covers `appVersion` only.

**R9 (P3) — `stream_ingest_error` rows are stored but reachable by nothing and
disclosed nowhere.** No aggregator case, alert condition, API filter, or UI
surface; no mention in known-limitations/FAQ.

**R10 (P3) — Three stale `v0.4.1` stamps in submission-facing docs, structurally
invisible to the 13-check guard.** `submission-package.md:4` (the header of the
index handed to Ant Media); its clean-room narrative pinned to `…:0.4.1`;
`known-limitations.md` body text while its header says v0.4.2.

**R11 (P3) — Installer overwrites a self-signed `PULSE_LICENSE_PUBKEY` on every
re-run** — same class as the N5 bug, for the self-signing case its own comment
anticipates.

**R12 (P3) — Grep-extraction misses `export`-prefixed or indented keys**,
re-introducing Issue-G data loss for hand-edited `.env` files (new key minted →
meta store undecryptable).

**R13 (P3) — Production IP still present in 8 operator-facing spots** outside the
scrubbed set, plus ~85 historical hits under `agents/handoffs/**`.

**R14 (P3) — `compose-boot` CI skips on exactly the commit a release gates on.**
The job defers when the compose pin equals `VERSION` but isn't on GHCR yet, and
the release gate requires a green `ci` run for that same SHA — so the README
evaluator path is never booted against the about-to-be-released state.

**R15 (P3) — Cross-surface node-ID mismatch after N2:** stream/ingest events still
carry `p.cfg.NodeID` while node_stats carry real cluster IDs; filtering streams by
a node ID shown in the Fleet view returns zero rows on a cluster.

---

## Disposition (S108 / D-175)

Every finding was re-verified against the tree before any change. One sub-claim
was refuted; one was already dispositioned by operator decision.

| ID | Verified | Action |
|---|---|---|
| R1 | CONFIRMED | Fixed — failure-streak events fan out over the last successful cluster poll's real node IDs; standalone keeps `cfg.NodeID`; IDs cleared when the cluster goes away. 3 regression tests. |
| R2 | CONFIRMED | Fixed — `discovery.go` emission made conditional, mirroring `NormalizeClusterNode` exactly. Anti-drift test asserts the two emitters agree key-for-key. |
| R3 | CONFIRMED | Fixed — all 32 `file.go:NNN` citations de-numbered (symbol names only, no drift mechanism left); the wrong-file `publishType` citation repointed to `normalizePublishType`; env table re-scoped with `admin-guide.md` named as the complete reference. |
| R4 | CONFIRMED (numeric sub-claim CONFIRMED on re-check) | Fixed — `max_threads` moved into the default profile, deprecated `max_memory_usage_for_all_queries` dropped, `max_server_memory_usage` repointed to the 768 MB value so the chart matches the compose XML. Old values key kept as a deprecated alias. Goldens regenerated. |
| R5 | CONFIRMED | Fixed — new `CPUPctOK`/`MemPctOK` return a presence flag; a non-finite reading with no usable alias is reported ABSENT, and both emitters skip the field. 4 tests. |
| R6 | CONFIRMED | Fixed — AMS `status` and `lastUpdateTime` now mark a node down, overriding the load heuristic; transition logged. 5 tests incl. the dead-edge viewer-suppression consequence. |
| R7 | CONFIRMED | Fixed — `if: always()` cleanup deletes the quarantine image, but ONLY when the candidate tag is the digest's sole tag. (A GHCR "version" is a digest and promotion re-tags the same digest, so unconditional deletion would have deleted the published release, its SBOM and its signature.) |
| R8 | CONFIRMED | Fixed — chart bumped to 0.3.1; new release-guard check #16 fails the release when `deploy/helm/pulse/` changed since the previous tag without a chart-version bump. |
| R9 | CONFIRMED | Disclosed — LIM-27 added with the ClickHouse query operators need. Read-path surfacing remains open work, now stated honestly rather than implied. |
| R10 | CONFIRMED | Fixed — all three stamps corrected; new guard checks #14 (submission-package header) and #15 (SDK install-section tarball) close the structural blind spot. |
| R11 | CONFIRMED | Fixed — an existing `PULSE_LICENSE_PUBKEY` is preserved across re-runs, with a notice when a self-signed key is detected. |
| R12 | CONFIRMED | Fixed — extraction anchor tolerates leading whitespace and `export `; stale comment corrected. |
| R13 | ALREADY DISPOSITIONED | No change. D-174 (operator decision) deliberately keeps the IP where it is *functional* — nginx vhost, load-lane prod-guard blocklist, runbook `--resolve` commands — because public DNS for the host resolves to it anyway. Rotation remains on the operator queue. |
| R14 | CONFIRMED | Fixed — the deferral path now BUILDS the pinned image locally and boots it, instead of skipping. |
| R15 | CONFIRMED | Disclosed — LIM-28 + a code comment at each site. The real fix needs the owning node threaded through per-app polling and is deferred until the cluster path is live-validated (LIM-10), rather than guessed. |

### Found independently in the same pass (not in this review)

A parallel verify-first audit of the published v0.4.2 artifact surfaced five more,
all fixed here:

- **Production ClickHouse password prefix committed to the public repo** —
  `server/cmd/pulse/migrate_test.go` used the first 32 of the 48 hex chars of the
  live `CLICKHOUSE_PASSWORD` as test input, in git history since `98b011c`.
  Replaced with a dummy; **rotation is on the operator queue and is now the
  highest-priority item there**.
- **SDK install docs advertised the broken 0.4.1 tarball** — the version whose
  `Pulse.init()` was a silent no-op (N7). Repointed to 0.4.2 and guarded (#15).
- **`licensing-public.md` understated the Pro tier** — analytics CSV export is
  gated at Pro+ (`CheckDataAPI`), not Business+; only the usage/billing report
  export is Business+ (`CheckReports`).
- **The flagship listing screenshot showed a broken product** — `ss1-dashboard.png`
  rendered UNKNOWN state/health on all 8 streams and 0 viewers/0 publishers in the
  BY APPLICATION panel, because the capture mocks used field names the schema does
  not define. `ss3-alerting.png` showed only the Rules tab while its caption
  promised incident history. Both regenerated and visually verified; the protocol
  donut's labels, which overlapped its own legend, were also fixed.
- **The documented README evaluator command published no port** — the base compose
  file is `expose:`-only and explicit `-f` flags suppress the auto-override, so
  `docker compose -f deploy/docker-compose.yml up -d` produced a healthy stack with
  an unreachable UI. Added `deploy/docker-compose.evaluator.yml`; CI now boots the
  documented command and asserts the UI over the published host port.
