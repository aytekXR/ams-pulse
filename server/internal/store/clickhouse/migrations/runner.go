// Package migrations runs the ClickHouse DDL migrations from contracts/db/clickhouse/.
//
// The runner is idempotent: it tracks applied migrations in a
// schema_migrations table and skips already-applied ones. Variable
// substitution replaces {db}, {retention_days}, {rollup_ttl_days} per the
// contract mechanism.
package migrations

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// Config holds migration runner configuration.
type Config struct {
	// MigrationsDir is the path to the directory containing *.sql files.
	// Defaults to contracts/db/clickhouse/ relative to the binary.
	MigrationsDir string

	// Database name substituted for {db}.
	Database string

	// RetentionDays substituted for {retention_days} (default: 90).
	RetentionDays int

	// RollupTTLDays substituted for {rollup_ttl_days} (default: 395).
	RollupTTLDays int
}

// Runner applies ClickHouse migrations idempotently.
type Runner struct {
	cfg    Config
	conn   clickhouse.Conn
	logger *slog.Logger
}

// New creates a Runner.
func New(conn clickhouse.Conn, cfg Config, logger *slog.Logger) *Runner {
	if cfg.Database == "" {
		cfg.Database = "pulse"
	}
	if cfg.RetentionDays == 0 {
		cfg.RetentionDays = 90
	}
	if cfg.RollupTTLDays == 0 {
		cfg.RollupTTLDays = 395 // ~13 months
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{cfg: cfg, conn: conn, logger: logger}
}

// Run applies all pending migrations.
func (r *Runner) Run(ctx context.Context) error {
	if err := r.ensureMigrationsTable(ctx); err != nil {
		return fmt.Errorf("migrations: ensure table: %w", err)
	}

	files, err := r.listMigrationFiles()
	if err != nil {
		return fmt.Errorf("migrations: list files: %w", err)
	}

	applied, err := r.loadApplied(ctx)
	if err != nil {
		return fmt.Errorf("migrations: load applied: %w", err)
	}

	for _, f := range files {
		name := filepath.Base(f)
		if applied[name] {
			r.logger.Debug("migrations: already applied, skipping", "file", name)
			continue
		}

		r.logger.Info("migrations: applying", "file", name)
		if err := r.applyFile(ctx, f); err != nil {
			return fmt.Errorf("migrations: apply %s: %w", name, err)
		}
		if err := r.markApplied(ctx, name); err != nil {
			return fmt.Errorf("migrations: mark applied %s: %w", name, err)
		}
		r.logger.Info("migrations: applied", "file", name)
	}
	return nil
}

// ensureMigrationsTable creates the schema_migrations table if it doesn't exist.
func (r *Runner) ensureMigrationsTable(ctx context.Context) error {
	db := r.cfg.Database
	// Ensure database exists.
	if err := r.conn.Exec(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", db)); err != nil {
		return err
	}
	ddl := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.schema_migrations (
			name        String,
			applied_at  DateTime DEFAULT now(),
			checksum    String
		) ENGINE = MergeTree()
		ORDER BY name
	`, db)
	return r.conn.Exec(ctx, ddl)
}

// listMigrationFiles returns *.sql files in MigrationsDir, sorted by name.
func (r *Runner) listMigrationFiles() ([]string, error) {
	dir := r.cfg.MigrationsDir
	if dir == "" {
		return nil, fmt.Errorf("MigrationsDir is required")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

// loadApplied returns the set of already-applied migration names.
func (r *Runner) loadApplied(ctx context.Context) (map[string]bool, error) {
	rows, err := r.conn.Query(ctx,
		fmt.Sprintf("SELECT name FROM %s.schema_migrations", r.cfg.Database))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}
	return applied, rows.Err()
}

// applyFile reads a migration file, substitutes variables, and executes it.
func (r *Runner) applyFile(ctx context.Context, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	sql := r.substitute(string(raw))

	// Split on semicolons and execute each statement.
	stmts := splitStatements(sql)
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		// Strip leading comment lines before checking if statement has real SQL.
		// A statement may begin with "--\n-- comment\n" followed by actual DDL.
		effective := stripLeadingComments(stmt)
		if effective == "" {
			continue // only comments, no SQL
		}
		if err := r.conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("statement error: %w\nSQL: %.200s", err, stmt)
		}
	}
	return nil
}

// stripLeadingComments removes leading -- comment lines from a SQL string,
// returning the remaining non-comment content trimmed.
func stripLeadingComments(sql string) string {
	lines := strings.Split(sql, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		// Found the first non-comment, non-empty line.
		return strings.TrimSpace(strings.Join(lines[i:], "\n"))
	}
	return ""
}

// markApplied records a migration as applied in the tracking table.
func (r *Runner) markApplied(ctx context.Context, name string) error {
	// Compute checksum of the migration name for idempotency.
	h := sha256.Sum256([]byte(name))
	checksum := fmt.Sprintf("%x", h[:8])

	return r.conn.Exec(ctx,
		fmt.Sprintf("INSERT INTO %s.schema_migrations (name, applied_at, checksum) VALUES (?, ?, ?)",
			r.cfg.Database),
		name,
		time.Now().UTC(),
		checksum,
	)
}

// substitute replaces {db}, {retention_days}, {rollup_ttl_days} in SQL.
func (r *Runner) substitute(sql string) string {
	sql = strings.ReplaceAll(sql, "{db}", r.cfg.Database)
	sql = strings.ReplaceAll(sql, "{retention_days}", fmt.Sprintf("%d", r.cfg.RetentionDays))
	sql = strings.ReplaceAll(sql, "{rollup_ttl_days}", fmt.Sprintf("%d", r.cfg.RollupTTLDays))
	return sql
}

// splitStatements splits SQL on semicolons but preserves multi-line statements.
// Handles comments and quoted strings naively — good enough for DDL files.
func splitStatements(sql string) []string {
	var stmts []string
	var buf strings.Builder
	inComment := false
	inLineComment := false

	for i := 0; i < len(sql); i++ {
		c := sql[i]
		if inLineComment {
			if c == '\n' {
				inLineComment = false
			}
			buf.WriteByte(c)
			continue
		}
		if inComment {
			buf.WriteByte(c)
			if c == '*' && i+1 < len(sql) && sql[i+1] == '/' {
				buf.WriteByte('/')
				i++
				inComment = false
			}
			continue
		}
		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			inLineComment = true
			buf.WriteByte(c)
			continue
		}
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			inComment = true
			buf.WriteByte(c)
			continue
		}
		if c == ';' {
			stmt := strings.TrimSpace(buf.String())
			if stmt != "" {
				stmts = append(stmts, stmt)
			}
			buf.Reset()
			continue
		}
		buf.WriteByte(c)
	}
	if stmt := strings.TrimSpace(buf.String()); stmt != "" {
		stmts = append(stmts, stmt)
	}
	return stmts
}
