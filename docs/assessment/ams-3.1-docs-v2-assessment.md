# AMS 3.1.0 + Documentation v2 — do AMS operators still need Pulse?

*Assessment date: 2026-09-02 (S124 / D-192). Immutable snapshot — records what the sources said
on this date; supersedes nothing and is superseded by re-running the method below.*

**Sources.** All **289 current-version pages** of the new `https://docs.antmedia.io/v2/`
documentation site (mirrored to plain text and swept exhaustively — a completeness critic
confirmed zero uncovered files), the **`ams-v3.1.0` release notes** (published 2026-08-31, 127
resolved issues), and AMS source at tag `ams-v3.1.0` for the wire-format fields Pulse consumes.

**Method.** 34-agent workflow: 9 parallel doc-slice sweeps (110 relevance findings) + a Pulse
feature/positioning map + a release-notes read → one overlap verdict per PRD feature → an
independent **adversarial verification of every verdict** (each evidence quote re-opened in the
source page; every "no overlap" claim re-searched from scratch with fresh terms). All 11
verdicts survived verification; 3 received minor corrections that did not change the verdict.

---

## Verdict

**Yes — an AMS operator still needs Pulse.** What Ant Media shipped is a documentation
re-platform (Docusaurus, versions 2.16 / 2.17 / 3.0 / Next) and a management-panel rebuild —
**not** an analytics product. Docs v2 actually *sharpens* Pulse's case: it commits to writing,
in Ant Media's own voice, that observability is the operator's problem to assemble from
third-party parts.

The single strongest sentence in the entire corpus is in their enterprise production checklist
(`/v2/enterprise-guide/`):

> "Monitoring with alerting is set up (Grafana, Prometheus/Loki, or New Relic)"

— i.e. even Ant Media's own definition of a production-ready AMS deployment requires an
external monitoring stack. Pulse *is* that checkbox, as one container instead of four-to-six.

## Per-feature verdicts (adversarially verified)

| Pulse feature | Overlap | Threat | Closest thing AMS offers (docs v2 + 3.1.0) |
|---|---|---|---|
| F1 Real-time ops dashboard | partial | **medium** | New 3.1.0 React panel: live CPU/mem/disk/JVM + per-stream viewer counts by protocol. No history, no multi-node aggregation, no network throughput |
| F2 Historical audience analytics | partial | low | Analytics JSON log (v2.10+) + live count REST endpoints. No storage, no query engine, no uniques, no geo — the webhooks page literally lists "viewer analytics" as a build-it-yourself use case |
| F3 Player QoE beacon SDK | partial | low | WebRTC-only client `getStats`/`enableStats` in the JS SDK. No rebuffer/startup-time tracking, no HLS coverage, no collection pipeline |
| F4 Publisher/ingest health | partial | low | `publishTimeoutError` webhook — binary, fires only after frames stop entirely. No health score, no server-side publisher loss/jitter |
| F5 Alerting + incidents | partial | low | Raw failure webhooks + a fixed 75%-CPU WebSocket warning. No channels, no rule engine, no history, no maintenance windows |
| F6 Usage/billing reports | **none** | none | Raw `watchTimeMs`/bytes in log lines; zero aggregation or export |
| F7 Cluster fleet view | partial | low | EE panel Cluster tab lists nodes; dashboard shows the *current node only*; no node-down alerting |
| F8 Data API / Prometheus / exports | partial | low | Scattered REST stats + optional Kafka producer. **Still no native `/metrics`** — issue #3122 remains unbuilt in 3.1.0 |
| F9 Anomaly detection | **none** | none | Fixed thresholds in `red5.properties` only; zero baseline/deviation concepts anywhere in 289 pages |
| F10 Synthetic viewer probes | **none** | none | Manual load-test tools; no periodic playback verification, no TTFB measurement |
| Cross-cutting positioning claims | — | low | All 8 load-bearing "AMS natively lacks X" claims re-verified intact (below) |

Every "partial" is the same shape: **AMS emits raw data; Pulse is the product layer on top**
(storage, correlation, alerting, visualization). That is a data-source relationship, not a
competitor relationship.

## What the docs prescribe for monitoring — all four paths are external

