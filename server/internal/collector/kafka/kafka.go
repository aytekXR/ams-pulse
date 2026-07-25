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
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/domain"
	kafkago "github.com/segmentio/kafka-go"
)

// errSkipMessage is returned by normalizeKafkaMessage when a message is
// intentionally skipped (not a parse error) — the topic is recognised but its
// shape does not map to any domain event type.
var errSkipMessage = errors.New("skip")

// Config for the Kafka source.
type Config struct {
	// Brokers is the list of Kafka broker addresses.
	Brokers []string

	// GroupID is the consumer group ID.
	GroupID string

	// Topics is the list of Kafka topics to consume.
	// AMS native producer publishes to "ams-instance-stats" and "ams-webrtc-stats".
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
		Topics:      []string{"ams-instance-stats", "ams-webrtc-stats"},
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
	// Accessed via Lag() from the healthz goroutine; updated by Run() — must be race-safe.
	lag atomic.Int64

	// parseErrors counts malformed messages since start.
	// Accessed via ParseErrors() from the healthz goroutine; updated by processMessage() — race-safe.
	parseErrors atomic.Int64

	// webrtcSkipOnce ensures the "ams-webrtc-stats skipped" debug log fires at most
	// once per broker session — Run() resets it, so the log re-fires after each
	// supervisor reconnect rather than being suppressed for the Source's lifetime.
	webrtcSkipOnce sync.Once
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
		cfg.Topics = []string{"ams-instance-stats", "ams-webrtc-stats"}
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
// This is surfaced in /healthz component detail. Safe to call from any goroutine.
func (s *Source) Lag() int64 { return s.lag.Load() }

// ParseErrors returns the count of malformed messages since start.
// Safe to call from any goroutine.
func (s *Source) ParseErrors() int64 { return s.parseErrors.Load() }

// Run implements collector.Source. It blocks until ctx is cancelled.
// On broker failure the function returns an error; the supervisor restarts
// with backoff.
func (s *Source) Run(ctx context.Context) error {
	if len(s.cfg.Brokers) == 0 {
		return fmt.Errorf("kafka: no brokers configured")
	}

	// New broker session: re-arm the once-per-session skip log. Safe: processMessage
	// only runs from this goroutine, and the supervisor calls Run serially.
	s.webrtcSkipOnce = sync.Once{}

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

		// Update lag from reader stats after each successful fetch.
		// Stats().Lag reflects the lag at the time of the last fetch.
		s.lag.Store(r.Stats().Lag)

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
// Intentionally skipped messages (errSkipMessage) do not increment the counter.
func (s *Source) processMessage(msg kafkago.Message) {
	if len(msg.Value) == 0 {
		return
	}

	// AMS publishes JSON event objects. Try to decode as a generic map first
	// so we can route to the correct normalized event type.
	var raw map[string]any
	if err := json.Unmarshal(msg.Value, &raw); err != nil {
		s.parseErrors.Add(1)
		s.logger.Debug("kafka: malformed JSON, skipping",
			"topic", msg.Topic,
			"partition", msg.Partition,
			"offset", msg.Offset,
			"error", err,
		)
		return
	}

	ev, err := normalizeKafkaMessage(raw, s.cfg.NodeID, msg.Topic)
	if err != nil {
		if errors.Is(err, errSkipMessage) {
			// Known topic with no domain mapping — log once then stay silent.
			s.webrtcSkipOnce.Do(func() {
				s.logger.Debug("kafka: ams-webrtc-stats messages skipped"+
					" (per-viewer client shape does not map to a domain event type;"+
					" no metrics fabricated)",
					"topic", msg.Topic,
				)
			})
			return
		}
		s.parseErrors.Add(1)
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
// Messages are routed by topic name:
//
//   - "ams-instance-stats" → EventNodeStats using the official AMS nested shape
//     (StatsCollector.java: cpuUsage.systemCPULoad, systemMemoryInfo, fileSystemInfo).
//
//   - "ams-webrtc-stats" → skipped (errSkipMessage); the per-viewer client shape
//     (webrtcClientId, measured_bitrate, send_bitrate, audioFrameSendPeriod …) does
//     not map cleanly to EventIngestStats or EventStreamStats.
//
//   - any other topic → legacy field-sniffing (back-compat for operators feeding a
//     custom bridge topic that emits flat cpuUsage / bitrate+fps / viewer-count fields).
func normalizeKafkaMessage(raw map[string]any, nodeID, topic string) (domain.ServerEvent, error) {
	now := time.Now().UnixMilli()

	// Extract common fields present in most message shapes.
	streamID, _ := raw["streamId"].(string)
	app, _ := raw["app"].(string)
	if app == "" {
		app = "live"
	}

	// Timestamp: AMS may include "timestamp" as epoch ms.
	ts := now
	if tsRaw, ok := raw["timestamp"].(float64); ok && tsRaw > 0 {
		ts = int64(tsRaw)
	}

	switch topic {

	case "ams-instance-stats":
		// Official AMS instance stats (StatsCollector.java sendInstanceStats).
		//
		// Node identity: configured NodeID wins (operator-controlled); fallback to
		// instanceId from the message, then nodeId (back-compat with bridge topics).
		nid := nodeID
		if nid == "" {
			if id, ok := raw["instanceId"].(string); ok && id != "" {
				nid = id
			} else if id, ok := raw["nodeId"].(string); ok && id != "" {
				nid = id
			}
		}

		// cpu_pct: cpuUsage.systemCPULoad is ALREADY an integer percent (0-100).
		// AMS SystemUtils.getSystemCpuLoad() does the [0,1]→percent conversion itself
		// ((int)(osBean.getSystemCpuLoad() * 100.0)) before StatsCollector serializes
		// it — do NOT multiply again (caught by S105 adversarial review: the *100
		// would have reported 4200% CPU against a real AMS).
		// We use systemCPULoad (whole-system view) rather than processCPULoad (JVM-only)
		// because node_stats is intended to represent node-level health, not JVM health.
		cpuObj := nestedMapField(raw, "cpuUsage")
		cpuPct := floatField(cpuObj, "systemCPULoad")

		// mem_pct: systemMemoryInfo.inUseMemory / totalMemory (guard div-by-zero).
		memObj := nestedMapField(raw, "systemMemoryInfo")
		memPct := pctSafe(floatField(memObj, "inUseMemory"), floatField(memObj, "totalMemory"))

		// disk_pct: fileSystemInfo.inUseSpace / totalSpace (guard div-by-zero).
		fsObj := nestedMapField(raw, "fileSystemInfo")
		diskPct := pctSafe(floatField(fsObj, "inUseSpace"), floatField(fsObj, "totalSpace"))

		return domain.ServerEvent{
			Version: 1,
			Type:    domain.EventNodeStats,
			TS:      ts,
			Source:  domain.SourceKafka,
			NodeID:  nid,
			App:     app,
			Data: map[string]any{
				"cpu_pct":  cpuPct,
				"mem_pct":  memPct,
				"disk_pct": diskPct,
			},
		}, nil

	case "ams-webrtc-stats":
		// Per-viewer WebRTC client stats (StatsCollector.java sendWebRTCClientStats2Kafka).
		// Fields: webrtcClientId, measured_bitrate, send_bitrate, audioFrameSendPeriod,
		// videoFrameSendPeriod, ipAddress, hostAddress, time.
		// This shape does not map cleanly to EventIngestStats (ingest-side per-stream
		// metrics) or EventStreamStats (viewer counts) — skip to avoid fabricating metrics.
		return domain.ServerEvent{}, errSkipMessage

	default:
		// Legacy field-sniffing: back-compat for operators feeding a custom bridge topic
		// that emits flat fields. Routing by field presence, unchanged from the original
		// implementation.
		//
		// Node identity: configured NodeID wins; fallback to nodeId in message.
		nid := nodeID
		if nid == "" {
			if n, ok := raw["nodeId"].(string); ok && n != "" {
				nid = n
			}
		}

		var evType string
		var data map[string]any

		switch {
		case hasKey(raw, "cpuUsage"):
			// Node stats message (flat format from custom bridge topics).
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
			// FIX 4 (VD-??): dashViewerCount was omitted from the sum, causing Kafka
			// and REST paths to disagree. Add it to match NormalizeBroadcast behaviour.
			evType = domain.EventStreamStats
			total := int(floatField(raw, "hlsViewerCount")) +
				int(floatField(raw, "webRTCViewerCount")) +
				int(floatField(raw, "rtmpViewerCount")) +
				int(floatField(raw, "dashViewerCount"))
			data = map[string]any{
				"viewer_count": total,
				"viewer_count_by_protocol": map[string]any{
					"webrtc": int(floatField(raw, "webRTCViewerCount")),
					"hls":    int(floatField(raw, "hlsViewerCount")),
					"rtmp":   int(floatField(raw, "rtmpViewerCount")),
					"dash":   int(floatField(raw, "dashViewerCount")),
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
}

// nestedMapField extracts a nested map from m[key].
// Returns nil if the key is absent or the value is not map[string]any.
func nestedMapField(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok {
		return nil
	}
	nested, _ := v.(map[string]any)
	return nested
}

// pctSafe computes inUse/total*100 with a div-by-zero guard.
// Returns 0 when total is 0; caps the result at 100.
func pctSafe(inUse, total float64) float64 {
	if total == 0 {
		return 0
	}
	v := inUse / total * 100
	if v > 100 {
		v = 100
	}
	return v
}

func hasKey(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

func floatField(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
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
