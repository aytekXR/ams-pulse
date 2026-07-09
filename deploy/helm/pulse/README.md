# Pulse Helm Chart

Self-hosted analytics, QoE monitoring and alerting for Ant Media Server.
Kubernetes deployment for clustered AMS installs (PRD §7.10, Phase 2).

## Architecture decision: bundled ClickHouse StatefulSet

This chart bundles a single-replica ClickHouse StatefulSet (no Bitnami sub-chart
dependency). See `Chart.yaml` for the full rationale. To use an external ClickHouse
set `clickhouse.enabled=false` and provide `clickhouse.externalDSN`.

## Security posture

- **One internet-facing Service**: `pulse-ingest` (beacon QoE endpoint, port 8091).
  Set `ingestService.type=LoadBalancer` and configure TLS termination at the load
  balancer or enable `ingressIngest`. Never expose HTTP directly.
- **All secrets via secretRef**: AMS token, encryption key, webhook secret, Postgres
  DSN, and S3 credentials are never stored in `values.yaml`. Create a Kubernetes
  Secret and set `pulse.secretRef.name`.
- **PULSE_SECRET_KEY is required** (`optional: false`): missing this key causes the
  pod to fail scheduling loudly rather than crashing at runtime.
- **ClickHouse auth**: set `clickhouse.auth.existingSecret` to a Secret containing
  `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`, and `PULSE_CLICKHOUSE_DSN`. Without this,
  the ClickHouse 'default' user has no password — acceptable only for isolated
  dev/test clusters protected by NetworkPolicy.
- **Digest pinning**: set `pulse.image.digest` to the manifest digest for immutable
  production pulls (see values.yaml for how to obtain the digest).
- Pulse runs as UID 1000, non-root, `allowPrivilegeEscalation: false`.

## Prerequisites

- Kubernetes 1.25+
- Helm 3.12+
- A Kubernetes Secret with sensitive values (see **Secrets** below)

## Quick install

```bash
# 1. Create the pulse secrets (PULSE_SECRET_KEY is required).
kubectl create secret generic pulse-secrets \
  --from-literal=PULSE_AMS_AUTH_TOKEN=<your-ams-token> \
  --from-literal=PULSE_SECRET_KEY=$(openssl rand -hex 32)

# 1b. Create the ClickHouse auth secret (required for production).
CH_PASS=$(openssl rand -hex 16)
kubectl create secret generic pulse-clickhouse-secret \
  --from-literal=CLICKHOUSE_USER=pulse \
  --from-literal=CLICKHOUSE_PASSWORD="${CH_PASS}" \
  --from-literal=PULSE_CLICKHOUSE_DSN="clickhouse://pulse:${CH_PASS}@pulse-clickhouse:9000/pulse"

# 2. Install the chart (default: SQLite meta, bundled ClickHouse).
helm install pulse ./deploy/helm/pulse \
  --set pulse.ams.url=http://your-ams:5080 \
  --set pulse.ams.nodeId=node-01 \
  --set pulse.secretRef.name=pulse-secrets \
  --set clickhouse.auth.existingSecret=pulse-clickhouse-secret

# 3. Apply migrations (ClickHouse DDL).
kubectl exec deploy/pulse -- pulse migrate
```

## Upgrade

```bash
helm upgrade pulse ./deploy/helm/pulse -f my-values.yaml
```

## Values table

