# Pulse Helm chart — Phase 2

Kubernetes deployment for clustered AMS installs (PRD §7.10): pulse Deployment
(roles split via `--role` flag for larger installs), ClickHouse StatefulSet or
external ClickHouse reference, optional Postgres meta store for HA.

Owner: INFRA-01. Not started — Docker Compose is the only supported deployment
through Phase 1.
