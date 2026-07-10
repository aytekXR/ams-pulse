package prober_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/prober"
)

// ─── Fake clock ───────────────────────────────────────────────────────────────

// FakeClock is a deterministic clock for testing the interval loop.
// After(0) returns an already-closed channel (fires immediately, no Advance needed).
type FakeClock struct {
	mu      sync.Mutex
	current time.Time
	waiters []fakeCh
}

type fakeCh struct {
	fireAt time.Time
	ch     chan time.Time
}

func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{current: start}
}

func (fc *FakeClock) Now() time.Time {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.current
}

func (fc *FakeClock) After(d time.Duration) <-chan time.Time {
	fc.mu.Lock()
	now := fc.current
	fireAt := now.Add(d)
	fc.mu.Unlock()

	ch := make(chan time.Time, 1)

	if !fireAt.After(now) {
		// Zero or negative duration: fire immediately.
		ch <- now
		return ch
	}

	fc.mu.Lock()
	fc.waiters = append(fc.waiters, fakeCh{fireAt: fireAt, ch: ch})
	fc.mu.Unlock()
	return ch
}

// Advance moves the fake clock forward by d and fires any waiting timers.
func (fc *FakeClock) Advance(d time.Duration) {
	fc.mu.Lock()
	fc.current = fc.current.Add(d)
	now := fc.current
	remaining := fc.waiters[:0]
	var toFire []chan time.Time
	for _, w := range fc.waiters {
		if !w.fireAt.After(now) {
			toFire = append(toFire, w.ch)
		} else {
			remaining = append(remaining, w)
		}
	}
	fc.waiters = remaining
	fc.mu.Unlock()

	for _, ch := range toFire {
		// Non-blocking send: channel is buffered(1).
		select {
		case ch <- now:
		default:
		}
	}
}

// ─── Fake ProbeConfigSource ───────────────────────────────────────────────────

type fakeSource struct {
	mu      sync.Mutex
	probes  []domain.ProbeConfig
	results []domain.ProbeResult
}

func (f *fakeSource) ListEnabled(_ context.Context) ([]domain.ProbeConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]domain.ProbeConfig, len(f.probes))
	copy(cp, f.probes)
	return cp, nil
}

func (f *fakeSource) RecordResult(_ context.Context, r domain.ProbeResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = append(f.results, r)
	return nil
}

func (f *fakeSource) Results() []domain.ProbeResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]domain.ProbeResult, len(f.results))
	copy(cp, f.results)
	return cp
}

// ─── Fake ResultStore ─────────────────────────────────────────────────────────

type fakeStore struct {
	mu      sync.Mutex
	results []domain.ProbeResult
}

func (f *fakeStore) InsertProbeResult(_ context.Context, r domain.ProbeResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = append(f.results, r)
	return nil
}

func (f *fakeStore) Results() []domain.ProbeResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]domain.ProbeResult, len(f.results))
	copy(cp, f.results)
	return cp
}

// ─── HLS test helpers ─────────────────────────────────────────────────────────

// buildHLSOrigin creates a test HTTP server serving:
//   - /playlist.m3u8: a minimal HLS media playlist with one segment.
//   - /seg.ts: synthetic segment bytes of the given size.
func buildHLSOrigin(t *testing.T, segmentBytes int, segmentDurationS float64) *httptest.Server {
	t.Helper()
	segData := make([]byte, segmentBytes)
	for i := range segData {
		segData[i] = byte(i % 256)
	}

	var srv *httptest.Server

	mux := http.NewServeMux()
	mux.HandleFunc("/playlist.m3u8", func(w http.ResponseWriter, r *http.Request) {
		manifest := fmt.Sprintf(
			"#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:6\n#EXTINF:%.3f,\n%s/seg.ts\n#EXT-X-ENDLIST\n",
			segmentDurationS, srv.URL,
		)
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(manifest))
	})
	mux.HandleFunc("/seg.ts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/MP2T")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(segData)
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// waitForResults polls store.Results() until at least n results are present
// or the deadline is reached.
func waitForResults(store *fakeStore, n int, deadline time.Duration) []domain.ProbeResult {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		results := store.Results()
		if len(results) >= n {
			return results
		}
		time.Sleep(10 * time.Millisecond)
	}
	return store.Results()
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestHLSProbe_Success verifies the happy path:
//   - HLS origin returns a valid manifest + segment.
//   - Result: Success=true, TTFB > 0, BitrateKbps > 0, ErrorCode = "".
func TestHLSProbe_Success(t *testing.T) {
	// 50 KB segment, 6-second duration → expected ~66 kbps.
	const segBytes = 50_000
	const segDurS = 6.0
	srv := buildHLSOrigin(t, segBytes, segDurS)

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-1",
				Name:      "test-hls",
				URL:       srv.URL + "/playlist.m3u8",
				Protocol:  "hls",
				IntervalS: 60,
				TimeoutS:  5,
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}

	// MaxJitterFraction=0 → After(0) fires immediately (no Advance needed).
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 2, MaxJitterFraction: 0}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	// With jitter=0, After(0) returns an immediately-readable channel, so the
	// first probe fires without any Advance call. Wait for the result.
	results := waitForResults(store, 1, 5*time.Second)

	cancel()

	if len(results) == 0 {
		t.Fatal("expected at least one probe result; got none")
	}

	result := results[0]
	t.Logf("probe result: success=%v ttfb_ms=%d bitrate_kbps=%.1f error_code=%q error_msg=%q",
		result.Success, result.TTFBMs, result.BitrateKbps, result.ErrorCode, result.ErrorMsg)

	if !result.Success {
		t.Errorf("expected Success=true, got false: error_code=%q error_msg=%q",
			result.ErrorCode, result.ErrorMsg)
	}
	if result.TTFBMs == 0 {
		t.Errorf("expected TTFB > 0 ms")
	}
	if result.BitrateKbps <= 0 {
		t.Errorf("expected BitrateKbps > 0, got %.1f", result.BitrateKbps)
	}
	if result.SegmentTTFBMs == 0 {
		t.Errorf("expected SegmentTTFBMs > 0")
	}
	if result.ErrorCode != "" {
		t.Errorf("expected empty ErrorCode on success, got %q", result.ErrorCode)
	}
	if result.ProbeID != "probe-1" {
		t.Errorf("ProbeID mismatch: expected probe-1, got %q", result.ProbeID)
	}

	// Also verify the result was passed to the source for denorm update.
	sourceResults := source.Results()
	if len(sourceResults) == 0 {
		t.Error("expected RecordResult to be called on source; got 0 results")
	}

	// Verify expected bitrate range: 50000 * 8 / 6 / 1000 ≈ 66.7 kbps.
	// Allow generous tolerance (10–5000 kbps) since timing can vary.
	if result.BitrateKbps < 10 || result.BitrateKbps > 5000 {
		t.Errorf("BitrateKbps out of expected range [10,5000]: %.1f", result.BitrateKbps)
	}
	t.Logf("PASS: success=true, ttfb_ms=%d, bitrate_kbps=%.1f",
		result.TTFBMs, result.BitrateKbps)
}

