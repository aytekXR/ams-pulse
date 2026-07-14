# SESSION-34 — operator-intake gate + §2.19 Wave 3 (planned at S33 close, D-095)

> Written by SESSION-33 close (D-095, 2026-07-14). Paste-ready prompt for the next
> session. Repo `/home/aytek/repo/ams-pulse` on VPS `161.97.172.146`. Read
> `agents/handoffs/wave-uipro/WAVE-PLAN.md` + `ROADMAP-V2.md` §2.19/§2.18 +
> `RESUME-PROMPT.md` before dispatching.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12): review the backlog, revise this plan

Before dispatching: re-read ROADMAP-V2 §2 (§2.19 Waves 0–2 are DONE; §2.18 item 6 is the
operator-gated remainder) + the final-assessment §5 roadmap and REVISE this plan if a
higher-leverage move exists. This file is a starting point, not a contract. Record any
revision in the D-096 open block. Carry this header into SESSION-35.md.

## Mission

Exit = (a) **operator intake applied** — SIX standing items (GHCR public flip, trial-key
mint, final-assessment review, Ant Media contact, MaxNodes ruling, matbu vhost ruling)
**+ SIX design gaps** (G1 mobile input font, G2 icon library, **G3** light CTA contrast,
**G4** touch-target 44 vs 24, **G5** the brandkit WCAG table's wrong ratio, **G6** light-theme
info Badge) **+ the StatCard look question**. G3/G5/G6 are `tokens.json`/brandkit edits —
**ONLY the operator may authorise them (D-071).** If any is answered → act; else re-surface.
**⏰ (b) LICENSE RENEWAL — the key expires 2026-07-27T13:45Z.** From ~07-25 this is the
top intake item: a lapse **+** the next AMS restart = total ingest death (both arms proven,
D-092/D-093). Surface it every session until renewed.
(c) **§2.19 Wave 3 [M] [primary]** per WAVE-PLAN §4 W3: Ingest + Anomalies.
(d) CI promotions if run date ≥ 2026-07-23 (else skip carry ×23).
(e) standing re-checks + AMS observation at open. PR-first, ≤2 pushes.

## S34 carries (from D-095; pick by leverage after intake)

1. **§2.19 Wave 3 (Ingest + Anomalies) [M]** — primary. Per WAVE-PLAN §4 W3:
   - Ingest: 4 chart stroke hex → `CHART_COLORS[]`. **★ The plan flags an open question it
     could not resolve: one series uses `#FF5C68` (critical/error), and the plan is unsure
     whether that encodes a drop/error event (→ `var(--color-error)`) or is a plain dataviz
     series (→ `CHART_COLORS[3]`, which is `#F06BB2` — a DIFFERENT hex).** Read the code and
     decide; **substituting the wrong one is a silent colour change.** Document the ruling
     in a code comment naming which series is which channel.
   - Ingest: drop-event panel inline `rgba()` → `var(--color-error-bg)`; px → tokens.
   - Anomalies: px → tokens; `<TierGate>` consumption; sigma sensitivity selector a11y.
2. **G3 / G5 / G6 token fixes [XS each, operator-gated]** — the moment the operator says so.
   G5 is the highest-value: it is a *wrong number in a binding table*, and every future wave
   reads that table.
3. **Marketplace upload prep [gated]** — operator items.
4. **D-V2-1 unsigned-webhook ingest mode [operator decision first]** — re-surface only.

## ⚠️ Check these BEFORE dispatching anything

1. **`docs/operator-expected.md` ⚡ — six standing + G1…G6 + the StatCard look question.**
   **NEVER let an agent claim the operator approved/sanctioned/waived anything** — an S31
   agent falsely wrote "Operator waiver granted" and it had to be corrected in three places
   (D-093). Sessions do not self-approve operator decisions.
