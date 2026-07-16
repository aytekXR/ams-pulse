package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	oidclib "github.com/coreos/go-oidc/v3/oidc"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/alert/channels"
	"github.com/pulse-analytics/pulse/server/internal/anomaly"
	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/cluster"
	"github.com/pulse-analytics/pulse/server/internal/collector"
	"github.com/pulse-analytics/pulse/server/internal/collector/aggregator"
	beaconingest "github.com/pulse-analytics/pulse/server/internal/collector/beacon"
	"github.com/pulse-analytics/pulse/server/internal/collector/ingest"
	kafkasrc "github.com/pulse-analytics/pulse/server/internal/collector/kafka"
	"github.com/pulse-analytics/pulse/server/internal/collector/restpoller"
	"github.com/pulse-analytics/pulse/server/internal/collector/sessions"
	webhooksrc "github.com/pulse-analytics/pulse/server/internal/collector/webhook"
	"github.com/pulse-analytics/pulse/server/internal/config"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/prober"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/reports"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// resolveMetaBackend determines the meta store backend name and DSN from
// environment variables, with a PULSE_POSTGRES_DSN convenience override.
//
// Resolution order (last wins):
//  1. PULSE_META_DSN for the DSN (default: "pulse_meta.db").
//  2. PULSE_META for the backend (default: "sqlite").
//  3. PULSE_POSTGRES_DSN non-empty → backend="postgres", dsn=PULSE_POSTGRES_DSN
//     (convenience override; sets both without requiring PULSE_META separately).
//
// The function accepts getenv to make it fully unit-testable without touching
// the real environment (see meta_backend_test.go).
func resolveMetaBackend(getenv func(string) string) (backend, dsn string) {
	dsn = getenv("PULSE_META_DSN")
	if dsn == "" {
		dsn = "pulse_meta.db"
	}
	backend = getenv("PULSE_META")
	if backend == "" {
		backend = "sqlite"
	}
	if pgDSN := getenv("PULSE_POSTGRES_DSN"); pgDSN != "" {
		dsn = pgDSN
		backend = "postgres"
	}
	return backend, dsn
}

// anomalyDetectorBridge adapts *anomaly.Detector to the api.AnomalyDetector interface.
// The anomaly package uses anomaly.AnomalyFlag; the api package uses api.AnomalyFlagAPI.
// These are structurally identical; the bridge converts between them.
type anomalyDetectorBridge struct {
	det *anomaly.Detector
}

func (b *anomalyDetectorBridge) ComputeFlags(ctx context.Context, sigmaThreshold float64) ([]api.AnomalyFlagAPI, error) {
	flags, err := b.det.ComputeFlags(ctx, sigmaThreshold)
	if err != nil {
		return nil, err
	}
	out := make([]api.AnomalyFlagAPI, len(flags))
	for i, f := range flags {
		out[i] = api.AnomalyFlagAPI{
			ID:       f.ID,
			Metric:   f.Metric,
			Scope:    f.Scope,
			Observed: f.Observed,
			Expected: f.Expected,
			Sigma:    f.Sigma,
			TS:       f.TS,
		}
	}
	return out, nil
}

// flagQueryer is the subset of *clickhouse.Store needed by flagHistoryBridge.
// Unexported to keep the package interface narrow; *clickhouse.Store satisfies it
// and tests can inject a stub without requiring a real ClickHouse connection.
type flagQueryer interface {
	QueryFlagHistory(ctx context.Context, from, to time.Time, metric, app, stream string, minSigma float64, limit int, cursor string) ([]anomaly.AnomalyFlagEvent, string, error)
}

// flagHistoryBridge adapts a flagQueryer to the api.FlagHistoryQuerier interface.
// Converts []anomaly.AnomalyFlagEvent → api.FlagHistoryPage, mapping each event's
// NodeID/App/StreamID fields to domain.AlertScope so both the ComputeFlags
// (in-memory) and QueryFlagHistory (persisted) paths serialize scope identically.
// Wraps clickhouse.ErrInvalidCursor as api.ErrBadCursor so handleAnomalies maps
// it to HTTP 400 rather than 500.
// Follows the anomalyDetectorBridge precedent (serve.go:63–88). ADR-0009 §6.
type flagHistoryBridge struct {
	store flagQueryer
}

