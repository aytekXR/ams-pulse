package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// EnvConfig holds configuration loaded from environment variables.
// This is a temporary shim until BE-02 implements internal/config.
//
// HOOK(BE-02): Replace this with config.Load(os.Args) which reads YAML +
// flags + env in precedence order. The Config struct here covers the
// minimal surface needed for wave 1 data-plane assembly.
type EnvConfig struct {
	// ListenAddr is the HTTP listen address for the API server.
	ListenAddr string

	// ClickHouseDSN is the ClickHouse native protocol DSN.
	ClickHouseDSN string

	// ClickHouseDatabase is the ClickHouse database name.
	ClickHouseDatabase string

	// MigrationsDir is the path to ClickHouse migration SQL files.
	MigrationsDir string

	// RetentionDays for raw events (default 90).
	RetentionDays int

	// RollupTTLDays for rollup tables (default 395).
	RollupTTLDays int

	// AMSBaseURL is the AMS REST API base URL.
	AMSBaseURL string

	// AMSNodeID identifies this AMS node in events.
	AMSNodeID string

	// AMSAuthToken is the bearer token for AMS REST API (optional).
	AMSAuthToken string

	// AMSApplications is a comma-separated list of AMS app names to poll.
	// Empty = poll all apps.
	AMSApplications []string

	// PollInterval is the AMS REST poll interval.
	PollInterval time.Duration

	// LogTailPath is the path to the AMS analytics log file (empty = disabled).
	LogTailPath string

	// WebhookListenAddr is the address for the webhook HTTP server (empty = disabled).
	WebhookListenAddr string

	// WebhookSharedSecret for HMAC validation.
	WebhookSharedSecret string

	// LogLevel is the log level (debug|info|warn|error).
	LogLevel string
}

// loadEnvConfig reads configuration from PULSE_* environment variables.
func loadEnvConfig() (EnvConfig, error) {
	cfg := EnvConfig{
		ListenAddr:         envOrDefault("PULSE_LISTEN_ADDR", ":8090"),
		ClickHouseDSN:      envOrDefault("PULSE_CLICKHOUSE_DSN", "clickhouse://localhost:9000/pulse"),
		ClickHouseDatabase: envOrDefault("PULSE_CLICKHOUSE_DATABASE", "pulse"),
		MigrationsDir:      envOrDefault("PULSE_MIGRATIONS_DIR", ""),
		AMSBaseURL:         envOrDefault("PULSE_AMS_URL", "http://localhost:5080"),
		AMSNodeID:          envOrDefault("PULSE_AMS_NODE_ID", "standalone"),
		AMSAuthToken:       os.Getenv("PULSE_AMS_AUTH_TOKEN"),
		LogTailPath:        os.Getenv("PULSE_LOG_TAIL_PATH"),
		WebhookListenAddr:  os.Getenv("PULSE_WEBHOOK_ADDR"),
		WebhookSharedSecret: os.Getenv("PULSE_WEBHOOK_SECRET"),
		LogLevel:           envOrDefault("PULSE_LOG_LEVEL", "info"),
	}

	// Parse retention days.
	cfg.RetentionDays = envInt("PULSE_RETENTION_DAYS", 90)
	cfg.RollupTTLDays = envInt("PULSE_ROLLUP_TTL_DAYS", 395)

	// Parse poll interval.
	if v := os.Getenv("PULSE_POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("PULSE_POLL_INTERVAL: %w", err)
		}
		cfg.PollInterval = d
	} else {
		cfg.PollInterval = 5 * time.Second
	}

	// Parse comma-separated AMS applications.
	if v := os.Getenv("PULSE_AMS_APPLICATIONS"); v != "" {
		for _, app := range strings.Split(v, ",") {
			app = strings.TrimSpace(app)
			if app != "" {
				cfg.AMSApplications = append(cfg.AMSApplications, app)
			}
		}
	}

	return cfg, nil
}

func envOrDefault(key, dflt string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return dflt
}

func envInt(key string, dflt int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return dflt
}