// TestHLSProbe_SegmentBodyTooLarge verifies that a segment body exceeding the
// 32 MB cap is reported as segment_too_large (Success=true because the manifest
// was valid) with BitrateKbps=0 and SegmentTTFBMs>0.
//
// The httptest handler streams cap+1 bytes in 1 MB chunks so the test never
// allocates 32 MB at once.
func TestHLSProbe_SegmentBodyTooLarge(t *testing.T) {
	const capBytes = 32 << 20  // mirrors segBodyCapBytes in prober.go
	const chunkBytes = 1 << 20 // 1 MB per write, avoids large single alloc
	const segDurS = 6.0

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/playlist.m3u8", func(w http.ResponseWriter, r *http.Request) {
		manifest := fmt.Sprintf(
			"#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:6\n#EXTINF:%.3f,\n%s/seg.ts\n#EXT-X-ENDLIST\n",
			segDurS, srv.URL,
		)
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte(manifest))
	})
	mux.HandleFunc("/seg.ts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/MP2T")
		buf := make([]byte, chunkBytes)
		// Write capBytes/chunkBytes full chunks (= capBytes bytes total).
		for i := 0; i < capBytes/chunkBytes; i++ {
			if _, err := w.Write(buf); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		// Write the +1 byte that pushes the total past the cap.
		_, _ = w.Write([]byte{0})
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-hls-toolarge",
				Name:      "test-hls-toolarge",
				URL:       srv.URL + "/playlist.m3u8",
				Protocol:  "hls",
				IntervalS: 60,
				TimeoutS:  30, // generous: must transfer cap+1 bytes over loopback
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	results := waitForResults(store, 1, 45*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("expected at least one probe result; got none")
	}
	result := results[0]
	t.Logf("SegmentBodyTooLarge result: success=%v code=%q ttfb_ms=%d seg_ttfb_ms=%d bitrate=%.4f msg=%q",
		result.Success, result.ErrorCode, result.TTFBMs, result.SegmentTTFBMs, result.BitrateKbps, result.ErrorMsg)

	if !result.Success {
		t.Errorf("expected Success=true (manifest OK), got false: code=%q msg=%q",
			result.ErrorCode, result.ErrorMsg)
	}
	if result.ErrorCode != "segment_too_large" {
		t.Errorf("expected ErrorCode=segment_too_large, got %q", result.ErrorCode)
	}
	if result.BitrateKbps != 0 {
		t.Errorf("expected BitrateKbps=0 (truncated; no valid bitrate), got %.4f", result.BitrateKbps)
	}
	if result.SegmentTTFBMs == 0 {
		t.Errorf("expected SegmentTTFBMs > 0 (TTFB recorded before body read), got 0")
	}
	t.Logf("PASS: Success=true, ErrorCode=segment_too_large, BitrateKbps=0, SegmentTTFBMs=%d",
		result.SegmentTTFBMs)
}

