package meta

import _ "embed"

// EmbeddedDDL is the meta store schema (contracts/db/meta/0001_init.sql) embedded
// at compile time. It is used by runMigrate so operators do not need to supply
// PULSE_META_DDL_PATH for a working binary.
//
// The source of truth is contracts/db/meta/0001_init.sql; this copy is kept in
// sync at build time. If the contracts DDL changes, re-copy it here:
//
//	cp contracts/db/meta/0001_init.sql server/internal/store/meta/sql/0001_init.sql
//
//go:embed sql/0001_init.sql
var EmbeddedDDL string
