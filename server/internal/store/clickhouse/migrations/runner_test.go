// Package migrations — unit tests for pure functions.
// No build tag: runs with plain go test (no integration infrastructure needed).
package migrations

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// slicesEqual reports whether two string slices contain the same elements.
// nil and empty slices are considered equal (len both == 0).
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ─── splitStatements ────────────────────────────────────────────────────────

func TestSplitStatements(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want []string
	}{
		{
			name: "empty input",
			sql:  "",
			want: nil,
		},
		{
			name: "whitespace only",
			sql:  "   \n\t  ",
			want: nil,
		},
		{
			name: "single statement with trailing semicolon",
			sql:  "SELECT 1;",
			want: []string{"SELECT 1"},
		},
		{
			name: "single statement without trailing semicolon",
			sql:  "SELECT 1",
			want: []string{"SELECT 1"},
		},
		{
			name: "two statements",
			sql:  "SELECT 1; SELECT 2;",
			want: []string{"SELECT 1", "SELECT 2"},
		},
		{
			name: "multiple without trailing semicolon",
			sql:  "SELECT 1; SELECT 2",
			want: []string{"SELECT 1", "SELECT 2"},
		},
		{
			// Line comment containing a semicolon must NOT trigger a split.
			name: "line comment with semicolon does not split",
			sql:  "-- ignore;\nSELECT 1;",
			want: []string{"-- ignore;\nSELECT 1"},
		},
		{
			// Block comment containing a semicolon must NOT trigger a split.
			name: "block comment with semicolon does not split",
			sql:  "/* ignore; */ SELECT 1;",
			want: []string{"/* ignore; */ SELECT 1"},
		},
		{
			// A file that is entirely comment lines with no semicolon produces a
			// single "statement" equal to the comment text.  applyFile then
			// calls stripLeadingComments and skips it — but splitStatements itself
			// does not discard it.
			name: "comments-only input returns single element",
			sql:  "-- only comment",
			want: []string{"-- only comment"},
		},
		{
			// The splitter is documented as "naive" for quoted strings — it DOES
			// split on semicolons inside single-quoted literals.  This test
			// documents and locks that known behavior.
			name: "embedded semicolon in quoted string is split (naive behavior)",
			sql:  "SELECT 'a;b';",
			want: []string{"SELECT 'a", "b'"},
		},
		{
			// Verify the IF NOT EXISTS marker (common in migrations) passes through
			// without corruption.
			name: "IF NOT EXISTS marker is preserved",
			sql:  "CREATE TABLE IF NOT EXISTS t (id Int32);",
			want: []string{"CREATE TABLE IF NOT EXISTS t (id Int32)"},
		},
		{
			// Multi-line DDL statement (typical migration shape).
			name: "multi-line statement",
			sql:  "CREATE TABLE t\n(\n    a Int32\n)\nENGINE = MergeTree();",
			want: []string{"CREATE TABLE t\n(\n    a Int32\n)\nENGINE = MergeTree()"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitStatements(tc.sql)
			if !slicesEqual(got, tc.want) {
				t.Errorf("splitStatements(%q)\n  got  %v (len %d)\n  want %v (len %d)",
					tc.sql, got, len(got), tc.want, len(tc.want))
			}
		})
	}
}

// ─── stripLeadingComments ────────────────────────────────────────────────────

func TestStripLeadingComments(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "empty input",
			sql:  "",
			want: "",
		},
		{
			name: "comment-only single line",
			sql:  "-- just a comment",
			want: "",
		},
		{
			name: "multiple comment lines only",
			sql:  "-- line 1\n-- line 2\n-- line 3",
			want: "",
		},
		{
			name: "SQL only, no leading comment",
			sql:  "SELECT 1",
			want: "SELECT 1",
		},
		{
			name: "blank lines only",
			sql:  "\n\n\n",
			want: "",
		},
		{
			name: "leading comment then SQL",
			sql:  "-- header\nSELECT 1",
			want: "SELECT 1",
		},
		{
			name: "leading comment with spaces then SQL",
			sql:  "  -- header  \nSELECT 1",
			want: "SELECT 1",
		},
		{
			name: "blank lines then comment then SQL",
			sql:  "\n-- header\n\nSELECT 1",
			want: "SELECT 1",
		},
		{
			name: "comment then multi-line SQL",
			sql:  "-- comment\nCREATE TABLE t\n(\n    a Int32\n)",
			want: "CREATE TABLE t\n(\n    a Int32\n)",
		},
		{
			// Trailing comment on a SQL line: the function only strips LEADING
			// comment lines, so a trailing inline comment stays in the result.
			name: "SQL with trailing inline comment is preserved",
			sql:  "SELECT 1 -- this stays",
			want: "SELECT 1 -- this stays",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripLeadingComments(tc.sql)
			if got != tc.want {
				t.Errorf("stripLeadingComments(%q)\n  got  %q\n  want %q",
					tc.sql, got, tc.want)
			}
		})
	}
}