2. **★★ VERIFY THE LAST SESSION ACTUALLY MERGED — S33 found S32's PR still OPEN, and its
   branch missing a file the gates had run against (D-095).** At open: `git log origin/main`,
   `gh pr list --state open`. **A session that says "DONE" is not evidence that it landed.**
   Then `git status` — the ONLY expected dirty file is `deploy/config/Caddyfile.prod` (the
   matbu block, operator-gated). **Anything else dirty is a dead-session tree: re-audit it
   from scratch, never trust it** (D-082/D-086/D-091/D-093/D-095 — 5 occurrences).
3. **★ A className is a CONTRACT with the stylesheet.** `web/src/styles/__tests__/
   focus-rings.test.ts` pins both halves (rule exists ⟺ component uses it). **Any wave adding
   a bare styling className must add it to that map**, or the guard is silently incomplete.
4. **★ The px→token trap, and its cousin.** Scale = 4/8/12/16/24/32/48/64/96. Substitute ONLY
   on an EXACT match; a non-matching literal is LEFT ALONE (snapping 13→12px is a silent 1px
   regression). **And: `width`/`height`/`minWidth` are DIMENSIONS, not spacing — never
   `--space-*`** (S33 rejected `width: 32` → `--space-6`). Radii have their own tokens.
5. **★ hex → `CHART_COLORS[N]` must be the SAME hex.** A wrong index is a silent colour change.
6. **★ Verify a mutation LANDED before trusting a GREEN.** S33's `perl -0pi -e 's/…/…/'`
   (no `/g`) hit a doc comment instead of the JSX and reported a false GREEN. Grep the file
   after mutating (D-091 class, 2nd occurrence).
7. **AMS at open.** `bash qa/realams/harness/expiry-sweep.sh s34open` — NO PULSE_TOKEN prefix
   (S29 gotcha). Expect byte-identical. **`ams-teststream` does NOT auto-restart across a VPS
   reboot** → `docker start ams-teststream`. **SRT publishes MUST use the plain streamid**
   `srt://<host>:4200?streamid=<App>/<streamId>` (D-093). **Check `uptime` before gates AND
   before any publish** (AMS refuses publishes above 75% CPU; a vitest run flaked at load 19.8).
8. **Prod runs v0.3.0-34-g58a9c84 since S27.** Read-only health check at open; next rollout
   carries D-089..D-095 (the UI waves are web-only — no runtime behaviour change).

## Gates (ORCH, before any commit)

- **★ RUN THE SPECS OF THE COMPONENTS THIS WAVE TOUCHES — not just the default gate list.**
  Wave 3 touches Ingest + Anomalies; **neither has an e2e spec** (S33 had to write Analytics'
  and Fleet's from scratch). Write them.
- Web: `cd web && npm run gen:api && git diff --exit-code` (drift) + lint + build +
  `npx vitest run --coverage` (floors 59/54/45; **S33 census: 548 tests / 35 files**) +
  Playwright-docker (`mcr.microsoft.com/playwright:v1.61.1-noble`, `--network host`, mount
  `web/` at `/work`) + WCAG re-check on changed components (design-rationale §2 is BINDING —
  **but see G5: one of its rows is WRONG; recompute, do not trust it**) + zero contract
  changes + `brandkit/` byte-untouched + no NEW bare hex/px on ADDED lines.
  **Note:** the §2.2 hex grep scans whole files — **do not put hex literals in comments** in
  `src/features/` (S33 tripped its own gate that way).
- **Don't overlap gate runs with heavy jobs** on this box.
- Any Go change: FULL §8 (CI-faithful golang:1.25 docker, 0 FAIL / 0 unexpected SKIP,
  coverage ≥ floor 70.2, gofmt, vet, contract-drift). Not expected this session.
- docs/marketplace/ stays DRAFT-INTERNAL (D-081 external gate).

## Closing protocol (ROADMAP §6, unchanged)

1. Commits per scope on a BRANCH; PR; contexts green; **merge — and VERIFY the merge landed.**
   ≤2 pushes total.
1b. `codegraph sync` + `codegraph status`.
2. decisions.md **D-096** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-35; ROADMAP-V2 ledgers.
4. REFRESH `docs/operator-expected.md` + PushNotification.
5. Write `sessions/SESSION-35.md` (carry the standing directive header).
