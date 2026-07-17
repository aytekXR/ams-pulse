# Documentation Gaps — Phase 6 Deliverable

**Produced:** S18 close (2026-07-11)
**Source sessions:** S16 (capability map), S17 (P0 live run + AV triage), S18 (P1 + this file)
**Authoring target:** S19

This file catalogs every documentation gap discovered during the D-078 validation
program. Each gap is traceable to a TC-DOC-* scenario row, an AV triage result, a
bug report, or a live S17 drift finding. The prioritized authoring plan at the end
tells S19 which documents to write first and why.

---

## Gap Table

| Gap ID | Description | Target Document | Severity | Sample User Question | Source Evidence |
|--------|-------------|-----------------|----------|----------------------|-----------------|
| DG-01 | **HLS viewer count CDN degradation** — HLS viewer counts are segment-request-based; a CDN that caches segments intercepts those requests and AMS never sees them. Pulse inherits whatever AMS reports, so counts silently undercount behind a CDN. No disclosure exists in any Pulse operator document. | `docs/AMS-INTEGRATION.md` → new §10.1 "HLS viewer counts behind a CDN" subsection in Troubleshooting | incomplete | "My HLS viewer count in Pulse is much lower than Cloudflare says — is Pulse broken?" | TC-DOC-01; capability-map.md §2a "HLS counts are segment-request-based; CDN-cached segments are not counted" |
| DG-02 | **RTMP pull viewer count = -1 semantics** — The AMS `broadcast-statistics` endpoint returns `totalRTMPWatchersCount: -1` as an "untracked" sentinel (not a real count). Pulse never calls that endpoint (dead code, BUG-001), but the `-1` value can appear in raw AMS REST captures; users who inspect the AMS API directly are confused. There is no FAQ entry explaining why RTMP pull counts show 0 in Pulse. | `docs/AMS-INTEGRATION.md` → new §10.2 "RTMP viewer count shows 0" FAQ entry; also cross-reference BUG-001 | missing-entirely | "Why does Pulse show 0 RTMP pull viewers? My AMS `broadcast-statistics` shows -1 — is that a bug?" | TC-DOC-02; capability-map.md §2a, §2b; BUG-001; AV-02 CONFIRMED |
| DG-03 | **FPS always 0 on AMS 3.x REST** — `currentFPS` is absent from the BroadcastDTO response on AMS 3.0.3 (confirmed at `client.go:97` comment). Pulse stores `fps = 0` for all REST-polled deployments. The health_score FPS weight is redistributed when fps=0, but the dashboard shows "0 FPS" with no explanation. No Known Limitations section exists in any Pulse document that names this gap. | `docs/AMS-INTEGRATION.md` → new Known Limitations section; entry: "FPS metric on AMS 3.x REST deployments" | missing-entirely | "Pulse shows FPS = 0 for all my streams even though they are playing perfectly at 30 fps. What's wrong?" | TC-DOC-03; TC-I-06 (AMS Ground Truth: `currentFPS` absent); capability-map.md §4 |
| DG-04 | **Webhook events require HMAC signing; AMS 3.0.3 cannot sign** — AMS 3.0.3 exposes `listenerHookURL` but has no HMAC secret field in any app settings panel. Pulse's webhook listener is fail-closed: it rejects every unsigned delivery with HTTP 401. The net effect is that the entire webhook path (`liveStreamStarted`, `liveStreamEnded`, `vodReady`) is silently non-functional on AMS 3.0.3. `docs/AMS-INTEGRATION.md` §4.5 has a `⚠️ REALITY CHECK` callout but does not spell out the downstream impact (recording_gb always 0, no low-latency stream events). Operators who follow the Installation Guide and set `listenerHookURL` will see only 401 WARN noise and no events. | `docs/AMS-INTEGRATION.md` §4.5 — expand the callout to include: (a) what features are degraded (recording reports, real-time webhook events), (b) the workaround (REST polling covers start/stop within 15 s; VoD recording requires a REST-poll fallback — BUG-002 roadmap item), (c) future path (unsigned-webhook mode or signing proxy — operator decision D-V2-1) | incomplete | "I configured `listenerHookURL` in AMS pointing at Pulse. Why are there no webhook events in Pulse logs and why is `recording_gb` always 0?" | TC-DOC-04; TC-WH-01; TC-WH-03; BUG-002; AV-08 CONFIRMED; capability-map.md §9; decisions.md D-066 (O3) |
| DG-05 | **CPU/mem/disk unavailable for standalone AMS via REST** — `GET /rest/v2/system-status` on AMS 3.x returns only `{osName, osArch, javaVersion, processorCount}`; no CPU, memory, or disk fields. Pulse Fleet page shows a node card with OS/JVM info but blank resource gauges. No document explains why or what the operator must do to get resource metrics (Kafka path, `PULSE_KAFKA_BROKERS`). An operator expecting Fleet health gauges will see permanent blanks and assume a configuration error. | `docs/AMS-INTEGRATION.md` → new §3.7 "Fleet resource metrics (CPU/mem/disk)" subsection; cover: (a) REST limitation on standalone AMS 3.x, (b) Kafka consumer path activation (`PULSE_KAFKA_BROKERS`), (c) cluster mode alternative (cluster nodes carry `cpuUsage`, `memoryUsage`) | missing-entirely | "My Pulse Fleet page shows the AMS node but CPU and memory are always blank. How do I enable resource monitoring?" | TC-DOC-05; TC-H-06; TC-AN-03; TC-H-01; AV-06 CONFIRMED; capability-map.md §5; AV-15 BLOCKED (Kafka, operator decision) |
| DG-06 | **Egress bytes estimate semantics** — `egress_gb` in `GET /api/v1/reports/usage` is derived from a `bitrate × watch_time` heuristic stored in `mv_usage_1d`, NOT from actual CDN/network egress data. Prod AMS shows `egress_gb: 0.0025` (one beacon session × 2 Mbps), which is a rough estimate. The session-plan.md and original scenario matrix described this as "always 0", but the S18 TC-A-08 premise correction established the actual semantics: it is a bitrate-based proxy, not measured egress. No user-facing document discloses this distinction. | `docs/AMS-INTEGRATION.md` → new Known Limitations subsection "Egress measurement"; also the Reports API reference if one exists | misleading | "Pulse shows 0.002 GB egress this month but our CDN says 50 GB. Is Pulse measuring egress?" | TC-DOC-06; TC-A-08 (S18 premise correction: egress_gb=0.0025 estimate, not truly 0); capability-map.md §15; decisions.md D-080 |
| DG-07 | **Beacon SDK integration path for AMS deployments** — `docs/AMS-INTEGRATION.md` §1.4 mentions the beacon-js SDK in two sentences but gives no step-by-step for embedding it in an AMS-served player page. Operators do not know: which player adapter to use (`ams-webrtc`, `hls.js`, `video.js`, or `native`), how to obtain an ingest token, what the `PULSE_INGEST_LISTEN_ADDR` or beacon ingest URL is, or what license tier is required. Without this guide the QoE/audience analytics features (PRD F2, F9) are unreachable for most operators. | New document: `docs/beacon-sdk.md` — step-by-step guide: (a) token provisioning, (b) player adapter selection for HLS / WebRTC / AMS native, (c) `POST /ingest/beacon` URL, (d) license gate (Pro+), (e) verifying events in `GET /api/v1/qoe/summary` | missing-entirely | "How do I add the Pulse beacon to my AMS HLS player page to get QoE data?" | TC-V-10; TC-A-05; TC-A-06; capability-map.md §10; session-plan.md §S18 Phase 6 expected gaps |
| DG-08 | **Per-app `remoteAllowedCIDR` onboarding requirement** — AMS controls REST API access per-application via `remoteAllowedCIDR`. If an app is locked to `127.0.0.1` only, Pulse receives HTTP 403 and logs a warning per poll cycle. Operators adding new AMS apps must manually open the CIDR for the Pulse container's IP before Pulse will poll them. This requirement is never mentioned in `docs/AMS-INTEGRATION.md`. The S17 live run found all 4 remaining apps were CIDR-open; TC-APP-02 (blocked app handling) could not run because no blocked app existed — precisely because the operator had already opened them, likely without documentation telling them to. | `docs/AMS-INTEGRATION.md` §2 "Prerequisites" — add: "For each AMS application Pulse should monitor, verify that `remoteAllowedCIDR` includes the Pulse container's IP (or `0.0.0.0/0` for open-access deployments). Apps with `remoteAllowedCIDR=127.0.0.1` will return HTTP 403 from the Pulse container and produce per-app warning logs." | missing-entirely | "Pulse is logging `403 Forbidden` for my `Conference` app every 5 seconds but `LiveApp` works fine. How do I fix it?" | TC-F-07; TC-APP-02 (SKIP — no blocked app on live AMS); capability-map.md §7; AV-07 |
| DG-09 | **Multi-account lockout avoidance strategy (admin@ vs aytek@)** — AMS enforces a 2-failed-login lockout per email address (not per IP) with a 5-minute hold. The production Pulse poller authenticates as `admin@`; a human operator running console sessions or the validation harness with the same account risks locking out Pulse's poller. No Pulse document advises operators on this. The `qa/realams/README.md` documents the policy for harness use, but operator-facing docs are silent. | `docs/AMS-INTEGRATION.md` §2 "Prerequisites" — add: "AMS account strategy: use a dedicated account (e.g. `admin@`) for Pulse's `PULSE_AMS_LOGIN_EMAIL` and a separate account for human console sessions. AMS locks an email address after 2 failed login attempts for 5 minutes; a human console login failure on the Pulse account will disrupt polling." | missing-entirely | "Pulse suddenly stopped polling AMS and the logs show repeated auth errors. Could my console login have caused this?" | qa/realams/README.md "AMS Lockout Warning"; MEMORY.md `ams-brute-force-lockout.md`; session-plan.md §S17 Risk Register; AMS-INTEGRATION.md §10 "401 from AMS REST API" (partial mention, not account strategy) |
| DG-10 | **S17 drift — flat HLS URL path form** — AMS 3.0.3 serves HLS at `/{app}/streams/{id}.m3u8` (flat). The `/{app}/streams/{id}/playlist.m3u8` form (used in S16 docs and TC-P-04/TC-P-05 scenario rows) always returns 404 on this build. Any operator or integration document that specifies the sub-directory form will cause HLS probe creation failures and broken player URLs. `docs/AMS-INTEGRATION.md` does not document the HLS URL form at all; `scenario-matrix.md` S17 Corrections #1 records the fix for scenario scripts, but no operator-facing document has been updated. | `docs/AMS-INTEGRATION.md` — add explicit HLS URL pattern under §1 (or a new AMS URL reference appendix); note that AMS 3.x uses flat `/{app}/streams/{id}.m3u8` and the nested `/playlist.m3u8` form does not exist on this build | misleading | "I created an HLS probe with URL `http://my-ams/LiveApp/streams/mystream/playlist.m3u8` and it always fails. What is the correct HLS URL?" | TC-DOC scenario-matrix.md S17 Corrections #1; TC-P-04; TC-P-05; scenario scripts (corrected to flat form) |
| DG-11 | **S17 drift — RTMP implicit-broadcast deletion on stop** — An RTMP publisher that connects without a pre-created broadcast (the normal case) is represented as an auto-created broadcast while live. When the publisher disconnects, AMS **deletes** the broadcast object entirely (`GET /broadcasts/{id}` → 404). It does NOT transition to `finished` or `terminated_unexpectedly`. `docs/AMS-INTEGRATION.md` §1.1 lists the states as "created, broadcasting, finished, terminated_unexpectedly" with no mention of object deletion as the terminal case. Developers building on the Pulse API who check for `finished` will miss the deletion path. | `docs/AMS-INTEGRATION.md` §1.1 (Broadcast states) — add: "Implicit RTMP broadcasts (no REST pre-create) are deleted from AMS on stop; `GET /broadcasts/{id}` returns 404. `status: finished` and `status: terminated_unexpectedly` apply to REST pre-created broadcasts. Pulse treats a 404 after broadcasting as equivalent to `finished`." | misleading | "My RTMP stream went offline but Pulse never showed it as `finished`. I only see it disappear. Is that normal?" | TC-DOC scenario-matrix.md S17 Corrections #2; TC-L-01; TC-F-01; TC-F-02; decisions.md D-079 S17 CLOSE |
| DG-12 | **S17 drift — `GET /rest/v2/applications/info` returns HTTP 405** — In AMS build 20260504_1443, the applications/info endpoint that previously returned per-app `liveStreamCount`, `vodCount`, and `storage` bytes now returns "Method Not Allowed". BUG-002 notes this as an aggravating factor: even if someone attempted a manual VoD ground-truth check against this endpoint, it would fail. `docs/AMS-INTEGRATION.md` §1.1 lists `BroadcastStatistics` and `WebRTCClientStats` but does not mention `applications/info` — however any operator following S16-era internal notes would be confused. The stable per-app VoD path (`GET /{app}/rest/v2/vods/count`) is not documented. | `docs/AMS-INTEGRATION.md` §1.1 — add footnote: "`GET /rest/v2/applications/info` is HTTP 405 on AMS 3.0.3 build 20260504_1443+. Use per-app `GET /{app}/rest/v2/vods/count` for VoD ground truth." | misleading | "I tried `GET /rest/v2/applications/info` to get VoD storage totals and got 405. Is there another endpoint?" | BUG-002 "Aggravating S17 finding"; scenario-matrix.md S17 Corrections #4; decisions.md D-079 S17 CLOSE |
| DG-13 | **S17 drift — app inventory reset detection** — AMS app inventory can change without notice (S17 observed a drop from 16 apps to 4 after an `antmedia` container restart ~18 hours before the session). Pulse auto-discovery continues but silently switches to polling only the currently live apps. There is no documented operator procedure for detecting this change or auditing which apps Pulse is currently polling. The operator-expected.md S17 section asks the operator to confirm the reset, but no document tells them what to do when they detect it or how to know if Pulse is polling the right apps after an AMS reset. | `docs/AMS-INTEGRATION.md` §10 "Troubleshooting" — add: "AMS app inventory reset: if the `antmedia` container is recreated or AMS is reinstalled, the application list changes. Check `docker logs pulse | grep 'resolveApps'` to see which apps Pulse is currently discovering. Set `PULSE_AMS_APPLICATIONS` explicitly to lock polling to a known-good app list." | missing-entirely | "We recreated our AMS container and now Pulse is not seeing streams from some apps. What changed?" | scenario-matrix.md S17 Corrections #3; decisions.md D-079 "App inventory change: 16→4"; operator-expected.md S17 section |
| DG-14 | **S17 drift — `versionType` field value** — AMS 3.0.3 returns `"versionType": "Enterprise Edition"` (two words). S16 captures and some internal notes used `"Enterprise"` (one word). Any integration code or test that does a string comparison against `"Enterprise"` will fail silently. The Fleet node card renders the version, but no Pulse document specifies the expected wire value. | `docs/AMS-INTEGRATION.md` — add a version field reference note: "`versionType` is `\"Enterprise Edition\"` (not `\"Enterprise\"`) on AMS 3.0.3." Scenario TC-FL-02 already tests for this value; the narrative doc should match. | misleading | "I'm writing a script to check if our AMS is Enterprise and `versionType == 'Enterprise'` is not matching. What is the correct value?" | scenario-matrix.md S17 Corrections #5; AV-05 CONFIRMED (fleet node shows "3.0.3"); decisions.md D-079 live version JSON |
| DG-15 | **Kafka consumer path — no configuration guide** — `docs/AMS-INTEGRATION.md` §1.3 mentions the Kafka source in two sentences: "Activated when `PULSE_KAFKA_BROKERS` is non-empty. Not covered further here; see `server/internal/collector/kafka/`." No operator document explains what Kafka topics to configure, what AMS must emit (the `ams-instance-stats` topic referenced in capability-map.md §4), what fields carry CPU/mem data, or how to verify Kafka consumption in Pulse. This gap blocks the only known path to getting Fleet resource metrics on standalone AMS. AV-15 is BLOCKED pending operator Kafka decision (operator-expected.md). | New document: `docs/kafka-integration.md` — cover: (a) what data Kafka provides that REST cannot (CPU/mem via `ams-instance-stats`), (b) AMS-side Kafka configuration, (c) `PULSE_KAFKA_BROKERS` / `PULSE_KAFKA_GROUP_ID` env vars, (d) verification steps, (e) relationship to standalone vs cluster mode | incomplete | "How do I get CPU and memory metrics for my standalone AMS in Pulse? I don't have cluster mode." | TC-DOC-05 (Kafka path); AV-15 BLOCKED; capability-map.md §4 "Kafka alternative: `ams-instance-stats` Kafka topic"; AMS-INTEGRATION.md §1.3; session-plan.md §S18 Dependencies #3 |
| DG-16 | **`speed_read_kbps` field name is misleading** — The Pulse API field `speed_read_kbps` stores the AMS `speed` realtime ratio (approximately 1.0 for a healthy ingest), NOT a bitrate in kbps. Any dashboard query or client that interprets this as a bitrate will read ~1 kbps and conclude the stream is failing. The field name is documented as misleading in `capability-map.md §4` ("MISLEADING: this is the AMS realtime ratio...") but there is no API reference document or release note warning consumers. | `docs/AMS-INTEGRATION.md` §1.1 — add note: "`speed_read_kbps` in Pulse ingest events stores the AMS `speed` ratio (~1.0 = real-time, <0.8 = ingestion backpressure), NOT a bitrate in kbps. The legacy column name is retained for backward compatibility." | misleading | "Pulse shows `speed_read_kbps: 1.02` for my 2 Mbps stream. Why is the speed reading only 1 kbps?" | capability-map.md §4 ("speed_read_kbps column name is misleading"); AMS-INTEGRATION.md §1.1 D-029v real-wire units table (speed is ratio, not bitrate) |
| DG-17 | **GeoLite2 DB absent from prod — country always blank** — `GET /api/v1/analytics/geo` returns `{country: "", views: 1}` when `PULSE_GEO_MMDB_PATH` is not set (AV-11 confirmed: prod has no GeoLite2-City.mmdb). The empty string is correct behavior, but no operator document explains how to obtain and mount the MaxMind database to get non-blank country data. AV-10 confirmed the file is absent from the prod container and the env var is commented out. | `docs/AMS-INTEGRATION.md` §6 env table — expand `PULSE_GEO_MMDB_PATH` row to include: how to obtain GeoLite2-City.mmdb (MaxMind free account), where to mount it in Docker Compose, and that geo analytics are blank (not an error) without it | missing-entirely | "My geo analytics page shows no countries even though I have real viewers. Is geo detection broken?" | AV-10 CONFIRMED ABSENT; AV-11 CONFIRMED (country="" not error); capability-map.md §14; AMS-INTEGRATION.md §6 (PULSE_GEO_MMDB_PATH row present but no procurement/mount guide) |
| DG-18 | **`packetLostRatio` semantics per ingest protocol — TCP masks loss** — AMS `packetLostRatio` and `packetsLost` in BroadcastDTO reflect UDP-layer packet loss counters populated only by WebRTC and SRT ingest paths. For RTMP ingest, TCP retransmission repairs packet loss below the application layer before AMS observes it; `packetLostRatio` is always 0.0 regardless of actual network loss. TC-I-05 (S18 AMS-SEMANTICS-FINDING) confirmed this by injecting netem 10% loss on an RTMP publisher's NIC: AMS reported `packetLostRatio=0.0`, `packetsLost=0`, `bitrate≈2 Mbps`, `status=broadcasting` — the stream was entirely healthy. Operators who monitor `packetLostRatio` for RTMP ingest will see a permanently-zero value and may incorrectly conclude the metric is broken. No document explains the protocol dependency. `packetLostRatio` for SRT/WebRTC ingest is an S19+ validation item requiring an SRT publisher setup. | `docs/AMS-INTEGRATION.md` Known Limitations section — add: "`packetLostRatio` is always 0 for RTMP ingest (TCP retransmission masks network-level loss); non-zero values are produced only by UDP-based ingest paths (SRT, WebRTC). Monitoring `packetLostRatio` is meaningful only when the ingest protocol is SRT or WebRTC." | misleading | "I injected packet loss on my RTMP ingest network but Pulse shows `packetLostRatio=0`. Is the metric broken?" | TC-I-05 S18 AMS-SEMANTICS-FINDING: netem 10% loss on RTMP/TCP publisher → AMS ratio=0, bitrate=2 Mbps, status=broadcasting; TCP absorbed all loss; tc qdisc show confirmed netem rule applied |

