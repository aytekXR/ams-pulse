// Package clickhouse implements the event store: batched inserts of normalized
// events and query helpers over raw tables and rollups.
//
// Schema is owned by contracts/db/clickhouse/ migrations — this package never
// issues DDL outside `pulse migrate`. Performance budgets from the PRD:
// 13-month rollup queries < 3s (F2); ~1–2 GB per million viewer-sessions at
// default sampling (§7.10).
package clickhouse

// Store is the ClickHouse-backed event store.
type Store struct {
	// TODO(BE-01): conn pool, async insert batching, retention enforcement.
}
