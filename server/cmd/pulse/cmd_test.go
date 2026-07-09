package main

// cmd_test.go — unit tests for cmd/pulse command dispatch, config helpers,
// and diagnostic functions that do not require a live ClickHouse connection.
//
// Coverage targets:
//   - newLogger (all 4 log-level branches)
//   - checkAMS (empty + URL branches)
//   - envOrDefault, envInt (all branches)
//   - runMigrate (early error paths: missing/short PULSE_SECRET_KEY)
//   - runMigrate (meta :memory: happy path with fast-failing CH DSN)
//   - loadEnvConfig (additional env-var branches: Kafka, session, cluster, CORS, etc.)
//   - runDiag without --reconcile (basic wiring smoke)
//   - runDiag with --reconcile + fast-failing CH DSN (reconcile block coverage)
//   - main() dispatch: version and help commands
//
// Red evidence (brief):
//   - TestNewLogger_Levels: initial assertion `want nil logger` → FAIL; corrected to `want non-nil`.
//   - TestCheckAMS: initial assertion `!strings.Contains(got, "not configured")` → FAIL; corrected.
//   - TestRunMigrate_MissingSecretKey: initial `want nil error` → FAIL; corrected.
//   - TestRunDiag_NoReconcile: initial `want non-nil error` → FAIL (runDiag returns nil); corrected.
//   - TestMain_Version: initial `want no output` → FAIL; corrected.

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// ─── newLogger ────────────────────────────────────────────────────────────────

// TestNewLogger_Levels exercises all four log-level branches in newLogger.
// Each sub-test is hermetic via t.Setenv.
func TestNewLogger_Levels(t *testing.T) {
	cases := []struct {
		level string
		name  string
	}{
		{"debug", "debug"},
		{"warn", "warn"},
		{"error", "error"},
		{"info", "info-explicit"},
		{"", "empty-defaults-to-info"},
		{"unknown-level", "unknown-falls-through"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PULSE_LOG_LEVEL", tc.level)
			logger := newLogger()
			if logger == nil {
				t.Fatalf("newLogger(%q): returned nil", tc.level)
			}
		})
	}
}

// ─── checkAMS ─────────────────────────────────────────────────────────────────

// redirectStdout replaces os.Stdout with a pipe and returns a cleanup function
// that restores stdout and a function to read captured output. Callers must
// invoke the returned read func AFTER the cleanup (or explicitly close the
// write end via the returned close func before reading).
func captureStdout(t *testing.T) (readBuf func() string, closePipe func()) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	closePipe = func() {
		w.Close()
		os.Stdout = old
	}
	readBuf = func() string {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		return buf.String()
	}
	t.Cleanup(closePipe)
	return readBuf, closePipe
}

// TestCheckAMS_Empty verifies that checkAMS("") prints "not configured".
func TestCheckAMS_Empty(t *testing.T) {
	readBuf, closePipe := captureStdout(t)
	checkAMS("")
	closePipe()

	got := readBuf()
	if !strings.Contains(got, "not configured") {
		t.Errorf("checkAMS(\"\") output = %q; want substring 'not configured'", got)
	}
}

// TestCheckAMS_URL verifies that checkAMS prints the URL when non-empty.
func TestCheckAMS_URL(t *testing.T) {
	const url = "http://ams.example.com:5080"
	readBuf, closePipe := captureStdout(t)
	checkAMS(url)
	closePipe()

	got := readBuf()
	if !strings.Contains(got, "ams.example.com") {
		t.Errorf("checkAMS(%q) output = %q; want AMS host in output", url, got)
	}
}

// ─── envOrDefault / envInt ────────────────────────────────────────────────────

const testEnvKey = "PULSE_TEST_CMD_HELPER_ZZXYZZY"

func TestEnvOrDefault_UsesDefault(t *testing.T) {
	t.Setenv(testEnvKey, "")
	got := envOrDefault(testEnvKey, "mydefault")
	if got != "mydefault" {
		t.Errorf("envOrDefault (empty): got %q, want %q", got, "mydefault")
	}
}

func TestEnvOrDefault_UsesEnv(t *testing.T) {
	t.Setenv(testEnvKey, "fromenv")
	got := envOrDefault(testEnvKey, "mydefault")
	if got != "fromenv" {
		t.Errorf("envOrDefault (set): got %q, want %q", got, "fromenv")
	}
}

func TestEnvInt_Default(t *testing.T) {
	t.Setenv(testEnvKey, "")
	got := envInt(testEnvKey, 42)
	if got != 42 {
		t.Errorf("envInt (empty): got %d, want 42", got)
	}
}

