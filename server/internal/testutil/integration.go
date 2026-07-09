//go:build integration

// Package testutil provides shared helpers for integration tests.
package testutil

import (
	"os"
	"testing"
)

// RequireClickHouseBin checks that /tmp/clickhouse exists.
// When env CI is non-empty and the binary is missing the test fails loudly
// (t.Fatalf) so a broken download step cannot silently yield a green run.
// Outside CI it skips, preserving the local-dev experience for contributors
// who have not downloaded the binary.
// Returns the binary path on success.
func RequireClickHouseBin(t *testing.T) string {
	t.Helper()
	const chBin = "/tmp/clickhouse"
	if _, err := os.Stat(chBin); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("CI env is set but /tmp/clickhouse not found: %v — "+
				"did the 'Download ClickHouse binary' step in ci.yml fail?", err)
		}
		t.Skipf("clickhouse binary not found at %s (set CI=true to fail loudly): %v", chBin, err)
	}
	return chBin
}
