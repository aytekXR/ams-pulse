# Support Policy

> **Support policy set for launch (operator-delegated, D-169).**
> All SLA, channel, email, hours, and version-support decisions are resolved below.
> ✅ The **support@beyondkaira.com** mailbox is **provisioned and live** (operator confirmed
> 2026-07-25, D-171) — no provisioning task remains. Operator may override any value.
> Marketplace-readiness checklist row 7.

---

## 1. Support channels

| Channel | Where | Notes |
|---|---|---|
| GitHub Issues | [github.com/aytekXR/ams-pulse/issues](https://github.com/aytekXR/ams-pulse/issues) | Source-available (PolyForm NC); public bug tracking |
| Email | support@beyondkaira.com | Non-security requests; provisioned and live (D-171) |
| Security vulnerabilities | aytek@beyondkaira.com | **Do not open a public issue** — see §3 |

### Response-time targets by tier

| Tier | Support channel | First-response target |
|---|---|---|
| **Free** | GitHub Issues only | Community / best-effort; no SLA |
| **Pro** | GitHub Issues + email | **2 business days** |
| **Business** | GitHub Issues + email | **1 business day** |
| **Enterprise** | Named contact + shared Slack/Teams channel | **4 business hours** (critical); named contact + onboarding assistance; custom SLA addendum available |

Support email: **support@beyondkaira.com** — provisioned and live (operator, 2026-07-25).
Set the same address in the marketplace listing. A ticketing alias (Freshdesk/Zendesk) can front
it later without changing the published address.
_Set for launch (operator-delegated, D-169) — subject to operator override._

**Trial key requests:** email **support@beyondkaira.com** with your deployment
details to request a 14-day Pro trial key (no credit card required). The key
typically arrives within 1 business day and activates via Settings → License.
See `docs/licensing-public.md` §3 for the full trial policy.

Business hours: **Monday–Friday 09:00–18:00 UTC**, excluding public holidays.
State this explicitly in the Enterprise SLA addendum.
_Set for launch (operator-delegated, D-169) — subject to operator override._

---

## 2. Supported versions

| Version | Status |
|---|---|
| v0.4.x | Supported — security fixes and bug fixes |
| < v0.4.0 | Not supported — upgrade to the latest v0.4.x release |

This matches the supported-versions table in `SECURITY.md` exactly. "Supported" means security
patches are backported to the current v0.4.x line.

Previous minor (e.g. v0.3.x): best-effort for **90 days** after a new minor GA, then EOL.
Extend this table with that row when applicable and update `SECURITY.md` to match.
_Set for launch (operator-delegated, D-169) — subject to operator override._

---

## 3. Security vulnerability reports

Report vulnerabilities by email to **aytek@beyondkaira.com**. Include a description of the issue,
reproduction steps, and potential impact. You will receive a response within **5 business days**.
**Do not open a public GitHub issue** for security vulnerabilities.

This policy is published in `SECURITY.md` and is already decided — no operator action required here.

---

## 4. What to include in a bug report

Attach the following to every bug report (GitHub Issue or email). Omitting items slows triage.

1. **Pulse version** — output of `pulse version`
   ```
   pulse 0.4.x (commit abc1234, built 2026-01-01T00:00:00Z)
   ```

2. **Diagnostic bundle** — output of `pulse diag`
   Credentials are redacted automatically (AMS URL passwords are replaced with `xxxxx`
   via `url.URL.Redacted()`; this is code-verified as of D-136). Attach the full output.

3. **Container state** — output of `docker compose ps` from your deploy directory.

4. **Relevant log lines** — `docker compose logs --tail=200 pulse` and, if relevant,
   `docker compose logs --tail=100 clickhouse`.

5. **AMS version** — visible in the AMS web panel under **About**, or via the AMS REST API
   `/LiveApp/rest/v2/version`.

A bug-report issue template is available at `.github/ISSUE_TEMPLATE/bug_report.yml`
and pre-fills these fields for GitHub Issues submissions.

---

## 5. Feature requests

Open a GitHub Issue with the label **enhancement** and describe:

- The use-case / problem you are solving (not just the proposed solution).
- Your deployment scale (stream count, viewer count, AMS version).
- Whether this is blocking a purchase or renewal decision.

> **OPERATOR-DECISION** Define the public roadmap artifact — proposed: a pinned GitHub Issue or a
> public GitHub Project board. Link it from the README and marketplace listing. Items accepted into
> the roadmap are tagged **roadmap** in the issue tracker.

Enterprise customers may submit feature requests directly through their named contact; prioritisation
is subject to the commercial agreement.

---

*Support policy set for launch (operator-delegated, D-169). Remaining open `OPERATOR-DECISION`
item (public roadmap §5) is non-blocking for marketplace submission.*
