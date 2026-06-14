// Package kafka consumes the native AMS Kafka producer feed (instance and
// stream stats every 15s when server.kafka_brokers is configured). Optional
// source: customers who already enabled Kafka for DIY Grafana get instant
// richer data — part of converting the DIY crowd (PRD §7.7).
//
// Library choice: segmentio/kafka-go (pure-Go, no CGO, well-maintained, simple
// consumer group API). franz-go is also pure-Go but has a steeper API surface;
// kafka-go is sufficient for the AMS use case and aligns with CGO_ENABLED=0.
//
// Verification: in-process fake/contract tests only (D-007.5 — no broker on
// this machine). The consumer is exercised by FakeConsumer in the test harness.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	kafkago "github.com/segmentio/kafka-go"
)

// Config for the Kafka source.
type Config struct {
	// Brokers is the list of Kafka broker addresses.
	Brokers []string

	// GroupID is the consumer group ID.
	GroupID string

	// Topics is the list of Kafka topics to consume.
	// AMS native producer typically publishes to "ams-server-events".
	Topics []string

	// NodeID identifies this node in emitted events.
	NodeID string

	// StartOffset configures where to start reading (default: latest).
	// Use kafkago.LastOffset for live-only; kafkago.FirstOffset for replay.
	StartOffset int64

	// MaxWait is the maximum time to wait for a batch (default: 1s).
	MaxWait time.Duration

	// MinBytes / MaxBytes for fetch requests.
	MinBytes int
	MaxBytes int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(brokers []string, nodeID string) Config {
	return Config{
		Brokers:     brokers,
		GroupID:     "pulse-collector",
		Topics:      []string{"ams-server-events"},
		NodeID:      nodeID,
		StartOffset: kafkago.LastOffset,
		MaxWait:     1 * time.Second,
		MinBytes:    1,
		MaxBytes:    10 << 20, // 10 MB
	}
}

// Source implements collector.Source consuming AMS Kafka topics.
//
// AMS publishes JSON messages in a shape compatible with our domain events.
// The consumer reads, JSON-decodes, and normalizes each message, fanning it
// out to the EventSink. Malformed messages are skipped with a counter
// increment (never crash). Reconnect with exponential backoff is handled by
// the parent collector.Collector supervisor.
type Source struct {
	cfg    Config
	sink   domain.EventSink
	logger *slog.Logger

	// lag holds the last observed consumer lag (for /healthz reporting).
	// Accessed via Lag(); updated on each fetch.
	lag int64

	// parseErrors counts malformed messages since start.
	parseErrors int64
}

// New creates a Kafka Source.
func New(cfg Config, sink domain.EventSink, logger *slog.Logger) *Source {
	if cfg.MaxWait == 0 {
		cfg.MaxWait = 1 * time.Second
	}
	if cfg.MinBytes == 0 {
		cfg.MinBytes = 1
	}
	if cfg.MaxBytes == 0 {
		cfg.MaxBytes = 10 << 20
	}
	if len(cfg.Topics) == 0 {
		cfg.Topics = []string{"ams-server-events"}
	}
	if cfg.GroupID == "" {
		cfg.GroupID = "pulse-collector"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Source{cfg: cfg, sink: sink, logger: logger}
}

// Name implements collector.Source.
func (s *Source) Name() string { return "kafka" }

// Lag returns the last observed consumer lag across all topic-partitions.
// This is surfaced in /healthz component detail.
func (s *Source) Lag() int64 { return s.lag }

// ParseErrors returns the count of malformed messages since start.
func (s *Source) ParseErrors() int64 { return s.parseErrors }

// Run implements collector.Source. It blocks until ctx is cancelled.
// On broker failure the function returns an error; the supervisor restarts
// with backoff.
func (s *Source) Run(ctx context.Context) error {
	if len(s.cfg.Brokers) == 0 {
		return fmt.Errorf("kafka: no brokers configured")
	}

	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:     s.cfg.Brokers,
		GroupID:     s.cfg.GroupID,
		GroupTopics: s.cfg.Topics,
		StartOffset: s.cfg.StartOffset,
		MaxWait:     s.cfg.MaxWait,
		MinBytes:    s.cfg.MinBytes,
		MaxBytes:    s.cfg.MaxBytes,
	})
	defer r.Close()

	s.logger.Info("kafka: consumer started",
		"brokers", s.cfg.Brokers,
		"topics", s.cfg.Topics,
		"group", s.cfg.GroupID,
	)

	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				// Normal shutdown.
				return nil
			}
			return fmt.Errorf("kafka: fetch: %w", err)
		}

		s.processMessage(msg)

		// Commit offset after successful processing (at-least-once delivery).
		if err := r.CommitMessages(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.logger.Warn("kafka: commit offset failed", "error", err)
		}
	}
}