// TestHLSProbe_HTTP500 verifies that a 500 origin returns success=false with
// error_code="http_5xx".
func TestHLSProbe_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-500",
				Name:      "test-500",
				URL:       srv.URL + "/playlist.m3u8",
				Protocol:  "hls",
				IntervalS: 60,
				TimeoutS:  5,
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	results := waitForResults(store, 1, 5*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("expected at least one probe result; got none")
	}

	result := results[0]
	t.Logf("probe result: success=%v error_code=%q error_msg=%q",
		result.Success, result.ErrorCode, result.ErrorMsg)

	if result.Success {
		t.Error("expected Success=false on 500 origin")
	}
	if result.ErrorCode != "http_5xx" {
		t.Errorf("expected error_code=http_5xx, got %q", result.ErrorCode)
	}
	t.Logf("PASS: success=false, error_code=%q", result.ErrorCode)
}

// TestHLSProbe_Timeout verifies that a slow origin returns success=false with
// error_code="timeout".
func TestHLSProbe_Timeout(t *testing.T) {
	// Server that blocks until the request context is cancelled (simulates hang).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		http.Error(w, "timeout", http.StatusGatewayTimeout)
	}))
	t.Cleanup(srv.Close)

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-timeout",
				Name:      "test-timeout",
				URL:       srv.URL + "/playlist.m3u8",
				Protocol:  "hls",
				IntervalS: 60,
				TimeoutS:  1, // 1 second probe timeout
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	// Wait up to 5s — the probe has a 1s timeout, so we should get a result quickly.
	results := waitForResults(store, 1, 5*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("expected at least one probe result; got none")
	}

	result := results[0]
	t.Logf("probe result: success=%v error_code=%q error_msg=%q",
		result.Success, result.ErrorCode, result.ErrorMsg)

	if result.Success {
		t.Error("expected Success=false on timeout origin")
	}
	if result.ErrorCode != "timeout" {
		t.Errorf("expected error_code=timeout, got %q", result.ErrorCode)
	}
	t.Logf("PASS: success=false, error_code=%q", result.ErrorCode)
}

// TestProbe_NotProbed verifies that an unrecognised protocol string (e.g. "srt")
// falls through to probeReachability and returns error_code="not_probed".
// rtmp and dash now have dedicated probes — see TestProbe_RTMPDispatch and
// TestProbe_DASHDispatch below.
func TestProbe_NotProbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-srt",
				Name:      "test-srt",
				URL:       srv.URL + "/stream",
				Protocol:  "srt",
				IntervalS: 60,
				TimeoutS:  5,
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	results := waitForResults(store, 1, 5*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("expected at least one probe result for srt; got none")
	}
	result := results[0]
	t.Logf("[srt] result: success=%v error_code=%q msg=%q", result.Success, result.ErrorCode, result.ErrorMsg)

	if result.Success {
		t.Error("[srt] expected Success=false for unrecognised protocol")
	}
	if result.ErrorCode != "not_probed" {
		t.Errorf("[srt] expected error_code=not_probed (forward-compat default), got %q", result.ErrorCode)
	}
	if !strings.Contains(result.ErrorMsg, "Phase 3") {
		t.Errorf("[srt] expected error_msg to mention Phase 3, got %q", result.ErrorMsg)
	}
	t.Log("PASS [srt]: success=false, error_code=not_probed (probeReachability forward-compat)")
}

// TestProbe_RTMPDispatch verifies that protocol="rtmp" dispatches to probeRTMP
// and NOT to probeReachability. The probe runs against a TCP address that is
// immediately closed after binding, so probeRTMP returns rtmp_refused or
// rtmp_error — either way, never "not_probed".
func TestProbe_RTMPDispatch(t *testing.T) {
	// Grab a free port, note address, close immediately → ECONNREFUSED on dial.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-rtmp-dispatch",
				Name:      "test-rtmp-dispatch",
				URL:       "rtmp://" + addr + "/live",
				Protocol:  "rtmp",
				IntervalS: 60,
				TimeoutS:  5,
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	results := waitForResults(store, 1, 5*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("[rtmp dispatch] expected at least one probe result; got none")
	}
	result := results[0]
	t.Logf("[rtmp dispatch] result: success=%v error_code=%q msg=%q",
		result.Success, result.ErrorCode, result.ErrorMsg)

	if result.Success {
		t.Error("[rtmp dispatch] expected Success=false against dead RTMP address")
	}
	// probeRTMP must have handled the request — any rtmp_* code is valid;
	// "not_probed" means dispatch still fell through to probeReachability.
	if result.ErrorCode == "not_probed" {
		t.Errorf("[rtmp dispatch] error_code=not_probed: dispatch fell through to probeReachability — case \"rtmp\" missing from executeProbe switch")
	}
	if result.ErrorCode != "rtmp_refused" && result.ErrorCode != "rtmp_error" && result.ErrorCode != "rtmp_timeout" {
		t.Errorf("[rtmp dispatch] unexpected error_code %q; want rtmp_refused, rtmp_error, or rtmp_timeout", result.ErrorCode)
	}
	t.Logf("PASS [rtmp dispatch]: success=false, error_code=%q (probeRTMP reached)", result.ErrorCode)
}

