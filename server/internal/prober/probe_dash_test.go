// Tests for probeDASH.
// Internal test package (package prober) so the unexported probeDASH method is
// accessible without going through the scheduler dispatch (wiring is added by a
// separate wiring author in the "dash" case of executeProbe).
//
// Fixture provenance note (SPEC-DERIVED, DASH-IF):
// The real AMS has DASH muxing disabled (404 verified live 2026-07-10, D-073)
// so live capture is impossible without mutating prod AMS.  All MPD fixtures
// in this file are derived from DASH-IF IOP test vectors and the ISO/IEC
// 23009-1 §5 examples.
package prober

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// dashTestRunner returns a minimal Runner with a real http.Client sufficient
// for probeDASH unit tests.  probeDASH uses r.cfg.HTTPUserAgent and r.client;
// all other Runner fields are unused.
func dashTestRunner() *Runner {
	return &Runner{
		cfg:    Config{HTTPUserAgent: "pulse-probe/dash-test"},
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// dashConfig returns a ProbeConfig for the given DASH manifest URL.
func dashConfig(rawURL string) domain.ProbeConfig {
	return domain.ProbeConfig{
		ID:        "test-dash-probe",
		Name:      "test-dash",
		URL:       rawURL,
		Protocol:  "dash",
		IntervalS: 60,
		TimeoutS:  5,
	}
}

// dashResult returns a minimal initial ProbeResult for direct probeDASH calls.
func dashResult() domain.ProbeResult {
	return domain.ProbeResult{
		ID:      "test-result-id",
		ProbeID: "test-dash-probe",
		TS:      time.Now().UTC(),
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestProbeDASH_SegmentTemplate verifies the happy-path with a DASH-IF style
// MPD that uses SegmentTemplate:
//   - timescale=90000, duration=180000 → 2 s per segment
//   - printf-width $Number%05d$ substitution
//   - $RepresentationID$ substitution
//   - relative media URL resolved against the MPD URL
//
// Assertions: Success=true, TTFBMs>=1, SegmentTTFBMs>=1,
// BitrateKbps == segBytes*8/2.0/1000 (exact float32).
func TestProbeDASH_SegmentTemplate(t *testing.T) {
	const segBytes = 50_000
	segData := make([]byte, segBytes)
	for i := range segData {
		segData[i] = byte(i % 256)
	}

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.mpd", func(w http.ResponseWriter, r *http.Request) {
		// SegmentTemplate: timescale=90000, duration=180000 → 2 s/segment.
		// media="seg$RepresentationID$_$Number%05d$.m4s", repID="1", startNumber=1
		// → first segment path: seg1_00001.m4s
		const mpd = `<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT60S">
  <Period start="PT0S">
    <AdaptationSet mimeType="video/mp4" codecs="avc1.640028">
      <SegmentTemplate media="seg$RepresentationID$_$Number%05d$.m4s" timescale="90000" duration="180000" startNumber="1"/>
      <Representation id="1" bandwidth="2000000" width="1280" height="720"/>
    </AdaptationSet>
  </Period>
</MPD>`
		w.Header().Set("Content-Type", "application/dash+xml")
		_, _ = w.Write([]byte(mpd))
	})
	mux.HandleFunc("/seg1_00001.m4s", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(segData)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	r := dashTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := r.probeDASH(ctx, dashConfig(srv.URL+"/manifest.mpd"), dashResult())

	t.Logf("SegmentTemplate result: success=%v ttfb_ms=%d seg_ttfb_ms=%d bitrate=%.4f code=%q msg=%q",
		result.Success, result.TTFBMs, result.SegmentTTFBMs, result.BitrateKbps, result.ErrorCode, result.ErrorMsg)

	if !result.Success {
		t.Fatalf("expected Success=true, got false: code=%q msg=%q", result.ErrorCode, result.ErrorMsg)
	}
	if result.TTFBMs < 1 {
		t.Errorf("expected TTFBMs >= 1, got %d", result.TTFBMs)
	}
	if result.SegmentTTFBMs < 1 {
		t.Errorf("expected SegmentTTFBMs >= 1, got %d", result.SegmentTTFBMs)
	}
	if result.ErrorCode != "" {
		t.Errorf("expected empty ErrorCode on success, got %q", result.ErrorCode)
	}

	// BitrateKbps must exactly match the formula: bytes*8/segDurS/1000.
	// segDurS = 180000/90000 = 2.0 s.
	wantBitrate := float32(float64(segBytes) * 8.0 / 2.0 / 1000.0)
	if result.BitrateKbps != wantBitrate {
		t.Errorf("BitrateKbps: want %v, got %v", wantBitrate, result.BitrateKbps)
	}
}

// TestProbeDASH_SegmentList verifies success with a SegmentList MPD.
// The probe must use the first <SegmentURL media="..."> as the segment target.
// timescale=1, duration=2 → 2 s/segment; BitrateKbps checked exactly.
func TestProbeDASH_SegmentList(t *testing.T) {
	const segBytes = 8_000
	segData := make([]byte, segBytes)

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.mpd", func(w http.ResponseWriter, r *http.Request) {
		const mpd = `<?xml version="1.0"?>
<MPD>
  <Period>
    <AdaptationSet>
      <Representation id="1">
        <SegmentList timescale="1" duration="2">
          <SegmentURL media="seg001.m4s"/>
          <SegmentURL media="seg002.m4s"/>
        </SegmentList>
      </Representation>
    </AdaptationSet>
  </Period>
</MPD>`
		w.Header().Set("Content-Type", "application/dash+xml")
		_, _ = w.Write([]byte(mpd))
	})
	mux.HandleFunc("/seg001.m4s", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(segData)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	r := dashTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := r.probeDASH(ctx, dashConfig(srv.URL+"/manifest.mpd"), dashResult())

	t.Logf("SegmentList result: success=%v ttfb=%d seg_ttfb=%d bitrate=%.4f code=%q",
		result.Success, result.TTFBMs, result.SegmentTTFBMs, result.BitrateKbps, result.ErrorCode)

	if !result.Success {
		t.Fatalf("expected Success=true, got false: code=%q msg=%q", result.ErrorCode, result.ErrorMsg)
	}
	if result.TTFBMs < 1 {
		t.Errorf("expected TTFBMs >= 1, got %d", result.TTFBMs)
	}
	if result.SegmentTTFBMs < 1 {
		t.Errorf("expected SegmentTTFBMs >= 1, got %d", result.SegmentTTFBMs)
	}
	// segDurS = 2/1 = 2.0 s
	wantBitrate := float32(float64(segBytes) * 8.0 / 2.0 / 1000.0)
	if result.BitrateKbps != wantBitrate {
		t.Errorf("BitrateKbps: want %v, got %v", wantBitrate, result.BitrateKbps)
	}
}

// TestProbeDASH_BaseURLAbsolute verifies RFC 3986 resolution via
// net/url.ResolveReference when the MPD contains an absolute-URL BaseURL element.
// The segment URL is relative and must be resolved against the BaseURL, not the
// MPD URL (this diverges from HLS resolveURI's string-truncation approach).
func TestProbeDASH_BaseURLAbsolute(t *testing.T) {
	const segBytes = 2_000
	segData := make([]byte, segBytes)

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.mpd", func(w http.ResponseWriter, r *http.Request) {
		// MPD BaseURL is absolute (points at this test server's /streams/ prefix).
		// SegmentURL media is relative → resolved against BaseURL → /streams/seg001.m4s.
		mpd := fmt.Sprintf(`<?xml version="1.0"?>
<MPD>
  <BaseURL>%s/streams/</BaseURL>
  <Period>
    <AdaptationSet>
      <Representation id="1">
        <SegmentList timescale="1" duration="4">
          <SegmentURL media="seg001.m4s"/>
        </SegmentList>
      </Representation>
    </AdaptationSet>
  </Period>
</MPD>`, srv.URL)
		w.Header().Set("Content-Type", "application/dash+xml")
		_, _ = w.Write([]byte(mpd))
	})
	mux.HandleFunc("/streams/seg001.m4s", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(segData)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	r := dashTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := r.probeDASH(ctx, dashConfig(srv.URL+"/manifest.mpd"), dashResult())

	t.Logf("BaseURL result: success=%v ttfb=%d seg_ttfb=%d bitrate=%.4f code=%q msg=%q",
		result.Success, result.TTFBMs, result.SegmentTTFBMs, result.BitrateKbps, result.ErrorCode, result.ErrorMsg)

	if !result.Success {
		t.Fatalf("expected Success=true (absolute BaseURL resolution), got false: code=%q msg=%q",
			result.ErrorCode, result.ErrorMsg)
	}
	if result.TTFBMs < 1 {
		t.Errorf("expected TTFBMs >= 1, got %d", result.TTFBMs)
	}
	if result.SegmentTTFBMs < 1 {
		t.Errorf("expected SegmentTTFBMs >= 1 (segment fetched at resolved URL), got %d", result.SegmentTTFBMs)
	}
	if result.BitrateKbps <= 0 {
		t.Errorf("expected BitrateKbps > 0, got %.4f", result.BitrateKbps)
	}
}

// TestProbeDASH_NonXML verifies that a non-XML manifest body (200 OK) returns
// Success=false, ErrorCode="parse".
func TestProbeDASH_NonXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("this is not an MPD or XML document at all"))
	}))
	t.Cleanup(srv.Close)

	r := dashTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := r.probeDASH(ctx, dashConfig(srv.URL+"/manifest.mpd"), dashResult())

	t.Logf("NonXML result: success=%v code=%q msg=%q", result.Success, result.ErrorCode, result.ErrorMsg)

	if result.Success {
		t.Error("expected Success=false on non-XML body")
	}
	if result.ErrorCode != "parse" {
		t.Errorf("expected ErrorCode=parse, got %q", result.ErrorCode)
	}
	// TTFBMs must be set — the manifest was fetched (HTTP 200) before the parse failed.
	if result.TTFBMs < 1 {
		t.Errorf("expected TTFBMs >= 1 (manifest was fetched), got %d", result.TTFBMs)
	}
}