| Documented path | What the operator must run | Notes |
|---|---|---|
| Grafana on VMs (`/v2/guides/monitoring/monitoring-ams-with-grafana/`) | Kafka + ZooKeeper + Logstash + Elasticsearch + Grafana — a dedicated ≥4 GB server, Java 11 | The classic 5-component DIY stack, unchanged in substance from the old docs |
| Kubernetes (`…/loki-prometheus-setup/`) | kube-prometheus-stack + Loki + Promtail via Helm | Prometheus scrapes *cluster/pod* metrics, not AMS stream stats — this is not an AMS exporter |
| New Relic (`…/monitor-ant-media-server-statistics-with-new-relic/`) | New Relic SaaS agent + hand-built parsing rules over `ant-media-server-analytics.log` | Data leaves the operator's infrastructure |
| Centralized logging (`…/centralized-logging/`) | Fluent Bit → `log.antmedia.io:80` | **For Ant Media's own support team** — multi-tenant vendor platform, not operator-facing analytics; ships over plain HTTP port 80 |

## Positioning claims re-tested — all standing

- **No native Prometheus exporter** — confirmed; #3122 stays unbuilt through 3.1.0; the only
  Prometheus in docs v2 is the operator-run kube-prometheus-stack.
- **Webhooks unsigned** — confirmed; the new `/v2/guides/developer-sdk-and-api/webhooks/` page
  documents every event and no signing/HMAC field of any kind (grep across all 289 pages: zero
  hits). LIM-03 and D-066/O3 stand.
- **No historical retention, no built-in alerting, no client-side QoE, no billing reports,
  no anomaly detection, no synthetic probes** — all confirmed absent.
- **#7926 (server freezes after ~24 h under RTMP load, OS metrics normal)** — NOT in the
  3.1.0 fixed list; still open. Our §2.16 early-warning demand evidence stands.
- Source-verified at tag `ams-v3.1.0`: all 10 `Broadcast` fields Pulse consumes are present
  and identically typed; `currentFPS` still absent (LIM-04 unchanged); `ClusterNode` still has
  no role/version field (LIM-10 applies to 3.1.0 too; one additive `note` field is harmless).

## What 3.1.0 changes for us

**Helps us:** status-accuracy fixes to data Pulse consumes — #7726 (stream offline but reported
active), #2724 (broadcast stuck in `broadcasting`), a missing `play_finished` webhook case.
Cleaner inputs, same integration surface.

**Watch items (honest threats):**
1. **The new panel is the one moving part** (F1 threat = medium). It shipped with a stream
   details drawer and some graphs, and #7911 says the work "enhanc[es] backend APIs to provide
   what we need" — i.e. panel-driven REST drift is possible. Our G-27 9-endpoint pin
   (`docs/compatibility.md`) must be re-validated against a live 3.1.0 (ROADMAP §2.48).
   If Ant Media ever wires the analytics log into that panel with retention, F1/F2 threat
   rises — re-run this assessment at every AMS minor.
2. **Java 21 / Tomcat 10.1.56 / BCrypt panel-auth changes** — no wire impact expected, but
   "expected" is not "verified": §2.48 covers it.
3. **Docs v2 quality** — the DIY guides got genuinely better (step-by-step, troubleshooting
   tables), which lowers the pain of the DIY path slightly. It does not change what must be
   run and maintained.
4. **Marketplace** — "marketplace" in docs v2 means *cloud* marketplaces (AWS/Azure/GCP images)
   exclusively; the plugin/solutions marketplace we are submitting to is referenced only in the
   plugins guide. Our submission channel exists but is not doc-surfaced — expectations about
   listing-driven discovery should stay modest; the Ankush/developer-meeting track remains the
   real path.

## Actions taken from this assessment

- ROADMAP-V2 **§2.48**: AMS 3.1.0 compatibility lane (autonomous via the community image) +
  marketplace-collateral refresh with docs-v2 citations + the per-minor re-assessment trigger.
- `docs/compatibility.md`: 3.1.0 row added (source-verified, not yet live-validated).
- `docs/operator-expected.md`: the docs-feedback form (Ant Media's 2026-09 email; 2–3 min,
  $25 gift card, and a free pre-submission relationship touch) queued as an operator item.

*Found during this session's opening gate, unrelated to the competitive question but urgent:
prod Pulse has been blind since 2026-08-12 (AMS container recreated without published ports)
and `pulse.beyondkaira.com` has no nginx vhost since 2026-08-11 — see `docs/operator-expected.md`
§0 and D-192.*
