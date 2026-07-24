<!--
  DRAFT — INTERNAL. External use gated on operator review of
  docs/assessment/final-assessment.md (D-081).
-->

> **DRAFT — INTERNAL. External use gated on operator review of
> `docs/assessment/final-assessment.md` (D-081).**

---

# Pulse — Licensing Explained

**Last updated:** 2026-07-22

This page is the human-readable guide to Pulse licensing. It covers the
open-source licenses that govern the code, the commercial tiers and what each
unlocks, how license keys work, trial access, and frequently asked questions.

The *license texts themselves* always govern; summaries on this page are for
clarity and do not modify or replace any license. Where this page and a
license text conflict, the license text prevails.

---

## 1. Software licenses

Pulse is made up of two independently licensed components.

### 1.1 Server, web UI, and deploy tooling — PolyForm Noncommercial 1.0.0

The main Pulse codebase (everything in `server/`, `web/`, and `deploy/`) is
released under the **PolyForm Noncommercial 1.0.0** license. The full text is
in the root `LICENSE` file and at
<https://polyformproject.org/licenses/noncommercial/1.0.0>.

**What noncommercial users may do freely:**

- Use, run, and operate Pulse for any noncommercial purpose — including
  personal study, research, hobby projects, private entertainment, and
  religious observance.
- Self-host Pulse on your own infrastructure.
- Modify the source code and make derivative works, for noncommercial purposes.
- Share copies (modified or unmodified) with others, provided you pass along
  these license terms and any Required Notices.

**Who qualifies as noncommercial without a separate agreement:**

Charitable organisations, educational institutions, public research
organisations, public-safety and health organisations, environmental
protection organisations, and government institutions may use Pulse under the
PolyForm NC terms regardless of their funding source.

**What requires a commercial license:**

Any use that does not qualify as noncommercial under the above definitions —
including running Pulse as part of a commercial service, embedding it in a
paid product, or using it in an organisation whose primary activity is
commercial — requires a separate commercial license from the copyright holder.
The commercial license is delivered through the tier subscription model
described in §2 below.

> The license text itself governs all rights and restrictions. This summary is
> provided for readability only.

### 1.2 Beacon SDK — MIT

The player QoE beacon SDK (`sdk/beacon-js/`) is released under the **MIT
License** (`sdk/beacon-js/LICENSE`). You may embed the beacon in any player —
including commercial products and services — freely, without a commercial
license from Pulse. The MIT license places no restriction on commercial use.

---

## 2. Commercial tiers

A Pulse commercial license is delivered as a signed **license key** that is
activated on your Pulse deployment. The key encodes your tier and
entitlements; no phone-home is required for verification.

### 2.1 Tier entitlements

The table below reflects the runtime entitlements enforced by
`server/internal/license/license.go`. The Code column names the authoritative
source; where the PRD and code diverge, the code governs (divergences are
noted).

| Tier | Price | Max Nodes | Max Streams | Retention | Alert Channels | Data API | White-label |
|------|-------|-----------|-------------|-----------|----------------|----------|-------------|
| **Free** | $0/month | 1 | Unlimited | 7 days | Email only | No | No |
| **Pro** | $99/month (PROPOSED) | 10 | Unlimited | 90 days | Email, Slack, Telegram | Yes | No |
| **Business** | $299/month (PROPOSED) | 50 | Unlimited | 396 days (13 months) | Email, Slack, Telegram, PagerDuty, Webhook | Yes | No |
| **Enterprise** | from $799/month (PROPOSED) | Unlimited | Unlimited | Unlimited | All channels | Yes | Yes |

**Notes on the table:**

- **Max Streams** is unlimited at every tier; there is no per-tier stream cap.
- **Max Nodes (Pro):** The PRD §7.11 states "1 to 2 nodes" but the code enforces `MaxNodes = 10`. The code is the operative value. The deliberate tier ladder is Free 1 / Pro 10 / Business 50 / Enterprise unlimited — Pro is positioned for multi-node edge networks; Business adds multi-tenant billing and reporting.
- **Retention** for Enterprise is absent/null in the claims (`retention_days`
  absent or null maps to unlimited at runtime via `buildEntitlements`; the
  value `0` in the claims JSON is also treated as unlimited, but `null` is the
  canonical encoding per the `license.go` header comment).

**Additional features by tier:**

| Feature | Minimum tier |
|---------|-------------|
| Player QoE beacon SDK events | Pro+ |
| CSV data export | Business+ |
| Usage and billing reports (viewer-minutes, egress, VoD storage) | Business+ |
| Multi-tenant billing | Business+ |
| Prometheus `/metrics` endpoint | Business+ |
| Anomaly detection (Welford baselines) | Enterprise |
| White-label PDF reports | Enterprise |
| SSO / OIDC | Enterprise |

