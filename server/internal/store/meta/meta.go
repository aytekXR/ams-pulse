// Package meta implements the metadata store (config, users, API tokens, AMS
// sources, alert rules/channels/history, report schedules, license state).
//
// Backends: SQLite (default, single node, CGO-free) and Postgres (HA option),
// behind one interface; DDL lives in contracts/db/meta/ and sticks to the
// common SQL subset. Secrets (AMS credentials, channel tokens) are encrypted
// at rest ("per-node credentials vaulted locally", PRD F7).
package meta

// Store is the metadata store interface implemented by sqlite and postgres backends.
type Store interface {
	// TODO(BE-02): Users, Tokens, Sources, AlertRules, AlertChannels,
	// AlertHistory, ReportSchedules, License sub-repositories.
}
