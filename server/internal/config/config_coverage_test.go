// Package config — additional unit tests to raise statement coverage.
// This file is an internal test (package config) so unexported functions
// (validate, applyEnv, loadYAML) can be called directly.
package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// validate()
// ---------------------------------------------------------------------------

// TestValidate_RejectsEmptyListen confirms that validate returns an error
// that mentions "server.listen" when the listen address is empty.
func TestValidate_RejectsEmptyListen(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ""},
		Storage: StorageConfig{
			Retention: RetentionConfig{RawDays: 90},
		},
		Beacon: BeaconConfig{SampleRate: 1.0},
	}
	err := validate(cfg)
	if err == nil {
		t.Fatal("validate() returned nil; want error for empty server.listen")
	}
	if !strings.Contains(err.Error(), "server.listen") {
		t.Errorf("error %q should mention 'server.listen'", err.Error())
	}
}

// TestValidate_RejectsZeroRawDays confirms that validate rejects raw_days <= 0.
func TestValidate_RejectsZeroRawDays(t *testing.T) {
	for _, days := range []int{0, -1} {
		cfg := &Config{
			Server: ServerConfig{Listen: ":8090"},
			Storage: StorageConfig{
				Retention: RetentionConfig{RawDays: days},
			},
			Beacon: BeaconConfig{SampleRate: 1.0},
		}
		err := validate(cfg)
		if err == nil {
			t.Errorf("validate() returned nil for raw_days=%d; want error", days)
		}
	}
}

// TestValidate_RejectsInvalidSampleRate confirms that validate rejects
// sample_rate values outside the closed interval [0, 1].
func TestValidate_RejectsInvalidSampleRate(t *testing.T) {
	for _, sr := range []float64{-0.1, 1.1, 2.0} {
		cfg := &Config{
			Server: ServerConfig{Listen: ":8090"},
			Storage: StorageConfig{
				Retention: RetentionConfig{RawDays: 90},
			},
			Beacon: BeaconConfig{SampleRate: sr},
		}
		err := validate(cfg)
		if err == nil {
			t.Errorf("validate() returned nil for sample_rate=%v; want error", sr)
		}
	}
}

// TestValidate_AcceptsValidConfig confirms that validate returns nil for a
// fully valid configuration.
func TestValidate_AcceptsValidConfig(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8090"},
		Storage: StorageConfig{
			Retention: RetentionConfig{RawDays: 90},
		},
		Beacon: BeaconConfig{SampleRate: 0.5},
	}
	if err := validate(cfg); err != nil {
		t.Fatalf("validate() returned unexpected error: %v", err)
	}
}

// TestValidate_SampleRateBoundary checks the boundary values 0 and 1 are both
// accepted (closed interval).
func TestValidate_SampleRateBoundary(t *testing.T) {
	for _, sr := range []float64{0.0, 1.0} {
		cfg := &Config{
			Server: ServerConfig{Listen: ":8090"},
			Storage: StorageConfig{
				Retention: RetentionConfig{RawDays: 90},
			},
			Beacon: BeaconConfig{SampleRate: sr},
		}
		if err := validate(cfg); err != nil {
			t.Errorf("validate() returned error for sample_rate=%v (boundary): %v", sr, err)
		}
	}
}

// ---------------------------------------------------------------------------
// ClickHouseDSN()
// ---------------------------------------------------------------------------

// TestClickHouseDSN_PlainAddr verifies that a plain host:port address is
// wrapped into "clickhouse://<addr>/pulse".
func TestClickHouseDSN_PlainAddr(t *testing.T) {
	cfg := &Config{
		Storage: StorageConfig{ClickHouseAddr: "myhost:9000"},
	}
	got := cfg.ClickHouseDSN()
	want := "clickhouse://myhost:9000/pulse"
	if got != want {
		t.Errorf("ClickHouseDSN() = %q; want %q", got, want)
	}
}

// TestClickHouseDSN_FullDSN verifies that a value that already contains "://"
// is returned unchanged (the caller provided a complete DSN, possibly with
// credentials).
func TestClickHouseDSN_FullDSN(t *testing.T) {
	fullDSN := "clickhouse://user:s3cr3t@myhost:9000/mydb?dial_timeout=5s"
	cfg := &Config{
		Storage: StorageConfig{ClickHouseAddr: fullDSN},
	}
	got := cfg.ClickHouseDSN()
	if got != fullDSN {
		t.Errorf("ClickHouseDSN() = %q; want it returned as-is (%q)", got, fullDSN)
	}
}

// ---------------------------------------------------------------------------
// RollupTTLDays()
// ---------------------------------------------------------------------------

