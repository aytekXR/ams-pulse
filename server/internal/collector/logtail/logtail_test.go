// Logtail tests: malformed line, unknown type, counter increments, no crash.
package logtail_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/collector/logtail"
	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// sinkCapture captures events for assertions.
type sinkCapture struct {
	mu     sync.Mutex
	events []domain.ServerEvent
}

func (s *sinkCapture) WriteServerEvent(ev domain.ServerEvent) {
	s.mu.Lock()
	s.events = append(s.events, ev)
	s.mu.Unlock()
}
func (s *sinkCapture) WriteBeaconEvent(_ domain.BeaconEvent)     {}
func (s *sinkCapture) WriteViewerSession(_ domain.ViewerSession) {}

func (s *sinkCapture) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

// TestLogtail_MalformedLine verifies that malformed JSON lines don't crash the
// tailer and increment the parse error counter.
func TestLogtail_MalformedLine(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	f, _ := os.Create(logPath)
	f.Close()

	sink := &sinkCapture{}
	tailer := logtail.NewForTest(logtail.Config{
		LogPath: logPath,
		NodeID:  "node1",
	}, sink, nil)

	// Process malformed line — must not panic.
	tailer.ProcessLineForTest([]byte(`not valid json {{{`))

	_, parseErrors, _, _ := tailer.Metrics()
	if parseErrors == 0 {
		t.Error("expected parseErrors > 0 for malformed JSON line")
	}
	if sink.count() != 0 {
		t.Errorf("expected 0 events for malformed line, got %d", sink.count())
	}
}

// TestLogtail_UnknownType verifies unknown event types are skipped and counted.
func TestLogtail_UnknownType(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	f, _ := os.Create(logPath)
	f.Close()

	sink := &sinkCapture{}
	tailer := logtail.NewForTest(logtail.Config{
		LogPath: logPath,
		NodeID:  "node1",
	}, sink, nil)

	tailer.ProcessLineForTest([]byte(`{"event":"ams_v99_unknown_event","streamId":"x","ts":1234}`))

	_, _, unknownTypes, _ := tailer.Metrics()
	if unknownTypes == 0 {
		t.Error("expected unknownTypes > 0 for unknown event type")
	}
	if sink.count() != 0 {
		t.Errorf("expected 0 events for unknown type, got %d", sink.count())
	}
}

// TestLogtail_ValidEvents verifies that known event types are translated correctly.
func TestLogtail_ValidEvents(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	f, _ := os.Create(logPath)
	f.Close()

	sink := &sinkCapture{}
	tailer := logtail.NewForTest(logtail.Config{
		LogPath: logPath,
		NodeID:  "node1",
	}, sink, nil)

	lines := [][]byte{
		[]byte(`{"event":"publish_started","streamId":"s1","appName":"live","ts":1700000000000,"publishType":"rtmp"}`),
		[]byte(`{"event":"publish_ended","streamId":"s1","appName":"live","ts":1700000060000,"duration":60}`),
		[]byte(`{"event":"stream_stats","streamId":"s1","appName":"live","ts":1700000010000,"totalHlsViewerCount":5,"webRTCViewerCount":2,"bitrate":2500.0}`),
	}
	for _, line := range lines {
		tailer.ProcessLineForTest(line)
	}

	if sink.count() != 3 {
		t.Errorf("expected 3 events, got %d", sink.count())
	}
}

// TestLogtail_MixedLines verifies combined malformed + valid + unknown lines.
func TestLogtail_MixedLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	f, _ := os.Create(logPath)
	f.Close()

	sink := &sinkCapture{}
	tailer := logtail.NewForTest(logtail.Config{
		LogPath: logPath,
		NodeID:  "node1",
	}, sink, nil)

	inputs := [][]byte{
		[]byte(`{"event":"publish_started","streamId":"s1","appName":"live","ts":1700000000000,"publishType":"webrtc"}`),
		[]byte(`{bad json`),
		[]byte(`{"event":"future_unknown","ts":1234}`),
		[]byte(`{"event":"publish_ended","streamId":"s1","appName":"live","ts":1700000030000}`),
	}
	for _, line := range inputs {
		tailer.ProcessLineForTest(line)
	}

	lines, parseErrors, unknownTypes, _ := tailer.Metrics()
	t.Logf("lines=%d parseErrors=%d unknownTypes=%d events=%d", lines, parseErrors, unknownTypes, sink.count())

	if parseErrors == 0 {
		t.Error("expected at least 1 parse error")
	}
	if unknownTypes == 0 {
		t.Error("expected at least 1 unknown type")
	}
	if sink.count() != 2 {
		t.Errorf("expected 2 valid events (publish_start + publish_end), got %d", sink.count())
	}
}
