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

	"github.com/aytekXR/ams-pulse/server/internal/collector"
	"github.com/aytekXR/ams-pulse/server/internal/domain"
	"github.com/aytekXR/ams-pulse/server/pkg/amsclient"
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
	// webrtcStatsFailing marks apps whose webrtc-client-stats fetches failed last
	// cycle, so the failure is surfaced ONCE per outage at Warn (visible at the
	// default info level — REVIEW-MP3 N9) instead of only per-stream Debug spam.
	webrtcStatsFailing map[string]bool

	// vodState is the persistent seen-set backend (nil = VoD polling disabled).
	vodState VodStateStore
	// vodTick counts poll() invocations for VoD cadence gating.
	// Single-goroutine invariant: poll() runs only from Run's loop, so no mutex needed.
	vodTick int

	// consecAPIErrors is the count of consecutive SystemStats/ClusterNodes call
	// failures. Resets to 0 on any successful call. Single-goroutine invariant:
	// poll() runs only from Run's loop, same as vodTick. D-087.
	consecAPIErrors int

	// lastClusterNodeIDs is the real node-ID set from the most recent SUCCESSFUL
	// cluster poll. On a later cluster failure the failure-streak event must be
	// keyed to these IDs, not to cfg.NodeID: post-N2 the aggregator knows cluster
	// nodes only by their real IDs, and its D-087 contract drops api_unreachable
	// events for unknown keys ("failure events create nothing"). Stamping the
	// single configured ID (default "standalone") therefore left ConsecAPIErrors
	// pinned at 0 for every real node, so node_degraded could never fire during an
	// AMS outage — the whole point of the streak ladder (REVIEW-MP3-R1).
	// Single-goroutine invariant: same as consecAPIErrors.
	lastClusterNodeIDs []string

	// Poll-loop freshness for /healthz (D-164). Written by Run's goroutine,
	// read concurrently by the API handler, so it has its own lock — never take
	// this and mu together, so no lock-order rule is needed.
	healthMu    sync.RWMutex
	startedAt   time.Time
	lastSuccess time.Time
	lastErr     string
}

// staleAfter is the poll age beyond which /healthz reports the collector
// degraded: three missed intervals, floored at 30 s so the default 5 s cadence
// does not flap the health signal on a single slow response.
func (p *Poller) staleAfter() time.Duration {
	if d := 3 * p.cfg.PollInterval; d > minStaleAfter {
		return d
	}
	return minStaleAfter
}

// minStaleAfter is the floor for staleAfter.
const minStaleAfter = 30 * time.Second

// PollHealth implements domain.CollectorHealth.
func (p *Poller) PollHealth() domain.PollHealthSnapshot {
	p.healthMu.RLock()
	defer p.healthMu.RUnlock()
	return domain.PollHealthSnapshot{
		StartedAt:   p.startedAt,
		LastSuccess: p.lastSuccess,
		LastError:   p.lastErr,
		StaleAfter:  p.staleAfter(),
	}
}

