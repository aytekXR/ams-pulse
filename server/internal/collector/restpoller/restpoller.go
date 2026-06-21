// Package restpoller polls AMS REST API v2 endpoints (broadcasts,
// broadcast-statistics, cluster nodes) and emits normalized events.
// This is the universal-fallback source: it must work against every supported
// AMS version with no server-side configuration (PRD Appendix A.5).
//
// F1 acceptance dependency: poll interval default must surface a new stream on
// the dashboard within 10 seconds of publish. Default interval = 5 s satisfies
// the ≤10 s budget with polling headroom.
package restpoller

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/collector"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// DefaultPollInterval is the default broadcast poll interval.
// 5 s guarantees ≤10 s stream visibility (F1): worst case = two polls.
const DefaultPollInterval = 5 * time.Second

// Config holds restpoller configuration.
type Config struct {
	// NodeID is the AMS node identifier to stamp on all events.
	// Use "standalone" for single-node deployments.
	NodeID string

	// PollInterval is the interval between polls. Default: 5 s.
	PollInterval time.Duration

	// Applications restricts polling to specific AMS apps.
	// Empty slice = poll all applications via ListApplications.
	Applications []string

	// GeoResolver and UAParser are optional enrichment hooks.
	GeoResolver collector.GeoResolver
	UAParser    collector.UAParser
}

// Poller implements collector.Source by polling AMS REST API v2.
type Poller struct {
	cfg    Config
	client *amsclient.Client
	sink   domain.EventSink
	dedup  *collector.Deduplicator
	logger *slog.Logger

	// prevStatus tracks each stream's last known AMS status for transition detection.
	mu         sync.Mutex
	prevStatus map[string]string // key = nodeID+"/"+app+"/"+streamID
}

