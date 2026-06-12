// Package config loads and validates Pulse server configuration.
//
// Sources, in precedence order: flags > environment (PULSE_*) > YAML file
// (deploy/config/pulse.example.yaml documents the full surface). Pre-tuned
// defaults matter commercially: the Free/Pro tiers must run on a 2-vCPU
// sidecar with zero tuning (PRD §7.13).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// AMSSource is one configured AMS data source.
type AMSSource struct {
	// Name is the source identifier used in PULSE_AMS_<NAME>_TOKEN env resolution.
	Name string `yaml:"name"`

	// RestURL is the AMS REST API base URL.
	RestURL string `yaml:"rest_url"`

	// RestUser is the AMS REST username (optional).
	RestUser string `yaml:"rest_user"`

	// AnalyticsLog is the path to the AMS analytics log file (optional).
	AnalyticsLog string `yaml:"analytics_log"`

	// KafkaBrokers is a list of Kafka broker addresses (optional).
	KafkaBrokers []string `yaml:"kafka_brokers"`

	// WebhookSecret is the HMAC secret for AMS webhooks (optional).
	WebhookSecret string `yaml:"webhook_secret"`

	// Token is the AMS REST bearer token — resolved from env PULSE_AMS_<NAME>_TOKEN.
	// Never written to the config file.
	Token string `yaml:"-"`
}

// StorageConfig holds data store configuration.
type StorageConfig struct {
	// ClickHouseAddr is host:port for ClickHouse native protocol.
	ClickHouseAddr string `yaml:"clickhouse_addr"`

	// Meta is the meta store backend: "sqlite" (default) or "postgres".
	Meta string `yaml:"meta"`

	// MetaDSN is the SQLite file path or Postgres DSN.
	// Resolved from PULSE_META_DSN or PULSE_POSTGRES_DSN env vars.
	MetaDSN string `yaml:"-"`

	// Retention holds raw event and rollup TTL settings.
	Retention RetentionConfig `yaml:"retention"`
}

// RetentionConfig holds TTL settings.
type RetentionConfig struct {
	// RawDays is the number of days raw events are kept (default 90).
	RawDays int `yaml:"raw_days"`

	// RollupMonths is the number of months rollups are kept (default 13).
	RollupMonths int `yaml:"rollup_months"`
}

// BeaconConfig holds beacon ingest settings.
type BeaconConfig struct {
	// SampleRate is the fraction of beacon events to ingest (0–1).
	SampleRate float64 `yaml:"sample_rate"`

	// AnonymizeIP strips the last octet of viewer IPs before geo-lookup.
	AnonymizeIP bool `yaml:"anonymize_ip"`
}

