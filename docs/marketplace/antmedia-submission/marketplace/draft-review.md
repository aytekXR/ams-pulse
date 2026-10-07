# Review of Ant Media's draft marketplace page

**Source reviewed:** Google Doc "ams-pulse-page-draft" (Ant Media marketing; read 2026-10-01).
**Reviewed against:** the code and configuration at `main` @ `0cae261` (v0.4.5 + 10 commits),
a fresh build of that tree, and a live run of the published installer (v0.4.5 image).
**Updated 2026-10-07** for the developer's decisions — Pulse is free (PolyForm Shield 1.0.0),
developed by Aytekin Erdogan (individual), page title "Pulse for Ant Media Server", no purchase
link — and for the v0.5.0 release they ship in.

Each claim below is marked:

- **OK** — accurate as written.
- **ADJUST** — substantially right, but needs a qualifier or a tighter word.
- **FIX** — inaccurate, or a command that fails as written. Replacement text is given.
- **ANSWERED** — one of Ant Media's open questions, answered here or in
  [`answers-for-ant-media.md`](answers-for-ant-media.md).
- **DECIDED** — was a developer decision; the decision is recorded in the row.

The corrected page text is assembled in [`marketplace-copy.md`](marketplace-copy.md).

---

## Hero

| # | Draft text | Verdict | Why / replacement |
|---|---|---|---|
| H1 | "Know What's Happening On Every Stream. Instantly." | **OK** | Marketing headline. The dashboard pushes updates over a WebSocket; a newly published stream appeared within 4 s in the live AMS 3.0.3 validation (budget: 10 s). |
| H2 | "No blind spots. No silent outages. No missed viewers." | **ADJUST** | Alerts are opt-in: the shipped default rules are *enabled but muted* and no notification channel exists until the operator adds one. "No silent outages" is true once a rule and a channel are configured. Suggested: *"No blind spots. No missed outages. No guesswork."* — or keep the line and add "once alerting is configured" to the install step. |
| H3 | "It polls the same REST API your server already exposes to tell you who's watching, where, on what device, at what quality" | **FIX** | REST polling gives streams, viewer counts per protocol, ingest bitrate and node health. *Where*, *what device* and viewer-side *quality* come from the Pulse Beacon SDK embedded in the player; *where* additionally needs a GeoIP database the operator supplies (MaxMind GeoLite2 is not bundled). Replacement: *"It reads the REST API your server already exposes for live streams, viewers and ingest health — and its lightweight player SDK measures what each viewer actually experiences: startup time, rebuffering, bitrate and errors, by device."* |
| H4 | "alerts you the moment something breaks" | **OK** | Rules are evaluated every 5 s; detection-to-notification measured at 201 ms in lab validation, and on 2026-10-01 the alert e-mail arrived 0.8 s after the rule fired. |
| H5 | CTA "[Get ams-pulse] — link to vendor's install/pricing page [URL needed]" | **DECIDED** | Pulse is free, so there is no purchase or checkout link. The button becomes **"Install Pulse — free"** → `https://aytekxr.github.io/ams-pulse/get/` (the install page). |
| H6 | CTA "View Documentation — …/docs/user-guide.md" | **OK** | URL resolves (HTTP 200). Keep it on `main`: the docs corrections from this audit (e.g. the IP-privacy statement) live there once merged, while the `v0.4.5` copies still carry the old wording. |
| H7 | Hero visual "[Screenshot needed] Dashboard view with a live alert callout … 'Alert Triggered — Viewer QoE Degraded'" | **ANSWERED** | Delivered: `assets/hero/`. The callout uses a real rule type (`rebuffer_ratio`) that fired in the demo environment; the values on the callout are the ones Pulse recorded. |

## Intro

| # | Draft text | Verdict | Why / replacement |
|---|---|---|---|
| I1 | "developed by [vendor/company name needed]" | **DECIDED** | **"developed by Aytekin Erdogan"** — an individual developer (beyondkaira.com), not a company. |
| I2 | "installs alongside AMS as a single Go binary plus ClickHouse" | **OK** | Plus an embedded SQLite meta store inside the binary (Postgres optional). |
| I3 | "covers the ground core AMS doesn't: alerting and notification channels, player-side QoE measurement, long-term historical analytics, usage and billing reports, and synthetic probes for anomaly detection" | **FIX** | "Synthetic probes for anomaly detection" merges two separate features. *Synthetic probes* actively test stream endpoints (HLS, DASH, WebRTC, RTMP); *anomaly detection* flags statistical deviations in viewers, bitrate and node resources. Replacement: *"…usage and billing reports, synthetic stream probes and anomaly detection."* |
| I4 | "ams-pulse is read-only: it polls AMS's REST API and never writes back" | **OK** | Every AMS call is a GET; the only POST is the login that obtains a session cookie. No AMS setting, stream or file is changed. |
| I5 | "the panel shows live server metrics, ams-pulse adds alerting, viewer-side QoE, 13-month history, reporting and probing on top" | **ADJUST** | About 13 months is the default rollup retention (`PULSE_ROLLUP_TTL_DAYS` = 395; raw events 90 days); both are configurable. Replacement: *"…viewer-side QoE, about 13 months of history (by default), reporting and probing on top."* |

