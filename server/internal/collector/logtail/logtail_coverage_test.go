// Coverage-focused tests for package logtail.
// Targets gaps identified from the baseline coverage report:
//   - New() defaults, Name()
//   - json* helpers: nil / type-mismatch branches
//   - translateLogEvent: all remaining event-type aliases
//   - normalizeLogPublishType: hls, dash, unknown, empty-string branches
//   - hashIP
//   - Run(): context-cancel path, file-not-found error, rotation detection
package logtail_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/collector/logtail"
	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// newTS creates a fresh tailer + sink pair backed by an empty temp file.
// The temp dir is registered for automatic cleanup via t.TempDir().
func newTS(t *testing.T) (*logtail.Tailer, *sinkCapture) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "ams.log")
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	sink := &sinkCapture{}
	tailer := logtail.NewForTest(logtail.Config{
		LogPath:      logPath,
		NodeID:       "node-test",
		PollInterval: 10 * time.Millisecond,
	}, sink, nil)
	return tailer, sink
}

// ─── New() constructor defaults ───────────────────────────────────────────────

// TestNew_Defaults verifies that New() fills in NodeID when callers pass a
// zero-value Config.  The assertion on ev.NodeID == "standalone" would FAIL if
// the `cfg.NodeID = "standalone"` default-assignment is removed from New().
func TestNew_Defaults(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "a.log")
	f, _ := os.Create(logPath)
	f.Close()

	sink := &sinkCapture{}
	// Pass zero-value Config (empty NodeID) — New() must default it to "standalone".
	tailer := logtail.New(logtail.Config{LogPath: logPath}, sink, nil)

	tailer.ProcessLineForTest([]byte(`{"event":"publish_started","streamId":"s1","ts":1700000000000}`))
	if sink.count() != 1 {
		t.Fatalf("expected 1 event after New() with zero Config, got %d", sink.count())
	}
	// This is the real regression guard: ev.NodeID must carry the default value.
	if got := sink.events[0].NodeID; got != "standalone" {
		t.Errorf("NodeID default: expected %q, got %q — New() did not apply the NodeID default", "standalone", got)
	}
}

// ─── Name() ───────────────────────────────────────────────────────────────────

// TestName verifies the Name() format includes the configured path.
func TestName(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "mylog.log")
	f, _ := os.Create(logPath)
	f.Close()
	sink := &sinkCapture{}
	tailer := logtail.NewForTest(logtail.Config{LogPath: logPath, NodeID: "n1"}, sink, nil)
	name := tailer.Name()
	if !strings.Contains(name, logPath) {
		t.Errorf("Name() = %q does not contain log path %q", name, logPath)
	}
	if !strings.HasPrefix(name, "logtail(") {
		t.Errorf("Name() = %q does not start with 'logtail('", name)
	}
}

// ─── JSON helper nil / type-mismatch branches ─────────────────────────────────

// TestJsonHelpers_NilFields verifies that absent JSON fields produce zero values
// rather than crashes. Each json* helper has an early-return for nil raw.
// We exercise them through processLine with a stream_stats line that omits all
// numeric fields (jsonInt / jsonFloat nil paths) and a line that omits "ts"
// (jsonInt64 nil path).
func TestJsonHelpers_NilFields(t *testing.T) {
	t.Run("jsonInt nil path via absent viewer counts", func(t *testing.T) {
		tailer, sink := newTS(t)
		// No totalHlsViewerCount / webRTCViewerCount / etc → jsonInt(nil) for each.
		tailer.ProcessLineForTest([]byte(`{"event":"stream_stats","streamId":"s1","ts":1700000000000}`))
		if sink.count() != 1 {
			t.Fatalf("expected 1 event, got %d", sink.count())
		}
		vc, _ := sink.events[0].Data["viewer_count"].(int)
		if vc != 0 {
			t.Errorf("expected viewer_count=0 when all count fields absent, got %d", vc)
		}
	})

	t.Run("jsonFloat nil path via absent bitrate", func(t *testing.T) {
		tailer, sink := newTS(t)
		tailer.ProcessLineForTest([]byte(`{"event":"ingest_stats","streamId":"s1","ts":1700000000000}`))
		if sink.count() != 1 {
			t.Fatalf("expected 1 event, got %d", sink.count())
		}
		bk, _ := sink.events[0].Data["bitrate_kbps"].(float64)
		if bk != 0 {
			t.Errorf("expected bitrate_kbps=0.0 for absent field, got %f", bk)
		}
	})

	t.Run("jsonInt64 nil path via absent ts uses time.Now() fallback", func(t *testing.T) {
		before := time.Now().UnixMilli()
		tailer, sink := newTS(t)
		// Omit "ts" entirely — jsonInt64(nil) returns 0 → fallback to time.Now().
		tailer.ProcessLineForTest([]byte(`{"event":"publish_started","streamId":"s1"}`))
		after := time.Now().UnixMilli()
		if sink.count() != 1 {
			t.Fatalf("expected 1 event, got %d", sink.count())
		}
		ts := sink.events[0].TS
		if ts < before || ts > after {
			t.Errorf("expected TS in [%d, %d] (time.Now fallback), got %d", before, after, ts)
		}
	})
}