// TestProbe_DASHDispatch verifies that protocol="dash" dispatches to probeDASH
// and NOT to probeReachability. The server returns 200 OK with a non-MPD body;
// probeDASH fails with error_code="parse" — never "not_probed".
func TestProbe_DASHDispatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("this is not an MPD"))
	}))
	t.Cleanup(srv.Close)

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-dash-dispatch",
				Name:      "test-dash-dispatch",
				URL:       srv.URL + "/manifest.mpd",
				Protocol:  "dash",
				IntervalS: 60,
				TimeoutS:  5,
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	results := waitForResults(store, 1, 5*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("[dash dispatch] expected at least one probe result; got none")
	}
	result := results[0]
	t.Logf("[dash dispatch] result: success=%v error_code=%q msg=%q",
		result.Success, result.ErrorCode, result.ErrorMsg)

	if result.Success {
		t.Error("[dash dispatch] expected Success=false (non-MPD response)")
	}
	// probeDASH returns "parse" when the body is not parseable as MPD;
	// "not_probed" means dispatch still fell through to probeReachability.
	if result.ErrorCode == "not_probed" {
		t.Errorf("[dash dispatch] error_code=not_probed: dispatch fell through to probeReachability — case \"dash\" missing from executeProbe switch")
	}
	if result.ErrorCode != "parse" {
		t.Errorf("[dash dispatch] expected error_code=parse (non-MPD body), got %q", result.ErrorCode)
	}
	t.Logf("PASS [dash dispatch]: success=false, error_code=%q (probeDASH reached)", result.ErrorCode)
}

// ─── WebRTC probe tests ───────────────────────────────────────────────────────

