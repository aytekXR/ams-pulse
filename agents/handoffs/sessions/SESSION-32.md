# SESSION-32 — operator-intake gate + §2.19 Wave 1 (planned at S31 close, D-093)

> Written by SESSION-31 close (D-093, 2026-07-14). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/wave-uipro/WAVE-PLAN.md` + `ROADMAP-V2.md` §2.19/§2.18 +
> `RESUME-PROMPT.md` before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12): review the backlog, revise this plan

Before dispatching: re-read ROADMAP-V2 §2 (§2.19 Wave 0 is DONE; §2.18 item 6 is the
operator-gated remainder) + the final-assessment §5 roadmap and REVISE this plan if a
higher-leverage move exists. This file is a starting point, not a contract. Record any
revision in the D-094 open block. Carry this header into SESSION-33.md.

## Mission

Exit = (a) **operator intake applied** — SIX standing items (operator-expected.md ⚡:
GHCR public flip, trial-key mint, final-assessment review, Ant Media contact, MaxNodes
ruling, matbu vhost ruling) **+ THREE design gaps** (G1 mobile input font-size, G2 icon
library, **G3 NEW — light-theme CTA contrast, needs a `tokens.json` change and therefore
an operator ruling**) — if any is answered, act; else re-surface.
**⏰ (b) LICENSE RENEWAL — the key expires 2026-07-27T13:45Z.** From ~07-25 this is the
top intake item: a lapse + the next AMS restart = total ingest death (D-092/D-093 model,
both arms now proven). Surface it in every session until renewed.
(c) **§2.19 Wave 1 [M] [primary]** per WAVE-PLAN §4 W1: LiveOverview + QoE — chart-stroke
hex → `CHART_COLORS[]`, px → `--space-*` tokens, uipro a11y/touch/perf passes. Full web
gates (§2.2 checklist is BINDING).
(d) CI promotions if run date ≥ 2026-07-23 (else skip carry ×21).
(e) standing re-checks + AMS observation at open. PR-first, ≤2 pushes.

## S32 carries (from D-093; pick by leverage after intake)

1. **§2.19 Wave 1 (LiveOverview + QoE) [M]** — primary (Mission (c)).
2. **G3 token fix [XS, operator-gated]** — the moment the operator says "apply the G3
   token fix": `tokens.json color.light.accent` → `#087A59` (5.33:1), cascade through
   `global.css [data-theme=light] --color-accent`, re-run the WCAG pass. Do NOT touch
   `brandkit/` before the ruling (D-071).
3. **Marketplace upload prep [gated]** — operator items.
4. **D-V2-1 unsigned-webhook ingest mode [operator decision first]** — re-surface only.
5. **Browser-accept of the trial banner [operator-assisted]** — realams :18090.
6. **SegmentedControl extraction [XS, optional]** — the S31 tabs scout found FleetPage's
   cards/table toggle is a *different widget* (fill-background, 11px, no underline), NOT a
   `<Tabs>` candidate. If a wave touches Fleet, extract `<SegmentedControl>` separately.
   Never convert it to `<Tabs>` (would change the design intent).

## ⚠️ Check these BEFORE dispatching anything

1. **`docs/operator-expected.md` ⚡ — six standing + G1/G2/G3.** If answered → act; else
   re-surface. **G3 is new and is the only one that blocks a code path** (the light-theme
   contrast fix cannot land without it).
2. **Concurrent-session hazard (D-062, 5th occurrence S31).** Foreign work → inspect →
   preserve → never revert working files. The on-disk `Caddyfile.prod` matbu block stays
   UNCOMMITTED until the operator rules. Check `git status` + `.git/index` mtime at open.
   The session shell may lack the docker group — `sg docker -c "…"`.
   **★ S31's own lesson: a DEAD session's tree was found on the branch at open** (a crashed
   earlier S31 run left TierGate + 3 modified pages uncommitted). It was re-verified from
   scratch, and the audit found a vacuous test + two WCAG failures in it. **Never trust a
   tree you did not gate** (D-082/D-086/D-091 — now 4 occurrences).
3. **AMS state at open.** `bash qa/realams/harness/expiry-sweep.sh s32open` — NO
   PULSE_TOKEN prefix (S29 gotcha). Known truth since S31: license APPLIED, ingest
   RESTORED, and **restart-durable** (survived the 02:02Z reboot). Expect the sweep
   byte-identical to `S21-sweep-preexpiry-20260712T014135Z/stable.txt`. A teststream-down
   row = the S14/S22 ffmpeg-crash class → `docker start ams-teststream` (it does NOT
   auto-restart across a reboot). **NEVER restart/fix AMS without cause.**
   **VPS load:** check `uptime` before Playwright/gates AND before any publish scenario —
   AMS's 75% CPU admission guard refuses RTMP *and* SRT under host load (a distinct
   rejection string; TC-I-05 now has a SKIP arm for it, so it can no longer mislabel as FAIL).
4. **SRT publishes use the PLAIN streamid** `srt://<host>:4200?streamid=<App>/<streamId>`.
   The ACF form (`#!::h=` / `#!::r=`) is REJECTED by AMS EE 3.0.3's SRTAdaptor (D-093).
   Any new SRT scenario must use the plain form.
5. **Prod runs v0.3.0-34-g58a9c84 since S27.** Read-only health check at open; next rollout
   carries D-089..D-093 (Wave 0 is web-only; no runtime behaviour change).
6. **uipro skill presence:** `.claude/skills/ui-ux-pro-max/` is local-only + gitignored
   (license blocker, D-092). If missing, bootstrap per WAVE-PLAN §1.1b. NEVER commit it.

## Gates (ORCH, before any commit)

- Wave 1 (web change): `cd web && npm run gen:api && git diff --exit-code` (gen:api drift)
  + lint + build + `npx vitest run --coverage` (floors 59/54/45; **S31 census: 451 tests /
  32 files**) + Playwright-docker `dashboard-render`, `auth-gate`, `csp`, `prefs`
  (`mcr.microsoft.com/playwright:v1.61.1-noble`, `--network host`, mount `web/` at `/work`)
  + WCAG re-check on changed components (design-rationale §2 BINDING) + zero contract
  changes + `brandkit/` byte-untouched + no NEW bare hex/px in changed files (verify against
  ADDED lines only — pre-existing literals in Wave-2/5 pages are not this wave's gate).
  **WAVE-PLAN §2.2 is BINDING — read it verbatim.**
- Any Go change: FULL §8 (CI-faithful golang:1.25 docker, 0 FAIL / 0 unexpected SKIP,
  coverage ≥ floor 70.2, gofmt, vet, contract-drift). Not expected this session.
- Never trust a tree a dead/stalled agent left (D-082/D-086/D-091/D-093).
- Harness/bash edits: `bash -n` + shellcheck + memory `shell-harness-false-green-patterns`.
- docs/marketplace/ stays DRAFT-INTERNAL (D-081 external gate).

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; merge. ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-094** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-33; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + PushNotification.
5. Write `sessions/SESSION-33.md` (carry the standing directive header).
