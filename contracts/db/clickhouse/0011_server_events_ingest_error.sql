-- 0011: persist the stream_ingest_error payload (REVIEW-MP3 N4).
--
-- D-173 mapped the official AMS error webhooks (endpointFailed,
-- publishTimeoutError, encoderNotOpenedError) to event_type='stream_ingest_error',
-- but the sink extracted only pre-existing typed columns — action and stream name
-- were discarded, making stored rows indistinguishable from each other. These two
-- columns complete the pipeline. Both default to '' so every other event type is
-- unaffected; columns are appended at the table end to keep the positional batch
-- insert in server/internal/store/clickhouse/clickhouse.go aligned.

ALTER TABLE {db}.server_events ADD COLUMN IF NOT EXISTS action LowCardinality(String) DEFAULT '';

ALTER TABLE {db}.server_events ADD COLUMN IF NOT EXISTS stream_name String DEFAULT '';