func (b *flagHistoryBridge) QueryFlagHistory(ctx context.Context, from, to time.Time, metric, app, stream string, minSigma float64, limit int, cursor string) (api.FlagHistoryPage, error) {
	events, nextCursor, err := b.store.QueryFlagHistory(ctx, from, to, metric, app, stream, minSigma, limit, cursor)
	if err != nil {
		if errors.Is(err, clickhouse.ErrInvalidCursor) {
			return api.FlagHistoryPage{}, api.ErrBadCursor
		}
		return api.FlagHistoryPage{}, err
	}
	items := make([]api.AnomalyFlagAPI, len(events))
	for i, ev := range events {
		// Build AlertScope from the stored denormalized columns (NodeID, App, StreamID).
		// This mirrors parseScopeJSON's empty-value convention: absent fields stay "".
		// Both paths (ComputeFlags and QueryFlagHistory) produce the same scope shape.
		items[i] = api.AnomalyFlagAPI{
			ID:     ev.ID,
			Metric: ev.Metric,
			Scope: domain.AlertScope{
				NodeID:   ev.NodeID,
				App:      ev.App,
				StreamID: ev.StreamID,
			},
			Observed: ev.Observed,
			Expected: ev.Expected,
			Sigma:    ev.Sigma,
			TS:       ev.DetectedAt.UnixMilli(),
		}
	}
	return api.FlagHistoryPage{Items: items, NextCursor: nextCursor}, nil
}

// metaIngestTokenStore adapts *meta.Store to the beacon.TokenStore interface.
//
// The beacon transport passes the RAW token from the X-Pulse-Ingest-Token
// header; we forward it to meta.Store.LookupToken which encodes the D-052
// semantics: HMAC-SHA256 lookup first (new tokens), plain SHA-256 fallback
// for legacy rows. We do NOT pre-hash here — all hashing stays in the store.
type metaIngestTokenStore struct {
	store *meta.Store
}

// LookupIngestToken implements beacon.TokenStore.
func (m *metaIngestTokenStore) LookupIngestToken(ctx context.Context, rawToken string) (string, bool, error) {
	tok, err := m.store.LookupToken(ctx, rawToken)
	if err != nil {
		return "", false, err
	}
	if tok == nil || tok.Kind != "ingest" {
		return "", false, nil
	}
	// Enforce token expiry — mirrors the check in api/server.go bearerAuthMiddleware.
	// ExpiresAt is Unix epoch milliseconds; nil means the token never expires.
	if tok.ExpiresAt != nil && *tok.ExpiresAt < time.Now().UnixMilli() {
		return "", false, nil
	}
	return tok.ID, true, nil
}

// server holds all running services for the pulse binary.
type server struct {
	store     *clickhouse.Store
	meta      *meta.Store
	agg       *aggregator.Aggregator
	col       *collector.Collector
	apiServer *api.Server
	alertEval *alert.Evaluator
	lic       *license.Manager
	logger    *slog.Logger

	// Wave 2: data-plane additions.
	sessionStitcher  *sessions.Stitcher
	ingestTracker    *ingest.HealthTracker
	clusterDiscovery *cluster.Discovery
	geoResolver      collector.GeoResolver
	uaParser         collector.UAParser

	// Wave 2: product-plane additions (BE-02).
	beaconServer    *beaconingest.Server // dedicated ingest listener (optional)
	reportScheduler *reports.Scheduler   // report schedule runner (WO-204)

	// Wave 3 (WO-301): F10 synthetic probe runner.
	probeRunner *prober.Runner // wired by BE-02 (WO-302) via ProbeConfigSource

	// Wave 3 (WO-302): F9 anomaly detector.
	anomalyDetector *anomaly.Detector

	// D-087 BUG-011: stale-node eviction threshold derivation.
	// pollInterval is the AMS REST poll interval (default 5 s), stored here so
	// Start() can compute the 3×PollInterval node-eviction threshold without
	// re-reading the config.
	pollInterval time.Duration
}