// processMessage decodes and normalizes a single Kafka message.
// Malformed messages increment parseErrors and are skipped.
func (s *Source) processMessage(msg kafkago.Message) {
	if len(msg.Value) == 0 {
		return
	}

	// AMS publishes JSON event objects. Try to decode as a generic map first
	// so we can route to the correct normalized event type.
	var raw map[string]any
	if err := json.Unmarshal(msg.Value, &raw); err != nil {
		s.parseErrors++
		s.logger.Debug("kafka: malformed JSON, skipping",
			"topic", msg.Topic,
			"partition", msg.Partition,
			"offset", msg.Offset,
			"error", err,
		)
		return
	}

	ev, err := normalizeKafkaMessage(raw, s.cfg.NodeID)
	if err != nil {
		s.parseErrors++
		s.logger.Debug("kafka: normalize failed, skipping",
			"topic", msg.Topic,
			"error", err,
		)
		return
	}

	s.sink.WriteServerEvent(ev)
}

// normalizeKafkaMessage converts a raw AMS Kafka message to a domain.ServerEvent.
//
// AMS publishes stats events on Kafka with a shape similar to REST v2 but as
// a flat JSON object. The message typically includes a "streamId", "app",
// event type fields (bitrate, fps, viewer counts, CPU usage, etc.).
//
// We map to domain.ServerEvent types using the presence of key fields:
//   - "streamId" + "cpuUsage" → node_stats
//   - "streamId" + "bitrate" + "fps" → ingest_stats (stream-level)
//   - "streamId" + "hlsViewerCount" → stream_stats
//   - else → stream_stats (best-effort)
func normalizeKafkaMessage(raw map[string]any, nodeID string) (domain.ServerEvent, error) {
	now := time.Now().UnixMilli()

	// Extract common fields.
	streamID, _ := raw["streamId"].(string)
	app, _ := raw["app"].(string)
	if app == "" {
		app = "live"
	}
	nid := nodeID
	if nid == "" {
		if n, ok := raw["nodeId"].(string); ok && n != "" {
			nid = n
		}
	}

	// Timestamp: AMS may include "timestamp" as epoch ms.
	ts := now
	if tsRaw, ok := raw["timestamp"].(float64); ok && tsRaw > 0 {
		ts = int64(tsRaw)
	}

	// Route by field presence.
	var evType string
	var data map[string]any

	switch {
	case hasKey(raw, "cpuUsage"):
		// Node stats message.
		evType = domain.EventNodeStats
		data = map[string]any{
			"cpu_pct":  floatField(raw, "cpuUsage"),
			"mem_pct":  floatField(raw, "memoryUsage"),
			"disk_pct": floatField(raw, "diskUsage"),
		}

	case hasKey(raw, "fps") && hasKey(raw, "bitrate"):
		// Ingest/stream stats (per-stream level from AMS Kafka producer).
		evType = domain.EventIngestStats
		data = map[string]any{
			"bitrate_kbps":        floatField(raw, "bitrate"),
			"fps":                 floatField(raw, "fps"),
			"keyframe_interval_s": floatField(raw, "keyFrameInterval"),
			"packet_loss_pct":     floatField(raw, "packetLost"),
			"jitter_ms":           floatField(raw, "jitter"),
		}

	default:
		// Stream stats (viewer counts).
		evType = domain.EventStreamStats
		total := int(floatField(raw, "hlsViewerCount")) +
			int(floatField(raw, "webRTCViewerCount")) +
			int(floatField(raw, "rtmpViewerCount"))
		data = map[string]any{
			"viewer_count": total,
			"viewer_count_by_protocol": map[string]any{
				"webrtc": int(floatField(raw, "webRTCViewerCount")),
				"hls":    int(floatField(raw, "hlsViewerCount")),
				"rtmp":   int(floatField(raw, "rtmpViewerCount")),
			},
			"bitrate_kbps": floatField(raw, "bitrate"),
		}
	}

	return domain.ServerEvent{
		Version:  1,
		Type:     evType,
		TS:       ts,
		Source:   domain.SourceKafka,
		NodeID:   nid,
		App:      app,
		StreamID: streamID,
		Data:     data,
	}, nil
}

func hasKey(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

func floatField(m map[string]any, key string) float64 {
	if v, ok := m[key]; ok {
		switch x := v.(type) {
		case float64:
			return x
		case int:
			return float64(x)
		}
	}
	return 0
}
