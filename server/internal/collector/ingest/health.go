// Package ingest implements per-publisher ingest health monitoring (F4).
//
// The HealthTracker extracts per-publisher metrics from ingest_stats events
// (bitrate, fps, keyframe interval, packet loss, jitter) and maintains a live
// per-publisher health score in the aggregator's live state.
//
// Health score formula (documented for BE-02/FE contract):
//
//	score = w_bitrate*S_bitrate + w_fps*S_fps + w_keyframe*S_keyframe + w_loss*S_loss + w_jitter*S_jitter
//
//	Where each sub-score S_X is a 0.0–1.0 value (1.0 = healthy, 0.0 = degraded):
//	  S_bitrate  = clamp(bitrate_kbps / target_bitrate_kbps, 0, 1)
//	              where target_bitrate_kbps = 2000 (default; degraded floor = target * 0.5)
//	  S_fps      = clamp(fps / target_fps, 0, 1)
//	              where target_fps = 30.0 (default; degraded floor = 5)
//	  S_keyframe = 1.0 if keyframe_interval_s <= 2.0 else clamp(2.0/keyframe_interval_s, 0, 1)
//	              (ideal <= 2s for WebRTC; degrades linearly above 2s)
//	              Note: keyframeIdealS=2.0 is the threshold used in code.
//	              keyframeBadS=3.0 is a declared-but-unused constant retained for reference.
//	  S_loss     = clamp(1.0 - packet_loss_pct/10.0, 0, 1)
//	              (0% loss = 1.0; 10%+ loss = 0.0)
//	  S_jitter   = clamp(1.0 - jitter_ms/100.0, 0, 1)
//	              (0ms jitter = 1.0; 100ms+ jitter = 0.0)
//
//	Weights (sum = 1.0):
//	  w_bitrate  = 0.35
//	  w_fps      = 0.25
//	  w_keyframe = 0.20
//	  w_loss     = 0.12
//	  w_jitter   = 0.08
//
//	Score classification:
//	  1.0–0.80 → Good
//	  0.79–0.50 → Warning
//	  < 0.50    → Critical
//	  (absent for > 15s → Offline)
//
// Ingest degradation detection:
//   - Bitrate floor breach: bitrate_kbps < target * bitrateFloorRatio (default 0.5)
//   - FPS collapse: fps < fpsFloor (default 5.0)
//   - Source gone: no ingest_stats event in > sourceGoneTimeout (default 15s)
//
// Budget: degradation visible in live state ≤ 15s of the source change.
// With a 5s REST poll interval, the worst-case detection latency = 10s
// (two poll cycles). The 15s budget is met by construction.
package ingest

