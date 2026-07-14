# SESSION-33 — operator-intake gate + §2.19 Wave 2 (planned at S32 close, D-094)

> Written by SESSION-32 close (D-094, 2026-07-14). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/wave-uipro/WAVE-PLAN.md` + `ROADMAP-V2.md` §2.19/§2.18 +
> `RESUME-PROMPT.md` before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12): review the backlog, revise this plan

Before dispatching: re-read ROADMAP-V2 §2 (§2.19 Waves 0–1 are DONE; §2.18 item 6 is the
operator-gated remainder) + the final-assessment §5 roadmap and REVISE this plan if a
higher-leverage move exists. This file is a starting point, not a contract. Record any
revision in the D-095 open block. Carry this header into SESSION-34.md.

## Mission

Exit = (a) **operator intake applied** — SIX standing items (GHCR public flip, trial-key
mint, final-assessment review, Ant Media contact, MaxNodes ruling, matbu vhost ruling)
**+ THREE design gaps** (G1 mobile input font-size, G2 icon library, **G3 light-theme CTA
contrast — needs a `tokens.json` change, so ONLY the operator can authorise it**). If any
is answered → act; else re-surface.
**⏰ (b) LICENSE RENEWAL — the key expires 2026-07-27T13:45Z.** From ~07-25 this is the
top intake item: a lapse **+** the next AMS restart = total ingest death (both arms of the
model proven in D-092/D-093). Surface it every session until renewed.
(c) **§2.19 Wave 2 [M] [primary]** per WAVE-PLAN §4 W2: Analytics + Fleet.
(d) CI promotions if run date ≥ 2026-07-23 (else skip carry ×22).
(e) standing re-checks + AMS observation at open. PR-first, ≤2 pushes.

## S33 carries (from D-094; pick by leverage after intake)

1. **§2.19 Wave 2 (Analytics + Fleet) [M]** — primary. Includes:
   - Analytics: 3 chart strokes → `CHART_COLORS[]` (`#58A6FF`→[1], `#2CE5A7`→[0],
     `#FFB224`→[4]); px → `--space-*` **exact matches only**; inline stat-card grids →
     the shared `<StatCard>`; tabs ALREADY converted to `<Tabs>` in S31 — do not redo.
   - Fleet: 2× `#58A6FF` LoadBar → `CHART_COLORS[1]`; px → tokens (exact only).
   - **★ `<SegmentedControl>` extraction [XS]** — Fleet's cards/table toggle is a
     SEGMENTED CONTROL, **not** a `<Tabs>` candidate (S31 scout: fill-background active
     state, 11px, no underline — converting it to `<Tabs>` would change the design
     intent). Wave 2 is the wave that touches Fleet, so extract it here.
2. **G3 token fix [XS, operator-gated]** — the moment the operator says "apply the G3
   token fix": `tokens.json color.light.accent` → `#087A59` (5.33:1), cascade through
   `global.css [data-theme=light] --color-accent`, re-run the WCAG pass.
   **Do NOT touch `brandkit/` before the ruling (D-071).**
3. **Marketplace upload prep [gated]** — operator items.
4. **D-V2-1 unsigned-webhook ingest mode [operator decision first]** — re-surface only.

## ⚠️ Check these BEFORE dispatching anything

1. **`docs/operator-expected.md` ⚡ — six standing + G1/G2/G3.** G3 is the only one that
   blocks a code path (the light-theme contrast fix). **NEVER let an agent claim the
   operator approved/sanctioned/waived anything** — an S31 agent falsely wrote "Operator
   waiver granted" and it had to be corrected in three places (D-093).
2. **Concurrent-session hazard (D-062).** Foreign work → inspect → preserve → never revert
   working files. `deploy/config/Caddyfile.prod` (matbu block) stays UNCOMMITTED until the
   operator rules — it is the ONLY expected dirty file at open. The session shell may lack
   the docker group — `sg docker -c "…"`.
   **Never trust a tree a dead/stalled agent left** (D-082/D-086/D-091/D-093 — 4 occurrences;
   S31 found one on its own branch at open and the audit caught a vacuous test + 2 WCAG fails).
3. **AMS at open.** `bash qa/realams/harness/expiry-sweep.sh s33open` — NO PULSE_TOKEN
   prefix (S29 gotcha). Expect byte-identical to the pre-expiry baseline. **`ams-teststream`
   does NOT auto-restart across a VPS reboot** → `docker start ams-teststream`.
   **SRT publishes MUST use the plain streamid** `srt://<host>:4200?streamid=<App>/<streamId>`
   — the ACF form (`#!::h=`/`#!::r=`) is REJECTED by AMS EE 3.0.3 (D-093).
   **VPS load:** check `uptime` before gates AND before any publish scenario (AMS refuses
   publishes above 75% CPU; TC-I-05 has a SKIP arm for it).
4. **Prod runs v0.3.0-34-g58a9c84 since S27.** Read-only health check at open; next rollout
   carries D-089..D-094 (the UI waves are web-only — no runtime behaviour change).

## Gates (ORCH, before any commit)

- **★ RUN THE SPECS OF THE COMPONENTS THIS WAVE TOUCHES — not just the default gate list.**
  S32's regression was caught ONLY because `streams-virtualization.spec` (not in the §2.2
  default set) was added when the wave touched StreamsTable. For Wave 2, that means any
  Analytics/Fleet e2e specs on top of the default four.
- **★ px → token: EXACT matches only.** Scale = 4/8/12/16/24/32/48/64/96. A literal with no
  exact token (6/11/13/20/36/160/180/260/520px, all typography sizes) is LEFT ALONE and
  reported. Snapping is a silent pixel regression and a MUST-FIX.
- **★ hex → `CHART_COLORS[N]` must be the SAME hex.** A wrong index is a silent colour change.
- Web: `cd web && npm run gen:api && git diff --exit-code` (drift) + lint + build +
  `npx vitest run --coverage` (floors 59/54/45; **S32 census: 515 tests / 33 files**) +
  Playwright-docker (`mcr.microsoft.com/playwright:v1.61.1-noble`, `--network host`, mount
  `web/` at `/work`) + WCAG re-check on changed components (design-rationale §2 BINDING) +
  zero contract changes + `brandkit/` byte-untouched + no NEW bare hex/px on ADDED lines.
  **WAVE-PLAN §2.2 is BINDING — read it verbatim.**
- **Don't overlap gate runs with heavy jobs** — a `vitest --coverage` run flaked 2 tests at
  host load 19.8 in S32; two clean re-runs returned 515/515.
- Any Go change: FULL §8 (CI-faithful golang:1.25 docker, 0 FAIL / 0 unexpected SKIP,
  coverage ≥ floor 70.2, gofmt, vet, contract-drift). Not expected this session.
- Harness/bash edits: `bash -n` + shellcheck + memory `shell-harness-false-green-patterns`.
- docs/marketplace/ stays DRAFT-INTERNAL (D-081 external gate).

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-095** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-34; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + PushNotification.
5. Write `sessions/SESSION-34.md` (carry the standing directive header).
