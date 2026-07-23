# Pulse — Operator FAQ

**Product:** Pulse v0.4.0 · **Last updated:** 2026-07-22 (D-161)

Short answers to the questions operators ask most often.
Each answer links to the canonical doc for deeper reading.

---

## General

### Q1. Does Pulse modify my Ant Media Server?

No. Pulse is read-only. It polls the AMS REST v2 API on a configurable
interval (default 5 s) and never writes to AMS, never changes AMS
configuration, and never touches the AMS filesystem or database. It
survives AMS upgrades without any coordination.

> Canonical source: `docs/product.md` §1 ("Read-only and upgrade-tolerant");
> `docs/AMS-INTEGRATION.md` §1.1 (full list of polled endpoints).

---

### Q2. Which AMS versions are supported?

| AMS Version | Support level |
|---|---|
| **3.0.3 Enterprise Edition (build 20260504\_1443)** | **Live-validated — primary target** (46/50 scenario scripts PASS, S17–S18) |
| 3.0.2, 2.14.x, 2.10.0 | Mock-profile only — compatible in CI, not tested against a live server |

Deploy AMS 3.x. Real Docker images for older versions are unavailable on
Docker Hub (all tags return 404 as of 2026-07-13), so those older rows are
CI-only and carry no live-wire guarantee.

> Full matrix and per-version field notes: `docs/compatibility.md`.

---

### Q3. How much load does Pulse add to AMS?

Pulse issues read-only REST GET requests on a 5 s cycle
(`PULSE_POLL_INTERVAL`, tunable). At 500 concurrent streams (CI-verified
with mock-ams) Pulse uses 18.6 MiB peak memory on its own host; the load it
places on an AMS REST endpoint is bounded by the
number of AMS applications and streams, not by viewer count. The validated
capacity number on a live AMS under load is **pending** — the load lane
(`qa/realams/load/`) requires a dedicated AMS instance and has not yet been
run.

Reducing polling scope: set `PULSE_AMS_APPLICATIONS` to a comma-separated
list of app names to poll only specific apps.

> Load lane specification: `docs/testing/full-e2e-validation-run.md` §Rev2;
> capacity target: `docs/ARCHITECTURE.md` §4 (N = 500 concurrent streams).

---

### Q4. Can I run Pulse on Kubernetes?

A Helm chart is included (`deploy/helm/pulse/`) and has passed `helm lint`
and golden-file template tests, but it is marked **experimental** until a
real cluster install is validated (`helm install` / `helm upgrade` have not
been run against a live cluster). Do not use the Helm path in production
without first validating on a clean cluster.

The Docker Compose path (`deploy/docker-compose.yml` with overlays) is the
supported production path.

> Helm chart values and HA notes: `deploy/helm/pulse/README.md`;
> install paths compared: `docs/runbooks/install.md` (Path C).

---

### Q5. Is there a one-command install?

Yes. Path A0 is a curl-pipe-bash quickstart:

```sh
curl -fsSL https://raw.githubusercontent.com/aytekXR/ams-pulse/main/deploy/quickstart/install.sh \
  | bash -s -- \
      --ams-url http://YOUR_AMS_HOST:5080 \
      --email your-ams-admin@example.com \
      --password your-ams-password
```

The script handles Docker preflight, `.env` writing, stack start,
healthcheck polling, and bootstrap-token extraction. Append
`--license-key <key>` to activate a paid tier on first boot.

> Full walkthrough (including the 3-command manual variant):
> `docs/runbooks/install.md` §Path A0.

---

## Installation

### Q6. Why does `docker pull ghcr.io/aytekxr/ams-pulse` fail?

The GHCR image is currently **private**. Until the package is flipped to
public, authenticate first with a GitHub PAT that has the `read:packages`
scope:

```sh
docker login ghcr.io
```

This requirement will be removed once the package visibility is set to
public.

Also note: image tags have **no `v` prefix** — the git tag `v0.4.0`
publishes as image tag `0.4.0`, not `v0.4.0`.

> `docs/runbooks/install.md` §Path A0 (GHCR visibility note).

---

### Q7. Why does Pulse get HTTP 403 from AMS?

This is the number-one install issue. AMS controls REST API access
**per-application** via the `remoteAllowedCIDR` setting. When an app is set
to `remoteAllowedCIDR=127.0.0.1` (the AMS default), Pulse's container IP
is not in the allowed list and AMS returns 403. Pulse logs a warning each
poll cycle and silently excludes that app from monitoring.

**Fix:** In the AMS Management Console, for each application Pulse should
monitor, add the Pulse container's IP (or its subnet) to
`remoteAllowedCIDR`.

> `docs/known-limitations.md` LIM-14; `docs/AMS-INTEGRATION.md` §10
> (troubleshooting).

---

### Q8. How do I create Pulse user accounts?

User management has no web UI yet (LIM-25). The Settings → Users page
shows "coming in a future update." Creating, updating, and deleting users
works today via the API:

