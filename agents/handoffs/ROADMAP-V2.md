# Pulse — v2 (post-GA) ROADMAP

> **Continuation plan** as of 2026-07-09 (D-066, v0.2.0 GA). Seeds the post-GA backlog
> declared in ROADMAP.md §2, plus carry-overs from D-065/D-066. Authorised by SESSION-09 WO-C.
>
> Every session follows ROADMAP.md §6 protocol and §7 standing rules. TDD remains binding.
> The successor session prompt is written at the close of the prior session (ROADMAP.md §6.6).

---

## 0. How to use this file

1. Next session = the lowest-numbered session in §3 not marked ✅. Its ready-to-run prompt is
   `agents/handoffs/sessions/SESSION-NN.md` — start there, not here.
2. This file owns the SEQUENCE and exit criteria. Session prompts own the per-work-order
   detail and TDD plans.
3. When a session completes: mark its §3 entry ✅ with D-0NN + commit refs, update §4/§5
   ledgers, then write the next `SESSION-NN.md`. A session that hasn't written its successor
   prompt is NOT done.
4. Scope changes are edited HERE first, then reflected in the next session prompt.

## 1. Starting state (v0.2.0 GA, 2026-07-09)

| Dimension | State at v2 open |
|---|---|
| Release | v0.2.0 shipped: CI-gated, Trivy, multi-arch amd64+arm64, SBOM+provenance, cosign. **G1 −O7** (GHCR package private — one operator click) |
| Prod | `pulse v0.2.0 (commit 4657512, built 2026-07-09T14:06:07Z)`; healthz ok; cpus=1.0 vCPU (raised D-065 WO-C); B7 live; honest QoE live |
| Coverage | Go 73.2% / floor 70.2; web 76/72/45; sdk 62/73/70 |
| Branch protection | main protected (API 200); `enforce_admins=false` (owner pushes directly during GA sprint — revisit S10) |
| License | PolyForm NC 1.0.0 (root) + MIT (SDK). docs/licensing.md complete. G7 fully met |
| CI promotions | web-e2e + csp-e2e advisory (`continue-on-error`); streaks 7/7 at 2026-07-09; date-gate ≥2026-07-23 |
| Dependabot | 20 PRs open post-D-066; #4 closed (golang 1.26 vs D-032 pin); S9 WO-B absorbs in 3 verified batches |
| AMS ingest | REST polling only (≤5 s); AMS 3.0.3 lifecycle hooks unsigned (O3 N/A, D-066) |
| Known hot path | O(N²) `rebuildSnapshot` at poll boundaries; mitigated to 1.0 vCPU (D-065 WO-C); real fix is post-GA backlog |
| Open operator items | O7 (GHCR public), U3 (Pro+ license key — optional QoE unlock) |

## 2. Backlog

All post-GA items. Roughly ascending delivery complexity. Each item notes its source and size.

### 2.1  enforce_admins=true revisit  [XS]

**Why:** Branch protection was set with `enforce_admins=false` so that sessions (running as
the repo owner) could push directly to main without going through PR-CI. That was the right
call during the GA push. Now that v0.2.0 is shipped and the dev cadence normalises to PRs,
flipping this closes the owner-bypass gap.
**Source:** D-065 WO-E not-due note; SESSION-09 WO-C spec.
**Due:** S10 — flip once sessions stop pushing directly to main, or record the explicit reason
it stays off (e.g. orchestrator still batches docs commits).
**Action:** `gh api PATCH /repos/aytekXR/ams-pulse/branches/main/protection` with
`enforce_admins: true`; verify via GET (`enforce_admins.enabled = true`). Or commit a prose
rationale to decisions.md if deferring further.
**RESOLVED S10 (D-068, 2026-07-09): stays `false` — rationale committed, revisit re-armed.**
Sessions (running as the repo owner) still push directly to `main` per the binding
Verify→Commit→Handoff flow (RESUME-PROMPT §11), and the protection requires 1 approving
review while the repo has a single human collaborator: GitHub forbids self-approval, so
`enforce_admins=true` today would deadlock ALL session pushes (no one can approve the PR).
Flip becomes possible when EITHER (a) a second trusted reviewer/bot-approver exists, or
(b) the operator drops `required_approving_review_count` to 0 and sessions move to a
PR-first cadence (PR-CI still gates via the 7 required contexts). Operator question filed
in docs/operator-expected.md; next revisit: S12 or on operator request, whichever first.