// newServer constructs all services from config.
func newServer(ctx context.Context, cfg EnvConfig, logger *slog.Logger) (*server, error) {
	// 1. ClickHouse store.
	chCfg := clickhouse.Config{
		DSN:           cfg.ClickHouseDSN,
		Database:      cfg.ClickHouseDatabase,
		BatchSize:     1000,
		FlushInterval: 2 * time.Second,
		MaxRetries:    10,
		RetryDelay:    2 * time.Second,
	}
	store, err := clickhouse.New(ctx, chCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: %w", err)
	}

	// 2. Live aggregator.
	agg := aggregator.New(3*time.Minute, nil, logger)
	// D-031: apply the configured ingest health targets so the dashboard's per-stream
	// health honors PULSE_INGEST_TARGET_BITRATE_KBPS/_FPS (the aggregator previously
	// hardcoded the package defaults, ignoring config).
	agg.SetIngestTargets(cfg.IngestTargetBitrateKbps, cfg.IngestTargetFPS)
	// Wire aggregator back as the eviction sink.
	// (Circular reference is fine — aggregator holds a domain.EventSink, not *Fanout)

	// 3. Fanout: routes events to both ClickHouse store and aggregator.
	fanout := collector.NewFanout(logger, store, agg)

	// 4. AMS REST client.
	amsClient := amsclient.New(amsclient.Config{
		BaseURL:       cfg.AMSBaseURL,
		AuthToken:     cfg.AMSAuthToken,
		LoginEmail:    cfg.AMSLoginEmail,
		LoginPassword: cfg.AMSLoginPassword,
		Timeout:       10 * time.Second,
	})

	// Wave 2: Geo and UA enrichment resolvers.
	var geoResolver collector.GeoResolver
	var uaParser collector.UAParser
	if cfg.GeoMMDBPath != "" {
		geoResolver = collector.NewMMDBGeoResolver(cfg.GeoMMDBPath, cfg.AnonymizeIP, logger)
	} else {
		geoResolver = collector.NoopGeoResolver{}
	}
	uaParser = collector.NewEmbeddedUAParser()

	// Wave 2: Session stitcher (Consumer that stitches viewer sessions).
	sessionStitcher := sessions.New(sessions.Config{
		IdleTimeout: cfg.SessionIdleTimeout,
	}, fanout, logger)

	// Wave 2: Ingest health tracker (Consumer that tracks publisher health).
	ingestTracker := ingest.New(ingest.Config{
		TargetBitrateKbps: cfg.IngestTargetBitrateKbps,
		TargetFPS:         cfg.IngestTargetFPS,
	}, logger)

	// Wire additional consumers into fanout (Wave 2 additions).
	// NOTE: fanout was created above with store + agg. We add session stitcher
	// and ingest tracker here. A production refactor could pass all consumers
	// at fanout creation time; for Wave 2, we re-create the fanout with the
	// full consumer list.
	fanout = collector.NewFanout(logger, store, agg, sessionStitcher, ingestTracker)

	// 5. Sources (populated below; poller added after metaStore is initialized so
	// VodState can be wired — see BUG-002 fix below).
	var sources []collector.Source

	// A5/B1/B7: webhook source wired below after metaStore is available
	// so that per-source HMAC secrets can be loaded from ams_sources. See
	// the block that follows the metaStore migration section.

	// Wave 2: Kafka source (when brokers are configured).
	// D-005 declared edit (BE-02-A): hoisted to function scope so kafkaSource is
	// accessible after apiServer is constructed for SetKafkaStats wiring (VD-27).
	var kafkaSource *kafkasrc.Source
	if len(cfg.KafkaBrokers) > 0 {
		kafkaSource = kafkasrc.New(kafkasrc.Config{
			Brokers:  cfg.KafkaBrokers,
			GroupID:  cfg.KafkaGroupID,
			NodeID:   cfg.AMSNodeID,
			MaxWait:  1 * time.Second,
			MinBytes: 1,
			MaxBytes: 10 << 20,
		}, fanout, logger)
		sources = append(sources, kafkaSource)
		logger.Info("pulse: kafka source configured", "brokers", cfg.KafkaBrokers)
	}

	// Wave 2: Cluster discovery (always enabled; single-node deployments
	// will get one node in the cluster list which is correct).
	clusterDiscovery := cluster.New(cluster.Config{
		PollInterval: cfg.ClusterDiscoveryInterval,
		NodeID:       cfg.AMSNodeID,
	}, amsClient, fanout, logger)
	sources = append(sources, clusterDiscovery)

	// VD-03: Wire cluster discovery into aggregator for origin/edge viewer dedup.
	agg.SetEdgeChecker(clusterDiscovery)

	// 6. Collector supervisor is created after the webhook wiring block below
	// (moved to after metaStore so SourceSecrets can be populated — B7).

	// HOOK(BE-02): Wire license manager.
	lic, err := license.New(os.Getenv("PULSE_LICENSE_KEY"), os.Getenv("PULSE_LICENSE_FILE"))
	if err != nil {
		logger.Warn("license: init failed, using free tier", "error", err)
		// license.New never returns an error on fallback — but guard anyway.
		lic, _ = license.New("", "")
	}
	logger.Info("pulse: license loaded", "tier", lic.Tier(), "valid", lic.Valid())

	// HOOK(BE-02): Wire meta store. Backend selected by PULSE_META (default: sqlite);
	// PULSE_POSTGRES_DSN is a convenience override that sets backend=postgres + DSN
	// in one step without requiring PULSE_META to be set separately.
	metaBackend, metaDSN := resolveMetaBackend(os.Getenv)
	metaSecretKey, err := config.GetSecret("PULSE_SECRET_KEY")
	if err != nil {
		return nil, fmt.Errorf("PULSE_SECRET_KEY: %w", err)
	}
	// Validate key length for non-:memory: DSNs, mirroring internal/config.validate.
	// meta.New silently falls back to a persisted key file when the key is empty, which
	// hides operator misconfiguration. Fail loudly here instead.
	if metaDSN != ":memory:" && len(metaSecretKey) < 16 {
		if metaSecretKey == "" {
			return nil, fmt.Errorf("PULSE_SECRET_KEY must be set (min 16 bytes); generate with: openssl rand -hex 32")
		}
		return nil, fmt.Errorf("PULSE_SECRET_KEY is too short (%d bytes); minimum is 16 bytes; generate with: openssl rand -hex 32", len(metaSecretKey))
	}
	metaStore, err := meta.New(ctx, metaBackend, metaDSN, metaSecretKey)
	if err != nil {
		return nil, fmt.Errorf("meta store: %w", err)
	}
	// Run embedded schema migration from contracts/db/meta/0001_init.sql.
	// In production the migrate command runs this; here we run it idempotently
	// on start for single-binary convenience.
	metaDDLPath := os.Getenv("PULSE_META_DDL_PATH")
	if metaDDLPath != "" {
		if err := metaStore.Migrate(ctx, metaDDLPath); err != nil {
			logger.Warn("meta store: explicit DDL migration failed", "path", metaDDLPath, "error", err)
		}
	}

	// REST poller (always enabled).
	// VD-07: pass geo/UA resolvers so REST-polled events get enrichment.
	// BUG-002: VodState wires the meta store so VoD seen-set is persisted across
	// restarts, preventing SummingMergeTree double-counts on recording_bytes.
	// Placed here (after metaStore) so *meta.Store is available as VodState.
	// *meta.Store satisfies restpoller.VodStateStore structurally via
	// ListSeenVodIDs / MarkVodSeen (server/internal/store/meta/vod_poll_state.go).
	poller := restpoller.New(
		restpoller.Config{
			NodeID:       cfg.AMSNodeID,
			PollInterval: cfg.PollInterval,
			Applications: cfg.AMSApplications,
			GeoResolver:  geoResolver,
			UAParser:     uaParser,
			VodState:     metaStore,
		},
		amsClient,
		fanout,
		logger,
	)
	sources = append(sources, poller)

	// A5/B1/B7: Wire webhook source when PULSE_WEBHOOK_ADDR is set.
	// Placed here (after metaStore) so per-source HMAC secrets can be loaded.
	// B2 fail-closed: refuse to start when the shared secret is absent.
	// B7 limitation: SourceSecrets is loaded once at startup; rotating a
	// per-source secret requires a process restart to take effect.
	if cfg.WebhookListenAddr != "" {
		if cfg.WebhookSharedSecret == "" {
			logger.Error("pulse: webhook listener skipped — PULSE_WEBHOOK_SECRET must be set when PULSE_WEBHOOK_ADDR is configured (fail-closed)")
		} else {
			// B7: build per-source HMAC secret map from meta store.
			sourceSecrets := make(map[string]string)
			if srcs, listErr := metaStore.ListAMSSources(ctx, 0, ""); listErr != nil {
				logger.Warn("pulse: webhook: could not load per-source secrets", "error", listErr)
			} else {
				for _, src := range srcs {
					if src.WebhookSecretEnc.Valid && src.WebhookSecretEnc.String != "" {
						plain, decErr := metaStore.Decrypt(src.WebhookSecretEnc.String)
						if decErr != nil {
							logger.Warn("pulse: webhook: decrypt per-source secret failed, skipping",
								"source", src.Name, "error", decErr)
							continue
						}
						sourceSecrets[src.Name] = plain
					}
				}
			}
			wh := webhooksrc.New(webhooksrc.Config{
				NodeID:        cfg.AMSNodeID,
				SharedSecret:  cfg.WebhookSharedSecret,
				SourceSecrets: sourceSecrets,
				ListenAddr:    cfg.WebhookListenAddr,
			}, fanout, logger)
			sources = append(sources, wh)
			logger.Info("pulse: webhook source configured",
				"addr", cfg.WebhookListenAddr,
				"per_source_secrets", len(sourceSecrets))
		}
	}

	// 6. Collector supervisor (moved after webhook B7 wiring).
	col := collector.New(logger, sources...)

	// HOOK(BE-02): Wire alert channel registry.
	// The registry starts empty. The evaluator rebuilds it from the meta store
	// on every tick (syncRegistryFromStore), so channels created, updated, or
	// deleted via the API are reflected within one tick interval (≤5 s).
	chanRegistry := channels.NewRegistry()

	// HOOK(BE-02): Wire alert evaluator.
	alertEval := alert.New(alert.Config{
		TickInterval: 5 * time.Second,
		BaseURL:      "http://" + cfg.ListenAddr,
	}, agg, metaStore, chanRegistry, nil, logger)

	// HOOK(BE-02): Wire query service.
	qsvc := query.New(agg, store.GetConn(), lic)
	// VD-39: wire cluster discovery so FleetNodes() returns real role (origin/edge)
	// instead of hardcoded "standalone".
	qsvc.SetClusterDiscovery(clusterDiscovery)

	// HOOK(BE-02): Wire API server.
	webDir := os.Getenv("PULSE_WEB_DIR")
	if webDir == "" {
		webDir = "/usr/share/pulse/web" // matches deploy/docker/pulse.Dockerfile
	}

	// S11 WO-C: OIDC provider initialization (nil when OIDC_ISSUER not set).
	var oidcCfg *api.OIDCProviderConfig
	if cfg.OIDCIssuer != "" {
		oidcProvider, err := oidclib.NewProvider(ctx, cfg.OIDCIssuer)
		if err != nil {
			return nil, fmt.Errorf("OIDC provider init: %w", err)
		}
		var groupRoleMap map[string]string
		if cfg.OIDCGroupRoleMap != "" {
			if err := json.Unmarshal([]byte(cfg.OIDCGroupRoleMap), &groupRoleMap); err != nil {
				return nil, fmt.Errorf("PULSE_OIDC_GROUP_ROLE_MAP parse: %w", err)
			}
		}
		oidcCfg = &api.OIDCProviderConfig{
			Issuer:       cfg.OIDCIssuer,
			ClientID:     cfg.OIDCClientID,
			ClientSecret: cfg.OIDCClientSecret,
			RedirectURL:  cfg.OIDCRedirectURL,
			GroupClaim:   cfg.OIDCGroupClaim,
			GroupRoleMap: groupRoleMap,
			DefaultRole:  cfg.OIDCDefaultRole,
			SessionTTL:   cfg.OIDCSessionTTL,
			SecretKey:    metaSecretKey, // reuse meta store key for state-cookie HMAC
			Provider:     oidcProvider,
		}
	}

	apiCfg := api.Config{
		ListenAddr:         cfg.ListenAddr,
		MetricsToken:       cfg.MetricsToken, // Wave 2: PULSE_METRICS_TOKEN gating
		WebDir:             webDir,
		CORSAllowedOrigins: cfg.CORSAllowedOrigins, // A1: PULSE_CORS_ALLOWED_ORIGINS
		AllowedWSOrigins:   cfg.AllowedWSOrigins,   // C2: PULSE_ALLOWED_WS_ORIGINS
		OIDCConfig:         oidcCfg,                // S11 WO-C
		// Explicit env config, not cfg.AMSBaseURL (which defaults to localhost and
		// is therefore never empty). Distinguishes "operator set an AMS URL" from
		// "fresh install on the default" for the onboarding guard.
		AMSEnvConfigured: os.Getenv("PULSE_AMS_URL") != "",
	}
	apiServer := api.New(apiCfg, metaStore, agg, qsvc, lic, logger)
	// Wire ClickHouse connection for /healthz probes (D-W1-002).
	apiServer.SetClickHouseConn(store.GetConn())
	// VD-10: Wire event sink so main-port /ingest/beacon persists events.
	// Without this, beacons POSTed to the main API port are authenticated but
	// silently discarded. The dedicated beacon server (PULSE_INGEST_LISTEN_ADDR)
	// has its own sink; this ensures the default single-port deployment works.
	apiServer.SetEventSink(fanout)
	// VD-23: Wire ingest health tracker so handleIngestHealth can read per-publisher
	// state (health scores, raw metrics) from the correct source.
	apiServer.SetIngestTracker(ingestTracker)
	// VD-27: Wire kafka stats provider for /healthz kafka component.
	// kafkaSource is nil when no brokers are configured; SetKafkaStats is a no-op for nil.
	if kafkaSource != nil {
		apiServer.SetKafkaStats(kafkaSource)
	}

	// Wave 2 (BE-02): Seed default alert rule pack on first run (closes G8).
	// Idempotent — no-op if rules already exist.
	if err := alert.SeedDefaultRulePack(ctx, metaStore, logger); err != nil {
		logger.Warn("pulse: default rule pack seeding failed (non-fatal)", "error", err)
	}

	// Wave 2 (BE-02): Dedicated beacon ingest listener (optional, PULSE_INGEST_LISTEN_ADDR).
	// When set, the beacon ingest endpoint is also available on a separate port
	// for DMZ/edge deployment without exposing the full API.
	var beaconSrv *beaconingest.Server
	if bcfg, ok := beaconListenerConfig(cfg.IngestListenAddr, lic); ok {
		ingestTokenStore := &metaIngestTokenStore{store: metaStore}
		beaconSrv = beaconingest.NewServer(bcfg, ingestTokenStore, fanout, logger)
		logger.Info("pulse: beacon ingest listener configured", "addr", cfg.IngestListenAddr)
	}

	// Wave 2 (BE-02): Cert expiry checker for cert_expiry alert rules.
	// Wired to the evaluator to enable TLS cert monitoring.
	certChecker := alert.NewCertChecker(10 * time.Second)
	alertEval.SetCertChecker(certChecker)
	// S39 (D-101): licence-key expiry checker for license_expiry alert rules —
	// warn through the operator's channels before the key downgrades.
	wireAlertLicenseExpiry(alertEval, licenseExpiryChecker{lic: lic})
	wireAlertQoEReader(alertEval, qsvc)          // D-062: honest rebuffer_ratio/error_rate from ClickHouse rollup_qoe_1h
	wireAlertAnomalyReader(alertEval, metaStore) // S11 WO-B: Welford baseline reader for anomaly rules

	// Wave 2 (BE-02, WO-204): Report accountant, scheduler, and generator.
	// Accountant queries ClickHouse rollup tables for usage figures.
	// Scheduler polls report_schedules and fires artifact generation.
	accountant := reports.NewAccountant(store.GetConn(), metaStore)

	s3Cfg := reports.S3Config{
		Endpoint:        cfg.S3Endpoint,
		Bucket:          cfg.S3Bucket,
		Prefix:          cfg.S3Prefix,
		Region:          cfg.S3Region,
		AccessKeyEnvRef: cfg.S3AccessKeyEnvRef,
		SecretKeyEnvRef: cfg.S3SecretKeyEnvRef,
	}
	// PULSE_REPORT_LOGO_PATH: validate at boot (WARN if set but unreadable, never crash).
	reports.ValidateLogoPath(cfg.ReportLogoPath, logger)

	reportScheduler := reports.NewScheduler(reports.SchedulerConfig{
		ArtifactsDir: cfg.ReportsDir,
		TickInterval: 60 * time.Second,
		S3:           s3Cfg,
		LogoPath:     cfg.ReportLogoPath,
	}, accountant, metaStore, logger)
	// Wire alert history for schedule failure notifications.
	reportScheduler.SetAlertStore(metaStore)
	// Gate scheduled execution by tier — a schedule created while licensed must
	// stop generating (and drop white-label branding) after a downgrade.
	reportScheduler.SetLicense(lic)

	reportGen := &reports.Generator{
		Accountant: accountant,
		Scheduler:  reportScheduler,
	}
	apiServer.SetReportGenerator(reportGen)

	// Wave 3 (WO-302): Wire F10 ProbeConfigSource + probe runner.
	// BE-02 implements MetaProbeConfigSource over the meta probes table.
	probeSource := meta.NewProbeConfigSource(metaStore)
	// EntitlementGate: a tenant that downgrades below the probe tier stops probing
	// at runtime, not just at the HTTP CRUD boundary (S37 / D-108).
	probeRunnerInstance := prober.New(
		prober.Config{Workers: 4, EntitlementGate: lic.CheckProbes},
		probeSource, store, logger, nil)

	// Wire probe result querier into query service for GET /probes/{id}/results.
	qsvc.SetProbeResultQuerier(store)

	// Wave 3 (WO-302): F9 anomaly detector.
	// Reads live snapshots and maintains rolling baselines in anomaly_baselines.
	// PULSE_ANOMALY_TICK_S overrides the default 60s tick interval (e.g. CI uses 5s).
	anomalyTickInterval := time.Duration(cfg.AnomalyTickS) * time.Second
	anomalyDet := anomaly.New(anomaly.Config{TickInterval: anomalyTickInterval}, metaStore, agg, logger)

	// Wire anomaly detector into API server.
	apiServer.SetAnomalyDetector(&anomalyDetectorBridge{det: anomalyDet})

	// BUG-008 Group B (ADR-0009 §4+6): wire flag-event store so the write path
	// persists detections and GET /anomalies ?from/?to serves honest history.
	// store (*clickhouse.Store) satisfies both anomaly.FlagEventStore (write)
	// and flagQueryer (read); guaranteed non-nil (clickhouse.New fails on error above).
	// Run() calls WarmHysteresis internally (anomaly.go:268); the nil-guard at
	// serve.go:610 prevents Run from being called when anomalyDetector is nil.
	anomalyDet.SetFlagStore(store)
	apiServer.SetFlagHistoryQuerier(&flagHistoryBridge{store: store})

	return &server{
		store:            store,
		meta:             metaStore,
		agg:              agg,
		col:              col,
		apiServer:        apiServer,
		alertEval:        alertEval,
		lic:              lic,
		logger:           logger,
		sessionStitcher:  sessionStitcher,
		ingestTracker:    ingestTracker,
		clusterDiscovery: clusterDiscovery,
		geoResolver:      geoResolver,
		uaParser:         uaParser,
		beaconServer:     beaconSrv,
		reportScheduler:  reportScheduler,
		probeRunner:      probeRunnerInstance,
		anomalyDetector:  anomalyDet,
		pollInterval:     cfg.PollInterval,
	}, nil
}