// buildWSSignalingServer creates an httptest.Server with a WebSocket handler at
// /{app}/websocket that speaks the minimal AMS signaling protocol.
// On receiving {"command":"play",...} it replies with a takeConfiguration/offer,
// then blocks until the client closes the WS (or the request context is done).
// This keeps the connection alive so phase-2a's continueWebRTCICE can attempt
// ICE exchange (which will time out because this server sends no candidates).
// If silent=true the server accepts the WS connection but never sends anything
// (used to test ws_timeout — the probe never reaches the ICE phase).
func buildWSSignalingServer(t *testing.T, silent bool, offerSDP string) *httptest.Server {
	t.Helper()

	// Minimal SDP offer used in tests — just enough for parse check.
	if offerSDP == "" {
		offerSDP = "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\na=recvonly\r\n"
	}

	sdpForOffer := offerSDP
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		if silent {
			// Block forever (ctx will be cancelled by the probe timeout).
			<-req.Context().Done()
			return
		}

		// Read the play command.
		var playMsg map[string]json.RawMessage
		if err := wsjson.Read(req.Context(), conn, &playMsg); err != nil {
			return
		}

		// Extract streamId from play message.
		var streamID string
		if raw, ok := playMsg["streamId"]; ok {
			_ = json.Unmarshal(raw, &streamID)
		}

		// Reply with takeConfiguration/offer.
		offer := map[string]interface{}{
			"command":  "takeConfiguration",
			"streamId": streamID,
			"type":     "offer",
			"sdp":      sdpForOffer,
		}
		_ = wsjson.Write(req.Context(), conn, offer)

		// Keep the WS open until the probe closes it (its context deadline fires
		// and the probe calls conn.Close) or the request context is cancelled.
		// This ensures phase-2a's continueWebRTCICE can send its answer + candidates
		// even though this server ignores them (no ICE connectivity → ice_timeout).
		// Draining incoming messages prevents the OS buffer from filling.
		for {
			var discard json.RawMessage
			if err := wsjson.Read(req.Context(), conn, &discard); err != nil {
				return // probe closed WS or context expired
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestProbe_WebRTC_SignalingOnly verifies signaling + ICE-timeout behavior:
//   - mock WS server speaks AMS signaling, keeps WS open but sends no ICE candidates.
//   - Phase-1 signaling succeeds: Success=true, connect_time_ms>0,
//     signaling_state=offer_received.
//   - Phase-2a ICE times out (TimeoutS=2, no server candidates): ice_state=timeout,
//     error_code=ice_timeout.  Success stays true (signaling succeeded).
//
// The full ICE-connected happy path is in TestProbeWebRTC_ICEHappyPath
// (probe_webrtc_ice_test.go), which uses a real pion OFFERER.
func TestProbe_WebRTC_SignalingOnly(t *testing.T) {
	srv := buildWSSignalingServer(t, false, "")

	// Derive ws:// URL from the httptest server URL and add ?streamId=.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/live/websocket?streamId=test-stream-1"

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-webrtc-ok",
				Name:      "test-webrtc-ok",
				URL:       wsURL,
				Protocol:  "webrtc",
				IntervalS: 60,
				TimeoutS:  2, // short: ICE times out quickly (no server candidates)
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	// Outer harness: 30s/20s (TimeoutS=2 is the behavior under test; outer waits
	// bound scheduler+goroutine latency on -race runs, D-039/D-042 class).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	results := waitForResults(store, 1, 20*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("expected at least one probe result; got none")
	}
	result := results[0]
	t.Logf("WebRTC result: success=%v error_code=%q signaling_state=%q ice_state=%q connect_time_ms=%v",
		result.Success, result.ErrorCode, result.SignalingState, result.IceState, result.ConnectTimeMs)

	// Phase-1 signaling assertions (unchanged from pre-phase-2a).
	if !result.Success {
		t.Errorf("expected Success=true (signaling succeeded), got false: error_code=%q error_msg=%q",
			result.ErrorCode, result.ErrorMsg)
	}
	if result.SignalingState != "offer_received" {
		t.Errorf("expected signaling_state=offer_received, got %q", result.SignalingState)
	}
	if result.ConnectTimeMs == nil {
		t.Error("expected ConnectTimeMs != nil on success")
	} else if *result.ConnectTimeMs == 0 {
		t.Error("expected ConnectTimeMs > 0")
	}
	// Phase-2a ICE assertions: the minimal test SDP ("v=0\r\n...") may be
	// rejected by pion's strict WebRTC SDP parser (missing ice-ufrag/fingerprint),
	// causing continueWebRTCICE to return early with IceState="" (ICE not
	// attempted).  If pion accepts it, ICE times out because the server sends no
	// candidates → IceState="timeout".  Both outcomes are valid for this regression
	// test which focuses on the SIGNALING layer.  The full ICE outcome is covered
	// by TestProbeWebRTC_ICEHappyPath and TestProbeWebRTC_ICETimeout.
	switch result.IceState {
	case "timeout":
		if result.ErrorCode != "ice_timeout" {
			t.Errorf("IceState=timeout but ErrorCode=%q, want ice_timeout", result.ErrorCode)
		}
		t.Logf("ICE timed out (SDP accepted by pion, no server candidates): ice_state=timeout")
	case "":
		// ICE not attempted: pion rejected the minimal test SDP (missing WebRTC fields).
		if result.ErrorCode != "" {
			t.Errorf("IceState='' but ErrorCode=%q, want '' (ICE not attempted)", result.ErrorCode)
		}
		t.Logf("ICE not attempted (minimal SDP rejected by pion): ice_state=''")
	default:
		t.Errorf("unexpected IceState=%q (want timeout or '')", result.IceState)
	}
	t.Logf("PASS: success=true, signaling_state=%q, ice_state=%q, error_code=%q",
		result.SignalingState, result.IceState, result.ErrorCode)
}

// TestProbe_WebRTC_WsTimeout verifies that a WS server that accepts then stays
// silent returns success=false, error_code=ws_timeout.
func TestProbe_WebRTC_WsTimeout(t *testing.T) {
	srv := buildWSSignalingServer(t, true, "")
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/live/websocket?streamId=silent-stream"

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-webrtc-timeout",
				Name:      "test-webrtc-timeout",
				URL:       wsURL,
				Protocol:  "webrtc",
				IntervalS: 60,
				TimeoutS:  1, // 1s → triggers timeout
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	// Generous harness budgets (30s/20s): the probe's own 1s timeout is the
	// behavior under test; the outer waits only bound scheduler+goroutine
	// latency, which can exceed several seconds on a CPU-contended -race run
	// (D-039/D-042 class — observed once at 6s during the S12 full-suite gate).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	results := waitForResults(store, 1, 20*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("expected at least one probe result; got none")
	}
	result := results[0]
	t.Logf("WsTimeout result: success=%v error_code=%q signaling_state=%q",
		result.Success, result.ErrorCode, result.SignalingState)

	if result.Success {
		t.Error("expected Success=false on WS timeout")
	}
	if result.ErrorCode != "ws_timeout" {
		t.Errorf("expected error_code=ws_timeout, got %q", result.ErrorCode)
	}
	if result.ConnectTimeMs != nil {
		t.Errorf("expected ConnectTimeMs=nil on timeout, got %v", result.ConnectTimeMs)
	}
	t.Logf("PASS: success=false, error_code=ws_timeout, signaling_state=%q", result.SignalingState)
}

// TestProbe_WebRTC_WsRefused verifies that a connection refused returns
// success=false, error_code=ws_refused.
func TestProbe_WebRTC_WsRefused(t *testing.T) {
	// Use a port that is not listening (httptest.Server was closed).
	srv := buildWSSignalingServer(t, false, "")
	closedURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/live/websocket?streamId=refused-stream"
	srv.Close() // close BEFORE the probe runs

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-webrtc-refused",
				Name:      "test-webrtc-refused",
				URL:       closedURL,
				Protocol:  "webrtc",
				IntervalS: 60,
				TimeoutS:  5,
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	results := waitForResults(store, 1, 5*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("expected at least one probe result; got none")
	}
	result := results[0]
	t.Logf("WsRefused result: success=%v error_code=%q signaling_state=%q",
		result.Success, result.ErrorCode, result.SignalingState)

	if result.Success {
		t.Error("expected Success=false on refused connection")
	}
	// ws_refused or ws_error are both acceptable — OS may return ECONNREFUSED or EHOSTUNREACH
	if result.ErrorCode != "ws_refused" && result.ErrorCode != "ws_error" {
		t.Errorf("expected error_code=ws_refused or ws_error, got %q", result.ErrorCode)
	}
	t.Logf("PASS: success=false, error_code=%q", result.ErrorCode)
}

// TestProbe_WebRTC_MissingStreamID verifies that a WebRTC probe URL without
// ?streamId= returns success=false, error_code=ws_error with a descriptive message.
func TestProbe_WebRTC_MissingStreamID(t *testing.T) {
	srv := buildWSSignalingServer(t, false, "")
	// URL without streamId query param.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/live/websocket"

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-webrtc-missing-id",
				Name:      "test-webrtc-missing-id",
				URL:       wsURL,
				Protocol:  "webrtc",
				IntervalS: 60,
				TimeoutS:  5,
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	results := waitForResults(store, 1, 5*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("expected at least one probe result; got none")
	}
	result := results[0]
	t.Logf("MissingStreamID result: success=%v error_code=%q msg=%q",
		result.Success, result.ErrorCode, result.ErrorMsg)

	if result.Success {
		t.Error("expected Success=false on missing streamId")
	}
	if result.ErrorCode != "ws_error" {
		t.Errorf("expected error_code=ws_error, got %q", result.ErrorCode)
	}
	if !strings.Contains(result.ErrorMsg, "streamId") {
		t.Errorf("expected error_msg to mention streamId, got %q", result.ErrorMsg)
	}
	if result.ConnectTimeMs != nil {
		t.Errorf("expected ConnectTimeMs=nil, got %v", result.ConnectTimeMs)
	}
	t.Logf("PASS: success=false, error_code=ws_error, mentions streamId")
}

// TestProbe_WebRTC_FixtureReplay feeds the captured real-AMS offer message bytes
// through the parser to pin the parse logic against the actual wire format.
// This is a table-driven unit test — it does NOT start any servers.
func TestProbe_WebRTC_FixtureReplay(t *testing.T) {
	// Captured from real AMS 3.0.3 WebRTC signaling (see
	// agents/handoffs/real-ams-captures/webrtc-signaling-play-offer.json and
	// the D-074 live capture: real AMS sends notification messages, e.g.
	// subtrackAdded, BEFORE takeConfiguration — the probe must skip them).
	cases := []struct {
		name            string
		msgs            []string
		wantOK          bool
		wantState       string
		wantMsgContains string
	}{
		{
			name:      "real_ams_offer",
			msgs:      []string{`{"command":"takeConfiguration","streamId":"teststream","type":"offer","sdp":"v=0\r\n"}`},
			wantOK:    true,
			wantState: "offer_received",
		},
		{
			// D-074 live-captured sequence: notification precedes the offer.
			name: "notification_then_offer",
			msgs: []string{
				`{"trackId":"teststream","definition":"subtrackAdded","command":"notification","mainTrack":"teststream"}`,
				`{"command":"takeConfiguration","streamId":"teststream","type":"offer","sdp":"v=0\r\n"}`,
			},
			wantOK:    true,
			wantState: "offer_received",
		},
		{
			name:      "answer_not_offer",
			msgs:      []string{`{"command":"takeConfiguration","streamId":"teststream","type":"answer","sdp":"v=0\r\n"}`},
			wantOK:    false,
			wantState: "ws_error",
		},
		{
			name:            "error_command",
			msgs:            []string{`{"command":"error","definition":"highResourceUsage"}`},
			wantOK:          false,
			wantState:       "ws_error",
			wantMsgContains: "highResourceUsage",
		},
		{
			name:      "take_candidate",
			msgs:      []string{`{"command":"takeCandidate","streamId":"teststream","label":1,"id":"1","candidate":"candidate:..."}`},
			wantOK:    false,
			wantState: "ws_error",
		},
		{
			// Notifications alone are NOT a signaling response: the probe
			// must keep reading until the deadline → ws_timeout.
			name:      "notification_only_times_out",
			msgs:      []string{`{"trackId":"teststream","definition":"subtrackAdded","command":"notification","mainTrack":"teststream"}`},
			wantOK:    false,
			wantState: "ws_timeout",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build a WS server that sends tc.msgs in order, then holds the
			// connection open (blocking read) until the client disconnects.
			rawMsgs := tc.msgs
			var srv *httptest.Server
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{InsecureSkipVerify: true})
				if err != nil {
					return
				}
				defer conn.Close(websocket.StatusNormalClosure, "done")
				// Read play command (discard).
				var dummy json.RawMessage
				_ = wsjson.Read(req.Context(), conn, &dummy)
				// Send fixture messages as raw JSON, in order.
				for _, m := range rawMsgs {
					_ = conn.Write(req.Context(), websocket.MessageText, []byte(m))
				}
				// Hold the connection open until the probe disconnects (mirrors
				// a real server that goes quiet rather than closing).
				var hold json.RawMessage
				_ = wsjson.Read(req.Context(), conn, &hold)
			}))
			t.Cleanup(srv.Close)

			wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/live/websocket?streamId=fixture-stream"
			source := &fakeSource{
				probes: []domain.ProbeConfig{{
					ID:        "probe-fixture-" + tc.name,
					Name:      tc.name,
					URL:       wsURL,
					Protocol:  "webrtc",
					IntervalS: 60,
					TimeoutS:  5,
					Enabled:   true,
				}},
			}
			store := &fakeStore{}
			clock := NewFakeClock(time.Now())
			runner := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()

			go func() { _ = runner.Run(ctx) }()

			// The wait budget must STRICTLY dominate the probe's own deadline
			// (TimeoutS=5): the notification_only_times_out case stores its
			// result at ~5.0s, so a 5s wait races the store write and loses
			// under whole-suite -race CPU contention (D-042 class — budget
			// inversion, not flakiness). 8s = probe 5s + 3s scheduler margin.
			results := waitForResults(store, 1, 8*time.Second)
			cancel()

			if len(results) == 0 {
				t.Fatal("expected at least one probe result; got none")
			}
			res := results[0]
			t.Logf("[%s] success=%v code=%q state=%q", tc.name, res.Success, res.ErrorCode, res.SignalingState)

			if res.Success != tc.wantOK {
				t.Errorf("expected Success=%v, got %v (error_code=%q msg=%q)",
					tc.wantOK, res.Success, res.ErrorCode, res.ErrorMsg)
			}
			if res.SignalingState != tc.wantState {
				t.Errorf("expected SignalingState=%q, got %q", tc.wantState, res.SignalingState)
			}
			if tc.wantMsgContains != "" && !strings.Contains(res.ErrorMsg, tc.wantMsgContains) {
				t.Errorf("expected ErrorMsg to contain %q, got %q", tc.wantMsgContains, res.ErrorMsg)
			}
			if tc.wantOK {
				if res.ConnectTimeMs == nil {
					t.Error("expected ConnectTimeMs != nil on success")
				}
				if res.ErrorCode != "" {
					t.Errorf("expected empty ErrorCode on success, got %q", res.ErrorCode)
				}
			}
			t.Logf("PASS [%s]", tc.name)
		})
	}
}

