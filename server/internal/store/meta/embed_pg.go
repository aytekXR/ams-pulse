package meta

import _ "embed"

// EmbeddedDDLPostgres is the complete PostgreSQL meta store schema: migration
// 0001 (tables + indexes) followed immediately by migration 0002 (anomaly rule
// columns), 0003 (vod_poll_state seen-set table) and 0004 (audit_log table). All
// files are applied in order by MigrateEmbedded when backend == "postgres".
//
// Source files are kept in sync with the contracts directory. They are exact
// copies EXCEPT for two provenance comment lines prepended to each embedded
// file (lines 2-3: "Embedded copy of ..." + "Sync command: ..."); when
// re-syncing, copy the contracts file and re-add those two header lines:
//
//	cp contracts/db/meta/postgres/0001_init.sql \
//	   server/internal/store/meta/sql/postgres_0001_init.sql
//	cp contracts/db/meta/postgres/0002_anomaly_alert_rule.sql \
//	   server/internal/store/meta/sql/postgres_0002_anomaly_alert_rule.sql
//	cp contracts/db/meta/postgres/0003_vod_poll_state.sql \
//	   server/internal/store/meta/sql/postgres_0003_vod_poll_state.sql
//	cp contracts/db/meta/postgres/0004_audit_log.sql \
//	   server/internal/store/meta/sql/postgres_0004_audit_log.sql

//go:embed sql/postgres_0001_init.sql
var embeddedPGDDL0001 string

//go:embed sql/postgres_0002_anomaly_alert_rule.sql
var embeddedPGDDL0002 string

//go:embed sql/postgres_0003_vod_poll_state.sql
var embeddedPGDDL0003 string

//go:embed sql/postgres_0004_audit_log.sql
var embeddedPGDDL0004 string

// EmbeddedDDLPostgres concatenates all PG migration files in version order.
// MigrateEmbedded routes here when backend == "postgres".
var EmbeddedDDLPostgres = embeddedPGDDL0001 + "\n" + embeddedPGDDL0002 + "\n" + embeddedPGDDL0003 + "\n" + embeddedPGDDL0004