// TestProbeDASH_404Manifest verifies that a 404 manifest response returns
// Success=false, ErrorCode="http_4xx".
func TestProbeDASH_404Manifest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	r := dashTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := r.probeDASH(ctx, dashConfig(srv.URL+"/manifest.mpd"), dashResult())

	t.Logf("404 result: success=%v code=%q msg=%q", result.Success, result.ErrorCode, result.ErrorMsg)

	if result.Success {
		t.Error("expected Success=false on 404")
	}
	if result.ErrorCode != "http_4xx" {
		t.Errorf("expected ErrorCode=http_4xx, got %q", result.ErrorCode)
	}
}

// TestProbeDASH_500Manifest verifies that a 500 manifest response returns
// Success=false, ErrorCode="http_5xx".
func TestProbeDASH_500Manifest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	r := dashTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := r.probeDASH(ctx, dashConfig(srv.URL+"/manifest.mpd"), dashResult())

	t.Logf("500 result: success=%v code=%q msg=%q", result.Success, result.ErrorCode, result.ErrorMsg)

	if result.Success {
		t.Error("expected Success=false on 500")
	}
	if result.ErrorCode != "http_5xx" {
		t.Errorf("expected ErrorCode=http_5xx, got %q", result.ErrorCode)
	}
}

