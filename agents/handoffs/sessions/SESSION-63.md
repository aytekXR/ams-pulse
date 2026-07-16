> ## ✅ SESSION-63 CLOSED (2026-07-16, D-125) — PR #120 merged `5172150`, prod `v0.4.0-64-g5172150` (smoke green)
> **Shipped:** alert-channels security cluster — [1] STARTTLS fail-closed (email refuses plaintext fallback on TLS
> downgrade), [2] Telegram bot-token redaction in error paths, [10] email Subject-header CRLF sanitization, [11]
> Telegram dashboard_url href attribute escaping. New `channels_security_test.go` (4 tests, fake-SMTP helper).
> **Honesty adjustments vs the ledger:** [1] scope stated precisely (residual real exposure = localhost/loopback relay
> or a server falsely advertising TLS; Go's `PlainAuth` already refuses remote non-TLS); **[11] downgraded HIGH→LOW**
> after tracing `dashboard_url` is only ever the operator's own `baseURL+"/alerts"` (test-fire path), never
> attacker-controlled — escaped anyway as defense-in-depth. STARTTLS kept fail-closed against the reviewer's
> availability concern (opportunistic opt-out would re-introduce the vuln; operators wanting plaintext already set
> `STARTTLS=false`). **Operator note:** email STARTTLS is now mandatory-when-enabled — see `operator-expected.md`
> D-125 block. **Ledger:** [1]/[2]/[10]/[11] ✅ DONE; **21 S62 findings remain** (4 HIGH, 13 MEDIUM, 4 LOW).
> **Next (SESSION-64):** reports_wave2 re-fetch nil-deref cluster — see `sessions/SESSION-64.md`.

# SESSION-63 — planned at S62 close (D-124)

> Written by SESSION-62 close (2026-07-16). Repo `/home/aytek/repo/ams-pulse` on VPS
> `161.97.172.146` (**this host IS prod** — the `pulse-prod` compose stack runs locally; no SSH).
> **Read `RESUME-PROMPT.md` ▶ START HERE + `agents/handoffs/S62-AUDIT-FINDINGS.md`** (the 25-finding ledger).

## ⚡ STANDING DIRECTIVE (operator, 2026-07-12) — carry into SESSION-64

Before dispatching: re-read ROADMAP-V2 §2 / assessment §5 and **choose the next-highest-leverage move.** Verify
candidate status AND product-viability against the code before committing. **Each S62 finding is an AGENT finding —
re-verify against the code; take the verified CORE, not the audit's literal suggested scope** (the S48 arc proved
this: NARROWER, BROADER, or DEFER). One coherent scope per PR.

## ★ Context — the S62 audit backlog (25 findings, 0 shipped)

SESSION-62 audited the un-swept subsystems → **25 CONFIRMED (6 HIGH, 15 MEDIUM, 4 LOW), 1 refuted** in
`S62-AUDIT-FINDINGS.md`. Work HIGH-first, in coherent clusters, one scope per PR, exactly as S49→S61 worked the S48
backlog. **The ledger numbering is by severity (1-25); [1]-[6] are the HIGH findings.**

## ★ Suggested SESSION-63 first scope: alert-channels security ([1] STARTTLS + [2] token leak)

Two HIGH secret/transport-security findings in `server/internal/alert/channels/`, plus two same-file MEDIUM injection
findings that bundle cleanly:
- **[1] HIGH** `channels.go:147` — `smtpClient.StartTLS()` error is discarded (`_ = err`, "Non-fatal"), so on a TLS
  downgrade the code proceeds on a plaintext socket and calls `smtp.PlainAuth`. **Fix:** `if err := StartTLS(...);
  err != nil { return fmt.Errorf(...) }`. **⚠ RE-VERIFY:** Go's `smtp.PlainAuth.Start()` already refuses a non-TLS
  **non-localhost** server ("unencrypted connection"), which partially mitigates the *remote-host* cred-theft path —
  so the residual real exposure is a localhost/loopback relay or a server that falsely advertises TLS. The fix (don't
  silently discard the error) is still correct; **state the scenario honestly** (verified CORE, not the audit's
  broad framing). Mutation: fake SMTP server returns `454` to STARTTLS → assert `Send()` returns an error.
- **[2] HIGH** `telegram.go:86` — the bot token is interpolated into a `slog.Warn` message on every failed send
  (`"telegram send failed url=https://api.telegram.org/bot<TOKEN>/sendMessage ..."`), leaking the secret to logs.
  **Fix:** redact the token from the logged URL (log the endpoint without the `bot<token>` segment, or a
  `sendMessage` label only). Mutation: assert the warn log field does NOT contain the token.
- **Bundle (same 2 files, MEDIUM):** [7] SMTP `Subject:` CRLF injection via `stream_id` scope → sanitize the header;
  [8] Telegram `dashboard_url` not HTML-escaped in `<a href>` under HTML parse-mode → `html.EscapeString`.

**Why first:** highest severity (secret + plaintext-credential exposure), self-contained (2 files), no contract/DB
change. A genuine security cluster → run the **multi-lens adversarial review** before merge.

## ★ Then (subsequent sessions), the other HIGH + coherent clusters

- **reports_wave2 re-fetch** ([HIGH] handleUpdateTenant nil-deref + [HIGH] handleUpdateReportSchedule nil-deref +
  [MEDIUM] transient-DB-error-returned-as-404) — one file, one post-update re-fetch pattern; mirrors the S40/D-102
  fix. A crash-on-input bug (panic) → HIGH.
- **prober untrusted-input** ([HIGH] MPD unbounded read → `io.LimitReader`; [HIGH] printf-format injection → static
  format / `%s`; [MEDIUM] RTMP CSID map growth cap) — DoS/security hardening of untrusted probed-server responses.