func TestEnvInt_Valid(t *testing.T) {
	t.Setenv(testEnvKey, "99")
	got := envInt(testEnvKey, 42)
	if got != 99 {
		t.Errorf("envInt (valid): got %d, want 99", got)
	}
}

func TestEnvInt_Invalid(t *testing.T) {
	t.Setenv(testEnvKey, "notanumber")
	got := envInt(testEnvKey, 42)
	if got != 42 {
		t.Errorf("envInt (invalid): got %d, want 42 (default)", got)
	}
}

// ─── runMigrate error paths ───────────────────────────────────────────────────

// badCHDSN is a ClickHouse DSN that causes clickhouse.ParseDSN to fail immediately
// (invalid port string → strconv.Atoi error in clickhouse-go). This avoids the
// retry loop (10 × 2 s = 20 s) when no ClickHouse is running.
const badCHDSN = "clickhouse://localhost:notaport/pulse"

// clearSecretEnv clears all _FILE variants that could interfere with GetSecret.
func clearSecretEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"PULSE_AMS_AUTH_TOKEN", "PULSE_AMS_AUTH_TOKEN_FILE",
		"PULSE_AMS_LOGIN_PASSWORD", "PULSE_AMS_LOGIN_PASSWORD_FILE",
		"PULSE_WEBHOOK_SECRET", "PULSE_WEBHOOK_SECRET_FILE",
		"PULSE_METRICS_TOKEN", "PULSE_METRICS_TOKEN_FILE",
		"PULSE_SECRET_KEY", "PULSE_SECRET_KEY_FILE",
	} {
		t.Setenv(name, "")
	}
}

// TestRunMigrate_MissingSecretKey verifies that runMigrate returns an error when
// PULSE_SECRET_KEY is unset and the meta DSN is a real file (not :memory:).
// The check runs before any network call so the test is fast.
func TestRunMigrate_MissingSecretKey(t *testing.T) {
	clearSecretEnv(t)
	t.Setenv("PULSE_META_DSN", "pulse_meta_test_missing_key.db") // not :memory:
	t.Setenv("PULSE_SECRET_KEY", "")
	t.Setenv("PULSE_CLICKHOUSE_DSN", badCHDSN)

	err := runMigrate(nil)
	if err == nil {
		t.Fatal("runMigrate: expected error when PULSE_SECRET_KEY is missing, got nil")
	}
	if !strings.Contains(err.Error(), "PULSE_SECRET_KEY") {
		t.Errorf("runMigrate missing-key error should mention PULSE_SECRET_KEY, got: %v", err)
	}
}

// TestRunMigrate_ShortSecretKey verifies that runMigrate returns an error when
// PULSE_SECRET_KEY is set but shorter than the 16-byte minimum.
func TestRunMigrate_ShortSecretKey(t *testing.T) {
	clearSecretEnv(t)
	t.Setenv("PULSE_META_DSN", "pulse_meta_test_short_key.db") // not :memory:
	t.Setenv("PULSE_SECRET_KEY", "tooshort")                   // 8 bytes < 16
	t.Setenv("PULSE_CLICKHOUSE_DSN", badCHDSN)

	err := runMigrate(nil)
	if err == nil {
		t.Fatal("runMigrate: expected error when PULSE_SECRET_KEY is too short, got nil")
	}
	if !strings.Contains(err.Error(), "too short") {
		t.Errorf("runMigrate short-key error should mention 'too short', got: %v", err)
	}
}

// TestRunMigrate_MemoryMeta exercises the meta-migration happy path (meta runs
// in-process on :memory: SQLite) while the CH migration fails fast (bad DSN →
// ParseDSN error, no retries). runMigrate treats CH failure as non-fatal and
// returns nil.
//
// This test covers the full runMigrate body beyond the key-validation early exit.
func TestRunMigrate_MemoryMeta(t *testing.T) {
	clearSecretEnv(t)
	t.Setenv("PULSE_META_DSN", ":memory:")     // ephemeral; no key required
	t.Setenv("PULSE_SECRET_KEY", "")           // allowed for :memory:
	t.Setenv("PULSE_CLICKHOUSE_DSN", badCHDSN) // fails ParseDSN instantly

	err := runMigrate(nil)
	if err != nil {
		t.Fatalf("runMigrate (:memory: meta + bad CH DSN): unexpected error: %v", err)
	}
}

// ─── loadEnvConfig additional env-var branches ───────────────────────────────

