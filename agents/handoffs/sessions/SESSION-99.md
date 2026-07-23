# SESSION-99 — planned at S98 close (D-162) — ★ THE MARKETPLACE-WAIT (the non-gated autonomous backlog is EMPTY)

> Written by SESSION-98 close (2026-07-23). Repo `/home/aytek/repo/ams-pulse` on VPS (**this host IS prod**; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-131-g6b5bd38** (S97 docs-only, S98 CI-config-only — no roll).
> S98 closed the LAST date-gated item (§2.7, incl. the branch-protection contexts — executed autonomously). Everything
> that remains needs the operator, a toolchain install, or a product ruling. **This session (and the ones after it,
> until an input arrives) is a low-frequency wait — that is the CORRECT behavior, not idleness.**

## ⚡ STANDING DIRECTIVE — carried
Re-read ROADMAP-V2 §2 / Future-roadmap §A–E and take the next-highest-leverage non-gated move when one exists; wait at
low frequency otherwise. Ultracode on. No backticks in Workflow prompt prose. `gofmt`/Go tests only via docker. This
host IS prod — never restart AMS, never `docker compose down -v`, never `git checkout <path>` (D-096). Workflow
subagents cannot read the session scratchpad — share context via repo paths (D-161).

## THE ONLY THING TO DO AT OPEN: the two-minute gate
1. `command -v gradle && command -v java` — if PRESENT → **sdk/beacon-kotlin auto-starts** (standing GO D-154,
   ROADMAP §2.12; turnkey plan there — Gradle Kotlin lib mirroring the iOS SDK, zero-dep, JUnit, `sdk-kotlin` CI job).
2. Check `docs/operator-expected.md` — has the operator: approved the docs pack (D-081)? answered support-SLA /
   pricing / MaxNodes / trial? delivered a load-lane capacity number? recorded the demo (or asked for the Playwright
   rough cut)? flipped GHCR public? reported the Ankush reply / meeting outcome (→ close A1–A10 in
   submission-process.md, fold into compatibility.md + listing)? ruled on `[FO-1]` (→ Lead B) or [20]?
3. Nothing new → **re-arm at max interval and stop in ONE line.** Do NOT manufacture an arc. Do NOT re-sweep
   (S89/S91/S92 ×3 + S95 delta + S96/S97/S98 arcs — the well is dry; a sweep IS an arc).

## Lead B — operator-input-driven (only if provided at open)
- **Capacity number** → `docs/compatibility.md` load-validation row + listing copy.
- **G-27 / meeting answers** → close the tagged questions in compatibility.md + submission-process.md (A1–A10).
- **`[FO-1]` ruling** → build the chosen firing-orphan resolution — adversarial review MANDATORY (live critical-alert
  path; must NOT touch `stream_offline` semantics where absence IS the alert).
- **[20] ruling (b)** → gate the admin-read surface behind `admin` scope (uniform; viewer loses those pages).
- **D-081 approval** → strip DRAFT-INTERNAL headers across docs/marketplace/* + licensing-public.md in one commit.
- **"attempt the demo rough cut"** → Playwright-driven screen capture per `docs/marketplace/demo-video-script.md`.

## Environment gotchas (carried, unchanged)
See `sessions/SESSION-97.md` §Environment gotchas (S97/S98 changed no runtime). Prod-deploy 5-overlay `DC` set,
5-check smoke, do-not-commit `Caddyfile.prod`, mutation-copy restore via `cp`.

## Notable state (for whoever reads this cold)
- Branch protection: 13 required contexts on `main`, strict=true (D-162 — verified). The loop's token holds
  repo-admin; the operator was told and may narrow scopes if preferred.
- web-e2e + csp-e2e are HARD gates now. If either flakes on a future PR: fix at the spec (the D-162 catch-all
  pattern), never re-soften `continue-on-error`.
- The marketplace submission is fully prepared (D-161 pack, `docs/marketplace/submission-package.md` = index) and
  gated ONLY on the operator's 6-step sequence (operator-expected.md ★S97 block).
