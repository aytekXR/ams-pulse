# SESSION-82 — planned at S81 close (D-143) — first arc after §2.33 complete

> Written by SESSION-81 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE.** Prod at **v0.4.0-98-g641b4e2**. The pulse container runs read-only-rootfs +
> cap_drop:[ALL] + no-new-privileges (D-142); report artifacts persist + auto-prune (D-143).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-83

Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and **choose the next-highest-leverage move.** Verify candidate status AND
product-viability against the code before committing. Take the verified CORE. Do NOT stop after one session — at close,
update all docs, regenerate this plan, record progress + operator-needs, continue until the roadmap is complete or a
human/operator is genuinely required. **Ultracode is on.** **Workflow-script gotcha:** no backticks in workflow prompt
prose. `gofmt -l` before every push; web gotchas (`vi.hoisted`, full `npm test` for coverage, binary embeds `web/dist`).

## Where things stand — the autonomous backlog is THINNING (be honest at open)

Four internal passes are done: three subsystem audits (§2.29/§2.30/§2.31/§2.32) + the cross-cutting security-posture
pass (§2.33, S80→S81). 40+ findings shipped. The security/correctness surface is well-swept. Prod is hardened
(read-only rootfs, cap_drop, dep-clean) and stable. **Most remaining ROADMAP §2 items are gated or operator-directed:**
- **§2.7 CI-promotions** — date-gated, unlocks **≥ 2026-07-23** (today is 2026-07-17). **CHECK THE DATE at open:** if
  today ≥ 07-23, this is the strongest bounded autonomous move — flip the soft CI jobs (web-e2e/csp-e2e/e2e/docker-build,
  per its spec) from advisory to required. High-signal, low-risk.
- **Operator-gated (surface, don't attempt):** §2.1 branch protection, §2.6 unsigned-webhook decision, §2.18 GHCR-public
  / licence ceremony, and the two open product calls **[20] audit-read model** and **[5] per-tenant QoE alerting**.
- **Operator-directed UI:** §2.15 phase 2 (light theme / density / motion) and §2.19 full UI/UX refactor.
- **§2.12 Mobile SDKs** — [L per platform], large.

## Lead — pick ONE at open (in priority order)

1. **§2.7 CI-promotions IF the date has unlocked (≥ 07-23).** Bounded, autonomous, high-signal. If today ≥ 07-23, do
   this. Read §2.7's spec; flip the named jobs to required (branch-protection contexts / workflow `if` gates per the
   spec); verify a PR still gates correctly. (If branch protection itself is operator-gated, surface the exact settings
   to the operator instead.)
2. **§2.15 light-theme — IF it is autonomous.** **Verify at open:** does `brandkit/design-system/tokens.json` already
   define LIGHT-theme values (a light palette + the WCAG-AA pairs in `brandkit/documentation/design-rationale.md` §2), and
   does `web/src/lib/ThemeContext.tsx` already carry theme state? If BOTH yes → implementing the toggle + wiring the light
   tokens is autonomous (the design is defined; tokens are the source of truth — do NOT invent color values). A clean web
   arc: theme toggle, `data-theme` on root, CSS-var switch, WCAG-contrast test, persistence. If tokens.json has NO light
   values → designing them is an operator/design call → do NOT invent; go to option 3.
3. **OPERATOR CHECKPOINT — if nothing above is cleanly autonomous.** Four passes done + a thinning autonomous backlog is
   a legitimate point to checkpoint. Produce a crisp `operator-expected.md` summary of the gated items ([20], [5], §2.6,
   §2.1, GHCR, licence ceremony, §2.15/§2.19 UI direction, §2.12 mobile), with a recommendation for each, so the operator
   can unblock the next wave. **Prefer a concrete autonomous move (1 or 2) first** — only checkpoint if both are genuinely
   blocked.

**Optional smaller autonomous wins (if you want a bounded arc over a big one):** a web test-coverage pass (SettingsPage
~50%, OnboardingWizard ~69% are low); an operator-runbook/docs completeness pass; a `/metrics` + observability
completeness check. None are roadmap-headline, but all are autonomous and reduce reviewer round-trips.

## Pipeline (the standard loop)
1. **Verify-at-open:** git state clean (only Caddyfile). **CHECK THE DATE** (§2.7 gate). Record **D-144 IN PROGRESS** in
   `decisions.md`. Branch `s82-d144`.
2. **Execute the chosen lead.** Contracts before code; tokens are the source of truth for any web color.
3. **Validate:** Go changes → mutation-prove + full 25-pkg suite; web changes → `npm test` (full, for coverage) +
   `npm run build` + typecheck + lint; any OpenAPI change → regen `schema.d.ts` + a param-conformance entry.
4. **Adversarial review** for any security-relevant change (a CI-gate or theme change is lower-risk; scale the review).
5. **PR → CI poll** → **squash-merge --delete-branch** → verify origin/main.
6. **Roll prod forward** ONLY if server/web SOURCE changed (a web theme change DOES change the bundle → rebuild; a
   CI-config or docs change does NOT). Stamped rebuild + 5-check smoke (+ GET / for web changes).
7. **Close docs:** D-144, CHANGELOG, ROADMAP (flip/append the tracker), RESUME rotation (→ SESSION-83),
   `operator-expected.md`, SESSION-82 CLOSED, SESSION-83 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`
  (25 pkgs). `gofmt`/`go` NOT on host PATH. Mutation copy `/tmp/pulsemut` (not `/mut`); restore mutated files via `cp`
  from a backup (NEVER `git checkout <path>`, D-096). Node at `/home/aytek/.local/bin/node`.
- **Web:** from `web/`, `npm test` (full suite for the coverage gate), `npm run build` (binary embeds `web/dist` — a
  theme change REQUIRES a rebuild to reach prod), typecheck, lint. `vi.hoisted` for `vi.mock`; `ApiError(status,{code,
  message})`. CI installs with `npm ci --legacy-peer-deps` (pre-existing eslint peer conflict).
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC_ARGS="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. Prod at **v0.4.0-98-g641b4e2**. Rollback tag
  `pulse-prod-pulse:pre-d143`. **Container runs read-only rootfs** — any NEW server-side file write must target
  `/var/lib/pulse` (volume) or `/tmp` (tmpfs). 5-check smoke: version stamp, healthz 200, signed webhook 200 (HMAC from
  PULSE_WEBHOOK_SECRET), limits 512M/0.5cpu, 0 error lines.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). **Do-not-commit:** `deploy/config/Caddyfile.prod` stays modified/unstaged (verify
  `git diff --cached --name-only | grep -q Caddyfile` is empty before every commit). Commit trailer `Co-Authored-By:
  Claude Opus 4.8 (1M context) <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
