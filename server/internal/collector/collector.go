// Package collector ingests all data sources and emits normalized domain
// events to the store. It is stateless: restart-safe, no local persistence
// beyond in-flight batching (PRD §7.10).
//
// Sources (each in its own subpackage, all implementing Source):
//
//	restpoller — polls AMS REST v2 at a configurable interval (the universal
//	             fallback; works on every AMS version — PRD Appendix A.5)
//	logtail    — tails ant-media-server-analytics.log (JSON, v2.10+)
//	kafka      — consumes the native AMS Kafka producer feed when enabled
//	webhook    — receives AMS publish/unpublish/recording webhooks
//	beacon     — public HTTPS ingest endpoint for the player QoE SDK
//
// Sources are deduplicating-by-design: REST polling and webhooks may report the
// same lifecycle event; the collector normalizes and dedupes before storage.
package collector

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"
)

// Source is one ingest pipeline producing normalized ServerEvents/BeaconEvents.
type Source interface {
	// Name identifies the source in logs and self-metrics.
	Name() string
	// Run blocks, emitting events until ctx is cancelled.
	Run(ctx context.Context) error
}

// Collector supervises all configured sources with per-source restart/backoff.
type Collector struct {
	sources []Source
	logger  *slog.Logger
}

// New creates a Collector that supervises the given sources.
func New(logger *slog.Logger, sources ...Source) *Collector {
	if logger == nil {
		logger = slog.Default()
	}
	return &Collector{sources: sources, logger: logger}
}

// Run starts all sources and supervises them with exponential backoff restart.
// It blocks until ctx is cancelled, then waits for all sources to exit.
func (c *Collector) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, src := range c.sources {
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()
			c.supervise(ctx, s)
		}(src)
	}
	wg.Wait()
}

// supervise runs a single source with exponential backoff restart.
// It stops when ctx is cancelled.
func (c *Collector) supervise(ctx context.Context, src Source) {
	const (
		minBackoff = 100 * time.Millisecond
		maxBackoff = 60 * time.Second
	)
	attempt := 0

	for {
		if ctx.Err() != nil {
			return
		}

		err := src.Run(ctx)

		if ctx.Err() != nil {
			// Normal shutdown — ctx was cancelled.
			return
		}

		if err != nil {
			backoff := minBackoff * time.Duration(math.Pow(2, float64(attempt)))
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			c.logger.Warn("collector: source exited, restarting",
				"source", src.Name(),
				"error", err,
				"backoff", backoff,
				"attempt", attempt+1,
			)
			attempt++
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		} else {
			// Clean exit (source returned nil) — reset backoff and restart.
			attempt = 0
			select {
			case <-ctx.Done():
				return
			case <-time.After(minBackoff):
			}
		}
	}
}
