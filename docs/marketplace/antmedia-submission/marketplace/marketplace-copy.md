# Marketplace page copy — Pulse for Ant Media Server

Finalized text for Ant Media's marketplace page, in the structure of their draft (hero → intro
→ key features → installation → trust → closing CTA). Every statement was checked against the
code and live runs (2026-10-01 and 2026-10-07); the claim-by-claim review of the draft is in
[`draft-review.md`](draft-review.md). Every decision is final — there are no open placeholders.

---

## Product name

**Pulse for Ant Media Server** — Analytics, QoE Monitoring & Alerting
*(repository and package name: `ams-pulse`, used in commands)*

## Short description (≤ 250 characters)

Self-hosted analytics, viewer QoE and alerting for Ant Media Server. Pulse runs beside AMS,
reads its REST API, measures what viewers experience, and alerts by e-mail, Slack, Telegram,
PagerDuty or webhook.

*(206 characters)*

---

## Hero

**Headline:** Know What's Happening On Every Stream. Instantly.

**Subheadline:** No blind spots. No missed outages. No guesswork.

**Body:** Pulse is a self-hosted analytics, QoE monitoring and alerting platform for Ant Media
Server. It reads the REST API your server already exposes for live streams, viewers and
ingest health — and its lightweight player SDK measures what each viewer actually
experiences: startup time, rebuffering, bitrate and errors. When something breaks, Pulse
alerts you the moment it happens.

**Buttons:** (there is no purchase link — Pulse is free)
- **Install Pulse — free** → `https://aytekxr.github.io/ams-pulse/get/`
- **View Documentation** → `https://github.com/aytekXR/ams-pulse/blob/main/docs/user-guide.md`

**Hero visual:** `assets/hero/pulse-hero-alert-1920x1080.png` (annotated: live dashboard +
"Alert triggered — Ingest bitrate below 1,500 kbps" + the e-mail Pulse delivered).
Clean alternative without annotation: `assets/hero/pulse-hero-dashboard-1920x1080.png`.

---

## Intro — Self-hosted analytics, QoE and alerting for Ant Media Server

Pulse is a self-hosted analytics, QoE monitoring and alerting platform for Ant Media Server,
developed by **Aytekin Erdogan**. It installs alongside AMS as a single Go binary plus
ClickHouse and covers the ground core AMS doesn't: alerting and notification channels,
player-side QoE measurement, long-term historical data, usage reports, synthetic stream probes
and anomaly detection. **Pulse is free** — every feature, with no license key, including for
commercial use.

Pulse is read-only: it calls the AMS REST API with GET requests only (plus the login that
obtains its session) and never changes a stream, an application setting or a file. It runs
alongside the Ant Media management panel without conflict — the panel shows live server
metrics; Pulse adds alerting, viewer-side QoE, about 13 months of history (by default),
reporting and probing on top.

---

## Key features

**Real-time alerting.** Rules on viewers, ingest bitrate, stream offline, node health, CPU and
memory, packet loss, jitter and more — evaluated every 5 seconds and delivered by e-mail,
Slack, Telegram, PagerDuty or HMAC-signed webhook, with mute, cooldown and maintenance
windows.

**Live operations dashboard.** Every stream, viewer count, protocol mix (WebRTC, HLS, RTMP,
DASH) and ingest bitrate, pushed live to the browser over a WebSocket.

**Viewer-side QoE.** A 3.5 KB (gzipped), MIT-licensed beacon SDK for hls.js, `<video>`/video.js
and the AMS WebRTC player measures startup time, rebuffering, bitrate switches and errors
from the player, not just the server.

**Ingest health.** A 0–100 health score per stream from bitrate, packet loss and jitter (and
frame rate where AMS reports it), with per-stream timelines that show exactly when an encoder
degraded.

**Synthetic probes and anomaly detection.** Probes test HLS, DASH, WebRTC and RTMP endpoints
on a schedule; anomaly detection learns a baseline for viewers, bitrate and node resources
and flags deviations.

**History, reports and APIs.** Raw events for 90 days and hourly/daily rollups for about 13
months by default (both configurable), usage reports with CSV/PDF and S3 export, a documented
REST/WebSocket API and a token-protected Prometheus `/metrics` endpoint.

**Read-only, self-hosted integration.** No plugin, JAR or WAR and no AMS restart — allow
Pulse's IP in each application's REST filter, give it an AMS login, and it starts polling.
There is no license key and no phone-home; your data never leaves your infrastructure.

> Audience analytics, QoE rebuffer ratio and usage-report viewer-minutes have known accuracy
> defects for player-SDK sessions (documented publicly as LIM-30; the fix is on the roadmap).
> Nothing on this page quotes those numbers — keep it that way until the fix ships.

---

## Installation — Install Pulse and start monitoring in about 15 minutes

**Before you start:** a Linux host with Docker Engine 24+ and Docker Compose v2, network
access from that host to your AMS REST API (port 5080), an AMS admin login, and — in each
AMS application Pulse should monitor — Pulse's IP added to the application's REST IP filter
(`remoteAllowedCIDR`; the default `127.0.0.1` blocks it).

