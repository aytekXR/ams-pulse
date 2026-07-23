# Runbooks

Operational guides written as features land (owner: INFRA-01 with DOC-01):

- `install.md` — the 15-minute marketplace install guide (launch asset, PRD §7.12), incl. the Upgrading section
- [`alerting.md`](alerting.md) — alert rule semantics, channels, maintenance windows
- [`probes.md`](probes.md) — synthetic probes (Pro+)
- [`reports.md`](reports.md) — usage/billing reports (Business+)
- [`productionize.md`](productionize.md) — the production wiring (host-nginx edge + consolidated compose)

Operator-facing reference docs (in `docs/`): [`../overview.md`](../overview.md) ·
[`../user-guide.md`](../user-guide.md) · [`../admin-guide.md`](../admin-guide.md) ·
[`../api-guide.md`](../api-guide.md) · [`../faq.md`](../faq.md) ·
[`../troubleshooting.md`](../troubleshooting.md) · [`../compatibility.md`](../compatibility.md) ·
[`../known-limitations.md`](../known-limitations.md) · [`../support.md`](../support.md) ·
[`../licensing-public.md`](../licensing-public.md)

Operational runbooks for the production stack (in `deploy/runbooks/`):

- [`deploy/runbooks/upgrade-rollback.md`](../../deploy/runbooks/upgrade-rollback.md) — upgrade + rollback procedure: canonical 3-file compose command, stamped-build pattern, rollback tags, ClickHouse DDL stance
- [`deploy/runbooks/monitoring.md`](../../deploy/runbooks/monitoring.md) — what to watch: backup daemon health, alert_history cap, CH disk, Prometheus metrics, WARN log taxonomy
- [`deploy/runbooks/backup-restore.md`](../../deploy/runbooks/backup-restore.md) — backup sidecar architecture, manual backup, ClickHouse + SQLite restore steps
