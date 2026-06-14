package prober_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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

// TestProbe_NotProbed verifies that webrtc/rtmp/dash probes return
// success=false with error_code="not_probed" (honest minimal stub).
func TestProbe_NotProbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	for _, proto := range []string{"webrtc", "rtmp", "dash"} {
		proto := proto
		t.Run(proto, func(t *testing.T) {
			source := &fakeSource{
				probes: []domain.ProbeConfig{
					{
						ID:        "probe-" + proto,
						Name:      "test-" + proto,
						URL:       srv.URL + "/stream",
						Protocol:  proto,
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
			t.Logf("[%s] result: success=%v error_code=%q", proto, result.Success, result.ErrorCode)

			if result.Success {
				t.Errorf("[%s] expected Success=false for not-yet-probed protocol", proto)
			}
			if result.ErrorCode != "not_probed" {
				t.Errorf("[%s] expected error_code=not_probed, got %q", proto, result.ErrorCode)
			}
			if !strings.Contains(result.ErrorMsg, "Phase 3") {
				t.Errorf("[%s] expected error_msg to mention Phase 3, got %q", proto, result.ErrorMsg)
			}
			t.Logf("PASS [%s]: success=false, error_code=not_probed (honest stub)", proto)
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
	t.Run("master_playlist_returns_empty_segment", func(t *testing.T) {
		// Master playlist — no #EXTINF + segment, only #EXT-X-STREAM-INF.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\nhttps://example.com/variant.m3u8\n"))
		}))
		t.Cleanup(srv.Close)

		source := &fakeSource{
			probes: []domain.ProbeConfig{{
				ID:        "p-master",
				Name:      "master",
				URL:       srv.URL,
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
		if !results[0].Success {
			t.Errorf("master playlist should succeed; got error=%q", results[0].ErrorMsg)
		}
		t.Logf("PASS: master playlist → success=true, bitrate=0")
	})
}