// TestProbeDASH_Segment404 verifies the HLS-mirror semantics for segment
// failures: a 404 on the segment (after a parseable MPD) leaves
// Success=true, SegmentTTFBMs=0, BitrateKbps=0.
// Mirrors prober.go:460-512 precedent exactly.
func TestProbeDASH_Segment404(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.mpd", func(w http.ResponseWriter, r *http.Request) {
		// Valid MPD pointing at /missing_segment.m4s (not registered → 404).
		const mpd = `<?xml version="1.0"?>
<MPD>
  <Period>
    <AdaptationSet>
      <Representation id="1">
        <SegmentList timescale="1" duration="2">
          <SegmentURL media="missing_segment.m4s"/>
        </SegmentList>
      </Representation>
    </AdaptationSet>
  </Period>
</MPD>`
		w.Header().Set("Content-Type", "application/dash+xml")
		_, _ = w.Write([]byte(mpd))
	})
	// /missing_segment.m4s is intentionally unregistered → 404 from ServeMux.
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	r := dashTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := r.probeDASH(ctx, dashConfig(srv.URL+"/manifest.mpd"), dashResult())

	t.Logf("Segment404 result: success=%v ttfb=%d seg_ttfb=%d bitrate=%.4f code=%q msg=%q",
		result.Success, result.TTFBMs, result.SegmentTTFBMs, result.BitrateKbps, result.ErrorCode, result.ErrorMsg)

	if !result.Success {
		t.Errorf("expected Success=true (manifest was parseable), got false: code=%q msg=%q",
			result.ErrorCode, result.ErrorMsg)
	}
	if result.SegmentTTFBMs != 0 {
		t.Errorf("expected SegmentTTFBMs=0 on segment 404, got %d", result.SegmentTTFBMs)
	}
	if result.BitrateKbps != 0 {
		t.Errorf("expected BitrateKbps=0 on segment 404, got %v", result.BitrateKbps)
	}
}