| Key | Default | Description |
|-----|---------|-------------|
| `pulse.image.repository` | `ghcr.io/aytekxr/ams-pulse` | Canonical GHCR image (cosign-signed on release) |
| `pulse.image.tag` | `0.1.0` | Image tag |
| `pulse.image.digest` | `""` | Manifest digest for immutable pinning (overrides tag when set) |
| `pulse.replicaCount` | `1` | Pulse replicas (use 1 with SQLite; N with postgres.enabled) |
| `pulse.resources.requests.cpu` | `250m` | CPU request (2-vCPU tier) |
| `pulse.resources.requests.memory` | `256Mi` | Memory request |
| `pulse.resources.limits.cpu` | `500m` | CPU limit |
| `pulse.resources.limits.memory` | `512Mi` | Memory limit |
| `pulse.listenAddr` | `:8090` | `PULSE_LISTEN_ADDR` — API + UI port |
| `pulse.ingestListenAddr` | `:8091` | Beacon ingest port (matches `ingestService.port`) |
| `pulse.ams.url` | `http://localhost:5080` | `PULSE_AMS_URL` — AMS REST base URL |
| `pulse.ams.nodeId` | `standalone` | `PULSE_AMS_NODE_ID` — node identifier in events |
| `pulse.ams.applications` | `""` | `PULSE_AMS_APPLICATIONS` — comma-separated app filter (empty = all) |
| `pulse.ams.pollInterval` | `5s` | `PULSE_POLL_INTERVAL` — AMS REST poll cadence |
| `pulse.clickhouse.database` | `pulse` | `PULSE_CLICKHOUSE_DATABASE` |
| `pulse.clickhouse.migrationsDir` | `""` | `PULSE_MIGRATIONS_DIR` (empty = embedded) |
| `pulse.retentionDays` | `90` | `PULSE_RETENTION_DAYS` — raw event TTL |
| `pulse.rollupTTLDays` | `395` | `PULSE_ROLLUP_TTL_DAYS` — rollup TTL (~13 months) |
| `pulse.meta.dsn` | `""` | `PULSE_META_DSN` (auto: `/var/lib/pulse/pulse_meta.db`) |
| `pulse.webhookAddr` | `""` | `PULSE_WEBHOOK_ADDR` — webhook HTTP listener (e.g. `:8092`) |
| `pulse.license.key` | `""` | `PULSE_LICENSE_KEY` (empty = Free tier) |
| `pulse.license.offlineFile` | `""` | `PULSE_LICENSE_FILE` (air-gapped Enterprise) |
| `pulse.logLevel` | `info` | `PULSE_LOG_LEVEL` (debug\|info\|warn\|error) |
| `pulse.secretRef.name` | `""` | Name of Secret providing sensitive env vars |
| `pulse.persistence.enabled` | `true` | PVC for SQLite meta store |
| `pulse.persistence.size` | `2Gi` | PVC size |
| `clickhouse.enabled` | `true` | Bundle ClickHouse StatefulSet |
| `clickhouse.externalDSN` | `""` | DSN when `clickhouse.enabled=false` |
| `clickhouse.auth.existingSecret` | `""` | Secret with `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`, `PULSE_CLICKHOUSE_DSN` — **required for production** |
| `clickhouse.image.tag` | `24.8` | ClickHouse image tag |
| `clickhouse.resources.requests.cpu` | `250m` | ClickHouse CPU request |
| `clickhouse.resources.requests.memory` | `512Mi` | ClickHouse memory request |
| `clickhouse.resources.limits.memory` | `1Gi` | ClickHouse memory limit |
| `clickhouse.persistence.size` | `20Gi` | ClickHouse PVC size |
| `clickhouse.config.maxMemoryUsage` | `536870912` | 512 MB ClickHouse server memory cap |
| `postgres.enabled` | `false` | Provision in-cluster Postgres for HA meta store |
| `postgres.auth.database` | `pulse_meta` | Postgres database name |
| `postgres.auth.username` | `pulse` | Postgres username |
| `postgres.persistence.size` | `5Gi` | Postgres PVC size |
| `s3Export.enabled` | `false` | Enable S3 export for CSV/PDF reports (Wave 2) |
| `s3Export.bucket` | `""` | S3 bucket name |
| `s3Export.region` | `us-east-1` | S3 region |
| `mmdb.enabled` | `false` | Mount MaxMind mmdb for geo enrichment (Wave 2) |
| `webhookService.enabled` | `false` | Expose the webhook port as a K8s Service (enable with `pulse.webhookAddr`) |
| `webhookService.type` | `ClusterIP` | Webhook Service type |
| `webhookService.port` | `8092` | Webhook Service port (keep in sync with `pulse.webhookAddr`) |
| `ingressWebhook.enabled` | `false` | Enable Ingress for AMS webhook callbacks |
| `ingestService.enabled` | `true` | Create the beacon ingest Service |
| `ingestService.type` | `ClusterIP` | Service type (ClusterIP\|LoadBalancer\|NodePort) |
| `ingestService.port` | `8091` | Beacon ingest port |
| `ingress.enabled` | `false` | Enable Ingress for API + UI |
| `ingressIngest.enabled` | `false` | Enable Ingress for beacon ingest (internet-facing) |
| `backup.enabled` | `false` | Deploy backup CronJob + wire ClickHouse backup disk |
| `backup.schedule` | `"0 2 * * *"` | Cron schedule (default: 02:00 UTC daily) |
| `backup.image.digest` | *(CH 24.8 digest)* | Digest-pinned ClickHouse image for backup container |
| `backup.persistence.existingClaim` | `""` | Existing backup PVC name (auto-created when empty) |
| `backup.persistence.size` | `10Gi` | Backup PVC size |

## Secrets

### Pulse app secret (`pulse.secretRef.name`)

| Secret key | Required | Binary env var | Description |
|------------|----------|----------------|-------------|
| `PULSE_SECRET_KEY` | **YES** | `PULSE_SECRET_KEY` | 32-byte hex key for AES-256-GCM; binary crashes at boot if absent (`optional: false`) |
| `PULSE_AMS_AUTH_TOKEN` | no | `PULSE_AMS_AUTH_TOKEN` | AMS REST bearer token |
| `PULSE_WEBHOOK_SECRET` | no | `PULSE_WEBHOOK_SECRET` | HMAC-SHA256 global webhook validation secret |
| `PULSE_METRICS_TOKEN` | no | `PULSE_METRICS_TOKEN` | Prometheus scrape token (Wave 2) |
| `PULSE_POSTGRES_DSN` | when `postgres.enabled` | `PULSE_META_DSN` | Full Postgres DSN |
| `PULSE_S3_EXPORT_KEY_ID` | no | `PULSE_S3_EXPORT_KEY_ID` | S3 access key ID (Wave 2) |
| `PULSE_S3_EXPORT_SECRET_KEY` | no | `PULSE_S3_EXPORT_SECRET_KEY` | S3 secret access key (Wave 2) |

