package main

import "fmt"

// versionString returns the canonical version line for the pulse binary.
// The three package-level vars (Version, GitCommit, BuildDate) are stamped at
// link time via -ldflags; the defaults ("dev" / "unknown") are used when
// the binary is built without ldflags (e.g. plain `go build ./...`).
func versionString() string {
	return fmt.Sprintf("pulse %s (commit %s, built %s)", Version, GitCommit, BuildDate)
}
