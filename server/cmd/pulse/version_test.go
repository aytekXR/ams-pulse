package main

import (
	"strings"
	"testing"
)

// TestVersionString verifies that versionString formats all three build vars
// into the canonical "pulse <ver> (commit <sha>, built <date>)" line.
func TestVersionString(t *testing.T) {
	// Save and restore package-level build vars so this test is hermetic.
	origVersion := Version
	origCommit := GitCommit
	origDate := BuildDate
	t.Cleanup(func() {
		Version = origVersion
		GitCommit = origCommit
		BuildDate = origDate
	})

	Version = "v1.2.3"
	GitCommit = "deadbeef"
	BuildDate = "2026-07-08T00:00:00Z"

	got := versionString()

	// Must contain all three stamped values.
	if !strings.Contains(got, "v1.2.3") {
		t.Errorf("versionString() missing version: got %q", got)
	}
	if !strings.Contains(got, "deadbeef") {
		t.Errorf("versionString() missing commit: got %q", got)
	}
	if !strings.Contains(got, "2026-07-08T00:00:00Z") {
		t.Errorf("versionString() missing build date: got %q", got)
	}

	// Must match the canonical format exactly.
	want := "pulse v1.2.3 (commit deadbeef, built 2026-07-08T00:00:00Z)"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}

// TestVersionStringDefaults verifies the unset ("dev"/"unknown") defaults.
func TestVersionStringDefaults(t *testing.T) {
	origVersion := Version
	origCommit := GitCommit
	origDate := BuildDate
	t.Cleanup(func() {
		Version = origVersion
		GitCommit = origCommit
		BuildDate = origDate
	})

	Version = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"

	got := versionString()
	want := "pulse dev (commit unknown, built unknown)"
	if got != want {
		t.Errorf("versionString() = %q, want %q", got, want)
	}
}