---

## S17 Drift Summary

The S17 live run against AMS 3.0.3 build 20260504_1443 invalidated five S16 assumptions
that are now embedded in gaps DG-10 through DG-14. All five are documented in
`scenario-matrix.md` S17 Corrections but have not yet propagated to any operator-facing
document. S19 should fix all five in a single `docs/AMS-INTEGRATION.md` editing pass
(low effort, high clarity payoff).

| S17 Correction | Gap | Priority |
|---------------|-----|---------|
| HLS flat path form | DG-10 | high |
| Implicit RTMP broadcast deleted on stop | DG-11 | high |
| applications/info → HTTP 405 | DG-12 | medium |
| App inventory 16→4 / reset detection | DG-13 | low |
| versionType "Enterprise Edition" | DG-14 | low |

---

## Prioritized Authoring Plan for S19

> **S84 status (D-146, 2026-07-17): ✅ WORKSTREAM COMPLETE — all 18 gaps closed.**
> A verify-before-writing pass found that `docs/known-limitations.md` (a comprehensive limitations doc created *after*
> this gap table) had already closed most gaps the notes below still implied were open. Full reconciliation (gap →
> closure location):
> - DG-01 → **LIM-02** (HLS count is a segment-request-window proxy, not a session count; LIM-02 cites DG-01)
> - DG-02 → **LIM-09** (RTMP-pull viewer count shows 0)
> - DG-03 → **LIM-04** (FPS always 0 on AMS 3.x REST)
> - DG-04 → AMS-INTEGRATION §4.5 + **LIM-03** (webhook non-functional + VoD ≤60 s latency)
> - DG-05 → AMS-INTEGRATION §3.7 + **LIM-01** (fleet resource metrics blank on standalone)
> - DG-06 → **LIM-07** (egress GB is a bitrate×watch-time estimate)
> - DG-07 → `docs/beacon-sdk.md`
> - DG-08 → **LIM-14** (each app needs `remoteAllowedCIDR` opened for Pulse)
> - DG-09 → **LIM-15** (AMS locks a login account after 2 failed attempts / 5 min)
> - DG-10 → **LIM-16** (HLS flat path form `/{app}/streams/{id}.m3u8`)
> - DG-11 → AMS-INTEGRATION §1.1 (implicit RTMP broadcast deleted on stop)
> - DG-15 → `docs/kafka-integration.md`
> - DG-16 → **LIM-13** (`speed_read_kbps` stores the AMS real-time ratio, not a bitrate)
> - DG-17 → **LIM-05** (GeoLite2 procurement + Docker mount)
> - DG-18 → **LIM-08** / **LIM-17** + AMS-INTEGRATION §1.1
>
> **Residual authored THIS session (S84/D-146) in `docs/AMS-INTEGRATION.md`** — the last three minor S17-drift footnotes:
> - **DG-12** — §1.1 endpoint-drift note: `GET /rest/v2/applications/info` → HTTP 405; use per-app `GET /{app}/rest/v2/vods/count`.
> - **DG-14** — §1.1 field-drift note: `versionType` is the two-word `"Enterprise Edition"`, not `"Enterprise"`.
> - **DG-13** — §10 Troubleshooting entry (app-inventory reset). **Corrected remediation:** the gap's suggested
>   `grep 'resolveApps'` marker does NOT exist — `resolveApps()` returns the app list without logging it. The accurate
>   guidance is to pin `PULSE_AMS_APPLICATIONS` and grep the real `restpoller: app poll error` warning (verified against
>   `server/internal/collector/restpoller/restpoller.go:238,492`).
>
> **Net:** the S18 Phase-6 documentation-gap deliverable is fully closed. No open documentation gaps remain.
>
> ---
>
> **S31 status (D-093, 2026-07-14): DG-18 CLOSED** — Live SRT validation
> TC-I-05-SRT PASS (2/2 assertions, 2026-07-14T02:29:45Z; evidence:
> `qa/realams/evidence/TC-I-05-SRT-20260714T022945Z/`): `status=broadcasting`,
> `bitrate=1148432 bps`, `packetLostRatio=0.0`, Pulse `packet_loss_pct=0`.
> Post-ARQ semantics confirmed: clean 0% loss run → ratio=0 (expected).
> SRT publishType live-observed: AMS reports `publishType="RTMP"` for SRT-ingested
> streams (F5 finding, D-093); Pulse mirrors AMS verbatim. Additional finding: AMS
> SRTAdaptor requires plain streamId form `LiveApp/<id>`; ACF prefix forms rejected.
> `docs/AMS-INTEGRATION.md` updated with S31 PASS record and both new findings.
> **DG-18 is CLOSED** — the documentation gap is filled; no further validation needed.
>
> *Prior blocked runs superseded:* S29/D-091 (license suspended); S30 late-session
> (license gate cleared, hit VPS high-resource-usage guard at load=14).
>
> **S29 status (D-091, 2026-07-13): DG-18 variant note AUTHORED** — the
> `packetLostRatio` per-protocol semantics note (RTMP always 0 / SRT post-ARQ /
> WebRTC UDP-native) was added to `docs/AMS-INTEGRATION.md` §1.1 as a callout
> table after the D-029v real-wire units paragraph. Live SRT ingest validation
> (TC-I-05-SRT-packet-loss.sh) was run on 2026-07-13 and blocked by AMS EE
> license suspension (evidence: `qa/realams/evidence/S29-TC-I-05-SRT-20260713T215932Z/`).
> Superseded by S31 PASS above.
>
> **S28 status (D-090, 2026-07-13): DG-15 and DG-05 are AUTHORED** — DG-15
> created `docs/kafka-integration.md` (code-authoritative guide: topic
> `ams-server-events` per kafka.go — note this CORRECTS the `ams-instance-stats`
> name this file and capability-map.md §4 carried; AV-15 stays BLOCKED and the
> doc says so prominently); DG-05 added `docs/AMS-INTEGRATION.md` §3.7 (fleet
> resource metrics stub pointing at the new guide). Both adversarially verified
> against the code (2 verifier catches fixed: healthz lag>10000 degradation,
> first-start FirstOffset replay).
>
> **S19 status (D-081, 2026-07-11): DG-04, DG-11, and DG-07 are AUTHORED** —
> DG-04 expanded `docs/AMS-INTEGRATION.md` §4.5 (downstream impact, workarounds,
> D-V2-1 future path); DG-11 added the implicit-broadcast deletion admonition to
> §1.1; DG-07 created `docs/beacon-sdk.md` (full integration guide, 12 sections).
> All three passed adversarial verification against primary sources. The
> remaining gaps below carry to future sessions.

