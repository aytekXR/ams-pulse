# ADR 0003: One binary, role flags — not microservices

**Status:** Accepted · **Date:** 2026-06-11

## Decision

Collector, query API, alert evaluator and report generator compile into one `pulse`
binary. Default mode runs everything in one process; `--role=collector|api|alerter`
splits roles for large installs. Services communicate in-process through Go
interfaces, with ClickHouse and the meta store as the only shared state.

## Rationale

- The customer operates this software, not us. Every additional container is
  installation friction against the 15-minute-install acceptance criterion and a
  support-load risk (§7.13). Two containers (pulse + ClickHouse) is the floor.
- The stateless-collector design (PRD §7.10) means role splitting is a deployment
  decision, not an architecture change — the in-process interfaces are the same ones
  a split deployment crosses via the stores.
- A small team (~3 FTE) cannot afford distributed-systems debugging across a fleet of
  heterogeneous customer environments.

## Consequences

Internal package boundaries must stay disciplined (enforced by the contracts/domain
rules in ARCHITECTURE.md §3) since the compiler won't stop cross-service imports the
way a network boundary would. INT-01's contract checks and code review carry that.
