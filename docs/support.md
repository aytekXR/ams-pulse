# Support Policy

> **Skeleton — operator must finalize all `OPERATOR-DECISION` items before publishing.**
> Marketplace-readiness checklist row 7.

---

## 1. Support channels

| Channel | Where | Notes |
|---|---|---|
| GitHub Issues | [github.com/aytekXR/ams-pulse/issues](https://github.com/aytekXR/ams-pulse/issues) | Source-available (PolyForm NC); public bug tracking |
| Email | See tier table below | Non-security requests; rotate to a shared alias before GA |
| Security vulnerabilities | aytek@beyondkaira.com | **Do not open a public issue** — see §3 |

### Response-time targets by tier

| Tier | Support channel | First-response target |
|---|---|---|
| **Free** | GitHub Issues only | Community / best-effort; no SLA |
| **Pro** | GitHub Issues + email | > **OPERATOR-DECISION** Proposed default: **3 business days** |
| **Business** | GitHub Issues + email | > **OPERATOR-DECISION** Proposed default: **1 business day** |
| **Enterprise** | Named contact + shared Slack/Teams channel | > **OPERATOR-DECISION** Proposed default: **4 business hours + named CSE** |

> **OPERATOR-DECISION** Support email address — proposed default: `support@beyondkaira.com` (or a
> ticketing alias such as Freshdesk/Zendesk). Set the same address in the marketplace listing.

> **OPERATOR-DECISION** Business hours definition — proposed default: Monday–Friday 09:00–18:00 UTC,
> excluding public holidays (specify which calendar). State this explicitly in the Enterprise SLA addendum.

---

## 2. Supported versions

| Version | Status |
|---|---|
| v0.4.x | Supported — security fixes and bug fixes |
| < v0.4.0 | Not supported — upgrade to the latest v0.4.x release |

This matches the supported-versions table in `SECURITY.md` exactly. "Supported" means security
patches are backported to the current v0.4.x line.

> **OPERATOR-DECISION** If you intend to offer best-effort support for a previous minor (e.g. v0.3.x)
> after a new minor is released, extend this table with that row and update `SECURITY.md` to match.
> Proposed window: 90 days after the new minor GA, then EOL. Confirm this cadence before publishing.

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

> **OPERATOR-DECISION** Decide whether to provide a bug-report issue template
> (`.github/ISSUE_TEMPLATE/bug_report.yml`) that pre-fills these fields — recommended
> for marketplace self-service.

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

*This document is a skeleton. Replace all `OPERATOR-DECISION` blocks with final decisions before
publishing to the marketplace or the public docs site.*
