# Release notes DRAFT — GA release (tag pending: v1.0.0 or v0.2.0)

> **Prepared by SESSION-08 (D-065, 2026-07-09). Do not publish as-is** — the
> operator picks the version, then a session (or the operator) runs:
> `git tag vX.Y.Z && git push origin vX.Y.Z` → release.yml does the rest
> (CI-gated build, Trivy scan, multi-arch, SBOM + provenance, cosign signing).
> After the run: `cosign verify` per the release.yml header, then a prod rollout
> carrying the tag (runbook: `deploy/runbooks/upgrade-rollback.md`).
> Body below is `gh release edit vX.Y.Z --notes-file`-ready after trimming this
> header block.

---

Pulse is self-hosted analytics, QoE monitoring, and alerting for
[Ant Media Server](https://antmedia.io). This is the first
general-availability release, following v0.1.0 (2026-07-08). Full change detail:
`CHANGELOG.md`; decision log: `agents/handoffs/decisions.md` (D-059…D-065).

## Highlights since v0.1.0

- **Honest QoE alerting** — `rebuffer_ratio` / `error_rate` rules now read real
  beacon-fed `rollup_qoe_1h` data instead of a health-score heuristic proxy;
  behavior on missing data is documented 3-case semantics
  (`docs/runbooks/alerting.md`). CI proves the full
  license → beacon → rollup → alert-fires chain on every push.
- **Fixed: alert delivery registry (P0)** — rule→channel delivery never worked
  in production paths since D-041; the evaluator now syncs channels from the
  meta store every tick. Live-verified in prod.
- **Per-source webhook secrets (B7)** — `/webhook/ams/{name}` with cross-source
  isolation and fail-closed 401 semantics; global `/webhook/ams` unchanged.
- **Load-validated** — 500 streams + 3,000 viewers, 15-min soak: pulse 18.6 MiB
  peak, API 9 ms avg, 0 errors (`docs/ARCHITECTURE.md` §4). Poll-boundary CPU
  bursts led to a raised container CPU limit (0.5 → 1.0 vCPU) and log-storm
  aggregation at scale.
- **Test depth** — Go coverage 59.4% → 73.2% (CI floor 70.2); 51/52 OpenAPI
  operations response-body-validated (1 documented WebSocket waiver); Playwright
  browser e2e incl. a byte-exact CSP assertion against a real Caddy stack;
  CodeQL (Go + JS/TS).
- **Operations** — verified backup/restore sidecar, graceful ClickHouse drain,
  upgrade/rollback + monitoring runbooks, SECURITY.md, digest-pinned images
  everywhere, Dependabot.

## Upgrade notes (compose deployments)

1. Follow `deploy/runbooks/upgrade-rollback.md` (tag a rollback image + backup
   first). The meta store gains `ams_sources.webhook_secret_enc` automatically
   at boot (additive; no manual migration).
2. Resource limits changed: pulse CPU cap is now 1.0 vCPU in the hardened
   overlay — `docker inspect pulse-prod-pulse-1` should show
   `cpus=1000000000` after the swap.
3. QoE alerts require a Pro+ Pulse license + beacon ingest; on Free tier they
   evaluate against 0.0 by design (no data), and the beacon endpoint returns
   403 `LICENSE_REQUIRED`.

## Verify the artifacts

Commands (cosign keyless verify, SBOM download) are in the header comment of
`.github/workflows/release.yml`. Images: `ghcr.io/aytekxr/ams-pulse`.

## Known gaps (tracked, non-blocking)

- Helm chart is functional but explicitly **experimental** (`docs/install.md`
  Path C); compose is the supported production path.
- Postgres meta backend, SSO/OIDC, native WebRTC/RTMP/DASH probes, mobile SDKs:
  post-GA backlog.
