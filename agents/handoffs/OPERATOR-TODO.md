# Operator TODO — the items only YOU can do (updated by SESSION-08, D-065, 2026-07-09)

> **Audience: the human operator.** Work these in parallel while an agent session runs — none
> of them conflict with agent work. The ledger of record is `ROADMAP.md` §5; this file is the
> actionable view and is refreshed at every session close. When you finish an item, just tell
> the agent (or do nothing — every session start re-verifies each item automatically).
> **Never commit secret VALUES anywhere; `deploy/.env` and `oguz-testing.md` are gitignored.**

## ★ THE HEADLINE — GA is DECLARED (D-065, 2026-07-09)

### O13 — Pick the GA release tag: `v1.0.0` or `v0.2.0` *(NEW — your call, nothing ships without it)*
- Every GA gate is met; prod already runs current main (`v0.1.0-50-g5d77a05`), smoke-green.
- Release material is ready: `CHANGELOG.md` GA section + `agents/handoffs/RELEASE-NOTES-DRAFT.md`.
- **Your action:** tell the next session "tag v1.0.0" (or v0.2.0). It will push the tag,
  watch the release pipeline (CI-gated, Trivy, SBOM, cosign), `cosign verify`, and roll prod
  onto the tag. No tag is pushed without your explicit word.
- Semantics hint: `v1.0.0` says "stable, breaking changes bump major"; `v0.2.0` keeps a
  pre-1.0 "APIs may still move" posture. Both run the identical pipeline.

## 🔴 High priority — gate remaining GA polish

### O5 — Choose the project LICENSE  *(the LAST G7 gap; legal decision)*
- The SDK is already MIT (`sdk/beacon-js/LICENSE`); the server has commercial tiers, so the
  usual candidates: Apache-2.0 (permissive), AGPL-3.0 (copyleft, protects the SaaS angle),
  BUSL-1.1 (source-available with commercial restriction).
- **Your action:** pick one and tell the agent — it drafts the LICENSE file same-session.
  Ideally decide this BEFORE the O13 tag so the release ships with a license.

### O12 — Enable secret-scanning + push-protection  *(repo is PUBLIC, both still OFF — re-verified at S8 close)*
- Click path: GitHub → `aytekXR/ams-pulse` → Settings → Advanced Security →
  enable **Secret scanning**, then **Push protection**.
- Or paste this in a session prompt (runs as you): `! gh api -X PATCH repos/aytekXR/ams-pulse -f 'security_and_analysis[secret_scanning][status]=enabled' -f 'security_and_analysis[secret_scanning_push_protection][status]=enabled'`

### U3 — Activate a Pro+ Pulse license in prod
- Add to `deploy/.env` on the VPS: `PULSE_LICENSE_KEY=<key>` (line 32 is a commented
  placeholder; never commit it), then tell a session — it restarts pulse and live-verifies
  the beacon → QoE chain (everything else is already in place: the endpoint answered
  403 LICENSE_REQUIRED, correctly fail-closed, during the S8 smoke).
- Until then: no viewer QoE data flows in prod; rebuffer/error-rate rules evaluate against
  0.0 by design (documented 3-case semantics in `docs/runbooks/alerting.md`).

## 🟡 Medium priority

### O7 — Make the GHCR package public  *(last G1 bit; re-verified still private)*
- Click path: github.com/aytekXR → Packages → `ams-pulse` → Package settings →
  Danger zone → **Change visibility → Public**.
- Matters more once O13 ships a public release others should be able to pull + verify.

### O3 — Point the AMS console at the Pulse webhook
- AMS console → each application (e.g. LiveApp) → webhook/listener settings → URL
  `https://beyondkaira.com/webhook/ams`, HMAC secret = `PULSE_WEBHOOK_SECRET` from
  `deploy/.env`.
- Optional (B7, per-source isolation — **live in prod since S8**): use
  `https://beyondkaira.com/webhook/ams/<source_name>` with that source's own secret —
  instructions in `AMS-INTEGRATION.md` §4.5.
- After you set it: the agent re-checks that `webhook: invalid signature` WARNs don't recur (O4).

### O11 — Rotate the Slack CI webhook + reset the other session's clone
- Rotate: api.slack.com/apps → your app → Incoming Webhooks → regenerate, then:
  `gh secret set SLACK_WEBHOOK_URL` (paste the new URL when prompted).
- In the OTHER Claude session's working copy (if it still exists):
  `git fetch && git reset --hard origin/main` (its unpushed `ee4fc00` content is already
  contained in main; the local branch `backup/slack-notify-original` must never be pushed).

## 🟢 Low priority

### U5 — Browser + CSP check of prod
- Open `https://beyondkaira.com` AND `https://pulse.beyondkaira.com`, DevTools (F12) →
  Console, confirm the SPA renders on both with **zero CSP violation messages**.
  (CI asserts CSP byte-exact against a Caddy stack since S4; this is the one-time human
  confirmation on real prod.)

### O8 — Review the dependabot PRs (still 21 open, re-verified)
- `gh pr list --author app/dependabot` — protection requires 1 approving review, which only
  you can give. The caddy digest bump was CI+e2e green (mergeable as-is). The majors
  (vite 8, vitest 4, eslint 10, size-limit 12, plugin-react 6) are riskier — ask a session
  to absorb/verify them as a work order instead of merging blind.

---
*Status snapshot (2026-07-09, post-D-065): **GA DECLARED.** G1 ✅ except O7 · G2 ✅ (prod =
current main since S8) · G3 ✅ (73.2%, floor 70.2) · G4 ✅ · G5 time-gated (CI job promotions
~2026-07-23, agent-owned) · G6 ✅ live-verified · G7 ✅ except O5 · G8 = U3 + U5 + O3 above.
YOUR list here — headlined by the O13 tag choice — is everything that remains.*
