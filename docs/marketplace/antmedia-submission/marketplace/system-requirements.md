# Pulse — System Requirements

## Pulse host (Docker Compose — the supported path)

| | Minimum | Notes |
|---|---|---|
| OS | 64-bit Linux | Images published for `linux/amd64` and `linux/arm64`. |
| Container runtime | Docker Engine 24+ with Docker Compose v2 | The quickstart checks both and explains what is missing. |
| CPU / RAM | 2 vCPU / 2 GB | 4 GB recommended above ~100 concurrent streams. |
| Disk | Grows with retention and traffic | ClickHouse stores events compressed; size the volume for your `PULSE_RETENTION_DAYS` (default 90 days raw, 13 months of rollups). |
| Network in | TCP 8090 (UI + API + beacons) | 8091 / 8092 only if you enable them. Put TLS in front for anything public. |
| Network out | AMS REST API (5080/5443); `ghcr.io` for the image (or side-load it); your notification targets | No vendor endpoint — Pulse never phones home. |

**Capacity.** No formal load-test result has been published yet. The project's provisional
guidance is that a single 2 vCPU / 2 GB instance handles the polling and beacon load of a
small-to-mid AMS deployment; the load-test harness is ready and waits for a dedicated AMS
instance (see `docs/compatibility.md`). Do not quote a viewer or stream ceiling until it has
run.

## Ant Media Server

| | |
|---|---|
| Live-validated | **AMS 3.1.0 Enterprise** (2026-10-07, with Pulse v0.5.0) — alert delivery, player QoE and the installer verified end to end; 44 of 61 scenarios passing, and none of the others is an AMS incompatibility (outdated test expectations, the test host's CPU limit, and one Pulse UI defect fixed in v0.5.0). **AMS 3.0.3 Enterprise** — 46 of 50 scenarios passing, remaining 4 documented. |
| Best effort | AMS 2.10–2.17 (wire-format profiles tested in CI; not live-validated). Versions before 2.10 are not supported. |
| Edition | Validated on Enterprise Edition. Community Edition has not been validated. |
| Access Pulse needs | REST API reachable from the Pulse host; an AMS admin login (or a JWT bearer token); Pulse's IP allowed in each monitored application's REST IP filter. |
| Changes to AMS | No software change — no plugin, JAR or WAR, no restart. The only setting is the REST IP filter above. Pulse only reads. |
| Clusters | Nodes are discovered from the cluster API; AMS 3.x does not expose node roles, so all nodes show as origin (multi-node operation not yet validated live). |

## Kubernetes (experimental)

Kubernetes 1.25+, Helm 3.12+. The chart has been rendered and linted but not yet deployed to a
real cluster — see the installation guide §6.

## Browsers (dashboard)

Current Chrome, Edge, Firefox or Safari. The UI loads no external resources (no CDN, no web
fonts from third parties, no analytics), so it works on isolated networks.

## Player SDK (viewer QoE, optional)

Any page that can load a 3.5 KB ES module; integrations for hls.js, the HTML `<video>` element
(including video.js) and the AMS WebRTC player. Viewers' browsers must be able to reach the
beacon endpoint over HTTPS.