// ─── Runner.substitute ───────────────────────────────────────────────────────

func TestSubstitute(t *testing.T) {
	r := New(nil, Config{
		Database:      "mydb",
		RetentionDays: 90,
		RollupTTLDays: 365,
	}, nil)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no placeholders",
			input: "SELECT 1",
			want:  "SELECT 1",
		},
		{
			name:  "{db} replaced",
			input: "CREATE DATABASE {db}",
			want:  "CREATE DATABASE mydb",
		},
		{
			name:  "{retention_days} replaced",
			input: "TTL {retention_days} DAYS",
			want:  "TTL 90 DAYS",
		},
		{
			name:  "{rollup_ttl_days} replaced",
			input: "TTL {rollup_ttl_days}",
			want:  "TTL 365",
		},
		{
			name:  "all three placeholders in one string",
			input: "USE {db}; TTL {retention_days}; ROLLUP {rollup_ttl_days}",
			want:  "USE mydb; TTL 90; ROLLUP 365",
		},
		{
			name:  "unknown placeholder unchanged",
			input: "{unknown} stays",
			want:  "{unknown} stays",
		},
		{
			name:  "multiple occurrences of same placeholder",
			input: "CREATE TABLE {db}.t1; CREATE TABLE {db}.t2;",
			want:  "CREATE TABLE mydb.t1; CREATE TABLE mydb.t2;",
		},
		{
			// Verifies the actual migration SQL pattern: {db}.table_name references.
			name:  "database-qualified table reference",
			input: "INSERT INTO {db}.server_events SELECT * FROM {db}.beacon_events",
			want:  "INSERT INTO mydb.server_events SELECT * FROM mydb.beacon_events",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := r.substitute(tc.input)
			if got != tc.want {
				t.Errorf("substitute(%q)\n  got  %q\n  want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ─── New defaults ────────────────────────────────────────────────────────────

func TestNew_Defaults(t *testing.T) {
	t.Run("zero Config receives all defaults", func(t *testing.T) {
		r := New(nil, Config{}, nil)
		if r.cfg.Database != "pulse" {
			t.Errorf("default Database = %q, want %q", r.cfg.Database, "pulse")
		}
		if r.cfg.RetentionDays != 90 {
			t.Errorf("default RetentionDays = %d, want 90", r.cfg.RetentionDays)
		}
		if r.cfg.RollupTTLDays != 395 {
			t.Errorf("default RollupTTLDays = %d, want 395", r.cfg.RollupTTLDays)
		}
		if r.logger == nil {
			t.Error("nil logger arg should fall back to slog.Default(), got nil")
		}
	})

	t.Run("explicit non-zero values are not overridden", func(t *testing.T) {
		r := New(nil, Config{
			Database:      "customdb",
			RetentionDays: 30,
			RollupTTLDays: 180,
		}, nil)
		if r.cfg.Database != "customdb" {
			t.Errorf("Database = %q, want %q", r.cfg.Database, "customdb")
		}
		if r.cfg.RetentionDays != 30 {
			t.Errorf("RetentionDays = %d, want 30", r.cfg.RetentionDays)
		}
		if r.cfg.RollupTTLDays != 180 {
			t.Errorf("RollupTTLDays = %d, want 180", r.cfg.RollupTTLDays)
		}
	})
}

// ─── listMigrationFiles (no ClickHouse conn needed) ──────────────────────────

func TestListMigrationFiles(t *testing.T) {
	t.Run("empty MigrationsDir returns error", func(t *testing.T) {
		r := New(nil, Config{MigrationsDir: ""}, nil)
		files, err := r.listMigrationFiles()
		if err == nil {
			t.Fatal("expected error for empty MigrationsDir, got nil")
		}
		if files != nil {
			t.Errorf("expected nil files, got %v", files)
		}
		if !strings.Contains(err.Error(), "MigrationsDir") {
			t.Errorf("error should mention MigrationsDir, got: %v", err)
		}
	})

	t.Run("non-existent directory returns error", func(t *testing.T) {
		r := New(nil, Config{MigrationsDir: "/does/not/exist/xyz123"}, nil)
		files, err := r.listMigrationFiles()
		if err == nil {
			t.Fatal("expected error for non-existent dir, got nil")
		}
		if files != nil {
			t.Errorf("expected nil files, got %v", files)
		}
	})

	t.Run("returns only .sql files sorted by name", func(t *testing.T) {
		dir := t.TempDir()

		// Create files: .sql files (unsorted) and some non-.sql files to filter out.
		for _, name := range []string{"0003_c.sql", "0001_a.sql", "0002_b.sql", "README.md", ".hidden"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("-- stub"), 0o644); err != nil {
				t.Fatalf("create %s: %v", name, err)
			}
		}

		r := New(nil, Config{MigrationsDir: dir}, nil)
		files, err := r.listMigrationFiles()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{
			filepath.Join(dir, "0001_a.sql"),
			filepath.Join(dir, "0002_b.sql"),
			filepath.Join(dir, "0003_c.sql"),
		}
		if !slicesEqual(files, want) {
			t.Errorf("listMigrationFiles\n  got  %v\n  want %v", files, want)
		}
	})

	t.Run("empty directory returns nil", func(t *testing.T) {
		dir := t.TempDir()
		r := New(nil, Config{MigrationsDir: dir}, nil)
		files, err := r.listMigrationFiles()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 0 {
			t.Errorf("expected empty result for empty dir, got %v", files)
		}
	})

	t.Run("sub-directories are not included", func(t *testing.T) {
		dir := t.TempDir()
		// Create a sub-directory with a .sql suffix to verify IsDir check.
		if err := os.Mkdir(filepath.Join(dir, "subdir.sql"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		// And a real sql file.
		if err := os.WriteFile(filepath.Join(dir, "0001_a.sql"), []byte("-- ok"), 0o644); err != nil {
			t.Fatalf("create file: %v", err)
		}
		r := New(nil, Config{MigrationsDir: dir}, nil)
		files, err := r.listMigrationFiles()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 1 {
			t.Errorf("expected 1 file (not the dir), got %v", files)
		}
	})
}

// ─── applyFile (no ClickHouse conn needed) ───────────────────────────────────

func TestApplyFile_FileNotFound(t *testing.T) {
	r := New(nil, Config{Database: "testdb", RetentionDays: 90, RollupTTLDays: 395}, nil)
	err := r.applyFile(context.Background(), "/no/such/file.sql")
	if err == nil {
		t.Fatal("expected error reading non-existent file, got nil")
	}
	if !os.IsNotExist(err) {
		t.Logf("error (expected not-exist): %v", err)
	}
}

// TestApplyFile_CommentsOnlyFile verifies that a file containing only SQL
// comment lines is treated as a no-op: the loop body executes, each
// statement is seen by stripLeadingComments, and since every effective body
// is empty the conn.Exec path is never reached (no conn needed).
func TestApplyFile_CommentsOnlyFile(t *testing.T) {
	dir := t.TempDir()
	content := "-- line 1\n-- line 2\n-- no real SQL here"
	path := filepath.Join(dir, "comments.sql")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	r := New(nil, Config{Database: "testdb", RetentionDays: 90, RollupTTLDays: 395}, nil)
	if err := r.applyFile(context.Background(), path); err != nil {
		t.Errorf("comments-only file should be a no-op (no conn.Exec called), got error: %v", err)
	}
}

// TestApplyFile_EmptyFile verifies that an empty file is a no-op:
// splitStatements returns nil so the loop body never executes and no
// conn access occurs.
func TestApplyFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.sql")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	r := New(nil, Config{Database: "testdb", RetentionDays: 90, RollupTTLDays: 395}, nil)
	if err := r.applyFile(context.Background(), path); err != nil {
		t.Errorf("empty file should be a no-op, got error: %v", err)
	}
}

// TestApplyFile_WhitespaceOnlyStatements verifies that statements that are
// entirely whitespace after TrimSpace are skipped (the stmt=="" branch).
func TestApplyFile_WhitespaceOnlyStatements(t *testing.T) {
	dir := t.TempDir()
	// Two semicolons with only whitespace between/around them produce empty statements.
	content := "   ;   \n  ;  "
	path := filepath.Join(dir, "whitespace.sql")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	r := New(nil, Config{Database: "testdb", RetentionDays: 90, RollupTTLDays: 395}, nil)
	if err := r.applyFile(context.Background(), path); err != nil {
		t.Errorf("whitespace-only statements should be a no-op, got error: %v", err)
	}
}
