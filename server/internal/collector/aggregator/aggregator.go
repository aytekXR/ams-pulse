// Package aggregator implements the in-memory live state aggregator.
//
// It consumes domain.ServerEvents and maintains a LiveSnapshot consumed by
// BE-02's API/WebSocket push and alert evaluator via the domain.LiveProvider
// interface.
//
// Eviction: streams with no stats update for staleThreshold intervals
// transition to offline and emit a stream_publish_end event.
//
// Edge dedup (VD-03 / F1+F7 AC):
// In origin+edge cluster deployments, origin nodes report viewer_count that
// already includes edge-forwarded viewers — summing across all nodes would
// double-count. The aggregator consults an optional EdgeStreamChecker.
// When IsEdgeStream(streamID) is true and the reporting node's role is "origin",
// the viewer_count from that node is skipped for TotalViewers aggregation.
//
// Incremental snapshot (S10/D-068):
// rebuildSnapshot (O(N) full scan) has been replaced by O(1)-per-event incremental
// maintenance. Each event handler calls snapRemoveStream / snapAddStream to apply
// only the delta to the in-flight snapshot. rebuildSnapshot is kept for the three
// rare, non-hot paths: New(), EvictStale(), EvictStaleNodes(). Subscriber
// notification is leading-edge rate-limited to ≤1 push/second per event burst;
// max staleness is ~1 s while events keep arriving (in production the restpoller
// emits every ≤5 s, so the practical bound is one poll interval). A trailing
// dirty update after a burst followed by total event silence is only flushed by
// the next EvictStale/EvictStaleNodes tick — acceptable because no events means
// nothing user-visible changed except staleness eviction, which flushes.
package aggregator