// Start launches background services. Returns an error if any service fails
// to start (not if a transient operational error occurs — those are logged).
func (s *server) Start(ctx context.Context) error {
	// Start ClickHouse flush goroutines. Deliberately NOT the signal-aware ctx:
	// on SIGTERM the flushers must keep running until Close() drains the queued
	// events (graceful drain, D-051). Producers stop via ctx; the store stops
	// via s.store.Close() in Shutdown.
	s.store.Start(context.Background())

	// Start stale-stream eviction loop.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.agg.EvictStale()
			}
		}
	}()

	// BUG-011 fix (D-087): stale-node eviction loop.
	// EvictStaleNodes was implemented (VD-30) but never wired — node_down could
	// never fire in production. threshold = 3×PollInterval (default 15 s);
	// cadence = threshold/2 so evictions fire in the first half-threshold after stale.
	wireNodeEviction(ctx, s.agg, s.pollInterval)

	// Wave 2: Session idle eviction loop (evict every 60s).
	if s.sessionStitcher != nil {
		go func() {
			ticker := time.NewTicker(60 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s.sessionStitcher.EvictIdle()
				}
			}
		}()
	}

	// Wave 2: Ingest health sweep loop (sweep every 5s for F4 budget).
	if s.ingestTracker != nil {
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s.ingestTracker.SweepStale()
				}
			}
		}()
	}

	// Start collector (non-blocking — runs in goroutines).
	go s.col.Run(ctx)

	// HOOK(BE-02): Start alert evaluator.
	s.alertEval.Start(ctx)

	// HOOK(BE-02): Start API server (non-blocking; listener in goroutine).
	if err := s.apiServer.Start(ctx); err != nil {
		return fmt.Errorf("api server: %w", err)
	}

	// Wave 2 (BE-02): Start dedicated beacon ingest listener if configured.
	if s.beaconServer != nil {
		if err := s.beaconServer.Start(); err != nil {
			return fmt.Errorf("beacon ingest server: %w", err)
		}
	}

	// Wave 2 (BE-02, WO-204): Start report scheduler.
	if s.reportScheduler != nil {
		s.reportScheduler.Start(ctx)
	}

	// Wave 3 (WO-301 + WO-302): Start probe runner (ProbeConfigSource is now wired).
	if s.probeRunner != nil {
		go func() {
			if err := s.probeRunner.Run(ctx); err != nil && ctx.Err() == nil {
				s.logger.Error("pulse: probe runner exited unexpectedly", "error", err)
			}
		}()
		s.logger.Info("pulse: probe runner started")
	}

	// Wave 3 (WO-302): Start anomaly baseline update loop (F9).
	if s.anomalyDetector != nil {
		go s.anomalyDetector.Run(ctx)
		s.logger.Info("pulse: anomaly detector started")
	}

	s.logger.Info("pulse: all services started")
	return nil
}

