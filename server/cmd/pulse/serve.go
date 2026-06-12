package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/alert/channels"
	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/collector"
	"github.com/pulse-analytics/pulse/server/internal/collector/aggregator"
	"github.com/pulse-analytics/pulse/server/internal/collector/restpoller"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/clickhouse"
	chstore "github.com/pulse-analytics/pulse/server/internal/store/clickhouse"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// server holds all running services for the pulse binary.
type server struct {
	store     *chstore.Store
	meta      *meta.Store
	agg       *aggregator.Aggregator
	col       *collector.Collector
	apiServer *api.Server
	alertEval *alert.Evaluator
	lic       *license.Manager
	logger    *slog.Logger
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

	// 5. Sources.
	var sources []collector.Source

	// REST poller (always enabled).
	poller := restpoller.New(
		restpoller.Config{
			NodeID:       cfg.AMSNodeID,
			PollInterval: cfg.PollInterval,
			Applications: cfg.AMSApplications,
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
		ListenAddr: cfg.ListenAddr,
	}
	apiServer := api.New(apiCfg, metaStore, agg, qsvc, lic, logger)

	return &server{
		store:     store,
		meta:      metaStore,
		agg:       agg,
		col:       col,
		apiServer: apiServer,
		alertEval: alertEval,
		lic:       lic,
		logger:    logger,
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

	// Start collector (non-blocking — runs in goroutines).
	go s.col.Run(ctx)

	// HOOK(BE-02): Start alert evaluator.
	s.alertEval.Start(ctx)

	// HOOK(BE-02): Start API server (non-blocking; listener in goroutine).
	if err := s.apiServer.Start(ctx); err != nil {
		return fmt.Errorf("api server: %w", err)
	}

	s.logger.Info("pulse: all services started")
	return nil
}

// Stop shuts down all services gracefully.
func (s *server) Stop() {
	s.alertEval.Stop()
	if s.meta != nil {
		s.meta.Close()
	}
	s.store.Close()
	s.logger.Info("pulse: stopped")
}
