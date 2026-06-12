// Package collector — Fanout implements domain.EventSink.
//
// Fanout receives normalized events from all collectors and fans them out
// to registered consumers (ClickHouse writer, live aggregator, etc.).
// All writes are non-blocking: if a consumer's channel is full the event is
// dropped with a counter increment — collectors must not block on slow consumers.
package collector

import (
	"log/slog"
	"sync/atomic"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// Consumer is any component that wants to receive normalized events.
type Consumer interface {
	// OnServerEvent is called for each normalized ServerEvent.
	OnServerEvent(event domain.ServerEvent)
	// OnBeaconEvent is called for each normalized BeaconEvent.
	OnBeaconEvent(event domain.BeaconEvent)
	// OnViewerSession is called for each viewer session upsert.
	OnViewerSession(session domain.ViewerSession)
}

// Fanout implements domain.EventSink and fans events out to all registered
// consumers synchronously. For async delivery, consumers buffer internally.
type Fanout struct {
	consumers []Consumer
	dropped   atomic.Int64
	logger    *slog.Logger
}

// NewFanout creates a Fanout with the given consumers.
func NewFanout(logger *slog.Logger, consumers ...Consumer) *Fanout {
	if logger == nil {
		logger = slog.Default()
	}
	return &Fanout{consumers: consumers, logger: logger}
}

// WriteServerEvent implements domain.EventSink.
func (f *Fanout) WriteServerEvent(event domain.ServerEvent) {
	for _, c := range f.consumers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					f.logger.Error("fanout: consumer panic on server event",
						"panic", r,
						"event_type", event.Type,
					)
					f.dropped.Add(1)
				}
			}()
			c.OnServerEvent(event)
		}()
	}
}

// WriteBeaconEvent implements domain.EventSink.
func (f *Fanout) WriteBeaconEvent(event domain.BeaconEvent) {
	for _, c := range f.consumers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					f.logger.Error("fanout: consumer panic on beacon event", "panic", r)
					f.dropped.Add(1)
				}
			}()
			c.OnBeaconEvent(event)
		}()
	}
}

// WriteViewerSession implements domain.EventSink.
func (f *Fanout) WriteViewerSession(session domain.ViewerSession) {
	for _, c := range f.consumers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					f.logger.Error("fanout: consumer panic on viewer session", "panic", r)
					f.dropped.Add(1)
				}
			}()
			c.OnViewerSession(session)
		}()
	}
}

// DroppedCount returns the total number of events dropped due to consumer panics.
func (f *Fanout) DroppedCount() int64 { return f.dropped.Load() }