The order below weights: (1) operator confusion that causes real production problems,
(2) user questions with no answer anywhere, (3) gaps that block PRD feature
adoption, (4) S17 drift fixes that are low-effort and prevent future confusion.

### Tier 1 — Write first (blocks operator or hides production degradation)

**1. DG-04 — Webhook signing limitation + VoD recording impact**
Operators who configure `listenerHookURL` will see silent 401 rejections and a
permanently-zero `recording_gb`. This is the most consequential documentation gap;
it touches PRD F6 (billing) and PRD F10 (webhooks). The `docs/AMS-INTEGRATION.md`
callout exists but is incomplete. Extend it with the downstream impact list and
the two workarounds (REST poll covers start/stop; VoD REST poll fallback on the
roadmap). Estimated effort: 1 hour (extending existing section).

**2. DG-05 + DG-15 — Standalone CPU/mem blank + Kafka guide**
A blank Fleet page is the first thing any new operator notices and complains about.
DG-05 (add a §3.7 note in AMS-INTEGRATION.md) should be done in the same pass as
DG-15 (new `docs/kafka-integration.md`). Together they close the only question
that blocks Fleet health adoption. Kafka is also the operator's pending decision
(AV-15 blocked); the doc should state clearly what the operator needs to decide.
Estimated effort: 2–3 hours (one new doc + one section extension).