// Stop shuts down all services gracefully.
func (s *server) Stop() {
	// Wave 2 (BE-02, WO-204): Stop report scheduler.
	if s.reportScheduler != nil {
		s.reportScheduler.Stop()
	}
	s.alertEval.Stop()
	// Wave 2 (BE-02): Gracefully stop dedicated beacon ingest listener.
	if s.beaconServer != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.beaconServer.Stop(stopCtx); err != nil {
			s.logger.Warn("pulse: beacon server stop error", "error", err)
		}
	}
	if s.meta != nil {
		s.meta.Close()
	}
	s.store.Close()
	s.logger.Info("pulse: stopped")
}

// ─── Alert evaluator QoE wiring ───────────────────────────────────────────────

// wireAlertQoEReader wires the ClickHouse QoE reader to the alert evaluator so
// rebuffer_ratio and error_rate rules receive honest values from rollup_qoe_1h.
// Without this call the evaluator silently skips QoE rules each tick.
//
// D-062 wiring pin: serve_wiring_test.go calls this function directly; deleting
// it causes the test file to fail to compile (analogous to beaconListenerConfig).
func wireAlertQoEReader(eval *alert.Evaluator, reader alert.QoEReader) {
	if reader != nil {
		eval.SetQoEReader(reader)
	}
}

