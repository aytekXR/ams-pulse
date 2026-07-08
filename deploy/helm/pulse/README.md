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
- Pulse runs as UID 1000, non-root, `allowPrivilegeEscalation: false`.

## Prerequisites

- Kubernetes 1.25+
- Helm 3.12+
- A Kubernetes Secret with sensitive values (see **Secrets** below)

## Quick install

```bash
# 1. Create the secrets first.
kubectl create secret generic pulse-secrets \
  --from-literal=PULSE_AMS_AUTH_TOKEN=<your-ams-token> \
  --from-literal=PULSE_SECRET_KEY=<32-byte-hex-key>

# 2. Install the chart (default: SQLite meta, bundled ClickHouse).
helm install pulse ./deploy/helm/pulse \
  --set pulse.ams.url=http://your-ams:5080 \
  --set pulse.ams.nodeId=node-01 \
  --set pulse.secretRef.name=pulse-secrets

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
| `pulse.image.repository` | `ghcr.io/aytekxr/ams-pulse` | Pulse image |
| `pulse.image.tag` | `0.1.0` | Image tag (pin to digest in production) |
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
| `pulse.logTailPath` | `""` | `PULSE_LOG_TAIL_PATH` — AMS log file path in container |
| `pulse.webhookAddr` | `""` | `PULSE_WEBHOOK_ADDR` — webhook HTTP listener |
| `pulse.license.key` | `""` | `PULSE_LICENSE_KEY` (empty = Free tier) |
| `pulse.license.offlineFile` | `""` | `PULSE_LICENSE_FILE` (air-gapped Enterprise) |
| `pulse.logLevel` | `info` | `PULSE_LOG_LEVEL` (debug\|info\|warn\|error) |
| `pulse.secretRef.name` | `""` | Name of Secret providing sensitive env vars |
| `pulse.persistence.enabled` | `true` | PVC for SQLite meta store |
| `pulse.persistence.size` | `2Gi` | PVC size |
| `clickhouse.enabled` | `true` | Bundle ClickHouse StatefulSet |
| `clickhouse.externalDSN` | `""` | DSN when `clickhouse.enabled=false` |
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
| `ingestService.enabled` | `true` | Create the beacon ingest Service |
| `ingestService.type` | `ClusterIP` | Service type (ClusterIP\|LoadBalancer\|NodePort) |
| `ingestService.port` | `8091` | Beacon ingest port |
| `ingress.enabled` | `false` | Enable Ingress for API + UI |
| `ingressIngest.enabled` | `false` | Enable Ingress for beacon ingest (internet-facing) |

## Secrets

Create a Kubernetes Secret with any subset of these keys:

| Secret key | Binary env var | Description |
|------------|----------------|-------------|
| `PULSE_AMS_AUTH_TOKEN` | `PULSE_AMS_AUTH_TOKEN` | AMS REST bearer token |
| `PULSE_SECRET_KEY` | `PULSE_SECRET_KEY` | 32-byte hex key for AES-256-GCM at-rest encryption |
| `PULSE_WEBHOOK_SECRET` | `PULSE_WEBHOOK_SECRET` | HMAC-SHA256 secret for AMS webhook validation |
| `PULSE_METRICS_TOKEN` | `PULSE_METRICS_TOKEN` | Prometheus scrape token (Wave 2) |
| `PULSE_POSTGRES_DSN` | `PULSE_META_DSN` | Full Postgres DSN (when `postgres.enabled=true`) |
| `PULSE_S3_EXPORT_KEY_ID` | `PULSE_S3_EXPORT_KEY_ID` | S3 access key ID (when `s3Export.enabled=true`) |
| `PULSE_S3_EXPORT_SECRET_KEY` | `PULSE_S3_EXPORT_SECRET_KEY` | S3 secret access key |

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

## D-002 limitation

This chart was authored and lint-validated locally (`helm lint`, `helm template`
golden file tests). It has not been deployed to a real Kubernetes cluster on this
machine (per decision D-002: no Docker/cluster in the dev environment). QA-01
should validate on a clean cluster before marking the Helm AC as fully green.