**3. DG-11 — Implicit RTMP broadcast deletion on stop**
This is a developer-facing correctness issue. Any integration that polls for
`status: finished` on an implicit RTMP broadcast will never see it. Fix is a
single paragraph in AMS-INTEGRATION.md §1.1. High impact, low effort.
Estimated effort: 30 minutes.

### Tier 2 — Write next (user confusion, no answer anywhere)

**4. DG-02 — RTMP -1 semantics FAQ**
The `-1` value from `broadcast-statistics` is the most common "is this a bug?"
question users will file after inspecting raw AMS API output. A FAQ entry prevents
spurious bug reports. Add to AMS-INTEGRATION.md §10.
Estimated effort: 30 minutes.

**5. DG-07 — Beacon SDK integration guide**
Without this, QoE analytics (PRD F2, F9) are adopted by essentially zero operators.
The SDK exists and works (CI-proven); the gap is purely documentation.
A new `docs/beacon-sdk.md` with player adapter selection, token provisioning, and
a quick-start snippet. Estimated effort: 2 hours.

**6. DG-09 — Lockout avoidance strategy**
A production outage caused by a human operator accidentally locking the Pulse poller
account is a high-severity incident. A two-sentence prerequisite note in
AMS-INTEGRATION.md §2 eliminates the risk.
Estimated effort: 20 minutes.

