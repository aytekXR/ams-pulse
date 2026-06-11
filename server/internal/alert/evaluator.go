// Package alert implements the rule engine (F5): rules evaluated on streaming
// aggregates held in memory with ClickHouse fallback for window queries.
//
// PRD acceptance criteria this package owns:
//   - detection-to-notification < 30 seconds
//   - no duplicate storms: grouping + per-rule cooldowns
//   - maintenance windows suppress firing
//   - rules survive restarts (persisted in meta store)
//   - default rule pack ships enabled-but-muted
//
// Channel adapters live in alert/channels; the evaluator only emits
// domain.Notification values (contracts/events/alert-notification.schema.json).
package alert

// Evaluator runs alert rules against live aggregates.
type Evaluator struct {
	// TODO(BE-02)
}