// TestJsonHelpers_TypeMismatch verifies that a JSON field with the wrong type
// (e.g., a string where a number is expected) causes the json* helper to return
// zero — not a process-level parse error, because the outer object is valid JSON.
func TestJsonHelpers_TypeMismatch(t *testing.T) {
	t.Run("jsonFloat error path: bitrate as string", func(t *testing.T) {
		tailer, sink := newTS(t)
		tailer.ProcessLineForTest([]byte(`{"event":"ingest_stats","streamId":"s1","ts":1700000000000,"bitrate":"not-a-number"}`))
		// Event must still be emitted.
		if sink.count() != 1 {
			t.Fatalf("type-mismatch in sub-field must not drop event; got %d events", sink.count())
		}
		bk, _ := sink.events[0].Data["bitrate_kbps"].(float64)
		if bk != 0 {
			t.Errorf("expected bitrate_kbps=0 for string-valued bitrate, got %f", bk)
		}
		// Must not increment outer parse error counter.
		_, pe, _, _ := tailer.Metrics()
		if pe != 0 {
			t.Errorf("type-mismatch in sub-field must not increment parseErrors, got %d", pe)
		}
	})

	t.Run("jsonInt error path: viewer count as string", func(t *testing.T) {
		tailer, sink := newTS(t)
		tailer.ProcessLineForTest([]byte(`{"event":"stream_stats","streamId":"s1","ts":1700000000000,"webRTCViewerCount":"bad"}`))
		if sink.count() != 1 {
			t.Fatalf("expected 1 event, got %d", sink.count())
		}
		vc, _ := sink.events[0].Data["viewer_count"].(int)
		if vc != 0 {
			t.Errorf("expected viewer_count=0 for string-typed count field, got %d", vc)
		}
	})

	t.Run("jsonInt64 error path: fileSize as string", func(t *testing.T) {
		tailer, sink := newTS(t)
		tailer.ProcessLineForTest([]byte(`{"event":"recording_ready","streamId":"s1","ts":1700000000000,"filePath":"/a/b.mp4","fileSize":"huge"}`))
		if sink.count() != 1 {
			t.Fatalf("expected 1 event, got %d", sink.count())
		}
		size, _ := sink.events[0].Data["size_bytes"].(int64)
		if size != 0 {
			t.Errorf("expected size_bytes=0 for string-typed fileSize, got %d", size)
		}
	})

	t.Run("jsonString error path: viewerId as JSON number", func(t *testing.T) {
		tailer, sink := newTS(t)
		tailer.ProcessLineForTest([]byte(`{"event":"viewer_join","streamId":"s1","ts":1700000000000,"viewerId":42,"protocol":"hls"}`))
		if sink.count() != 1 {
			t.Fatalf("expected 1 event, got %d", sink.count())
		}
		vid, _ := sink.events[0].Data["viewer_id"].(string)
		if vid != "" {
			t.Errorf("expected viewer_id=\"\" when JSON number given for string field, got %q", vid)
		}
	})
}

// ─── translateLogEvent: uncovered event types and aliases ─────────────────────