### ClickHouse auth secret (`clickhouse.auth.existingSecret`)

Required for production. Set `clickhouse.auth.existingSecret` to a Secret with:

| Secret key | Description |
|------------|-------------|
| `CLICKHOUSE_USER` | Named user the ClickHouse image creates at first boot |
| `CLICKHOUSE_PASSWORD` | Strong random password (min 16 chars recommended) |
| `PULSE_CLICKHOUSE_DSN` | Full authenticated DSN: `clickhouse://USER:PASS@<release>-clickhouse:9000/<database>` |

Create the Secret before install:

```bash
CH_PASS=$(openssl rand -hex 16)
kubectl create secret generic pulse-clickhouse-secret \
  --from-literal=CLICKHOUSE_USER=pulse \
  --from-literal=CLICKHOUSE_PASSWORD="${CH_PASS}" \
  --from-literal=PULSE_CLICKHOUSE_DSN="clickhouse://pulse:${CH_PASS}@pulse-clickhouse:9000/pulse"
```

### Webhook (B7 per-source webhooks)

Routes served when `pulse.webhookAddr` is set:
- `POST /webhook/ams` — global AMS webhook (uses `PULSE_WEBHOOK_SECRET` from `pulse.secretRef`)
- `POST /webhook/ams/{source_name}` — per-source webhook (per-source secret in meta store)

**Secret rotation**: per-source webhook secrets are read at startup. Changing a
per-source secret requires a pod restart (`kubectl rollout restart deploy/<release>`).

## HA deployment (Postgres meta store)

```yaml
# ha-values.yaml
pulse:
  replicaCount: 2
  secretRef:
    name: pulse-secrets   # must include PULSE_POSTGRES_DSN

postgres:
  enabled: true

clickhouse:
  enabled: true
  persistence:
    size: 100Gi
```

Note: `pulse.replicaCount > 1` requires `postgres.enabled=true`. SQLite is
single-writer and will corrupt with multiple replicas sharing a PVC.

## Resource sizing guide

| Tier | Streams | Viewers | pulse CPU | pulse Mem | ClickHouse Mem |
|------|---------|---------|-----------|-----------|----------------|
| Free (2 vCPU) | ≤50 | ≤2000 | 250m/500m | 256Mi/512Mi | 512Mi/1Gi |
| Pro (4 vCPU) | ≤500 | ≤20000 | 500m/1000m | 512Mi/1Gi | 1Gi/2Gi |
| Enterprise | 500+ | 200k+ | 1000m/2000m | 1Gi/2Gi | 2Gi/4Gi |

## Backup

Parity with `docker-compose.backup.yml`. Disabled by default (same as compose: the
backup overlay is opt-in). Enable with `backup.enabled=true`.

**Prerequisites**:
1. `clickhouse.auth.existingSecret` must be set (named CH user for `clickhouse-client` auth).
2. The chart automatically wires the ClickHouse backup disk config when `backup.enabled=true`.
3. A 10 Gi backup PVC is created automatically (`<release>-backups`), or point to an
   existing PVC via `backup.persistence.existingClaim`.

**What is backed up** (mirrors `pulse-backup.sh`):
- ClickHouse: `BACKUP DATABASE pulse TO Disk('backups', 'ch/pulse-<ts>.zip')`
- SQLite meta store: file copy with WAL consistency

**Retention**: 7 newest backups per artifact type (hardcoded in `pulse-backup.sh`).

**S3 push**: not enabled — the ClickHouse image does not ship `aws-cli`. To enable S3:
build a custom sidecar image on top of the ClickHouse digest-pinned base that adds `aws-cli`,
then set `backup.image.repository` and `backup.image.digest` accordingly, and inject
`PULSE_BACKUP_S3_BUCKET`/`PULSE_BACKUP_S3_PREFIX`/`PULSE_BACKUP_S3_REGION` via `backup.extraEnv`.

## D-002 limitation (EXPERIMENTAL)

This chart was authored and lint-validated locally (`helm lint`, `helm template`
golden file tests). It has **not** been deployed to a real Kubernetes cluster
(per decision D-002: no cluster in the dev environment). QA-01 should validate
on a clean cluster before marking the Helm AC as fully green.

**What this chart now provides** (S6 parity batch):
- ClickHouse auth via `clickhouse.auth.existingSecret` (parity with compose hardened overlay)
- Webhook Service + Ingress for `/webhook/ams` and `/webhook/ams/{source_name}` (B7)
- Backup CronJob mirroring compose backup sidecar (disabled by default, same as compose)
- `PULSE_SECRET_KEY` wired with `optional: false` — fails loud at scheduling, not at runtime
- Digest pinning support via `pulse.image.digest`
- NOTES.txt with post-install smoke test
