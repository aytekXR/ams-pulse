# SESSION-80 — planned at S79 close (D-141) — first POST-S73-audit arc

> Written by SESSION-79 close (2026-07-17). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE.** The S73 audit (§2.32) is COMPLETE (7 shipped + [5] deferred).
> THREE subsystem audits are now done (S44/§2.29, S48/§2.30, S62/§2.31, S73/§2.32) — the subsystem surface is well-swept.

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-81

Re-read ROADMAP-V2 §2 / `docs/assessment/` §5 and **choose the next-highest-leverage move.** Verify candidate status AND
product-viability against the code before committing. Take the verified CORE. Do NOT stop after one session — at close,
update all docs, regenerate this plan, record progress + operator-needs, continue until the roadmap is complete or a
human/operator is genuinely required. **Ultracode is on.** **Workflow-script gotcha:** no backticks in workflow prompt
prose. `gofmt -l` before every push; web gotchas (`vi.hoisted`, full `npm test` for coverage, binary embeds `web/dist`).

## Where things stand (post-3-audits)

The security+correctness SUBSYSTEM surface is now well-covered (40+ findings shipped across 3 audits). Prod at
**v0.4.0-93-g8858b5f**. Remaining ROADMAP §2 items are mostly gated or operator-directed:
- **§2.7 CI job promotions** — date-gated, unlocks **≥ 2026-07-23** (CHECK THE DATE at open; if today ≥ 07-23, this is a
  strong autonomous candidate — flip the CI jobs from soft to required per its spec).
- **Operator-gated:** §2.1 branch protection, §2.6 unsigned-webhook DECISION, §2.18 item 6 / GHCR-public / licence
  ceremony, plus the two open product calls: **[20] audit-read model** and **[5] per-tenant QoE alerting** (D-141).
- **Operator-directed UI:** §2.15 phase 2 (light theme / density / motion — brandkit `tokens.json` is the source of
  truth, so this is MORE autonomous than it looks) and §2.19 full UI/UX refactor via uipro.
- **§2.12 Mobile SDKs** — [L per platform], large.

## Lead — a CROSS-CUTTING security-posture pass (dependency / supply-chain / deploy hardening)

The three prior audits were SUBSYSTEM (by-package) sweeps. A cross-cutting pass covers what they structurally can't and
is high-value + autonomous:
- **Go dependency vulnerabilities:** run `govulncheck` against the module (`cd server && govulncheck ./...` in a golang
  container that has it, or `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`). Triage each reported CVE against
  actual reachability; bump the dependency (go.mod) or document not-reachable. Same for `qa/mock-ams`, `qa/licensegen`.
- **Web dependency vulnerabilities:** `cd web && npm audit --production` (+ dev). Triage → bump or document. Watch the
  15 KB SDK size gate if `sdk/beacon-js` deps change.
- **Deploy/Dockerfile hardening review:** the multi-stage Dockerfile (non-root user? pinned base digests? minimal final
  image?), the compose overlays (already hardened per D-082+), and `deploy/config/*` — a focused adversarial review for
  container-escape / privilege / secret-handling gaps the subsystem audits didn't cover. **Do NOT touch the
  do-not-commit `deploy/config/Caddyfile.prod`.**
- Produce findings the same way (mutation-prove code fixes; adversarial-review security fixes) — but a dependency bump is
  validated by the full suite + CI, not mutation testing.

**Alternatives (weigh at open):**
- **§2.7 CI promotions** if the date has unlocked (07-23) — bounded, autonomous, high-signal.
- **§2.15 light-theme** from brandkit tokens — user-facing, and the tokens define it (autonomous-ish); a clean web arc.
- **If nothing autonomous is high-leverage:** produce a crisp OPERATOR CHECKPOINT — summarize the gated items ([20],
  [5], §2.6, §2.1, GHCR, licence ceremony, §2.15/§2.19 direction) and recommend, so the operator can unblock the next
  wave. (Three audits done + a thinning autonomous backlog is a legitimate point to checkpoint — but prefer a concrete
  autonomous move first.)

## Pipeline (the standard loop)
1. **Verify-at-open:** git state clean (only Caddyfile). Record **D-142 IN PROGRESS** in `decisions.md`. Branch
   `s80-d142`. **CHECK THE DATE** (§2.7 gate ≥ 2026-07-23).
2. **Run the posture tools** (govulncheck, npm audit) + triage; OR execute the chosen alternative.
3. **Fix → validate** (dependency bumps: full 25-pkg Go suite + web build/test; code fixes: mutation-prove).
4. **Full Go suite + web** as touched; `gofmt -l`.
5. **Adversarial review** for any security-relevant code change.
6. **PR → CI poll** → **squash-merge --delete-branch** → verify origin/main.
7. **Roll prod forward** if server/web source changed (standard sequence + 5-check smoke).
8. **Close docs:** D-142, CHANGELOG, ROADMAP (new §2.33 tracker if it's a new audit/arc), RESUME rotation,
   `operator-expected.md`, SESSION-80 CLOSED, SESSION-81 written. Re-arm the `/loop`.

## Environment gotchas (carried)
- **Go only in docker:** `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v
  pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...`
  (25 pkgs). `gofmt`/`go` NOT on host PATH. `govulncheck` isn't installed — `go run golang.org/x/vuln/cmd/govulncheck@latest`
  in the container (needs network; if the sandbox blocks it, note that + fall back to `go list -m all` + manual CVE check).
  Node at `/home/aytek/.local/bin/node` (npx). Mutation copy `/tmp/pulsemut` (not `/mut`). CodeQL flags production
  `InsecureSkipVerify` only.
- **Prod deploy LOCAL** (this host IS prod): 5-overlay compose `DC_ARGS="-p pulse-prod -f deploy/docker-compose.yml -f
  deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f
  deploy/docker-compose.backup.yml --env-file deploy/.env"`. Prod at **v0.4.0-93-g8858b5f**. 5-check smoke: startup
  version stamp, healthz 200, signed webhook 200 (HMAC from PULSE_WEBHOOK_SECRET), limits 512M/0.5cpu, 0 error lines.
- **Admin token** (side-effect-free GET only, never commit): gitignored `oguz-testing.md`. Prod API base
  `https://beyondkaira.com` with `--resolve beyondkaira.com:443:161.97.172.146`.
- **Never** restart/fix AMS; never `docker compose down -v` on prod; never `git reset/checkout/stash/restore <path>`
  (D-096; `git restore --staged` OK). Commit trailer `Co-Authored-By: Claude Opus 4.8 (1M context)
  <noreply@anthropic.com>`; PR-body trailer `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