## Key features

| # | Feature card | Verdict | Why / replacement |
|---|---|---|---|
| F1 | Real-Time Alerting — "Configurable notification channels flag problems the moment they happen." | **OK** | Five channel types: email, Slack, Telegram, PagerDuty, signed webhook — all available on every install. |
| F2 | Viewer-Side QoE — "A lightweight Beacon SDK measures quality of experience from the player, not just the server." | **OK** | 3.52 KB gzipped, MIT licensed; available on every install. |
| F3 | 13-Month Historical Analytics — "Long-term trend data goes well beyond what AMS retains natively." | **ADJUST** | A configurable default (see I5). Suggested title: *"About 13 Months of History"*. See also the accuracy note in [LIM-30](https://github.com/aytekXR/ams-pulse/blob/main/docs/known-limitations.md#lim-30-audience-analytics-usage-viewer-minutes-and-qoe-ratios-are-wrong-for-player-sdk-sessions) (known limitation) before quoting view or watch-time totals. |
| F4 | Usage & Billing Reports — "Built-in reporting for usage tracking and billing reconciliation." | **ADJUST** | Available on every install. Egress is an estimate from bitrate × watch time, not measured bytes. Read [LIM-30](https://github.com/aytekXR/ams-pulse/blob/main/docs/known-limitations.md#lim-30-audience-analytics-usage-viewer-minutes-and-qoe-ratios-are-wrong-for-player-sdk-sessions) (known limitation) before using "billing reconciliation" in marketing. |
| F5 | Synthetic Probes & Anomaly Detection — "Proactively tests stream health and flags anomalies before viewers notice." | **ADJUST** | Both available on every install. "Before viewers notice" is a marketing claim; probes do detect an unreachable stream with no viewer involved. |
| F6 | Read-Only, Self-Hosted — "Polls /rest/v2 only — no server-side plugin, JAR, WAR or config changes. Data never leaves your infrastructure." | **FIX** | "No plugin, JAR or WAR" and "data never leaves your infrastructure" are accurate (no phone-home; licenses are verified offline). "No config changes" is not: AMS restricts each application's REST API to `127.0.0.1` by default (`remoteAllowedCIDR`), so the operator must add Pulse's IP per application (the number-one install issue in the FAQ), and Pulse needs an AMS login. Replacement: *"No plugin, JAR or WAR and no AMS restart — allow Pulse's IP in each application's REST filter, give it an AMS login, and it starts polling. Data never leaves your infrastructure."* |

## Installation steps

| # | Draft text | Verdict | Why / replacement |
|---|---|---|---|
| S0 | "start monitoring your Ant Media Server in about 15 minutes" | **OK** | The installer itself finished in about two minutes in our run; the 15 minutes covers prerequisites (Docker, the AMS IP filter). |
| S1 | `curl -fsSL …/install.sh \| bash -s -- --ams-url http://YOUR-AMS:5080 --email you@example.com` | **FIX** | **Fails as written.** The installer needs `--password` too, and with `curl \| bash` there is no terminal to prompt on, so it exits with `Error: --password is required (no TTY available for interactive prompt)`. Corrected command in [`marketplace-copy.md`](marketplace-copy.md) step 1. Also say: needs Docker Engine 24+ with Compose v2; publishes the dashboard on port 8090 over plain HTTP. |
| S1b | "[Screenshot needed] Installation output" | **ANSWERED** | Delivered: `assets/walkthrough/step-1-install-output.png` — the real output of the published installer (token masked). |
| S2 | `helm install pulse oci://ghcr.io/aytekxr/charts/pulse --version 0.3.3 --set pulse.ams.url=… --set pulse.ams.nodeId=node-01` | **FIX** | Two problems. (1) The project's own install runbook labels the Helm chart **experimental — not yet deployed to a real cluster**. (2) The command alone does not produce a working install: it passes no secret (`pulse.secretRef.name`) and therefore no AMS credentials, and it skips the required schema step (`kubectl exec deploy/pulse -- pulse migrate`) — `pulse serve` never creates the ClickHouse tables itself. Recommendation: drop Helm from the marketplace page, or show it as "Kubernetes (experimental) — see the install guide" and link to the full four-step sequence in [`installation-guide.md`](installation-guide.md). Chart `0.3.3` / app `0.4.5` is correct. |
| S3 | `cosign verify …` on the 0.4.5 image (`ghcr.io/aytekxr/ams-pulse`), "Requires Cosign v3+", the v2 note | **OK** | Identity regexp and issuer match `release.yml`. The exact command was re-run on 2026-10-01 with Cosign v3.1.3 and passed. The v2 "no signatures found" behaviour is documented in the project's release notes. The page copy now names `ghcr.io/aytekxr/ams-pulse:0.5.0`, the release this listing ships with. Optional on a marketing page. |
| S4 | "Open the Dashboard — [Dashboard URL / port needed from vendor]" | **ANSWERED** | `http://<your-server>:8090` (default port 8090, plain HTTP; change with `PULSE_HOST_PORT`). Sign in with the one-time admin token the installer prints (`plt_…`). In production put a TLS reverse proxy in front and use `https://pulse.your-domain`. Details: [`dashboard-access.md`](dashboard-access.md). |
| S4b | "[Screenshot needed] Dashboard first-run view" | **ANSWERED** | Delivered: `assets/walkthrough/step-4a-sign-in.png` and `step-4b-first-dashboard.png`, from the same installer run. |

## Trust & validation

| # | Draft text | Verdict | Why / replacement |
|---|---|---|---|
| T1 | "Live-validated against AMS 3.0.3 Enterprise — 46 of 50 scenarios passing; remaining 4 documented, not hidden" | **ADJUST** | True for 3.0.3 (July 2026), but Pulse has since been live-validated on **AMS 3.1.0** (2026-10-07) — the version your customers run now. Suggested: "Live-validated against AMS 3.1.0 and 3.0.3 Enterprise" (see the page copy's Trust section). |
| T2 | "Multi-architecture images, Cosign-signed with SBOM and SLSA provenance" | **OK** | `release.yml`: linux/amd64 + linux/arm64, `sbom: true`, `provenance: true`, keyless cosign signing. |
| T3 | "Releases gated by Trivy vulnerability scanning; CI includes CodeQL, dependency scanning, and nightly AMS compatibility tests" | **ADJUST** | Trivy (HIGH/CRITICAL) and CodeQL: correct. The nightly job tests the AMS REST *wire format* against simulated AMS 2.10–3.0 response profiles, not real AMS installations. Replacement: *"…and nightly AMS wire-format compatibility tests."* |
| T4 | "Viewer IPs SHA-256 hashed (optional full anonymization)" | **FIX** | Pulse does not store viewer IP addresses at all — hashed or otherwise. The IP is used only in memory, for an optional GeoIP country lookup, and can be truncated first (`PULSE_ANONYMIZE_IP` zeroes the last IPv4 octet / last 80 IPv6 bits); it is not "full anonymization". Replacement: *"Viewer IP addresses are never stored (an optional GeoIP lookup uses them transiently, and can be set to truncate them first)."* |
| T5 | "secrets encrypted at rest; admin changes audit-logged" | **OK** | AES-256-GCM for stored credentials; `/admin/audit-log` records admin writes. |
| T6 | "Upgrade-tolerant — AMS upgrades don't require ams-pulse changes" | **ADJUST** | True so far (read-only integration, tolerant decoding): the move from AMS 3.0.3 to 3.1.0 needed no Pulse change — an unmodified v0.4.5 build resumed collecting from 3.1.0 — but it is not a guarantee for every future AMS release. Replacement: *"Upgrade-tolerant — read-only REST integration; the move from AMS 3.0.3 to 3.1.0 required no Pulse change."* |

## Closing CTA

| # | Draft text | Verdict | Why / replacement |
|---|---|---|---|
| C1 | "no config changes, no downtime, no credentials shared" | **FIX** | "No downtime": correct (AMS is never restarted). "No config changes": see F6. "No credentials shared": Pulse *does* need an AMS login; the accurate promise is that it stays on the operator's own server. Replacement: *"No plugin, no downtime — and your AMS credentials never leave your own server."* |

## "Still needed" table

| Item | Status |
|---|---|
| Logo / brand mark | **Delivered** — `assets/branding/` (SVG, outlined SVG, transparent PNG; product brand mark). |
| Dashboard & alert screenshots | **Delivered** — `assets/screenshots/`, `assets/hero/`, `assets/walkthrough/`. |
| Company / legal entity name | **Answered** — individual developer, Aytekin Erdogan (no company). |
| Pricing / purchase link | **Answered** — free (every feature, commercial use included; at least the first year); no purchase link — the button points at the install page. |
| Dashboard URL / port | **Answered** — `http://<your-server>:8090`, HTTPS behind a reverse proxy in production. |

## Naming

The draft calls the product **"ams-pulse"** throughout. The product's brand name — on the
logo, in the UI and in every document — is **Pulse**; `ams-pulse` is the repository and
package name. **Decided:** the page title is **"Pulse for Ant Media Server"**, with `ams-pulse` in commands
and URLs.