// wireAlertAnomalyReader wires the meta store as the anomaly baseline reader for
// anomaly alert rules. *meta.Store satisfies alert.AnomalyBaselineReader via
// GetAnomalyBaseline (store/meta/anomaly.go:57). Without this call, anomaly rules
// are skipped each tick with a WARN log.
//
// D-WOB wiring pin: SetAnomalyBaselineReader has the same comment; deleting either
// breaks compilation of any serve_wiring_test.go that references these symbols.
func wireAlertAnomalyReader(eval *alert.Evaluator, store alert.AnomalyBaselineReader) {
	if store != nil {
		eval.SetAnomalyBaselineReader(store)
	}
}

// wireAlertLicenseExpiry wires the licence-key expiry checker to the alert evaluator
// so license_expiry rules receive the real days-until-expiry from *license.Manager.
// Without this call the evaluator silently skips license_expiry rules each tick — the
// operator would never be warned before the key downgrades, and no unit test that
// calls SetLicenseChecker directly would catch the omission.
//
// D-101 wiring pin: serve_wiring_test.go calls this function directly; deleting it
// causes the test file to fail to compile (analogous to wireAlertQoEReader / D-062).
func wireAlertLicenseExpiry(eval *alert.Evaluator, checker alert.LicenseExpiryChecker) {
	if checker != nil {
		eval.SetLicenseChecker(checker)
	}
}

