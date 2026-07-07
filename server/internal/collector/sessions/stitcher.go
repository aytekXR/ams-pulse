// Package sessions implements viewer-session stitching.
//
// Session stitching combines viewer_join / beacon heartbeats / viewer_leave
// events into ViewerSession rows written to ClickHouse's viewer_sessions
// ReplacingMergeTree. Absent leave events (network drops, browser close) are
// handled by a timeout close: idle sessions are closed after configurable idle
// time (default 5 min).
//
// Event flow:
//
//	collector → Fanout → Stitcher.OnServerEvent / OnBeaconEvent
//	    Stitcher writes ViewerSession upserts via EventSink
//
// The stitcher is Consumer-shaped so it plugs into the existing Fanout.
package sessions

import (
	"log/slog"
	"sync"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

const (
	// DefaultIdleTimeout is the maximum idle time before an open session is
	// closed by the timeout sweep. If no heartbeat arrives within this window,
	// the session is finalized with the last known state.
	DefaultIdleTimeout = 5 * time.Minute
)

// sessionState tracks the mutable state of an in-flight viewer session.
type sessionState struct {
	sess     domain.ViewerSession
	lastSeen time.Time
	closed   bool
}

// Stitcher implements collector.Consumer and stitches viewer events into sessions.
// Thread-safe.
type Stitcher struct {
	mu          sync.Mutex
	sessions    map[string]*sessionState // key = session_id
	sink        domain.EventSink
	idleTimeout time.Duration
	logger      *slog.Logger
}

// Config holds stitcher configuration.
type Config struct {
	// IdleTimeout: sessions with no activity for this long are closed.
	// Default: 5 min.
	IdleTimeout time.Duration
}

// New creates a Stitcher wired to the given EventSink.
func New(cfg Config, sink domain.EventSink, logger *slog.Logger) *Stitcher {
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Stitcher{
		sessions:    make(map[string]*sessionState),
		sink:        sink,
		idleTimeout: cfg.IdleTimeout,
		logger:      logger,
	}
}

// ─── collector.Consumer implementation ──────────────────────────────────────

// OnServerEvent processes viewer_join and viewer_leave events.
func (s *Stitcher) OnServerEvent(ev domain.ServerEvent) {
	switch ev.Type {
	case domain.EventViewerJoin:
		s.handleJoin(ev)
	case domain.EventViewerLeave:
		s.handleLeave(ev)
	}
}

// OnBeaconEvent processes beacon heartbeat events to update session progress.
// BE-02 feeds beacon events through the same fanout, so the stitcher receives them.
func (s *Stitcher) OnBeaconEvent(ev domain.BeaconEvent) {
	// Update watch_time from heartbeat events.
	for _, item := range ev.Events {
		if item.Type == "heartbeat" {
			s.handleBeaconHeartbeat(ev, item)
		}
	}
}

// OnViewerSession is a no-op (stitcher produces sessions, it doesn't consume them).
func (s *Stitcher) OnViewerSession(_ domain.ViewerSession) {}

// ─── Event handlers ───────────────────────────────────────────────────────────

func (s *Stitcher) handleJoin(ev domain.ServerEvent) {
	viewerID, _ := ev.Data["viewer_id"].(string)
	if viewerID == "" {
		return
	}
	protocol, _ := ev.Data["protocol"].(string)

	// session_id = viewer_id for server-side events (no SDK session id).
	sessID := viewerID

	var geo domain.GeoEnrichment
	var client domain.ClientEnrichment
	if ev.Enrichment != nil {
		if ev.Enrichment.Geo != nil {
			geo = *ev.Enrichment.Geo
		}
		if ev.Enrichment.Client != nil {
			client = *ev.Enrichment.Client
		}
	}

	now := time.UnixMilli(ev.TS).UTC()
	sess := domain.ViewerSession{
		SessionID:     sessID,
		StreamID:      ev.StreamID,
		App:           ev.App,
		NodeID:        ev.NodeID,
		StartedAt:     now,
		EndedAt:       now,
		UpdatedAt:     now,
		Protocol:      protocol,
		GeoCountry:    geo.Country,
		GeoRegion:     geo.Region,
		ClientDevice:  client.Device,
		ClientOS:      client.OS,
		ClientBrowser: client.Browser,
	}

	s.mu.Lock()
	s.sessions[sessID] = &sessionState{
		sess:     sess,
		lastSeen: now,
	}
	s.mu.Unlock()

	// Write initial session row.
	s.sink.WriteViewerSession(sess)

	s.logger.Debug("session: join", "session_id", sessID, "stream_id", ev.StreamID, "protocol", protocol)
}

func (s *Stitcher) handleLeave(ev domain.ServerEvent) {
	viewerID, _ := ev.Data["viewer_id"].(string)
	if viewerID == "" {
		return
	}
	sessID := viewerID
	watchTimeS, _ := ev.Data["watch_time_s"].(float64)

	now := time.UnixMilli(ev.TS).UTC()

	s.mu.Lock()
	state, ok := s.sessions[sessID]
	if !ok {
		// Leave without join — create a minimal closed session.
		s.mu.Unlock()
		return
	}
	state.sess.EndedAt = now
	state.sess.UpdatedAt = now
	state.sess.WatchTimeS = uint32(watchTimeS)
	if state.sess.WatchTimeS == 0 {
		// Derive from timestamps if AMS didn't send it.
		elapsed := now.Sub(state.sess.StartedAt)
		if elapsed > 0 {
			state.sess.WatchTimeS = uint32(elapsed.Seconds())
		}
	}
	state.closed = true
	finalSess := state.sess
	delete(s.sessions, sessID)
	s.mu.Unlock()

	s.sink.WriteViewerSession(finalSess)
	s.logger.Debug("session: leave", "session_id", sessID, "watch_s", finalSess.WatchTimeS)
}

func (s *Stitcher) handleBeaconHeartbeat(batch domain.BeaconEvent, item domain.BeaconItem) {
	sessID := batch.SessionID
	if sessID == "" {
		return
	}

	watchMS, _ := item.Data["watch_ms"].(float64)

	now := time.UnixMilli(item.TS).UTC()

	s.mu.Lock()
	state, ok := s.sessions[sessID]
	if !ok {
		// Session not seen via viewer_join — create from beacon context.
		var geo domain.GeoEnrichment
		var client domain.ClientEnrichment
		if batch.Enrichment != nil {
			if batch.Enrichment.Geo != nil {
				geo = *batch.Enrichment.Geo
			}
			if batch.Enrichment.Client != nil {
				client = *batch.Enrichment.Client
			}
		}
		state = &sessionState{
			sess: domain.ViewerSession{
				SessionID:     sessID,
				StreamID:      batch.StreamID,
				App:           batch.App,
				StartedAt:     now,
				EndedAt:       now,
				UpdatedAt:     now,
				GeoCountry:    geo.Country,
				GeoRegion:     geo.Region,
				ClientDevice:  client.Device,
				ClientOS:      client.OS,
				ClientBrowser: client.Browser,
				Tenant:        batch.Tenant,
			},
			lastSeen: now,
		}
		s.sessions[sessID] = state
	}

	state.lastSeen = now
	state.sess.UpdatedAt = now
	state.sess.EndedAt = now
	// Update watch time from heartbeat (beacon sends cumulative watch_ms).
	if watchMS > 0 {
		state.sess.WatchTimeS = uint32(watchMS / 1000)
	}
	snap := state.sess
	s.mu.Unlock()

	// Upsert: write updated session.
	s.sink.WriteViewerSession(snap)
}

// ─── Idle timeout sweep ────────────────────────────────────────────────────────

// EvictIdle closes sessions that have been idle longer than idleTimeout.
// Call this periodically (e.g. every minute from a goroutine in serve.go).
//
// Each evicted session is written to the EventSink with a final upsert row
// so ClickHouse ReplacingMergeTree will finalize it.
func (s *Stitcher) EvictIdle() int {
	now := time.Now()
	var toEvict []string

	s.mu.Lock()
	for id, state := range s.sessions {
		if !state.closed && now.Sub(state.lastSeen) > s.idleTimeout {
			toEvict = append(toEvict, id)
		}
	}

	// Close the timed-out sessions.
	evicted := make([]domain.ViewerSession, 0, len(toEvict))
	for _, id := range toEvict {
		state := s.sessions[id]
		state.sess.EndedAt = state.lastSeen // last known activity
		elapsed := state.lastSeen.Sub(state.sess.StartedAt)
		if state.sess.WatchTimeS == 0 && elapsed > 0 {
			state.sess.WatchTimeS = uint32(elapsed.Seconds())
		}
		state.sess.UpdatedAt = now
		evicted = append(evicted, state.sess)
		delete(s.sessions, id)
	}
	s.mu.Unlock()

	// Write final upserts outside the lock.
	for _, sess := range evicted {
		s.sink.WriteViewerSession(sess)
		s.logger.Debug("session: idle timeout close",
			"session_id", sess.SessionID,
			"watch_s", sess.WatchTimeS,
		)
	}

	if len(evicted) > 0 {
		s.logger.Info("session: idle eviction", "count", len(evicted))
	}
	return len(evicted)
}

// ActiveCount returns the number of currently open sessions.
func (s *Stitcher) ActiveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// Snapshot returns a copy of all active session IDs (for testing/introspection).
func (s *Stitcher) Snapshot() []domain.ViewerSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.ViewerSession, 0, len(s.sessions))
	for _, state := range s.sessions {
		out = append(out, state.sess)
	}
	return out
}