import (
	"log/slog"
	"sync"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/collector/ingest"
	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// EdgeStreamChecker is satisfied by *cluster.Discovery (and test doubles).
// The aggregator uses it to avoid double-counting viewer counts in
// origin+edge cluster deployments (VD-03, F1+F7 acceptance criterion).
type EdgeStreamChecker interface {
	// IsEdgeStream returns true when the stream is served by at least one edge node.
	IsEdgeStream(streamID string) bool
	// NodeRole returns the role of a node ("origin" | "edge" | "").
	NodeRole(nodeID string) string
}

const (
	// DefaultStaleThreshold is how long to keep a stream in the live map
	// without an update before emitting an offline transition event.
	DefaultStaleThreshold = 3 * time.Minute

	// notifyRateLimit is the minimum interval between subscriber pushes when
	// events arrive continuously (leading-edge coalescing window).
	// Max staleness under continuous load: ~notifyRateLimit.
	notifyRateLimit = time.Second
)

// Aggregator implements both domain.LiveProvider and collector.Consumer.
// It is goroutine-safe.
type Aggregator struct {
	mu       sync.RWMutex
	streams  map[string]*domain.LiveStream    // key = nodeID+"/"+app+"/"+streamID
	nodes    map[string]*domain.LiveNodeStats // key = nodeID
	snapshot *domain.LiveSnapshot

	staleThreshold time.Duration
	sink           domain.EventSink // for eviction events (may be nil)
	// edgeChecker enables origin/edge viewer-count dedup (VD-03).
	// May be nil (standalone deployments); in that case no dedup occurs.
	edgeChecker EdgeStreamChecker
	subs        map[chan *domain.LiveSnapshot]struct{}
	logger      *slog.Logger

	// Ingest health-score targets (D-031). Default to the package defaults; the
	// configured PULSE_INGEST_TARGET_* values are applied via SetIngestTargets so
	// the dashboard's per-stream health honors the operator's source profile
	// (previously onIngestStats hardcoded the defaults, ignoring config).
	targetBitrateKbps float64
	targetFPS         float64

	// S10/D-068: incremental notify rate-limiting.
	// Leading-edge: the first event after a ≥1 s gap fires immediately; events
	// within the window set snapDirty so the next tick or the next out-of-window
	// event delivers a consolidated push. Max staleness: ~1 s.
	lastNotifyAt time.Time
	snapDirty    bool
}

// New creates an Aggregator.
// sink is used to emit stale-stream publish_end events; may be nil.
func New(staleThreshold time.Duration, sink domain.EventSink, logger *slog.Logger) *Aggregator {
	if staleThreshold == 0 {
		staleThreshold = DefaultStaleThreshold
	}
	if logger == nil {
		logger = slog.Default()
	}
	a := &Aggregator{
		streams:           make(map[string]*domain.LiveStream),
		nodes:             make(map[string]*domain.LiveNodeStats),
		staleThreshold:    staleThreshold,
		sink:              sink,
		subs:              make(map[chan *domain.LiveSnapshot]struct{}),
		logger:            logger,
		targetBitrateKbps: ingest.DefaultTargetBitrateKbps,
		targetFPS:         ingest.DefaultTargetFPS,
	}
	a.rebuildSnapshot()
	return a
}

// SetIngestTargets overrides the ingest health-score targets from configuration
// (PULSE_INGEST_TARGET_BITRATE_KBPS / PULSE_INGEST_TARGET_FPS). Call once after
// New, before events flow. Zero values are ignored (keep the defaults). D-031.
func (a *Aggregator) SetIngestTargets(bitrateKbps, fps float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if bitrateKbps > 0 {
		a.targetBitrateKbps = bitrateKbps
	}
	if fps > 0 {
		a.targetFPS = fps
	}
}

// SetEdgeChecker wires the cluster discovery service for origin/edge viewer dedup.
// Call from serve.go after cluster.Discovery is created (VD-03).
func (a *Aggregator) SetEdgeChecker(c EdgeStreamChecker) {
	a.mu.Lock()
	a.edgeChecker = c
	a.mu.Unlock()
}

// ─── domain.Consumer implementation ──────────────────────────────────────────

// OnServerEvent processes a normalized server event and updates live state.
func (a *Aggregator) OnServerEvent(ev domain.ServerEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch ev.Type {
	case domain.EventStreamPublishStart:
		a.onPublishStart(ev)
	case domain.EventStreamPublishEnd:
		a.onPublishEnd(ev)
	case domain.EventStreamStats:
		a.onStreamStats(ev)
	case domain.EventNodeStats:
		a.onNodeStats(ev)
	case domain.EventIngestStats:
		a.onIngestStats(ev)
	case domain.EventWebRTCClientStats:
		a.onWebRTCClientStats(ev)
	}

	// Snapshot is updated incrementally by each handler above (O(1) per event).
	// notifySubs is leading-edge rate-limited: first event after a ≥1 s gap fires
	// immediately; events within the window set snapDirty for the next flush.
	a.notifySubs()
}

// OnBeaconEvent is a no-op for the live aggregator (beacon data goes to ClickHouse).
func (a *Aggregator) OnBeaconEvent(_ domain.BeaconEvent) {}

// OnViewerSession is a no-op for the live aggregator.
func (a *Aggregator) OnViewerSession(_ domain.ViewerSession) {}

// ─── domain.LiveProvider implementation ─────────────────────────────────────

// CurrentSnapshot returns a deep copy of the current live state.
func (a *Aggregator) CurrentSnapshot() *domain.LiveSnapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return copySnapshot(a.snapshot)
}