// TestTranslateLogEvent_AllAliases verifies that both the canonical and AMS v1
// alias event names map to the correct domain event types.
func TestTranslateLogEvent_AllAliases(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantType string
	}{
		// AMS v1 alias for publish_started
		{
			name:     "startBroadcast → EventStreamPublishStart",
			line:     `{"event":"startBroadcast","streamId":"s1","appName":"live","ts":1700000001000}`,
			wantType: domain.EventStreamPublishStart,
		},
		// AMS v1 alias for publish_ended
		{
			name:     "stopBroadcast → EventStreamPublishEnd",
			line:     `{"event":"stopBroadcast","streamId":"s1","appName":"live","ts":1700000002000,"duration":30}`,
			wantType: domain.EventStreamPublishEnd,
		},
		// AMS v1 alias for stream_stats
		{
			name:     "broadcastStats → EventStreamStats",
			line:     `{"event":"broadcastStats","streamId":"s1","appName":"live","ts":1700000003000,"totalHlsViewerCount":3}`,
			wantType: domain.EventStreamStats,
		},
		// Canonical ingest_stats (not tested at all in existing suite)
		{
			name:     "ingest_stats → EventIngestStats",
			line:     `{"event":"ingest_stats","streamId":"s1","ts":1700000004000,"bitrate":3500.5,"fps":30.0,"keyFrameInterval":2.0,"packetLostRatio":0.01,"jitter":5.5}`,
			wantType: domain.EventIngestStats,
		},
		// Canonical viewer_join
		{
			name:     "viewer_join → EventViewerJoin",
			line:     `{"event":"viewer_join","streamId":"s1","ts":1700000005000,"viewerId":"v1","protocol":"webrtc","ip":"10.0.0.1"}`,
			wantType: domain.EventViewerJoin,
		},
		// AMS v1 alias for viewer_join
		{
			name:     "viewerJoin → EventViewerJoin",
			line:     `{"event":"viewerJoin","streamId":"s1","ts":1700000006000,"viewerId":"v2","protocol":"hls"}`,
			wantType: domain.EventViewerJoin,
		},
		// Canonical viewer_leave
		{
			name:     "viewer_leave → EventViewerLeave",
			line:     `{"event":"viewer_leave","streamId":"s1","ts":1700000007000,"viewerId":"v1","protocol":"hls","watchTime":120}`,
			wantType: domain.EventViewerLeave,
		},
		// AMS v1 alias for viewer_leave
		{
			name:     "viewerLeave → EventViewerLeave",
			line:     `{"event":"viewerLeave","streamId":"s1","ts":1700000008000,"viewerId":"v2","protocol":"rtmp","watchTime":60}`,
			wantType: domain.EventViewerLeave,
		},
		// Canonical recording_ready
		{
			name:     "recording_ready → EventRecordingReady",
			line:     `{"event":"recording_ready","streamId":"s1","ts":1700000009000,"filePath":"/vod/s1.mp4","fileSize":1048576,"duration":60}`,
			wantType: domain.EventRecordingReady,
		},
		// AMS v1 alias for recording_ready
		{
			name:     "vodReady → EventRecordingReady",
			line:     `{"event":"vodReady","streamId":"s1","ts":1700000010000,"filePath":"/vod/s2.mp4","fileSize":2097152,"duration":120}`,
			wantType: domain.EventRecordingReady,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tailer, sink := newTS(t)
			tailer.ProcessLineForTest([]byte(tc.line))
			if sink.count() != 1 {
				t.Fatalf("expected 1 event, got %d", sink.count())
			}
			got := sink.events[0].Type
			if got != tc.wantType {
				t.Errorf("expected event type %q, got %q", tc.wantType, got)
			}
		})
	}
}

// ─── ingest_stats payload detail ──────────────────────────────────────────────

// TestIngestStats_Payload verifies all float fields are extracted correctly.
func TestIngestStats_Payload(t *testing.T) {
	tailer, sink := newTS(t)
	tailer.ProcessLineForTest([]byte(`{"event":"ingest_stats","streamId":"s1","appName":"live","ts":1700000000000,"bitrate":3500.5,"fps":30.0,"keyFrameInterval":2.0,"packetLostRatio":0.01,"jitter":5.5}`))
	if sink.count() != 1 {
		t.Fatalf("expected 1 event, got %d", sink.count())
	}
	data := sink.events[0].Data
	check := func(key string, want float64) {
		t.Helper()
		got, _ := data[key].(float64)
		if got != want {
			t.Errorf("data[%q] = %f, want %f", key, got, want)
		}
	}
	check("bitrate_kbps", 3500.5)
	check("fps", 30.0)
	check("keyframe_interval_s", 2.0)
	check("packet_loss_pct", 0.01)
	check("jitter_ms", 5.5)
}

// ─── viewer_join / viewer_leave payload detail ────────────────────────────────