// TestInterval_Honored verifies that a probe with interval_s=60 fires
// approximately once per interval under a fake clock.
// With MaxJitterFraction=0, After(0) fires immediately; subsequent After(60s)
// fires when we call Advance(60s).
func TestInterval_Honored(t *testing.T) {
	srv := buildHLSOrigin(t, 10_000, 4.0)

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-interval",
				Name:      "test-interval",
				URL:       srv.URL + "/playlist.m3u8",
				Protocol:  "hls",
				IntervalS: 60,
				TimeoutS:  5,
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 2, MaxJitterFraction: 0}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	// First fire: immediate (After(0) fires at once).
	// Wait for it.
	results := waitForResults(store, 1, 5*time.Second)
	if len(results) < 1 {
		t.Fatal("first probe did not fire")
	}
	t.Logf("first fire: got %d results", len(results))

	// Advance 60s → second fire.
	time.Sleep(20 * time.Millisecond) // let scheduler goroutine reach its After(60s)
	clock.Advance(60 * time.Second)
	results = waitForResults(store, 2, 5*time.Second)
	if len(results) < 2 {
		t.Fatalf("second fire did not happen; got %d results", len(results))
	}
	t.Logf("second fire: got %d results", len(results))

	// Advance another 60s → third fire.
	time.Sleep(20 * time.Millisecond)
	clock.Advance(60 * time.Second)
	results = waitForResults(store, 3, 5*time.Second)
	if len(results) < 3 {
		t.Fatalf("third fire did not happen; got %d results", len(results))
	}
	t.Logf("third fire: got %d results", len(results))

	cancel()

	// Verify result IDs are unique.
	seen := make(map[string]bool)
	for _, r := range results {
		if seen[r.ID] {
			t.Errorf("duplicate probe result ID: %s", r.ID)
		}
		seen[r.ID] = true
	}

	t.Logf("PASS: interval honored — %d firings in 2 intervals + initial (expected ≥3)", len(results))
}

