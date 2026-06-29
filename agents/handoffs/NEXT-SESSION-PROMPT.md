# Next session — Pulse: integration keys + test/CI hardening

> Paste this to start. Pulse = self-hosted analytics/QoE/alerting for Ant Media Server.
> Repo `/home/aytek/repo/ams-pulse`, branch `ams-integration`, VPS `161.97.172.146`.
> Deep context: `agents/handoffs/RESUME-PROMPT.md` + `decisions.md` (D-001…D-035, binding).

## Verified state (2026-06-28)
- Prod LIVE on **self-hosted AMS Enterprise 3.0.3** (D-034). Subdomains live w/ real TLS:
  `https://pulse.beyondkaira.com` (app), `https://ams.beyondkaira.com` (AMS panel). Apex still serves Pulse.
- Go suite: **EXIT=0, all pass, no races. Total coverage 47.5%** (`go test -race -cover`, repo-root mount).
- `ams-integration` is **NOT merged to main** (main stale — ships old code if deployed).

## PART 1 — Integration keys (provide any subset; agent wires + verifies each on staging first, then prod)
Agent stores in `deploy/.env` (gitignored), wires, and verifies **real** behavior end-to-end. Never commit keys.

| Capability | Provide | Unlocks |
|---|---|---|
| **Pulse license** (Pro+/Business/Ent) | `PULSE_LICENSE_KEY` (or signed file + `PULSE_LICENSE_PUBKEY`) | QoE/beacon ingest (U3), anomalies, data API, probes, reports, Prometheus, multi-tenant — today gated to Free |
| **Email alerts** | SMTP host/port/user/pass (or SES/SendGrid key) | email alert delivery |
| **Slack alerts** | Slack incoming-webhook URL | Slack alert delivery |
| **PagerDuty** | routing/integration key | PagerDuty alert delivery |
| **Telegram** | bot token + chat id | Telegram alert delivery |
| **Generic webhook** | target URL + shared secret | webhook alert delivery |
| **S3 report export** | `PULSE_S3_ACCESS_KEY_ID`/`_SECRET_ACCESS_KEY`/`_BUCKET`/`_REGION`(/`_ENDPOINT`) | CSV/PDF report storage |
| **Geo enrichment** | MaxMind license key → GeoLite2-City.mmdb (`PULSE_GEO_MMDB_PATH`) | viewer country/region |
| **Prometheus** | `PULSE_METRICS_TOKEN` (self-generate) | authed `/metrics` |

Implemented alert channels: **email, slack, pagerduty, telegram, webhook**.
⚠️ Wire these alongside fixing the **stubs the keys would otherwise hide**: alert **test-fire is a no-op** (returns 202, never calls `Send()`); **3 license gates defined-but-UNENFORCED** (`CheckDataAPI`, `CheckNodeLimit`, `CheckPrometheus`). TDD each.

## PART 2 — Test & CI hardening (so breakage is caught in CI) — orchestrate as workflows, TDD red→green
Baseline coverage (2026-06-28): total **47.5%**, all pass.

**ZERO unit coverage (write tests first):**
- `internal/query` **0%** — powers every dashboard chart + API read (highest blast radius). Unit-test with a mock Conn.
- `internal/config` **0%** — env parsing / startup correctness. Every var + bad-input failure paths.
- `internal/store/clickhouse` **0% unit** (integration covers only ~3/12 query methods) + `.../migrations` **0%**.
- `cmd/pulse` **1.2%** — serve/migrate/diag wiring.

**LOW + critical:** `internal/license` **36.9%** (billing/tier gates = revenue), `store/meta` **29.7%**, `collector/logtail` **37.5%**, `internal/api` **52.2%**, `alert/channels` **56.8%**.
**STRONG (keep ratcheting):** collector/ingest 85%, cluster 89%, sessions 81%, anomaly 76%, amsclient 76%, restpoller 72%, alert 72%.

**Priority (critical-business-logic-first):**
1. `license` 37→≥85 **and ENFORCE** the 3 gates + alert test-fire real `Send()`.
2. `query` 0→≥70 (mock-Conn unit) — analytics behind every chart.
3. alert firing→delivery (`channels` 57→≥80) + **retry** + alert→history e2e. **[VERIFIED 2026-06-29 — real gap]** Unmuting the `Stream offline` default rule + stopping the zombi RTMP test stream produced **NO** history entry in 130s: `evalStreamOffline` reads the live snapshot and a vanished stream isn't in it. To *demonstrate* a visible alert (operator's bar) use a snapshot-present metric (e.g. `ingest_bitrate_floor` with threshold above the live bitrate) or a tracked/registered stream — and the firing→history path itself has no e2e. Fix + test this FIRST.
4. `config` 0→≥80 — all env vars + failure paths.
5. `store/clickhouse` + `meta` — unit + expand integration to all query methods.
6. AMS wire **fixture-replay regression** pinning D-029/D-031 (bps→kbps, FPS-redistribution, `terminated_unexpectedly`, WebRTC single-track).

**CI gaps to close (`.github/workflows`) — the "see breakage in CI" asks:**
- **ADD a coverage gate** — fail the build if total < floor OR any package regresses (ratchet). *(the #1 request)*
- **ADD Playwright browser e2e** (`web/e2e/`, NEW — none today): SPA renders, auth redirect, CSP enforced, large-table virtualization, zero console errors.
- **ADD response-body contract tests** (kin-openapi) in `internal/api`: assert real responses conform to `contracts/openapi/pulse-api.yaml` (CI only lints the spec today, never the responses).
- **ADD web coverage threshold** (`vitest --coverage` gate).
- **DEEPEN `e2e.yml`**: assert alert fires→delivered, beacon→QoE (after license), real-AMS fixture replay (today only checks overview activity>0 vs mock-ams).

## PART 3 — also pending (RESUME-PROMPT backlog)
Merge `ams-integration`→`main`; Caddy `/webhook/*` route (AMS lifecycle webhooks 404 today); alert retry; backups; container resource limits; CH drain-on-close; branch protection + `v*` tag (needs `gh`).

## Binding flow (every change)
TDD red→green→refactor; **verify on an isolated staging project before prod**; commit by **EXPLICIT path** (ORCH commits centrally, agents author); update RESUME-PROMPT + decisions (new D-0NN); **never commit `deploy/.env` / `oguz-testing.md`**. Run `go test` only in `golang:1.25` with the **REPO-ROOT mount** (D-028). Coverage gate command: RESUME-PROMPT §5.