- **alert-evaluator** ([1]/[2]/[3]/[4] in the finder's terms: D-088 presence guards, stream_offline compare bypass,
  license_expiry stuck-firing, cert DaysUntilExpiry -1) — correctness of alert firing.
- **anomaly** (ComputeFlags/tick hysteresis interaction, cooldown off-by-one, scopeJSON escaping).
- **license** (tier validation, New() error discard, wrong-err-var) — mostly LOW/MEDIUM.
- **prober-core** (HLS zero-EXTINF, protocol-relative URI, WebRTC-ICE timer leak).
- **api** ([MEDIUM] SSRF via unvalidated probe URL scheme/host → validate; **[24] audit-log admin gate — RE-VERIFY
  vs D-105 FIRST**, likely DEFER as the deliberate "reads-open" model or escalate as an operator ruling, NOT a silent
  tighten).

## ⛔ At open — verify, do not assume (D-095)

- `git log --oneline origin/main -4` — S62 (D-124, PR #NNN — docs/ledger only) should be on `origin/main`.
- Prod should print **`v0.4.0-61-g28812db`** (UNCHANGED — S62 shipped no code). `/healthz` all-ok. Signed-webhook
  smoke still 200 (replay check default-off).
- **§2.7 CI promotions unlock ≥ 2026-07-23 — CHECK THE DATE.** If ≥ 07-23, bundle the `web-e2e`/`csp-e2e` promotion.
- Operator queue: GHCR 401; AMS trial-expiry doc discrepancy (07-12 vs 07-27) — operator-only.

## 🔧 Environment gotchas (unchanged — read before any gate)

- **Go only in docker**: `docker run --rm -v /home/aytek/repo/ams-pulse:/repo -v pulse-gocache:/go/pkg/mod -v pulse-gocache-build:/root/.cache/go-build -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...` (24/24). **`-buildvcs=false` required.** After a `contracts/` edit re-run the api package with `-count=1`.
- **★ RUN `gofmt -l .` BEFORE EVERY PUSH** — CI's `server` job has a gofmt gate the local `go build && go vet` misses (S54). (Memory: `ci-gofmt-gate`.)
- **Mutation-prove on a COPY**: `cp -a /repo/server /mut && cd /mut`; mutate; test there. `perl -0pi -e 's/\Q<literal>\E/<replacement>/'` handles metachars (S61). Never `git reset/checkout/stash/restore <path>` (D-096); `git restore --staged` is fine.
- **CodeQL is a required check.** **Contract change? `cd web && npm run gen:api`** (types-drift guard; node 22, `npm ci --legacy-peer-deps`, S55). **New CH migration? lineage at 0010** (next = 0011; do NOT edit 0001 — S60). **Playwright** in `mcr.microsoft.com/playwright:v1.61.1-jammy` after `npm run build`.
- **`deploy/config/Caddyfile.prod` stays UNCOMMITTED** (clean status = FAILURE signal). **Never restart/fix AMS.**
- **Prod deploy LOCAL** — `deploy/runbooks/upgrade-rollback.md`: 5-overlay `DC_ARGS`; **rollback = a DOCKER image tag** `docker tag pulse-prod-pulse:latest pulse-prod-pulse:pre-dNNN` → backup (`exec -T backup … once`, rc 0) → STAMPED `build --build-arg VERSION=$(git describe --tags --always) …` (backgrounded, >2 min) → assert stamp ≠ dev → `up -d` (no `--build`) → smoke (healthz, version, **signed webhook 200**, limits 512M/0.5cpu, logs clean). Roll forward ONLY if server/web *source* changed. Admin token in gitignored `oguz-testing.md` (side-effect-free requests only).

## Binding lessons (carry into every wave)

1. Verify product-viability AND mechanism before building; take the verified CORE — NARROWER, BROADER, DEFER (dead/
   vestigial or duplicate-of-a-ruling — S59/S60, and **[24] vs D-105**), or SHIP-OPT-IN after a contract check (S61).
   Trace an existing test before trusting it (S49). A named goal / audit claim can be stale or overstated (S37/S48;
   [1]/STARTTLS scenario is narrower than stated) — re-verify.
2. Mutation-prove every guard; positive control so the harness can't be vacuous. Cover BOUNDARY conditions (S61).
3. Independent review before merge: a genuine SEMANTIC/security/auth/contract change warrants the multi-lens
   adversarial workflow (S55/S61 — the alert-channels security cluster qualifies); a purely mechanical mutation-proven
   fix can take a careful self-review.
4. Positive allowlists over blocklists (D-098). Respect documented contract/design even when an audit disagrees
   (S59/S60; **[24]**). Migrations forward-only (lineage 0010; never edit 0001).
5. No silent scope caps; persist verified findings to the ledger; state latency/impact honestly. Default-off /
   backward-compatible is the way to ship a security/contract change without breaking live traffic (S61).
6. **Run `gofmt -l` before every push** (S54).

## Closing protocol (ROADMAP §6)

1. Commits per scope on a BRANCH; PR; **merge — VERIFY it landed on `origin/main`.**
2. `decisions.md` **D-125** evidence — append EARLY.
3. RESUME-PROMPT ▶ START HERE → SESSION-64; ROADMAP-V2 §2.31 ledger; mark shipped findings ✅ in `S62-AUDIT-FINDINGS.md`.
4. REFRESH `docs/operator-expected.md` (esp. if [24] becomes an operator ruling).
5. Write `sessions/SESSION-64.md`.
6. **Roll prod forward** if server/web *source* changed.
