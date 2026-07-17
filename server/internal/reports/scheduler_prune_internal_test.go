// scheduler_prune_internal_test.go — S81 (D-143) test for report-artifact
// retention pruning. Internal test (package reports) because pruneArtifacts and
// isReportArtifact are unexported.
//
// The crux is SAFETY: the prune must remove ONLY old scheduler-generated
// artifacts and must NEVER touch the SQLite metastore / secret-key files, which
// (post-D-142) share the parent pulse-data volume when ArtifactsDir is
// /var/lib/pulse/reports.
package reports

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func pruneTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestIsReportArtifact(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"pulse-usage-2026-06-01-to-2026-06-30.csv", true},
		{"pulse-usage-2026-06-01-to-2026-06-30.pdf", true},
		// Non-artifacts that may share the directory — MUST be false so the prune
		// can never delete them.
		{"pulse_meta.db", false},
		{"pulse_meta.db-wal", false},
		{"pulse_meta.db-shm", false},
		{"pulse_secret.key", false},
		// Wrong prefix.
		{"report-2026-06.csv", false},
		{"usage-2026-06.csv", false},
		// Wrong suffix.
		{"pulse-usage-2026-06.txt", false},
		{"pulse-usage-2026-06.csv.bak", false},
		{"pulse-usage-2026-06.pdfx", false},
		// Degenerate but pattern-shaped.
		{".csv", false},
		{"pulse-usage-.csv", true},
	}
	for _, c := range cases {
		if got := isReportArtifact(c.name); got != c.want {
			t.Errorf("isReportArtifact(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// mkfile creates a file with the given mtime.
func mkfile(t *testing.T, dir, name string, mtime time.Time) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", name, err)
	}
}

func remaining(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestPruneArtifacts_AgeAndPatternGate(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -100)  // older than the 90-day window
	fresh := now.AddDate(0, 0, -10) // within the window

	// Old artifacts — MUST be deleted.
	mkfile(t, dir, "pulse-usage-2026-03-01-to-2026-03-31.csv", old)
	mkfile(t, dir, "pulse-usage-2026-03-01-to-2026-03-31.pdf", old)
	// Fresh artifact — MUST be kept.
	mkfile(t, dir, "pulse-usage-2026-06-01-to-2026-06-30.csv", fresh)
	// Non-artifacts, all OLD — MUST be kept (gated by pattern, not age).
	mkfile(t, dir, "pulse_meta.db", old)
	mkfile(t, dir, "pulse_meta.db-wal", old)
	mkfile(t, dir, "pulse_secret.key", old)
	mkfile(t, dir, "notes.txt", old)
	// A wrong-prefix .csv, OLD — MUST be kept (proves the prefix gate is load-bearing,
	// not just the .csv/.pdf suffix).
	mkfile(t, dir, "invoice-2026-03.csv", old)
	// A pulse-usage-prefixed non-report suffix, OLD — MUST be kept (proves the suffix
	// gate is load-bearing, not just the prefix).
	mkfile(t, dir, "pulse-usage-2026-03.txt", old)
	// A directory named like an artifact and OLD — MUST be kept (IsDir guard;
	// without it os.Remove would delete an empty dir).
	subdir := filepath.Join(dir, "pulse-usage-subdir.csv")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	if err := os.Chtimes(subdir, old, old); err != nil {
		t.Fatalf("chtimes subdir: %v", err)
	}

	s := &Scheduler{
		cfg:    SchedulerConfig{ArtifactsDir: dir, RetentionDays: 90},
		logger: pruneTestLogger(),
	}
	s.pruneArtifacts(now)

	got := remaining(t, dir)
	want := []string{
		"invoice-2026-03.csv",
		"notes.txt",
		"pulse-usage-2026-03.txt",
		"pulse-usage-2026-06-01-to-2026-06-30.csv",
		"pulse-usage-subdir.csv",
		"pulse_meta.db",
		"pulse_meta.db-wal",
		"pulse_secret.key",
	}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("after prune: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("after prune: got %v, want %v", got, want)
		}
	}
}

func TestPruneArtifacts_DisabledKeepsEverything(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -1000) // ancient
	mkfile(t, dir, "pulse-usage-2024-01-01-to-2024-01-31.csv", old)
	mkfile(t, dir, "pulse-usage-2024-01-01-to-2024-01-31.pdf", old)

	for _, rd := range []int{0, -1} {
		s := &Scheduler{
			cfg:    SchedulerConfig{ArtifactsDir: dir, RetentionDays: rd},
			logger: pruneTestLogger(),
		}
		s.pruneArtifacts(now)
		if got := remaining(t, dir); len(got) != 2 {
			t.Fatalf("RetentionDays=%d should disable pruning; got %v", rd, got)
		}
	}
}

// TestPruneArtifacts_SkipsSymlinks proves the regular-file guard: a symlink whose
// base name matches the artifact pattern must NOT be unlinked, even when it is old.
// (D-143 review finding: e.IsDir() returns false for symlinks, so the earlier guard
// would have deleted a pattern-named symlink; Type().IsRegular() excludes them.)
func TestPruneArtifacts_SkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "keepme.db") // stand-in for a metastore-shaped file
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "pulse-usage-link.csv")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}
	// A far-future cutoff so the symlink's own (creation-time) mtime is older than
	// it — without the regular-file guard the symlink would be unlinked.
	future := time.Now().AddDate(10, 0, 0)
	s := &Scheduler{
		cfg:    SchedulerConfig{ArtifactsDir: dir, RetentionDays: 90},
		logger: pruneTestLogger(),
	}
	s.pruneArtifacts(future)

	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("pattern-named symlink was deleted (regular-file guard failed): %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("symlink target was affected: %v", err)
	}
}

// TestRunDue_PrunesDespiteEarlyReturn proves the prune is decoupled from schedule
// listing (D-143 review finding): runDue with a nil meta store returns early at the
// meta==nil guard — the SAME early-return shape as a ListDueReportSchedules error —
// yet the deferred prune must still run so a persistent DB/volume error can't defeat
// retention.
func TestRunDue_PrunesDespiteEarlyReturn(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "pulse-usage-2020-01-01-to-2020-01-31.csv")
	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	oldTime := time.Now().AddDate(-1, 0, 0)
	if err := os.Chtimes(old, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	s := &Scheduler{
		cfg:    SchedulerConfig{ArtifactsDir: dir, RetentionDays: 90},
		logger: pruneTestLogger(),
		// meta intentionally nil → runDue returns early before running schedules.
	}
	s.runDue(context.Background())

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old artifact should have been pruned by the deferred prune on the early-return path; stat err=%v", err)
	}
}

func TestPruneArtifacts_MissingDirIsNoop(t *testing.T) {
	s := &Scheduler{
		cfg:    SchedulerConfig{ArtifactsDir: filepath.Join(t.TempDir(), "does-not-exist"), RetentionDays: 90},
		logger: pruneTestLogger(),
	}
	// Must not panic or error on a not-yet-created artifacts dir.
	s.pruneArtifacts(time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC))
}