// TestViewerJoin_IPHashAndProtocol verifies IP hashing and protocol normalization.
func TestViewerJoin_IPHashAndProtocol(t *testing.T) {
	t.Run("IP is hashed not stored raw", func(t *testing.T) {
		tailer, sink := newTS(t)
		tailer.ProcessLineForTest([]byte(`{"event":"viewer_join","streamId":"s1","ts":1700000000000,"viewerId":"v1","protocol":"rtmp","ip":"192.168.1.1","userAgent":"Go-test"}`))
		if sink.count() != 1 {
			t.Fatalf("expected 1 event, got %d", sink.count())
		}
		ipHash, _ := sink.events[0].Data["ip_hash"].(string)
		if ipHash == "192.168.1.1" {
			t.Error("ip_hash must not contain the raw IP address")
		}
		if len(ipHash) != 64 {
			t.Errorf("ip_hash must be a 64-char hex SHA-256, got %q (len=%d)", ipHash, len(ipHash))
		}
	})

	t.Run("absent IP produces empty hash", func(t *testing.T) {
		tailer, sink := newTS(t)
		tailer.ProcessLineForTest([]byte(`{"event":"viewer_join","streamId":"s1","ts":1700000000000,"viewerId":"v2","protocol":"hls"}`))
		if sink.count() != 1 {
			t.Fatalf("expected 1 event, got %d", sink.count())
		}
		ipHash, _ := sink.events[0].Data["ip_hash"].(string)
		if ipHash != "" {
			t.Errorf("expected empty ip_hash for absent ip field, got %q", ipHash)
		}
	})
}

// TestViewerLeave_WatchTime verifies watchTime is extracted into watch_time_s.
func TestViewerLeave_WatchTime(t *testing.T) {
	tailer, sink := newTS(t)
	tailer.ProcessLineForTest([]byte(`{"event":"viewer_leave","streamId":"s1","ts":1700000000000,"viewerId":"v1","protocol":"hls","watchTime":300}`))
	if sink.count() != 1 {
		t.Fatalf("expected 1 event, got %d", sink.count())
	}
	wt, _ := sink.events[0].Data["watch_time_s"].(int)
	if wt != 300 {
		t.Errorf("expected watch_time_s=300, got %d", wt)
	}
}

// ─── recording_ready payload detail ──────────────────────────────────────────

// TestRecordingReady_Payload verifies path, size_bytes (int64), and duration.
func TestRecordingReady_Payload(t *testing.T) {
	tailer, sink := newTS(t)
	tailer.ProcessLineForTest([]byte(`{"event":"recording_ready","streamId":"s1","ts":1700000000000,"filePath":"/vod/s1.mp4","fileSize":1048576,"duration":120}`))
	if sink.count() != 1 {
		t.Fatalf("expected 1 event, got %d", sink.count())
	}
	ev := sink.events[0]
	path, _ := ev.Data["path"].(string)
	if path != "/vod/s1.mp4" {
		t.Errorf("expected path='/vod/s1.mp4', got %q", path)
	}
	size, _ := ev.Data["size_bytes"].(int64)
	if size != 1048576 {
		t.Errorf("expected size_bytes=1048576, got %d", size)
	}
	dur, _ := ev.Data["duration_s"].(int)
	if dur != 120 {
		t.Errorf("expected duration_s=120, got %d", dur)
	}
}

// ─── normalizeLogPublishType: uncovered branches ──────────────────────────────

// TestNormalizePublishType exercises every branch of normalizeLogPublishType via
// the viewer_join protocol field (which calls the same helper).
func TestNormalizePublishType(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		// Already covered by other tests but included for completeness / documentation.
		{"webrtc", "webrtc"},
		{"WebRTC", "webrtc"},
		{"WEBRTC", "webrtc"},
		{"rtmp", "rtmp"},
		{"RTMP", "rtmp"},
		// Not covered by existing tests.
		{"hls", "hls"},
		{"HLS", "hls"},
		{"dash", "dash"},
		{"DASH", "dash"},
		{"", "other"},
		{"mp4", "other"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run("protocol="+tc.raw, func(t *testing.T) {
			tailer, sink := newTS(t)
			line := `{"event":"viewer_join","streamId":"s1","ts":1700000000000,"viewerId":"v1","protocol":"` + tc.raw + `"}`
			tailer.ProcessLineForTest([]byte(line))
			if sink.count() != 1 {
				t.Fatalf("expected 1 event, got %d", sink.count())
			}
			proto, _ := sink.events[0].Data["protocol"].(string)
			if proto != tc.want {
				t.Errorf("raw=%q: expected %q, got %q", tc.raw, tc.want, proto)
			}
		})
	}
}

// ─── processLine edge cases ───────────────────────────────────────────────────

