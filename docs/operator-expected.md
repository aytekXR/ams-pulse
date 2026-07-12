# Operator TODO — the items only YOU can do (updated at SESSION-24 close, D-086, 2026-07-12; rides S24's PR)

## ⚡ TL;DR — expected from you right now (2026-07-12, SESSION-24 closed — D-086)

> **Nothing is needed from you right now.** S24 ran fully autonomously (your
> open items were re-checked at open: no answers had arrived, nothing
> blocked; the one gated decision — building the anomaly history store — was
> resolved by the plan's own rules and recorded in D-086).
>
> **★ THE HEADLINE: the last of the API parameter debt (outside
> multi-tenancy) is gone.** Since S21 your API contract's declared-parameter
> audit had `GET /anomalies ?from/?to` pinned as "declared but ignored"
> (BUG-008's final piece — answering a time-range question needs history
> that was never stored). Pulse now **persists every anomaly detection** to
> a ClickHouse table (with your tier's retention), survives restarts without
> double-recording, and `/anomalies?from=…&to=…` answers honestly from that
> history — with real pagination. Of the 86 declared parameters, **84 are
> now proven honored; the 2 remaining are the `?tenant` pair**, which needs
> a multi-tenancy data model (a product decision, not a bug).
>
> **The quality net earned its keep twice this session:** (1) the build
> found the ClickHouse driver silently truncates timestamps to whole seconds
> in query parameters — the spec'd pagination design would have duplicated
> rows at page boundaries; caught live, fixed, and pinned so it can never
> return. (2) One build agent stalled and was auto-retried mid-work; per the
> standing rule its half-gated work was NOT trusted — all 9 "would the tests
> really catch it?" sabotage proofs were re-derived from scratch, which
> exposed two weak tests (one could silently skip, one passed by luck ~999
> times in 1000) — both strengthened before merge.
>
> **Also re-proven today:** your recording/billing fix (S23) is holding live
> — the usage report still shows exactly 0.003126 GB after 3 more hours of
> polling (no double-billing drift), and your AMS post-expiry state is
> byte-identical for the third consecutive sweep (your `antmedia` container
> still hasn't restarted since before the lapse, so the "enforcement bites
> at restart" hypothesis stays untested; sessions keep observing, never
> restarting it).
>
> **Still waiting on your two standing decisions (unchanged, non-blocking):**
> caddy-vhost merge + final-assessment DRAFT review — details in the S21
> TL;DR below. **The rollout question grows again:** a prod rollout now
> carries FIVE sessions of fixes (D-082..D-086 — every BUG-002..010 fix,
> recording/billing, and persistent anomaly history). Say "roll out"
> whenever you want them live; prod stays untouched until then.

## 🔎 What SESSION-24 did (2026-07-12, closed — D-086)

| Area | Result |
|---|---|
| **BUG-008 fully fixed** | `GET /anomalies` time-range queries (`?from`/`?to`) now work: anomaly detections are persisted (ClickHouse, tier retention: Free 7 d / Pro 90 d / Business 13 mo), restart-safe (warm-up prevents duplicate records), queryable with cursor pagination, and the endpoint's `metric`/`app`/`stream`/`min_sigma` filters work on the history path too. Design per ADR-0009 (now Accepted), which S23 wrote and adversarially fact-checked. |
| **Driver bug caught pre-ship** | The ClickHouse Go driver sends timestamps at second precision in query parameters; the millisecond-precision pagination cursor therefore re-admitted same-second rows (duplicates at page boundaries — later proven able to loop forever). Fixed with integer-millisecond comparison; a regression test now forces the same-second case deterministically. |
| **Conformance registry** | 37 live parameter probes (was 35) / **2 known-violations remain, both `?tenant`** (multi-tenancy — product decision) / floor raised so it can't decay. The BUG-004/005-class ("declared but ignored") is now structurally extinct outside tenancy. |
| **Recording fix re-proven** | TC-REC-01 re-run against the validation stack: 3/3 PASS, recording_gb byte-stable after ~3 h of continued polling — the no-double-billing memory holds live. |
| **Quality net** | 10 workflow agents (4 scouts, 3 authors [1 auto-retried after a stall — its tree was re-gated, not trusted], 3 adversarial verifiers). 9/9 sabotage-mutation proofs RED; 2 weak tests found by the verifiers (silent-skip pin, luck-dependent fixture) strengthened and re-proven same-session. |
| **Ops** | Gates: 24/24 Go packages race-clean, 0 failures (3 pre-existing env-gated infra skips only), coverage 75.5% (floor 70.2 — the small dip from 76.0 is new store code covered by integration tests rather than unit tests), contract byte-unchanged. Prod untouched; AMS read-only; third byte-identical post-expiry sweep. CI-promotion date gate still closed (opens 07-23) → skip carry ×13. One PR. |

## (superseded) S23-close header follows

## ⚡ TL;DR — expected from you right now (2026-07-12, SESSION-23 closed — D-085)

> **Nothing is needed from you right now.** S23 ran fully autonomously (your
> open items were re-checked at open: no answers had arrived, nothing blocked).
>
> **★ THE HEADLINE: the recording/billing gap is FIXED (BUG-002) — and proven
> against your real AMS.** Since S17 we knew `recording_gb` was structurally 0
> on every AMS 3.0.3 deployment (AMS can't sign the `vodReady` webhook, and
> Pulse's webhook door stays locked by design). Pulse now polls the AMS VoD
> REST list directly (read-only, once a minute), remembers per VoD what it
> already counted (restart-safe — no double-billing), and rolls sizes into the
> billing report. **Live proof on your server today: the usage report now
> shows recording_gb = 0.003126 for the S17 test VoD — within 0.02% of the
> file's true 3,125,555 bytes.** This was the LAST failing row in the
> marketplace-readiness checklist; "No P0 open bugs" now reads PASS.
>
> **The assessment numbers moved (still DRAFT, still waiting on your review):**
> product completeness is now **65.2% strict / 83.0% weighted** (was
> 60.6/79.9 at S19 close) after the S20–S23 fixes were folded into the
> validation matrix. Only BUG-001 (low, no user impact) remains open of the
> 10 bugs the program filed.
>
> **Also this session:** the design for the last `/anomalies` parameter gap
> (BUG-008 `from`/`to`) is written as ADR-0009 — building it is a full
> session and waits for the plan/your nod; the two parameters stay honestly
> pinned until then. FYI: your isolated validation stack (`pulse-realams`,
> loopback :18090) was reset and now runs today's build — its old test data
> was disposable by design.
>
> **Your AMS after the trial lapse — still nothing changed** (second
> byte-identical sweep; your `antmedia` container has not restarted since
> before the lapse, so the "enforcement bites at restart" hypothesis remains
> untested; sessions keep observing, never restarting it).
>
> **Still waiting on your two standing decisions (unchanged, non-blocking):**
> caddy-vhost merge + final-assessment DRAFT review — details in the S21
> TL;DR below. **A third one is now worth a thought:** a prod rollout would
> carry FOUR sessions of fixes (D-082..D-085 — every BUG-002..010 fix
> including recording/billing). Say "roll out" whenever you want them live;
> prod stays untouched until then.

## 🔎 What SESSION-23 did (2026-07-12, closed — D-085)

| Area | Result |
|---|---|
| **BUG-002 fixed (recording/billing)** | VoD REST poll fallback exactly per the S20 design note, upgraded to a safer dedup after a live probe confirmed AMS exposes a stable `vodId` (all five of the design note's open questions were answered by ONE read-only REST call at session open). Two additive migrations (ClickHouse view + meta seen-set table), 8 poller tests, restart-resume and no-double-count regression pins. **Live-validated end-to-end on your AMS: 3/3 PASS, 0.02% reconciliation.** |
| **Two traps caught before code** | The scouts found (1) the poller's event deduplicator would have silently swallowed same-minute VoD events (bypassed + pinned by a regression test) and (2) the AMS field `streamName` on VoDs is actually the FILE name — attribution uses `streamId`. Either would have shipped a subtly wrong fix. |
| **BUG-008 phase-2 designed** | ADR-0009: a persistent anomaly flag-event store (ClickHouse) that will make `/anomalies?from&to` honest. Adversarially fact-checked (19 code citations verified). Build deferred — it's a full session of work; the 2 parameters stay visibly pinned until built. |
| **Assessment refreshed** | All S20–S22 bug fixes + today's BUG-002 fix folded into the validation matrix and the marketplace report: completeness 65.2% strict / 83.0% weighted; marketplace "No P0 open bugs" flips FAIL→PASS. Both docs remain DRAFT pending your review. |
| **Quality net** | 13 workflow agents (4 scouts, 6 authors, 3 adversarial verifiers), 0 errors, 0 must-fix findings; 5 mutation proofs run in pristine copies (the one gap found — a Postgres migration-chain omission no test caught — got its guard test same-session). |
| **Ops** | Gates: 24/24 Go packages race-clean, 0 failures, coverage 75.9%→**76.0%** (floor 70.2). Prod untouched; AMS read-only. CI-promotion date gate still closed (opens 07-23, ~11 days) → skip carry ×12. One PR. |

## (superseded) S22-close header follows

## ⚡ TL;DR — expected from you right now (2026-07-12, SESSION-22 closed — D-084)

> **Nothing is needed from you right now.** S22 opened early (05:23Z, before
> your 12:09Z AMS trial lapse), held itself open, and ran the post-expiry
> sweep at 12:11Z — nothing was skipped, nothing re-gated a 4th time.
>
> **★ THE EXPIRY ANSWER (D-084): your AMS trial lapsed at 12:09Z and — so far
> — NOTHING changed.** The post-expiry sweep is **byte-identical** to the
> pre-expiry baseline: still "Enterprise Edition" 3.0.3, licence-status
> endpoint unchanged, all 4 apps + settings intact, and AMS **accepted a
> fresh RTMP publish after the lapse** (live-probed: the teststream was found
> crashed — 5 h *before* the lapse, plain ffmpeg crash, unrelated — restarted,
> AMS took the publish, HLS serves, Pulse sees it). No validation scenario is
> blocked. **One caveat for you:** trial enforcement may only kick in when the
> AMS *process restarts* (boot-time license check). Sessions will NOT restart
> your `antmedia` container to test that — if it ever restarts and features
> start 403ing, sessions will observe + report per your "handled" directive.
>
> **The parameter-debt cleanup landed, fully gated:** the declared-parameter
> debt S21 pinned is fixed test-first and adversarially verified — pagination
> is now real on 10 list endpoints (BUG-006/007), `/live/streams` paging
> actually advances (BUG-009 partial; `tenant` filtering honestly stays
> pinned until the multi-tenancy data model exists), the audience CSV export
> is now declared in the API contract (BUG-010, the one deliberate contract
> change), and 4 of 6 dead `/anomalies` filters now work (BUG-008 partial;
> honoring `from`/`to` needs a persistent flag-event store — S23 designs it,
> written triage in `docs/assessment/bugs/BUG-008-triage-s22.md`).
> **Declared-param violations: 28/85 at S21 close → 4/86 now**, each remaining
> one pinned with a written reason and a path (2× tenant → multi-tenancy; 2×
> anomalies time-range → S23).
>
> **Still waiting on your two standing decisions (unchanged, non-blocking):**
> caddy-vhost merge + final-assessment DRAFT review — details in the S21 TL;DR
> below. Today's fixes reach prod only with your next approved rollout; a
> rollout now carries three sessions of API-correctness fixes
> (BUG-003/004/005/006/007 + partials of 008/009 + two panic fixes).

## 🔎 What SESSION-22 did (2026-07-12, closed — D-084)

| Area | Result |
|---|---|
| **License-expiry sweep** | Your AMS trial lapsed 12:09Z; the post-expiry sweep is **byte-identical** to the pre-expiry baseline — still Enterprise Edition 3.0.3, all 4 apps intact, and AMS **accepted a fresh RTMP publish after the lapse** (live-probed). Nothing is blocked. Caveat on record: enforcement may only kick in when the AMS process restarts — sessions will not restart it to test; they re-check at each open. |
| **Teststream crash (found+fixed)** | The synthetic test publisher had crashed ~07:10 (plain ffmpeg crash, 5 h before the lapse — unrelated). Restarted; it doubled as the post-expiry publish probe. |
| **Pagination real everywhere** | Every list endpoint that declared `limit`/`cursor` now honors them, down to the database layer, with proper `next_cursor` paging. A caller asking for 1 item now gets 1 item, not the whole table. |
| **Two crashes prevented** | The adversarial verifiers caught two panic bugs *introduced-then-fixed inside the session, before anything shipped*: a stale page-cursor crashing `/live/streams`, and `?limit=-1` crashing alert history into an HTTP 500. Both now have regression tests. |
| **Contract honesty** | The audience CSV export your API always had is now declared in the OpenAPI contract (generated clients can finally see it). The conformance gate grew: 35 live parameter probes (was 11 at S21 close), floor raised so it can't silently decay. |
| **Quality net** | 16 workflow agents (4 scouts, designer, 3 TDD authors, assessor, 3 remediation authors, 4 adversarial verifiers), 0 errors. Every fix mutation-proofed: revert the fix in a copy → its tests go red, verified per bug. |
| **Ops** | Gates: 24/24 Go packages race-clean, 0 failures / 0 skips, coverage 74.9%→**75.9%** (floor 70.2); web 360/360. Prod untouched; AMS read-only except the teststream restart. CI-promotion date gate still closed (opens 07-23) → skip carry ×11. One PR. |

## (superseded) S21-close header follows

## ⚡ TL;DR — expected from you right now (2026-07-12, SESSION-21 closed ~03:45Z)

> **ONE small new item, at your convenience: start the next session AFTER
> 14:09 today (12:09 UTC)** — that's when your AMS trial lapses. You asked S21
> to close now and continue in a fresh session instead of holding open ~9 h
> (good call — recorded as the operator direction in D-083). The post-expiry
> sweep is fully staged: the sweep tool is committed
> (`qa/realams/harness/expiry-sweep.sh`, validated — its output is
> byte-identical to the baseline run), and your pre-expiry baseline is on disk,
> re-confirmed three times this session. The next session just runs it and
> diffs. **If a session opens before 12:10Z it will wait, not skip.**
>
> **Still waiting on your decision (both non-blocking, unchanged):**
>
> **1. Caddy vhost merge.** `origin/main` still lacks the
> `bedirhandemirel.beyondkaira.com` vhost that live prod Caddy HAS. A redeploy
> from a clean main checkout would drop that site. Say **"merge the caddy
> vhost"** and a session opens the one-commit PR from `caddy-bedirhan-vhost`.
> Until then: branch preserved, on-disk file untouched, your `.bak` untouched.
>
> **2. Final-assessment review.** `docs/assessment/final-assessment.md`
> + `prd-validation-matrix.md` stay **DRAFT**; nothing goes external until you
> reply "approved" or send edits.
>
> **FYI, no action needed:**
> - **BUG-005 is fixed** (the `/qoe/ingest` `interval` parameter — the last of
>   the S18-filed Pulse bugs), test-first, contract unchanged, adversarially
>   verified. Reaches prod with your next approved rollout; prod untouched.
> - **A new test now guards the whole bug class:** every one of the **85**
>   query params your API contract declares must be explicitly accounted for
>   in `param_conformance_test.go` — a declared-but-ignored parameter (the
>   BUG-004/005 pattern) can no longer land silently.
> - **★ That sweep found the class was much bigger: 28 of 85 declared params
>   were not honored.** 1 fixed (BUG-005), 27 pinned visibly with bug docs:
>   **BUG-006** (`limit`/`cursor` pagination dead on 8 list endpoints),
>   **BUG-007** (`cursor` missing where `limit` works), **BUG-008**
>   (`/anomalies` ignores every declared filter), **BUG-009** (`tenant`
>   dropped one layer deeper, inside the query layer), plus **BUG-010**
>   (reverse direction: the audience CSV export `?format=csv` works but was
>   never declared in the contract). Fixing them starts next session — the
>   debt is pinned, not silent.

## 🔎 What SESSION-21 did (2026-07-12, closed — D-083)

| Area | Result |
|---|---|
| **BUG-005 fixed** | `GET /qoe/ingest` now honors the `interval` parameter (hourly/daily buckets); when absent, the fine 60-second buckets your dashboard's "degradation visible in 15 s" promise depends on are preserved. Test-first; the API contract itself unchanged. |
| **Parameter-conformance gate** | A new CI-enforced test loads the OpenAPI contract and requires every declared query parameter (85 today) to be explicitly proven honored, exempted with a reason, or pinned against a bug. The class of bug that produced BUG-004 and BUG-005 can no longer enter the codebase silently. |
| **5 new bugs filed** | BUG-006…BUG-010 (see TL;DR) — found by the conformance sweep and its adversarial verifiers, including one a layer deeper than the original audit looked. All evidence-cited, none fixed this session (scope discipline); they head the S22 backlog. |
| **Post-expiry sweep** | Re-gated to S22 **on your direction** (close now, continue in a new session after the 12:09Z lapse). Zero cost: the sweep tool is committed and validated, and the pre-expiry baseline (Enterprise 3.0.3, build 20260504_1443, 4 apps, all endpoints green) was re-confirmed three times today — the next session's diff will be exact. |
| **Quality net** | 8 workflow agents (2 scouts, designer, 2 TDD authors, 3 adversarial verifiers), 0 errors. Verifier catches applied same-session: an enumeration-floor guard, a misclassified exemption (became BUG-009), and a latent silent-skip path in shared test helpers made loud. |
| **Ops** | Prod and your AMS untouched (read-only). Gates: 24/24 Go packages race-clean, 0 failures / 0 skips, coverage 74.9% (floor 70.2), contract-drift clean. CI-promotion date gate still closed (opens 07-23) → skip carry ×10. One PR. |

## (superseded) S20-close header follows

> **Nothing blocks the work — two things want a decision from you, neither urgent.**
>
> **1. NEW — your other Claude session committed to Pulse's session branch, and
> your prod Caddy config is now ahead of `main`.** At 00:44 tonight a concurrent
> session (the `~/repo/bedo` portfolio work) added a
> `bedirhandemirel.beyondkaira.com` vhost to `deploy/config/Caddyfile.prod` and
> committed it onto whatever branch was checked out — which happened to be this
> session's branch. I inspected it (**clean — no secrets, 35 additive lines, a
> plain TLS reverse-proxy to host:3200**), **preserved it on its own branch
> `caddy-bedirhan-vhost`**, and kept it OUT of the Pulse S20 pull request — a PR
> titled "P0 bug fixes" should not quietly carry an unrelated vhost change.
> **I did NOT revert the file on disk**: that file is what the live
> `pulse-prod-caddy-1` container mounts, so reverting it would have taken
> `bedirhandemirel.beyondkaira.com` down.
>
> **⚠️ The consequence you should know about:** `origin/main` does **not** have
> that vhost block, but **live prod does**. If anyone ever redeploys or reloads
> Caddy from a clean `main` checkout, **`bedirhandemirel.beyondkaira.com` will
> drop off**. The fix is one small PR from the `caddy-bedirhan-vhost` branch.
> **Say "merge the caddy vhost" and a session will open it** (it is your own
> commit — I just won't ship someone else's work through my PR without you
> saying so). Also left untouched: an untracked backup file
> `deploy/config/Caddyfile.prod.bak-bedirhan-20260712` that the other session
> created — yours to keep or delete.
>
> *Process note: this is the second time a concurrent session has committed into
> a Pulse session branch. It is harmless when caught, but if you run two Claude
> sessions in this repo, it helps to give each one its own git worktree.*
>
> **2. STILL OPEN (re-surfaced, non-blocking) — review the final assessment
> draft.** From last session: `docs/assessment/final-assessment.md` (the
> marketplace-readiness report for the Ant Media team) plus its companion
> `docs/assessment/prd-validation-matrix.md` are written and marked **DRAFT**.
> **Nothing goes external until you review them.** Reply "approved", send edits,
> or ignore — they stay internal either way. (One correction landed this session:
> the draft claimed the recording/billing fix needs no schema change; the design
> work showed it needs two — see below.)
>
> **FYI, no action needed:**
> - **Your AMS trial license lapses today at 12:09 UTC.** This session (like the
>   last) ran *before* that moment, so the post-expiry sweep — recording exactly
>   which AMS features start refusing — is the **first thing next session does**.
>   Your pre-expiry baseline is confirmed unchanged (Enterprise 3.0.3, build
>   20260504_1443, 4 apps).
> - **Two Pulse bugs fixed this session** (BUG-004: an API endpoint advertised
>   time-window filters it silently ignored — this was corrupting the real
>   Ingest page's charts, not just tests; BUG-003: the probe scheduler wrote a
>   duplicate result row every 60 s). Both fixed test-first and adversarially
>   verified. They reach prod with your next approved rollout; prod is untouched.
> - **Recording/billing gap (BUG-002) now has a design note** —
>   `docs/assessment/bugs/BUG-002-design-note-vod-rest-poll.md`. Key finding: the
>   fix needs **two additive schema migrations**, not zero as the assessment draft
>   assumed. Nothing is committed to building it; the note is a proposal.
>
> **Still waiting on (all non-blocking, unchanged):** AMS-reset confirmation
> (S17), browser-accept of the re-branded UI, brandkit token proposals, Kafka
> yes/no, Ant Media marketplace contact.

## 🔎 What SESSION-20 did (2026-07-12, closed — D-082)

| Area | Result |
|---|---|
| **BUG-004 fixed** | `GET /qoe/ingest` declared `from`/`to`/`app`/`stream`/`node` filters in its API contract and silently ignored every one of them — so it returned all-time data mixed across eras. **Your Ingest dashboard page was affected** (it asks for the last 15 minutes and was getting everything). Fixed test-first; the API contract itself did not change (the implementation caught up to the spec it already published). |
| **BUG-003 fixed** | The probe scheduler wrote a duplicate result row every 60 s. Root cause was not what the bug report guessed: the runner's 60-second "reload the probe list" loop was **cancelling and restarting every probe's scheduler on every tick**, even when nothing about the probe had changed — and the restarted scheduler fires immediately, landing ~1 ms on top of the original's own tick. Now a probe is only restarted when its config actually changed. First-check-under-100 ms behavior for new probes is preserved. |
| **BUG-002 design note** | The recording/billing gap (`recording_gb` always 0) now has a written design for the VoD REST-poll fix, with a correction to the assessment: it needs two additive migrations (a ClickHouse view to reach the billing rollup, and a table to remember what was already counted). Not built — a proposal for a future session. |
| **Concurrent-session incident** | Your other session's Caddy commit was found on this session's branch, inspected (clean), preserved on `caddy-bedirhan-vhost`, and excluded from the PR. See item 1 above — `main` is now behind live prod Caddy config. |
| **Ops** | Prod and your AMS untouched (read-only checks only). CI-promotion date gate still closed (opens 07-23) → skip carry ×9; the latest main CI/e2e runs are fully green including `csp-e2e` and `web-e2e`. |

## (superseded) S19-close header follows

## ⚡ TL;DR — expected from you at SESSION-19 close (2026-07-11)

> **ONE new action is requested (the first real one in three sessions): review
> the final assessment draft.** Your validation program's end deliverable is
> written: **`docs/assessment/final-assessment.md`** — the marketplace-readiness
> report for the Ant Media team. It is a clearly-marked DRAFT and **nothing
> goes external until you review it.** What to look at:
>
> 1. **Section 1–2:** the headline story — 46/50 scenarios pass, 0 failures;
>    product completeness **79.9% weighted / 60.6% strict**, architecture
>    budgets **91.7%**. Check you're comfortable with these numbers being shown
>    to Ant Media.
> 2. **Section 3:** five rows are **NEEDS-OPERATOR-CONTACT** (marketplace
>    listing requirements, revenue-share, support channel, licensing terms,
>    co-marketing) — they need your Ant Media contact to resolve. The PRD's
>    20–30% revenue-share figure is flagged UNVERIFIED.
> 3. **Section 5:** the proposed roadmap — three P0 items (VoD recording fix,
>    the unsigned-webhook decision D-V2-1 that's been waiting on you, and the
>    BUG-004 API fix). Next session starts fixing the two bug P0s.
> 4. **Section 6:** five open questions for the Ant Media team, ready to send
>    once you approve.
>
> Companion deliverable: **`docs/assessment/prd-validation-matrix.md`** — every
> PRD feature and every numeric budget with verdict + evidence (also DRAFT).
> Reply with edits, "approved", or nothing — it stays internal until you act.
>
> **FYI, no action:** the AMS trial license lapses **tomorrow 2026-07-12 at
> 12:09 UTC** (you said "handled"/observe-report). All validation so far ran
> pre-expiry; the next session opens with a read-only sweep to record what
> changes. Also two new operator docs went live: `docs/beacon-sdk.md` (how
> customers embed the QoE beacon) and expanded AMS-INTEGRATION.md sections on
> the webhook limitation and RTMP stream-end semantics.
>
> **Still waiting on (all non-blocking, unchanged):** AMS-reset confirmation
> (S17), browser-accept of the re-branded UI, brandkit token proposals,
> Kafka yes/no, Ant Media marketplace contact (now item #1 above makes this
> one concrete).

## 🔎 What SESSION-19 did (2026-07-11, closed — D-081)

| Area | Result |
|---|---|
| **PRD validation matrix (Phase 7)** | Every PRD feature (F1–F10) and all 36 architecture budgets now have an evidence-cited verdict. 40 of 66 sub-requirements FULLY validated against your live AMS; the gaps are precisely characterized (4 MISSING, incl. the recording/billing gap; 7 "works differently than the PRD says", each explained). |
| **Final assessment (Phase 8)** | The Ant-Media-facing report DRAFTED (see TL;DR — your review is the gate). Includes a 13-item prioritized roadmap and 5 open questions for the AMS team. |
| **Customer docs (Phase 6)** | 3 highest-priority gaps authored: a complete Beacon SDK integration guide (new `docs/beacon-sdk.md`), the webhook-limitation impact + workarounds, and the RTMP stream-end semantics your S17 drift finding uncovered. |
| **Quality net** | 14 workflow agents; 3 independent adversarial verifiers re-derived every number from primary evidence. They caught 7 real errors before you ever saw the docs — including one citation pointing at a FAILED test run and one fabricated option in a decision writeup. All fixed and re-verified. |
| **Honesty items** | Scores recomputed after fixes (79.9% not 80.4%); a "node up/down alerts" claim downgraded because no direct node-offline test was run; stress claims bounded to the 5-stream VPS capacity. |
| **Ops** | Prod and your AMS untouched (one read-only version check). One PR, docs only. |

## (superseded) S18-close header follows

## ⚡ TL;DR — expected from you right now (2026-07-11, SESSION-18 closed)

> **Nothing is needed from you.** S18 finished the deep-scenario half of your
> validation program: **21 more scenarios PASS / 3 honest skips / 0 failures**
> against your live AMS (viewer ramps, alert firing, beacon QoE parity, failure
> injection, token-gated publish rejection) — and the WebRTC viewer skip from
> S17 was root-caused and fixed, upgrading S17's run to **25/26 green**.
> Total across the program so far: **46 of 50 automated scenarios PASS, 4
> documented skips, 0 parity failures.**
>
> **Two real Pulse bugs were found and filed** (the program working as
> intended): the probe scheduler occasionally writes duplicate result rows
> (BUG-003), and one API endpoint advertises time-window filters it silently
> ignores (BUG-004). Both are documented with evidence for the fix backlog —
> neither affects your prod dashboards' correctness.
>
> **One capacity fact you should know (no action needed):** your AMS VPS
> accepts only ~5–7 simultaneous RTMP streams — beyond that it answers
> "current system resources not enough". The 20-stream stress test therefore
> can't run on this hardware; if you ever want that validated, it needs a
> bigger AMS instance (or the same box with more headroom).
>
> **Still waiting on (all non-blocking):** your confirmation that the AMS app
> reset (16→4 apps, VoDs wiped) was intentional; the browser-accept of the
> re-branded UI; optionals (brandkit token proposals, Kafka yes/no, Ant Media
> marketplace contact — the last one becomes relevant next session when the
> marketplace-readiness report is drafted).

## 🔎 What SESSION-18 did (2026-07-11, closed — D-080)

| Area | Result |
|---|---|
| **P1 scenarios (your program, Phases 3+4)** | 24 new automated scenarios; final **21 PASS / 3 SKIP / 0 FAIL**. Alert rules fire in seconds; beacon QoE numbers match sent data exactly (startup 450 ms, rebuffer ratio 0.2); bitrate-drop degrades health scores as designed; invalid stream keys are rejected with no phantom streams; network-cut publishers recover cleanly. |
| **WebRTC viewer fixed** | S17's skip was a one-line container env bug (invisible because the container ran detached). Now a real headless browser viewer registers on your AMS — S17's TC-V-02 re-ran green. |
| **2 Pulse bugs filed** | BUG-003 probe-scheduler duplicate rows; BUG-004 `/qoe/ingest` ignores its declared `from`/`to` filters. Evidence-complete, ready for a fix session. |
| **AMS behavior documented** | HLS viewer counts are a sliding request-window (≈9× higher than real sessions, >90 s decay — now documented for your customers); RTMP over TCP masks packet loss (loss metrics only meaningful for SRT/WebRTC ingest); app settings change via POST; ~5–7 concurrent RTMP stream capacity on this VPS. |
| **Documentation gap list (Phase 6)** | `docs/assessment/documentation-gaps.md` — 18 gaps, each with target doc + severity + the user question it answers; next session authors the top three. |
| **Quality net** | 13 workflow agents (authors, live debuggers, adversarial verifier); every failure diagnosed to root cause and retested; 2 new shell landmines saved to agent memory. Prod untouched; one PR. |

## (superseded) S17-close header follows

> **Audience: the human operator.** Ledger of record: `ROADMAP.md` §5 + `ROADMAP-V2.md` §4; this
> file is the actionable view, refreshed at every session close. When you finish an item, just
> tell the agent (or do nothing — every session start re-verifies each item automatically).
> **Never commit secret VALUES anywhere; `deploy/.env` and `oguz-testing.md` are gitignored.**

## ⚡ TL;DR — expected from you right now (2026-07-11, SESSION-17 closed)

> **Nothing is needed from you.** S17 executed your validation directive (D-078):
> the real-AMS test harness is BUILT (26 automated parity scenarios under
> `qa/realams/`, rerunnable any time via `make validate-realams-p0`) and the full
> P0 suite ran against your live AMS: **24 PASS / 2 SKIP / 0 FAIL.** Highlights:
> **Pulse saw stream start in 4 s and stream end in 7 s (the PRD promises 10 s)**;
> ingest bitrate parity within ±10%; WebRTC/RTMP/HLS probes green against your
> server; viewer counts matched AMS exactly. The 2 skips are honest "nothing to
> test against" cases, not failures (details in the S17 table below). The suite
> also caught and fixed its own false-green bug before trusting any result —
> every PASS is now backed by a fresh evidence file, never just an exit code.
>
> **One thing to confirm when you have a second (non-blocking):** your AMS's app
> inventory changed since S16 — 16 apps (8 IP-blocked) shrank to **4 apps, all
> open** (`LiveApp`, `WebRTCAppEE`, `live`, `pulse-test`), the old VoDs are gone,
> and `GET /rest/v2/applications/info` now returns HTTP 405. If YOU reset/
> recreated the AMS apps, all good — just say so; if not, tell a session and it
> will investigate. (The `antmedia` container shows a restart ~18 h before S17.)
>
> **FYI, two harmless test artifacts on your AMS:** S17 briefly enabled MP4
> recording on the `pulse-test` app to create ONE small test VoD (~20 s test
> pattern) as ground truth for the recording-gap validation — the setting was
> restored to off; the VoD stays as standing test evidence. Test streams named
> `val-*` may appear/disappear on LiveApp while validation suites run.

## 📣 SESSION-18 load heads-up (2026-07-11, in progress — D-080)

> Per our protocol ("sessions will tell you before any load beyond a handful of
> test streams/viewers"): **S18 runs the 20-concurrent-publisher stress scenario
> (TC-S-01) on your AMS today** — low bitrate (500 kbps each), ~2 minutes, run
> last in the batch, health-monitored, all streams named `val-*` and cleaned up
> after. Also up to ~30 simulated HLS viewers during the viewer-ramp scenario.
> Nothing is needed from you; this is the notify-before-load notice.

## 🔎 What SESSION-17 did (2026-07-11, closed — D-079)

| Area | Result |
|---|---|
| **Validation harness (your D-078 program)** | LANDED: `qa/realams/` — 26 automated scenarios that drive your AMS (real ffmpeg publishers, HLS viewers, headless-browser WebRTC viewers, probes) and cross-check every Pulse number against the AMS REST API. Rerunnable forever: `make validate-realams-p0`. |
| **P0 parity results** | **24 PASS / 2 SKIP / 0 FAIL.** Stream start visible in Pulse in 4 s, stream end in 7 s (PRD: ≤10 s). Bitrate parity ±10%. Viewer counts exact. Probes (WebRTC incl. rtt/jitter/loss, RTMP, HLS, DASH-404) all green live. Fleet card honest (no fake zeros for CPU/mem your AMS doesn't report). |
| **The 2 SKIPs (honest, not failures)** | (1) "IP-blocked app handling" — your AMS currently has no blocked app to test against; (2) "WebRTC viewer count" — the headless browser loaded the player page but playback never started; next session debugs it (needed for deeper WebRTC stats anyway). |
| **Trust hardening** | The suite's first run claimed 21 passes in under 4 minutes — impossible. Root cause found (a script exiting early made "didn't run" look like "passed"); now every PASS requires fresh evidence on disk. Three reusable shell pitfalls saved to agent memory. |
| **Your AMS drifted since last week (please confirm)** | 16 apps → 4 apps (all open), old VoDs gone, `applications/info` endpoint now rejects GET (405), HLS path form changed. All validation docs corrected to match reality. **If this reset wasn't you, tell a session.** |
| **Bugs filed (for the assessment)** | BUG-001: dead code around an AMS statistics endpoint (low). BUG-002: `recording_gb` always 0 because AMS 3.0.3 can't sign the `vodReady` webhook — the recording/billing gap; suggested fix (VoD REST polling) is on the program roadmap. |
| **UI polish (S16 leftovers)** | Info-blue text now theme-aware-ready (6 hardcoded colors → variables), 21 new unit tests (360 total). The light-mode blue + link-color need two tiny brandkit token additions — **proposal awaiting your/designer sign-off** (non-blocking): `agents/handoffs/proposals/D-079-linkbody-token-proposal.md`. |
| **Ship vehicle** | Everything rides ONE PR; prod untouched all session. Gates: lint/types clean, 360/360 unit, coverage up, browser suite 15/15. |

## (superseded) TL;DR at SESSION-16 close

> **Nothing new is needed from you.** S16 landed everything it promised — the **light
> theme + density modes**, the **WebRTC stats columns** on the Probes page, and the
> login-gate bug fix (your prod was never affected) — all gates green (339/339 unit
> tests, 15/15 browser tests, coverage up across the board), one PR, prod untouched.
> **PR #28 MERGED with all 15 CI checks green — including `web-e2e`'s first green in
> 13 runs, the in-CI proof of the login-gate fix.** (This merge-confirmation line rides
> S17's first PR — push budget.)
> **Your new validation directive was received and is now the plan of record (D-078):**
> the full 8-phase Pulse × AMS real-validation & product-fit program is planned under
> `docs/assessment/` and EXECUTION STARTS next session (S17) — building the real test
> environment that drives your AMS (streams up/down, viewers ramping) and
> auto-cross-checks every Pulse number against the AMS APIs. It will need your AMS
> instance healthy and reachable; nothing else from you to start.

**Your open items (unchanged — one):**

| Priority | Item | What to do |
|---|---|---|
| 👀 **finish the eyeball** | Browser-accept the re-branded UI (icon now fixed) | Hard-refresh (Ctrl+Shift+R) https://pulse.beyondkaira.com — log in with the fresh `plt_0352…` token at the BOTTOM of `oguz-testing.md` — check the look + console, then tell a session "UI accepted". **Tip: after S16's PR merges and a future rollout, you'll also get a theme toggle (light/dark) + density switch in the sidebar.** |

## ✅ Key hygiene — DONE (2026-07-11, S16 open, D-077)

You confirmed the private key is stored on your side ("I have stored the file for
myself") → `deploy/.env.bak-d076` was **securely shredded** at S16 open. The private
signing key now exists only in your vault; `deploy/.env` (live prod config with the
minted enterprise LICENSE — not the private key) is untouched and stays gitignored.

## 🔎 What SESSION-16 did (2026-07-11, closed — D-077)

| Area | Result |
|---|---|
| **Key hygiene (your say-so)** | Backup shredded at open — done (above). |
| **CI promotion audit** | Date gate still closed (opens 07-23) — but the audit found the `web-e2e` browser-test job **red for 12 straight runs**, silently (it's non-blocking during its bake period). Root-caused to a real bug, not flakiness. |
| **Real bug found & FIXED** | The SSO login change (S14) made the app treat *any* "200 OK" reply on its session-check as a valid login — even an HTML error/fallback page. In the wrong topology (stale server, misconfigured proxy) you'd see a broken dashboard instead of the login screen. Fixed with tests, **proven in the browser gate: all 15 end-to-end tests green**, including the 3 that had been failing for 12 runs. Your prod was never affected. |
| **Light theme + density (brandkit phase 2)** | LANDED: light/dark toggle (remembers your choice, follows OS preference by default), compact & wall-screen density modes, motion tokens + reduced-motion support. Every light-theme color verified EXACTLY against your brandkit tokens.json; link color follows the WCAG note (§2). |
| **Probes page** | LANDED: WebRTC network-quality columns (ICE state badge + RTT / jitter / loss) — a dash means "not measured", 0 means genuinely measured zero. |
| **Quality net** | 2 implementation workflows + 3 adversarial verifiers (they found 3 real issues — all fixed same-session) + the docker browser gate (found 3 more test bugs — fixed, 15/15). Coverage rose to 65.8/61.1/54.9 (gates 59/54/45). The session also survived a terminal crash mid-work with zero loss (recovered from persisted workflow state). |
| **Ship vehicle** | Everything rides ONE PR (PR-first, ≤2 pushes/session) and reaches prod with the next rollout you approve — **prod is untouched this session.** |

## 🚀 NEW — your validation directive is now the plan of record (D-078, starts S17)

You asked for a **real validation environment**: simulate a real customer using AMS +
Pulse — control AMS (create/start/stop broadcasts, ramp concurrent streams and
viewers, force failures/reconnects) and verify the effects in Pulse, cross-checking
every number against the AMS APIs automatically (mismatch = test FAILS with evidence),
plus the full product-fit ladder up to a marketplace-readiness report for the Ant
Media team. **Planned this session** under `docs/assessment/` (program README,
AMS-capability × Pulse-coverage map, test-environment design, scenario matrix,
session plan). **Execution starts S17.** From you to start: nothing — just keep your
AMS reachable. Heads-up: publisher/viewer load runs will exercise your AMS instance;
sessions will tell you before any load beyond a handful of test streams/viewers.

**New (non-blocking) items the program will eventually want from you:**

| When | Item |
|---|---|
| before S19 | **Ant Media marketplace contact** — the Phase-8 assessment needs the real listing requirements + revenue-share terms (PRD's 20–30% is unverified). When you have a contact/thread with the Ant Media team, tell a session. |
| whenever | **Kafka broker: available or planned?** Standalone AMS 3.0.3 exposes no CPU/mem/disk via REST — without Kafka, Fleet resource gauges stay empty forever. A yes/no shapes the roadmap priority. |
| whenever | **AMS trial license after 07-12** — you said "handled"; if any Enterprise feature does lapse, the validation surface shrinks (sessions will observe + report what 403s). |

**Everything else: nothing needed.** Optionals whenever: D-V2-1 ("build"/"wontfix"),
O7 GHCR-public, O11 rotation, `gh auth refresh -s workflow`.

## ✅ What SESSION-15c did (2026-07-11, D-076b — your two mid-accept reports)

| Area | Result |
|---|---|
| **Broken icon (your report)** | Root cause: the server only served `/assets/*` — every other asset (favicon, icons, manifest, logo) got the HTML page instead. Fixed with a regression test, merged via PR #27 (all 9 required checks), redeployed; `/favicon.svg` → `image/svg+xml` verified live. |
| **Login token (your question)** | Fresh admin token minted and appended to `oguz-testing.md` (bottom); an earlier mint lost to a shell-quoting slip was revoked, not orphaned. Login placeholder corrected `pulse_tok_…` → `plt_…`. |
| **Prod now** | `v0.3.0-4-ge8f8f5f`, healthy, `tier=enterprise`; rollback tags + backup stand. |

## ✅ What SESSION-15b did (2026-07-11, D-076)

| Area | Result |
|---|---|
| **v0.3.0 shipped** | Tagged, released (Trivy/SBOM/cosign), stamped build rolled to prod with migrations; smoke green (health, UI on both domains, webhook fail-closed, clean logs). Rollback image + fresh backup staged first. |
| **Security gate win** | The release pipeline BLOCKED the first tag: a HIGH DoS CVE (go-jose, OIDC stack). Fixed 4.0.5→4.1.4 and re-released — no vulnerable image ever published. |
| **Your license (U3)** | TWO hidden problems found live: (1) the prod compose never passed license env vars to the container (CI-only wiring — fixed); (2) your `.env` held the private signing key, not a license — a proper **enterprise, perpetual** license was minted from it and installed. Verified: `tier=enterprise`, beacon event → 202 accepted → shows up in `/qoe/summary`. QoE/beacon, probes, data API and the anomaly detector are now all live in prod. |
| **CodeQL required** | Your "decide for me" → enabled (29-run green streak, zero maintenance, Go+JS scanning). |
| **PR-first ON** | enforce_admins=true, required reviews 0 (a solo owner can't self-approve); sessions work via PRs from S16 on. |
| **Recorded** | Mobile SDKs deferred (revisit whenever); DASH fixture skipped; push budget: max 2 pushes/session (your directive — saved to agent memory). |

## ✅ What SESSION-15 did (2026-07-10, D-075 — nothing was needed from you)

| Area | Result |
|---|---|
| **WebRTC network-quality stats** | Probes now measure the media path itself: after connecting, they report round-trip time, jitter, and packet loss per probe run. **Verified live against YOUR AMS: rtt 0.47 ms / jitter 22.33 ms / loss 0%, measured from your real teststream's video in 2.2 s.** |
| **Honest data semantics** | A metric that wasn't measured is *absent*, never shown as 0 — so "0% loss" always means genuinely measured zero loss. |
| **CI now exercises the full path** | The CI mock server sends real video packets after connecting, so the whole measurement chain is tested on every PR. |
| **Flaky test hardened** | A pre-existing alert test would have started randomly failing CI under load (it measured scheduler noise, not the behavior it guarded). Caught at this session's gate and rebuilt so it can neither false-fail nor false-pass. |
| **Docs corrected** | An operator-facing doc still claimed the WebRTC/RTMP/DASH probes were stubs — an operator reading it would think their working probes were broken. Fixed, plus ~19 smaller staleness fixes. |
| **Quality net** | 3 workflows (13 agents): scouts → authors (all TDD red→green) → 3 adversarial verifiers (correctness verdict: CONFIRMED-OK, zero functional must-fix) + the live real-AMS run; all CI gates green. |

**Not landed (nothing was due):** the CI-promotions work order stayed date-gated
(opens 2026-07-23 → S16); the v0.3.0 rollout and iOS SDK work orders are waiting on
your answers above; brandkit light-theme moved to S16.

## ✅ What SESSION-14 did (2026-07-10, D-074 — nothing was needed from you)

| Area | Result |
|---|---|
| **WebRTC media-path probe** | WebRTC probes no longer stop at signaling: they now negotiate a real media connection (ICE) and report `ice_state`. **Verified live against YOUR AMS: connected in 0.2 s.** |
| **Real-server bug found & fixed** | The live check exposed a bug CI could never see: your real AMS sends a notification message before the offer, which made every WebRTC probe against a live stream fail. Fixed, pinned by tests captured from your server, and the CI mock now mirrors your AMS exactly. |
| **SSO login UI** | The login screen now shows "Sign in with SSO" when OIDC is configured, and browser sessions from the SSO flow work without pasting a token. Sign-out revokes the SSO session too. |
| **Anomaly alerts** | Anomaly rules can now watch `ingest_bitrate_kbps` and `disk_pct` (was: viewers/CPU/memory only). |
| **Probe safety cap** | Probe segment downloads are capped at 32 MB — a huge/misbehaving segment can no longer produce a silently wrong bitrate or eat unbounded memory. |
| **Your teststream** | The `ams-teststream` publisher container had crashed (2 h earlier); the session restarted it. |
| **Quality net** | 3 workflows (14 agents): scouts → authors → 3 adversarial verifiers + a live cross-pair + the live AMS check; every finding fixed same-session; all CI gates green. |

**Not landed (honestly re-gated to S15 with a written plan):** WebRTC per-stream network
stats (rtt/jitter/loss) — needs mock-side media sending; kept off this push to avoid
landing a fresh flake surface late in a long session.

### 🟢 NEW optional: enable DASH muxing on an AMS app
Your AMS has DASH muxing disabled (verified read-only: `.mpd` → 404), so the DASH probe's
test fixtures are spec-derived rather than captured from your server. Purely optional: if
you enable DASH muxing on any AMS app (AMS panel → app settings → muxing), tell a session
and it will capture a real MPD fixture to pin the parser against your server's exact
output. Nothing is broken without it.

## ✅ AMS trial license expiry (2026-07-12) — operator says handled (2026-07-10)

You said "don't worry about AMS" — recorded as operator-handled/accepted. S13 verified the
real AMS still answers (RTMP handshake + HLS manifest both live-confirmed today). Sessions
keep observing + reporting only.

## ✅ Standing questions — ALL ANSWERED 2026-07-11 (D-076, executing now)

1. **Ship v0.3.0?** → **"proceed"** — rollout in progress this session.
2. **CodeQL required?** → **"decide for me"** → ORCH enabled it (29-run green streak,
   zero maintenance; contexts `Analyze (go)` + `Analyze (javascript-typescript)`).
3. **PR-first?** → **"switch going forward"** — flips at this session's close
   (enforce_admins=true, required reviews 0); sessions work via PRs from S16 on.
4. **Mobile SDKs?** → **"leave out for now, revisit later"** — deferred, work order cut.

## ✅ U3 — DONE (2026-07-11, D-076): you placed the key; the session verifies it live
during the v0.3.0 swap (tier + beacon→QoE chain). Evidence lands in decisions.md D-076.

## 🟢 Optional / your policy call

- ~~DASH muxing~~ — **SKIPPED by you (D-076)**; re-open anytime by enabling DASH muxing and telling a session.
- **O7 — GHCR package visibility** (outsiders-only): the package is private; outside users
  can't `docker pull` or `cosign verify`. Click path: github.com/aytekXR → Packages →
  `ams-pulse` → Package settings → Danger zone → **Change visibility → Public** (UI-only).
- **D-V2-1 — unsigned-webhook ingest mode** (AMS 3.0.3 can't sign hooks): build an optional
  IP-allowlisted unsigned mode, or keep REST-polling-only (current, meets the ≤10 s budget)?
  Reply "build" or "wontfix" whenever; no work happens until you decide.
- **gh `workflow` scope:** the gh token can't update PR branches touching `.github/workflows/*`
  (sessions detour via `@dependabot rebase`). One-time fix: type `! gh auth refresh -s workflow`
  in a session (interactive, ~1 min). Pure convenience.
- **O11 rotation** (if policy demands): api.slack.com/apps → regenerate webhook →
  `gh secret set SLACK_WEBHOOK_URL`. (Exposure was never public; risk-accepted D-066.)

---
*Status snapshot (2026-07-11, S16 close): **prod = v0.3.0-4-ge8f8f5f + ENTERPRISE
license, live + healthy**; QoE/beacon, all four probe protocols (WebRTC through
rtt/jitter/loss), data API and anomaly detection all active in prod. CodeQL required;
PR-first active (9 contexts, enforce_admins). Dependabot queue zero. Go coverage 74.5%
(floor 70.2); web 65.80/61.13/54.85 (gates 59/54/45); sdk 3.52 KB. Plan of record:
`ROADMAP-V2.md` + **`docs/assessment/` (D-078 validation program — S17's primary
track)**; CI-promotion gate opens 07-23 (csp-e2e candidate; web-e2e ~07-25). Your
list: 👀 UI browser-accept + optionals (O7, D-V2-1, O11, workflow-scope).*
