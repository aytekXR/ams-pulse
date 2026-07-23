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

## Environment gotchas (UPDATED 2026-07-23 after the operator's Caddy→nginx cutover, PR #199 / commit 810b36f)
**The SESSION-97 gotchas are PARTIALLY STALE.** New truth: prod edge = **host nginx** (vhosts in `deploy/nginx/`,
TLS via certbot, cert at `/etc/letsencrypt/live/beyondkaira.com/`); Caddy containers/config are GONE from repo AND
VPS (`Caddyfile.prod` no longer exists — that do-not-commit rule is moot); the canonical prod compose command is the
**3-file set** in `deploy/runbooks/upgrade-rollback.md` (`-p pulse-prod -f deploy/docker-compose.prod.yml
-f deploy/docker-compose.real-ams.yml -f deploy/docker-compose.backup.yml --env-file deploy/.env`) — NOT the old
5-overlay set. Never touch the host nginx (:80/:443) or certbot state without operator direction. CI keeps its own
containerised Caddy (`Caddyfile.ci`, csp-e2e) — deliberately, do not "clean it up". Still true: 5-check smoke,
mutation-copy restore via `cp`, gofmt/Go tests only via docker, never `docker compose down -v` on prod.

## Notable state (for whoever reads this cold)
- Branch protection: 13 required contexts on `main`, strict=true (D-162 — verified). The loop's token holds
  repo-admin; the operator was told and may narrow scopes if preferred.
- web-e2e + csp-e2e are HARD gates now. If either flakes on a future PR: fix at the spec (the D-162 catch-all
  pattern), never re-soften `continue-on-error`.
- The marketplace submission is fully prepared (D-161 pack, `docs/marketplace/submission-package.md` = index) and
  gated ONLY on the operator's 6-step sequence (operator-expected.md ★S97 block).

---

# EXECUTION LOG (S99, 2026-07-23) — wait-session gate found external change → doc-reconciliation arc (D-163)

Ticks 1–4 (hourly): gate quiet (toolchain absent; main unchanged at `4d8c746`; no operator input) → re-armed, no arc.
Tick 5: gate found **PR #199 merged by the operator** (`810b36f` — Caddy→host-nginx cutover complete, prod compose
consolidated). Session goal determined at open: reconcile the living docs #199 deliberately left untouched with the
new edge/compose reality (the un-swept delta), nothing more.

- Ground truth read FIRST (nginx vhosts incl. the real `/webhook/` + `/beacon/` routing semantics, prod.yml service
  list, upgrade-rollback.md canonical 3-file command) — every edit grounded in merged config, not memory.
- Fixed: admin-guide §7 (nginx = default edge; deleted-Caddyfile table gone), overview.md (diagram + 3-file table),
  AMS-INTEGRATION (§3.2 + appendix DC commands with dead `prod-tls`; §4.4 nginx webhook route; §4.6/§5.2/§5.4;
  webhook-404 fix), troubleshooting.md WS section, dependabot-policy staging smoke, TC-I-05 comment, and THIS file's
  Environment gotchas (Caddyfile.prod rule moot; 5-overlay DC dead; hands off host nginx/certbot).
- Verified: no living script/config references a deleted artifact; marketplace pack + faq/user-guide/api-guide have
  ZERO caddy/prod-tls/PULSE_DOMAIN refs; historical docs left per #199's own convention.
- 3-lens adversarial verify (grounding / completeness / cross-refs) → PR #200 → squash-merge.

**Operator-action check: NO operator action is required.** The cutover was the operator's own work; this arc only
made the docs tell the truth about it. The wait-state queue (submission sequence, toolchain, [FO-1], [20]) is
unchanged — see operator-expected.md ★S99. → back to the marketplace-wait (SESSION-100 = same protocol as this file's
plan section; next session continues under it).
