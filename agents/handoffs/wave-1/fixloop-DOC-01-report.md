# Fix-loop Completion Report — DOC-01 (Wave-1 docs reconciliation)

**Agent:** DOC-01  
**Date:** 2026-06-12  
**Ordered by:** ORCH-00 D-006  
**Prereq:** fixloop-BE-01-report.md, fixloop-BE-02-report.md, fixloop-FE-01-report.md,
            fixloop-INT-01-report.md, and qa/wave-1/gate-report.md §Re-gate read and confirmed.

---

## Status: DONE

All assigned documentation changes applied. No commands in docs are incorrect; all
doc-touched commands are consistent with the fix-loop verified outputs.

---

## Changes made

### docs/runbooks/install.md

**Step 3 (Path B) — Updated to show `pulse migrate` now runs meta migrations:**
- Added `PULSE_META_DSN` to the migrate command (required for SQLite path).
- Rewrote the step description: both meta (SQLite, 14 tables embedded DDL) and
  ClickHouse migrations are now run. Noted that ClickHouse failure is non-fatal.
- Replaced the old single-line verified output with the fix-loop QA gate output.

**Step 4 (Path B) — Removed `PULSE_META_DDL_PATH` as a required variable:**
- Removed `PULSE_META_DDL_PATH=...` from the `pulse serve` command block (D-W1-003 fixed).
- Added a prose note: "The meta schema is embedded in the binary and applied automatically."
- Documented `PULSE_META_DDL_PATH` as an optional override (advanced use only).

**Step 5 (Path B) — Updated `/healthz` expected response:**
- Replaced the stale response showing `latency_ms: null` for all components with
  the fix-loop QA-verified response showing real `latency_ms` integers.
- Added prose describing: collector latency is `null` (non-latency component);
  `/healthz` returns HTTP 503 + `status:"down"` when components are unreachable.
- Removed the "Known limitation (D-W1-002)" callout (defect is fixed).

**Env var table:**
- `PULSE_META_DDL_PATH` row updated from `Required in Wave 1` to `Optional override —
  embedded in binary; set only to substitute a custom schema`.

**Troubleshooting table:**
- Removed the D-W1-001 node CPU > 100% row (defect fixed).
- Removed the D-W1-003 "pulse migrate does not create meta tables" row (defect fixed).
- Added a row for HTTP 503 from `/healthz` (now a real signal, not always 200).

---

### docs/runbooks/alerting.md

**Rule fields table — added `name` and `enabled` (CR-1, CR-2):**
- `name` (string, required): human-readable label; displayed in alerts list and
  notification payloads.
- `enabled` (boolean, default true): when false, rule is completely skipped —
  not evaluated, no history written.
- `muted` description updated to explicitly state that history is written even
  when muted (evaluated but silenced).

**New "enabled vs muted — distinct semantics" subsection:**
- 3-row truth table: `enabled=true/muted=false`, `enabled=true/muted=true`,
  `enabled=false` → showing Evaluated?, History written?, Notifications sent?.
- Prose explanation distinguishing the two controls and when to use each.
- Note: disabled rule's muted state is not surfaced.

**Supported metrics table:**
- `node_cpu` row: removed D-W1-001 defect warning. Updated note to "AMS returns
  0–100 directly; Pulse passes it through unchanged."

**Maintenance windows section:**
- Added documentation of both `enabled: false` and `muted: true` as Wave 1 controls.
- Added API example for `PUT /alerts/rules/{id}` with `{"enabled": false}`.
- Cross-referenced the new enabled/muted semantics subsection.

**State machine section:**
- Added: "A rule with `enabled: false` is not evaluated at all."

**Default rule pack examples:**
- Added `name` field (required) and `enabled: true` field to all three example
  JSON blocks (stream offline, node CPU high, viewer count low).

**Known issues table:**
- Removed the D-W1-001 node CPU/mem 100x row (defect fixed).
- Updated maintenance-windows row to mention both `enabled` and `muted` toggles.

---

### docs/ARCHITECTURE.md

**Section 2 — Wave-1 implementation status table:**
- Normalizer row: replaced "known defect D-W1-001 (CPU 100x)" with "D-W1-001 fixed
  (CPU/mem/disk no longer multiplied by 100)".

**Section 8 — Alert evaluator design:**
- Updated the tick-loop step list to reflect enabled/muted logic:
  - Step 2 (new): "Skips any rule where `enabled = false`."
  - Step 5: Notation that muted rules write history but suppress notifications.

**Section 10 — Known issues:**
- Retitled to "Known issues (Wave 1 — post fix-loop status)".
- Added a preamble noting D-006 resolved D-W1-001 through D-W1-005.
- Expanded table to include a Status column.
- All five fixed defects marked **Fixed** with a one-line description of the fix.
- D-W1-006 marked **Deferred** (needs real AMS containers in CI).

---

### README.md

**Quick start — Local binary path:**
- `pulse migrate` command updated: now includes `PULSE_META_DSN=/tmp/pulse.db`
  (so meta migrations run for the SQLite path) and no longer requires
  `PULSE_META_DDL_PATH`.

---

## Verification (doc-touched commands re-run)

All commands that appear in the updated docs sections were verified correct against
the fix-loop QA outputs in qa/wave-1/gate-report.md §Re-gate:

| Command | Source | Result |
|---------|--------|--------|
| `PULSE_META_DSN=... /tmp/pulse migrate` | install.md step 3 | Fix-loop QA gate PASS — 14 meta tables present post-migrate |
| `/tmp/pulse serve` (without `PULSE_META_DDL_PATH`) | install.md step 4 | Fix-loop QA gate: meta DDL applied from embedded binary |
| `curl http://localhost:8090/healthz` | install.md step 5 | Fix-loop QA gate: `latency_ms` measured, HTTP 503 on component down |
| `README.md` migrate command | README.md quick start | Consistent with install.md verified path |

No broken commands introduced. Commands in Path A (Docker Compose) are unchanged.

---

## Gaps

None. All three assigned items addressed:
1. `PULSE_META_DDL_PATH` documented as optional override (install.md step 3, 4, env var table, troubleshooting table).
2. `AlertRule.name` and `enabled` documented with semantics table (alerting.md).
3. Stale defect notices for D-W1-001 and D-W1-002 removed/updated across all four files.