// TestRollupTTLDays verifies that the conversion is RollupMonths × 30.
func TestRollupTTLDays(t *testing.T) {
	cases := []struct {
		months int
		want   int
	}{
		{1, 30},
		{13, 390}, // default Free tier
		{24, 720},
		{3, 90},
	}
	for _, tc := range cases {
		cfg := &Config{
			Storage: StorageConfig{
				Retention: RetentionConfig{RollupMonths: tc.months},
			},
		}
		got := cfg.RollupTTLDays()
		if got != tc.want {
			t.Errorf("RollupTTLDays() with RollupMonths=%d: got %d, want %d",
				tc.months, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// applyEnv()
// ---------------------------------------------------------------------------

// TestApplyEnv_SecretKey verifies PULSE_SECRET_KEY is stored in cfg.SecretKey.
func TestApplyEnv_SecretKey(t *testing.T) {
	key := "thisisathirtytwobytesecretkey!!"
	t.Setenv("PULSE_SECRET_KEY", key)

	cfg := &Config{}
	applyEnv(cfg)

	if cfg.SecretKey != key {
		t.Errorf("SecretKey = %q; want %q", cfg.SecretKey, key)
	}
}

// TestApplyEnv_MetricsToken verifies PULSE_METRICS_TOKEN is stored in
// cfg.MetricsToken.
func TestApplyEnv_MetricsToken(t *testing.T) {
	t.Setenv("PULSE_METRICS_TOKEN", "prometheus-scrape-token")

	cfg := &Config{}
	applyEnv(cfg)

	if cfg.MetricsToken != "prometheus-scrape-token" {
		t.Errorf("MetricsToken = %q; want %q", cfg.MetricsToken, "prometheus-scrape-token")
	}
}

// TestApplyEnv_PostgresDSN verifies that PULSE_POSTGRES_DSN both sets
// MetaDSN and forces Meta to "postgres".
func TestApplyEnv_PostgresDSN(t *testing.T) {
	dsn := "postgres://pulse:pass@db:5432/pulse"
	t.Setenv("PULSE_POSTGRES_DSN", dsn)

	cfg := &Config{Storage: StorageConfig{Meta: "sqlite"}}
	applyEnv(cfg)

	if cfg.Storage.Meta != "postgres" {
		t.Errorf("Meta = %q after PULSE_POSTGRES_DSN; want %q", cfg.Storage.Meta, "postgres")
	}
	if cfg.Storage.MetaDSN != dsn {
		t.Errorf("MetaDSN = %q; want %q", cfg.Storage.MetaDSN, dsn)
	}
}

// TestApplyEnv_PollInterval verifies that PULSE_POLL_INTERVAL is parsed into
// a time.Duration and stored in cfg.PollInterval.
func TestApplyEnv_PollInterval(t *testing.T) {
	t.Setenv("PULSE_POLL_INTERVAL", "30s")

	cfg := &Config{}
	applyEnv(cfg)

	if cfg.PollInterval != 30*time.Second {
		t.Errorf("PollInterval = %v; want 30s", cfg.PollInterval)
	}
}

// TestApplyEnv_PollInterval_Invalid verifies that an unparseable duration
// leaves cfg.PollInterval at whatever value it had before (no silent zero).
func TestApplyEnv_PollInterval_Invalid(t *testing.T) {
	t.Setenv("PULSE_POLL_INTERVAL", "not-a-duration")

	before := 5 * time.Second
	cfg := &Config{PollInterval: before}
	applyEnv(cfg)

	if cfg.PollInterval != before {
		t.Errorf("PollInterval = %v; want original %v after invalid env", cfg.PollInterval, before)
	}
}

// TestApplyEnv_AnonymizeIP verifies all truth-y and false-y values for
// PULSE_ANONYMIZE_IP.
func TestApplyEnv_AnonymizeIP(t *testing.T) {
	truthy := []string{"1", "true", "TRUE", "True"}
	for _, val := range truthy {
		t.Run("set="+val, func(t *testing.T) {
			t.Setenv("PULSE_ANONYMIZE_IP", val)
			cfg := &Config{}
			applyEnv(cfg)
			if !cfg.Beacon.AnonymizeIP {
				t.Errorf("AnonymizeIP should be true for PULSE_ANONYMIZE_IP=%q", val)
			}
		})
	}

	// "false" must reset a previously-true value.
	t.Run("set=false", func(t *testing.T) {
		t.Setenv("PULSE_ANONYMIZE_IP", "false")
		cfg := &Config{Beacon: BeaconConfig{AnonymizeIP: true}}
		applyEnv(cfg)
		if cfg.Beacon.AnonymizeIP {
			t.Error("AnonymizeIP should be false for PULSE_ANONYMIZE_IP=false")
		}
	})
}

// TestApplyEnv_MigrationsDir verifies PULSE_MIGRATIONS_DIR is stored.
func TestApplyEnv_MigrationsDir(t *testing.T) {
	t.Setenv("PULSE_MIGRATIONS_DIR", "/data/migrations")

	cfg := &Config{}
	applyEnv(cfg)

	if cfg.MigrationsDir != "/data/migrations" {
		t.Errorf("MigrationsDir = %q; want %q", cfg.MigrationsDir, "/data/migrations")
	}
}

// TestApplyEnv_ClickHouseAddr verifies that PULSE_CLICKHOUSE_ADDR overrides
// the storage address (and takes precedence over PULSE_CLICKHOUSE_DSN when
// both are set, because it is applied second).
func TestApplyEnv_ClickHouseAddr(t *testing.T) {
	t.Setenv("PULSE_CLICKHOUSE_ADDR", "ch-prod:9000")

	cfg := &Config{Storage: StorageConfig{ClickHouseAddr: "localhost:9000"}}
	applyEnv(cfg)

	if cfg.Storage.ClickHouseAddr != "ch-prod:9000" {
		t.Errorf("ClickHouseAddr = %q; want %q", cfg.Storage.ClickHouseAddr, "ch-prod:9000")
	}
}

// TestApplyEnv_AllowedWSOrigins_CommaSplit calls applyEnv directly (not
// through Load) to verify the comma-split, space-trim, and empty-drop logic.
func TestApplyEnv_AllowedWSOrigins_CommaSplit(t *testing.T) {
	t.Setenv("PULSE_ALLOWED_WS_ORIGINS", "https://a.com , https://b.com , ")

	cfg := &Config{}
	applyEnv(cfg)

	want := []string{"https://a.com", "https://b.com"}
	if len(cfg.AllowedWSOrigins) != len(want) {
		t.Fatalf("AllowedWSOrigins = %v; want %v", cfg.AllowedWSOrigins, want)
	}
	for i := range want {
		if cfg.AllowedWSOrigins[i] != want[i] {
			t.Errorf("AllowedWSOrigins[%d] = %q; want %q", i, cfg.AllowedWSOrigins[i], want[i])
		}
	}
}

// TestApplyEnv_RetentionDays verifies PULSE_RETENTION_DAYS overrides
// RawDays, and that an invalid value is silently ignored (no panic, no zero).
func TestApplyEnv_RetentionDays(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		t.Setenv("PULSE_RETENTION_DAYS", "180")
		cfg := &Config{Storage: StorageConfig{Retention: RetentionConfig{RawDays: 90}}}
		applyEnv(cfg)
		if cfg.Storage.Retention.RawDays != 180 {
			t.Errorf("RawDays = %d; want 180", cfg.Storage.Retention.RawDays)
		}
	})
	t.Run("invalid string is ignored", func(t *testing.T) {
		t.Setenv("PULSE_RETENTION_DAYS", "bad")
		cfg := &Config{Storage: StorageConfig{Retention: RetentionConfig{RawDays: 90}}}
		applyEnv(cfg)
		if cfg.Storage.Retention.RawDays != 90 {
			t.Errorf("RawDays = %d; want original 90 when env is invalid", cfg.Storage.Retention.RawDays)
		}
	})
}

// ---------------------------------------------------------------------------
// loadYAML() + applyEnv() precedence
// ---------------------------------------------------------------------------

// TestLoadYAML_EnvOverridesYAML verifies the layering contract: values from
// environment variables override the same values loaded from the YAML file.
func TestLoadYAML_EnvOverridesYAML(t *testing.T) {
	yamlContent := `
server:
  listen: ":9090"
  ingest_listen: ":9091"
storage:
  clickhouse_addr: "yaml-ch:9000"
`
	dir := t.TempDir()
	yamlFile := dir + "/pulse.yaml"
	if err := os.WriteFile(yamlFile, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Env overrides only listen and clickhouse_addr.
	t.Setenv("PULSE_LISTEN_ADDR", ":7070")
	t.Setenv("PULSE_CLICKHOUSE_ADDR", "env-ch:9000")

	cfg := &Config{
		Server:  ServerConfig{Listen: ":8090", IngestListen: ":8091"},
		Storage: StorageConfig{ClickHouseAddr: "localhost:9000", Retention: RetentionConfig{RawDays: 90}},
		Beacon:  BeaconConfig{SampleRate: 1.0},
	}

	if err := loadYAML(cfg, yamlFile); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}
	// After YAML: listen=":9090", ingest_listen=":9091", ch="yaml-ch:9000"
	if cfg.Server.Listen != ":9090" {
		t.Errorf("after YAML: Listen = %q; want :9090", cfg.Server.Listen)
	}

	applyEnv(cfg)
	// After env: listen=":7070" (env wins), ingest_listen=":9091" (YAML, no env set)
	if cfg.Server.Listen != ":7070" {
		t.Errorf("after env: Listen = %q; want :7070 (env should override YAML)", cfg.Server.Listen)
	}
	if cfg.Server.IngestListen != ":9091" {
		t.Errorf("after env: IngestListen = %q; want :9091 (YAML preserved when no env)", cfg.Server.IngestListen)
	}
	if cfg.Storage.ClickHouseAddr != "env-ch:9000" {
		t.Errorf("after env: ClickHouseAddr = %q; want env-ch:9000 (env wins)", cfg.Storage.ClickHouseAddr)
	}
}

// TestLoadYAML_NonExistentFile verifies that a missing YAML file is silently
// accepted (defaults + env are used instead).
func TestLoadYAML_NonExistentFile(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Listen: ":8090"},
	}
	err := loadYAML(cfg, "/nonexistent/path/pulse.yaml")
	if err != nil {
		t.Errorf("loadYAML() for missing file returned error %v; want nil", err)
	}
	// cfg should be unchanged.
	if cfg.Server.Listen != ":8090" {
		t.Errorf("Listen changed to %q; want :8090", cfg.Server.Listen)
	}
}