**7. DG-08 — Per-app CIDR onboarding**
Without this, operators adding new AMS apps to a restricted deployment will see
repeated 403 logs with no guidance. Add to AMS-INTEGRATION.md §2.
Estimated effort: 20 minutes.

### Tier 3 — Write alongside Tier 2 (S17 drift, low effort, batch together)

**8. DG-10 + DG-12 + DG-14 — S17 drift corrections in AMS-INTEGRATION.md**
HLS flat path form, applications/info 405, and versionType "Enterprise Edition"
can all be fixed in a single editing pass on AMS-INTEGRATION.md. Each is a one-line
factual correction. Estimated effort: 30 minutes total.

**9. DG-01 — HLS CDN viewer count**
A Troubleshooting subsection in AMS-INTEGRATION.md. Prevents user confusion about
viewer count discrepancies between Pulse and CDN analytics panels.
Estimated effort: 30 minutes.

**10. DG-03 — FPS always 0 Known Limitation**
Add to AMS-INTEGRATION.md Known Limitations (or health score docs). Prevents
"my health score seems wrong" questions tied to FPS=0.
Estimated effort: 20 minutes.

### Tier 4 — Write last (semantic nuances, lower immediate operator impact)

**11. DG-06 — Egress estimate semantics**
The bitrate×watch-time heuristic vs real CDN egress is a billing accuracy concern,
but `egress_gb` is small enough (~0.002 GB/session) that operators are unlikely to
rely on it for actual billing without verification. Document in Reports API reference.
Estimated effort: 30 minutes.

