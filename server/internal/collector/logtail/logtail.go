// Package logtail tails the AMS analytics log
// (/var/log/antmedia/ant-media-server-analytics.log, structured JSON, AMS
// v2.10+) with rotation awareness, and emits normalized events. Richer than
// REST polling for keyframe/bitrate ingest health (F4).
package logtail

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// Config holds logtail configuration.
type Config struct {
	// LogPath is the path to the AMS analytics log file.
	LogPath string

	// NodeID is stamped on all emitted events.
	NodeID string

	// PollInterval is how often to check for new data / rotation (default 1 s).
	PollInterval time.Duration
}

// Tailer implements collector.Source by tailing the AMS analytics log.
type Tailer struct {
	cfg    Config
	sink   domain.EventSink
	logger *slog.Logger

	// Metrics counters.
	linesProcessed atomic.Int64
	parseErrors    atomic.Int64
	unknownTypes   atomic.Int64
	rotations      atomic.Int64
}

// New creates a Tailer.
func New(cfg Config, sink domain.EventSink, logger *slog.Logger) *Tailer {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.NodeID == "" {
		cfg.NodeID = "standalone"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Tailer{cfg: cfg, sink: sink, logger: logger}
}

// Name implements collector.Source.
func (t *Tailer) Name() string {
	return fmt.Sprintf("logtail(%s)", t.cfg.LogPath)
}

// Run implements collector.Source. It opens the log file and tails it until
// ctx is cancelled. Handles rotation (inode change / truncation) gracefully.
func (t *Tailer) Run(ctx context.Context) error {
	f, err := os.Open(t.cfg.LogPath)
	if err != nil {
		return fmt.Errorf("logtail: open %s: %w", t.cfg.LogPath, err)
	}
	defer f.Close()

	// Seek to end on initial open — we don't replay history.
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("logtail: seek: %w", err)
	}

	var currentInode uint64
	if fi, err := f.Stat(); err == nil {
		currentInode = inode(fi)
	}

	reader := bufio.NewReaderSize(f, 64*1024)
	ticker := time.NewTicker(t.cfg.PollInterval)
	defer ticker.Stop()

	var lineBuf []byte

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		// Check for rotation (inode change or truncation).
		if rotated, newFile := t.detectRotation(f, currentInode); rotated {
			t.rotations.Add(1)
			t.logger.Info("logtail: rotation detected, reopening", "path", t.cfg.LogPath)
			_ = f.Close()
			f = newFile
			currentInode = inode(mustStat(f))
			reader = bufio.NewReaderSize(f, 64*1024)
			lineBuf = nil
			continue
		}

		// Read all available lines.
		for {
			line, err := reader.ReadBytes('\n')
			lineBuf = append(lineBuf, line...)

			if err == io.EOF {
				// Partial line — wait for more data.
				break
			}
			if err != nil {
				t.logger.Warn("logtail: read error", "error", err)
				break
			}

			// We have a complete line (ends with \n).
			t.processLine(lineBuf)
			lineBuf = nil
		}
	}
}

// processLine parses and emits one JSON log line.
func (t *Tailer) processLine(line []byte) {
	t.linesProcessed.Add(1)

	// Trim whitespace.
	if len(line) == 0 {
		return
	}

	// Parse as generic JSON map for forward compatibility.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		t.parseErrors.Add(1)
		t.logger.Debug("logtail: parse error", "error", err)
		return
	}

	eventType := jsonString(raw["event"])
	if eventType == "" {
		eventType = jsonString(raw["type"])
	}
	if eventType == "" {
		t.parseErrors.Add(1)
		return
	}

	ev, ok := t.translateLogEvent(eventType, raw)
	if !ok {
		t.unknownTypes.Add(1)
		t.logger.Debug("logtail: unknown event type, skipping", "type", eventType)
		return
	}

	t.sink.WriteServerEvent(ev)
}