// recordPoll updates the freshness state after one poll attempt.
func (p *Poller) recordPoll(err error) {
	p.healthMu.Lock()
	defer p.healthMu.Unlock()
	if err != nil {
		p.lastErr = err.Error()
		return
	}
	p.lastSuccess = time.Now()
	p.lastErr = ""
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
		cfg:                cfg,
		client:             client,
		sink:               sink,
		dedup:              collector.NewDeduplicator(cfg.PollInterval * 2),
		logger:             logger,
		prevStatus:         make(map[string]string),
		webrtcStatsFailing: make(map[string]bool),
		vodState:           cfg.VodState,
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
	// D-164: StartedAt is the age reference until the first poll succeeds, so a
	// collector that never reaches AMS reports degraded instead of healthy.
	p.healthMu.Lock()
	p.startedAt = time.Now()
	p.healthMu.Unlock()
	if p.vodState == nil {
		p.logger.Info("restpoller: VoD polling disabled (VodState not configured)")
	}
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	// Initial poll immediately so the first broadcast is visible within one
	// interval, not two.
	err := p.poll(ctx)
	p.recordPoll(err)
	if err != nil {
		p.logger.Warn("restpoller: initial poll error", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			err := p.poll(ctx)
			p.recordPoll(err)
			if err != nil {
				p.logger.Warn("restpoller: poll error", "error", err)
				// Non-fatal: keep running, supervisor handles persistent failures.
				// The failure is now also visible on /healthz once the last
				// success ages past staleAfter (D-164).
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

	// Poll cluster nodes (best-effort). ClusterNodes first probes cluster-mode-status;
	// success=false means standalone — returns (nil, nil) without warning. A standalone
	// AMS 3.0.3 returns HTTP 500 (not 404) on the paginated cluster/nodes path, which
	// ClusterNodes defensively maps to (nil, nil) as well. Any OTHER error (network,
	// auth) is surfaced so a clustered deployment's node pipeline doesn't go dark
	// silently (D-029v / finding #10, A-amssource live-probe 2026-07-26).
	//
	// Standalone path: when ClusterNodes returns no error AND zero nodes the AMS
	// node is standalone. Fall back to SystemStats so the fleet node card is
	// populated even without a cluster endpoint (item B).
	//
	// D-087: RTT is measured around the deterministic stats call:
	//   cluster path → ClusterNodes RTT; standalone path → SystemStats RTT.
	// Failures increment p.consecAPIErrors and emit a FAILURE-STREAK event
	// (api_unreachable=true). Successes reset the counter and include the RTT.
	// Per-poll deadline: without it, a hung nodes route (or a proxy that ignores
	// the offset segment) would stall the whole poll loop indefinitely
	// (REVIEW-MP3 N-cluster b). 10 s matches cluster.Discovery's own timeout.
	t0 := time.Now()
	clusterCtx, clusterCancel := context.WithTimeout(ctx, 10*time.Second)
	nodes, clusterErr := p.client.ClusterNodes(clusterCtx)
	clusterCancel()
	clusterRTTMS := float64(time.Since(t0).Microseconds()) / 1000.0

	if clusterErr == nil {
		if len(nodes) > 0 {
			// Cluster path: successful — reset streak, emit each node with RTT.
			p.consecAPIErrors = 0
			// Remember the real IDs so a later cluster failure can key its
			// failure-streak events to nodes the aggregator actually knows
			// (REVIEW-MP3-R1). Rebuilt every successful poll, so nodes that
			// leave the cluster stop receiving streak events.
			p.lastClusterNodeIDs = p.lastClusterNodeIDs[:0]
			for _, n := range nodes {
				// Skip identity-less DTOs. PrimaryID() falls back id → nodeId → ip and
				// returns "" when all three are absent; emitting that key produces a
				// blank phantom node in the fleet view and a "" entry in the streak
				// fan-out set. cluster.Discovery already dedups on this key; the poller
				// had no guard at all (round-4 review F-14).
				if n.PrimaryID() == "" {
					continue
				}
				p.lastClusterNodeIDs = append(p.lastClusterNodeIDs, n.PrimaryID())
			}
			for _, n := range nodes {
				if n.PrimaryID() == "" {
					p.logger.Warn("restpoller: cluster node has no id/nodeId/ip, skipping")
					continue
				}
				// NormalizeClusterNode keys the event to the node's REAL cluster ID
				// (PrimaryID). Never override with p.cfg.NodeID here: stamping every
				// node with the single configured ID collapses an N-node fleet onto
				// one flickering identity in the aggregator and poisons per-node
				// ClickHouse rollups, while cluster.Discovery emits the same nodes
				// under their real IDs (REVIEW-MP3 N2). p.cfg.NodeID remains the
				// identity only for the standalone SystemStats path below, where
				// AMS provides no node ID.
				ev := collector.NormalizeClusterNode(n)
				ev.Data["api_latency_ms"] = clusterRTTMS
				// Emit the live counter (0 after the reset above), never a literal:
				// a hardcoded 0 here would mask a missing reset (D-087 verify M4).
				ev.Data["consec_api_errors"] = float64(p.consecAPIErrors)
				p.sink.WriteServerEvent(ev)
			}
		} else {
			// len(nodes)==0 && err==nil → standalone AMS (404 mapped to nil,nil).
			// Best-effort: log and continue on any SystemStats error.
			// D-087: measure RTT around the standalone stats call specifically.
			//
			// D-179 (review round 6, H-08): PREFER /rest/v2/system-resources. It
			// returns the same identity body as /rest/v2/system-status PLUS the
			// cpu/mem/disk gauges LIM-01 used to say were Kafka-only, PLUS the
			// version — one call replacing two. Older AMS 404s it, in which case
			// SystemResources yields (nil, nil) and we take the historical path.
			// A payload with no gauges is treated as no better than system-status
			// (HasResourceMetrics), so a stripped/proxied response cannot cost us
			// the identity fields.
			//
			// D-181 (review round 7, I-04): each call is timed SEPARATELY and
			// sysRTTMS carries the RTT of the call that actually produced the event.
			// A single window opened before SystemResources and closed after the
			// fallback chain reported up to three round-trips as one "API latency",
			// inflating the metric — and the ams_api_latency_ms anomaly baseline fed
			// from it — on precisely the deployments that take the fallback. The
			// GetVersion call is deliberately outside the window on the fallback
			// path, exactly as it was before D-179.
			var (
				ev       domain.ServerEvent
				haveEv   bool
				sErr     error
				stats    map[string]any
				resFail  bool
				sysRTTMS float64
			)
			t1 := time.Now()
			res, rErr := p.client.SystemResources(ctx)
			resRTTMS := float64(time.Since(t1).Microseconds()) / 1000.0
			switch {
			case rErr != nil:
				// A real error (not 404/405) — remember it, but still try
				// system-status before declaring the node unreachable: the two
				// routes are served by different AMS REST services.
				resFail = true
				p.logger.Debug("restpoller: system-resources poll failed, falling back to system-status", "error", rErr)
			case collector.HasResourceMetrics(res):
				ev = collector.NormalizeSystemResources(res, p.cfg.NodeID, "")
				haveEv = true
				sysRTTMS = resRTTMS
			}
			if !haveEv {
				t2 := time.Now()
				stats, sErr = p.client.SystemStats(ctx)
				statsRTTMS := float64(time.Since(t2).Microseconds()) / 1000.0
				if sErr == nil {
					sysRTTMS = statsRTTMS
					// GetVersion is best-effort: version="" on error (older AMS
					// without /rest/v2/version). Outside the RTT window on purpose.
					versionName := ""
					if vDTO, vErr := p.client.GetVersion(ctx); vErr == nil && vDTO != nil {
						versionName = vDTO.VersionName
					}
					ev = collector.NormalizeSystemStats(stats, p.cfg.NodeID, versionName)
					haveEv = true
				} else if resFail {
					// Both routes failed — surface the resources error too, so an
					// AMS that breaks only the console service is diagnosable.
					p.logger.Warn("restpoller: system-resources poll also failed", "error", rErr)
				}
			}

			if haveEv {
				// Standalone success: reset streak, emit with RTT.
				p.consecAPIErrors = 0
				// AMS is answering as standalone now, so any remembered cluster
				// IDs are stale — drop them rather than address failure-streak
				// events to nodes that no longer exist (REVIEW-MP3-R1).
				p.lastClusterNodeIDs = p.lastClusterNodeIDs[:0]
				ev.Data["api_latency_ms"] = sysRTTMS
				// Live counter, not a literal — see the cluster-path note (D-087 M4).
				ev.Data["consec_api_errors"] = float64(p.consecAPIErrors)
				p.sink.WriteServerEvent(ev)
			} else {
				// Standalone failure: BOTH /rest/v2/system-resources and
				// /rest/v2/system-status failed. Increment streak, emit
				// FAILURE-STREAK event. Standalone identity is cfg.NodeID — AMS
				// provides no node ID on this path, and both normalizers key their
				// success events the same way, so the aggregator knows this node.
				p.logger.Warn("restpoller: system stats poll failed", "error", sErr)
				p.consecAPIErrors++
				p.sink.WriteServerEvent(p.failureStreakEvent(p.cfg.NodeID))
			}
		}
	} else {
		// Cluster failure (non-404 error): increment streak, emit FAILURE-STREAK events.
		p.logger.Warn("restpoller: cluster nodes poll failed", "error", clusterErr)
		p.consecAPIErrors++
		p.emitFailureStreak()
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

// failureStreakEvent builds a FAILURE-STREAK EventNodeStats event for the current
// node. Called when SystemStats or ClusterNodes fails. Per the ORCH design ruling:
//   - api_unreachable=true marks this as a failure event (not a metrics update).
//   - consec_api_errors carries the (already-incremented) counter.
//   - api_latency_ms is deliberately ABSENT (key-absent = not measured, D-075 semantics).
//
// The aggregator's onNodeStats handles api_unreachable=true events in-place
// (updates ConsecAPIErrors only; does NOT refresh LastSeenAt). D-087.
//
// nodeID must be an identity the aggregator already knows: its D-087 contract
// drops api_unreachable events for unknown keys, so an event addressed to a
// node that never reported is silently a no-op (REVIEW-MP3-R1).
func (p *Poller) failureStreakEvent(nodeID string) domain.ServerEvent {
	return domain.ServerEvent{
		Version: 1,
		Type:    domain.EventNodeStats,
		TS:      time.Now().UnixMilli(),
		Source:  domain.SourceRestPoll,
		NodeID:  nodeID,
		Data: map[string]any{
			"api_unreachable":   true,
			"consec_api_errors": float64(p.consecAPIErrors),
		},
	}
}

// emitFailureStreak emits the FAILURE-STREAK event(s) for a failed CLUSTER poll.
//
// One event per node seen in the last successful cluster poll: the AMS API being
// unreachable is a fleet-wide condition, and every real node's ConsecAPIErrors
// must advance so the node_degraded ladder (rung 3 of D-087) fires during the
// outage. Before REVIEW-MP3-R1 this emitted a single event keyed to cfg.NodeID —
// an identity no cluster node carries post-N2, so the aggregator dropped it and
// the ladder stayed dead for the whole outage.
//
// Fallback to cfg.NodeID when nothing has been discovered yet (first poll fails,
// or a cluster that has never answered): the event is then a no-op at the
// aggregator, exactly as before, but /healthz and the logs still carry the failure.
func (p *Poller) emitFailureStreak() {
	if len(p.lastClusterNodeIDs) == 0 {
		p.sink.WriteServerEvent(p.failureStreakEvent(p.cfg.NodeID))
		return
	}
	for _, nodeID := range p.lastClusterNodeIDs {
		p.sink.WriteServerEvent(p.failureStreakEvent(nodeID))
	}
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
			Version: 1,
			Type:    domain.EventRecordingReady,
			TS:      ts,
			Source:  domain.SourceRestPoll,
			// Stream/VoD events are keyed to the CONFIGURED node ID, while
			// node_stats on a cluster carry AMS's real per-node IDs (N2). On a
			// cluster the two therefore disagree, so filtering streams by a node
			// ID taken from the Fleet view returns nothing (REVIEW-MP3-R15,
			// disclosed as LIM-28). Fixing it needs the owning node threaded
			// through per-app polling — deferred until the cluster path is
			// live-validated (LIM-10), rather than guessed at now.
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

	webrtcStatsFailures := 0
	var lastWebRTCStatsErr error

	for _, b := range broadcasts {
		key := p.cfg.NodeID + "/" + app + "/" + b.StreamID

		p.mu.Lock()
		prev := p.prevStatus[key]
		p.prevStatus[key] = b.Status
		p.mu.Unlock()

		events := collector.NormalizeBroadcast(
			b,
			// Stream events carry the CONFIGURED node ID, while cluster node_stats
			// carry AMS's real per-node IDs — the two disagree on a cluster
			// (REVIEW-MP3-R15, disclosed as LIM-28). Threading the owning node
			// through per-app polling is deferred until a live cluster exists.
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
			stats, wsErr := p.client.WebRTCClientStats(ctx, app, b.StreamID)
			if wsErr != nil {
				webrtcStatsFailures++
				lastWebRTCStatsErr = wsErr
				// Per-stream detail stays at Debug; the once-per-outage Warn
				// below keeps the failure visible at the default info level
				// without per-stream spam (REVIEW-MP3 N9).
				p.logger.Debug("restpoller: webrtc client stats fetch failed",
					"app", app,
					"stream", b.StreamID,
					"error", wsErr,
				)
			} else {
				for _, s := range stats {
					ev := collector.NormalizeWebRTCStats(s, app, b.StreamID, p.cfg.NodeID)
					if !p.dedup.IsDuplicate(ev) {
						p.sink.WriteServerEvent(ev)
					}
				}
			}
		}
	}

	// Surface webrtc-stats failures once per outage at Warn; reset on a clean
	// cycle so the next outage warns again.
	p.mu.Lock()
	wasFailing := p.webrtcStatsFailing[app]
	p.webrtcStatsFailing[app] = webrtcStatsFailures > 0
	p.mu.Unlock()
	if webrtcStatsFailures > 0 && !wasFailing {
		p.logger.Warn("restpoller: webrtc client stats fetches failing (per-stream detail at debug level)",
			"app", app,
			"failed_this_cycle", webrtcStatsFailures,
			"error", lastWebRTCStatsErr,
		)
	} else if webrtcStatsFailures == 0 && wasFailing {
		p.logger.Info("restpoller: webrtc client stats fetches recovered", "app", app)
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

	// Evict ALL of this app's disappeared streams from prevStatus, but emit
	// publish_end only for those that were "broadcasting". Gating the eviction on
	// "broadcasting" (the old bug) leaked every idle/created stream that was seen
	// once and then removed from AMS — prevStatus grew without bound. Decouple the
	// two: `stale` drives eviction (any status), `ended` drives event emission.
	p.mu.Lock()
	var ended []string
	var stale []string
	for key, status := range p.prevStatus {
		if !strings.HasPrefix(key, prefix) || currentIDs[key] {
			continue // another app's key, or still present this poll
		}
		stale = append(stale, key)
		if status == "broadcasting" {
			ended = append(ended, key)
		}
	}
	for _, key := range stale {
		delete(p.prevStatus, key)
	}
	p.mu.Unlock()

	for _, key := range ended {
		streamID := strings.TrimPrefix(key, prefix)
		ev := domain.ServerEvent{
			Version: 1,
			Type:    domain.EventStreamPublishEnd,
			TS:      time.Now().UnixMilli(),
			Source:  domain.SourceRestPoll,
			// CONFIGURED node ID, not the real cluster node ID — see LIM-28
			// (REVIEW-MP3-R15). Same deferral as the stream-event site above.
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
