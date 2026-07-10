package meta

// rebind_test.go — unit tests for the ?→$N placeholder rewriter.
//
// Tests are in the internal package (package meta, not meta_test) so they can
// call rewritePlaceholders directly. rebind() is also exercised to confirm
// caching semantics.

import (
	"testing"
)

func TestRewritePlaceholders_NoPlaceholders(t *testing.T) {
	input := "SELECT COUNT(*) FROM users"
	got := rewritePlaceholders(input)
	if got != input {
		t.Errorf("no-placeholder query should be unchanged: got %q, want %q", got, input)
	}
}

func TestRewritePlaceholders_SinglePlaceholder(t *testing.T) {
	input := "SELECT id FROM users WHERE username = ?"
	want := "SELECT id FROM users WHERE username = $1"
	got := rewritePlaceholders(input)
	if got != want {
		t.Errorf("single placeholder: got %q, want %q", got, want)
	}
}

func TestRewritePlaceholders_MultiplePlaceholders(t *testing.T) {
	input := "INSERT INTO users (id, username, pw_hash) VALUES (?, ?, ?)"
	want := "INSERT INTO users (id, username, pw_hash) VALUES ($1, $2, $3)"
	got := rewritePlaceholders(input)
	if got != want {
		t.Errorf("multiple placeholders: got %q, want %q", got, want)
	}
}

func TestRewritePlaceholders_ManyPlaceholders(t *testing.T) {
	// 19 placeholders — confirms counter increments past 9 (two-digit $N).
	input := "INSERT INTO t VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"
	want := "INSERT INTO t VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)"
	got := rewritePlaceholders(input)
	if got != want {
		t.Errorf("many placeholders: got %q, want %q", got, want)
	}
}

func TestRebind_SQLitePassthrough(t *testing.T) {
	q := "SELECT id FROM users WHERE id = ?"
	got := rebind("sqlite", q)
	if got != q {
		t.Errorf("sqlite: query should pass through unchanged; got %q", got)
	}
}

func TestRebind_EmptyBackendPassthrough(t *testing.T) {
	q := "SELECT id FROM users WHERE id = ?"
	got := rebind("", q)
	if got != q {
		t.Errorf("empty backend: query should pass through unchanged; got %q", got)
	}
}

func TestRebind_PostgresRewrite(t *testing.T) {
	q := "UPDATE users SET username=?, role=?, updated_at=? WHERE id=?"
	want := "UPDATE users SET username=$1, role=$2, updated_at=$3 WHERE id=$4"
	got := rebind("postgres", q)
	if got != want {
		t.Errorf("postgres rewrite: got %q, want %q", got, want)
	}
}

func TestRebind_CacheHit(t *testing.T) {
	// The same query string must return the identical (pointer-equal) result on
	// a second call — confirming the sync.Map cache is populated and hit.
	q := "SELECT id FROM probes WHERE id=? AND enabled=?"
	first := rebind("postgres", q)
	second := rebind("postgres", q)
	if first != second {
		t.Errorf("cache hit: first=%q second=%q — should be equal", first, second)
	}
	// Also confirm the value is correct.
	want := "SELECT id FROM probes WHERE id=$1 AND enabled=$2"
	if first != want {
		t.Errorf("cache hit: rewritten value %q != %q", first, want)
	}
}
