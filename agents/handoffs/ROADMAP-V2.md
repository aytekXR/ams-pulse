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

### 2.2  keep-7 backup cycle-8 pruning verification  [XS] ✅ DONE S12 (D-072, 2026-07-10: cycle-8 prune observed live — both 07-07 artifacts removed, 7/7 kept; CH RESTORE-verify green (613,939 server_events rows via `RESTORE DATABASE pulse AS pulse_restore_verify`); meta integrity_check ok)

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

### 2.3  qa/licensegen -privkey/-expires flags  [S] ✅ DONE (ledger corrected S34/D-096: the flags were built and never ticked)

**Ledger correction (S34, 2026-07-14).** This entry was carried as OPEN across every session
handoff since S9, but the work is finished. `qa/licensegen/main.go` exposes `-privkey` (line 92),
`-expires` (line 93) AND a third flag the roadmap never asked for, `-expires-minutes` (line 94,
for live trial-flow demos). `flag.Visit` enforces `-expires`/`-expires-minutes` mutual exclusion.
Verified S34: `go test ./qa/licensegen/...` → ok (8.6s), and `go run . -h` lists all four flags.
The tests cover hex-length validation, signature verification under a supplied privkey, and the
`-expires-minutes` signature path (`TestExpiresMinutesSignatureVerifies`).
**What genuinely remains is NOT code — it is the vendor key ceremony** (generate the production
ed25519 pair, sign a real key, verify Pulse accepts it under a `PULSE_LICENSE_PUBKEY` swap). That
is an operator action and is tracked as such in `docs/operator-expected.md` (trial-key mint), not
here.
**Gotcha for future sessions:** the Go toolchain is NOT on the default PATH on this host. The only
copy is pre-commit's:
`export PATH="/home/aytek/.cache/pre-commit/repoiavouv2x/golangenv-default/.go/bin:$PATH"` (go1.26.5).

**Why (original):** `docs/licensing.md §2.1` documents that `qa/licensegen` will accept a `-privkey
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

### 2.4  Dependabot steady-state policy  [XS] ✅ DONE S9 (ledger corrected S26/D-088: `docs/dependabot-policy.md` — 208 lines, S9 WO-E — already covers all four deliverable items incl. the D-032 golang pin; this entry had simply never been marked)

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

### 2.5  O(N²) rebuildSnapshot hot path  [M] ✅ DONE S10 (D-068, commit 2d475a2 — O(N²) rebuildSnapshot → O(1) incremental snapshot deltas; retained rebuildSnapshot only on non-hot paths, aggregator.go:587 comment). Ledger drift, 2nd of its class after §2.4.

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

### 2.10  SSO / OIDC  [L] — ✅ PHASE 1 (server) DONE S11 (D-070) · ✅ PHASE 2 (SPA login) DONE S14 (D-074, 2026-07-10: /auth/oidc/status + /auth/me, AuthGate cookie-session path + SSO button, OIDC logout wired; bearer flows unchanged)

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

### 2.11  Native WebRTC / RTMP / DASH probes  [L per protocol] — ⚙ WebRTC PHASE 1 ✅ S12 (D-072) · RTMP PHASE 1 (handshake) + DASH (full MPD+segment) ✅ S13 (D-073) · **WebRTC PHASE 2a (pion ICE) ✅ S14 (D-074, 2026-07-10: ice_state connected|failed|timeout + CH 0007, live-verified ICE-connected vs real AMS 3.0.3 in 0.2s; PLUS the notification-skip signaling fix — real AMS sends subtrackAdded BEFORE the offer, the D-072 parse failed live-only, now fixed + mock mirrors it)** · **phase-2b (rtt/jitter/loss) ✅ S15 (D-075, 2026-07-10: rtt_ms/jitter_ms/loss_pct Nullable(Float32) CH 0008, key-absent semantics, ~2s ctx-bounded post-connect hold, pc.GetStats(); live vs real AMS 3.0.3: rtt 0.47 ms / jitter 22.33 ms / loss 0 in 2.2 s; remaining F10 tail = RTMP AMF0 connect + probe-stats UI surface)** · **★ F10 TAIL DONE ✅ S29 (D-091, 2026-07-14): RTMP AMF0 connect round-trip (hand-rolled AMF0 + minimal chunk demuxer honoring SetChunkSize; app_accepted/app_rejected/rtmp_connect_timeout; description-only contract CR; live vs real AMS 3.0.3: app_accepted + 281-byte wire fixture committed) + probe-stats UI completed (ProbesPage Signaling badge + Connect ms columns — the S15-noted gap). §2.11 protocol matrix COMPLETE for implemented scopes.**

**Why:** Current QoE probes are HLS-only; non-HLS streams return `not_probed` (stub from
ROADMAP.md §1 audit). AMS supports WebRTC, RTMP, and DASH. Full QoE measurement requires
probing across all delivery protocols. This directly affects the accuracy of the anomaly
expansion (§2.8) for non-HLS streams.
**Source:** ROADMAP.md §2 post-GA backlog; §1 stubs note ("probes non-HLS = not_probed").
**Approach:** One protocol per session WO to manage scope: WebRTC first (headless browser
or native WebRTC stack), then RTMP, then DASH. Each protocol adds: probe implementation,
probe result schema extension (contract CR), CI fixture from `real-ams-captures/`.
**Size:** [L] per protocol.
**⚠ RE-SCOPED at S12 (D-072 ruling, scout-verified):** headless-browser probing is
REJECTED outright (violates the single-binary CGO=0 deployment model); WebRTC lands in
two slices instead: **phase 1 (S12) = signaling-only** — pure-Go WS dial → `play` →
offer, real `connect_time_ms` + `signaling_state`, fixture self-captured from the real
AMS, [M] — and **phase 2 (S13) = pion media path** (ICE/DTLS/SRTP, rtt/jitter/loss,
[L], new pion deps). RTMP/DASH sizing unchanged.

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

### 2.13  Postgres meta backend (HA)  [L] ✅ DONE S12 (D-072, 2026-07-10: pgx/v5, rebind, embedded PG DDL parity, 19 integration tests green in CI vs postgres:16; SQLite default unchanged)

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

### 2.14  Anomaly Detector metric expansion  [S]  (NEW, seeded by S11 WO-B) — ✅ DONE S14 (D-074, 2026-07-10: +ingest_bitrate_kbps (stream) + disk_pct (node), all 5 whitelist copies atomic, e2e A5b; beacon QoE + viewer_* EXCLUDED w/ reason — U3 gate / sparsity — revisit post-U3)

**Why:** Anomaly alert rules (§2.8) support exactly the metrics the Welford Detector
baselines: `viewers`, `cpu_pct`, `mem_pct`. Rules on `ingest_bitrate_kbps` (or QoE metrics)
are rejected 400 because no baseline would ever exist — extending `UpdateBaselines`
(`server/internal/anomaly/anomaly.go`) adds them. ⚠ `server/internal/anomaly/` has NO
manifest owner — ORCH must assign scope first (flagged D-070).
**Action:** add bitrate (and candidate QoE) observations to the Detector; widen
`ValidateAnomalyRule`'s supported set + UI metric list; extend e2e A5 or add a unit-level
equivalence; keep window semantics aligned with the Detector's windowS.
**Size:** [S].

**S25/D-087 assessed: SPARSITY GATE** — prod `beacon_events` = 2 rows / 1 stream
(`u3-smoke` smoke test, 2026-07-10); `realams` = 0 rows; all-zero baselines ⇒
epsilon-floor makes the FIRST real rebuffer event an instant false alarm (violates
PRD F9's '<1 false alarm/node-week'); `rollup_qoe_1h` buckets ACCUMULATE within the
hour ⇒ 30 Welford ticks read non-independent samples (windowing redesign needed:
minute-granularity or tick-deltas). Re-assess when a real beacon deployment shows
sustained multi-viewer traffic AND a sub-hour windowing design exists.

---

### 2.15  Brand adoption — `brandkit/` → product UI  [M–L]  (OPERATOR-DIRECTED, D-071) — ✅ PHASE 1 DONE S12 (D-072) · ✅ PHASE 2 DONE (light theme + density modes + motion tokens, D-077, commit 08922ff) — verified S82/D-144: `theme.ts` + `ThemeContext` (localStorage + prefers-color-scheme + data-theme stamping), a Sun/Moon toggle in `Layout.tsx`, `[data-theme="light"]` token overrides in global.css with WCAG-adjusted colors, and theme/density tests. §2.15 is COMPLETE; §2.19 (full UI/UX refactor via uipro) is the only remaining UI item and stays operator-directed.

**Why:** The operator landed `brandkit/` at the repo root (2026-07-10, D-071) — a complete
brand & design package: machine-readable `design-system/tokens.json` (dark+light token
sets), full logo suite + favicons + PWA/iOS/Android icons, 8 hi-fi product screens
(`ui/Pulse App - Screens.dc.html`: login, dashboard, stream detail, analytics, settings,
users/tokens, error/empty/gated states, mobile ×2), a component library
(`design-system/Design System.dc.html`), and a WCAG-verified palette
(`documentation/design-rationale.md` §2 — BINDING). The current web UI is a GitHub-dark
placeholder (blue `#1f6feb` accent, no favicon, no logo asset, no light theme) that predates
the brand. **Operator directive: the frontend adopts the brandkit in the next session.**

**Source of truth:** `brandkit/design-system/tokens.json` is authoritative for every color/
space/radius/type value — do not invent values. Hi-fi screens + design-system doc are the
layout/component reference. `brandkit/documentation/README.md` maps the package.

**Scope (phase 1 = S12 WO-G, dark-theme parity):**
- **Tokens:** regenerate `web/src/styles/global.css` `:root` vars from tokens.json (bg
  `#0A0E14`, surface `#10161D`, signal `#2CE5A7`, status warn `#FFB224`/crit `#FF5C68`…),
  then sweep ALL hardcoded hexes in components — scouted (D-071): health-bar ternaries in
  `FleetPage.tsx`, chart series in `ProtocolDonut.tsx`/`AnalyticsPage.tsx`/`QoePage.tsx`,
  badge/toast background literals. ⚠ `FleetPage.test.tsx:146-168` pins the OLD hexes by
  value — update test WITH component (TDD).
- **Type:** self-host IBM Plex Sans + IBM Plex Mono (OFL) as woff2 under `web/` — NO CDN
  (ARCHITECTURE §3; the brandkit HTML previews reference Google Fonts for preview ONLY,
  never copy that). `font-variant-numeric: tabular-nums` on all metric values.
- **CSP:** self-hosted fonts need `font-src 'self'` — ⚠ `web/e2e/csp.spec.ts` asserts the
  CSP header BYTE-FOR-BYTE vs the Caddy config; update Caddyfile(s) + `CANONICAL_CSP`
  atomically or CI reds (INFRA-01 coordination for `deploy/`).
- **Identity:** create `web/public/` (does not exist) with `favicon.svg` + PNG 16/32/48,
  apple-touch-icon 180, PWA manifest icons 192/512 (+maskable) from `brandkit/{logo,icons}/
  png`; `<link rel="icon">` + title in `index.html`; primary-dark logo in the login screen +
  app shell per screens 01/02.
- **Components:** restyle per design-system — app shell/nav (active = signal left-border +
  `rgba(44,229,167,0.1)` tint), KPI stat cards (40px/700 tabular metric), tables (40px rows,
  11px mono uppercase headers), buttons/inputs/toggles/toasts; status is ALWAYS shape+color
  paired (dot/diamond/triangle/outline circle — CVD rule, never hue-only).
- **Charts:** recharts adopts the 8-color dataviz palette in order (series 1 = `#2CE5A7`),
  horizontal-only grid `#1E2833`, 2px strokes, mono 10px axis labels.
- **Reports (optional [XS] sub-item, BE-02):** swap the embedded default PDF logo to a
  rasterized brand asset — canonical white-label default is `logo/powered-by-pulse-badge.svg`
  (must rasterize: the embed path requires PNG/JPEG); `PULSE_REPORT_LOGO_PATH` override
  behavior unchanged.

**Explicitly OUT of phase 1 (→ phase 2 backlog):** light theme (tokens.json has the set,
but no theme-switch mechanism exists in the SPA), density/wall-screen modes, motion
language, marketing-site build, mobile bottom-tab layout.

**Verification:** vitest runs `css: false` — CSS-var typos are INVISIBLE to unit tests; the
Playwright specs (dashboard zero-console-errors + csp byte-equality) are the real gate. Web
coverage gates (59/54/45) must stay green. Visual acceptance = operator browser check
(U5 pattern); attach screenshots to the handoff.

**Size:** [M–L] — mostly FE-01 (`web/`); + optional reports [XS] (BE-02) + one Caddy CSP
line (INFRA-01). `brandkit/` itself is read-mostly design source, owner FE-01 (manifest
updated D-071).

### 2.16  AMS operational early-warning — demand-driven (OPERATOR-APPROVED 2026-07-12, D-086 addendum)  [S–M]

Seeded by an operator-directed review of the Ant Media issue tracker (2026-07-12).
Two upstream issues are direct demand evidence for Pulse:

- **ant-media/Ant-Media-Server#3122** (Prometheus exporter; closed 2023 UNBUILT —
  community json_exporter workaround with a moved blog + lost dashboard). Pulse's
  `/metrics` endpoint (server.go:882–906) already delivers this natively →
  **positioning item, not a build item**: cite as demand evidence in the
  marketplace assessment.
- **ant-media/Ant-Media-Server#7926** (OPEN, 2026-07-06: AMS freezes after ~24 h
  under ~90–100 RTMP publishers; Java alive, HLS/API dead, **OS metrics normal**
  — so cpu/mem/disk anomaly metrics are blind by construction). Pulse already
  DETECTS the freeze (node_down absence eviction ~3×PollInterval + HLS probe
  failure alerts). The gap is EARLY warning:
  1. **`ams_api_latency_ms`** (node scope): measure poll round-trip in
     restpoller (today errors are only logged, nothing measured) → live
     snapshot → anomaly whitelist (same 5-copy plumbing as §2.14/F9) — catches
     the "gradual" degradation the report describes.
  2. **API error-streak → `node_degraded`**: consecutive poll-failure counter
     feeding the existing `node_degraded` rule type (evaluator.go:379) before
     full absence.
  3. Stretch: probe TTFB trend anomaly (HLS/DASH TTFB already stored in
     probe_results; nothing watches the trend, only hard failures alert).
  The S24 flag-event store persists these detections with timestamps — the
  forensic timeline the #7926 reporter lacks.

Rides the S25 anomaly-expansion session naturally (WO-D there). Plus two
one-line demand-evidence citations in `docs/assessment/final-assessment.md`.

