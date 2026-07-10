package meta

// rebind.go — deterministic ? → $N placeholder rewriter for Postgres.
//
// All SQL in this package uses ? placeholders (SQLite / MySQL style).
// When the backend is "postgres", pgx/stdlib requires $1, $2, … positional
// placeholders. rebind() converts ? → $N on first use and caches the result
// in rebindCache so the string-manipulation cost is paid only once per unique
// query string.
//
// PRECONDITION: no SQL string in this package contains a literal ? character
// inside a single-quoted string literal. Verified by checking all backtick
// strings in meta.go, anomaly.go, probe.go: grep "'[^']*?[^']*'" returns no
// matches (only the Go-level string 'indexByte(dir, '?')' in non-SQL code).

import (
	"fmt"
	"strings"
	"sync"
)

// rebindCache stores the $N-rewritten form of each SQL query string keyed by
// the original ? form. Populated lazily; never evicted (bounded by the finite
// set of unique query strings in the package).
var rebindCache sync.Map // map[string]string

// rebind returns query with every ? placeholder replaced by $1, $2, … when
// backend == "postgres". For all other backends (or when the query has no ?),
// the original string is returned unchanged. Results are cached so repeated
// calls with the same query string are O(1) after the first call.
func rebind(backend, query string) string {
	if backend != "postgres" {
		return query
	}
	if cached, ok := rebindCache.Load(query); ok {
		return cached.(string)
	}
	bound := rewritePlaceholders(query)
	rebindCache.Store(query, bound)
	return bound
}

// rewritePlaceholders replaces every ? in query with $1, $2, … in order.
// It does not attempt to skip quoted substrings because no SQL in this package
// contains a literal ? inside quotes (see package-level comment above).
func rewritePlaceholders(query string) string {
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			fmt.Fprintf(&b, "$%d", n)
			n++
		} else {
			b.WriteByte(query[i])
		}
	}
	return b.String()
}
