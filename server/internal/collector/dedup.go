// Package collector — event deduplication.
//
// REST polling and webhooks can both report the same lifecycle event
// (stream_publish_start, stream_publish_end) for the same stream within a
// short window. The Deduplicator keeps a rolling window of seen event keys
// and drops duplicates.
package collector

import (
	"fmt"
	"sync"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// dedupKey is the composite key used for deduplication.
type dedupKey struct {
	eventType string
	nodeID    string
	streamID  string
	window    int64 // coarse time bucket (ts / windowMs)
}

// Deduplicator drops duplicate events from multiple sources within a time window.
type Deduplicator struct {
	mu       sync.Mutex
	seen     map[dedupKey]time.Time
	windowMs int64 // dedup window in milliseconds (default 10 000 ms = 10 s)
	gcEvery  int   // GC every N calls
	gcCount  int
}

// NewDeduplicator creates a Deduplicator with the given window.
// window=0 defaults to 10 seconds.
func NewDeduplicator(window time.Duration) *Deduplicator {
	ms := window.Milliseconds()
	if ms <= 0 {
		ms = 10_000
	}
	return &Deduplicator{
		seen:     make(map[dedupKey]time.Time),
		windowMs: ms,
		gcEvery:  500,
	}
}

// IsDuplicate returns true if this event was already seen within the dedup window.
// Only lifecycle events (publish_start/end) and recording_ready are deduplicated;
// stats events are always passed through.
func (d *Deduplicator) IsDuplicate(e domain.ServerEvent) bool {
	switch e.Type {
	case domain.EventStreamPublishStart,
		domain.EventStreamPublishEnd,
		domain.EventRecordingReady:
		// Deduplicate these.
	default:
		// Stats events are always unique.
		return false
	}

	key := dedupKey{
		eventType: e.Type,
		nodeID:    e.NodeID,
		streamID:  e.StreamID,
		window:    e.TS / d.windowMs,
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.seen[key]; ok {
		return true
	}
	d.seen[key] = time.Now()
	d.gcCount++
	if d.gcCount >= d.gcEvery {
		d.gc()
	}
	return false
}

// gc removes entries older than 2× the dedup window (called with lock held).
func (d *Deduplicator) gc() {
	cutoff := time.Now().Add(-2 * time.Duration(d.windowMs) * time.Millisecond)
	for k, seen := range d.seen {
		if seen.Before(cutoff) {
			delete(d.seen, k)
		}
	}
	d.gcCount = 0
}

// String returns a debug representation.
func (d *Deduplicator) String() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return fmt.Sprintf("Deduplicator{seen=%d, windowMs=%d}", len(d.seen), d.windowMs)
}