```
GET  /api/v1/admin/users
POST /api/v1/admin/users
PUT  /api/v1/admin/users/{userId}
DELETE /api/v1/admin/users/{userId}
```

An admin-scoped API token is required; every change is recorded in the
audit log. SSO/OIDC user provisioning (Enterprise) is unaffected — first-
login auto-provisioning works end to end. As a workaround, manage access
via API tokens (Settings → API Tokens, which has a full UI).

> `docs/known-limitations.md` LIM-25; `docs/api-guide.md` (full API
> reference).

---

## Licensing and tiers

### Q9. What happens when my license expires?

Pulse degrades gracefully to Free tier. The server keeps running; you can
still read all already-collected data. Tier-gated features return
`403 LICENSE_REQUIRED`. A banner appears in the UI indicating the license
state. Nothing crashes.

To renew, activate a new key via any of three routes:

1. `PULSE_LICENSE_KEY=<key>` in `.env` and restart.
2. `PULSE_LICENSE_FILE=/path/to/license.key` and restart.
3. `PUT /api/v1/admin/license {"key":"<key>"}` — takes effect immediately,
   no restart required.

> `docs/guides/license-activation.md`; `docs/licensing.md` §2.4.

---

### Q10. What does each tier include?

| Limit / feature | Free | Pro | Business | Enterprise |
|---|---|---|---|---|
| AMS source nodes | 1 | **10** | **5** | Unlimited |
| Data retention | 7 days | 90 days | 13 months | Unlimited |
| Beacon QoE ingest (F3) | No | Yes | Yes | Yes |
| Data API + Prometheus `/metrics` (F8) | No | API only | Yes | Yes |
| Usage/billing reports (F6) | No | No | Yes | Yes |
| Anomaly detection (F9) | No | No | No | Yes |
| SSO / OIDC | No | No | No | Yes |
| White-label PDF | No | No | No | Yes |
| Notification channels | Email | Email, Slack, Telegram | + PagerDuty, Webhook | All |

**Note on node limits:** Pro allows more monitored nodes (10) than Business (5). This is intentional pricing: Pro is optimised for a single operator running many AMS edge nodes, while Business adds multi-tenant billing, Prometheus, and scheduled reports for fewer high-value nodes. Check which features matter most for your deployment before choosing.

> `docs/runbooks/install.md` §Free tier limits; `docs/product.md` §1 (feature table).

---

## Data and privacy

### Q11. Does my data leave my infrastructure?

No. Pulse is entirely self-hosted. There is no SaaS component and no
phone-home. License verification is offline — keys are validated locally
via an ed25519 signature check against the `PULSE_LICENSE_PUBKEY` you
deploy; no activation server is contacted.

Viewer IPs from the beacon SDK are **SHA-256 hashed** before storage in
ClickHouse — no raw IP is written to the database (verified:
`normalize.go:281`, assessment TC-15 PASS). For additional GDPR/KVKK
posture, set `PULSE_ANONYMIZE_IP=true` to also zero the last IPv4 octet
(last 80 bits for IPv6) before geo lookup.

Geo-country enrichment is opt-in and uses only an **operator-supplied**
MaxMind GeoLite2 mmdb file (`PULSE_GEO_MMDB_PATH`). No lookup requests
are made to MaxMind or any external service.

> `docs/product.md` §1 ("Data never leaves the customer");
> `docs/licensing.md` §2.4; `docs/known-limitations.md` LIM-05 (GeoIP
> setup); `docs/assessment/final-assessment.md` §1 TC-15.

---

### Q12. Where do viewer QoE numbers come from?

Viewer-side QoE metrics (startup time, stall/rebuffer ratio, bitrate,
error rate) come from the **Pulse Beacon JS SDK** embedded in your player.
The SDK (`sdk/beacon-js`, 3.52 KB gzip, MIT license) posts events to
`/ingest/beacon` on your Pulse instance. No data is sent to any third
party.

Beacon ingest is gated to **Pro tier and above**. Without a Pro+ license,
`/ingest/beacon` returns `403 LICENSE_REQUIRED`.

QoE summary data is available at `GET /api/v1/qoe/summary`. Server-side
metrics (bitrate, viewer count, packet loss) are always available from REST
polling regardless of beacon deployment.

> `docs/beacon-sdk.md`; `docs/AMS-INTEGRATION.md` §1.4.

---

## Features

### Q13. How do I get a PDF report?

There are two separate paths — do not confuse them:

- **Scheduled PDF reports** (Business+ tier): a report schedule with
  `format: pdf` generates a PDF statement each run, with the logo set by
  `PULSE_REPORT_LOGO_PATH`. A white-label header (your company name and
  address) additionally requires an Enterprise license with the
  `white_label` claim.
- **Interactive on-demand export**: only CSV is available
  (`GET /api/v1/reports/export?format=csv`). Requesting `format=pdf` there
  returns `501 NOT_IMPLEMENTED`; the "Export PDF" button has been removed
  (LIM-24).

