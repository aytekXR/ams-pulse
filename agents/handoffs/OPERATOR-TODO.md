# Operator TODO — the items only YOU can do (updated by SESSION-07, D-064, 2026-07-09)

> **Audience: the human operator.** Work these in parallel while an agent session runs — none
> of them conflict with agent work. The ledger of record is `ROADMAP.md` §5; this file is the
> actionable view and is refreshed at every session close. When you finish an item, just tell
> the agent (or do nothing — every session start re-verifies each item automatically).
> **Never commit secret VALUES anywhere; `deploy/.env` and `oguz-testing.md` are gitignored.**

## 🔴 High priority — these gate the GA declaration

### O5 — Choose the project LICENSE  *(the LAST G7 gap; legal decision)*
- Decide the license for this repo. Considerations: the SDK is already MIT
  (`sdk/beacon-js/LICENSE`); the server has commercial tiers (free/pro/business/enterprise),
  so candidates people commonly pick here: Apache-2.0 (permissive), AGPL-3.0 (copyleft,
  protects the SaaS angle), BUSL-1.1 (source-available with commercial restriction).
- **Your action:** pick one and tell the agent — it drafts the LICENSE file same-session.

### O12 — Enable secret-scanning + push-protection  *(NEW, D-064; repo is PUBLIC, both are OFF)*
- Click path: GitHub → `aytekXR/ams-pulse` → Settings → Advanced Security →
  enable **Secret scanning**, then **Push protection**.
- Or paste this in a session prompt (runs as you): `! gh api -X PATCH repos/aytekXR/ams-pulse -f 'security_and_analysis[secret_scanning][status]=enabled' -f 'security_and_analysis[secret_scanning_push_protection][status]=enabled'`
- Why: platform-level guard against the next O11-class incident (a live webhook URL once sat
  in a local commit on this public repo).

### U3 — Activate a Pro+ Pulse license in prod
- Obtain the license key (this is a **Pulse** license, separate from the AMS one).
- Add to `deploy/.env` on the VPS: `PULSE_LICENSE_KEY=<key>` (never commit it).
- Timing tip: SESSION-08 starts with a prod rollout (WO-A) — if the key is in `deploy/.env`
  before/while that runs, the restart picks it up for free and the session can live-verify the
  QoE/beacon chain immediately after.
- Why: beacon/QoE ingest is 403 `LICENSE_REQUIRED` on Free — no viewer QoE data flows in prod
  until this lands; rebuffer/error-rate alerts have nothing to read.

## 🟡 Medium priority

### O7 — Make the GHCR package public  *(last G1 bit)*
- Click path: github.com/aytekXR → Packages → `ams-pulse` → Package settings →
  Danger zone → **Change visibility → Public**.
- Verify (agent re-checks too): `docker pull ghcr.io/aytekxr/ams-pulse:v0.1.0` from any
  unauthenticated machine, and `cosign verify` per the header comment in
  `.github/workflows/release.yml`.

### O3 — Point the AMS console at the Pulse webhook
- AMS console → each application (e.g. LiveApp) → webhook/listener settings → URL
  `https://beyondkaira.com/webhook/ams`, HMAC secret = `PULSE_WEBHOOK_SECRET` from
  `deploy/.env`.
- Optional (B7, per-source isolation): use `https://beyondkaira.com/webhook/ams/<source_name>`
  with that source's own secret — full instructions in `AMS-INTEGRATION.md` §4.5.
- The Pulse side has been live since D-054; 24h of Caddy logs show **zero** webhook traffic,
  so nothing is configured on the AMS side yet. After you set it: the agent re-checks that the
  old `webhook: invalid signature` WARN does not recur (O4).

### O11 — Rotate the Slack CI webhook + reset the other session's clone
- Rotate: api.slack.com/apps → your app → Incoming Webhooks → regenerate the webhook, then:
  `gh secret set SLACK_WEBHOOK_URL` (paste the new URL when prompted).
- In the OTHER Claude session's working copy (if it still exists):
  `git fetch && git reset --hard origin/main` (its unpushed `ee4fc00` content is already
  contained in `bc15d43`; the local branch `backup/slack-notify-original` must never be pushed).

## 🟢 Low priority

### U5 — Browser + CSP check of prod
- Open `https://beyondkaira.com` AND `https://pulse.beyondkaira.com` in a real browser,
  open DevTools (F12) → Console, confirm the SPA renders on both with **zero CSP violation
  messages**. Report anything red — a fix is usually same-day.

### O8 — Review the dependabot PRs (21 open)
- `gh pr list --author app/dependabot` — branch protection requires 1 approving review, which
  only you can give. The caddy digest bump was already CI+e2e green (mergeable as-is). The
  major bumps (vite 8, vitest 4, eslint 10, size-limit 12, plugin-react 6) are riskier — you
  can ask a session to absorb/verify them as a work order instead of merging blind.

## ⏭ Decision coming up (SESSION-08 will ask — you can pre-decide)

- **GA release tag: `v1.0.0` or `v0.2.0`?** SESSION-08 prepares the release material and will
  NOT push any tag without your explicit word. The tag triggers the full release pipeline
  (CI-gated, Trivy, SBOM, cosign) and a prod rollout carrying it.

---
*Status snapshot (2026-07-09, post-D-064): G1 ✅ except O7 · G2 ❌ (prod rollout = agent's S8
WO-A, not yours) · G3–G4 ✅ · G5 time-gated (~2026-07-23) · G6 ✅ · G7 ✅ except O5 · G8 = U3 +
U5 + O3 above. Once the agent lands S8 WO-A and the promotion clocks expire, YOUR list here is
exactly what separates the project from GA.*
