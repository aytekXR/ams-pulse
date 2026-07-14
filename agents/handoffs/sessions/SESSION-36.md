# SESSION-36 — RECORD (closed 2026-07-15, D-098)

> Planned as "§2.16 AMS operational early-warning." **Revised at open** under the standing directive
> (a higher-leverage move existed) — exactly as S35 revised its own plan. The operator's question,
> *"are we ready for user intake? how do they sign up and log in?"*, exposed that the post-login flow
> was broken, which outranked building a retention feature for users we could not yet retain.

## Goal (as executed)

Answer **"can a customer sign up, log in, and be onboarded?"** empirically, then fix what a session
can fix. Method: a 161-agent adversarial audit (7 investigation lenses → 3-refuter panel per finding
→ synthesis). **51 raw findings → 29 confirmed / 22 refuted.**

## The answer

**There is no signup** — Pulse is self-hosted, sold by signed licence key. The first credential is a
**bootstrap admin token** minted on first boot (zero rows in `api_tokens`) and printed to
stderr/container logs, once (`bootstrapIfFirstRun`, `server/internal/api/server.go`). Login is that
token or OIDC/SSO. Bootstrap works. The breakage was **entirely after authentication.**

## Delivered (PR #53 → merged; D-098)

Three confirmed, code-fixable blockers — all fixed and gated:

1. **Privilege escalation** — `bearerAuthMiddleware` never read `Scopes`; a `viewer` OIDC token
   could `POST /api/v1/admin/tokens` and self-escalate to admin. Added `requireWriteScope` on the
   `/api/v1` group — a **positive allowlist** (writes need `admin`; empty scopes grandfathered so no
   existing token is locked out; GET/HEAD/OPTIONS always pass). The implementing agent's first cut
   denied only `"viewer"` while the UI mints `"read"` — a fake fix that was green against a wide-open
   path; caught by adversarial review, rewritten, and **mutation-proven** with a `read`-scope
   escalation test that reddens against the blocklist.
2. **Onboarding dead-end** — `OnboardingGuard` sends a user who lands on `/` with **no configured
   AMS** into the (functional) wizard. Fires only on `/`; fails open on error. **A pre-rollout check
   caught that the first version would have trapped the live operator** (prod configures AMS by env
   var, so `ams_sources` is empty but the system is running) — fixed by adding `ams_env_configured`
   to `/healthz` and short-circuiting on it, plus a localStorage dismissal flag. See D-098.
3. **Credential-loss trap** — persistent token copy-panel replacing the 4-second toast; create flow
   now asks admin-vs-read.

Plus `install.md` first-login corrections.

## Refuted with live evidence (did not propagate)

"AMS creds in cleartext" — `PULSE_AMS_AUTH_TOKEN` empty, AMS 403s anonymous calls, collector healthy
(826k+ rows, live). Real residue: AMS:5080 on `0.0.0.0` with no ufw rule → an **AMS** hardening
question for the operator, not a Pulse defect.

## Gates

Go **24/24** pkgs, vet+gofmt clean · web tsc+eslint clean, vitest **638/638** (was 626) ·
Playwright **60/60** · `schema.d.ts` no drift · CI green on #53.

## Blockers a session CANNOT close (re-verified live)

- **⛔ GHCR image private** — anonymous manifest → **401**. Kills the one-command quickstart.
- **⏰ AMS licence expires 2026-07-27T13:45Z** — from ~07-25 outranks GHCR.

## Non-blocker gaps surfaced (see `docs/operator-expected.md` D-098 table)

Team/invite UI, audit trail, OIDC licence-gating, tenant isolation, self-serve trial/billing,
out-of-band licence-expiry alerting. None block a first sale to a technical single-operator customer.

## Process lesson (recorded in D-098)

**Playwright tests the built `dist/`, not source.** Narrowed the guard, re-ran Playwright against a
**stale build**, saw the same 13 failures, nearly re-debugged a fixed bug. **Rebuild before every
Playwright run.** 13 → 1 → 0 once rebuilt.