// translateLogEvent maps an AMS analytics log event to a domain.ServerEvent.
// Returns (event, true) on success, or (_, false) for unknown types.
func (t *Tailer) translateLogEvent(eventType string, raw map[string]json.RawMessage) (domain.ServerEvent, bool) {
	// Extract common fields from AMS log line.
	streamID := jsonString(raw["streamId"])
	app := jsonString(raw["appName"])
	if app == "" {
		app = jsonString(raw["app"])
	}
	tsMs := jsonInt64(raw["ts"])
	if tsMs == 0 {
		tsMs = time.Now().UnixMilli()
	}

	ev := domain.ServerEvent{
		Version:  1,
		TS:       tsMs,
		Source:   domain.SourceLogTail,
		NodeID:   t.cfg.NodeID,
		App:      app,
		StreamID: streamID,
	}

	switch eventType {
	case "publish_started", "startBroadcast":
		publishType := normalizeLogPublishType(jsonString(raw["publishType"]))
		ev.Type = domain.EventStreamPublishStart
		ev.Data = map[string]any{"publish_type": publishType}

	case "publish_ended", "stopBroadcast":
		ev.Type = domain.EventStreamPublishEnd
		ev.Data = map[string]any{
			"duration_s": jsonInt(raw["duration"]),
			"reason":     jsonString(raw["reason"]),
		}

	case "stream_stats", "broadcastStats":
		vc := jsonInt(raw["totalHlsViewerCount"]) +
			jsonInt(raw["webRTCViewerCount"]) +
			jsonInt(raw["rtmpViewerCount"]) +
			jsonInt(raw["dashViewerCount"])
		ev.Type = domain.EventStreamStats
		ev.Data = map[string]any{
			"viewer_count": vc,
			"viewer_count_by_protocol": map[string]any{
				"hls":    jsonInt(raw["totalHlsViewerCount"]),
				"webrtc": jsonInt(raw["webRTCViewerCount"]),
				"rtmp":   jsonInt(raw["rtmpViewerCount"]),
				"dash":   jsonInt(raw["dashViewerCount"]),
				"other":  0,
			},
			"bitrate_kbps":    jsonFloat(raw["bitrate"]),
			"speed_read_kbps": jsonFloat(raw["speed"]),
		}

	case "ingest_stats":
		ev.Type = domain.EventIngestStats
		ev.Data = map[string]any{
			"bitrate_kbps":        jsonFloat(raw["bitrate"]),
			"fps":                 jsonFloat(raw["fps"]),
			"keyframe_interval_s": jsonFloat(raw["keyFrameInterval"]),
			"packet_loss_pct":     jsonFloat(raw["packetLostRatio"]),
			"jitter_ms":           jsonFloat(raw["jitter"]),
		}

	case "viewer_join", "viewerJoin":
		ev.Type = domain.EventViewerJoin
		ev.Data = map[string]any{
			"viewer_id":  jsonString(raw["viewerId"]),
			"protocol":   normalizeLogPublishType(jsonString(raw["protocol"])),
			"ip_hash":    hashIP(jsonString(raw["ip"])),
			"user_agent": jsonString(raw["userAgent"]),
		}

	case "viewer_leave", "viewerLeave":
		ev.Type = domain.EventViewerLeave
		ev.Data = map[string]any{
			"viewer_id":    jsonString(raw["viewerId"]),
			"protocol":     normalizeLogPublishType(jsonString(raw["protocol"])),
			"watch_time_s": jsonInt(raw["watchTime"]),
		}

	case "recording_ready", "vodReady":
		ev.Type = domain.EventRecordingReady
		ev.Data = map[string]any{
			"path":       jsonString(raw["filePath"]),
			"size_bytes": jsonInt64(raw["fileSize"]),
			"duration_s": jsonInt(raw["duration"]),
		}

	default:
		return domain.ServerEvent{}, false
	}

	return ev, true
}

// detectRotation checks if the file has been rotated (inode change or truncation).
// Returns (true, newFile) if rotation is detected.
func (t *Tailer) detectRotation(f *os.File, currentInode uint64) (bool, *os.File) {
	// Check if the file on disk has a different inode.
	fi, err := os.Stat(t.cfg.LogPath)
	if err != nil {
		return false, nil
	}
	if inode(fi) != currentInode {
		newFile, err := os.Open(t.cfg.LogPath)
		if err != nil {
			return false, nil
		}
		return true, newFile
	}

	// Check for truncation: current seek position > file size.
	pos, _ := f.Seek(0, io.SeekCurrent)
	if pos > fi.Size() {
		// Truncated — reopen from start.
		newFile, err := os.Open(t.cfg.LogPath)
		if err != nil {
			return false, nil
		}
		return true, newFile
	}

	return false, nil
}

// Metrics exposes internal counters for observability.
func (t *Tailer) Metrics() (lines, parseErrors, unknownTypes, rotations int64) {
	return t.linesProcessed.Load(), t.parseErrors.Load(), t.unknownTypes.Load(), t.rotations.Load()
}

// ─── JSON helpers ─────────────────────────────────────────────────────────────

func jsonString(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func jsonInt(raw json.RawMessage) int {
	if raw == nil {
		return 0
	}
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0
	}
	return n
}

func jsonInt64(raw json.RawMessage) int64 {
	if raw == nil {
		return 0
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0
	}
	return n
}

func jsonFloat(raw json.RawMessage) float64 {
	if raw == nil {
		return 0
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0
	}
	return f
}

func normalizeLogPublishType(t string) string {
	switch t {
	case "webrtc", "WebRTC", "WEBRTC":
		return "webrtc"
	case "rtmp", "RTMP":
		return "rtmp"
	case "hls", "HLS":
		return "hls"
	case "dash", "DASH":
		return "dash"
	default:
		if t == "" {
			return "other"
		}
		return "other"
	}
}

func hashIP(ip string) string {
	if ip == "" {
		return ""
	}
	h := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(h[:])
}

func inode(fi os.FileInfo) uint64 {
	return inodeFromStat(fi)
}

func mustStat(f *os.File) os.FileInfo {
	fi, err := f.Stat()
	if err != nil {
		return nil
	}
	return fi
}