// TestProbeDASH_NoSegmentInfo verifies that a valid MPD XML document containing
// a Representation but no SegmentTemplate or SegmentList returns
// Success=false, ErrorCode="parse".
//
// Per D-073 ruling: 'parses as MPD' requires a derivable segment URL;
// a well-formed document without segment addressing is a probe failure.
func TestProbeDASH_NoSegmentInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Valid XML, valid MPD structure, but no SegmentTemplate or SegmentList.
		const mpd = `<?xml version="1.0"?>
<MPD>
  <Period>
    <AdaptationSet>
      <Representation id="1" bandwidth="1000000" width="1280" height="720"/>
    </AdaptationSet>
  </Period>
</MPD>`
		w.Header().Set("Content-Type", "application/dash+xml")
		_, _ = w.Write([]byte(mpd))
	}))
	t.Cleanup(srv.Close)

	r := dashTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := r.probeDASH(ctx, dashConfig(srv.URL+"/manifest.mpd"), dashResult())

	t.Logf("NoSegmentInfo result: success=%v code=%q msg=%q", result.Success, result.ErrorCode, result.ErrorMsg)

	if result.Success {
		t.Error("expected Success=false (no SegmentTemplate/SegmentList → cannot derive segment)")
	}
	if result.ErrorCode != "parse" {
		t.Errorf("expected ErrorCode=parse, got %q", result.ErrorCode)
	}
}

// TestProbeDASH_Timeout verifies that a slow origin (server blocks until
// context is cancelled) causes Success=false, ErrorCode="timeout".
// The context deadline (500 ms) fires before the server writes any bytes.
// Total test wall-time is bounded well under 5 s.
func TestProbeDASH_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block forever — mirrors TestHLSProbe_Timeout technique (prober_test.go:332-338).
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	r := dashTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := r.probeDASH(ctx, dashConfig(srv.URL+"/manifest.mpd"), dashResult())
	elapsed := time.Since(start)

	t.Logf("Timeout result: success=%v code=%q elapsed=%v msg=%q",
		result.Success, result.ErrorCode, elapsed, result.ErrorMsg)

	if result.Success {
		t.Error("expected Success=false on timeout")
	}
	if result.ErrorCode != "timeout" {
		t.Errorf("expected ErrorCode=timeout, got %q", result.ErrorCode)
	}
	// Sanity: test must complete well within 5 s (context fires at 500 ms).
	if elapsed > 5*time.Second {
		t.Errorf("test took too long (%v); expected < 5s", elapsed)
	}
}

// TestProbeDASH_BaseURLChain verifies the full BaseURL resolution chain
// (ISO/IEC 23009-1 §5.6): MPD → Period → AdaptationSet → Representation,
// each level resolved against its parent's effective base (D-073 verifier
// finding: Period/AdaptationSet-level BaseURL elements were ignored).
func TestProbeDASH_BaseURLChain(t *testing.T) {
	const segBytes = 4_000
	segData := make([]byte, segBytes)

	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.mpd", func(w http.ResponseWriter, r *http.Request) {
		// Chain: MPD URL dir → Period "p/" → AdaptationSet "a/" →
		// Representation "r/" → media "seg1.m4s" ⇒ /p/a/r/seg1.m4s.
		mpd := `<?xml version="1.0"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011">
  <Period>
    <BaseURL>p/</BaseURL>
    <AdaptationSet>
      <BaseURL>a/</BaseURL>
      <Representation id="1">
        <BaseURL>r/</BaseURL>
        <SegmentList timescale="2" duration="8">
          <SegmentURL media="seg1.m4s"/>
        </SegmentList>
      </Representation>
    </AdaptationSet>
  </Period>
</MPD>`
		w.Header().Set("Content-Type", "application/dash+xml")
		_, _ = w.Write([]byte(mpd))
	})
	mux.HandleFunc("/p/a/r/seg1.m4s", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(segData)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	r := dashTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := r.probeDASH(ctx, dashConfig(srv.URL+"/manifest.mpd"), dashResult())

	t.Logf("BaseURL-chain result: success=%v ttfb=%d seg_ttfb=%d bitrate=%.4f code=%q msg=%q",
		result.Success, result.TTFBMs, result.SegmentTTFBMs, result.BitrateKbps, result.ErrorCode, result.ErrorMsg)

	if !result.Success {
		t.Fatalf("expected Success=true (chained BaseURL resolution), got false: code=%q msg=%q",
			result.ErrorCode, result.ErrorMsg)
	}
	if result.SegmentTTFBMs < 1 {
		t.Fatalf("expected SegmentTTFBMs >= 1 (segment fetched via chained base), got %d", result.SegmentTTFBMs)
	}
	// timescale=2, duration=8 → 4s; 4000 bytes*8/4/1000 = 8 kbps.
	if result.BitrateKbps < 7.9 || result.BitrateKbps > 8.1 {
		t.Fatalf("expected BitrateKbps ≈ 8 (timescale-adjusted via chain), got %.4f", result.BitrateKbps)
	}
}
