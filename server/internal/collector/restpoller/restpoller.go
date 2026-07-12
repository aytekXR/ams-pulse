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
	"sort"
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

// vodPollEveryNTicks defines how often VoD polling fires relative to the broadcast
// poll cadence. At the 5 s default: 12 ticks × 5 s = 60 s VoD poll interval.
// No new env var — adjust PULSE_POLL_INTERVAL if a shorter cadence is needed.
const vodPollEveryNTicks = 12

// VodStateStore persists the per-app seen-set for VoD deduplication across Pulse
// restarts. *meta.Store satisfies this interface structurally (vod_poll_state.go).
type VodStateStore interface {
	// ListSeenVodIDs returns the set of VoD IDs already marked seen for app.
	// Returns a non-nil empty map (not an error) when the app has no entries yet.
	ListSeenVodIDs(ctx context.Context, app string) (map[string]struct{}, error)
	// MarkVodSeen records (app, vodID) as seen. Idempotent (ON CONFLICT DO NOTHING).
	// createdMS is the VoD creation timestamp in Unix epoch milliseconds.
	MarkVodSeen(ctx context.Context, app, vodID string, createdMS int64) error
}

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

	// VodState is the persistent seen-set backend for VoD deduplication.
	// nil disables VoD polling (logged once at Run start). *meta.Store satisfies
	// this interface structurally via ListSeenVodIDs / MarkVodSeen.
	VodState VodStateStore
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

	// vodState is the persistent seen-set backend (nil = VoD polling disabled).
	vodState VodStateStore
	// vodTick counts poll() invocations for VoD cadence gating.
	// Single-goroutine invariant: poll() runs only from Run's loop, so no mutex needed.
	vodTick int
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
		vodState:   cfg.VodState,
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
	if p.vodState == nil {
		p.logger.Info("restpoller: VoD polling disabled (VodState not configured)")
	}
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

	// VoD cadence gate (Option A): fire every vodPollEveryNTicks ticks.
	// Check BEFORE increment so tick 0 fires immediately (backfill on startup).
	vodDue := p.vodState != nil && p.vodTick%vodPollEveryNTicks == 0
	p.vodTick++

	// Poll cluster nodes (best-effort). A standalone AMS returns 404, which
	// ClusterNodes maps to (nil, nil) — no warning. Any OTHER error (500, network,
	// auth) is surfaced so a clustered deployment's node pipeline doesn't go dark
	// silently (D-029v / finding #10).
	//
	// Standalone path: when ClusterNodes returns no error AND zero nodes the AMS
	// node is standalone. Fall back to SystemStats so the fleet node card is
	// populated even without a cluster endpoint (item B).
	if nodes, err := p.client.ClusterNodes(ctx); err == nil {
		if len(nodes) > 0 {
			for _, n := range nodes {
				ev := collector.NormalizeClusterNode(n)
				ev.NodeID = p.cfg.NodeID // override with our configured ID
				p.sink.WriteServerEvent(ev)
			}
		} else {
			// len(nodes)==0 && err==nil → standalone AMS (404 mapped to nil,nil).
			// Best-effort: log and continue on any SystemStats error.
			if stats, sErr := p.client.SystemStats(ctx); sErr == nil {
				// GetVersion is best-effort: version="" on error (older AMS without /rest/v2/version).
				versionName := ""
				if vDTO, vErr := p.client.GetVersion(ctx); vErr == nil && vDTO != nil {
					versionName = vDTO.VersionName
				}
				p.sink.WriteServerEvent(collector.NormalizeSystemStats(stats, p.cfg.NodeID, versionName))
			} else {
				p.logger.Warn("restpoller: system stats poll failed", "error", sErr)
			}
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

	// VoD polling runs after broadcast work, once per vodPollEveryNTicks ticks.
	if vodDue {
		for _, app := range apps {
			if err := p.pollVods(ctx, app); err != nil {
				p.logger.Warn("restpoller: vod poll error",
					"app", app,
					"error", err,
				)
				// Continue with remaining apps.
			}
		}
	}

	return nil
}

// pollVods polls the AMS vods/list endpoint and emits EventRecordingReady for each
// VoD not yet recorded in the persistent seen-set.
//
// At-most-once semantics: MarkVodSeen is called BEFORE emitting the event. A mark
// failure aborts the cycle immediately — better to miss one cycle than to double-emit
// (SummingMergeTree would double-count recording_bytes on the next restart).
//
// Events are emitted DIRECTLY via p.sink.WriteServerEvent — NEVER through p.dedup.
// The Deduplicator would silently drop same-window recording events that share a
// StreamID (common during backfill when multiple VoDs originate from the same stream),
// causing missed recording_ready events. The seen-set in VodState is the correct
// dedup mechanism for VoDs.
func (p *Poller) pollVods(ctx context.Context, app string) error {
	vods, err := p.client.ListVodsPaged(ctx, app)
	if err != nil {
		return fmt.Errorf("list vods: %w", err)
	}

	// Never fall back to empty-seen on error — that would mass double-emit.
	seen, err := p.vodState.ListSeenVodIDs(ctx, app)
	if err != nil {
		return fmt.Errorf("list seen vod ids: %w", err)
	}

	// Collect unseen VoDs; skip entries with empty VodID (no stable dedup key — emit
	// would be unsafe because the next cycle could emit the same file again).
	var unseen []amsclient.VodDTO
	for _, v := range vods {
		if v.VodID == "" {
			p.logger.Warn("restpoller: skipping VoD with empty vodId — no stable dedup key, cannot emit safely",
				"app", app,
				"vod_name", v.VodName,
			)
			continue
		}
		if _, ok := seen[v.VodID]; !ok {
			unseen = append(unseen, v)
		}
	}

	// Sort ascending by CreationDate so older VoDs are ingested before newer ones.
	sort.Slice(unseen, func(i, j int) bool {
		return unseen[i].CreationDate < unseen[j].CreationDate
	})

	if len(unseen) > 1000 {
		p.logger.Warn("restpoller: large VoD backfill — may approach ClickHouse channel capacity (~2000)",
			"app", app,
			"count", len(unseen),
		)
	}

	var emitted int
	for _, v := range unseen {
		// At-most-once ruling: mark FIRST, then emit.
		// A mark failure must not lead to double emission on the next cycle — abort.
		if err := p.vodState.MarkVodSeen(ctx, app, v.VodID, v.CreationDate); err != nil {
			p.logger.Error("restpoller: MarkVodSeen failed — aborting VoD cycle to prevent double-emit",
				"app", app,
				"vod_id", v.VodID,
				"error", err,
			)
			return err
		}

		ts := v.CreationDate
		if ts == 0 {
			ts = time.Now().UnixMilli()
		}

		data := map[string]any{
			"path":       v.VodName,
			"size_bytes": v.FileSize,
		}
		// Duration from AMS vods/list is in MILLISECONDS; convert to seconds.
		// Omit duration_s entirely when AMS does not report it (zero duration).
		if v.Duration > 0 {
			data["duration_s"] = v.Duration / 1000
		}

		ev := domain.ServerEvent{
			Version:  1,
			Type:     domain.EventRecordingReady,
			TS:       ts,
			Source:   domain.SourceRestPoll,
			NodeID:   p.cfg.NodeID,
			App:      app,
			StreamID: v.StreamID,
			Data:     data,
		}
		// Emit DIRECTLY via p.sink.WriteServerEvent — NEVER through p.dedup.
		// The Deduplicator would silently drop same-window recording events sharing
		// the same StreamID and TS, causing missed VoD events during backfill.
		p.sink.WriteServerEvent(ev)
		emitted++
	}

	if emitted > 0 {
		p.logger.Info("restpoller: VoD events emitted", "app", app, "count", emitted)
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
