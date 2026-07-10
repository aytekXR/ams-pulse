package meta

// db.go — thin wrappers over sql.DB that apply ?→$N rebinding for Postgres.
//
// All methods in this package use s.execContext / s.queryRowContext /
// s.queryContext instead of calling s.db.ExecContext etc. directly. The
// wrappers pass the query through rebind(s.backend, query) so that Postgres
// receives $1, $2, … placeholders while SQLite continues to receive ?.

import (
	"context"
	"database/sql"
)

// execContext executes a non-returning SQL statement after applying placeholder
// rebinding for the Postgres backend.
func (s *Store) execContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, rebind(s.backend, query), args...)
}

// queryRowContext executes a single-row SQL query after applying placeholder
// rebinding for the Postgres backend.
func (s *Store) queryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, rebind(s.backend, query), args...)
}

// queryContext executes a multi-row SQL query after applying placeholder
// rebinding for the Postgres backend.
func (s *Store) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, rebind(s.backend, query), args...)
}