// New creates a new Poller.
func New(
	cfg Config,
	client *amsclient.Client,
	sink domain.EventSink,
	logger *slog.Logger,
) *Poller {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = DefaultPollInterval
	}
	if cfg.NodeID == "" {
		cfg.NodeID = "standalone"
	}
	if cfg.GeoResolver == nil {
		cfg.GeoResolver = collector.NoopGeoResolver{}
	}
	if cfg.UAParser == nil {
		cfg.UAParser = collector.NoopUAParser{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Poller{
		cfg:        cfg,
		client:     client,
		sink:       sink,
		dedup:      collector.NewDeduplicator(cfg.PollInterval * 2),
		logger:     logger,
		prevStatus: make(map[string]string),
	}
}

// Name implements collector.Source.
func (p *Poller) Name() string {
	return fmt.Sprintf("restpoller(%s)", p.client.BaseURL())
}

// Run implements collector.Source. It polls AMS at cfg.PollInterval until ctx
// is cancelled or a fatal error occurs.
func (p *Poller) Run(ctx context.Context) error {
	p.logger.Info("restpoller: starting", "node_id", p.cfg.NodeID, "interval", p.cfg.PollInterval)
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	// Initial poll immediately so the first broadcast is visible within one
	// interval, not two.
	if err := p.poll(ctx); err != nil {
		p.logger.Warn("restpoller: initial poll error", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := p.poll(ctx); err != nil {
				p.logger.Warn("restpoller: poll error", "error", err)
				// Non-fatal: keep running, supervisor handles persistent failures.
			}
		}
	}
}

// poll performs one full poll cycle.
func (p *Poller) poll(ctx context.Context) error {
	apps, err := p.resolveApps(ctx)
	if err != nil {
		return fmt.Errorf("list applications: %w", err)
	}

	// Poll cluster nodes (best-effort). A standalone AMS returns 404, which
	// ClusterNodes maps to (nil, nil) — no warning. Any OTHER error (500, network,
	// auth) is surfaced so a clustered deployment's node pipeline doesn't go dark
	// silently (D-029v / finding #10).
	if nodes, err := p.client.ClusterNodes(ctx); err == nil {
		for _, n := range nodes {
			ev := collector.NormalizeClusterNode(n)
			ev.NodeID = p.cfg.NodeID // override with our configured ID
			p.sink.WriteServerEvent(ev)
		}
	} else {
		p.logger.Warn("restpoller: cluster nodes poll failed", "error", err)
	}

	for _, app := range apps {
		if err := p.pollApp(ctx, app); err != nil {
			p.logger.Warn("restpoller: app poll error",
				"app", app,
				"error", err,
			)
			// Continue with remaining apps.
		}
	}
	return nil
}

// pollApp polls broadcasts for one AMS application.
func (p *Poller) pollApp(ctx context.Context, app string) error {
	broadcasts, err := p.client.ListBroadcastsPaged(ctx, app)
	if err != nil {
		return fmt.Errorf("list broadcasts: %w", err)
	}

	for _, b := range broadcasts {
		key := p.cfg.NodeID + "/" + app + "/" + b.StreamID

		p.mu.Lock()
		prev := p.prevStatus[key]
		p.prevStatus[key] = b.Status
		p.mu.Unlock()

		events := collector.NormalizeBroadcast(
			b,
			p.cfg.NodeID,
			prev,
			p.cfg.GeoResolver,
			p.cfg.UAParser,
		)

		for _, e := range events {
			if p.dedup.IsDuplicate(e) {
				continue
			}
			p.sink.WriteServerEvent(e)
		}

		// Fetch WebRTC client stats for active streams.
		if b.Status == "broadcasting" && b.WebRTCViewerCount > 0 {
			if stats, err := p.client.WebRTCClientStats(ctx, app, b.StreamID); err == nil {
				for _, s := range stats {
					ev := collector.NormalizeWebRTCStats(s, app, b.StreamID, p.cfg.NodeID)
					if !p.dedup.IsDuplicate(ev) {
						p.sink.WriteServerEvent(ev)
					}
				}
			}
		}
	}

	// Detect streams that disappeared (publish_end transition).
	p.detectEnded(app, broadcasts)
	return nil
}

// detectEnded emits publish_end for streams that were "broadcasting" last poll
// but are no longer in the current broadcast list.
func (p *Poller) detectEnded(app string, current []amsclient.BroadcastDTO) {
	// Keys are scoped per application: nodeID/app/streamID. detectEnded runs once
	// per app and must ONLY consider streams of THIS app — otherwise a broadcasting
	// stream in another app (absent from this app's list) would be falsely "ended",
	// deleting a genuinely-live stream. Real AMS nodes host many apps and can even
	// reuse a streamId across apps (e.g. "test123" in LiveApp and PetarTest2), which
	// a node-only key conflated.
	prefix := p.cfg.NodeID + "/" + app + "/"
	currentIDs := make(map[string]bool, len(current))
	for _, b := range current {
		currentIDs[prefix+b.StreamID] = true
	}

	p.mu.Lock()
	var ended []string
	for key, status := range p.prevStatus {
		if status == "broadcasting" && strings.HasPrefix(key, prefix) && !currentIDs[key] {
			ended = append(ended, key)
		}
	}
	for _, key := range ended {
		delete(p.prevStatus, key)
	}
	p.mu.Unlock()

	for _, key := range ended {
		streamID := strings.TrimPrefix(key, prefix)
		ev := domain.ServerEvent{
			Version:  1,
			Type:     domain.EventStreamPublishEnd,
			TS:       time.Now().UnixMilli(),
			Source:   domain.SourceRestPoll,
			NodeID:   p.cfg.NodeID,
			App:      app,
			StreamID: streamID,
			Data: map[string]any{
				"reason": "disappeared",
			},
		}
		p.sink.WriteServerEvent(ev)
	}
}

// resolveApps returns the apps to poll — either the configured list or all apps.
func (p *Poller) resolveApps(ctx context.Context) ([]string, error) {
	if len(p.cfg.Applications) > 0 {
		return p.cfg.Applications, nil
	}
	return p.client.ListApplications(ctx)
}