// Subscribe registers a subscriber channel. Call the returned cancel function
// to unsubscribe and close the channel.
func (a *Aggregator) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 16)
	a.mu.Lock()
	a.subs[ch] = struct{}{}
	a.mu.Unlock()

	cancel := func() {
		a.mu.Lock()
		delete(a.subs, ch)
		a.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

// ─── EvictStaleNodes removes nodes with no updates for nodeStaleThreshold ────────

// EvictStaleNodes removes nodes that have not been seen within nodeStaleThreshold.
// This enables node_down alerting on genuine node disappearance (VD-30).
// nodeStaleThreshold should be 3×PollInterval; call this from the same
// periodic goroutine that calls EvictStale.
func (a *Aggregator) EvictStaleNodes(nodeStaleThreshold time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for nodeID, n := range a.nodes {
		// LastSeenAt is zero for nodes created before VD-30 was applied; skip those.
		if n.LastSeenAt.IsZero() {
			continue
		}
		if now.Sub(n.LastSeenAt) > nodeStaleThreshold {
			a.logger.Info("aggregator: node stale, evicting",
				"node_id", nodeID,
				"last_seen", n.LastSeenAt,
				"threshold", nodeStaleThreshold,
			)
			delete(a.nodes, nodeID)
		}
	}
	// Full rebuild after bulk node removal; this is an infrequent eviction tick.
	a.rebuildSnapshot()
	a.notifySubsForced()
}

// ─── EvictStale removes streams with no updates for staleThreshold ────────────

// EvictStale checks for stale streams and emits offline events.
// Call this periodically (e.g. from a goroutine in serve.go).
func (a *Aggregator) EvictStale() {
	var pending []domain.ServerEvent
	a.mu.Lock()

	now := time.Now()
	for key, s := range a.streams {
		if s.Active && now.Sub(s.LastSeenAt) > a.staleThreshold {
			a.logger.Info("aggregator: stream stale, marking offline",
				"stream_id", s.StreamID,
				"node_id", s.NodeID,
				"last_seen", s.LastSeenAt,
			)
			s.Active = false
			s.Health = domain.StreamHealthOffline

			// Collect the publish_end event; emit after releasing a.mu (see below).
			if a.sink != nil {
				pending = append(pending, domain.ServerEvent{
					Version:  1,
					Type:     domain.EventStreamPublishEnd,
					TS:       now.UnixMilli(),
					Source:   domain.SourceRestPoll,
					NodeID:   s.NodeID,
					App:      s.App,
					StreamID: s.StreamID,
					Data: map[string]any{
						"reason": "stale_eviction",
					},
				})
			}
			// Remove from live map after eviction.
			delete(a.streams, key)
		}
	}
	// Full rebuild after bulk stream removal; this is an infrequent eviction tick.
	a.rebuildSnapshot()
	a.notifySubsForced()
	a.mu.Unlock()

	// Emit eviction events to the sink only AFTER releasing a.mu: the sink fans
	// back into this aggregator's OnServerEvent (a.mu.Lock), so emitting under
	// the lock would self-deadlock (mirror of the cluster.Discovery.poll fix).
	for _, ev := range pending {
		a.sink.WriteServerEvent(ev)
	}
}

// ─── Event handlers (called with lock held) ───────────────────────────────────

func (a *Aggregator) onPublishStart(ev domain.ServerEvent) {
	key := ev.NodeID + "/" + ev.App + "/" + ev.StreamID
	pt := ""
	if pt2, ok := ev.Data["publish_type"].(string); ok {
		pt = pt2
	}
	startedAt := time.UnixMilli(ev.TS).UTC()
	s := &domain.LiveStream{
		StreamID:    ev.StreamID,
		App:         ev.App,
		NodeID:      ev.NodeID,
		PublishType: pt,
		Active:      true,
		StartedAt:   startedAt,
		LastSeenAt:  startedAt,
		Health:      domain.StreamHealthGood,
	}
	// If a stream with the same compound key already exists in the map (restart
	// without an intervening publish_end), remove its old contribution first.
	if old, ok := a.streams[key]; ok {
		a.snapRemoveStream(old)
	}
	a.streams[key] = s
	a.snapAddStream(s)
	a.snapshot.UpdatedAt = time.Now()
}

func (a *Aggregator) onPublishEnd(ev domain.ServerEvent) {
	key := ev.NodeID + "/" + ev.App + "/" + ev.StreamID
	if s, ok := a.streams[key]; ok {
		// CRITICAL: subtract BEFORE mutating Active/Health and deleting from map.
		// onPublishEnd must read the old viewer/bitrate values (D-068 design note).
		a.snapRemoveStream(s)
		s.Active = false
		s.Health = domain.StreamHealthOffline
		delete(a.streams, key)
		a.snapshot.UpdatedAt = time.Now()
	}
}

func (a *Aggregator) onStreamStats(ev domain.ServerEvent) {
	key := ev.NodeID + "/" + ev.App + "/" + ev.StreamID
	s, ok := a.streams[key]
	if ok {
		// Existing stream: subtract old contributions before updating fields.
		a.snapRemoveStream(s)
	} else {
		// New stream discovered via stats (start event may have been missed).
		s = &domain.LiveStream{
			StreamID:  ev.StreamID,
			App:       ev.App,
			NodeID:    ev.NodeID,
			Active:    true,
			StartedAt: time.UnixMilli(ev.TS).UTC(),
			Health:    domain.StreamHealthGood,
		}
		a.streams[key] = s
	}
	s.LastSeenAt = time.Now()

	// VD-03: edge/origin viewer dedup.
	// When an edge node serves the stream, the origin already includes edge
	// viewers in its viewer_count. Skip origin viewer_count to prevent doubling.
	skipViewerCount := false
	if a.edgeChecker != nil {
		if a.edgeChecker.IsEdgeStream(ev.StreamID) && a.edgeChecker.NodeRole(ev.NodeID) == "origin" {
			skipViewerCount = true
		}
	}

	if !skipViewerCount {
		if vc, ok := ev.Data["viewer_count"].(int); ok {
			s.ViewerCount = vc
		} else if vcf, ok := ev.Data["viewer_count"].(float64); ok {
			s.ViewerCount = int(vcf)
		}
	}
	if bps, ok := ev.Data["bitrate_kbps"].(float64); ok {
		s.IngestBitrate = bps
	}

	// Per-protocol viewer counts (only from non-origin when edge is active).
	if !skipViewerCount {
		if pcMap, ok := ev.Data["viewer_count_by_protocol"].(map[string]any); ok {
			s.ViewersByProto = domain.ProtocolViewerCounts{
				WebRTC: intFromAny(pcMap["webrtc"]),
				HLS:    intFromAny(pcMap["hls"]),
				RTMP:   intFromAny(pcMap["rtmp"]),
				DASH:   intFromAny(pcMap["dash"]),
				Other:  intFromAny(pcMap["other"]),
			}
		}
	}

	// Add updated contributions (covers both new-stream and existing-stream paths).
	a.snapAddStream(s)
	a.snapshot.UpdatedAt = time.Now()
}

func (a *Aggregator) onNodeStats(ev domain.ServerEvent) {
	// D-087 ORCH design ruling: FAILURE-STREAK events (api_unreachable=true) must
	// NOT refresh LastSeenAt and must NOT replace the LiveNodeStats struct.
	// Refreshing LastSeenAt would keep the node "fresh" forever and prevent
	// EvictStaleNodes from ever firing (rung 3 of the AMS early-warning ladder).
	// In-place update: only ConsecAPIErrors is written; all other fields stay frozen.
	if unreachable, _ := ev.Data["api_unreachable"].(bool); unreachable {
		existing, ok := a.nodes[ev.NodeID]
		if !ok {
			// Unknown node: failure events create nothing (D-087 contract).
			return
		}
		if errs, ok := ev.Data["consec_api_errors"].(float64); ok {
			existing.ConsecAPIErrors = int(errs)
		}
		// Snapshot already references the same pointer — update is visible immediately.
		a.snapshot.UpdatedAt = time.Now()
		return
	}

	// Normal path: full replace + LastSeenAt=now.
	now := time.Now()
	ns := &domain.LiveNodeStats{
		NodeID:     ev.NodeID,
		UpdatedAt:  now,
		LastSeenAt: now, // VD-30: track when we last heard from this node
	}
	if v, ok := ev.Data["cpu_pct"].(float64); ok {
		ns.CPUPCT = v
	}
	if v, ok := ev.Data["mem_pct"].(float64); ok {
		ns.MemPCT = v
	}
	if v, ok := ev.Data["disk_pct"].(float64); ok {
		ns.DiskPCT = v
	}
	if v, ok := ev.Data["net_in_mbps"].(float64); ok {
		ns.NetIn = v
	}
	if v, ok := ev.Data["net_out_mbps"].(float64); ok {
		ns.NetOut = v
	}
	// VD-40: propagate version string through to the live snapshot.
	if v, ok := ev.Data["version"].(string); ok && v != "" {
		ns.Version = v
	}
	// Standalone node identity fields from real AMS 3.x /rest/v2/system-status.
	if v, ok := ev.Data["os_name"].(string); ok && v != "" {
		ns.OsName = v
	}
	if v, ok := ev.Data["os_arch"].(string); ok && v != "" {
		ns.OsArch = v
	}
	if v, ok := ev.Data["java_version"].(string); ok && v != "" {
		ns.JavaVersion = v
	}
	// processor_count is stored as int in Data but JSON round-trips as float64.
	if v, ok := ev.Data["processor_count"].(float64); ok && v > 0 {
		ns.ProcessorCount = int(v)
	} else if v, ok := ev.Data["processor_count"].(int); ok && v > 0 {
		ns.ProcessorCount = v
	}
	// D-087: extract API latency and consecutive error counter from normal-path events.
	if v, ok := ev.Data["api_latency_ms"].(float64); ok {
		ns.APILatencyMS = v
	}
	if v, ok := ev.Data["consec_api_errors"].(float64); ok {
		ns.ConsecAPIErrors = int(v)
	}
	a.nodes[ev.NodeID] = ns
	// O(1): update snapshot node map in-place (no rebuild needed).
	a.snapshot.Nodes[ev.NodeID] = ns
	a.snapshot.UpdatedAt = time.Now()
}

func (a *Aggregator) onIngestStats(ev domain.ServerEvent) {
	key := ev.NodeID + "/" + ev.App + "/" + ev.StreamID
	s, ok := a.streams[key]
	if !ok {
		return
	}
	// Subtract old IngestBitrate contribution before updating the field.
	a.snapRemoveStream(s)

	// fps is absent on the AMS REST path (currentFPS omitted); use the -1
	// "unavailable" sentinel for scoring so the FPS weight is redistributed rather
	// than scoring a phantom 0 fps. s.FPS keeps its display value (0). D-029v.
	fpsArg := -1.0
	if fps, ok := ev.Data["fps"].(float64); ok {
		s.FPS = fps
		fpsArg = fps
	}
	if bps, ok := ev.Data["bitrate_kbps"].(float64); ok {
		s.IngestBitrate = bps
	}
	if loss, ok := ev.Data["packet_loss_pct"].(float64); ok {
		s.PacketLossPct = loss
	}
	if jitter, ok := ev.Data["jitter_ms"].(float64); ok {
		s.JitterMS = jitter
	}
	if kf, ok := ev.Data["keyframe_interval_s"].(float64); ok {
		s.KeyframeIntervalS = kf
	}

	// VD-20a: bridge HealthTracker → aggregator.
	// Compute health score inline so LiveStream.HealthScore is non-zero
	// whenever ingest_stats are received (F4 PRD acceptance criterion).
	score := ingest.ComputeHealthScore(
		a.targetBitrateKbps, a.targetFPS,
		s.IngestBitrate, fpsArg, s.KeyframeIntervalS, s.PacketLossPct, s.JitterMS,
	)
	s.HealthScore = score
	s.Health = ingest.ScoreToHealth(score)

	// Re-add with the new IngestBitrate.
	a.snapAddStream(s)
	a.snapshot.UpdatedAt = time.Now()
}

// onWebRTCClientStats updates the live stream's viewer-side QoE metrics from a
// webrtc_client_stats event (emitted by collector.NormalizeWebRTCStats).
//
// Multiple peer-stats events may arrive per poll interval when a stream has
// multiple WebRTC viewers. This implementation uses last-write-wins so the
// snapshot always reflects the most recently polled viewer — which is
// deterministic (events are processed sequentially under a.mu) and sufficient
// for the live dashboard. An averaging approach would require tracking counts
// across events; that complexity is deferred until the query/API layer needs it.
// See followup note in aggregator_test.go.
func (a *Aggregator) onWebRTCClientStats(ev domain.ServerEvent) {
	key := ev.NodeID + "/" + ev.App + "/" + ev.StreamID
	s, ok := a.streams[key]
	if !ok {
		// Stream not in the live map (may have ended before this stat arrived).
		return
	}
	if v, ok := ev.Data["rtt_ms"].(float64); ok {
		s.ViewerRTTMS = v
	}
	if v, ok := ev.Data["jitter_ms"].(float64); ok {
		s.ViewerJitterMS = v
	}
	if v, ok := ev.Data["packet_loss_pct"].(float64); ok {
		s.ViewerLossPct = v
	}
	// No aggregate counter changes; a.snapshot.Streams[s.StreamID] already holds
	// the pointer to s, so the field updates above are immediately visible to
	// copySnapshot callers. Just refresh the timestamp.
	a.snapshot.UpdatedAt = time.Now()
}

// UpdateIngestHealth sets the health score for a stream from the ingest health tracker.
// Called by the ingest.HealthTracker via a bridge goroutine or directly.
func (a *Aggregator) UpdateIngestHealth(nodeID, app, streamID string, score float64, health domain.StreamHealth) {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := nodeID + "/" + app + "/" + streamID
	if s, ok := a.streams[key]; ok {
		s.HealthScore = score
		s.Health = health
		// HealthScore/Health are pointer-tracked through snap.Streams; no
		// aggregate counter changes, so no snapRemove/snapAdd needed.
		a.snapshot.UpdatedAt = time.Now()
		a.notifySubs()
	}
}

// ─── Incremental snapshot helpers (called with lock held) ────────────────────

// snapRemoveStream subtracts s's contributions from the live snapshot.
// MUST be called BEFORE any mutation of the fields it reads (Active, ViewerCount,
// IngestBitrate, App, StreamID) and BEFORE deleting s from a.streams.
func (a *Aggregator) snapRemoveStream(s *domain.LiveStream) {
	if !s.Active {
		return
	}
	a.snapshot.ActiveStreams--
	a.snapshot.TotalViewers -= s.ViewerCount
	a.snapshot.IngestBitrate -= s.IngestBitrate
	delete(a.snapshot.Streams, s.StreamID)
	a.snapshot.AppViewers[s.App] -= s.ViewerCount
	if a.snapshot.AppViewers[s.App] <= 0 {
		delete(a.snapshot.AppViewers, s.App)
	}
}

// snapAddStream adds s's contributions to the live snapshot.
// MUST be called AFTER all field mutations so the new values are reflected.
func (a *Aggregator) snapAddStream(s *domain.LiveStream) {
	if !s.Active {
		return
	}
	a.snapshot.ActiveStreams++
	a.snapshot.TotalViewers += s.ViewerCount
	a.snapshot.IngestBitrate += s.IngestBitrate
	// Bare StreamID key — preserved from the original rebuildSnapshot behaviour.
	// Last-write-wins when two nodes host the same bare StreamID in the same cycle
	// (edge/origin pair or cross-app same ID); same semantics as the old rebuild scan.
	a.snapshot.Streams[s.StreamID] = s
	a.snapshot.AppViewers[s.App] += s.ViewerCount
}

// ─── Snapshot builder (called with lock held) ────────────────────────────────

// rebuildSnapshot performs a full O(N) rebuild of a.snapshot from a.streams and
// a.nodes. Retained for the three non-hot paths: New(), EvictStale(),
// EvictStaleNodes(). All per-event paths use incremental snapRemoveStream /
// snapAddStream instead (S10/D-068).
func (a *Aggregator) rebuildSnapshot() {
	snap := &domain.LiveSnapshot{
		Streams:    make(map[string]*domain.LiveStream, len(a.streams)),
		AppViewers: make(map[string]int),
		Nodes:      make(map[string]*domain.LiveNodeStats, len(a.nodes)),
		UpdatedAt:  time.Now(),
	}

	for _, s := range a.streams {
		if !s.Active {
			continue
		}
		snap.ActiveStreams++
		snap.TotalViewers += s.ViewerCount
		snap.IngestBitrate += s.IngestBitrate
		snap.Streams[s.StreamID] = s
		snap.AppViewers[s.App] += s.ViewerCount
	}
	for nodeID, n := range a.nodes {
		snap.Nodes[nodeID] = n
	}

	a.snapshot = snap
}

// ─── Subscriber notification (called with lock held) ─────────────────────────

// notifySubs pushes a deep copy of the snapshot to all subscribers with
// leading-edge rate-limiting: the first call after a ≥1 s gap fires immediately;
// calls within the 1 s window set snapDirty=true so the next out-of-window call
// delivers the accumulated state. No goroutine is spawned; slow subscribers are
// dropped via the non-blocking send (buffered chan cap 16).
func (a *Aggregator) notifySubs() {
	if len(a.subs) == 0 {
		return
	}
	now := time.Now()
	if now.Sub(a.lastNotifyAt) < notifyRateLimit {
		// Still within the coalescing window — mark dirty for next flush.
		a.snapDirty = true
		return
	}
	a.doNotifySubs(now)
}

// notifySubsForced unconditionally pushes a snapshot to all subscribers.
// Used by EvictStale and EvictStaleNodes, where the snapshot has just been
// fully rebuilt and freshness matters regardless of the rate-limit window.
func (a *Aggregator) notifySubsForced() {
	if len(a.subs) == 0 {
		return
	}
	a.doNotifySubs(time.Now())
}

// doNotifySubs performs the actual snapshot copy and fan-out (lock held).
func (a *Aggregator) doNotifySubs(now time.Time) {
	a.lastNotifyAt = now
	a.snapDirty = false
	snap := copySnapshot(a.snapshot)
	for ch := range a.subs {
		select {
		case ch <- snap:
		default:
			// Subscriber is slow — drop this update.
		}
	}
}

// ─── Deep copy ────────────────────────────────────────────────────────────────

func copySnapshot(s *domain.LiveSnapshot) *domain.LiveSnapshot {
	if s == nil {
		return &domain.LiveSnapshot{
			Streams:    make(map[string]*domain.LiveStream),
			AppViewers: make(map[string]int),
			Nodes:      make(map[string]*domain.LiveNodeStats),
			UpdatedAt:  time.Now(),
		}
	}
	cp := &domain.LiveSnapshot{
		ActiveStreams: s.ActiveStreams,
		TotalViewers:  s.TotalViewers,
		IngestBitrate: s.IngestBitrate,
		Streams:       make(map[string]*domain.LiveStream, len(s.Streams)),
		AppViewers:    make(map[string]int, len(s.AppViewers)),
		Nodes:         make(map[string]*domain.LiveNodeStats, len(s.Nodes)),
		UpdatedAt:     s.UpdatedAt,
	}
	for k, v := range s.Streams {
		vCopy := *v
		cp.Streams[k] = &vCopy
	}
	for k, v := range s.AppViewers {
		cp.AppViewers[k] = v
	}
	for k, v := range s.Nodes {
		vCopy := *v
		cp.Nodes[k] = &vCopy
	}
	return cp
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func intFromAny(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	case int64:
		return int(x)
	}
	return 0
}