// TestHLSManifest_Parse verifies correct handling of a master HLS playlist.
func TestHLSManifest_Parse(t *testing.T) {
	t.Run("master_playlist_follows_variant", func(t *testing.T) {
		// Master playlist → variant → segment chain.
		// The prober must follow the master to the variant and measure bitrate.
		const segBytes = 20_000
		const segDurS = 4.0
		segData := make([]byte, segBytes)

		var srv *httptest.Server
		mux := http.NewServeMux()

		// /master.m3u8 → points to /variant.m3u8.
		mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = fmt.Fprintf(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\n%s/variant.m3u8\n", srv.URL)
		})
		// /variant.m3u8 → media playlist with one segment.
		mux.HandleFunc("/variant.m3u8", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = fmt.Fprintf(w,
				"#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:6\n#EXTINF:%.3f,\n%s/seg.ts\n#EXT-X-ENDLIST\n",
				segDurS, srv.URL,
			)
		})
		// /seg.ts → synthetic segment data.
		mux.HandleFunc("/seg.ts", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "video/MP2T")
			_, _ = w.Write(segData)
		})

		srv = httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		source := &fakeSource{
			probes: []domain.ProbeConfig{{
				ID:        "p-master",
				Name:      "master",
				URL:       srv.URL + "/master.m3u8",
				Protocol:  "hls",
				IntervalS: 60,
				TimeoutS:  5,
				Enabled:   true,
			}},
		}
		store := &fakeStore{}
		clock := NewFakeClock(time.Now())
		r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		go func() { _ = r.Run(ctx) }()

		results := waitForResults(store, 1, 5*time.Second)
		cancel()

		if len(results) == 0 {
			t.Fatal("no results for master playlist probe")
		}
		result := results[0]
		t.Logf("master→variant result: success=%v bitrate=%.1f segment_ttfb_ms=%d error=%q",
			result.Success, result.BitrateKbps, result.SegmentTTFBMs, result.ErrorMsg)
		if !result.Success {
			t.Errorf("master→variant probe should succeed; got error=%q", result.ErrorMsg)
		}
		if result.BitrateKbps <= 0 {
			t.Errorf("expected BitrateKbps > 0 after following master→variant, got %.1f", result.BitrateKbps)
		}
		t.Logf("PASS: master playlist → variant followed → bitrate=%.1f kbps", result.BitrateKbps)
	})
}