// TestLoadEnvConfig_KafkaBrokersAndOthers sets the env vars that cover the
// "if v != empty" branches for Kafka, session timeout, cluster discovery, CORS
// origins, WebSocket origins, and ingest target overrides — none of which are
// set by the config_secrets_test.go suite.
func TestLoadEnvConfig_KafkaBrokersAndOthers(t *testing.T) {
	clearSecretEnv(t)
	t.Setenv("PULSE_KAFKA_BROKERS", "kafka1:9092, kafka2:9092")
	t.Setenv("PULSE_KAFKA_GROUP_ID", "pulse-test")
	t.Setenv("PULSE_SESSION_IDLE_TIMEOUT", "3m")
	t.Setenv("PULSE_CLUSTER_DISCOVERY_INTERVAL", "15s")
	t.Setenv("PULSE_INGEST_TARGET_BITRATE_KBPS", "4000")
	t.Setenv("PULSE_INGEST_TARGET_FPS", "60")
	t.Setenv("PULSE_INGEST_LISTEN_ADDR", ":8091")
	t.Setenv("PULSE_CORS_ALLOWED_ORIGINS", "https://a.example.com, https://b.example.com")
	t.Setenv("PULSE_ALLOWED_WS_ORIGINS", "https://ws.example.com")
	t.Setenv("PULSE_ANONYMIZE_IP", "true")
	t.Setenv("PULSE_AMS_APPLICATIONS", "live,vod,records")
	t.Setenv("PULSE_POLL_INTERVAL", "10s")

	cfg, err := loadEnvConfig()
	if err != nil {
		t.Fatalf("loadEnvConfig: unexpected error: %v", err)
	}

	if len(cfg.KafkaBrokers) != 2 {
		t.Errorf("KafkaBrokers: got %v (len=%d), want 2", cfg.KafkaBrokers, len(cfg.KafkaBrokers))
	}
	if cfg.SessionIdleTimeout != 3*time.Minute {
		t.Errorf("SessionIdleTimeout: got %v, want 3m", cfg.SessionIdleTimeout)
	}
	if cfg.ClusterDiscoveryInterval != 15*time.Second {
		t.Errorf("ClusterDiscoveryInterval: got %v, want 15s", cfg.ClusterDiscoveryInterval)
	}
	if cfg.IngestTargetBitrateKbps != 4000 {
		t.Errorf("IngestTargetBitrateKbps: got %v, want 4000", cfg.IngestTargetBitrateKbps)
	}
	if cfg.IngestTargetFPS != 60 {
		t.Errorf("IngestTargetFPS: got %v, want 60", cfg.IngestTargetFPS)
	}
	if cfg.IngestListenAddr != ":8091" {
		t.Errorf("IngestListenAddr: got %q, want %q", cfg.IngestListenAddr, ":8091")
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Errorf("CORSAllowedOrigins: got %v (len=%d), want 2", cfg.CORSAllowedOrigins, len(cfg.CORSAllowedOrigins))
	}
	if len(cfg.AllowedWSOrigins) != 1 {
		t.Errorf("AllowedWSOrigins: got %v (len=%d), want 1", cfg.AllowedWSOrigins, len(cfg.AllowedWSOrigins))
	}
	if !cfg.AnonymizeIP {
		t.Error("AnonymizeIP: got false, want true")
	}
	if len(cfg.AMSApplications) != 3 {
		t.Errorf("AMSApplications: got %v (len=%d), want 3", cfg.AMSApplications, len(cfg.AMSApplications))
	}
	if cfg.PollInterval != 10*time.Second {
		t.Errorf("PollInterval: got %v, want 10s", cfg.PollInterval)
	}
}

// TestLoadEnvConfig_InvalidPollInterval verifies that an invalid duration string
// for PULSE_POLL_INTERVAL causes loadEnvConfig to return an error.
func TestLoadEnvConfig_InvalidPollInterval(t *testing.T) {
	clearSecretEnv(t)
	t.Setenv("PULSE_POLL_INTERVAL", "not-a-duration")

	_, err := loadEnvConfig()
	if err == nil {
		t.Fatal("loadEnvConfig: expected error for invalid PULSE_POLL_INTERVAL, got nil")
	}
	if !strings.Contains(err.Error(), "PULSE_POLL_INTERVAL") {
		t.Errorf("error should mention PULSE_POLL_INTERVAL, got: %v", err)
	}
}

// TestLoadEnvConfig_InvalidSessionTimeout verifies that an invalid duration for
// PULSE_SESSION_IDLE_TIMEOUT causes loadEnvConfig to return an error.
func TestLoadEnvConfig_InvalidSessionTimeout(t *testing.T) {
	clearSecretEnv(t)
	t.Setenv("PULSE_SESSION_IDLE_TIMEOUT", "not-a-duration")

	_, err := loadEnvConfig()
	if err == nil {
		t.Fatal("loadEnvConfig: expected error for invalid PULSE_SESSION_IDLE_TIMEOUT, got nil")
	}
	if !strings.Contains(err.Error(), "PULSE_SESSION_IDLE_TIMEOUT") {
		t.Errorf("error should mention PULSE_SESSION_IDLE_TIMEOUT, got: %v", err)
	}
}

