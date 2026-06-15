package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

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
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/prober"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/reports"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

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

// metaIngestTokenStore adapts meta.Store to the beacon.TokenStore interface.
// beacon.TokenStore requires GetIngestTokenByHash; meta.Store exposes GetTokenByHash.
type metaIngestTokenStore struct {
	store *meta.Store
}

func (m *metaIngestTokenStore) GetIngestTokenByHash(ctx context.Context, hash string) (string, bool, error) {
	tok, err := m.store.GetTokenByHash(ctx, hash)
	if err != nil {
		return "", false, err
	}
	if tok == nil || tok.Kind != "ingest" {
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
	beaconServer    *beaconingest.Server  // dedicated ingest listener (optional)
	reportScheduler *reports.Scheduler   // report schedule runner (WO-204)

	// Wave 3 (WO-301): F10 synthetic probe runner.
	probeRunner *prober.Runner // wired by BE-02 (WO-302) via ProbeConfigSource

	// Wave 3 (WO-302): F9 anomaly detector.
	anomalyDetector *anomaly.Detector
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
	// Wire aggregator back as the eviction sink.
	// (Circular reference is fine — aggregator holds a domain.EventSink, not *Fanout)

	// 3. Fanout: routes events to both ClickHouse store and aggregator.
	fanout := collector.NewFanout(logger, store, agg)

	// 4. AMS REST client.
	amsClient := amsclient.New(amsclient.Config{
		BaseURL:   cfg.AMSBaseURL,
		AuthToken: cfg.AMSAuthToken,
		Timeout:   10 * time.Second,
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

	// 5. Sources.
	var sources []collector.Source

	// REST poller (always enabled).
	// VD-07: pass geo/UA resolvers so REST-polled events get enrichment.
	poller := restpoller.New(
		restpoller.Config{
			NodeID:       cfg.AMSNodeID,
			PollInterval: cfg.PollInterval,
			Applications: cfg.AMSApplications,
			GeoResolver:  geoResolver,
			UAParser:     uaParser,
		},
		amsClient,
		fanout,
		logger,
	)
	sources = append(sources, poller)

	// HOOK(BE-02/Wave2): logtail and webhook sources are wired here when
	// their config is provided. They are fully implemented; just need config.
	// if cfg.LogTailPath != "" {
	//     tailer := logtail.New(logtail.Config{...}, fanout, logger)
	//     sources = append(sources, tailer)
	// }
	// if cfg.WebhookListenAddr != "" {
	//     wh := webhook.New(webhook.Config{...}, fanout, logger)
	//     sources = append(sources, wh)
	// }

	// Wave 2: Kafka source (when brokers are configured).
	if len(cfg.KafkaBrokers) > 0 {
		kafkaSource := kafkasrc.New(kafkasrc.Config{
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

	// 6. Collector supervisor.
	col := collector.New(logger, sources...)

	// HOOK(BE-02): Wire license manager.
	lic, err := license.New(os.Getenv("PULSE_LICENSE_KEY"), os.Getenv("PULSE_LICENSE_FILE"))
	if err != nil {
		logger.Warn("license: init failed, using free tier", "error", err)
		// license.New never returns an error on fallback — but guard anyway.
		lic, _ = license.New("", "")
	}
	logger.Info("pulse: license loaded", "tier", lic.Tier(), "valid", lic.Valid())

	// HOOK(BE-02): Wire meta store (SQLite).
	metaDSN := os.Getenv("PULSE_META_DSN")
	if metaDSN == "" {
		metaDSN = "pulse_meta.db" // default: file in working directory
	}
	metaSecretKey := os.Getenv("PULSE_SECRET_KEY")
	metaStore, err := meta.New(ctx, "sqlite", metaDSN, metaSecretKey)
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

	// HOOK(BE-02): Wire alert channel registry.
	chanRegistry := channels.NewRegistry()
	// Built-in channels are registered when alert channel configs are loaded
	// from the meta store at startup (handled inside api.Server.Start).

	// HOOK(BE-02): Wire alert evaluator.
	alertEval := alert.New(alert.Config{
		TickInterval: 5 * time.Second,
		BaseURL:      "http://" + cfg.ListenAddr,
	}, agg, metaStore, chanRegistry, nil, logger)

	// HOOK(BE-02): Wire query service.
	qsvc := query.New(agg, store.GetConn(), lic)

	// HOOK(BE-02): Wire API server.
	apiCfg := api.Config{
		ListenAddr:   cfg.ListenAddr,
		MetricsToken: cfg.MetricsToken, // Wave 2: PULSE_METRICS_TOKEN gating
	}
	apiServer := api.New(apiCfg, metaStore, agg, qsvc, lic, logger)
	// Wire ClickHouse connection for /healthz probes (D-W1-002).
	apiServer.SetClickHouseConn(store.GetConn())
	// VD-10: Wire event sink so main-port /ingest/beacon persists events.
	// Without this, beacons POSTed to the main API port are authenticated but
	// silently discarded. The dedicated beacon server (PULSE_INGEST_LISTEN_ADDR)
	// has its own sink; this ensures the default single-port deployment works.
	apiServer.SetEventSink(fanout)

	// Wave 2 (BE-02): Seed default alert rule pack on first run (closes G8).
	// Idempotent — no-op if rules already exist.
	if err := alert.SeedDefaultRulePack(ctx, metaStore, logger); err != nil {
		logger.Warn("pulse: default rule pack seeding failed (non-fatal)", "error", err)
	}

	// Wave 2 (BE-02): Dedicated beacon ingest listener (optional, PULSE_INGEST_LISTEN_ADDR).
	// When set, the beacon ingest endpoint is also available on a separate port
	// for DMZ/edge deployment without exposing the full API.
	var beaconSrv *beaconingest.Server
	if cfg.IngestListenAddr != "" {
		ingestTokenStore := &metaIngestTokenStore{store: metaStore}
		beaconSrv = beaconingest.NewServer(beaconingest.Config{
			ListenAddr:           cfg.IngestListenAddr,
			RateLimitPerTokenRPS: 100,
			RateBurst:            200,
		}, ingestTokenStore, fanout, logger)
		logger.Info("pulse: beacon ingest listener configured", "addr", cfg.IngestListenAddr)
	}

	// Wave 2 (BE-02): Cert expiry checker for cert_expiry alert rules.
	// Wired to the evaluator to enable TLS cert monitoring.
	certChecker := alert.NewCertChecker(10 * time.Second)
	alertEval.SetCertChecker(certChecker)

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
	reportScheduler := reports.NewScheduler(reports.SchedulerConfig{
		ArtifactsDir: cfg.ReportsDir,
		TickInterval: 60 * time.Second,
		S3:           s3Cfg,
	}, accountant, metaStore, logger)
	// Wire alert history for schedule failure notifications.
	reportScheduler.SetAlertStore(metaStore)

	reportGen := &reports.Generator{
		Accountant: accountant,
		Scheduler:  reportScheduler,
	}
	apiServer.SetReportGenerator(reportGen)

	// Wave 3 (WO-302): Wire F10 ProbeConfigSource + probe runner.
	// BE-02 implements MetaProbeConfigSource over the meta probes table.
	probeSource := meta.NewProbeConfigSource(metaStore)
	probeRunnerInstance := prober.New(prober.Config{Workers: 4}, probeSource, store, logger, nil)

	// Wire probe result querier into query service for GET /probes/{id}/results.
	qsvc.SetProbeResultQuerier(store)

	// Wave 3 (WO-302): F9 anomaly detector.
	// Reads live snapshots and maintains rolling baselines in anomaly_baselines.
	anomalyDet := anomaly.New(anomaly.Config{}, metaStore, agg, logger)

	// Wire anomaly detector into API server.
	apiServer.SetAnomalyDetector(&anomalyDetectorBridge{det: anomalyDet})

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
	}, nil
}

// Start launches background services. Returns an error if any service fails
// to start (not if a transient operational error occurs — those are logged).
func (s *server) Start(ctx context.Context) error {
	// Start ClickHouse flush goroutines.
	s.store.Start(ctx)

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
