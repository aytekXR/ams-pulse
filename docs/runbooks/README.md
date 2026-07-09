# Runbooks

Operational guides written as features land (owner: INFRA-01 with DOC-01):

- `install.md` — the 15-minute marketplace install guide (launch asset, PRD §7.12)
- `upgrade.md` — Pulse upgrades and AMS version-compatibility matrix
- `troubleshooting.md` — diagnostic bundle (`pulse diag`), common source misconfigs
- `sizing.md` — ClickHouse sizing guidance per tier (from §7.10 budgets)
- `air-gapped.md` — offline licensing for Enterprise (Phase 3)

Operational runbooks for the production stack (in `deploy/runbooks/`):

- [`deploy/runbooks/upgrade-rollback.md`](../../deploy/runbooks/upgrade-rollback.md) — upgrade + rollback procedure: 5-overlay compose command, stamped-build pattern, rollback tags, ClickHouse DDL stance
- [`deploy/runbooks/monitoring.md`](../../deploy/runbooks/monitoring.md) — what to watch: backup daemon health, alert_history cap, CH disk, Prometheus metrics, WARN log taxonomy
- [`deploy/runbooks/backup-restore.md`](../../deploy/runbooks/backup-restore.md) — backup sidecar architecture, manual backup, ClickHouse + SQLite restore steps