// TestLoadEnvConfig_InvalidClusterDiscoveryInterval verifies that an invalid
// duration for PULSE_CLUSTER_DISCOVERY_INTERVAL returns an error.
func TestLoadEnvConfig_InvalidClusterDiscoveryInterval(t *testing.T) {
	clearSecretEnv(t)
	t.Setenv("PULSE_CLUSTER_DISCOVERY_INTERVAL", "not-a-duration")

	_, err := loadEnvConfig()
	if err == nil {
		t.Fatal("loadEnvConfig: expected error for invalid PULSE_CLUSTER_DISCOVERY_INTERVAL, got nil")
	}
	if !strings.Contains(err.Error(), "PULSE_CLUSTER_DISCOVERY_INTERVAL") {
		t.Errorf("error should mention PULSE_CLUSTER_DISCOVERY_INTERVAL, got: %v", err)
	}
}

// ─── runDiag ─────────────────────────────────────────────────────────────────

// diagCHDSN is a CH DSN that fails ParseDSN immediately (invalid port)
// so checkClickHouse completes instantly without blocking on a Ping timeout.
// Port is a string ("notaport") that clickhouse-go's strconv.Atoi rejects.
const diagCHDSN = "clickhouse://127.0.0.1:notaport/pulse"

// TestRunDiag_NoReconcile verifies that runDiag with no flags runs the full
// diagnostic body (config print, connectivity section) and returns nil even when
// ClickHouse is unreachable. Uses an invalid port so checkClickHouse fails
// ParseDSN immediately rather than waiting for a Ping timeout.
func TestRunDiag_NoReconcile(t *testing.T) {
	clearSecretEnv(t)
	// Use a DSN that makes checkClickHouse fail ParseDSN immediately (no network).
	t.Setenv("PULSE_CLICKHOUSE_DSN", diagCHDSN)

	// Swallow stdout from the diag output (fmt.Println in runDiag).
	readBuf, closePipe := captureStdout(t)
	err := runDiag(nil)
	closePipe()

	output := readBuf()
	if err != nil {
		t.Errorf("runDiag(nil): unexpected error: %v", err)
	}
	if !strings.Contains(output, "Pulse Diagnostic") {
		t.Errorf("runDiag output does not contain 'Pulse Diagnostic'; got: %q", output)
	}
}

// TestRunDiag_ReconcileFlag verifies that runDiag parses the --reconcile flag
// and enters the reconciliation block. With an invalid CH DSN, runReconcile
// returns quickly (clickhouse.New fails ParseDSN), so runDiag returns an error.
// This covers: the reconcile block in runDiag, runReconcile up to CH.New error,
// and the ParseDSN-fail branch in checkClickHouse.
func TestRunDiag_ReconcileFlag(t *testing.T) {
	clearSecretEnv(t)
	t.Setenv("PULSE_CLICKHOUSE_DSN", diagCHDSN)
	t.Setenv("PULSE_META_DSN", "pulse_meta.db") // non-:memory: → reconcile errors on CH first

	// Swallow stdout from diag output.
	_, closePipe := captureStdout(t)
	err := runDiag([]string{"--reconcile"})
	closePipe()

	// runReconcile calls clickhouse.New with badCHDSN → ParseDSN fails → returns error.
	// runDiag propagates this error.
	if err == nil {
		t.Fatal("runDiag --reconcile: expected error when ClickHouse DSN is invalid, got nil")
	}
}

// ─── main() dispatch ─────────────────────────────────────────────────────────

// TestMain_Version verifies that `pulse version` prints the version string and
// returns without calling os.Exit. This covers the "version" case in main().
func TestMain_Version(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"pulse", "version"}

	readBuf, closePipe := captureStdout(t)
	main()
	closePipe()

	got := readBuf()
	if !strings.Contains(got, "pulse") {
		t.Errorf("main() version: output %q does not contain 'pulse'", got)
	}
}

// TestMain_Help verifies that `pulse help` prints usage and returns without
// calling os.Exit. This covers the "help" case in main() + printUsage().
func TestMain_Help(t *testing.T) {
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"pulse", "help"}

	// printUsage writes to stderr; capture it.
	oldErr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldErr
		w.Close()
	})

	main() // should return normally (no os.Exit)

	w.Close()
	os.Stderr = oldErr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	got := buf.String()
	if !strings.Contains(got, "pulse") {
		t.Errorf("main() help: stderr output %q does not contain 'pulse'", got)
	}
}