// LicenseConfig holds license settings.
type LicenseConfig struct {
	// Key is the license key string (empty = Free tier).
	Key string `yaml:"key"`

	// OfflineFile is the path to an offline license signature file.
	OfflineFile string `yaml:"offline_file"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	// Listen is the address for the UI+API server (default :8090).
	Listen string `yaml:"listen"`

	// IngestListen is the address for the beacon ingest server (default :8091).
	IngestListen string `yaml:"ingest_listen"`

	// BaseURL is used for alert deep-link URLs (optional).
	BaseURL string `yaml:"base_url"`
}

// Config is the root configuration for the Pulse server.
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	AMS     AMSConfig     `yaml:"ams"`
	Storage StorageConfig `yaml:"storage"`
	Beacon  BeaconConfig  `yaml:"beacon"`
	License LicenseConfig `yaml:"license"`

	// Operational (env-only)
	LogLevel      string        `yaml:"-"`
	PollInterval  time.Duration `yaml:"-"`
	MigrationsDir string        `yaml:"-"`
	SecretKey     string        `yaml:"-"` // PULSE_SECRET_KEY — for AES-GCM encryption
	MetricsToken  string        `yaml:"-"` // PULSE_METRICS_TOKEN — optional scrape token
}

// AMSConfig holds all AMS source definitions.
type AMSConfig struct {
	Sources []AMSSource `yaml:"sources"`
}

// yamlConfig is the raw unmarshalled YAML shape (mirrors pulse.example.yaml).
type yamlConfig struct {
	Server struct {
		Listen       string `yaml:"listen"`
		IngestListen string `yaml:"ingest_listen"`
		BaseURL      string `yaml:"base_url"`
	} `yaml:"server"`
	AMS struct {
		Sources []struct {
			Name          string   `yaml:"name"`
			RestURL       string   `yaml:"rest_url"`
			RestUser      string   `yaml:"rest_user"`
			AnalyticsLog  string   `yaml:"analytics_log"`
			KafkaBrokers  []string `yaml:"kafka_brokers"`
			WebhookSecret string   `yaml:"webhook_secret"`
		} `yaml:"sources"`
	} `yaml:"ams"`
	Storage struct {
		ClickHouseAddr string `yaml:"clickhouse_addr"`
		Meta           string `yaml:"meta"`
		Retention      struct {
			RawDays      int `yaml:"raw_days"`
			RollupMonths int `yaml:"rollup_months"`
		} `yaml:"retention"`
	} `yaml:"storage"`
	Beacon struct {
		SampleRate  float64 `yaml:"sample_rate"`
		AnonymizeIP bool    `yaml:"anonymize_ip"`
	} `yaml:"beacon"`
	License struct {
		Key         string `yaml:"key"`
		OfflineFile string `yaml:"offline_file"`
	} `yaml:"license"`
}

// Load resolves configuration from flags/env/file.
//
// Resolution order:
//  1. Parse --config=<path> flag from args (default: pulse.yaml beside binary)
//  2. Load YAML file if found (errors are surfaced with actionable messages)
//  3. Apply PULSE_* env overrides
//
// AMS credentials: PULSE_AMS_<NAME>_TOKEN where <NAME> is the uppercase source name.
func Load(args []string) (*Config, error) {
	configFile := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "--config=") {
			configFile = strings.TrimPrefix(arg, "--config=")
		} else if arg == "--config" || strings.HasPrefix(arg, "-config=") {
			// handle -config=value (single dash)
			if strings.HasPrefix(arg, "-config=") {
				configFile = strings.TrimPrefix(arg, "-config=")
			}
		}
	}

	// Defaults
	cfg := &Config{
		Server: ServerConfig{
			Listen:       ":8090",
			IngestListen: ":8091",
		},
		Storage: StorageConfig{
			ClickHouseAddr: "localhost:9000",
			Meta:           "sqlite",
			Retention: RetentionConfig{
				RawDays:      90,
				RollupMonths: 13,
			},
		},
		Beacon: BeaconConfig{
			SampleRate: 1.0,
		},
		LogLevel:     "info",
		PollInterval: 5 * time.Second,
	}

	// Load YAML file if available.
	if err := loadYAML(cfg, configFile); err != nil {
		return nil, err
	}

	// Apply env overrides.
	applyEnv(cfg)

	// Validate.
	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadYAML(cfg *Config, configFile string) error {
	// Determine path: explicit flag > pulse.yaml in cwd.
	path := configFile
	if path == "" {
		path = "pulse.yaml"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No file is fine — use defaults + env.
			return nil
		}
		return fmt.Errorf("config: read %s: %w", path, err)
	}

	var raw yamlConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("config: parse %s: %w", path, err)
	}

	// Map into cfg, only overriding non-zero values from YAML.
	if raw.Server.Listen != "" {
		cfg.Server.Listen = raw.Server.Listen
	}
	if raw.Server.IngestListen != "" {
		cfg.Server.IngestListen = raw.Server.IngestListen
	}
	if raw.Server.BaseURL != "" {
		cfg.Server.BaseURL = raw.Server.BaseURL
	}
	if raw.Storage.ClickHouseAddr != "" {
		cfg.Storage.ClickHouseAddr = raw.Storage.ClickHouseAddr
	}
	if raw.Storage.Meta != "" {
		cfg.Storage.Meta = raw.Storage.Meta
	}
	if raw.Storage.Retention.RawDays > 0 {
		cfg.Storage.Retention.RawDays = raw.Storage.Retention.RawDays
	}
	if raw.Storage.Retention.RollupMonths > 0 {
		cfg.Storage.Retention.RollupMonths = raw.Storage.Retention.RollupMonths
	}
	if raw.Beacon.SampleRate > 0 {
		cfg.Beacon.SampleRate = raw.Beacon.SampleRate
	}
	cfg.Beacon.AnonymizeIP = raw.Beacon.AnonymizeIP
	if raw.License.Key != "" {
		cfg.License.Key = raw.License.Key
	}
	if raw.License.OfflineFile != "" {
		cfg.License.OfflineFile = raw.License.OfflineFile
	}

	// AMS sources.
	for _, s := range raw.AMS.Sources {
		src := AMSSource{
			Name:          s.Name,
			RestURL:       s.RestURL,
			RestUser:      s.RestUser,
			AnalyticsLog:  s.AnalyticsLog,
			KafkaBrokers:  s.KafkaBrokers,
			WebhookSecret: s.WebhookSecret,
		}
		cfg.AMS.Sources = append(cfg.AMS.Sources, src)
	}

	return nil
}

func applyEnv(cfg *Config) {
	// Server
	if v := os.Getenv("PULSE_LISTEN_ADDR"); v != "" {
		cfg.Server.Listen = v
	}
	if v := os.Getenv("PULSE_INGEST_LISTEN_ADDR"); v != "" {
		cfg.Server.IngestListen = v
	}
	if v := os.Getenv("PULSE_BASE_URL"); v != "" {
		cfg.Server.BaseURL = v
	}

	// Storage
	if v := os.Getenv("PULSE_CLICKHOUSE_DSN"); v != "" {
		// Support full DSN — extract addr portion for clickhouse_addr.
		cfg.Storage.ClickHouseAddr = v // stored as full DSN when it looks like a DSN
	}
	if v := os.Getenv("PULSE_CLICKHOUSE_ADDR"); v != "" {
		cfg.Storage.ClickHouseAddr = v
	}
	if v := os.Getenv("PULSE_META"); v != "" {
		cfg.Storage.Meta = v
	}
	if v := os.Getenv("PULSE_META_DSN"); v != "" {
		cfg.Storage.MetaDSN = v
	}
	if v := os.Getenv("PULSE_POSTGRES_DSN"); v != "" {
		cfg.Storage.MetaDSN = v
		cfg.Storage.Meta = "postgres"
	}
	if v := os.Getenv("PULSE_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Storage.Retention.RawDays = n
		}
	}

	// Beacon
	if v := os.Getenv("PULSE_BEACON_SAMPLE_RATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Beacon.SampleRate = f
		}
	}
	if v := os.Getenv("PULSE_ANONYMIZE_IP"); v != "" {
		cfg.Beacon.AnonymizeIP = v == "1" || strings.EqualFold(v, "true")
	}

	// License
	if v := os.Getenv("PULSE_LICENSE_KEY"); v != "" {
		cfg.License.Key = v
	}
	if v := os.Getenv("PULSE_LICENSE_OFFLINE_FILE"); v != "" {
		cfg.License.OfflineFile = v
	}

	// Operational
	if v := os.Getenv("PULSE_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("PULSE_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.PollInterval = d
		}
	}
	if v := os.Getenv("PULSE_MIGRATIONS_DIR"); v != "" {
		cfg.MigrationsDir = v
	}
	if v := os.Getenv("PULSE_SECRET_KEY"); v != "" {
		cfg.SecretKey = v
	}
	if v := os.Getenv("PULSE_METRICS_TOKEN"); v != "" {
		cfg.MetricsToken = v
	}

	// AMS sources: resolve PULSE_AMS_<NAME>_TOKEN for each source.
	for i, src := range cfg.AMS.Sources {
		if src.Name != "" {
			envKey := "PULSE_AMS_" + strings.ToUpper(src.Name) + "_TOKEN"
			if token := os.Getenv(envKey); token != "" {
				cfg.AMS.Sources[i].Token = token
			}
		}
	}

	// If no sources configured, create a default from legacy env vars.
	if len(cfg.AMS.Sources) == 0 {
		amsURL := os.Getenv("PULSE_AMS_URL")
		if amsURL == "" {
			amsURL = "http://localhost:5080"
		}
		src := AMSSource{
			Name:    "main",
			RestURL: amsURL,
			Token:   os.Getenv("PULSE_AMS_AUTH_TOKEN"),
		}
		if nodeID := os.Getenv("PULSE_AMS_NODE_ID"); nodeID != "" {
			_ = nodeID // stored at collector level not per-source
		}
		cfg.AMS.Sources = append(cfg.AMS.Sources, src)
	}
}

func validate(cfg *Config) error {
	var errs []string

	if cfg.Server.Listen == "" {
		errs = append(errs, "server.listen must not be empty")
	}
	if cfg.Storage.Retention.RawDays <= 0 {
		errs = append(errs, "storage.retention.raw_days must be > 0")
	}
	if cfg.Beacon.SampleRate < 0 || cfg.Beacon.SampleRate > 1.0 {
		errs = append(errs, "beacon.sample_rate must be between 0 and 1.0")
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// ClickHouseDSN returns a ClickHouse DSN suitable for clickhouse-go.
// If ClickHouseAddr already looks like a full DSN (has ://), return it as-is.
func (c *Config) ClickHouseDSN() string {
	addr := c.Storage.ClickHouseAddr
	if strings.Contains(addr, "://") {
		return addr
	}
	return "clickhouse://" + addr + "/pulse"
}

// RollupTTLDays converts RollupMonths to approximate days.
func (c *Config) RollupTTLDays() int {
	return c.Storage.Retention.RollupMonths * 30
}