### 1. Install Pulse

Run the quickstart installer on your server:

```sh
curl -fsSL https://raw.githubusercontent.com/aytekXR/ams-pulse/main/deploy/quickstart/install.sh \
  | bash -s -- --ams-url http://YOUR-AMS:5080 --email YOUR-AMS-ADMIN-EMAIL --password 'YOUR-AMS-PASSWORD'
```

All three flags are required when the script is piped into `bash`. The installer pulls the
signed v0.5.0 image, starts Pulse and ClickHouse, confirms Pulse can reach your AMS, and
prints a one-time admin token. It exits `0` when Pulse is installed and reaching AMS, `2` when
it is installed but cannot reach AMS (check the URL, login and REST filter). No license key is
needed — every feature is included.

Screenshot: `assets/walkthrough/step-1-install-output.png` (real run; token and password masked).

### 2. Kubernetes (experimental)

A Helm chart is published (`oci://ghcr.io/aytekxr/charts/pulse`) but
is **experimental — not yet deployed to a real cluster**. It needs a Kubernetes Secret and a
separate schema step; follow the installation guide rather than a one-line command.
*(Recommendation: leave Helm off the marketplace page until the chart has been validated on a
cluster.)*

### 3. Verify the image signature (optional)

Pulse images are signed with Cosign and ship with an SBOM and SLSA provenance. Requires
**Cosign v3 or newer** (Cosign v2 reports "no signatures found" on these images because they
use OCI 1.1 referrers):

```sh
cosign verify \
  --certificate-identity-regexp '^https://github\.com/aytekXR/ams-pulse/\.github/workflows/release\.yml@refs/tags/v.+$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/aytekxr/ams-pulse:0.5.1
```

### 4. Open the dashboard

Open **`http://<your-server>:8090`** and sign in with the admin token the installer printed
(`plt_…`). The live dashboard opens straight away and shows your AMS streams within seconds
— the green **Live** badge in the top-right corner means updates are streaming in. Port 8090
is the default (`PULSE_HOST_PORT` changes it) and serves plain HTTP: for anything beyond a
private network, put a TLS reverse proxy in front and use `https://pulse.your-domain`.

Screenshots: `assets/walkthrough/step-4a-sign-in.png`, `assets/walkthrough/step-4b-dashboard-first-run.png`.

---

## Trust & validation

- Live-validated against AMS 3.1.0 and 3.0.3 Enterprise. On 3.1.0, alert delivery, player QoE
  and installation were verified end to end, and none of the scenario failures was an AMS
  incompatibility; on 3.0.3, 46 of 50 scenarios pass and the remaining 4 are documented.
- Multi-architecture images (amd64, arm64), Cosign-signed, with SBOM and SLSA provenance.
- Releases gated by Trivy vulnerability scanning; CI includes CodeQL, dependency audit and
  nightly AMS wire-format compatibility tests.
- Viewer IP addresses are never stored (an optional GeoIP lookup uses them transiently, and can
  truncate them first); stored credentials are encrypted (AES-256-GCM); admin changes are
  audit-logged.
- Upgrade-tolerant — read-only REST integration; the move from AMS 3.0.3 to 3.1.0 required no
  Pulse change.

---

## Closing CTA

**Ready to see every stream clearly?**

Install Pulse alongside your existing Ant Media Server — free, no plugin, no downtime, and your
AMS credentials never leave your own server.

- **Install Pulse — free** → `https://aytekxr.github.io/ams-pulse/get/`
- **View Documentation** → `https://github.com/aytekXR/ams-pulse/blob/main/docs/user-guide.md`

---

## Pricing

**Free.** Every feature, with no license key, no node or retention limits, and commercial use
included — under the PolyForm Shield License 1.0.0, whose one restriction is that Pulse may not be
used to build a competing product. This is the launch policy for at least the first year (from
October 2026). There is no purchase link, no trial and no paid tier.

## Developer

Developed by **Aytekin Erdogan** — an individual developer (beyondkaira.com).

## Support and contact

- Support: **support@beyondkaira.com** or GitHub Issues — best effort, with no guaranteed
  response times.
- Security reports: **aytek@beyondkaira.com** (not a public issue).

## Links

| What | URL |
|---|---|
| Product site | https://aytekxr.github.io/ams-pulse/ |
| Install page | https://aytekxr.github.io/ams-pulse/get/ |
| Source & issues | https://github.com/aytekXR/ams-pulse |
| User guide | https://github.com/aytekXR/ams-pulse/blob/main/docs/user-guide.md |
| Install runbook | https://github.com/aytekXR/ams-pulse/blob/main/docs/runbooks/install.md |
| Licensing | https://github.com/aytekXR/ams-pulse/blob/main/docs/licensing-public.md |
| Known limitations | https://github.com/aytekXR/ams-pulse/blob/main/docs/known-limitations.md |
| Container image | `ghcr.io/aytekxr/ams-pulse:0.5.1` |