// licenseExpiryChecker adapts *license.Manager to alert.LicenseExpiryChecker for
// license_expiry rules (S39/D-101). A nil expiry (perpetual key or free fallback)
// reports ok=false so the rule is skipped; an already-expired key clamps to 0 days.
type licenseExpiryChecker struct{ lic *license.Manager }

func (c licenseExpiryChecker) DaysUntilExpiry() (float64, bool) {
	if c.lic == nil {
		return 0, false
	}
	exp := c.lic.ExpiresAt()
	if exp == nil {
		return 0, false
	}
	days := time.Until(*exp).Hours() / 24
	if days < 0 {
		days = 0
	}
	return days, true
}

// wireNodeEviction starts the stale-node eviction goroutine (BUG-011 fix, D-087).
//
// threshold = 3×pollInterval: a node must miss three consecutive poll windows before
// it is considered gone (generous enough to survive transient jitter, tight enough
// to catch a frozen AMS within ~15 s at the 5 s default). cadence = threshold/2
// ensures the ticker fires at least twice before the threshold elapses, so the
// first eviction opportunity arrives in the first half-threshold window.
//
// D-087 wiring pin: serve_wiring_test.go calls this function directly; deleting
// it causes the test file to fail to compile (analogous to beaconListenerConfig).
func wireNodeEviction(ctx context.Context, agg *aggregator.Aggregator, pollInterval time.Duration) {
	threshold := nodeEvictionThreshold(pollInterval)
	cadence := threshold / 2
	go func() {
		ticker := time.NewTicker(cadence)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				agg.EvictStaleNodes(threshold)
			}
		}
	}()
}