// TestHLSProbe_MasterFollowsVariant verifies master → variant → segment chain end-to-end.
func TestHLSProbe_MasterFollowsVariant(t *testing.T) {
	const segBytes = 50_000
	const segDurS = 6.0
	segData := make([]byte, segBytes)

	var srv *httptest.Server
	mux := http.NewServeMux()

	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = fmt.Fprintf(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\n%s/variant.m3u8\n", srv.URL)
	})
	mux.HandleFunc("/variant.m3u8", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = fmt.Fprintf(w,
			"#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:8\n#EXTINF:%.3f,\n%s/seg.ts\n#EXT-X-ENDLIST\n",
			segDurS, srv.URL,
		)
	})
	mux.HandleFunc("/seg.ts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/MP2T")
		_, _ = w.Write(segData)
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	source := &fakeSource{
		probes: []domain.ProbeConfig{{
			ID:        "p-master-variant",
			Name:      "master-variant",
			URL:       srv.URL + "/master.m3u8",
			Protocol:  "hls",
			IntervalS: 60,
			TimeoutS:  5,
			Enabled:   true,
		}},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = r.Run(ctx) }()

	results := waitForResults(store, 1, 5*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("expected at least one probe result; got none")
	}
	result := results[0]
	t.Logf("master→variant: success=%v bitrate=%.1f seg_ttfb_ms=%d error=%q",
		result.Success, result.BitrateKbps, result.SegmentTTFBMs, result.ErrorMsg)

	if !result.Success {
		t.Errorf("expected Success=true after following master→variant; error=%q", result.ErrorMsg)
	}
	if result.BitrateKbps <= 0 {
		t.Errorf("expected BitrateKbps > 0, got %.1f", result.BitrateKbps)
	}
	// Expected: 50000 * 8 / 6 / 1000 ≈ 66.7 kbps
	if result.BitrateKbps < 10 || result.BitrateKbps > 5000 {
		t.Errorf("BitrateKbps out of expected range [10,5000]: %.1f", result.BitrateKbps)
	}
	t.Logf("PASS: master→variant→segment: success=true, bitrate=%.1f kbps, seg_ttfb_ms=%d",
		result.BitrateKbps, result.SegmentTTFBMs)
}

// TestProbe_WebRTC_EncodedStreamID verifies that a percent-encoded streamId in
// the probe URL reaches the AMS play command DECODED (D-072 verifier finding:
// the raw query value was previously forwarded verbatim).
func TestProbe_WebRTC_EncodedStreamID(t *testing.T) {
	received := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		var play struct {
			Command  string `json:"command"`
			StreamID string `json:"streamId"`
		}
		if err := wsjson.Read(req.Context(), conn, &play); err != nil {
			return
		}
		received <- play.StreamID

		offer := map[string]interface{}{
			"command":  "takeConfiguration",
			"streamId": play.StreamID,
			"type":     "offer",
			"sdp":      "v=0\r\n",
		}
		_ = wsjson.Write(req.Context(), conn, offer)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/live/websocket?streamId=enc%20oded%2Fid"

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-webrtc-encoded",
				Name:      "test-webrtc-encoded",
				URL:       wsURL,
				Protocol:  "webrtc",
				IntervalS: 60,
				TimeoutS:  5,
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go func() { _ = r.Run(ctx) }()

	results := waitForResults(store, 1, 20*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("expected a probe result; got none")
	}
	if !results[0].Success {
		t.Errorf("expected success, got error_code=%q msg=%q", results[0].ErrorCode, results[0].ErrorMsg)
	}

	select {
	case got := <-received:
		if got != "enc oded/id" {
			t.Errorf("play streamId = %q, want %q (percent-decoded)", got, "enc oded/id")
		}
	default:
		t.Fatal("signaling server never received a play command")
	}
}
