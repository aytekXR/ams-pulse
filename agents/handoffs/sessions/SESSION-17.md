# SESSION-17 — Pulse × AMS validation program launch (D-078) + CI-promotion date gate (ROADMAP-V2 S17, planned at S16 close)

> Written by SESSION-16 close (D-077/D-078, 2026-07-11). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/ROADMAP-V2.md` (plan of record) + `RESUME-PROMPT.md` §7/§8/§12 AND
> **`docs/assessment/README.md` + `docs/assessment/session-plan.md`** (the D-078 program
> plan — the operator's primary track from S17 on) before dispatching.

## Mission

Execute ROADMAP-V2 §3 S17. Exit = (a) D-078 validation program Phases 1–2 delivered:
capability-map assumptions triaged, the reusable real-AMS harness BUILT and committed,
and the P0 scenario-matrix rows executing with AMS-vs-Pulse parity evidence; (b) CI
promotions decided-and-applied IF the ≥2026-07-23 gate is open at run time (else skip
carry ×6 with streak re-measure); (c) S16 verifier-findings backlog burned down;
(d) recurring re-checks done. ALL WORK VIA PRs (D-076 PR-first, ≤2 pushes, D-076
operator directive).

## Context you must load first

- **D-078 (operator directive, S16):** full 8-phase program — product understanding →
  real test env → e2e scenarios → automated parity validation → bug protocol → docs
  program → PRD matrix → marketplace assessment. Plan docs live in `docs/assessment/`
  (README, capability-map, validation-environment, scenario-matrix, session-plan).
  The operator wants a REAL validation environment ("simulate a real customer"):
  control AMS (create/start/stop broadcasts, ramp streams + watchers, failures,
  reconnects) and verify the effects in Pulse, cross-checking Pulse numbers against
  AMS APIs automatically — never trust the UI alone. Findings become issue docs
  another engineer could implement from.
- **Local real-AMS env:** `oguz-testing.md` (gitignored — how AMS + Pulse start, URLs,
  token location). Known AMS gotchas (agent memory + decisions.md): console MD5-hashes
  passwords in-browser (plaintext-provisioned accounts can't web-login); brute-force
  lockout 2 tries → 5 min, keyed by EMAIL not IP; AMS 3.0.3 webhooks are UNSIGNED (no
  HMAC — never point listenerHookURL at Pulse's fail-closed listener, O3/D-V2-1).
- **Playwright only runs in docker** on this host (`mcr.microsoft.com/playwright:v1.61.1-noble`,
  pre-pulled). Docker leaves root-owned artifacts in mounted dirs — clean via
  `docker run --rm -v <dir>:/s alpine rm -rf /s/<target>`.
- **S16 landed (D-077):** AuthGate fail-open fix (+ `/auth` vite proxy), light theme +
  density + motion (brandkit phase 2), ProbesPage WebRTC columns, web-e2e root cause
  fixed (its promotion clock restarted at the S16 merge).

## Work orders

1. **WO-A [M–L, PRIMARY]** D-078 Phases 1–2 per `docs/assessment/session-plan.md`:
   - Phase 1 close-out: verify/triage the capability-map "assumptions to validate"
     list against the live AMS (read-only API sweeps; capture fixtures to
     `agents/handoffs/real-ams-captures/` conventions).
   - Phase 2 build: the reusable harness per `docs/assessment/validation-environment.md`
     — publisher control (ffmpeg RTMP loops; WebRTC via Playwright-in-docker),
     viewer simulation (HLS pollers + WebRTC viewers, ramp profiles), failure
     injection (publisher kill, docker network disconnect, AMS/Pulse restart,
     invalid stream key, expired token), and the parity checker (AMS API snapshot
     == Pulse API snapshot or FAIL with evidence; tolerance windows for poll skew).
   - Start P0 scenario-matrix rows: broadcast lifecycle + viewer-count parity.
   - Every discrepancy → a bug doc per the program's Phase-5 protocol.
2. **WO-B [S, gate ≥2026-07-23]** CI promotions (§2.7): JOB-level streak re-measure
   FIRST; if gate open → FULL-LIST PUT + GET-diff proof; promote `csp-e2e` if still
   green; `e2e` is a separate decision (stable 40/40, intentionally non-required);
   `web-e2e` earliest ~07-25 (clock restarted at S16 fix). If before 07-23 → skip
   carry ×6 with evidence.
3. **WO-C [S]** S16 verifier backlog: ProbesPage delete-button border rgba →
   var-based; #58A6FF UI-text literals (light-mode contrast — classify dataviz-vs-UI
   first); unit pins for ttfbColor()/iceVariant()/memStatus(); draft the
   tokens.json `color.light.linkBody` proposal (brandkit change → needs operator/
   design sign-off; --color-link #087A59 currently sourced from design-rationale §2).
4. **WO-D [XS]** standing re-checks: branch protection drift (enforce_admins=true,
   strict, 9 contexts, 0 reviews), dependabot queue, prod health read-only
   (v0.3.0-4-ge8f8f5f, tier=enterprise), operator browser-accept follow-up (ping if
   still pending).

*(Backlog-if-light: post-U3 beacon-QoE anomaly metrics (§2.14 — directly feeds the
program's viewer-analytics scenarios; prefer folding INTO WO-A over standalone);
RTMP AMF0 connect round-trip (§2.11 tail).)*

## Preconditions (re-verify cheaply; note drift in decisions.md)

- Tree clean; S16 close PR merged; ci+e2e+codeql GREEN at HEAD.
- Standings (D-077): Go 74.5% (floor 70.2); web lines 65.80 / branches 61.13 /
  functions 54.85 (gates 59/54/45, vitest-4 — NEVER compare to pre-rebaseline);
  sdk untouched (66.06/45.79/70.42; gates 63/43/67; 3.52 KB).
- Prod: v0.3.0-4-ge8f8f5f healthy, ENTERPRISE license; rollback tags stand. S16's UI
  work reaches prod only with the next operator-approved rollout.
- Operator queue: 👀 browser-accept (standing); optionals D-V2-1/O7/O11/workflow-scope.

## Gates (ORCH, before any commit)

- Contract CR (if any) → redocly + ajv + gen:api drift (§8.6).
- Go touched → full `-race` repo-root mount, floor 70.2, 0 FAIL/0 unexpected SKIP,
  gofmt emptiness (gate on OUTPUT, `gofmt -l` exits 0 with findings), CGO_ENABLED=0.
- Web touched → lint + typecheck + coverage gates + build + Playwright-in-docker.
- New harness code (validation/ or qa/) → its own tests + a documented dry run against
  the live AMS (read-only first; publisher/viewer runs need the operator's AMS healthy).
- Prod untouched (read-only health checks only) unless the operator asks for a rollout.

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md D-079 evidence — append EARLY, commit handoffs FIRST.
3. RESUME-PROMPT ▶ START HERE → SESSION-18; ROADMAP-V2 §3/§4/§5 ledgers updated.
4. REFRESH `docs/operator-expected.md` + `docs/assessment/*` progress + PushNotification.
5. Write `sessions/SESSION-18.md` from ROADMAP-V2 §3 + `docs/assessment/session-plan.md`.