// nodeEvictionThreshold computes the staleness window for node eviction:
// 3×PollInterval per the aggregator's EvictStaleNodes contract (VD-30) —
// ~15 s freeze-detection at the 5 s default. The multiplier is load-bearing
// for the node_down lead-time claim and is pinned by a direct unit test
// (D-087 verify M8: an unpinned multiplier silently doubles detection time).
func nodeEvictionThreshold(pollInterval time.Duration) time.Duration {
	if pollInterval <= 0 {
		pollInterval = restpoller.DefaultPollInterval
	}
	return 3 * pollInterval
}

// ─── Beacon listener wiring ───────────────────────────────────────────────────

// beaconListenerConfig builds the beaconingest.Config for the optional dedicated
// beacon ingest listener (PULSE_INGEST_LISTEN_ADDR). Returns (cfg, true) when
// listenAddr is non-empty; (zero, false) when no dedicated listener is needed.
//
// VD-15 invariant (D-058): License MUST be non-nil so the listener never operates
// fail-open. When License is nil, beacon.Handler.Handle skips the license gate and
// Free-tier users receive HTTP 202 instead of 403 — the live defect found during
// D-058 staging verify. The caller (newServer) always passes the real *license.Manager.
func beaconListenerConfig(listenAddr string, lic beaconingest.LicenseChecker) (beaconingest.Config, bool) {
	if listenAddr == "" {
		return beaconingest.Config{}, false
	}
	return beaconingest.Config{
		ListenAddr:           listenAddr,
		RateLimitPerTokenRPS: 100,
		RateBurst:            200,
		License:              lic,
	}, true
}