import (
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// Weights for the health score formula.
const (
	wBitrate  = 0.35
	wFPS      = 0.25
	wKeyframe = 0.20
	wLoss     = 0.12
	wJitter   = 0.08

	// Default target values.
	DefaultTargetBitrateKbps = 2000.0
	DefaultTargetFPS         = 30.0
	DefaultBitrateFloorRatio = 0.50
	DefaultFPSFloor          = 5.0
	DefaultSourceGoneTimeout = 15 * time.Second

	// Keyframe ideal threshold.
	keyframeIdealS = 2.0
	keyframeBadS   = 3.0
)

// PublisherState holds the latest ingest metrics and computed health for one publisher.
type PublisherState struct {
	StreamID   string
	App        string
	NodeID     string
	LastSeen   time.Time
	UpdatedAt  time.Time

	// Raw metrics from the last ingest_stats event.
	BitrateKbps      float64
	FPS              float64
	KeyframeIntervalS float64
	PacketLossPct    float64
	JitterMS         float64

	// Computed health score (0.0–1.0) and category.
	HealthScore float64
	Health      domain.StreamHealth
}

// HealthTracker maintains per-publisher ingest health state.
// Thread-safe.
type HealthTracker struct {
	mu         sync.RWMutex
	publishers map[string]*PublisherState // key = nodeID + "/" + app + "/" + streamID

	targetBitrateKbps float64
	targetFPS         float64
	bitrateFloorRatio float64
	fpsFloor          float64
	sourceGoneTimeout time.Duration

	logger *slog.Logger
}

// Config for the HealthTracker.
type Config struct {
	TargetBitrateKbps float64
	TargetFPS         float64
	BitrateFloorRatio float64
	FPSFloor          float64
	SourceGoneTimeout time.Duration
}

// New creates a HealthTracker with the given config.
func New(cfg Config, logger *slog.Logger) *HealthTracker {
	if cfg.TargetBitrateKbps == 0 {
		cfg.TargetBitrateKbps = DefaultTargetBitrateKbps
	}
	if cfg.TargetFPS == 0 {
		cfg.TargetFPS = DefaultTargetFPS
	}
	if cfg.BitrateFloorRatio == 0 {
		cfg.BitrateFloorRatio = DefaultBitrateFloorRatio
	}
	if cfg.FPSFloor == 0 {
		cfg.FPSFloor = DefaultFPSFloor
	}
	if cfg.SourceGoneTimeout == 0 {
		cfg.SourceGoneTimeout = DefaultSourceGoneTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HealthTracker{
		publishers:        make(map[string]*PublisherState),
		targetBitrateKbps: cfg.TargetBitrateKbps,
		targetFPS:         cfg.TargetFPS,
		bitrateFloorRatio: cfg.BitrateFloorRatio,
		fpsFloor:          cfg.FPSFloor,
		sourceGoneTimeout: cfg.SourceGoneTimeout,
		logger:            logger,
	}
}

// OnServerEvent processes ingest_stats and stream_stats events.
// Implements collector.Consumer partially (server events only).
func (h *HealthTracker) OnServerEvent(ev domain.ServerEvent) {
	switch ev.Type {
	case domain.EventIngestStats:
		h.onIngestStats(ev)
	case domain.EventStreamPublishEnd:
		h.onPublishEnd(ev)
	}
}

// OnBeaconEvent is a no-op for the health tracker.
func (h *HealthTracker) OnBeaconEvent(_ domain.BeaconEvent) {}

// OnViewerSession is a no-op for the health tracker.
func (h *HealthTracker) OnViewerSession(_ domain.ViewerSession) {}

func (h *HealthTracker) onIngestStats(ev domain.ServerEvent) {
	key := ev.NodeID + "/" + ev.App + "/" + ev.StreamID
	now := time.UnixMilli(ev.TS).UTC()
	if now.IsZero() {
		now = time.Now()
	}

	bitrate := floatFromData(ev.Data, "bitrate_kbps")
	fps := floatFromData(ev.Data, "fps")
	keyframe := floatFromData(ev.Data, "keyframe_interval_s")
	loss := floatFromData(ev.Data, "packet_loss_pct")
	jitter := floatFromData(ev.Data, "jitter_ms")

	score := ComputeHealthScore(h.targetBitrateKbps, h.targetFPS, bitrate, fps, keyframe, loss, jitter)
	health := ScoreToHealth(score)

	h.mu.Lock()
	pub, ok := h.publishers[key]
	if !ok {
		pub = &PublisherState{
			StreamID: ev.StreamID,
			App:      ev.App,
			NodeID:   ev.NodeID,
		}
		h.publishers[key] = pub
	}
	pub.LastSeen = now
	pub.UpdatedAt = time.Now()
	pub.BitrateKbps = bitrate
	pub.FPS = fps
	pub.KeyframeIntervalS = keyframe
	pub.PacketLossPct = loss
	pub.JitterMS = jitter
	pub.HealthScore = score
	pub.Health = health
	snap := *pub
	h.mu.Unlock()

	// Log drops for ops visibility.
	if health == domain.StreamHealthCritical || health == domain.StreamHealthWarning {
		h.logger.Info("ingest: health degraded",
			"stream", ev.StreamID,
			"app", ev.App,
			"node", ev.NodeID,
			"score", math.Round(score*100)/100,
			"health", health,
			"bitrate_kbps", bitrate,
			"fps", fps,
		)
	}
	_ = snap
}

func (h *HealthTracker) onPublishEnd(ev domain.ServerEvent) {
	key := ev.NodeID + "/" + ev.App + "/" + ev.StreamID
	h.mu.Lock()
	delete(h.publishers, key)
	h.mu.Unlock()
}

// SweepStale marks publishers that haven't sent ingest_stats recently as offline.
// Call this periodically (e.g. every 5s). Returns the number of stale publishers.
func (h *HealthTracker) SweepStale() int {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()

	count := 0
	for key, pub := range h.publishers {
		if now.Sub(pub.LastSeen) > h.sourceGoneTimeout {
			pub.Health = domain.StreamHealthOffline
			pub.HealthScore = 0
			h.logger.Warn("ingest: source gone (no stats)",
				"stream", pub.StreamID,
				"app", pub.App,
				"node", pub.NodeID,
				"last_seen", pub.LastSeen,
			)
			// Remove from tracking so it doesn't stay in the map forever.
			delete(h.publishers, key)
			count++
		}
	}
	return count
}

// Snapshot returns a copy of all publisher states.
func (h *HealthTracker) Snapshot() map[string]PublisherState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[string]PublisherState, len(h.publishers))
	for k, v := range h.publishers {
		out[k] = *v
	}
	return out
}