// TestProcessLine_EmptyLine verifies that a zero-length line is not treated as a
// parse error and does not emit events.
func TestProcessLine_EmptyLine(t *testing.T) {
	tailer, sink := newTS(t)
	tailer.ProcessLineForTest([]byte{})
	if sink.count() != 0 {
		t.Errorf("expected 0 events for empty line, got %d", sink.count())
	}
	_, pe, _, _ := tailer.Metrics()
	if pe != 0 {
		t.Errorf("empty line must not increment parseErrors, got %d", pe)
	}
}

// TestProcessLine_TypeKeyFallback verifies that the "type" key is used when
// "event" is absent.
func TestProcessLine_TypeKeyFallback(t *testing.T) {
	tailer, sink := newTS(t)
	tailer.ProcessLineForTest([]byte(`{"type":"publish_started","streamId":"s1","ts":1700000000000}`))
	if sink.count() != 1 {
		t.Fatalf("expected 1 event via 'type' fallback key, got %d", sink.count())
	}
	if sink.events[0].Type != domain.EventStreamPublishStart {
		t.Errorf("wrong event type: %q", sink.events[0].Type)
	}
}

// TestProcessLine_AppKeyFallback verifies that "app" is used when "appName" is absent.
func TestProcessLine_AppKeyFallback(t *testing.T) {
	tailer, sink := newTS(t)
	tailer.ProcessLineForTest([]byte(`{"event":"publish_started","streamId":"s1","app":"myapp","ts":1700000000000}`))
	if sink.count() != 1 {
		t.Fatalf("expected 1 event, got %d", sink.count())
	}
	if sink.events[0].App != "myapp" {
		t.Errorf("expected App='myapp' via 'app' fallback key, got %q", sink.events[0].App)
	}
}

// TestProcessLine_NoEventOrTypeField verifies that valid JSON without an "event"
// or "type" field increments parseErrors and emits no event.
func TestProcessLine_NoEventOrTypeField(t *testing.T) {
	tailer, sink := newTS(t)
	tailer.ProcessLineForTest([]byte(`{"streamId":"s1","ts":1234567890}`))
	if sink.count() != 0 {
		t.Errorf("expected 0 events for JSON without event/type field, got %d", sink.count())
	}
	_, pe, _, _ := tailer.Metrics()
	if pe == 0 {
		t.Error("expected parseErrors > 0 for JSON without event/type field")
	}
}

// ─── NodeID / Source stamping ─────────────────────────────────────────────────

// TestNodeIDAndSource verifies that every emitted event carries the configured
// NodeID, the SourceLogTail source tag, and Version=1.
func TestNodeIDAndSource(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "ams.log")
	f, _ := os.Create(logPath)
	f.Close()
	sink := &sinkCapture{}
	tailer := logtail.NewForTest(logtail.Config{
		LogPath: logPath,
		NodeID:  "node-sentinel",
	}, sink, nil)
	tailer.ProcessLineForTest([]byte(`{"event":"publish_started","streamId":"s1","ts":1700000000000}`))
	if sink.count() != 1 {
		t.Fatalf("expected 1 event, got %d", sink.count())
	}
	ev := sink.events[0]
	if ev.NodeID != "node-sentinel" {
		t.Errorf("expected NodeID='node-sentinel', got %q", ev.NodeID)
	}
	if ev.Source != domain.SourceLogTail {
		t.Errorf("expected Source=%q, got %q", domain.SourceLogTail, ev.Source)
	}
	if ev.Version != 1 {
		t.Errorf("expected Version=1, got %d", ev.Version)
	}
}

// ─── stream_stats viewer count aggregation ────────────────────────────────────

// TestStreamStats_ViewerCountSum verifies that viewer_count is the sum of all
// four protocol viewer counts.
func TestStreamStats_ViewerCountSum(t *testing.T) {
	tailer, sink := newTS(t)
	tailer.ProcessLineForTest([]byte(`{"event":"stream_stats","streamId":"s1","ts":1700000000000,"totalHlsViewerCount":10,"webRTCViewerCount":5,"rtmpViewerCount":2,"dashViewerCount":1}`))
	if sink.count() != 1 {
		t.Fatalf("expected 1 event, got %d", sink.count())
	}
	vc, _ := sink.events[0].Data["viewer_count"].(int)
	if vc != 18 {
		t.Errorf("expected viewer_count=18 (10+5+2+1), got %d", vc)
	}
	byProto, _ := sink.events[0].Data["viewer_count_by_protocol"].(map[string]any)
	if byProto == nil {
		t.Fatal("expected viewer_count_by_protocol to be non-nil")
	}
	if hls, _ := byProto["hls"].(int); hls != 10 {
		t.Errorf("expected hls=10, got %d", hls)
	}
}