Workaround for a one-off PDF: create a schedule for the needed period,
download the PDF, then delete the schedule. Or use CSV in a spreadsheet,
or use your browser's Print → Save as PDF. The CSV export is
formula-injection-safe (LIM-24 note).

> `docs/known-limitations.md` LIM-24; `docs/runbooks/reports.md`.

---

### Q14. Do alerts auto-clear when a condition resolves?

It depends on the rule type:

- **"Stream offline" rules**: yes, fully auto-clear. Absence of the stream
  is the trigger; when the stream reappears, Pulse sends a resolved
  notification automatically. This works correctly (D-157/D-159).
- **Value-threshold rules** (bitrate, viewer count, CPU, QoE, etc.): yes,
  they auto-resolve when the metric returns below/above the threshold. The
  rule state machine transitions `firing → resolved` and sends a resolved
  notification.
- **Edge case — LIM-26**: if a non-"stream offline" alert is **currently
  firing** and the source it watches (stream, node) then **vanishes
  entirely** from monitoring (deleted, renamed, decommissioned), the alert
  stays in the `firing` state — there is no automatic resolution when a
  source disappears mid-fire. This occurs only when a source disappears
  while already firing; routine condition clearing is unaffected.

  Workaround: disable or delete the rule to clear the stuck alert, or wait
  for the source to reappear and recover normally.

> `docs/known-limitations.md` LIM-26; `docs/runbooks/alerting.md`
> §Rule semantics.

---

### Q15. Can I scrape Pulse metrics with Prometheus?

Yes. Pulse exposes `GET /metrics` in the Prometheus text exposition format.
The endpoint is gated to **Business tier and above** (Pro and Free receive
`403 LICENSE_REQUIRED`).

Available metrics: `pulse_live_viewers`, `pulse_live_streams`,
`pulse_live_publishers`, `pulse_ingest_bitrate_kbps`,
`pulse_node_cpu_pct{node=...}`, `pulse_node_mem_pct{node=...}`,
`pulse_alerts_firing`.

Set `PULSE_METRICS_TOKEN` to require a Bearer token for scrapes (recommended
in any deployment where the port is not fully private).

> Full scrape config, PromQL examples, and Grafana starter JSON:
> `docs/guides/prometheus.md`.

---

### Q16. Does Pulse support SSO / OIDC?

Yes, on **Enterprise tier**. OIDC-based SSO is implemented and ships with:

- Auto-provisioning of users on first SSO login.
- Role mapping from IdP groups to Pulse roles (`admin`, `viewer`).
- A "Sign in with SSO" button on the login screen when OIDC is configured.
- Sign-out that revokes the SSO session.

Configure via `PULSE_OIDC_CLIENT_SECRET` and the OIDC provider settings.
Note: the per-user `role` stored in the Pulse database is advisory only —
OIDC re-maps the role from IdP groups on every login and never reads the
stored value. Password login does not exist; SSO is the sole human login
path when OIDC is enabled.

> `docs/product.md` §2 (roadmap note); `docs/operator-expected.md` §[10]
> (team-management model discussion).

---

## Operations

### Q17. Is multi-tenant deployment supported?

Partially. The F6 multi-tenancy **API** (tenant-scoped usage reports,
billing statements, per-tenant stream ownership) is code-complete and
available on Business+ tier. A tenant-management UI and server-side
per-tenant AUTH are demand-driven and have not been built yet; they are
deferred unless a multi-tenant customer is imminent.

For now, operators running multi-tenant deployments can use the API
directly to generate per-tenant reports and manage tenant records.

> `docs/ARCHITECTURE.md` §Tier model.

---

### Q18. How do I back up and restore Pulse data?

Pulse has a backup sidecar that runs every 24 hours automatically. Enable
it by adding `deploy/docker-compose.backup.yml` to your compose overlay
chain. It backs up:

- **ClickHouse** (all metrics, rollups, viewer sessions) — ZIP archives
  retained for 7 cycles.
- **SQLite meta store** (alert rules, API tokens, users, license, probe
  config) — db file + WAL retained for 7 cycles.

Restore is done with `RESTORE DATABASE pulse FROM Disk('backups', '...')`
for ClickHouse and a file copy for the meta store. Full step-by-step
restore procedure including the required WAL-clearing step:

> `deploy/runbooks/backup-restore.md` (ClickHouse and SQLite restore;
> S3 off-site upload; monitoring backup health).

---

### Q19. How do I upgrade Pulse?

1. Stop Pulse: `docker compose stop pulse`
2. Update the image tag in your compose override (or pull the new binary).
3. Run `pulse migrate` to apply any new ClickHouse DDL.
4. Start Pulse. Meta-store migrations apply automatically on startup.

ClickHouse DDL migrations are append-only; no destructive changes are made.
The meta store schema is backwards-compatible within a major version.

> `deploy/runbooks/upgrade-rollback.md` (full overlay command, stamped-build
> pattern, rollback procedure).

---

*Every answer above is traceable to a primary source cited inline.
For the full list of known operator-facing limitations see
`docs/known-limitations.md` (26 LIMs as of D-161).*
