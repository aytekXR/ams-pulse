<!--
  DRAFT — INTERNAL. External use gated on operator review of
  docs/assessment/final-assessment.md (D-081). This brief is for the OPERATOR
  to use in the Ant Media developer meeting; the "share" column marks what can
  be handed over vs. what is internal negotiation posture.
-->

# Ant Media developer meeting — brief & agenda

**Purpose:** the meeting Ankush Banyal offered once "the plugin is fully ready and you
have the relevant documentation." Goals: (1) get the qualification steps, (2) settle the
listing artifact/format questions, (3) close the G-27/G-21 technical questions, (4) open
the business terms conversation.

**Attach / share:** [`submission-package.md`](submission-package.md) (artifact index),
[`../overview.md`](../overview.md), [`../compatibility.md`](../compatibility.md), the
capacity number if the load lane has run. Everything else in this repo stays internal
until the D-081 review clears it.

---

## Suggested agenda (60 min)

### 1. Product demo (10 min)

Mirror the demo Ankush already saw, deeper: live dashboard against a real AMS →
fire an alert (Slack/email) → viewer QoE from the beacon SDK → 13-month analytics →
synthetic probes → usage report with scheduled PDF. Script:
[`demo-video-script.md`](demo-video-script.md).

### 2. Technical compatibility questions (G-27 / G-21) — 10 min

Pre-answers from the **public** `ant-media/Management-panel-reborn` repo
(commit `c4a0235`, reviewed 2026-07-22 — public info only; confirm it reflects the
shipping plan):

| # | Question | Public-repo evidence (to confirm) |
|---|---|---|
| Q-i | Do `/rest/v2/*` management paths + response envelopes survive the panel revamp, or is a v2→v3 jump planned? | New panel's single API prefix is `/rest/v2` (`src/lib/api/client.ts`); zero v3 paths in 232 files → **likely survives; please confirm no server-side re-versioning is planned** |
| Q-ii | Does the new panel introduce a new auth mechanism (OAuth2/OIDC/JWT) replacing the cookie flow? Timeline? | Login is still `POST /rest/v2/users/authenticate` + session cookie; no auth libraries in its package.json → **likely unchanged; confirm** |
| Q-iii | In AMS 3.0.3 **cluster** mode, is `GET /rest/v2/cluster/nodes` a flat array or only the paginated `…/{offset}/{size}` form? (Settles our G-21.) | New panel calls the **paginated** form (`src/lib/api/endpoints/cluster.ts`) — evidence for pagination-canonical; **we need the definitive answer for 3.0.3 server behavior** |
| Q-iv | The panel repo references new endpoints (`/system-resources/history`, `…/metrics-history`) on an unreleased AMS branch — will these be public/stable REST surface we may consume? | Additive endpoints, `feature/management-panel-analytics` branch per the repo's status doc |

### 3. AMS data-source questions (from our validation program) — 10 min

From `docs/assessment/final-assessment.md` §6 (full context there):

- **Q1** — Webhook HMAC signing: any plans to sign AMS webhooks? (We require HMAC and
  built a signed-webhook path; AMS 3.0.3 sends unsigned.)
- **Q2** — `hlsViewerCount` sliding-window semantics (~9× inflation factor we measured):
  intended? Documented factor?
- **Q3** — WHEP viewer counts: exposure plans?
- **Q4** — Analytics-log FPS field: will `currentFPS` return to the REST surface, or is
  the analytics log the intended source?

### 4. Qualification & listing process (15 min) — the A1–A10 assumptions

Ask in this order (details: [`submission-process.md`](submission-process.md) §2):

1. **Listing artifact type (A1):** Pulse is a standalone self-hosted service (Go binary +
   ClickHouse next to AMS, read-only REST integration). Bitmovin lists as a WAR;
   GST-Ant Fusion as an external process. **Can Pulse list as-is, or do you want any
   AMS-side artifact?**
2. **The qualification steps** your dev team defined — checklist, artifact formats,
   screenshot/logo/video specs (A2/A3), docs hosting (A8).
3. **Review flow + timeline (A5); security review** — audited or self-certified? (Our
   posture: SECURITY.md, zero phone-home, signed images + SBOM, IP hashing.)
4. **Load-test evidence (A9):** we built a load lane that can drive your official tools
   (WebRTC Load Test Tool, hls_players.sh) and asserts our numbers stay correct under
   load; what evidence format / thresholds do you want?
5. **AMS version support requirement (A7):** we live-validate 3.0.3 (current stable);
   older versions are mock-verified only — acceptable?
6. **Trial mechanics (A6)** and **listing category + new-panel marketplace integration
   (A10)**.

### 5. Business terms (operator-led) — 10 min

- **Revenue:** first-year 100%/no-commission is publicly stated — confirm, and get
  **post-year-1 terms in writing**. (Do not anchor on the old internal 20–30% figure.)
- **API stability:** request an API-stability / deprecation-notice commitment in the
  vendor agreement (ties to Q-i/Q-ii — our integration is 100% REST v2).
- **Exclusivity / minimum-AMS-version constraints:** any?
- **Native-analytics overlap:** the new panel ships live charts (public repo). Raise the
  first-party-partner / soft non-compete conversation: Pulse's value is alerting, viewer
  QoE, long-horizon analytics, reports, probes, anomalies — complementary to panel
  charts. Explore positioning as the recommended analytics partner.
- **Co-marketing:** blog post + newsletter (40k+) + joint webinar at listing time
  (their stated promotion channels).

### 6. Close (5 min)

Agree: who receives the submission package link, the next review step, and the target
listing date. Log every answer in `docs/operator-expected.md` and close the A1–A10
rows in [`submission-process.md`](submission-process.md).

---

## One-page product brief (shareable)

**Pulse** — self-hosted analytics, QoE monitoring and alerting for Ant Media Server.
Installs next to AMS (Docker Compose, ~15 min); integration is **read-only REST v2**
polling plus optional webhook/Kafka — Pulse never modifies AMS and customer data never
leaves the customer's infrastructure (no SaaS backend, zero phone-home). A 3.52 KB MIT
player beacon adds real viewer QoE (startup time, rebuffering, bitrate, errors) that no
server-side view can capture. Live ops dashboard, 13-month historical analytics,
alerting to email/Slack/Telegram/PagerDuty/webhook, usage/billing reports (scheduled
CSV/PDF), cluster fleet view, synthetic HLS/DASH/WebRTC/RTMP probes, statistical anomaly
detection, Prometheus endpoint. Validated live against AMS 3.0.3 Enterprise (46/50
scenarios); tested version matrix back to 2.10 (mock profiles). Four tiers
(Free/Pro/Business/Enterprise), ed25519-signed license keys, graceful expiry.
Docs: overview, install, user guide, admin guide, API reference, compatibility matrix,
known limitations, security policy — all in the submission package.