### 2.2 What a license key is

A Pulse license key is a **self-contained, offline-verifiable signed token**.
It does not require a connection to any Pulse or vendor server to activate or
to stay active. The key is structured as:

```
base64(claimsJSON) . base64(ed25519_signature)
```

The claims JSON encodes your tier, node limit, retention period, feature
flags, and optional expiry. The signature is produced with the vendor's
ed25519 private key and verified at startup against the vendor public key
embedded in your deployment. A key that cannot be verified falls back
gracefully to the Free tier — the server continues to run.

### 2.3 Activation paths

You can activate a license key by any of three methods:

1. **Environment variable** — set `PULSE_LICENSE_KEY=<key>` in your
   environment before starting Pulse. The key is read at boot.
2. **Offline file** — set `PULSE_LICENSE_FILE=<path>` to a file containing
   the key string. Useful for air-gapped deployments where environment
   variables are not practical.
3. **Runtime API** — send `PUT /api/v1/admin/license {"key":"<key>"}` with an
   admin bearer token. The key takes effect immediately without a restart; use
   `GET /api/v1/admin/license` to confirm the active tier and expiry.

### 2.4 Expiry and graceful downgrade

Keys for subscription licenses carry an `expires_at` timestamp. When a key
expires:

- The server continues to run without interruption.
- The active tier reverts to **Free**.
- Paid-feature API endpoints return `403 LICENSE_REQUIRED` until a valid key
  is re-activated.
- No data is lost; data already collected is retained up to the Free-tier
  retention window (7 days).

Perpetual licenses (no `expires_at`) do not expire.

---

## 3. Trial access

> **OPERATOR-DECISION-PENDING** — The trial-key programme described in this
> section is proposed but not yet confirmed. The mechanism (self-serve vs.
> manual issuance, duration, renewal) is an operator decision pending before
> this page can be finalised for external publication. The wording below
> mirrors `docs/marketplace/listing-draft.md` §7.

A **14-day Pro trial key** is the proposed standard onboarding path for new
users who want to evaluate paid features before purchasing. The trial key
provides full Pro-tier entitlements (10 nodes, 90-day retention, Slack and
Telegram alerts, QoE beacon integration) for the trial period.

To request a trial key: [support channel — NEEDS-OPERATOR].

After the 14-day trial period the deployment reverts to Free tier
automatically (see §2.4). No credit card is required for the trial.

---

## 4. Pricing

The prices shown in §2.1 are **PROPOSED** figures from `docs/prd-report.md`
§7.11. They have not been confirmed or published through the Ant Media
marketplace at the time of writing. **Published pricing is pending final
operator confirmation** and may differ from the figures shown here. Check the
official listing or contact the vendor for current pricing before making a
purchasing decision.

---

## 5. Frequently asked questions

### Can I use the Free tier for a commercial deployment?

No. The Free tier is not a commercial license. The Pulse server, web UI, and
deploy tooling are licensed under PolyForm Noncommercial 1.0.0; any
commercial deployment requires a commercial tier (Pro, Business, or
Enterprise). The Free tier is available to noncommercial users (see §1.1) at
no cost.

### Does the beacon SDK have the same restriction?

No. The beacon SDK (`sdk/beacon-js/`) is MIT-licensed and may be embedded in
commercial players and products freely, without a commercial Pulse license.
Only the server-side components are under PolyForm Noncommercial 1.0.0.

### What happens when my subscription license expires?

Pulse downgrades gracefully to Free tier (see §2.4). The server keeps
running; only paid-tier features become unavailable. Renew your license key
and activate it via any of the three paths in §2.3 to restore full access.

### Can I run Pulse in an air-gapped environment?

Yes. License verification is entirely offline — no connection to any external
server is needed. Activate via environment variable (`PULSE_LICENSE_KEY`) or
offline file (`PULSE_LICENSE_FILE`) before starting the server. The
Enterprise tier is the recommended tier for air-gapped deployments (see §6
of `docs/marketplace/listing-draft.md`).

### Can I modify the source code and run my modified version commercially?

Modifications for noncommercial purposes are permitted under PolyForm NC 1.0.0.
Running a modified version commercially (including any derived service) still
requires a commercial license from the copyright holder.

---

*Sources: `LICENSE` (PolyForm NC 1.0.0), `sdk/beacon-js/LICENSE` (MIT),
`docs/licensing.md`, `server/internal/license/license.go:90–150`,
`docs/marketplace/listing-draft.md` §5–7.*