**✅ BUILT S25 (D-087)** — all three items + BUG-011 (EvictStaleNodes never
wired) fixed. **Follow-up [XS], seeded by the S25 verifier:** query.FleetNodes
sets status="degraded" only on CPUPCT>90 — it ignores ConsecAPIErrors>=3, so a
node firing the rung-2 node_degraded ALERT still shows status "up" on
/fleet/nodes + the Fleet page (display-consistency gap; not contracted in the
FleetNode schema; S26+ candidate). **✅ FIXED S26 (D-088)** — and the gap was
wider than filed: FleetNodes also missed the MemPCT>90 arm, and LiveOverview
carried a third drifted copy. All three now call the single predicate
`domain.LiveNodeStats.Degraded()` (CPUPCT>90 || MemPCT>90 ||
ConsecAPIErrors>=3) — alert and display can no longer drift. Same session:
standalone zero-mean cpu/mem/disk baseline guard (presence flags) + boot-time
sweep of the poisoned rows (live census: realams n=733, prod n=8813). See
§2.17 for the S26-seeded follow-ups.

---

### 2.17  Anomaly/fleet honesty tail — S26-seeded follow-ups  [XS–S each]  (NEW, D-088)

Observed during S26 (scouts + verifiers), deliberately not built (scope
discipline). Each is independent:

1. **viewer_count zero-mean baselines — needs a PRODUCT ruling first [S].**
   A stream with 0 viewers for ≥30 ticks (live: realams teststream, n=733)
   has a mean=0/stddev=0 baseline; the FIRST viewer produces z≫4 ⇒ anomaly
   flag. Unlike cpu/mem/disk this is a REAL measurement (0 viewers is
   true), so the presence-flag fix does not apply. Decide: is
   "audience appeared" a wanted signal (keep, document) or noise
   (needs e.g. a min-mean floor or count-metric variance floor)? Write the
   ruling into the anomaly docs either way.
   RULED S28 (D-090): kept + documented in docs/guides/anomaly-detection.md — 0 viewers is a real measurement; first-viewer z-spike is a true anomaly; suppression (observation-side skip mirroring the APILatencyMS>0 pattern) remains a 2-line follow-up if the operator overrules.
2. **TestAnomalyMetricMapSwitchParity derives from a hardcoded 6-case
   slice [XS]** (wave3_d087_test.go:189) — refactor to iterate
   `supportedAnomalyMetrics` so a 7th metric cannot be added to the map
   while silently missing from the parity pin.
   ✅ DONE S28 (D-090): `alert.SupportedAnomalyMetrics()` exported; the
   parity test fail-fasts on any canonical metric without a case (RED
   proof re-derived independently by the verifier: fake 7th metric →
   t.Fatalf naming it); the second hardcoded slice found at
   wave3_d087_test.go:44 (validator coverage) derived from the canonical
   set too.
3. **FleetNodes can never emit contracted status="down" [XS–S].**
   Eviction (D-087) removes a stale node from the snapshot entirely, so
   the pre-eviction window shows "up"→(gone); the contracted "down" enum
   value is unreachable. Decide: emit "down" during
   LastSeenAt>threshold-but-not-yet-evicted, or document the two-state
   reality and drop "down" from the enum (contract CR).
   ✅ RULED + DONE S28 (D-090): Option B — deliberate contract CR drops
   "down" from BOTH enum sites (NodeHealth.status + FleetNode.status;
   [up, degraded]); truer to the AMS lifecycle (no native soft-down
   state; the node_down ALERT keys on snapshot absence and is untouched).
   Regen idempotent; dead web consumer removed (FleetPage "Down" tile
   showed a structurally-permanent 0). Re-adding "down" later is an
   additive CR if two-phase eviction is ever built.
4. **DeleteZeroMeanNodeBaselines PG integration coverage [XS]** — ✅ DONE
   S26 (D-088): standalone `TestPG_DeleteZeroMeanNodeBaselines`
   (server/internal/store/meta/meta_pg_integration_test.go:769), explicit
   `-v` run PASS vs postgres:16 (D-088 close evidence). *(Ledger stamped
   S29/D-091 — 3rd ledger-drift find after §2.4/§2.5; the test landed
   same-session as the sweep, this row was never ticked.)*

### 2.18  Marketplace-readiness sprint  [M–L]  (OPERATOR-DIRECTED 2026-07-13, D-089 — ★ TOP PRIORITY, supersedes ordering of every non-gated item above) — ✅ ITEMS 1–5 DONE S27 (D-089, 2026-07-13: rollout live; trial lifecycle live-proven [7/7 mutations RED + live 3-min-key expiry]; quickstart live clean-install vs real AMS; rows 16/17→PASS + scores 66.7/84.5; listing draft INTERNAL). Item 6 (operator-gated) OPEN — 5 items in operator-expected.md ⚡ incl. NEW GHCR-public flip.

Operator directive (S27 prompt): app ready for the Ant Media marketplace
ASAP — installation easy, listing uploadable with a trial license key;
"rollout quick" (executed S27). Everything below is sequenced ahead of
§2.5/§2.11-tail/§2.17 until DONE. Operator-gated rows are listed last —
they gate the UPLOAD, not the build.

1. **Prod rollout D-082..D-088** [S] — ⚙ S27 (the standing offer,
   operator-triggered). Runbook path + `pre-d089` rollback tag + smoke.
2. **Trial-license lifecycle** [M] — expiry at runtime must degrade to
   free-tier entitlements GRACEFULLY (product keeps running; honest
   API/UI surface of "trial expired"); mint→install→expire documented;
   dev-key-signed short-expiry licenses in tests; mutation-pinned.
   Existing infra: license.go expires_at + tiers; licensegen -tier
   -expires -privkey (§2.3, S10). ⚠ The official trial key mint is
   OPERATOR-GATED (vault privkey — S16 key hygiene).