---

### 2.2  keep-7 backup cycle-8 pruning verification  [XS]

**Why:** The backup sidecar implements keep-7 retention. Cycle 8 is the first run where an
old backup should actually be pruned — this has never been exercised on a real clock, only
by unit tests and the S1 staging smoke. A silent failure would accumulate backups
indefinitely on the VPS disk.
**Source:** S1 verification scope (backup cycle N≥2 + keep-7 verified); SESSION-09 WO-C spec.
**Trigger:** ~2026-07-16 (GA date 2026-07-09 + 8 daily cycles).
**Action:** SSH to VPS; list `/backups/pulse/` before and after the cycle-8 run; confirm
(a) the oldest entry is pruned and (b) the total count remains ≤7. Record result in
decisions.md D-067 or the S10 decisions entry.

---

### 2.3  qa/licensegen -privkey/-expires flags  [S]

**Why:** `docs/licensing.md §2.1` documents that `qa/licensegen` will accept a `-privkey
<path>` flag (to use the vendor's production ed25519 key pair instead of the embedded dev
key) and an `-expires <days>` flag (for time-boxed licenses). Without these flags the tool
is dev/test-only; there is no supported code path for the vendor to mint production Pro+
license keys. D-066 noted "licensegen -privkey extension = S9 WO" but the flag was not
implemented during S8/S9 WO-C (this document is that WO-C's output).
**Source:** docs/licensing.md §2.1; D-066 release decision ("licensegen -privkey extension
= S9 WO"); SESSION-09 WO-C spec.
**Action:** TDD — red tests on `-privkey <path>` + `-expires <days>` flag parsing and key
signing; implement; green; the vendor key ceremony (generate prod ed25519 key pair, sign a
test key, verify Pulse accepts it with `PULSE_LICENSE_PUBKEY` env swap) is a manual QA step
to be documented in docs/licensing.md §3.
**Size:** [S] — flag parsing + ed25519 key-load plumbing; no new dependencies beyond those
already used by the license package.

---

### 2.4  Dependabot steady-state policy  [XS]

**Why:** After S9 WO-B absorbs the 20 open PRs, Dependabot will keep opening new ones.
Without a documented policy the operator faces an unbounded inbox with no guidance on merge
cadence, auto-merge eligibility, or when to defer. The current ad-hoc approach (batch
reviews in sessions) doesn't scale once GA traffic picks up.
**Source:** D-066 O8 decision + SESSION-09 WO-B spec.
**Deliverable:** A short policy section in `docs/ARCHITECTURE.md` §7 (or new
`docs/dependabot-policy.md`) covering: (a) digest/patch bumps → approve + squash-merge on
green CI, within 1 week; (b) minor bumps → review within 2 weeks, confirm no API breaks;
(c) major bumps → explicit session WO with TDD; (d) golang version bumps → blocked by the
D-032 pin (review when the pin is lifted, not before).

---

### 2.5  O(N²) rebuildSnapshot hot path  [M]

**Why:** During D-065 WO-C the per-stream `ingest: health degraded` log storm was fixed by
rate-limiting to one aggregated INFO/tick. The CPU cap was RAISED 0.5→1.0 vCPU in the same
commit as a mitigation, not a fix. The evidence memo (D-065 WO-C): "poll-boundary O(N²)
`rebuildSnapshot` bursts hit 147% of a core; CFS at 0.5 = up to ~65 ms goroutine freezes
per 100 ms period with UNKNOWN P99 (9 ms avg masks it); host nproc=6 so 1.0 = 16.7% of
host." At 1k+ streams the current mitigation will not hold. Raising the cap further is not
the right answer — it needs an algorithmic fix.
**Source:** D-065 WO-C evidence memo; ARCHITECTURE.md §4 (A10 500-stream numbers); SESSION-09
WO-C spec.
**Action:** Profile `rebuildSnapshot` under the 500-stream A10 fixture to isolate the N²
factor; redesign to incremental/delta update (O(N) amortised — e.g. maintain a delta set
rather than rebuilding from the full stream list); benchmark comparison before/after at 100,
500, and 1k synthetic streams; TDD regression test asserting the fix under the same 500-
stream fixture. Update ARCHITECTURE.md §4 with the new measured numbers.
**Size:** [M] — profiling + algorithmic redesign + benchmark harness + TDD.

---

### 2.6  Optional unsigned-webhook ingest mode with IP allowlist  [DECISION FIRST — see §4]

**Why:** AMS 3.0.3 lifecycle hooks are unsigned. D-066 O3 verified this live (182 settings,
no HMAC field). Pulse's fail-closed HMAC listener rejects all unsigned hooks, making AMS-
initiated push events impossible. REST polling (≤5 s) is the current workaround and meets
the PRD ≤10 s budget. An OPTIONAL mode — enabled by explicit operator config, IP allowlist
required — would let AMS push lifecycle events without a shared HMAC secret, trading
cryptographic integrity for network-layer trust (i.e. the AMS host IP is trusted).
**Source:** D-066 O3 N/A decision ("Optional post-GA WO seeded: unsigned-ingest mode w/ IP
allowlist — operator product call"); SESSION-09 WO-C spec.
**Status:** **OPERATOR DECISION FIRST** (§4 D-V2-1). Do not design or build before the
operator makes a build-vs-wontfix call. REST polling is correct and complete; this is a
pure product decision on the risk/convenience trade-off.
**If build decision:** [S] — new env var `PULSE_WEBHOOK_ALLOW_UNSIGNED_SOURCES` (comma-
separated CIDR allowlist); listener branches on presence of HMAC header; source IP checked
against CIDRs via `net.ParseCIDR`; TDD: valid-IP-unsigned allowed, invalid-IP-unsigned 403,
signed path unchanged, no regression to the B7 per-source secret path.

---

### 2.7  CI job promotions  [S] ⏳ date-gated ≥2026-07-23

**Why:** `web-e2e` and `csp-e2e` have run as advisory (`continue-on-error`) since S4
(2026-07-09). The 2-week bake clock (restarted 2026-07-09 after the `ba56c6e` spec-gating
red that made the streak deterministic) expires ~2026-07-23. Promoting them to required
contexts prevents any merge from silently breaking the CSP or the E2E flow. CodeQL is a
separate decision: the repo went public (D-062), CodeQL runs, streak is green, but GHAS
considerations mean explicit operator OK is required before making it a required context.
**Source:** S4 result + S8 WO-E not-due record + D-065; SESSION-09 WO-A.
**Note:** If NOT executed in S9 (date gate still closed), this becomes S10 WO-F.
**Action:**
- Re-measure job-level streaks at execution time (`gh api .../runs/<id>/jobs` — job-level,
  not workflow-level, because `continue-on-error` makes the workflow lie).
- FULL-LIST PUT (a partial list silently de-requires the rest): contracts, server, web, sdk,
  docker-build, helm, compose **+ web-e2e + csp-e2e**; GET-diff proof after.
- Drop `continue-on-error` from both jobs; actionlint; reproduce touched ci.yml steps.
- CodeQL: promote ONLY with explicit operator OK (§4 D-V2-2). Streak evidence to be shared
  with the operator at that session.

---

### 2.8  Anomaly expansion  [M] ✅ DONE S11 (D-070, 2026-07-10)

> Delivered: rule_type `anomaly` (contract CR-1 + migration 0002), z-score eval off the
> Welford Detector baselines for viewer_count/cpu_pct/mem_pct, UI rule builder, e2e A5,
> numeric target (≤50 ms/5 s tick @500 streams) in ARCHITECTURE §4. Follow-up: §2.14
> (Detector metric expansion — e.g. ingest_bitrate_kbps).

**Why:** Current alerting is threshold-based (operator-defined rules with numeric conditions).
At GA, the evaluator reads real `rollup_qoe_1h` data (G6). Anomaly detection would
automatically flag deviations from a per-stream baseline without requiring manual threshold
configuration — a key capability for high-stream-count deployments where per-stream rule
authoring is impractical.
**Source:** ROADMAP.md §2 post-GA backlog.
**Scope:** Rolling baseline (mean + σ) computed over a configurable lookback window in CH;
new alert rule type `anomaly` in the OpenAPI contract + alert engine; UI rule builder for
anomaly rules; PRD §7 does not specify a numeric latency target for anomaly evaluation —
define one at scoping time and add to ARCHITECTURE.md §4.
**Size:** [M] — contract CR + CH aggregation query + alert engine extension + UI + TDD.
Likely touches: `contracts/api/pulse-api.yaml`, `server/internal/alert/`, `server/internal/
query/`, `web/src/`.

---

### 2.9  White-label PDF logo  [XS] ✅ DONE S11 (D-070, 2026-07-10)

**Why:** Report exports currently embed the default Pulse wordmark. Multi-tenant and OEM
deployments need to substitute their own branding without rebuilding the binary.
**Source:** ROADMAP.md §2 post-GA backlog.
**Action:** `PULSE_REPORT_LOGO_PATH` env var; `reports/` package reads the file at PDF
generation time with fallback to the embedded default asset; path validated at boot (log
WARN if set but not readable, do not crash); TDD: fallback path returns default bytes,
override path returns file bytes, missing file does not crash.
**Size:** [XS] — env var plumbing + file-read + fallback; touches only `server/internal/
reports/` and boot validation in `cmd/pulse/serve.go`.

---

### 2.10  SSO / OIDC  [L] — ✅ PHASE 1 (server) DONE S11 (D-070); phase 2 = UI login flow

> Phase-1 limitation (documented): the OIDC session cookie authenticates API calls, but the
> SPA AuthGate still reads localStorage — after OIDC login the UI still shows the token
> gate. Phase 2 (S13+): login button + cookie-aware AuthGate + logout UI.

**Why:** Enterprise operators need single sign-on. Pulse currently manages its own user table
with bcrypt passwords and local sessions. SSO/OIDC enables Okta, Entra, and Google Workspace
auth without local credential management — a prerequisite for multi-tenant and regulated
deployments.
**Source:** ROADMAP.md §2 post-GA backlog.
**Scope:** OIDC provider config (issuer, client ID/secret via env vars); `/auth/oidc/callback`
handler; session token issuance re-using existing JWT machinery; group → role mapping;
TDD with a mock OIDC server. UI login flow change. Contract CR for the new auth endpoints.
**Size:** [L] — likely a full session; contract CR + multiple server handlers + UI changes +
TDD.

---

### 2.11  Native WebRTC / RTMP / DASH probes  [L per protocol]

**Why:** Current QoE probes are HLS-only; non-HLS streams return `not_probed` (stub from
ROADMAP.md §1 audit). AMS supports WebRTC, RTMP, and DASH. Full QoE measurement requires
probing across all delivery protocols. This directly affects the accuracy of the anomaly
expansion (§2.8) for non-HLS streams.
**Source:** ROADMAP.md §2 post-GA backlog; §1 stubs note ("probes non-HLS = not_probed").
**Approach:** One protocol per session WO to manage scope: WebRTC first (headless browser
or native WebRTC stack), then RTMP, then DASH. Each protocol adds: probe implementation,
probe result schema extension (contract CR), CI fixture from `real-ams-captures/`.
**Size:** [L] per protocol.

---

### 2.12  Mobile SDKs  [L per platform]

**Why:** `sdk/beacon-js` covers browser clients. Native mobile apps have no supported SDK.
Mobile QoE data (viewer sessions on iOS/Android apps using AMS streams) cannot currently
reach Pulse.
**Source:** ROADMAP.md §2 post-GA backlog.
**Scope:** At minimum, a Swift package (iOS) and a Kotlin library (Android), each
implementing the same beacon REST contract as `sdk/beacon-js`. Size gate analogous to the
JS SDK 15 KB gate (define per platform at scoping). Share the contract spec; do not diverge
from the JS beacon schema.
**Size:** [L] per platform.

---

### 2.13  Postgres meta backend (HA)  [L]

**Why:** The meta store is SQLite (single-file, single-writer). This works for single-node
deployments and remains the default. A Postgres backend enables HA configurations (active
primary + standby, connection pooling, managed database services) without changing the
application layer above the `store/meta` interface.
**Source:** ROADMAP.md §2 post-GA backlog.
**Scope:** New `store/meta/postgres` implementation satisfying the same interface as
`store/meta/sqlite`; migration runner parity; connection pool config; TDD with a Postgres
test container (CI integration tag). SQLite default is NOT deprecated.
**Size:** [L] — likely a full session; interface implementation + migration parity + CI
integration test.

---

### 2.14  Anomaly Detector metric expansion  [S]  (NEW, seeded by S11 WO-B)

**Why:** Anomaly alert rules (§2.8) support exactly the metrics the Welford Detector
baselines: `viewers`, `cpu_pct`, `mem_pct`. Rules on `ingest_bitrate_kbps` (or QoE metrics)
are rejected 400 because no baseline would ever exist — extending `UpdateBaselines`
(`server/internal/anomaly/anomaly.go`) adds them. ⚠ `server/internal/anomaly/` has NO
manifest owner — ORCH must assign scope first (flagged D-070).
**Action:** add bitrate (and candidate QoE) observations to the Detector; widen
`ValidateAnomalyRule`'s supported set + UI metric list; extend e2e A5 or add a unit-level
equivalence; keep window semantics aligned with the Detector's windowS.
**Size:** [S].

---

## 3. Sessions

S9 is already scoped — see `agents/handoffs/sessions/SESSION-09.md`. Entries from S10 onward
are rough plans; each session writes the full `SESSION-NN.md` prompt from this section at the
prior session's close.

Sizing: one session ≈ one prior GA-sprint session (D-055 scale) — a Workflow of ~10–20
agents + gates + handoff, survivable within a usage-limited session.

---

### S9 — post-GA: promotions + dependabot absorption + ROADMAP-v2 ✅ DONE (D-067, 2026-07-09)
**Result:** dependabot queue CLOSED (20+1 PRs; co-upgrade clusters landed as units); release
dry-run proven (run 29028802644); digests staged + prod-refreshed; coverage gates re-baselined
under vitest 4 (web 59/54/45, sdk 63/43/67); promotions date-gated → S10 WO-F; this plan seeded.
Prompt: `agents/handoffs/sessions/SESSION-09.md`
See SESSION-09.md for WO-A (CI promotions, date-gated), WO-B (dependabot 20 PRs), WO-C
(this seeding), WO-D (conditional operator triggers: U3, O7, O11).

---

### S10 — housekeeping + O(N²) fix + licensegen flags ✅ DONE (D-068, 2026-07-09)
**Result:** WO-A rationale committed (enforce_admins stays false — self-approval deadlock;
re-arm S12/operator); WO-C licensegen -privkey/-expires TDD-green + licensing.md §3 (minting
self-serve); WO-D O(N²) rebuildSnapshot → O(1) incremental deltas (~688× @1k, linear ratios
5.4×/2.1×, allocs/event 1021→1, equivalence+alloc guards; cap reverted 0.5/500m + goldens);
WO-E docs/dependabot-policy.md; WO-B (≥07-16) + WO-F (≥07-23) date-gated → S11 WO-D/WO-E.
Commits: 03f9965 / 2d475a2 / 760eda9 + close. Prompt: `sessions/SESSION-10.md`.

**Goal:** Close the maintenance tail left open at GA; fix the rebuildSnapshot algorithmic
problem before stream counts grow; enable production license key minting.

1. **WO-A [XS]** `enforce_admins=true` revisit (§2.1) — flip or commit rationale; overdue
   since GA.
2. **WO-B [XS]** keep-7 cycle-8 verification (§2.2) — SSH check; trigger ~2026-07-16;
   execute first S10 run after that date.
3. **WO-C [S]** `qa/licensegen` `-privkey`/`-expires` flags (§2.3) — TDD red→green; update
   docs/licensing.md §3 with vendor key ceremony steps.
4. **WO-D [M]** O(N²) rebuildSnapshot fix (§2.5) — profile → redesign → benchmark at 100/
   500/1k streams → TDD regression; update ARCHITECTURE.md §4 numbers.
5. **WO-E [XS]** Dependabot steady-state policy (§2.4) — post-S9-absorption write-up.
6. **WO-F [S, time-gated]** CI promotions carry-over (§2.7) — only if NOT completed in S9;
   same spec as S9 WO-A; re-measure streaks first.

**Exit:** enforce_admins flipped (or rationale committed); cycle-8 pruning observed and
recorded; licensegen flags TDD-green; rebuildSnapshot benchmark shows O(N) or flat on
500-stream fixture; dependabot policy committed; CI promotions landed or re-deferred with
next gate date.

---

### S11 — polish + anomaly expansion + SSO/OIDC phase 1 ✅ DONE (D-070, 2026-07-10)
**Result:** WO-A PDF logo TDD-green (9 tests incl. garbage-content pin; poppler-validated);
WO-B anomaly rule type end-to-end (contract CR-1 + migration 0002 + engine z-score eval for
viewer_count/cpu_pct/mem_pct + UI + e2e A5 under mock w/ PULSE_ANOMALY_TICK_S=5; ≤50 ms/tick
@500 streams target in ARCHITECTURE §4); WO-C OIDC phase 1 (contract CR-2, PKCE S256,
HMAC state+nonce cookie, fail-closed group→role, api_tokens sessions + pulse_session cookie,
27 tests; UI = phase 2); WO-F(D-069) SPLIT: 6 statically-verified install.md bugs FIXED,
empirical release test BLOCKED on operator (O7/read:packages) → S12; WO-D/WO-E date-gate
skips recorded (backup vol at 7/7 — prune verifiable from ~07-10). 2 workflows (4 scouts;
10 agents incl. 3 adversarial verifiers — verdicts C/PARTIAL/PARTIAL, all 4 findings fixed
same session incl. a D-028 silent-skip false-green). Go 73.9% / web gates green.
Commits: b9d96ff…9d4b8d3 (9). Prompt: `sessions/SESSION-11.md`.

**Goal (as planned):** Operator-visible feature additions on the stable GA base.

1. **WO-A [XS]** White-label PDF logo (§2.9) — `PULSE_REPORT_LOGO_PATH`; TDD; boot
   validation.
2. **WO-B [M]** Anomaly expansion (§2.8) — contract CR → CH aggregation → alert engine →
   UI rule builder → TDD. Define PRD numeric target at scoping.
3. **WO-C [L]** SSO/OIDC phase 1 (§2.10) — OIDC provider config + callback handler +
   session issuance; TDD with mock OIDC server. UI login flow change deferred to phase 2.

**Exit:** PDF logo env var TDD-green + boot-validated; anomaly rule type e2e-proven in CI
under CHF mock; OIDC login round-trip proven in CI with mock server.

---

### S12 — infrastructure scaling: Postgres meta backend + WebRTC probe (+ S11 carries)
**Goal:** Unlock HA deployments; extend probe coverage beyond HLS; drain the carry queue.

1. **WO-A [L]** Postgres meta backend (§2.13) — `store/meta/postgres` + migration parity +
   CI integration test; `PULSE_META_BACKEND=postgres` env gate; SQLite default unchanged.
2. **WO-B [L]** WebRTC probe phase 1 (§2.11) — headless-browser probe implementation; CI
   fixture from `real-ams-captures/`; contract CR for extended probe result schema.
3. **WO-C [XS, carry]** keep-7 backup cycle-8 pruning check (§2.2) — boundary REACHED:
   volume held 7/7 on 2026-07-09; first prune expected ~2026-07-10 cycle. Verify oldest
   (pulse-20260707-073113) pruned + count ≤7 + restore-verify green.
4. **WO-D [S, date-gated ≥2026-07-23]** CI promotions (§2.7) — unchanged spec; check
   docs/operator-expected.md for the CodeQL answer first.
5. **WO-E [M, operator-gated]** WO-F clean-install RELEASE test carry — execute the moment
   O7 (or `gh auth refresh -s read:packages`) lands; full runnable step list preserved in
   the S11 scout report (D-070) + SESSION-12 prompt. ⚠ needs a valid AMS license
   (trial expires 2026-07-12).
6. **WO-F [XS]** enforce_admins re-arm (§2.1 / D-V2-3) — flip if operator said "PR-first",
   else re-record rationale.

**Exit:** `PULSE_META_BACKEND=postgres` boots and passes migration parity tests in CI;
WebRTC probe returns a real result (not `not_probed`) for a WebRTC stream in CI; carries
executed or re-gated with evidence.

---

### S13 — probes phase 2 + iOS SDK phase 1
**Goal:** Complete RTMP/DASH protocol coverage; begin mobile SDK delivery.

1. **WO-A [M each]** RTMP probe + DASH probe (§2.11, phase 2) — one WO per protocol; CI
   fixtures required for each.
2. **WO-B [L]** iOS beacon SDK (§2.12, phase 1) — Swift package; beacon REST parity with
   `sdk/beacon-js`; size gate defined and enforced in CI.

*(S14+ planned from this roadmap at S13 close: Android SDK, SSO/OIDC phase 2, anomaly tuning.)*

---

## 4. Operator decision ledger

> Items the operator must decide before the agent can act. Surface every session.
> Counterpart to ROADMAP.md §5.

| # | Decision | Status | Notes |
|---|---|---|---|
| D-V2-1 | **Unsigned-webhook ingest (§2.6):** build an optional IP-allowlisted unsigned mode vs keep REST-polling-only | **OPEN** | O3 closed-N/A (D-066): AMS 3.0.3 hooks unsigned — verified live. No build commitment. Agent awaits "build" or "wontfix". |
| D-V2-2 | **CodeQL as required CI context:** promote CodeQL to a required branch-protection context | **OPEN — operator OK needed** | Streak green since D-062 (O9 closed). Evidence ready to share. Needs explicit OK given GHAS nuances even on the now-public repo. |
| D-V2-3 | **enforce_admins flip (§2.1):** flip `enforce_admins` to `true` once sessions stop pushing directly to main | **RESOLVED-DEFERRED (D-068, S10)** | Stays `false`: 1-review requirement + solo owner = self-approval impossible → flip would deadlock session pushes (§2.1 rationale). Re-arm: S12, or operator says "PR-first" (then drop reviews to 0 or add a reviewer). |
| D-V2-4 | **U3 — activate Pro+ license in prod:** set `PULSE_LICENSE_KEY` in `deploy/.env` | **OPEN — optional feature unlock** | Until then QoE/beacon data does not flow in prod; CI covers it with the mock license (G6 met). Minting instructions in docs/licensing.md. |
| D-V2-5 | **O7 — GHCR package public:** make `ghcr.io/aytekxr/ams-pulse` public | **DOWNGRADED to optional (2026-07-10)** | Operator granted `read:packages` instead → S12 WO-E unblocked (pull + cosign verified live: image tag `0.2.0` — NO v prefix, doc bug fixed; Rekor 2128354996). Package stays private: only outside users can't pull/verify until the one UI click (no API path, D-066). |

---

## 5. Coverage ratchet (carry-forward from ROADMAP.md)

| When | Go total | CI floor | Web lines / branches / functions | Notes |
|---|---|---|---|---|
| 2026-07-09 GA (v0.2.0, D-065) | **73.2%** | **70.2** | 76 / 72 / 45 | Baseline for v2 plan; floor = achieved−3 per GA rule |
| 2026-07-09 S10 (D-068) | **73.5%** | **70.2** | 62.13 / 57.6 / 51 (gates 59/54/45) | Web numbers = vitest-4 re-baseline (D-067); sdk 66.06/45.79/70.42 (gates 63/43/67) |
| 2026-07-10 S11 (D-070) | **73.9%** | **70.2** | 79.69 / 76.25 / 47.33 (gates 59/54/45) | api 76.1, reports 90.1, query 87.5, meta 67.7; sdk untouched (66.06/45.79/70.42, 3.52 KB) |
| *(update each session at close)* | | | | |