**12. DG-16 — `speed_read_kbps` field name**
A developer-facing gotcha in the API. Low operator impact today because few
integrations query this field directly. One note in AMS-INTEGRATION.md §1.1.
Estimated effort: 15 minutes.

**13. DG-13 — App inventory reset detection**
Rare scenario (AMS container recreation). A Troubleshooting entry is valuable but
not urgent. Estimated effort: 20 minutes.

**14. DG-17 — GeoLite2 procurement + mount guide**
`PULSE_GEO_MMDB_PATH` is documented in the env table but without a procurement path.
Geo analytics require a MaxMind free account and a Docker volume mount. Add to
AMS-INTEGRATION.md §6.
Estimated effort: 30 minutes.

---

## Document Creation vs Extension Summary

| Action | Document | Gaps Addressed |
|--------|----------|----------------|
| **Create new** | `docs/beacon-sdk.md` | DG-07 |
| **Create new** | `docs/kafka-integration.md` | DG-15 |
| **Extend existing** | `docs/AMS-INTEGRATION.md` §1.1 (Broadcast states + HLS path + speed_read_kbps + applications/info + versionType) | DG-11, DG-10, DG-16, DG-12, DG-14 |
| **Extend existing** | `docs/AMS-INTEGRATION.md` §2 (Prerequisites — CIDR + lockout strategy) | DG-08, DG-09 |
| **Extend existing** | `docs/AMS-INTEGRATION.md` §3 (new §3.7 Fleet resource metrics) | DG-05 |
| **Extend existing** | `docs/AMS-INTEGRATION.md` §4.5 (Webhook — expand callout) | DG-04 |
| **Extend existing** | `docs/AMS-INTEGRATION.md` §6 (env table — GeoLite2 row) | DG-17 |
| **Extend existing** | `docs/AMS-INTEGRATION.md` §10 (Troubleshooting — CDN, RTMP -1 FAQ, app reset) | DG-01, DG-02, DG-13 |
| **Extend existing** | `docs/AMS-INTEGRATION.md` Known Limitations (new section) | DG-03, DG-06 |

Total new documents: **2**. Total sections to extend in `docs/AMS-INTEGRATION.md`: **8**
(can be done in a single focused S19 editing pass of ~6–8 hours).