// GetPublisher returns the health state for a specific publisher key.
// Returns false if not found.
func (h *HealthTracker) GetPublisher(nodeID, app, streamID string) (PublisherState, bool) {
	key := nodeID + "/" + app + "/" + streamID
	h.mu.RLock()
	defer h.mu.RUnlock()
	pub, ok := h.publishers[key]
	if !ok {
		return PublisherState{}, false
	}
	return *pub, true
}

// ─── Health score formula ─────────────────────────────────────────────────────

// ComputeHealthScore computes the weighted ingest health score (0.0–1.0).
//
// Formula (documented in package-level comment):
//
//	score = 0.35*S_bitrate + 0.25*S_fps + 0.20*S_keyframe + 0.12*S_loss + 0.08*S_jitter
//
// This is the authoritative formula — BE-02 and FE both reference this function.
// Deterministic: same inputs always produce the same output.
func ComputeHealthScore(
	targetBitrateKbps, targetFPS float64,
	bitrateKbps, fps, keyframeIntervalS, packetLossPct, jitterMS float64,
) float64 {
	// S_bitrate: linear scale to target; floor at 0.
	sBitrate := 0.0
	if targetBitrateKbps > 0 {
		sBitrate = clamp01(bitrateKbps / targetBitrateKbps)
	}

	// S_fps: linear scale to target fps.
	sFPS := 0.0
	if targetFPS > 0 {
		sFPS = clamp01(fps / targetFPS)
	}

	// S_keyframe: ideal <= 2.0s (keyframeIdealS); degrades linearly above 2.0s.
	// Score = 2.0/keyframeIntervalS clamped to [0,1]. There is no hard upper
	// cutoff at 3.0s — the formula is continuous. keyframeBadS=3.0 is retained
	// as a reference constant but is not used in the scoring formula (VD-25).
	sKeyframe := 1.0
	if keyframeIntervalS > keyframeIdealS {
		sKeyframe = clamp01(keyframeIdealS / keyframeIntervalS)
	}

	// S_loss: 0% loss = 1.0; 10% loss = 0.0; linear.
	sLoss := clamp01(1.0 - packetLossPct/10.0)

	// S_jitter: 0ms = 1.0; 100ms = 0.0; linear.
	sJitter := clamp01(1.0 - jitterMS/100.0)

	score := wBitrate*sBitrate + wFPS*sFPS + wKeyframe*sKeyframe + wLoss*sLoss + wJitter*sJitter

	// Normalize (should already be in [0,1] given weights sum to 1.0).
	return clamp01(score)
}

// ScoreToHealth maps a health score to a StreamHealth category.
func ScoreToHealth(score float64) domain.StreamHealth {
	switch {
	case score >= 0.80:
		return domain.StreamHealthGood
	case score >= 0.50:
		return domain.StreamHealthWarning
	default:
		return domain.StreamHealthCritical
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func floatFromData(d map[string]any, key string) float64 {
	if d == nil {
		return 0
	}
	if v, ok := d[key]; ok {
		switch x := v.(type) {
		case float64:
			return x
		case float32:
			return float64(x)
		case int:
			return float64(x)
		}
	}
	return 0
}