// ─── Run(): file-not-found error path ────────────────────────────────────────

// TestRun_FileNotFound verifies that Run() returns a non-nil error immediately
// when the log file cannot be opened (covers the error path at the top of Run).
func TestRun_FileNotFound(t *testing.T) {
	sink := &sinkCapture{}
	tailer := logtail.NewForTest(logtail.Config{
		LogPath:      filepath.Join(t.TempDir(), "nonexistent.log"),
		NodeID:       "n1",
		PollInterval: 10 * time.Millisecond,
	}, sink, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := tailer.Run(ctx)
	if err == nil {
		t.Error("expected non-nil error from Run() when log file does not exist")
	}
}

// ─── Run(): context cancel and rotation detection ─────────────────────────────

// TestRun_ContextCancel verifies that Run() exits cleanly (nil error) when the
// context is cancelled, exercising the `<-ctx.Done()` select arm.
func TestRun_ContextCancel(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "ams.log")
	f, _ := os.Create(logPath)
	f.Close()

	sink := &sinkCapture{}
	tailer := logtail.NewForTest(logtail.Config{
		LogPath:      logPath,
		NodeID:       "n1",
		PollInterval: 5 * time.Millisecond,
	}, sink, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := tailer.Run(ctx)
	if err != nil {
		t.Errorf("Run() on context cancel should return nil, got: %v", err)
	}
}

// TestRun_RotationDetected verifies the file-rotation path: when the log file is
// replaced by a file with a different inode, Run() detects the rotation and
// increments its rotation counter.
func TestRun_RotationDetected(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "ams.log")

	// Create the initial log file.
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	sink := &sinkCapture{}
	tailer := logtail.NewForTest(logtail.Config{
		LogPath:      logPath,
		NodeID:       "n1",
		PollInterval: 5 * time.Millisecond,
	}, sink, nil)

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- tailer.Run(ctx) }()

	// Allow Run() to open the file and enter the poll loop (generous to survive CI load).
	time.Sleep(100 * time.Millisecond)

	// Simulate log rotation: rename the current file and create a fresh one.
	if err := os.Rename(logPath, logPath+".1"); err != nil {
		cancel()
		<-runDone
		t.Fatal(err)
	}
	nf, err := os.Create(logPath)
	if err != nil {
		cancel()
		<-runDone
		t.Fatal(err)
	}
	nf.Close()

	// Poll until detectRotation fires (up to 2 s), then cancel.
	// Using a retry loop avoids flakes under CI load while keeping fast success.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, _, _, rotations := tailer.Metrics()
		if rotations > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel()
	select {
	case runErr := <-runDone:
		if runErr != nil {
			t.Errorf("Run() returned unexpected error: %v", runErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}

	_, _, _, rotations := tailer.Metrics()
	if rotations == 0 {
		t.Error("expected rotations > 0 after inode-change rotation")
	}
}

// TestRun_PicksUpNewLines verifies that Run() reads lines appended to the log
// file after the initial seek-to-end and emits them via the sink.
func TestRun_PicksUpNewLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "ams.log")

	// Create empty log file.
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	sink := &sinkCapture{}
	tailer := logtail.NewForTest(logtail.Config{
		LogPath:      logPath,
		NodeID:       "n1",
		PollInterval: 5 * time.Millisecond,
	}, sink, nil)

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- tailer.Run(ctx) }()

	// Wait for Run() to open and seek to end (generous to survive CI load).
	time.Sleep(100 * time.Millisecond)

	// Append a JSON line after Run() has seeked to EOF.
	line := `{"event":"publish_started","streamId":"s2","appName":"live","ts":1700000000000}` + "\n"
	appF, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		cancel()
		<-runDone
		t.Fatal(err)
	}
	if _, err := appF.WriteString(line); err != nil {
		appF.Close()
		cancel()
		<-runDone
		t.Fatal(err)
	}
	appF.Close()

	// Poll until the tailer picks up the line (up to 2 s) instead of a fixed sleep,
	// so the test succeeds quickly on fast machines and survives load on CI.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && sink.count() < 1 {
		time.Sleep(5 * time.Millisecond)
	}

	cancel()
	select {
	case runErr := <-runDone:
		if runErr != nil {
			t.Errorf("Run() returned unexpected error: %v", runErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}

	if sink.count() < 1 {
		t.Errorf("expected at least 1 event for line appended after Run() started, got %d", sink.count())
	}
}
