-- Pulse ClickHouse schema — migration 0003
-- Owner: INT-01 (wave 3-plus, D-018 CR-GAP3001).
-- Executed by `pulse migrate` on startup after 0002_concurrency_rollup.sql.
--
-- Adds segment_ttfb_ms to probe_results: the time to first byte of the
-- first media segment (distinct from TTFB of the manifest/connection).
-- Fresh installs get it from 0001; existing installs get it via ALTER.
-- IF NOT EXISTS ensures idempotency on repeated migrations.

ALTER TABLE {db}.probe_results ADD COLUMN IF NOT EXISTS segment_ttfb_ms UInt32 DEFAULT 0;
