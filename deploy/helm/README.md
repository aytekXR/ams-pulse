# Pulse Helm chart

Kubernetes deployment for clustered AMS installs (PRD §7.10). The chart lives in
[`deploy/helm/pulse/`](pulse/) — see [`pulse/README.md`](pulse/README.md) for the
values table, secrets setup, HA deployment, and resource sizing.

**Status: experimental.** The chart is authored and CI-verified by `helm lint` +
`helm template` golden-file tests (default, Postgres+S3, and external-ClickHouse
value sets), but a real-cluster `helm install` has not yet been validated (D-002).
Docker Compose remains the supported production path — see
[`docs/runbooks/install.md`](../../docs/runbooks/install.md).
