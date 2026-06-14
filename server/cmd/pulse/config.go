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

	// ─── Wave 2 data-plane config ─────────────────────────────────────────────

	// KafkaBrokers is a comma-separated list of Kafka broker addresses.
	// Empty = Kafka source disabled.
	KafkaBrokers []string

	// KafkaGroupID is the consumer group ID for the Kafka source.
	KafkaGroupID string

	// GeoMMDBPath is the path to a MaxMind-format .mmdb file for geo enrichment.
	// Empty = geo enrichment disabled (no-op resolver).
	GeoMMDBPath string

	// AnonymizeIP controls IP anonymization before geo lookup and storage.
	// Set to true for GDPR/KVKK compliance.
	AnonymizeIP bool

	// SessionIdleTimeout is the viewer session idle close timeout.
	// Default: 5 min.
	SessionIdleTimeout time.Duration

	// ClusterDiscoveryInterval is how often to poll AMS for cluster nodes.
	// Default: 30 s. New node visible ≤ 1 interval ≤ 2 min budget.
	ClusterDiscoveryInterval time.Duration

	// IngestTargetBitrateKbps is the expected healthy ingest bitrate (health score formula).
	// Default: 2000.
	IngestTargetBitrateKbps float64

	// IngestTargetFPS is the expected healthy ingest frame rate.
	// Default: 30.
	IngestTargetFPS float64
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

	// Wave 2: Kafka source.
	if v := os.Getenv("PULSE_KAFKA_BROKERS"); v != "" {
		for _, broker := range strings.Split(v, ",") {
			broker = strings.TrimSpace(broker)
			if broker != "" {
				cfg.KafkaBrokers = append(cfg.KafkaBrokers, broker)
			}
		}
	}
	cfg.KafkaGroupID = envOrDefault("PULSE_KAFKA_GROUP_ID", "pulse-collector")

	// Wave 2: Geo enrichment.
	cfg.GeoMMDBPath = os.Getenv("PULSE_GEO_MMDB_PATH")
	cfg.AnonymizeIP = os.Getenv("PULSE_ANONYMIZE_IP") == "true"

	// Wave 2: Session stitcher.
	if v := os.Getenv("PULSE_SESSION_IDLE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("PULSE_SESSION_IDLE_TIMEOUT: %w", err)
		}
		cfg.SessionIdleTimeout = d
	} else {
		cfg.SessionIdleTimeout = 5 * time.Minute
	}

	// Wave 2: Cluster discovery.
	if v := os.Getenv("PULSE_CLUSTER_DISCOVERY_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("PULSE_CLUSTER_DISCOVERY_INTERVAL: %w", err)
		}
		cfg.ClusterDiscoveryInterval = d
	} else {
		cfg.ClusterDiscoveryInterval = 30 * time.Second
	}

	// Wave 2: Ingest health formula targets.
	if v := os.Getenv("PULSE_INGEST_TARGET_BITRATE_KBPS"); v != "" {
		n, err := strconv.ParseFloat(v, 64)
		if err == nil {
			cfg.IngestTargetBitrateKbps = n
		}
	}
	if cfg.IngestTargetBitrateKbps == 0 {
		cfg.IngestTargetBitrateKbps = 2000
	}
	if v := os.Getenv("PULSE_INGEST_TARGET_FPS"); v != "" {
		n, err := strconv.ParseFloat(v, 64)
		if err == nil {
			cfg.IngestTargetFPS = n
		}
	}
	if cfg.IngestTargetFPS == 0 {
		cfg.IngestTargetFPS = 30
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
