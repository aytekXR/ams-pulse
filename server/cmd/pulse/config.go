package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/config"
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

	// AMSLoginEmail is the AMS console email for cookie-session auth.
	// Corresponds to PULSE_AMS_LOGIN_EMAIL.
	AMSLoginEmail string

	// AMSLoginPassword is the AMS console password for cookie-session auth.
	// Corresponds to PULSE_AMS_LOGIN_PASSWORD.
	AMSLoginPassword string

	// AMSApplications is a comma-separated list of AMS app names to poll.
	// Empty = poll all apps.
	AMSApplications []string

	// PollInterval is the AMS REST poll interval.
	PollInterval time.Duration

	// WebhookListenAddr is the address for the webhook HTTP server (empty = disabled).
	WebhookListenAddr string

	// WebhookSharedSecret for HMAC validation.
	WebhookSharedSecret string

	// WebhookRequireTimestamp enables webhook replay protection (X-Ams-Timestamp
	// freshness + timestamp-bound HMAC; audit finding [8], D-123). Default false.
	// Enable ONLY when the signing sender is updated to send + sign the timestamp
	// (docs/AMS-INTEGRATION.md §4), else every webhook 401s (fail-closed).
	WebhookRequireTimestamp bool

	// WebhookTimestampSkew is the ± acceptance window for X-Ams-Timestamp when
	// WebhookRequireTimestamp is true (PULSE_WEBHOOK_TIMESTAMP_SKEW, a Go duration
	// like "5m" or "30s"). Zero (unset) → the handler default of 5 minutes.
	WebhookTimestampSkew time.Duration

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

	// ReportLogoPath is the filesystem path to a PNG/JPEG logo for PDF reports.
	// Corresponds to PULSE_REPORT_LOGO_PATH. Empty = embedded default Pulse waveform.
	ReportLogoPath string

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

	// ─── Wave 2 product-plane config (BE-02) ──────────────────────────────────

	// IngestListenAddr is the dedicated beacon ingest listener address.
	// If empty, beacon ingest is served on the main API listener.
	// Set to e.g. ":8091" to expose only the beacon endpoint on a separate port
	// (DMZ deployment). Corresponds to PULSE_INGEST_LISTEN_ADDR.
	IngestListenAddr string

	// MetricsToken, if set, requires Authorization: Bearer <token> on GET /metrics.
	// Corresponds to PULSE_METRICS_TOKEN.
	MetricsToken string

	// ─── Wave 2 product-plane: reports, schedules, S3 (BE-02, WO-204) ────────────

	// ReportsDir is the base directory for generated report artifacts.
	// Corresponds to PULSE_REPORTS_DIR (default: ./pulse-reports).
	ReportsDir string

	// S3Endpoint is the S3-compatible endpoint URL (e.g. https://s3.amazonaws.com).
	// Corresponds to PULSE_S3_ENDPOINT. Empty = S3 export disabled.
	S3Endpoint string

	// S3Bucket is the target bucket for report uploads.
	// Corresponds to PULSE_S3_BUCKET.
	S3Bucket string

	// S3Prefix is the key prefix applied to every uploaded object.
	// Corresponds to PULSE_S3_PREFIX (default: "reports/").
	S3Prefix string

	// S3Region is the AWS region (default: us-east-1).
	// Corresponds to PULSE_S3_REGION.
	S3Region string

	// S3AccessKeyEnvRef is the name of the env var that holds the S3 access key ID.
	// The ACTUAL key is read from that env var at upload time, never stored.
	// Corresponds to PULSE_S3_ACCESS_KEY_ENV (default: PULSE_S3_ACCESS_KEY_ID).
	S3AccessKeyEnvRef string

	// S3SecretKeyEnvRef is the name of the env var that holds the S3 secret access key.
	// Corresponds to PULSE_S3_SECRET_KEY_ENV (default: PULSE_S3_SECRET_ACCESS_KEY).
	S3SecretKeyEnvRef string

	// CORSAllowedOrigins is the list of origins permitted on /api/v1/* routes.
	// Parsed from PULSE_CORS_ALLOWED_ORIGINS (comma-separated).
	// Empty = no CORS headers emitted for API routes (same-origin requests still work).
	CORSAllowedOrigins []string

	// AllowedWSOrigins is the list of WebSocket origin patterns accepted by the
	// /live/ws endpoint. Parsed from PULSE_ALLOWED_WS_ORIGINS (comma-separated).
	// Empty = same-origin only (nhooyr.io/websocket derives the pattern from Host).
	AllowedWSOrigins []string

	// AnomalyTickS is the anomaly Detector tick interval in seconds (0 = default 60 s).
	// Overridden by PULSE_ANOMALY_TICK_S for CI (set to 5 in deploy/docker-compose.ci.yml).
	AnomalyTickS int

	// ─── S11 WO-C: SSO/OIDC phase 1 ─────────────────────────────────────────────

	// OIDCIssuer is the OIDC provider issuer URL. Empty = OIDC disabled.
	// Corresponds to PULSE_OIDC_ISSUER.
	OIDCIssuer string

	// OIDCClientID is the registered OAuth2 client ID.
	// Corresponds to PULSE_OIDC_CLIENT_ID.
	OIDCClientID string

	// OIDCClientSecret is the OAuth2 client secret (supports _FILE convention).
	// Corresponds to PULSE_OIDC_CLIENT_SECRET.
	OIDCClientSecret string

	// OIDCRedirectURL is the full callback URL registered with the provider.
	// e.g. https://pulse.example.com/auth/oidc/callback.
	// Corresponds to PULSE_OIDC_REDIRECT_URL.
	OIDCRedirectURL string

	// OIDCGroupClaim is the id_token claim name holding group membership.
	// Default: "groups". Corresponds to PULSE_OIDC_GROUP_CLAIM.
	OIDCGroupClaim string

	// OIDCGroupRoleMap is a JSON object mapping group names to Pulse roles.
	// e.g. {"ops-admins":"admin","pulse-users":"viewer"}.
	// Corresponds to PULSE_OIDC_GROUP_ROLE_MAP.
	OIDCGroupRoleMap string

	// OIDCDefaultRole is the Pulse role assigned when no group matches.
	// Empty (default) = fail-closed: no group match → 403 GROUP_DENIED.
	// Set to "viewer" to allow any authenticated OIDC user read-only access.
	// Corresponds to PULSE_OIDC_DEFAULT_ROLE. ORCH ruling: default is EMPTY.
	OIDCDefaultRole string

	// OIDCSessionTTL is the session cookie/token lifetime (default: 24h).
	// Corresponds to PULSE_OIDC_SESSION_TTL (duration string, e.g. "48h").
	OIDCSessionTTL time.Duration
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
		// AMSAuthToken resolved below via GetSecret.
		AMSLoginEmail:     os.Getenv("PULSE_AMS_LOGIN_EMAIL"),
		WebhookListenAddr: os.Getenv("PULSE_WEBHOOK_ADDR"),
		LogLevel:          envOrDefault("PULSE_LOG_LEVEL", "info"),
	}

	// Secret vars: use GetSecret so ${NAME}_FILE is honoured.
	amsAuthToken, err := config.GetSecret("PULSE_AMS_AUTH_TOKEN")
	if err != nil {
		return cfg, err
	}
	cfg.AMSAuthToken = amsAuthToken

	amsPassword, err := config.GetSecret("PULSE_AMS_LOGIN_PASSWORD")
	if err != nil {
		return cfg, err
	}
	cfg.AMSLoginPassword = amsPassword

	webhookSecret, err := config.GetSecret("PULSE_WEBHOOK_SECRET")
	if err != nil {
		return cfg, err
	}
	cfg.WebhookSharedSecret = webhookSecret
	cfg.WebhookRequireTimestamp = os.Getenv("PULSE_WEBHOOK_REQUIRE_TIMESTAMP") == "true"
	if v := os.Getenv("PULSE_WEBHOOK_TIMESTAMP_SKEW"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("PULSE_WEBHOOK_TIMESTAMP_SKEW: %w", err)
		}
		cfg.WebhookTimestampSkew = d
	}

	metricsToken, err := config.GetSecret("PULSE_METRICS_TOKEN")
	if err != nil {
		return cfg, err
	}
	cfg.MetricsToken = metricsToken

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
	cfg.ReportLogoPath = os.Getenv("PULSE_REPORT_LOGO_PATH")
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

	// Wave 2 product-plane config.
	cfg.IngestListenAddr = os.Getenv("PULSE_INGEST_LISTEN_ADDR")
	// MetricsToken resolved via GetSecret above (supports _FILE convention).

	// Wave 2 (WO-204): reports + S3 export config.
	cfg.ReportsDir = envOrDefault("PULSE_REPORTS_DIR", "pulse-reports")
	cfg.S3Endpoint = os.Getenv("PULSE_S3_ENDPOINT")
	cfg.S3Bucket = os.Getenv("PULSE_S3_BUCKET")
	cfg.S3Prefix = envOrDefault("PULSE_S3_PREFIX", "reports/")
	cfg.S3Region = envOrDefault("PULSE_S3_REGION", "us-east-1")
	cfg.S3AccessKeyEnvRef = envOrDefault("PULSE_S3_ACCESS_KEY_ENV", "PULSE_S3_ACCESS_KEY_ID")
	cfg.S3SecretKeyEnvRef = envOrDefault("PULSE_S3_SECRET_KEY_ENV", "PULSE_S3_SECRET_ACCESS_KEY")

	// A1: CORS allowlist for /api/v1/* routes.
	if v := os.Getenv("PULSE_CORS_ALLOWED_ORIGINS"); v != "" {
		for _, origin := range strings.Split(v, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				cfg.CORSAllowedOrigins = append(cfg.CORSAllowedOrigins, origin)
			}
		}
	}

	// WebSocket origin allowlist for /live/ws.
	if v := os.Getenv("PULSE_ALLOWED_WS_ORIGINS"); v != "" {
		for _, origin := range strings.Split(v, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				cfg.AllowedWSOrigins = append(cfg.AllowedWSOrigins, origin)
			}
		}
	}

	// S11 WO-B: anomaly detector tick interval.
	if v := os.Getenv("PULSE_ANOMALY_TICK_S"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.AnomalyTickS = n
		}
	}

	// S11 WO-C: OIDC/SSO phase 1 config.
	cfg.OIDCIssuer = os.Getenv("PULSE_OIDC_ISSUER")
	cfg.OIDCClientID = os.Getenv("PULSE_OIDC_CLIENT_ID")
	cfg.OIDCRedirectURL = os.Getenv("PULSE_OIDC_REDIRECT_URL")
	cfg.OIDCGroupClaim = envOrDefault("PULSE_OIDC_GROUP_CLAIM", "groups")
	cfg.OIDCGroupRoleMap = os.Getenv("PULSE_OIDC_GROUP_ROLE_MAP")
	cfg.OIDCDefaultRole = os.Getenv("PULSE_OIDC_DEFAULT_ROLE") // ORCH: default EMPTY = fail-closed

	oidcClientSecret, err := config.GetSecret("PULSE_OIDC_CLIENT_SECRET")
	if err != nil {
		return cfg, err
	}
	cfg.OIDCClientSecret = oidcClientSecret

	if v := os.Getenv("PULSE_OIDC_SESSION_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return cfg, fmt.Errorf("PULSE_OIDC_SESSION_TTL: %w", err)
		}
		cfg.OIDCSessionTTL = d
	} else {
		cfg.OIDCSessionTTL = 24 * time.Hour
	}

	// Boot validation: required fields when OIDC is enabled.
	if cfg.OIDCIssuer != "" {
		if cfg.OIDCClientID == "" {
			return cfg, fmt.Errorf("PULSE_OIDC_CLIENT_ID required when PULSE_OIDC_ISSUER is set")
		}
		if cfg.OIDCClientSecret == "" {
			return cfg, fmt.Errorf("PULSE_OIDC_CLIENT_SECRET required when PULSE_OIDC_ISSUER is set")
		}
		if cfg.OIDCRedirectURL == "" {
			return cfg, fmt.Errorf("PULSE_OIDC_REDIRECT_URL required when PULSE_OIDC_ISSUER is set")
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