3. **One-command install** [S–M] — quickstart path with a trial-key slot;
   install.md brought current (the pulse.example.yaml "not consumed at
   runtime" wart documented or fixed); clean-install verified per D-069's
   still-open verification intent.
4. **Marketplace checklist PARTIAL→PASS rows in our control** [S] —
   final-assessment §3: row 4 (beacon SDK guide exists since S19 — fold
   in + close the loop), row 12 (release/tag evidence — v0.3.0 EXISTS,
   row is stale; refresh + tag the rollout build if warranted), row 16
   (AMS version compat disclosure), row 17 (known-limitations doc from
   DG-01..18).
5. **Listing package DRAFT** [S] — category, short description, feature
   bullets, screenshot list (row 10). Stays INTERNAL until the
   final-assessment review (D-081 external gate) — drafting is not
   publishing.
6. **Operator-gated (recorded, not buildable):** official trial key mint
   (vault privkey); final-assessment DRAFT review (gates upload); Ant
   Media marketplace contact — rows 7–11 (support channel, licensing
   terms publication, revenue share, category confirm, co-marketing);
   AMS license re-apply (promised 2026-07-13).

### 2.20  Ship-readiness — install paths, licence ceremony, Export  [S] ✅ DONE S35 (D-097, 2026-07-14, PR #51)

Seeded by the operator's question *"have you finished all development? is installation and
generating license keys ready?"* — answered by **executing** the docs, not reading them. 42 agents;
36 raw findings → **33 confirmed / 3 refuted** after an adversarial pass. **3 blockers, 10 majors.**

- ✅ **`GET /api/v1/reports/export` implemented.** It did not exist — yet Reports shipped **Export
  CSV** and **Export PDF** buttons wired to it, so a paying Business customer got a **404**. A
  *missing feature*, not a doc bug. CSV implemented + tier-gated; **PDF removed rather than left
  broken (LIM-24)**.
- ✅ **Authenticated downloads.** The analytics Export CSV button had never worked: it used
  `?token=` in the URL, which `bearerAuthMiddleware` deliberately ignores → **401**. Now the
  `Authorization` header + a blob, which also keeps the token out of logs and browser history.
- ✅ **`docs/licensing.md` activation ceremony corrected** — it documented `POST
  /api/v1/license/activate`; the server registers **`PUT /api/v1/admin/license`**. Wrong path AND
  method, under a heading titled *"Verify activation."* Plus: an **expired key returns 200, not
  422**. New customer-facing `docs/guides/license-activation.md`.
- ✅ **Install paths repaired** — `make up` / `docker compose up -d` always failed
  (`pulse-migrate` had no `PULSE_SECRET_KEY`); the README Quick Start **silently monitored a mock
  AMS**. The GHCR quickstart remains **operator-blocked** (private package), but now fails
  honestly instead of surfacing a raw Docker error.
- ✅ `prometheus.md` metric table / tier / gate-fn corrected; `probes.md` no longer tells
  **Business** customers they have no probes.
- ⛔ **NOT fixable by a session:** GHCR visibility (one click), Kafka fleet-consumer live
  validation (needs a Kafka-enabled AMS lab), G7 brandkit values.

**Ledger correction:** *"No customer can install Pulse"* (carried since S33) was **overstated** —
clone-and-build never touches GHCR and **works**. Only the quickstart is dead.
**The vendor key ceremony is DONE** (S16/D-077); it had been wrongly carried as open.

### 2.37  F6 multi-tenancy — operator-directed "start F6"  [⚙ PHASE 1 ✅ + PHASE 2 ✅; PHASE 3 = [20] next]

**Phase 1 ✅ (D-148, S86, PR #168+#169, prod v0.4.0-112-g75031e7)** — server-side tenant resolution on the live endpoints;
`?tenant=` filter + `LiveStream.tenant`; **BUG-009 tenant portion CLOSED**. New shared `internal/tenant` (Matcher +
CachedResolver, fail-safe); `query.Service.SetTenantResolver`; fail-closed; single-tenant unaffected. Verify-first found
F6 was NOT greenfield — the tenant registry (`tenants` table + `stream_pattern`/`meta_tag` + CRUD + `/admin/tenants`)
already existed but was billing-only.

**Phase 2 ✅ (D-149, S87, PR #171, prod v0.4.0-114-ge295795)** — tenant-scoped QoE alert rules; **★ S73 finding [5]
CLOSED (the last one → S73 audit 8/8 shipped).** `domain.AlertScope.tenant` (stored in ScopeJSON → **no migration**;
backward-compatible); `QoEReader.QoEForStream` gains a `tenant` param passed to `QoeParams.Tenant`; the evaluator threads
`scope.Tenant`. Reachable via `POST {"scope":{"tenant":"acme"},...}` (no handler change — scope passes opaquely).
Mutation-proven; scoping applies to the tenant-blendable QoE-read metrics (rebuffer_ratio/error_rate); ingest_bitrate_floor
is publisher-side and unaffected.

**Phase 3 = [20] audit-log read model** (the last F6 item; an S62 defer-by-ruling product call). After Phase 3, F6 core is
complete; remaining multi-tenant polish is demand-driven. See SESSION-88.

### 2.36  OpenAPI contract drift — document GET /reports/export  [✅ DONE — contract/test/types only, no prod deploy]  ✅ S85 (D-147, 2026-07-17, PR #162, main e3abc3b)

Surfaced by an adversarial verification sweep at S85 open (3 scouts + judge) run to *confirm* the "backlog exhausted"
claim before idling — it confirmed the ROADMAP backlog is fully gated/done but found a real non-gated defect. `GET
/api/v1/reports/export` (the Business-tier CSV usage-report download — `export.go`, registered under
`downloadAuthMiddleware`, consumed by the web `reportsApi.downloadExport` client) was **absent from
`contracts/openapi/pulse-api.yaml`** and the generated `schema.d.ts`, violating CLAUDE.md §3 *"Contracts before code."*
Confirmed genuine drift not convention — the sibling `/analytics/audience?format=csv` IS documented.
- Added the `/reports/export` path block (`from/to/app/stream/tenant` `$ref` params + inline `format` enum `[csv,pdf]`;
  200 `text/csv`, 401, 403, 500, 501; download-auth security schemes).
- Regenerated `schema.d.ts` (additive; `exportUsageReport` op) + registered the 6 params in `param_conformance_test.go`
  (data-filter params exempt like `/reports/usage`; `format` a real csv→200 / pdf→501 probe); floors `minSpecParams
  88→94`, `minProbes 37→38`.
- Go 25/25 + gofmt clean; web typecheck/lint/build green. **No prod roll** (no runtime behavior change) — prod stays
  `v0.4.0-98-g641b4e2`. **Deferred (flagged, not taken):** CHANGELOG `[0.4.0]` gap (judgment-heavy reconstruction) and
  the cosmetic/unread VERSION-file `0.1.0` staleness.

### 2.35  Documentation-gaps deliverable — completion + tracker reconcile  [✅ DONE — docs-only, no prod deploy]  ✅ S84 (D-146, 2026-07-17, PR #160)

A verify-before-writing completeness pass on `docs/assessment/documentation-gaps.md` (the SESSION-84 Option-C bounded
arc). Found `docs/known-limitations.md` had already closed **15 of 18** gaps (the tracker's status notes were stale);
authored the last 3 residual S17-drift footnotes in `AMS-INTEGRATION.md` (DG-12 applications/info 405; DG-14 versionType
"Enterprise Edition"; DG-13 app-inventory-reset troubleshooting — with a corrected remediation, the gap's suggested
`grep 'resolveApps'` marker doesn't exist) and reconciled the tracker (gap → LIM-*/doc closure map). **All 18
documentation gaps are now closed; the S18 Phase-6 deliverable is complete.** No prod deploy.

### 2.34  Web test-coverage polish — lowest-covered UI files  [✅ DONE — test-only, no prod deploy]  ✅ S83 (D-145, 2026-07-17, PR #158)

A bounded, unobjectionable arc taken because §2.7 is date-locked (≥07-23) and the S82 checkpoint is unanswered — the
plan's Option B (a concrete safe move over idling). Raised the two lowest-covered web files with **pure test additions**
(no behavior change): `SettingsPage.tsx` **55.5→95.4% lines** (30.5→94.4% funcs) and `OnboardingWizard.tsx`
**73.0→93.7% lines** (57.1→90.5% funcs). New tests cover ingest-token creation + `IngestSnippet` clipboard copy,
populated list rows, delete/revoke success+cancel, the S3 export form, the license card + activation (success+failure),
and the full `handleTest` verify flow + `handleSourceSave` failures. Full suite **676 passed** (+23); global lines
coverage ~76%. No server/web source changed → prod stays `v0.4.0-98-g641b4e2`.

### 2.33  Cross-cutting security-posture pass — supply-chain + container hardening  [✅ COMPLETE — deps clean + pulse container hardened + report-artifact retention prune shipped]  ✅ S80→S81 (D-142→D-143, 2026-07-17, PR #152 + #155, prod v0.4.0-98-g641b4e2)

The FIRST non-subsystem audit (the three prior audits §2.29/§2.30/§2.31/§2.32 were by-package). Covers what subsystem
sweeps structurally can't: dependency/supply-chain + deploy hardening.
- **Deps:** Go `govulncheck` **0 reachable** (1 module-only `x/crypto/openpgp`, no fix, not imported → informational).
  Web `npm audit` 3 findings, all **dev-toolchain-only** (undici via jsdom; js-yaml via openapi-typescript/redocly — not
  in the shipped bundle) → pinned patched in-major via `overrides` (undici@7.28.0, js-yaml@^4.3.0) → **audit clean.**
- **Container hardening** (`deploy/docker-compose.hardened.yml` `pulse`): `read_only` + tmpfs `/tmp`, `cap_drop:[ALL]`,
  `no-new-privileges` on the already-non-root image; + `PULSE_REPORTS_DIR=/var/lib/pulse/reports` (was writing to the
  ephemeral container root — lost on redeploy). Prod-verified (recreate, 0 EROFS/permission errors, 5-check smoke green).
- **Adversarial review:** 5 findings, 4 refuted, **1 confirmed LOW** → the follow-up below.
- **✅ FOLLOW-UP DONE (S81, D-143):** report-artifact retention prune shipped — `PULSE_REPORT_ARTIFACT_RETENTION_DAYS`
  (default 90; `<=0` disables), strictly bounded to regular `pulse-usage-*.{csv,pdf}` files in the reports dir (never the
  metastore/secret-key), runs each tick independent of schedule-listing. Its own review found 4 issues (HIGH prune
  gated behind a schedule-listing error → decoupled; symlink guard; `envInt` whitespace; base-compose persistence), all
  fixed pre-commit. 8 mutations killed; prod-verified `v0.4.0-98-g641b4e2`.

### 2.32  Third fresh subsystem audit — remaining un-swept subsystems (8 findings)  [✅ COMPLETE — 8/8 dispositioned: 7 shipped (D-136…D-140) + 1 defer-by-ruling ([5] QoE cross-tenant, multi-tenant-only — a tenant-scoped-alerting FEATURE, escalated as a product call); ALL 3 HIGH + 4/5 MEDIUM shipped; every fix verified-CORE + mutation-proven + adversarially reviewed]  ✅ DONE S73→S79 (D-135…D-141, 2026-07-17, PR #140…#150, prod v0.4.0-93-g8858b5f)

### 2.31  Second fresh subsystem audit — un-swept subsystems (25 findings)  [✅ COMPLETE — 25/25 dispositioned: 24 shipped + 1 defer-by-ruling ([20] audit-read, non-code product call); ALL 6 HIGH + 15 MEDIUM + 3/4 LOW shipped; every fix verified-CORE + mutation-proven + adversarially reviewed]  ✅ DONE S62→S72 (D-124…D-134, 2026-07-16→17, PR #119…#138, prod v0.4.0-82-g8355127)

With the §2.30 (S48) audit COMPLETE, SESSION-62 followed the standing re-scan mandate and ran a **fresh adversarial
audit of the subsystems S44/S48 never swept** — `alert/evaluator`+`alert/channels`, `license`, `prober`, `anomaly`,
and the `api` handler families not covered by S44. Same workflow (7 finders + refute-by-default verifiers, 33 agents)
→ **26 raw → 25 CONFIRMED (6 HIGH, 15 MEDIUM, 4 LOW), 1 refuted.** All in `agents/handoffs/S62-AUDIT-FINDINGS.md`
(full mechanism/scenario/mutation/fix per finding).
- **6 HIGH:** STARTTLS silent-discard → SMTP cred risk (`channels.go`); Telegram token in error logs (`telegram.go`);
  unbounded MPD read (`probe_dash.go`); attacker-controlled printf format → GB alloc (`probe_dash.go`); two nil-deref
  panics in the reports_wave2 update re-fetch paths (`reports_wave2.go`).
- **Re-verify caveats:** [24] audit-log admin gate may DUPLICATE the S43/D-105 "reads-open" product ruling
  (re-verify → likely DEFER/escalate); [1]/STARTTLS partially mitigated by Go's `smtp.PlainAuth` non-TLS guard (fix
  still correct, narrower scenario). Each finding is an AGENT finding — re-verify + take the verified CORE.
- ✅ **S63 (D-125, PR #120)** — shipped the **alert-channels security cluster** (findings [1]/[2]/[10]/[11]).
  [1] email STARTTLS now fails closed (was `_ = err` → silent plaintext fallback of body + SMTP AUTH creds);
  [2] Telegram bot token redacted from returned errors (`client.Do`'s `*url.Error` embedded the token-bearing URL);
  [10] SMTP Subject CR/LF-sanitized (publisher `stream_id` → title → header injection); [11] DOWNGRADED to LOW +
  fixed (dashboard_url href-escaped — but it's operator-derived, no live exploit). Full suite 24/24; mutation-proven
  ×4 (fake SMTP server for STARTTLS); 2-lens review → 2 major (STARTTLS config semantics — kept fail-closed, resolved
  via docs; behavior change: `STARTTLS=true` now mandatory) + 2 refuted. Prod `v0.4.0-64-g5172150`.
- ✅ **S64 (D-126, PR #122)** — shipped the **reports_wave2 post-mutation re-fetch cluster** (findings [5]/[6]/[19]).
  [6] HIGH `handleUpdateReportSchedule` — DROPPED the redundant re-fetch (row is authoritative; no updated_at in the
  response), structurally eliminating the nil-deref + a DB round-trip; [5] HIGH `handleUpdateTenant` — KEPT the
  re-fetch (updated_at stamped inside the store, not returned in row) but GUARDED it (mirrors handleUpdateProbe);
  [19] MEDIUM — SPLIT transient-error(→500) from missing-row(→404) in the three existence checks. Full suite 24/24;
  [19] deterministically mutation-proven via a pre-canceled-ctx internal test; self-review (no auth/contract surface).
  Prod `v0.4.0-66-gfede961`.
- ✅ **S65 (D-127, PR #124)** — shipped the **prober DASH untrusted-input cluster** (findings [3]/[4], the last 2
  HIGH). [3] MPD manifest body now `io.LimitReader`-capped (16 MiB) before xml.Decode (segment body was already
  capped; manifest was the gap); [4] `$Number%<spec>$` printf format now positive-allowlisted (`^%0?\d{0,3}d$`) so a
  hostile `%999999999d` degrades to plain decimal. **A 4-lens adversarial review found — and this PR also fixed — a
  sibling sink:** `$RepresentationID$` `strings.ReplaceAll` was itself unbounded (TB-scale within the body cap), now
  bounded by `maxExpandedTemplateBytes` (64 KiB). Full suite 24/24; mutation-proven ×4; 1 review finding refuted. Prod
  `v0.4.0-68-g2a122fd`. **★ ALL 6 S62 HIGH now shipped.**
- ✅ **S66 (D-128, PR #126)** — shipped the **prober RTMP DoS** cluster (finding [13] + a review-found sink),
  completing the prober subsystem's untrusted-input hardening. [13] `readAMF0Command` now caps distinct CSID states at
  `maxCSIDStates=256` (was unbounded → ~4.3 GB); a 4-lens adversarial review also found + fixed a per-message 64 KiB
  copy for silently-skipped message types (GC-pressure DoS within the cap). Off-by-one (`>`→`>=`) declined as
  immaterial once the count is capped (review agreed); a `uint16`-truncation NIT declined + logged. Full suite 24/24;
  mutation-proven; 2 review findings confirmed (both addressed), 0 refuted. Prod `v0.4.0-70-g5a070cc`.
- ⏳ **15 remain (0 HIGH, 11 MEDIUM, 4 LOW)** → S67+. Suggested order: **alert-evaluator** ([7] `evalNodeMetric`
  missing D-088 presence guards → false threshold alerts on AMS 3.x nodes; stream_offline compare bypass;
  license_expiry stuck-firing) → **anomaly** ([18] `scopeJSON` raw-concat without escaping the ID fields → wrong
  stream attribution; hysteresis) → license → api. **⚠ [20] audit-log admin gate: re-verify vs D-105 "reads-open"
  ruling first (likely DEFER).** One coherent scope per PR. Plan: `sessions/SESSION-67.md`.

### 2.30  Fresh subsystem adversarial audit (16 findings)  [★ COMPLETE — 14 shipped (ALL 6 HIGH); 2 DEFERRED ([11],[12])]  ✅ DONE S48→S61 (D-110…D-123, 2026-07-16, PR #93…#117)

With the S44 13-bug backlog closed (§2.29) and the §2.7 CI-promotion gate not yet open (07-16 < 07-23), **S48
followed the standing re-scan mandate and ran a fresh adversarial audit of the subsystems the S44 audit never
swept** (collector, amsclient, reports, cluster, clickhouse): 7 finders + refute-by-default verifiers (27 agents)
→ **16 CONFIRMED (6 HIGH, 7 MEDIUM, 3 LOW), 4 refuted.** All recorded in `agents/handoffs/S48-AUDIT-FINDINGS.md`.
- ✅ **S48 (D-110, PR #93)** — shipped the most severe: a **cross-tenant data-isolation leak** —
  `AudienceAnalytics` omitted the `AND tenant = ?` filter its 3 sibling analytics queries all apply, so
  `?tenant=X` returned every tenant's audience rollups. Re-verified against the code; mutation-proven; prod
  `v0.4.0-37-g5e822e7`.
- ✅ **S49 (D-111, PR #95)** — shipped the **cross-app StreamID collision** cluster (findings [1]+[2], one root
  cause: AMS identity is `(app, streamId)` but two collector paths keyed on the bare `streamId`). (1) `dedup.go`
  `dedupKey` gained `app` (was dropping the 2nd app's `publish_start`/`end` in one window). (2) `aggregator.go`
  `snapRemoveStream` now guards its bare-`StreamID` map delete with a pointer-equality check (was evicting the
  other app's still-active stream). Re-verified against the code (the existing cross-app test passed trivially;
  guard is the proportionate fix — residual last-write shadowing is documented/self-healing, full rekey would
  break the alert groupKey lookup); mutation-proven ×2; 3-lens review (4 findings, all refuted); prod
  `v0.4.0-39-gc08ad6a`.
- ✅ **S50 (D-112, PR #97)** — shipped **[3] `amsclient` streamID URL-path-escaping**. `WebRTCClientStats` built
  its path with a bare `fmt.Sprintf`, so a publisher-chosen stream id with a URL-special char (`#`/`?`/space) made
  `url.Parse` target the wrong AMS endpoint → WebRTC QoE stats silently dropped through the poller's `err==nil`
  gate. Fix: `url.PathEscape(streamID)` (`app` left raw — audit-refuted; the other 4 path-builders have no
  streamID, so single fix point). Mutation-proven; 2-lens review (0 findings); prod `v0.4.0-41-g60f2a13`.
- ✅ **S51 (D-113, PR #99)** — shipped the **reports-scheduler date/tz cluster [4]+[15]**. [4] the monthly
  statement's inclusive upper bound was first-of-CURRENT-month, so that day's daily-rollup rows (`bucket <= ?`)
  bled into the previous month → extracted `previousCalendarMonthUTC(now)` (inclusive [first,last]-of-prev-month).
  [15] `nextCronTime` read cron fields in the seed's local tz while the pipeline is UTC → normalized the seed to
  UTC inside the function (DRY; latent on UTC prod, real for non-UTC installs). Mutation-proven ×2; 2-lens review
  (0 findings); prod `v0.4.0-43-g7c206a9`.
- ✅ **S52 (D-114, PR #101)** — shipped **[5] cluster edge-stream status** (the **last HIGH**). `IsEdgeStream`
  had no Status check, so a crashed edge (marked `down` but with stale non-zero `ActiveStreams`) stayed "serving"
  forever → the aggregator permanently suppressed origin viewer counts (VD-03 dedup). Fix: `n.Status != "down"`
  (degraded still counts). Mutation-proven; review refuted a split-brain concern; prod `v0.4.0-45-g0ab487f`.
  **★ All 6 HIGH audit findings are now shipped.**
- ✅ **S53 (D-115, PR #103)** — shipped **[7] ingest zero-timestamp guard**. `onIngestStats` guarded a missing TS
  with `if now.IsZero()`, but `time.UnixMilli(0)` is 1970 (not the Go zero time), so a `TS==0` event stamped
  `LastSeen=1970` → `SweepStale` falsely evicted the publisher ("source gone"). Fix: `if ev.TS <= 0`.
  Mutation-proven; prod `v0.4.0-47-gd32b165`.
- ✅ **S54 (D-116, PR #105)** — shipped **[9] restpoller prevStatus leak**. `detectEnded` only evicted
  `broadcasting` keys, so idle/created streams that vanished from AMS leaked forever. Fix: decouple eviction
  (`stale`, all disappeared app-scoped keys) from emission (`ended`, broadcasting-only). Mutation-proven; prod
  `v0.4.0-49-g6d60f53`. (Also: added a CI **gofmt gate** learning to agent memory — `gofmt -l` before push.)
- ✅ **S55 (D-117, PR #107)** — shipped **[10] reports egress-method disclosure**. `ComputeUsage` returned the
  report-level `egress_method` hardcoded to `bitrate_x_watch_time` even when per-row egress came from AMS byte
  counters → the F6 CSV/PDF disclosure header lied. Re-verified beyond the audit's literal fix: the daily path can
  be **mixed** (aggregate `Totals.EgressGB` blends both), so "any→byte-counter" is just the mirror over-claim. Fix:
  3-way report-level disclosure (`bitrate_x_watch_time` / `ams_rest_stats_byte_counter` / new **`mixed`**), tracked
  across the included rows. Free-text string (no enum) — OpenAPI + `schema.d.ts` document `"mixed"`. Mutation-proven
  ×3; 3-lens review (0 confirmed); prod `v0.4.0-51-ge5577f7`.
- ✅ **S56 (D-118, PR #109)** — shipped **[13] beacon insert atomicity**. `insertBeaconEvents` opened a fresh
  `PrepareBatch`+`Send` per `BeaconItem` inside the double loop, so a mid-batch `Send` failure partial-committed
  items 0..M-1 while the flusher (`runBeaconEventFlusher`) counted the whole flush as failed — under-reporting
  `inserted` and silently dropping the rest. Fix: one `PrepareBatch` + one `Send` for the flush (mirror
  `insertServerEvents`/`insertViewerSessions`) → atomic. Mutation-proven (spliced the exact original per-item func
  back → 2 distinguisher tests redden); self-review (mechanical); prod `v0.4.0-53-g500aabb`.
- ✅ **S57 (D-119, PR #111)** — shipped **[16] cluster duplicate node_stats**. `poll()` set `seen[nodeID]`
  unconditionally and processed every DTO, so two DTOs resolving to the same key (both missing NodeID+IP → "")
  overwrote `d.nodes` AND emitted a second `node_stats` event → 2x node metrics + a phantom node. Fix: dedup guard
  at the top of the loop (`seen` now backs both dedup and the stale-check). Mutation-proven (drop the guard's
  `continue` → dedup test reddens `got 2 want 1`, positive control green); self-review (mechanical); prod
  `v0.4.0-55-ge13eb1f`.
- ✅ **S58 (D-120, PR #113)** — shipped **[14] beacon 413 detection**. The handler classified 413-vs-400 by
  `len(body) >= maxBodyBytes-1`, so a mid-body connection reset on a large-but-in-limit body was misreported as 413.
  Fix: detect the limit breach by ERROR TYPE (`errors.As(err, &*http.MaxBytesError)`). Verified CORE narrower than
  the audit — KEPT the post-read exact-boundary check (audit wrongly called it unreachable; `MaxBytesReader` doesn't
  error on a body of exactly `maxBodyBytes`). Mutation-proven (revert to the heuristic → new test reddens `got 413
  want 400`, `OverSize_413` green); self-review (mechanical); prod `v0.4.0-57-g36c16ed`.
- ⏸️ **S59 (D-121) — DEFERRED [11] anomaly baseline wrong columns (no fix shipped).** `AnomalyBaselineForMetric`'s
  viewer_count case queries `avg(viewers)`/`event_time`; re-verification CONFIRMED the columns are wrong
  (`viewer_count`/`ts` per `0001_init.sql:48,58`) BUT the function is **DEAD CODE** (`grep` → only
  `wave3_anomaly_query_test.go`; the live `anomaly.Detector` uses meta-store Welford baselines, not this ClickHouse
  path) and this exact latent bug was **already deliberately deferred by D-087** ("fix only when actually wired to
  live code"; F9 ClickHouse-baseline path GATED on real traffic). Ruling: DEFER — fixing dead code against an
  explicit deferral is churn with zero prod impact, and a piecemeal column fix would be incomplete (needs the
  default-branch metric-allowlist redesign D-087 describes, done TOGETHER when wired). Shipped an inline deferral pin
  at `query.go:1092`. **No prod roll** (comment-only; prod stays `v0.4.0-57-g36c16ed`).
- ⏸️ **S60 (D-122) — DEFERRED [12] SummingMergeTree `peak_concurrency` (no migration shipped).** Mechanism real
  (`peak_concurrency` isn't in the sum-list) but impact REFUTED: a whole-repo grep confirms **nothing reads
  `rollup_usage_1d.peak_concurrency`** — every peak READ comes from an AggregatingMergeTree via `maxMerge` (billing →
  `rollup_concurrency_1d`; analytics → `rollup_audience_1h/1d`), a human-approved design (**D-018 CR-VD38** created
  `0002_concurrency_rollup.sql` for exactly this; `TestAccountant_CHIntegration` proves TRUE windowed max, drift
  0.0000%). `accounting.go:209-210` documents the column as an unread "session-count proxy." The audit's fix would be
  inert, semantically wrong if ever read (summing `toUInt32(1)`/session = session-count), and risky (live `ALTER …
  MODIFY ENGINE`). Also: the migration lineage is already at **0010**, not 0004. No code/DDL change; no prod roll.
- ✅ **S61 (D-123) — shipped [8] webhook replay protection (opt-in). ★ LAST finding — audit COMPLETE.** Verified
  product-viability first: AMS lifecycle webhooks are UNSIGNED (`AMS-INTEGRATION.md §4.5`), so `X-Ams-Signature` is a
  Pulse-defined contract → Pulse can extend it without an operator dependency. Shipped a **backward-compatible**
  `PULSE_WEBHOOK_REQUIRE_TIMESTAMP` (default off) + `PULSE_WEBHOOK_TIMESTAMP_SKEW` (default 5m): off = bare-body HMAC
  byte-for-byte (zero ingest risk); on = fresh `X-Ams-Timestamp` (±window) + canonical timestamp-bound HMAC. Full
  suite 24/24; mutation-proven ×3; 3-lens adversarial review (10 agents, 7 confirmed/0 refuted/0 blockers → addressed
  5, deferred 1 per-source override as YAGNI). Docs `AMS-INTEGRATION.md §4.7`. Prod rolled forward (default-off →
  smoke green).
- **★★ S48 AUDIT COMPLETE: all 16 findings triaged — 14 SHIPPED, 2 DEFERRED** ([11] D-121 dead code / D-087; [12]
  D-122 vestigial column / D-018). SESSION-62 picks the next highest-leverage move (fresh subsystem audit, or the
  §2.7 CI-promotion win once today ≥ 2026-07-23). Full list: `S48-AUDIT-FINDINGS.md`; plan: `sessions/SESSION-62.md`.

### 2.29  Security hardening + 13-bug adversarial audit  [S shipped; M–L backlog]  ✅ SECURITY CLUSTER DONE S44 (D-106, 2026-07-15, PR #85)

**★★ S44 ran an 8-finder adversarial audit → 13 CONFIRMED defects, 0 refuted** (the "backlog is thinning"
claim was wrong). Shipped the **security cluster** (3 fixes, PR #85, all mutation-proven + 2 adversarial
reviews → SHIP): (1) **CSV formula injection** — export + statement CSV neutralize publisher-controlled
`app`/`stream_id`/`tenant` cells via shared `reports.CSVSafeCell`/`UsageCSVRecord`/`WriteUsageCSV` (OWASP
single-quote); (2) **email/SMTP creds** now encrypted at rest (`secretFields` += `password`/`username`);
(3) **OIDC `pulse_oidc_state` cookie** `Secure` on https. No contract/web/brandkit change; prod rolled forward.

**★ The other 10 findings are the S45–S47 backlog (real, verified, autonomous).** Progress:
- ✅ **S45 (D-107, PR #87)** — reports-scheduler cluster: the `PUT /reports/schedules/{id}` **BLOCKER** (NULLed
  `next_run_at` → silenced the schedule) + `nextCronTime` dropping day-of-month (default "Monthly" preset fired
  daily). Both mutation-proven + adversarially reviewed.
- ✅ **S46 (D-108, PR #89)** — entitlement + WS auth: the probe runner ignored `CheckProbes()` on the background
  tick (S37 "enforced, not decorative" — a downgraded tenant kept probing) → new `prober.Config.EntitlementGate`
  wired to `lic.CheckProbes`, checked before every probe; and `GET /live/ws` rejected browser sessions (OIDC
  cookie + browser `?token=`) because the route sat behind header-only bearer middleware while the handler
  re-extracted the token itself → moved to `downloadAuthMiddleware` + read validated `ctxTokenKey` (also gains
  `kind=api` + expiry). Both mutation-proven; adversarial review (2 refuted, 1 LOW spec should-fix → fixed:
  OpenAPI `/live/ws` now documents the `cookieAuth` path).
- ✅ **S47 (D-109, PR #91) — the 13-bug backlog is now FULLY CLOSED.** Audit integrity + hardening: phantom
  `user.delete`/`token.revoke` audit on a missing id — but the OpenAPI contract deliberately documents idempotent
  204-on-missing, so the fix keeps 204 and only suppresses the phantom audit (`meta.ErrNotFound` on 0 rows);
  create-user/token audit moved before the re-fetch (S40 class); token `kind` positive-allowlist `{api, ingest}`
  → 422; anomaly eval `>` → `>=` to match detect. Plus a CodeQL-surfaced CWE-916: removed `hashPassword`'s
  SHA-256 fallback (reject >72-byte passwords → 422; legacy `sha256:` verify kept). 8 mutations RED; review clean.

**★★ The S44 13-bug adversarial-audit backlog is now FULLY CLOSED** (S44 security · S45 scheduler · S46
entitlement+WS · S47 audit-integrity+hardening). SESSION-48 has no queued audit findings — per the standing
directive it re-scans §2 / assessment §5 for the next-highest-leverage track. Full detail: `decisions.md` D-106…D-109.

### 2.28  Close the two S34 e2e gaps — probes-create + reports-schedules  [S, test-only] ✅ DONE S43 (D-105, 2026-07-15, PR #83)

Drove the two documented S34 e2e coverage gaps end-to-end: (a) `probes.spec.ts` probe **create happy-path**
(valid submit → `POST /probes` → returned probe appended + form closed); (b) `reports.spec.ts` Reports
**Schedules tab activation** (click tab → `GET /reports/schedules` → row renders, not the empty state). Both
**mutation-proven non-vacuous** (removing the append and the fetch-on-activate turns exactly these two RED,
14 others green) — addressing the project's repeated vacuous-e2e failure mode. 16/16 in the Playwright docker
image; `tsc`+`eslint` clean; CI all green. **Test-only — no src/contract change, no prod deploy.**

**★ Two verify-at-open overturns (S38-style):** SESSION-43's lead candidate (admin-scope-gating the audit
read) deviates from the deliberate reads-open/writes-gated model (`requireWriteScope`) → a **product ruling**,
deferred to operator; candidate 3 (`PULSE_LICENSE_OFFLINE_FILE`) is entangled with the unwired `HOOK(BE-02)`
config skeleton → not XS. Both recorded as operator/ruling items. **Operator action: none for the build.**

---

### 2.27  Audit trail Phase 2 tail — audit OIDC first-login provisioning  [S] ✅ DONE S42 (D-104, 2026-07-15, PR #81)

Closed the last unaudited mutating path: `oidc.go` provisions a user on first SSO login OUTSIDE
`handleCreateUser`, so that creation was never recorded (S40 documented it as out-of-scope). New
`oidcHandler.auditProvision` writes a `user.provision` `audit_log` entry — **actor model differs**: no bearer
token exists pre-session, so the SSO subject provisions itself (`actor_user_id == object_id`,
`actor_token_id` empty, `actor_name = "oidc:<sub>"`, `detail = {role, via, groups}`). Placed **only in the
create branch** (after the re-fetch that populates `user.ID`), so provisioning is audited **once per user**,
not per login, and never in the concurrent-UNIQUE-race branch (the winning login audits it). Same best-effort
contract as `s.audit` (cancel-detached, 5 s-bounded, log-on-failure).

**Gates:** Go 24/24 · vet · gofmt; web `tsc`+`build`+vitest 650; new
`TestOIDC_Callback_FirstLogin_AuditsProvision` **mutation-proven RED**; **adversarial review → no real
defects**. CI all required green. **Operator action: none** (dormant until OIDC is configured; off in prod).
Prod rolled forward to **`v0.4.0-25-g6a0226d`** (smoke: healthz all-ok, webhook 200, limits `512M/0.5cpu`,
logs clean). Every user-creation path is now audited.

---

### 2.26  Audit trail Phase 2 — audit-log web UI  [S] ✅ DONE S41 (D-103, 2026-07-15, PR #79)

Surfaced the S40 audit trail in the SPA: `GET /admin/audit-log` shipped in S40 but had no page. Added an
**Audit Log** page (`web/src/features/audit-log/AuditLogPage.tsx`) — a read-only table (Time / Actor /
Action / Object / Object ID / Source IP) with cursor **"Load more"** pagination, mirroring `AnomaliesPage`.
No tier gate (a core admin feature; admin-only via auth). Router + left-nav wired; `adminApi.listAuditLog`
added; `AuditEntry`/`AuditLogPage` re-exported. **No Go/contract change** (the endpoint + schema were S40).

**Gates:** `tsc` · 650 vitest (incl. 10 new: states, actor fallback, load-more append + cursor param,
design-token pins) · `build`; **3 Playwright e2e** proven in the official Playwright docker image. CI all
green. **Operator action: none.** Live: the served JS bundle contains the page (proven at deploy).

**Phase-2 tail still open:** OIDC auto-provisioning audit (`oidc.go` CreateUser — distinct actor model);
optional admin-only gating of the audit read. → SESSION-42 candidates.

---

### 2.25  Audit trail — actor on every admin/config write  [M] ✅ DONE S40 (D-102, 2026-07-15, PR #77)

Closed the compliance gap "no actor is recorded on mutating API calls — no 'who changed what, when'" that
**gates SOC 2 / ISO 27001 buyers**. An append-only `audit_log` table records every admin/config mutation;
`GET /api/v1/admin/audit-log` reads it back newest-first with keyset pagination.

- **24 handlers** emit `s.audit(...)` on success: create/update/delete of alert rules & channels, users,
  tokens, probes, report schedules, AMS sources, tenants + licence activation. The actor comes from the
  bearer token already in the request context (`ctxTokenKey`) — **no new middleware**. `detail` is a
  non-sensitive summary; **never** a secret (adversarial-review-verified across all 24 sites).
- **No FKs** to api_tokens/users — a row survives token revocation and user deletion. Best-effort write on a
  5 s cancel-detached context (audit is not a write-path SPOF, nor may it hang the response).
- **Migration 0004**: SQLite via idempotent `applySchemaUpgrades`; Postgres via `EmbeddedDDLPostgres`.
- **Documented out-of-scope (not silent):** the two `/test` fires, `/auth/oidc/logout`, and OIDC
  auto-provisioning (different actor model — the top Phase-2 follow-up, with an audit-log web UI).

**Gates:** full Go suite (24 pkgs) · `gofmt`/`vet` · web `tsc`+`vitest`+`build`; guard mutation-proven RED;
2 param-conformance probes. Adversarial review (1 agent): no secret leakage, migration/pagination correct,
**1 real defect found+fixed** (two update handlers audited after the re-fetch guards → could drop a committed
mutation on a failed re-read). **Operator action: none** — but a NEW operator item surfaced: the AMS trial
expiry is documented inconsistently (runbook 2026-07-12 vs ledger 2026-07-27) and needs operator confirmation.

---

### 2.24  Out-of-band licence-expiry alerting (`license_expiry` metric)  [S] ✅ DONE S39 (D-101, 2026-07-15, PR #75)

Closed the D-098 funnel gap "licence-expiry warning is a **UI banner only** — a customer who never opens
the dashboard gets no warning before a tier downgrade." Added a **`license_expiry`** alert metric so the
alert engine warns through the operator's configured channels (email/Slack/Telegram/PagerDuty/webhook)
when the Pulse key is within N days of expiry. Rule: `{metric:"license_expiry", operator:"lt", threshold:14}`.

- **Faithful mirror of `cert_expiry`:** a non-ClickHouse scalar ("days until expiry") injected via
  `LicenseExpiryChecker`, dispatched by the evaluator's metric switch, evaluated against the rule
  operator/threshold, delivered through the normal channel path. `serve.go` adapts `license.Manager.ExpiresAt()`.
- **Key-state semantics:** free / perpetual / no-key → `ok=false` → **skipped** (cannot false-alarm);
  valid & expiring → evaluates; already-expired → clamps to 0 days → fires.
- **No API/schema/web change** — metrics are not enum-constrained (`cert_expiry` is API-creatable the same
  way; verified, not assumed). Operator creates the rule + a channel, exactly as for `cert_expiry`.

**Gates:** `gofmt`; `go build ./...`; full Go suite green (24 pkgs). Two guards mutation-proven RED — the
perpetual-skip guard **and** a new **`wireAlertLicenseExpiry` wiring pin** (added on adversarial-review
feedback: the unit tests call the setter directly, so only the pin proves `serve.go` actually wires the
checker into the real evaluator — raising this above `cert_expiry`, which has no pin). Adversarial review
(1 agent): **no defects.** **Operator action: none for the build** (rule + channel still operator-created).

---

### 2.23  /admin/users CRUD correctness (team-management foundation)  [S] ✅ DONE S38 (D-100, 2026-07-15, PR #73)

Set out to build the **team-management UI** (top D-098 funnel gap: `/admin/users` CRUD exists, no page).
Verify-at-open found the feature is **advisory, not real**, and the API had bugs — so S38 fixed the API
correctness and **deferred the UI to a product ruling** (operator item 10):

- **Why the UI is deferred:** the stored `user.Role` is **non-authoritative** — OIDC re-maps role from
  IdP groups on every login (`oidc.go` `mapGroupsToRole` → session `Scopes:[]string{role}`) and never
  reads the stored value; and **there is no password-login route** (SSO auto-provisions users). So a
  role set in a UI wouldn't govern anyone's permissions, and "invite a teammate" has no flow. The intended
  model (SSO-group-driven only / add password login / make stored role authoritative) is an operator call.
- ✅ **`handleUpdateUser` correctness:** was an unconditional `SET username=?, role=?` — a role-only edit
  **blanked the username**, a missing id returned **200**, and the response was an **echo with
  `created_at:0`**. Now: 404 on missing id; full-replace requiring both fields (matches
  `UserWrite required:[username,role]` — omitted field → 400, not a silent blank); role validated;
  returns the **real stored row**; concurrent-delete race → 404 not 500.
- ✅ **`handleCreateUser`:** role allowlist (`admin`|`viewer`); duplicate username → **409** (was 500).
- ✅ **Contract:** declared `409` on POST/PUT `/admin/users`; `schema.d.ts` regenerated (adversarial-review finding).

**Gates:** Go api+meta + full suite green; `gofmt`; web `tsc` + vitest; `schema.d.ts` in sync. Every
guard mutation-proven RED. Adversarial review (1 agent) → 3 findings (409 spec gap, partial-vs-full-replace,
TOCTOU 500), all fixed. **Operator action: none for the fix;** one product ruling surfaced (item 10).

---

### 2.22  Tier-entitlement enforcement — "enforced, not decorative"  [S] ✅ DONE S37 (D-099, 2026-07-15, PR #71)

Generalized the D-098 bug class (*capability stored but never checked*) into an audit of **every paid
entitlement**. Six gaps of that exact shape — a `Check*` that exists but is never called, or a paid
feature with no gate at all — five from the audit, a sixth from the close-out adversarial review:

- ✅ **SSO/OIDC → Enterprise.** Priced at Enterprise (PRD §7) but `/auth/oidc/*` gated **nowhere**.
  Added `license.CheckSSO()`; gated login + callback (after the `s.oidc==nil` 501; **logout left open**)
  + made `/auth/oidc/status` report `enabled=false` unlicensed. **Closes the D-098 "unenforced
  revenue" funnel-gap row.**
- ✅ **White-label report headers → `white_label`.** `CheckWhiteLabel()` on schedule create/update
  **and** the scheduler timer path (drops branding after a downgrade).
- ✅ **Alert-channel type on update + test-fire** (create was already gated).
- ✅ **Scheduler re-checks the licence per fire** — a schedule created while licensed stops after a
  downgrade (the HTTP CRUD gate can't cover the timer).
- ✅ **Retention clamp on Geo/Device/QoE/Ingest** reads (only `AudienceAnalytics` clamped before).
- ✅ **★ Review-caught:** `QueryProbeResults` forwarded caller `from`/`to` unclamped (Free tenant →
  365 d of probe history, HIGH); and the `handleOIDCCallback` `CheckSSO` gate had **no test** — the
  S36 vacuous-test trap, which my own harness comment wrongly claimed was covered (MED). Both fixed,
  both mutation-proven. Adversarial workflow: 5 dimensions → refuter panel, **2 CONFIRMED / 0 uncertain.**

**Design ruling:** `MaxStreams` NOT gated — every shipped tier is `-1` (unlimited) and Pulse is
observe-only (no ingest-refusal point). A finite `MaxStreams` is a product decision, not engineering.

**Gates:** Go 24/24 + `gofmt`; web `tsc` + vitest; every guard mutation-proven RED. No web files
changed. **Operator action: none;** blockers unchanged (GHCR 401, AMS expiry 2026-07-27). Prod rolled
forward at close (behaviorally inert on the Enterprise prod licence).

---

### 2.21  User-intake — signup/login audit + the three post-login blockers  [S] ✅ DONE S36 (D-098, 2026-07-15, PR #53)

Seeded by the operator's question *"are we ready for user intake? how do they sign up and log in?"* —
answered by **executing** every auth path, not reading the docs. 161-agent adversarial audit
(7 lenses → 3-refuter panel → synthesis); **51 raw findings → 29 confirmed / 22 refuted.**

**The answer: there is no signup.** Pulse is self-hosted, sold by signed licence key. The first
credential is a **bootstrap admin token** minted on first boot and printed to the container logs,
once (`bootstrapIfFirstRun`). Login is that token or OIDC/SSO. Bootstrap works; the breakage was all
**after** authentication:

- ✅ **Privilege escalation closed.** `bearerAuthMiddleware` never read `Scopes`; a `viewer` OIDC
  token could `POST /api/v1/admin/tokens` and self-escalate. Added `requireWriteScope` on `/api/v1`
  — a **positive allowlist** (writes need `admin`; empty scopes grandfathered; reads always pass).
  The implementer's first cut denied only `"viewer"` while the UI mints `"read"` — a fake fix, green
  against a wide-open path; caught by adversarial review and **mutation-proven** with a read-scope
  escalation test.
- ✅ **Onboarding dead-end closed.** `OnboardingGuard` sends a user landing on `/` with zero sources
  into the wizard; fires only on `/` so Settings is never hijacked; fails open on error.
- ✅ **Credential-loss trap closed.** Persistent token copy-panel replacing the 4-second toast;
  create flow now asks admin-vs-read.
- ✅ `install.md` first-login steps corrected (token on the login screen, not the wizard; verify
  step calls `POST /admin/sources/{id}/test`; token-loss recovery cost stated up front).
- 🚫 **Refuted, not propagated:** "AMS creds in cleartext" — token empty, AMS 403s anon, collector
  healthy (826k+ rows). Residual: AMS:5080 on `0.0.0.0`, no ufw — an **AMS** hardening note.
- ⛔ **NOT fixable by a session:** GHCR visibility (still 401), AMS licence expiry (2026-07-27).

**Non-blocker gaps surfaced (funnel, not fixed):** team/invite UI, audit trail, OIDC licence-gating,
tenant isolation, self-serve trial/billing, out-of-band licence-expiry alerting. See D-098 table.

---

### 2.19  Full UI/UX refactor via uipro ("UI/UX Pro Max" skill)  [L, phased]  (OPERATOR-DIRECTED 2026-07-14, S29 mid-session)

**Directive (operator, verbatim intent):** "We have installed uipro to
refactor ui … refactor the all ui/ux by uipro."
**What exists today:** the `uipro` CLI v2.11.0 is installed globally
(`~/.nvm/versions/node/v20.20.2/bin/uipro`) — it is an installer that adds
the "UI/UX Pro Max" skill for AI coding assistants to a project
(`uipro init`). The skill is NOT yet initialized in this repo (no
`.claude/skills/` here or globally at directive time).
**Relationship to D-071 (brandkit):** `brandkit/` remains the design
source of truth — `tokens.json` colors/type/spacing authoritative, WCAG
table binding, IBM Plex self-hosted only — unless the operator explicitly
overrules D-071. uipro drives the refactor *method/quality*; brandkit
constrains the *values*. If the skill's guidance conflicts with a brandkit
token, the token wins and the conflict is filed for the operator/designer
(same class as the S28 dc.html CDN-font finding).
**Plan (phased, PR-gated):**
1. **S30 scoping WO [S]:** `uipro init` in-repo (rides a session PR;
   inspect what it installs BEFORE committing — third-party skill content
   gets the same review as any vendored code), inventory the skill's
   guidance, map it against `brandkit/design-system/tokens.json` +
   `brandkit/documentation/design-rationale.md` §2, and produce a
   page-by-page refactor wave plan (LiveOverview, Streams/StreamDetail,
   Ingest, Probes, Fleet, Alerts, Anomalies, Reports/Billing, Settings,
   app shell/nav) with per-wave acceptance gates.
2. **Waves [M each]:** refactor per page-group under the skill, gates per
   wave: vitest green + coverage floors (59/54/45), lint, build,
   Playwright-docker visual/e2e (light+dark, reduced-motion, density
   modes), WCAG table conformance re-checked, zero contract changes (UI
   uses the public API only, ARCHITECTURE §3).
3. **Close-out:** brandkit adoption ledger (§2.15) cross-updated; any
   token-vs-skill conflicts resolved by operator ruling.
**Sequencing:** behind the operator-gated §2.18 marketplace tail (upload
prep stays first when unblocked); ahead of §2.9/§2.10/§2.12. Does NOT
touch sdk/beacon-js (no UI) or server/.
**Scoping WO DONE — S30 (D-092, 2026-07-14):** vendored-skill review (DO_NOT_COMMIT verdict on
full bundle; ui-ux-pro-max core IN scope, design/ui-styling/slides OUT), conflict ledger (6
C-items resolved token-wins; 2 operator gaps G1/G2 filed), 6-wave page-group plan with
binding per-wave acceptance gates. Wave plan: `agents/handoffs/wave-uipro/WAVE-PLAN.md`.
Wave 0 (Shared Surface [S]) is the recommended first wave → S31 after §2.18 gate clears.
**Wave 0 DONE — S31 (D-093, 2026-07-14):** `TierGate` extracted from the triplicated
inline `TierUpsell` pattern in Reports/Anomalies/Probes; `Tabs` component created (keyboard
nav, ARIA roles, roving tabIndex); `global.css` extended with 4 radius/touch tokens + shared
focus-ring block; all three page files adopt `<TierGate>`; 44 unit tests pass (TierGate.test +
Tabs.test). CHART_COLORS[7] (`'#7C93AD'`) confirmed present — no change needed. Tab-site
inventory corrected: 4 pages carry the identical inline tab pattern (Analytics, Alerts,
Reports, Settings); QoE has no tab pattern and Fleet uses a segmented-control widget (not
tabs — needs a separate `<SegmentedControl>` component). Page tab conversions deferred to their
chartered waves (Analytics → Wave 2; Alerts/Settings → Wave 4; Reports → Wave 5).
C7 WCAG finding documented in wave conflict ledger: (b) and (c) fixed in Wave 0;
(a) light-theme CTA fails AA (3.12:1) — **NO WAIVER EXISTS. Filed as operator gap G3**
(pre-existing at 2f53414; the fix is a `tokens.json` change, and brandkit is the operator's
per D-071 — a session may not self-approve it). *(Corrected S33/D-095: this line previously
read "AA waiver granted". It was never granted — an S31 agent's draft falsely claimed the
operator had approved it, D-093 corrected that in three places, and this fourth copy survived.
A stale false claim in a plan of record is how the next session gets it wrong.)*
SRT publishType now KNOWN: AMS BroadcastDTO returns
`publishType="RTMP"` for SRT-ingested streams (F5 live finding, D-093); Pulse mirrors AMS
verbatim — SRT ingest is counted as RTMP in protocol breakdown until a heuristic is built.

**Wave 1 DONE — S32 (D-094, 2026-07-14):** LiveOverview + QoE. Chart hex → `CHART_COLORS[N]`
(same hex); stale hex fallbacks dropped from `var(--color-warning, #hex)`; a11y — StatCard
accessible names, donut aria-labels, `role=grid/rowgroup/row/columnheader` on StreamsTable.
Established the **px→token EXACT-MATCH rule** (the `--space-*` scale is 4/8/12/16/24/32/48/64/96;
a non-matching literal is LEFT ALONE — snapping 13px→12px is a silent regression).
**⚠ Wave 1 shipped incomplete and it was not caught until S33** — see below.

**Wave 2 DONE — S33 (D-095, 2026-07-14):** Analytics + Fleet + shared `<SegmentedControl>`
(`role=radiogroup`, **never `tablist`** — a tablist promises tabpanels that do not exist) +
`<StatCard size="compact">` (a 1:1 swap was **not** pixel-neutral: padding 14→24px, value
24→40px). 3+2 chart hex → `CHART_COLORS[N]`; Fleet's memory-healthy bar stays **dataviz blue,
never `statusColors.healthy`**; 18 px → `--space-*` exact-only; `--color-muted` eliminated from
both pages and from the shared `Badge`/`StatCard` (it fails AA at every size these pages use).
NEW e2e `analytics.spec.ts` + `fleet.spec.ts` (neither page had one). 548/548 web tests.

**★ S33 also fixed a Wave 1 ESCAPE: S32 gated a tree it never committed.** PR #46 was still
open at S33 open, and its branch was missing the `global.css` rule that `QoePage.tsx`'s
committed comment and tests both promised. **The gates had run green against a working-tree
file that never entered git.** Guard added (`styles/__tests__/focus-rings.test.ts`) pinning
both halves of every className↔stylesheet contract. **Standing rule: a session claiming DONE
is not evidence that it merged — check `origin/main` and open PRs at every session open.**

**★ THREE NEW OPERATOR GAPS from Wave 2 (all independently verified):**
**G4** touch targets — brandkit's `minTouchTarget=44` is WCAG **AAA**; the **AA** bar is 24×24,
which today's ~28px controls already pass. Enforcing 44 visibly rethemes every button and fights
brandkit's own desktop-density spec; coupled to G1. **Deferred, not skipped.**
**G5** — **the brandkit WCAG table itself is wrong**: design-rationale §2 (BINDING) claims muted
= ~4.6:1 AA; the true ratio is **3.72:1**, *below* AA for normal text. Every future wave reads
that table. **G6** — light-theme info Badge = **2.32:1** (`--color-info` intentionally not
overridden for light); needs a `color.light.info` token. G3/G5/G6 are all brandkit edits →
**operator-gated (D-071)**.

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

### S12 — infrastructure scaling: Postgres meta backend + WebRTC probe + brand adoption (+ S11 carries) ✅ DONE (D-072, 2026-07-10)
**Result:** ALL work orders landed — WO-A Postgres meta backend (pgx/v5, rebind, embedded
PG DDL, 19-test parity suite green in CI vs postgres:16 service); WO-B WebRTC signaling
probe phase 1 (real connect_time_ms in CI e2e — "PASS: WO-B" evidenced; pion media path →
S13); WO-C keep-7 cycle-8 prune observed + restore-verified; WO-D date-gate skip
re-recorded; WO-E clean-install release test PASSED (182s vs 15-min budget; 7 more doc
bugs fixed); WO-F enforce_admins rationale re-recorded; WO-G brandkit phase 1 shipped
(tokens/fonts/identity/components/charts; NO CSP change needed — trap dissolved by scout);
+ optional PDF-logo swap. 3 workflows (3 scouts / 7 authors / 3 adversarial verifiers —
verdicts PARTIAL×3, all 10 findings fixed-or-dispositioned same session incl. a CRITICAL
always-False e2e poll condition caught BEFORE push). Prompt: `sessions/SESSION-12.md`.
**Goal (as planned):** Unlock HA deployments; extend probe coverage beyond HLS; adopt the brandkit in the
web UI (operator-directed, D-071); drain the carry queue.

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
7. **WO-G [M–L, OPERATOR-DIRECTED, D-071]** Brand adoption phase 1 (§2.15) — brandkit →
   web UI: tokens → `global.css` + hardcoded-hex sweep (incl. the pinned FleetPage test),
   self-hosted IBM Plex woff2 + CSP updated ATOMICALLY with `csp.spec.ts`, favicon/PWA/logo
   identity (`web/public/` from scratch), component + recharts restyle per design-system;
   optional [XS] embedded PDF default-logo swap (BE-02). **Non-droppable** (operator
   directive: brandkit ships in this session); if the session runs hot, WO-B (WebRTC probe)
   yields to S13 BEFORE WO-G shrinks.

**Exit:** `PULSE_META_BACKEND=postgres` boots and passes migration parity tests in CI;
WebRTC probe returns a real result (not `not_probed`) for a WebRTC stream in CI; web UI
renders the Pulse brand (tokens/fonts/logo/favicon live; web coverage gates + Playwright
dashboard/csp specs green); carries executed or re-gated with evidence.

---

### S13 — probe protocol completion + promotions (REVISED at S12 close, D-072) ✅ DONE (D-073, 2026-07-10)
**Result:** WO-B RTMP handshake probe phase 1 (stdlib-only, zero deps, strict S2-echo
LIVE-VERIFIED vs real AMS 3.0.3) · WO-C DASH probe (full MPD+segment, SegmentTemplate/
SegmentList/BaseURL-chain, timescale-adjusted bitrate; spec-derived fixtures — AMS DASH
muxing disabled, capture gap recorded) · WO-F probe_results TTL → {retention_days}
(0001 fix + CH 0006, RED→GREEN integration test at RetentionDays=33) · WO-D pion
**RE-GATED to S14 with triage record** (cold-start dep ×2 modules, mock-ams answerer =
[M] on its own, fixture server→client-only) · WO-A date-gate skip re-recorded (07-10 <
07-23) · WO-E did NOT fire ("ship v0.3.0" unanswered) · WO-G rationale re-recorded.
3 workflows (4 scouts / 6 authors / 3 verifiers — CONFIRMED_OK ×2 + PARTIAL; live
cross-pair real-probe↔real-mock PASSED; findings: DASH BaseURL chain fixed + doc sweep).
Session opened by completing S12's interrupted close (terminal crash mid-close; no work
lost). Prompt: `sessions/SESSION-13.md`.
**Goal (as planned):** Complete probe protocol coverage (RTMP + DASH + WebRTC pion phase 2); land the
date-gated CI promotions (≥07-23); conditional v0.3.0 prod rollout. Mobile SDKs MOVED to
S14 and are operator-gated.

---

### S14 — pion media path + OIDC phase 2 + promotions ✅ DONE (D-074, 2026-07-10)
**Goal:** WebRTC media-path QoE (pion phase 2a/2b per the D-073 triage spec); SPA OIDC
login; CI promotions (date gate ≥07-23 opens during/near S14); conditional v0.3.0 rollout
(operator-gated, still pending); anomaly metric expansion. Mobile SDKs remain
operator-gated (§2.12 uncut until answered). Full prompt: `sessions/SESSION-14.md`.

1. **WO-A [S, ≥07-23]** CI promotions (§2.7) + CodeQL-answer check (carry ×2).
2. **WO-B [L]** WebRTC pion media path (§2.11): phase-2a = pion dep (server + mock-ams),
   ICE-connected assertion, `ice_state` field + CH 0007; phase-2b = rtt/jitter/loss stats
   (RTCP, needs ~2s RTP); live fixture capture (client→server shapes) from real AMS.
3. **WO-C [M]** SSO/OIDC phase 2 — SPA login UI uses the D-070 cookie flow.
4. **WO-D [M]** anomaly metric expansion (§2.14) — needs manifest-owner ruling first.
5. **WO-E [M, operator-gated "ship v0.3.0"]** prod rollout (now carries D-068/D-070/
   D-072/D-073) + post-rollout operator browser-accept of the re-branded UI.
6. **WO-F [S]** probe segment-body LimitReader hardening (HLS+DASH, D-073 verifier note —
   truncation must not silently corrupt bitrate).
7. **WO-G [XS]** enforce_admins/PR-first re-check (standing).
8. **WO-H [L, operator-gated]** iOS beacon SDK phase 1 — ONLY on explicit "need mobile
   SDKs: yes".

*(Backlog-if-light: brandkit phase 2 light theme; DASH live-fixture capture if operator
enables DASH muxing.)*

**S14 result (D-074):** WO-B phase-2a ✅ (pion v4.2.16 CGO=0; ice_state vertical; e2e ICE
120s/5s; live ICE-connected vs real AMS + the notification-skip fix for the live-only
D-072 signaling bug) · WO-C ✅ (SPA cookie login + SSO) · WO-D ✅ (+2 metrics, owner
anomaly→BE-02) · WO-F ✅ (32MB cap) · WO-G ✅ re-recorded · WO-A skip ×3 (date) · WO-E/
WO-H gated (operator). Phase-2b → S15. Coverage 74.4/62.96-59.04-52.05. 3 workflows,
14 agents, ~1.31M tok; verify: CONFIRMED_OK + PARTIAL×2, zero functional must-fix.

### S15 — pion phase-2b + carries ✅ DONE (D-075, 2026-07-10)
**Result:** WO-B phase-2b LANDED + LIVE-EVIDENCED (rtt_ms=0.47/jitter_ms=22.33/loss_pct=0
vs real AMS in 2.2 s); verify CONFIRMED_OK + PARTIAL×2, zero functional must-fix, ~20
findings fixed same-session (incl. probes.md MUST-FIX + the alert async-guard contention
flake caught at gate); WO-A skip carry ×4 (07-10 < 07-23 — gate OPEN by S16); WO-C/WO-F
did NOT fire (operator answers still open); WO-D brandkit-2 → S16; Go 74.5 (floor 70.2).
Prompt: `sessions/SESSION-15.md`; ledger: decisions.md D-075.

Execute `sessions/SESSION-15.md`. Check operator answers FIRST (v0.3.0 / CodeQL /
PR-first / mobile-SDK — all four still open at S14 close).

1. **WO-A [S]** CI promotions (§2.7) — the ≥2026-07-23 date gate OPENS before/during S15
   if run on schedule; JOB-level streak re-measure; FULL-LIST PUT; GET-diff proof;
   CodeQL only with explicit operator OK. (Carry ×3: S12/S13/S14.)
2. **WO-B [M]** pion phase-2b (§2.11, D-074 triage): mock-ams sends RTP over the existing
   VP8 track (~2s); probe reads inbound-RTP stats (jitter/loss) + ICE-candidate-pair RTT;
   contract CR rtt_ms/jitter_ms/loss_pct; CH **0008**; e2e asserts stats present.
   FIRST to yield if hot (same rule as S14).
3. **WO-C [M, operator-gated "ship v0.3.0"]** prod rollout — now carries D-068+D-070+
   D-072+D-073+D-074+**D-075**; §8.8 smoke + runbook; post-rollout operator browser-accept.
4. **WO-D [S]** brandkit phase 2 (light theme, §2.15 backlog) — if time permits.
5. **WO-E [XS]** enforce_admins/PR-first re-check (standing).
6. **WO-F [L, operator-gated]** iOS beacon SDK phase 1 — ONLY on explicit yes.

*(Backlog-if-light: DASH live-fixture capture if operator enables DASH muxing; post-U3
beacon-QoE anomaly metrics (§2.14 revisit).)*

---

### S15b — operator answer batch ✅ DONE (D-076, 2026-07-11)
**Result:** v0.3.0 SHIPPED + LIVE (first tag blocked by the Trivy gate — go-jose
CVE-2026-34986 fixed same session); U3 RESOLVED (two live-only root causes: missing
prod env wiring + private-key-instead-of-license; enterprise perpetual minted; chain
verified beacon 202 → qoe/summary); CodeQL → required; PR-first FLIPPED
(enforce_admins=true, reviews 0, 9 contexts); mobile SDKs deferred; DASH fixture
skipped; NEW binding operator directive: **max 2 pushes/session**. Ledger: D-076.

---

### S16 — CI promotions gate-check + brandkit phase 2 + probe-stats UI ✅ DONE (D-077, 2026-07-11)
**Result:** WO-D protection unchanged; WO-A gate CLOSED (07-11 < 07-23) → skip carry ×5,
but the streak audit found `web-e2e` RED ×12 (not flaky — deterministic D-074 AuthGate
fail-open on SPA-fallback 200 /auth/me, masked by continue-on-error; prior "passed on
PR #27" ledger claim corrected) → WO-FIX added + landed (JSON shape-guard + /auth vite
proxy, TDD); WO-B brandkit phase-2 LANDED (light theme [15/15 exact tokens], density
default/compact/wall, motion + reduced-motion, sidebar toggle+segment control, Badge/
status-color sweep, StreamsTable 44→40 density-aware); WO-C probe-stats UI LANDED
(ice_state badge + rtt/jitter/loss, absent=dash, 0=valid). Session survived a terminal
crash mid-workflow (verifiers re-ran verbatim from the persisted script; no work lost).
Verify PARTIAL×2+REFUTED → 3 must-fixes applied; Playwright-docker gate caught 3 spec
bugs → 15/15; coverage 65.80/61.13/54.85 (all ↑). ★ NEW operator directive mid-session
(D-078): **Pulse × AMS real-validation & product-fit program** — plan docs authored
under `docs/assessment/` (5 docs: program README, capability map, validation
environment, scenario matrix, session plan); EXECUTION starts S17.
Prompt: `sessions/SESSION-16.md`; ledger: decisions.md D-077 + D-078.

### S16 (original plan) — CI promotions (date gate OPENS 2026-07-23) + brandkit phase 2 + probe-stats UI (planned at S15 close, D-075)

Execute `sessions/SESSION-16.md`. Check operator answers FIRST (v0.3.0 / CodeQL /
PR-first / mobile-SDK — all four still open at S15 close).

1. **WO-A [S, gate ≥07-23]** CI promotions (§2.7) — the date gate OPENS 2026-07-23;
   JOB-level streak re-measure; FULL-LIST PUT; GET-diff proof; CodeQL only with explicit
   operator OK; also assess web-e2e → required (green since D-055, ~2 weeks by 07-21).
   (Carry ×4: S12/S13/S14/S15.)
2. **WO-B [S–M]** brandkit phase 2 (§2.15 backlog): light theme, density, motion —
   tokens.json is authoritative; WCAG table binding.
3. **WO-C [S]** probe-stats UI surface (D-075 verifier backlog): ProbesPage results
   panel shows ice_state badge + rtt_ms/jitter_ms/loss_pct for WebRTC probes (types
   already generated; key-absent = render dash).
4. **WO-D [XS]** enforce_admins/PR-first re-check (standing rationale-or-flip).
5. **WO-E [M, operator-gated "ship v0.3.0"]** prod rollout — carries D-068+D-070+D-072+
   D-073+D-074+D-075; §8.8 smoke + runbook; post-rollout operator browser-accept.
6. **WO-F [L, operator-gated]** iOS beacon SDK phase 1 — ONLY on explicit yes.

*(Backlog-if-light: DASH live-fixture capture if operator enables DASH muxing; post-U3
beacon-QoE anomaly metrics (§2.14 revisit); RTMP AMF0 connect round-trip (§2.11 tail).)*

---

### S17 — validation program launch ✅ DONE (D-079, 2026-07-11)
**Result:** WO-A LANDED — `qa/realams/` harness (7 helpers + 26 P0 scenario scripts +
Makefile, `make validate-realams-p0`) built via 12-agent workflow + adversarial verify;
**P0 executed against the LIVE AMS: 24 PASS / 2 SKIP / 0 FAIL** (SKIPs honest:
TC-APP-02 no blocked app exists; TC-V-02 headless WebRTC playback never registered —
S18 item). Headline parity: publish→Pulse 4 s, stop→Pulse 7 s (PRD ≤10 s); bitrate
÷1000 ±10% holds; probes WebRTC/RTMP/HLS live-green incl. rtt/jitter/loss key-present;
fleet standalone honest-absent holds. **Suite run 1 false-greened 17 scenarios**
(auth.sh exit-on-source; D-028 class) → runner now requires fresh verdict.txt for PASS
(+ jq `//`-on-boolean and `grep -c || echo 0` landmines fixed; memory saved).
**Live AMS drift caught (program working as designed):** app inventory 16→4 (all
open), applications/info → 405, HLS at flat `{id}.m3u8`, implicit RTMP broadcasts
DELETED on stop (404, never `finished`), versionType="Enterprise Edition" — all folded
into scenario-matrix ⚠ S17 Corrections. Bugs filed: BUG-001 (BroadcastStatistics dead
code), BUG-002 (recording_gb=0 webhook-blocked; real test VoD created on pulse-test as
standing ground truth, mp4 setting restored). AV triage: 9 CONFIRMED live. WO-B gate
CLOSED (07-11 < 07-23) → skip carry ×6 (csp-e2e 30/30 green; web-e2e clock restarted at
S16 merge). WO-C LANDED (6 UI-text #58A6FF → var(--color-info); border token; 21 unit
pins → 360 tests; light value escalated to proposals/D-079-linkbody-token-proposal.md
§7 — no invented colors, D-071). WO-D: protection/dependabot/prod all clean.
Prompt: `sessions/SESSION-17.md`; ledger: decisions.md D-079.

### S18 — validation program P1 + doc-gaps ✅ DONE (D-080, 2026-07-11)
**Result:** WO-A LANDED — 24 P1 scenario scripts + `make validate-p1`; **P1 final
21 PASS / 3 SKIP / 0 FAIL**; P0 upgraded to **25/1** (TC-V-02 fixed: detached
Playwright container died on missing NODE_PATH — invisible under `docker -d`).
**Pulse bugs filed: BUG-003** (probe scheduler near-duplicate result rows) +
**BUG-004** (/qoe/ingest declares-but-ignores from/to — contract violation).
**Env/AMS findings:** VPS AMS caps at ~5–7 concurrent RTMP streams (stress
scenarios ENV-LIMIT-skip w/ capacity probe; bigger AMS needed for TC-S-01/L-05);
hlsViewerCount = sliding request-window (~9× session inflation, >90 s expiry lag);
RTMP/TCP masks netem loss (packetLostRatio is UDP-only); settings mutate = POST.
Fix round (5 diagnose agents, all retested green) caught 4 more shell/API
landmines (memory updated). **WO-B LANDED:** documentation-gaps.md (DG-01..18 +
S19 authoring plan). WO-C skip carry ×7 (delta green). WO-D clean; prod untouched.
Prompt: `sessions/SESSION-18.md`; ledger: decisions.md D-080.

### S19 — D-078 Phases 7+8 ✅ DONE (D-081, 2026-07-11)
**Result:** **WO-A LANDED — `docs/assessment/prd-validation-matrix.md`**: F1–F10
feature-level 1 FULLY (F10) / 9 PARTIALLY; 66 sub-rows 40 FULLY / 14 PARTIALLY /
7 DIFFERENTLY / 4 MISSING / 1 NC; numeric N1–N36 33/1/2 — every verdict
evidence-cited, adversarially verified (3 must-fix caught & fixed, incl. a
FAIL-run evidence citation and a missing PRD acceptance-criterion row).
**WO-B LANDED — `final-assessment.md` DRAFT**: completeness **60.6% strict /
79.9% weighted / 91.7% numeric**; marketplace checklist 17 rows (5
NEEDS-OPERATOR-CONTACT, 1 FAIL = BUG-002); 13-item prioritized roadmap (P0:
BUG-002 VoD REST poll, D-V2-1, BUG-004); 5 open questions for Ant Media.
**→ operator action produced: review the draft (operator-expected.md).**
**WO-C LANDED — top-3 doc gaps authored:** DG-04 + DG-11 → AMS-INTEGRATION.md,
DG-07 → NEW `docs/beacon-sdk.md` (verifier killed a fabricated D-V2-1 "third
option" + 2 stale dist filenames + missing BUG-004 caveat). WO-D skip carry ×8
(07-11 < 07-23). WO-E clean; prod + AMS untouched (session ran PRE-expiry —
authed baseline Enterprise 3.0.3 at 18:2xZ; post-expiry sweep → S20).
Prompt: `sessions/SESSION-19.md`; ledger: decisions.md D-081.

### S20 — P0 bug fixes ✅ DONE (D-082, 2026-07-12)
**Result: both P0 code bugs FIXED.** **BUG-004** (`fix(api)`): `/qoe/ingest` now
honors the `from`/`to`/`app`/`stream`/`node` params it declared and discarded;
contract UNCHANGED. **★ Prod impact found while fixing** — the web Ingest page sends
`from=now-15min&to=now` on every load, so REAL dashboard charts were era-mixed, not
just tests. Residual → **BUG-005** (`interval`, same declared-but-ignored class).
**BUG-003** (`fix(prober)`): **the filed root-cause hypothesis was WRONG** — no
"immediate run on create" goroutine exists; the 60 s refresh loop cancel+respawned
EVERY probe on EVERY tick even when unchanged, and the respawn fires immediately
(prod `MaxJitterFraction`=0) → duplicates every 60 s + a silent phase reset on every
refresh. Fix = skip respawn on unchanged config + FakeClock-drivable refresh; all 3
filed fix suggestions REJECTED as symptom-hiding (D-042).
**★ The workflow partially DIED on the weekly subagent limit** (BUG-003 author wrote
code+tests, died before gating) — **ORCH gated inline and re-derived the missing RED
proof** in a pristine copy (pre-fix → 5 fires where 4 expected). Gates: 24/24 pkgs
`-race`, 0 FAIL / 0 SKIP; coverage **74.5% → 74.8%**. **BUG-002 design note** landed
and **corrects final-assessment §5** (needs TWO additive migrations, not "no schema
change"). Sweep **re-gated to S21** (S20 ran pre-expiry again). Skip carry ×9.
**⚠️ Concurrent-session incident #2:** foreign caddy commit preserved on
`caddy-bedirhan-vhost`; `origin/main` now lacks a vhost live prod HAS → operator call.
Prompt: `sessions/SESSION-20.md`; ledger: decisions.md D-082.

### S21 — BUG-005 + parameter-conformance class fix ✅ DONE (D-083, 2026-07-12)

**Result:** **BUG-005 FIXED** (`fix(api)` `2e9d026`, TDD): `/qoe/ingest` honors
`interval` (hour→3600 s, day→86400 s; absent keeps the 60 s default — documented
F4 deviation from the spec default). Contract UNCHANGED. **★ THE CLASS FIX
LANDED — `param_conformance_test.go`**: enumerates all **85** declared query
params, fails on any unclaimed one; 11 probes / 47 exempts / **27
known-violations pinned**; anti-vacuity floors; mutation-verified (3 mutation
classes all go RED). **★ Sweep yield: 28/85 declared params were not honored** —
BUG-006 (pagination dead ×8 endpoints), BUG-007 (cursor-only ×2), BUG-008
(/anomalies drops all 6 filters), BUG-009 (tenant/cursor dropped INSIDE
query.LiveOverview/LiveStreams — verifier catch one layer deeper), BUG-010
(reverse: `?format=csv` implemented, undeclared). Gates: 24/24 `-race` 0 FAIL /
0 SKIP; coverage **74.8 → 74.9** (floor 70.2). **Post-expiry sweep re-gated to
S22 BY OPERATOR DIRECTION** (S21 opened 01:30Z, still pre-expiry; operator chose
new-session-later over an 8.6 h hold) — zero-cost re-gate: sweep tool committed
(`qa/realams/harness/expiry-sweep.sh`, output byte-identical to baseline),
pre-expiry diff base on disk, baseline re-confirmed ×3. Skip carry ×10. No
concurrent-session incident. Prompt: `sessions/SESSION-21.md`; ledger:
decisions.md D-083.

### S22 — post-expiry sweep + conformance-debt fixes ✅ DONE (D-084, 2026-07-12)

- **WO-A DELIVERED — post-expiry sweep: NULL DELTA (byte-identical).** Opened
  05:23Z (pre-gate) → HELD OPEN per spec (no 4th re-gate); clock monitor fired
  12:10:03Z; sweep 12:11Z. Only diff = teststream offline — crashed 07:10Z,
  5 h PRE-lapse (ffmpeg, S14 class); restarted as a live probe → **AMS
  ACCEPTED an RTMP publish post-lapse**; re-sweep byte-identical to baseline.
  Blocked-scenario list EMPTY. Hypothesis pinned: enforcement may bite at AMS
  process restart — observe-only.
- **WO-C DELIVERED — conformance debt 27→4 known-violations (all TDD,
  mutation-verified):** BUG-006 FIXED (keyset limit+cursor through 8 list
  endpoints + store layer); BUG-007 FIXED (cursor: alerts/history +
  probe-results, real probes not exempts); BUG-009 PARTIAL (LiveStreams cursor
  decode + stability sort; tenant ×2 stays pinned — no tenant data model, F6);
  BUG-010 FIXED (the ONE contract CR: `format` json|csv on /analytics/audience
  + text/csv, gen:api idempotent); BUG-008 PARTIAL (app/stream/limit/cursor
  post-filter + pagination; from/to → S23 flag-event-store design, triage doc).
  Registry census 29/8/49 → **35 probe / 4 known-violation / 47 exempt**;
  minProbes 8→33; minSpecParams 85→86.
- **★ Verifier catches fixed in-session: TWO panics** — stale-cursor OOB in
  LiveStreams (`items[10:2]`) + `?limit=-1` slice panic → HTTP 500 in
  alert-history. Both red-first, both clamped. 5/5 remediation spot-mutations
  RED.
- WO-B: no operator answers (re-surfaced). WO-D did NOT fire (no room —
  remediation consumed it; → S23 primary). WO-E skip carry ×11. WO-F green.
- Workflows: 16 agents (12+4), 0 errors, ~1.28M subagent tokens.

### S23 — BUG-002 VoD REST-poll build + BUG-008 phase-2 design ✅ DONE (D-085, 2026-07-12)

All five WOs executed (SESSION-23.md; open checks clean — no concurrent-session
incident, s23open sweep byte-identical, no post-lapse antmedia restart):
1. **WO-A DONE — ★ BUG-002 FIXED, live-validated:** amsclient ListVods(Paged)
   (verbatim live-capture fixture) + restpoller.pollVods (12-tick cadence,
   tick-0 backfill, persistent seen-set on stable `vodId` — the live probe at
   open resolved all 5 design-note OQs) + mv_recording_1d (CH 0009) +
   vod_poll_state (meta 0003). TC-REC-01 3/3 PASS vs real AMS
   (recording_gb 0.02% reconciliation). Dedup-bypass + restart-resume pinned;
   5 mutation proofs; at-most-once mark-then-emit.
2. **WO-B DONE (design):** ADR-0009 anomaly flag-event store, Proposed;
   migration 0010; build DEFERRED (Effort L vs build-only-if-Small) → S24
   primary if approved.
3. **WO-C DONE:** assessment refreshed for S20–S23 — completeness
   60.6/79.9 → **65.2 strict / 83.0 weighted**; marketplace "No P0 open
   bugs" FAIL→PASS; only BUG-001 (low) open; stays DRAFT.
4. **WO-D skip carry ×12** (07-12 < 07-23).
5. **WO-E green** (protection/dependabot/prod-health). pulse-realams reset +
   now runs the S23 build. Prod untouched; a rollout now carries
   D-082+D-083+D-084+D-085.

### S24 — BUG-008 phase-2 build (ADR-0009 flag-event store) ✅ DONE (D-086, 2026-07-12)

All four WOs executed (SESSION-24.md; open checks clean — s24open sweep
byte-identical [3rd null delta], no post-lapse antmedia restart, no
concurrent-session incident; WO-A fired on the plan-approves path — no
operator answer, ORCH ruling recorded in D-086):
1. **WO-A DONE — ★ BUG-008 FULLY FIXED (Group B), ADR-0009 Accepted:**
   CH migration 0010 `anomaly_flag_events`; write path in the UpdateBaselines
   tick (shared detectFlagsLocked, detected_at = tick time, inserts outside
   d.mu, at-most-once); WarmHysteresis restart dedup; QueryFlagHistory
   (base64 keyset cursor, **toUnixTimestamp64Milli comparison — clickhouse-go
   sends time.Time params second-precision, which duplicated page boundaries;
   live-observed RED, fixed + structurally pinned**); /anomalies routes
   ?from/?to on raw presence (400 FLAG_STORE_NOT_CONFIGURED / BAD_REQUEST);
   metric/app/stream/min_sigma honored on the history path (ADR amendment).
   **Conformance: 37 probes / 2 known-violations (both BUG-009 tenant),
   minProbes 33→35.** 3 verifiers (V3 CONFIRMED_OK; V1/V2 must-fix →
   remediated same-session: skip→fatal pin, same-second pagination fixture,
   ADR amendments g/h); **9/9 mutation proofs RED + 2 re-derived** vs
   strengthened pins in pristine worktrees (A1 stalled mid-build and was
   auto-retried — its retry gated the predecessor tree per D-082).
2. **WO-B DONE (ruling):** no P2 Makefile list (validate-all auto-discovers;
   PULSE_HAS_VOD_POLL stays an explicit attestation). TC-REC-01 re-run vs the
   realams stack: **3/3 PASS, recording_gb stable after ~3 h of poll cycles**
   (seen-set no-double-billing holds live). recording_method CR not fired.
3. **WO-C skip carry ×13** (07-12 < 07-23).
4. **WO-D green** (protection/dependabot/prod-health read-only). Gates:
   24/24 -race 0 FAIL (3 pre-existing env-gated skips; D-028 class 0),
   coverage 76.0→**75.5** (≥70.2 floor; dilution = ~190 new CH-store lines
   are integration-covered, not unit-covered), gofmt/vet/contract-drift
   clean, full integration green (10 migrations idempotent). Prod untouched;
   a rollout now carries D-082..**D-086**.

### S28 — operator-intake gate + marketplace tail ✅ DONE (D-090, 2026-07-13)

1. **Intake:** all 5 operator items re-verified OPEN (7th null-delta sweep;
   GHCR anonymous 401); v0.4.0 release confirm-only PASSED; NEW item 6
   seeded (Pro MaxNodes pricing ruling — enforcement already built).
2. **Docs:** kafka-integration.md NEW (DG-15, code-authoritative,
   AV-15-BLOCKED honest; V2 caught 2 real behavior errors incl.
   first-start FirstOffset replay) + AMS-INTEGRATION 4-tier de-stale
   (~30 fixes) + DG-05 stub; DG rows marked AUTHORED.
3. **Assets:** render-screenshots.mjs (hermetic brandkit render;
   SS1/SS2/SS4 done, SS3/SS5/SS6 operator-manual; brandkit CDN-font
   violation filed); PNGs gitignored.
4. **Code:** §2.17.2 canonical-set parity guard (RED re-derived
   independently) + §2.17.3 Option B contract CR ("down" dropped;
   FleetPage dead tile removed) + §2.17.1 RULED keep+document + §2.5
   stamped already-DONE-S10 (ledger drift, 2nd find).
5. **Ops:** realams rebuilt on v0.4.0 (fresh token, orphan gotcha
   cleared); 14 agents 0 errors; 24/24 -race, coverage 76.1, web
   388/388+lint; promotions skip carry ×17.

### S27 — ★ operator marketplace directive: rollout + trial lifecycle + quickstart + docs pack ✅ DONE (D-089, 2026-07-13)

Operator prompt = the intake ("rollout quick … marketplace asap … trial
license key"). Delivered: prod rollout D-082..D-088 (runbook path, boot
self-proofs); license lazy-expiry lifecycle (NO contract CR — three states
fit LicenseInfo; live mid-run expiry proven, 7/7 mutations RED);
deploy/quickstart/ one-command install (migrations baked; live
clean-install vs real AMS); web TrialBanner + LicenseContext (388 tests);
docs/compatibility.md + known-limitations.md + marketplace/ drafts;
checklist 16/17→PASS; scores 66.7/84.5 (verifier-re-derived); v0.4.0
tagged (LOAD-BEARING for the quickstart pin). 11 agents, 0 errors.
Full evidence: decisions.md D-089.

### S26 — early-warning polish batch (§2.16 tail) + zero-mean guard ✅ DONE (D-088, 2026-07-13)

All WOs executed (SESSION-26.md; open checks clean — s26open sweep
byte-identical [5th null delta], no post-lapse antmedia restart; standing
backlog-review directive executed: plan confirmed, stretch widened by
scout findings):
1. **WO-A1: node-degraded predicate UNIFIED** — three drifted copies
   (wave2 alert / FleetNodes [CPU-only] / LiveOverview [no streak arm])
   → one `domain.LiveNodeStats.Degraded()`; an alerting node can no longer
   show "up" on the Fleet page. No contract CR; no web change.
2. **WO-A2/A3: standalone zero-mean baseline poison fixed cause+symptom** —
   presence flags (value==0 heuristic ruled out; anti-heuristic mutation
   pin) at all 3 eval sites + `DeleteZeroMeanNodeBaselines` boot sweep.
   **Live-validated on realams (meta preserved through rebuild): boot log
   `purged zero-mean baselines on startup count=3`; census 3→0; guard held
   over live ticks (api_latency n 801→803, node rows stayed 0).**
3. **Stretch:** BUG-001 deleted (**0 open bugs**); §2.4 found already
   delivered (ledger corrected); §2.17 seeded; PG sweep parity test added.
4. **Verify/gates:** 12/12 mutations RED (pristine copies); V2 confirmed
   prod sweep wiring ACTIVE; coverage 76.0 (floor 70.2); -race 24/24;
   integration green (CI-faithful CH 24.8 + postgres:16). WO-B skip carry
   ×15. 10 agents, 0 errors, one PR.

### S25 — AMS early-warning ladder (§2.16) + F9 sparsity gate ✅ DONE (D-087, 2026-07-12/13)

All WOs executed (SESSION-25.md; open checks clean — s25open sweep
byte-identical [4th null delta], no post-lapse antmedia restart; standing
backlog-review directive executed first time: plan confirmed, then WO-D
expanded to primary on scout evidence):
1. **WO-A (F9 beacon metrics) HONESTLY GATED** per its own assess-then-build
   clause: prod beacon_events = 2 smoke rows / realams 0; zero-variance
   first-event false alarm violates F9's acceptance; hourly rollup bucket
   accumulates ⇒ non-independent Welford samples (needs sub-hour windowing
   + real traffic). Gate documented (§2.14 / matrix F9 / assessment);
   scores unchanged 65.2/83.0; rebuffer_ratio exclusion pin untouched.
2. **WO-D DONE — ★ the 3-rung early-warning ladder (ant-media#7926 class):**
   `ams_api_latency_ms` poller-RTT anomaly metric (first live node-scoped
   metric on standalone AMS; key-absent-on-failure; budget 5×0.086=0.432<1.0)
   → API error-streak ≥3 → node_degraded (~15 s; was dead on standalone:
   cpu/mem never reported) → **BUG-011 FIXED: EvictStaleNodes was NEVER
   WIRED — node_down could never fire in ANY deployment** (also explains
   the S19 matrix downgrade). Load-bearing ruling pinned: failure events
   never refresh LastSeenAt (in-place streak update) so rung 2 can't starve
   rung 3. Map/switch parity pin kills the silent-nil metric trap class.
3. **Verify:** V2+V3 CONFIRMED_OK (contract text-only CR + gen:api; skip-
   when-0 parity ×3 sites; race ×3; ladder traced e2e; eviction blast
   radius safe). V1 PARTIAL → remediated: M4 GREEN_BAD fixed twice over
   (hardcoded-0 emission masked a missing reset; the replacement pin's own
   first draft was vacuous — caught by re-derivation, now RED at consec=3);
   M8 threshold multiplier extracted + pinned. 8 discrimination mutations
   + 2 re-derived. Latent AnomalyBaselineForMetric bug = dead code,
   TODO(D-087)-pinned.
4. **Gates:** 24/24 -race 0 FAIL (3 env-gated infra skips; D-028 class 0);
   coverage 75.5→**75.9** (floor 70.2); gofmt/vet clean; integration green;
   web 366 tests, gates met. Follow-up seeded: FleetNodes status ignores
   ConsecAPIErrors (display gap, V3 finding) → §2.16 note. Prod untouched;
   a rollout now carries D-082..**D-087**.

### S22 (original plan) — post-expiry sweep (operator-directed re-gate) + conformance-debt fixes (planned at S21 close, D-083)

Execute `sessions/SESSION-22.md`. **⚠️ OPEN AFTER 2026-07-12T12:10Z** — verify
the clock FIRST; if early, WAIT (do not re-gate a 4th time).

1. **WO-A [S, FIRST]** post-expiry sweep: `bash qa/realams/harness/expiry-sweep.sh
   postexpiry`, diff vs `evidence/S21-sweep-preexpiry-20260712T014135Z/stable.txt`
   → **D-084** delta + blocked-scenario list (a null delta is a real result).
2. **WO-B [S]** operator intake: caddy-vhost merge if approved; final-assessment
   edits if reviewed; else re-surface (non-blocking).
3. **WO-C [M]** conformance-debt fixes: BUG-006 (pagination through store layer),
   BUG-007 (cursor threading), BUG-009 (tenant/cursor in query layer) — flip each
   fixed registry entry known-violation → probe; BUG-010 contract CR (declare
   `format` on /analytics/audience, INT-01 scope, contract-first + gen:api).
   BUG-008 needs a ComputeFlags redesign — assess, split out if heavy.
4. **WO-D [S–M, backlog-if-light]** BUG-002 VoD REST-poll build (design note +
   two INT-01 migration CRs are written).
5. **WO-E [S, gate ≥07-23]** CI promotions — else skip carry ×11.
6. **WO-F [XS]** standing re-checks.

### S21 (original plan) — post-expiry sweep (finally real) + operator intake + BUG-005/class fix (planned at S20 close, D-082)

Execute `sessions/SESSION-21.md`. FIRST: the post-license-expiry read-only AMS sweep
— S19 AND S20 both ran pre-expiry and re-gated it; S21 is the first session after the
2026-07-12T12:09Z lapse, so it is no longer deferrable. Record the delta vs the
D-082 baseline in **D-083** + which scenarios become blocked (a null delta is a real
result — say so explicitly).

1. **WO-A [S, FIRST]** post-expiry sweep → D-083 delta + blocked-scenario list.
2. **WO-B [S]** operator intake: the `caddy-bedirhan-vhost` merge decision (main is
   BEHIND live prod Caddy until it lands) + final-assessment review; else re-surface.
3. **WO-C [M]** BUG-005 (`interval` declared-but-ignored) + **the class fix**:
   parameter-conformance contract tests (kin-openapi) that assert handlers honor every
   declared query param — CI lints the spec but never the handlers, which is exactly
   why BUG-004 and BUG-005 both slipped through.
4. **WO-D [S, gate ≥07-23]** CI promotions (csp-e2e candidate at the gate; web-e2e
   ~07-25) — else skip carry ×10.
5. **WO-E [XS]** standing re-checks.

*(Backlog-if-light: BUG-002 VoD REST poll — now a BUILD decision, design + its two
INT-01 migration CRs are written; remote-host WebRTC viewer; SRT loss; Kafka doc pair.)*

### S20 (original plan) — operator-review intake + post-expiry sweep + P0 bug fixes (planned at S19 close, D-081)

Execute `sessions/SESSION-20.md`. FIRST: post-license-expiry read-only AMS sweep
(trial lapses 2026-07-12T12:09Z — record what 403s/shrinks vs the S17–S19
pre-expiry baseline in D-082; note which scenarios become blocked).

1. **WO-A [S]** operator-review intake: if the operator has reviewed
   final-assessment.md → apply edits, resolve NEEDS-OPERATOR-CONTACT rows that
   got answers, finalize; else re-surface the request (non-blocking).
2. **WO-B [M]** P0 bug fixes from the assessment roadmap (code, TDD):
   BUG-004 (/qoe/ingest: parse from/to — make the handler honor the declared
   OpenAPI params; contract unchanged) + BUG-003 (probe scheduler duplicate
   rows — guard the immediate-on-create vs ticker race). Full §8 gates apply
   (Go tests -race repo-root mount, coverage floor).
3. **WO-C [S–M]** backlog-if-light: BUG-002 VoD REST-poll design note;
   remote-host WebRTC viewer for non-zero viewer-QoE parity; SRT publisher
   loss validation; TC-APP-02 if a blocked app exists; more DG authoring
   (DG-05+DG-15 Kafka pair next per the plan).
4. **WO-D [S, gate ≥07-23]** CI promotions (csp-e2e candidate lands AT the
   gate; web-e2e ~07-25) — else skip carry ×9.
5. **WO-E [XS]** standing re-checks + operator-answer sweep.

### S17 (original plan) — Pulse × AMS validation program launch (D-078) + CI-promotion date gate (planned at S16 close, D-077)

Execute `sessions/SESSION-17.md`. The operator's D-078 directive (real-validation &
product-fit program, plan of record `docs/assessment/session-plan.md`) is now the
primary track; CI promotions remain date-gated.

1. **WO-A [M–L, PRIMARY]** validation program Phases 1–2 (D-078): finalize the
   capability-map assumptions list, then BUILD the reusable real-AMS harness per
   `docs/assessment/validation-environment.md` (publisher control, viewer simulation,
   failure injection, AMS-vs-Pulse parity checker). Start executing the P0 rows of
   `docs/assessment/scenario-matrix.md` (broadcast lifecycle + viewer-count parity).
2. **WO-B [S, gate ≥07-23]** CI promotions (§2.7): if run on/after 07-23 → promote
   `csp-e2e` if still green (candidate lands exactly at gate); `e2e` separate decision;
   `web-e2e` clock restarted at S16's fix merge (earliest ~07-25). Else skip carry ×6.
3. **WO-C [S]** S16 verifier-findings backlog: ProbesPage delete-button border +
   #58A6FF UI-text literals (light-mode); ttfbColor()/iceVariant()/memStatus() unit
   pins; propose tokens.json color.light.linkBody upstream.
4. **WO-D [XS]** standing re-checks: protection/PR-first drift, dependabot queue, prod
   health (read-only), operator browser-accept follow-up.

*(Backlog-if-light: post-U3 beacon-QoE anomaly metrics (§2.14 — feeds the program's
viewer-analytics phase); RTMP AMF0 connect round-trip (§2.11 tail).)*

---

## 4. Operator decision ledger

> Items the operator must decide before the agent can act. Surface every session.
> Counterpart to ROADMAP.md §5.

| # | Decision | Status | Notes |
|---|---|---|---|
| D-V2-1 | **Unsigned-webhook ingest (§2.6):** build an optional IP-allowlisted unsigned mode vs keep REST-polling-only | **OPEN** | O3 closed-N/A (D-066): AMS 3.0.3 hooks unsigned — verified live. No build commitment. Agent awaits "build" or "wontfix". |
| D-V2-2 | **CodeQL as required CI context:** promote CodeQL to a required branch-protection context | **RESOLVED-ENABLED (D-076, 2026-07-11)** | Operator said "decide for me"; ORCH enabled: 29-run green streak since D-062, zero maintenance, Go+JS scanning on an exposed prod service. Contexts `Analyze (go)` + `Analyze (javascript-typescript)` required as of the D-076 protection flip. |
| D-V2-3 | **enforce_admins flip (§2.1):** flip `enforce_admins` to `true` once sessions stop pushing directly to main | **RESOLVED-FLIPPED (D-076, 2026-07-11)** | Operator said "PR-first going forward": enforce_admins=true + required reviews 1→0 (solo owner can't self-approve; contexts are the gate). Sessions from S16 on: branch → PR → contexts green → merge. |
| D-V2-4 | **U3 — activate Pro+ license in prod:** set `PULSE_LICENSE_KEY` in `deploy/.env` | **RESOLVED (D-076, 2026-07-11)** | Operator minted + placed the key; live-verify evidence (tier + beacon→QoE chain) in decisions.md D-076. |
| D-V2-5 | **O7 — GHCR package public:** make `ghcr.io/aytekxr/ams-pulse` public | **DOWNGRADED to optional (2026-07-10)** | Operator granted `read:packages` instead → S12 WO-E unblocked (pull + cosign verified live: image tag `0.2.0` — NO v prefix, doc bug fixed; Rekor 2128354996). Package stays private: only outside users can't pull/verify until the one UI click (no API path, D-066). |
| D-V2-6 | **Ship v0.3.0:** tag + prod rollout carrying D-068…D-075 | **RESOLVED-SHIPPED (D-076, 2026-07-11)** | Operator: "Let's proceed with v0.3.0." Tag at `ab9a5e1`; rollout + smoke evidence in decisions.md D-076. Browser-accept of the re-branded UI pinged post-rollout. |
| D-V2-7 | **Mobile SDKs needed?** native iOS/Android beacon SDKs (§2.12) | **DEFERRED (D-076, 2026-07-11)** | Operator: "leave them out for now, we'll revisit later." §2.12 stays on the roadmap marked deferred; the iOS work order is CUT from session plans until the operator re-opens it. |

---

## 5. Coverage ratchet (carry-forward from ROADMAP.md)

| When | Go total | CI floor | Web lines / branches / functions | Notes |
|---|---|---|---|---|
| 2026-07-09 GA (v0.2.0, D-065) | **73.2%** | **70.2** | 76 / 72 / 45 | Baseline for v2 plan; floor = achieved−3 per GA rule |
| 2026-07-09 S10 (D-068) | **73.5%** | **70.2** | 62.13 / 57.6 / 51 (gates 59/54/45) | Web numbers = vitest-4 re-baseline (D-067); sdk 66.06/45.79/70.42 (gates 63/43/67) |
| 2026-07-10 S11 (D-070) | **73.9%** | **70.2** | 79.69 / 76.25 / 47.33 (gates 59/54/45) | api 76.1, reports 90.1, query 87.5, meta 67.7; sdk untouched (66.06/45.79/70.42, 3.52 KB) |
| 2026-07-10 S13 (D-073) | **74.0%** | **70.2** | 62.68 / 58.78 / 51.54 (gates 59/54/45) | prober 70.1 (new rtmp+dash probes fully tested); web untouched (schema.d.ts JSDoc only — numbers are the vitest-4 rebaseline, the S11 row's 79.69 was the notation artifact); sdk untouched |
| 2026-07-10 S14 (D-074) | **74.4%** | **70.2** | 62.96 / 59.04 / 52.05 (gates 59/54/45) | prober 72.6 (ICE tests), anomaly 81.6, api 76.9, domain 100; sdk untouched (66.06/45.79/70.42, 3.52 KB) |
| 2026-07-10 S15 (D-075) | **74.5%** | **70.2** | 62.96 / 59.04 / 52.05 (gates 59/54/45) | prober 72.8 (RTP-stats tests), api 77.1, anomaly 81.6, domain 100; web untouched (schema.d.ts types/JSDoc only); sdk untouched (66.06/45.79/70.42, 3.52 KB) |
| *(update each session at close)* | | | | |
