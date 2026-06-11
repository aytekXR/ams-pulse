# ADR 0001: Core technology stack

**Status:** Accepted · **Date:** 2026-06-11

## Decision

- **Server: Go** (single static binary).
- **Web: React + TypeScript + Vite.**
- **Beacon SDK: TypeScript**, bundled to ESM/CJS/IIFE with tsup.
- **Deployment: Docker Compose** (Helm in Phase 2).

## Rationale

- The PRD (§7.10) specifies "single Go binary, stateless" for the collector. Go gives
  CGO-free static binaries (trivial Docker images, air-gap friendly), first-class
  concurrency for the poll/tail/consume fan-in, and mature ClickHouse/Kafka/Prometheus
  client libraries. The team profile in §7.15 (senior Go/Java + ClickHouse engineer)
  matches.
- React is specified in the PRD and has the deepest ecosystem for dashboard work
  (charting, virtualized tables). Vite keeps the toolchain thin.
- TypeScript for the SDK because its consumers are JS developers integrating players;
  type definitions are part of the developer-experience pitch, and the 15 KB budget is
  enforceable with size-limit in CI.
- "Boring proven stack" is an explicit risk mitigation in PRD §7.13 (small-team risk).
  No frameworks beyond these; no microservices; no message bus of our own — AMS's
  optional Kafka is consumed, never required.

## Consequences

Java plugin SDK route (running inside AMS) was rejected: a sidecar survives AMS
upgrades, can monitor whole clusters from one instance, and keeps the read-only
promise auditable. Frame-level media access is not needed for analytics.
