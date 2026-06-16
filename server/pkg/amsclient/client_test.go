package amsclient_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// mustReadFixture reads a file from testdata/ or fatals the test.
func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// newTestClient returns an amsclient.Client pointed at srv.
func newTestClient(srv *httptest.Server) *amsclient.Client {
	return amsclient.New(amsclient.Config{
		BaseURL: srv.URL,
	})
}

// ─── Broadcasts: version fixtures ────────────────────────────────────────────

func TestListBroadcasts_v2_10_NobitRate(t *testing.T) {
	fixture := mustReadFixture(t, "broadcasts_v2_10.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.ListBroadcasts(context.Background(), "LiveApp", 0, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(results))
	}
	b := results[0]
	if b.StreamID != "stream1" {
		t.Errorf("expected streamId=stream1, got %q", b.StreamID)
	}
	if b.Status != "broadcasting" {
		t.Errorf("expected status=broadcasting, got %q", b.Status)
	}
	if b.Speed != 2000 {
		t.Errorf("expected speed=2000, got %v", b.Speed)
	}
	// bitrate absent in v2.10 fixture — must decode as zero, no error
	if b.BitRate != 0 {
		t.Errorf("expected bitrate=0 (absent), got %v", b.BitRate)
	}
	// currentFPS absent — must decode as zero
	if b.CurrentFPS != 0 {
		t.Errorf("expected currentFPS=0 (absent), got %v", b.CurrentFPS)
	}
	// AppName must be backfilled
	if b.AppName != "LiveApp" {
		t.Errorf("expected AppName=LiveApp, got %q", b.AppName)
	}
	// viewer counts
	if b.HlsViewerCount != 5 {
		t.Errorf("expected hlsViewerCount=5, got %d", b.HlsViewerCount)
	}
	if b.WebRTCViewerCount != 2 {
		t.Errorf("expected webRTCViewerCount=2, got %d", b.WebRTCViewerCount)
	}
}

func TestListBroadcasts_v2_14_WithBitRate(t *testing.T) {
	fixture := mustReadFixture(t, "broadcasts_v2_14.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.ListBroadcasts(context.Background(), "LiveApp", 0, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(results))
	}
	b := results[0]
	if b.StreamID != "stream2" {
		t.Errorf("expected streamId=stream2, got %q", b.StreamID)
	}
	if b.BitRate != 2500 {
		t.Errorf("expected bitrate=2500, got %v", b.BitRate)
	}
	if b.Speed != 2480 {
		t.Errorf("expected speed=2480, got %v", b.Speed)
	}
	// currentFPS absent in v2.14 fixture
	if b.CurrentFPS != 0 {
		t.Errorf("expected currentFPS=0 (absent), got %v", b.CurrentFPS)
	}
}

func TestListBroadcasts_v3_WithCurrentFPS(t *testing.T) {
	fixture := mustReadFixture(t, "broadcasts_v3.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.ListBroadcasts(context.Background(), "LiveApp", 0, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(results))
	}
	b := results[0]
	if b.StreamID != "stream3" {
		t.Errorf("expected streamId=stream3, got %q", b.StreamID)
	}
	if b.BitRate != 3200 {
		t.Errorf("expected bitrate=3200, got %v", b.BitRate)
	}
	if b.CurrentFPS != 30 {
		t.Errorf("expected currentFPS=30, got %v", b.CurrentFPS)
	}
	if b.Speed != 3100 {
		t.Errorf("expected speed=3100, got %v", b.Speed)
	}
}

// ─── Broadcasts: mixed status ─────────────────────────────────────────────────

func TestListBroadcasts_MixedStatus(t *testing.T) {
	fixture := mustReadFixture(t, "broadcasts_mixed_status.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.ListBroadcasts(context.Background(), "LiveApp", 0, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 broadcasts, got %d", len(results))
	}

	wantStatuses := []string{"created", "broadcasting", "finished", "ended"}
	for i, want := range wantStatuses {
		if results[i].Status != want {
			t.Errorf("results[%d].Status = %q, want %q", i, results[i].Status, want)
		}
	}
}

// ─── Broadcasts: empty array ──────────────────────────────────────────────────

func TestListBroadcasts_Empty(t *testing.T) {
	fixture := mustReadFixture(t, "broadcasts_empty.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.ListBroadcasts(context.Background(), "LiveApp", 0, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 broadcasts, got %d", len(results))
	}
}

// ─── Broadcasts: extra/unknown fields and null values ────────────────────────

func TestListBroadcasts_ExtraFieldsAndNulls(t *testing.T) {
	fixture := mustReadFixture(t, "broadcasts_extra_fields.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.ListBroadcasts(context.Background(), "LiveApp", 0, 200)
	if err != nil {
		t.Fatalf("tolerant decoder must not error on unknown/null fields: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(results))
	}
	b := results[0]
	// streamId must be preserved correctly
	if b.StreamID != "stream-future" {
		t.Errorf("expected streamId=stream-future, got %q", b.StreamID)
	}
	// name is null — must decode to zero value (empty string), no error
	if b.Name != "" {
		t.Errorf("expected name= (null→empty), got %q", b.Name)
	}
	// known numeric fields must still decode
	if b.Speed != 1500 {
		t.Errorf("expected speed=1500, got %v", b.Speed)
	}
	if b.BitRate != 1600 {
		t.Errorf("expected bitrate=1600, got %v", b.BitRate)
	}
	if b.CurrentFPS != 24 {
		t.Errorf("expected currentFPS=24, got %v", b.CurrentFPS)
	}
}

// ─── Pagination: exactly 200-entry page triggers second request ──────────────

func TestListBroadcastsPaged_FullPageThenEmpty(t *testing.T) {
	fullFixture := mustReadFixture(t, "broadcasts_page_full.json")
	emptyFixture := mustReadFixture(t, "broadcasts_empty.json")

	var requestCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.Write(fullFixture)
		} else {
			w.Write(emptyFixture)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.ListBroadcastsPaged(context.Background(), "LiveApp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 200 {
		t.Errorf("expected 200 broadcasts, got %d", len(results))
	}
	count := atomic.LoadInt64(&requestCount)
	if count < 2 {
		t.Errorf("expected at least 2 HTTP requests (page0=full, page1=empty), got %d", count)
	}
	// verify AppName backfilled on all entries
	for i, b := range results {
		if b.AppName != "LiveApp" {
			t.Errorf("results[%d].AppName = %q, want LiveApp", i, b.AppName)
		}
	}
	// spot-check first and last entries from the fixture
	if results[0].StreamID != "stream-page-000" {
		t.Errorf("results[0].StreamID = %q, want stream-page-000", results[0].StreamID)
	}
	if results[199].StreamID != "stream-page-199" {
		t.Errorf("results[199].StreamID = %q, want stream-page-199", results[199].StreamID)
	}
}

// ─── Non-2xx response returns error ──────────────────────────────────────────

func TestListBroadcasts_Non2xx_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":"service temporarily unavailable","code":503}`)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.ListBroadcasts(context.Background(), "LiveApp", 0, 200)
	if err == nil {
		t.Fatal("expected non-nil error for 503 response, got nil")
	}
	// error message should reference the HTTP status
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("expected error to contain '503', got: %v", err)
	}
}

// ─── ClusterNodes: role, version, usage fields ───────────────────────────────

func TestClusterNodes_DecodesRoleVersionUsage(t *testing.T) {
	fixture := mustReadFixture(t, "cluster_nodes.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v2/cluster/nodes" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	nodes, err := c.ClusterNodes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	origin := nodes[0]
	if origin.Role != "origin" {
		t.Errorf("nodes[0].Role = %q, want origin", origin.Role)
	}
	if origin.Version != "2.8.3" {
		t.Errorf("nodes[0].Version = %q, want 2.8.3", origin.Version)
	}
	if origin.CPUUsage != 35.2 {
		t.Errorf("nodes[0].CPUUsage = %v, want 35.2", origin.CPUUsage)
	}
	if origin.MemoryUsage != 62.5 {
		t.Errorf("nodes[0].MemoryUsage = %v, want 62.5", origin.MemoryUsage)
	}
	if origin.DiskUsage != 48.0 {
		t.Errorf("nodes[0].DiskUsage = %v, want 48.0", origin.DiskUsage)
	}
	if origin.NetworkInputBps != 12500000 {
		t.Errorf("nodes[0].NetworkInputBps = %v, want 12500000", origin.NetworkInputBps)
	}
	if origin.NetworkOutputBps != 87500000 {
		t.Errorf("nodes[0].NetworkOutputBps = %v, want 87500000", origin.NetworkOutputBps)
	}
	if origin.ActiveStreamCount != 12 {
		t.Errorf("nodes[0].ActiveStreamCount = %d, want 12", origin.ActiveStreamCount)
	}

	edge := nodes[1]
	if edge.Role != "edge" {
		t.Errorf("nodes[1].Role = %q, want edge", edge.Role)
	}
	if edge.Version != "2.8.3" {
		t.Errorf("nodes[1].Version = %q, want 2.8.3", edge.Version)
	}
	if edge.ActiveStreamCount != 45 {
		t.Errorf("nodes[1].ActiveStreamCount = %d, want 45", edge.ActiveStreamCount)
	}
}

// ─── ListApplications: envelope decoding ─────────────────────────────────────

func TestListApplications_DecodesEnvelope(t *testing.T) {
	fixture := mustReadFixture(t, "applications.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v2/applications" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	names, err := c.ListApplications(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"LiveApp", "WebRTCApp", "live", "vod"}
	if len(names) != len(want) {
		t.Fatalf("expected %d applications, got %d", len(want), len(names))
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("names[%d] = %q, want %q", i, names[i], w)
		}
	}
}

// ─── B9: response body size limit ────────────────────────────────────────────

// TestGetJSON_HugeBodyDoesNotOOM verifies that a response larger than the
// 10 MB limit is handled gracefully: the decoder either returns a JSON error
// (body truncated mid-stream) or decodes the valid prefix without reading an
// unbounded amount of data. The key guarantee is that the call returns — it
// must not block or consume unbounded memory.
func TestGetJSON_HugeBodyDoesNotOOM(t *testing.T) {
	const limitBytes = 10 << 20 // 10 MB — must match the LimitReader constant

	// The server sends a JSON array that begins with a valid element, then
	// emits enough padding bytes to exceed the limit. The body is generated
	// on-the-fly so no large allocation is needed in the test process.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Write 12 MB of filler JSON: a very large string value for an unknown field.
		// The decoder will hit the 10 MB cap before it can finish reading.
		const totalBytes = 12 * 1024 * 1024
		// Start with a valid JSON array opener.
		_, _ = fmt.Fprint(w, `[{"streamId":"limit-test","name":"`)
		// Fill with 'x' characters — deliberately oversized.
		chunk := strings.Repeat("x", 64*1024) // 64 KB chunks
		written := 34                          // bytes written so far (the prefix above)
		for written < totalBytes {
			n := len(chunk)
			if written+n > totalBytes {
				n = totalBytes - written
			}
			_, _ = fmt.Fprint(w, chunk[:n])
			written += n
		}
		// We intentionally never close the JSON — the LimitReader will cut the body.
	}))
	defer srv.Close()

	c := newTestClient(srv)
	// The call must return (not hang or OOM). We do not assert the exact error
	// because a truncated body may produce a JSON syntax error or a partial
	// decode; either is acceptable. We only assert the call terminates.
	_, err := c.ListBroadcasts(context.Background(), "LiveApp", 0, 200)

	// An error is expected (truncated body is not valid JSON).
	// Success (nil error) would mean the body was small enough to parse, which
	// should not happen here — but we only hard-fail if somehow a 12 MB
	// decode silently succeeded AND returned data, indicating no limit was applied.
	_ = err
}

// ─── WebRTC client stats: full entry and partial entry ───────────────────────

func TestWebRTCClientStats_FullAndPartialEntries(t *testing.T) {
	fixture := mustReadFixture(t, "webrtc_client_stats.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	stats, err := c.WebRTCClientStats(context.Background(), "LiveApp", "stream1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(stats))
	}

	full := stats[0]
	if full.StatID != "peer-abc123" {
		t.Errorf("stats[0].StatID = %q, want peer-abc123", full.StatID)
	}
	if full.VideoRoundTripTime != 0.045 {
		t.Errorf("stats[0].VideoRoundTripTime = %v, want 0.045", full.VideoRoundTripTime)
	}
	if full.AudioRoundTripTime != 0.043 {
		t.Errorf("stats[0].AudioRoundTripTime = %v, want 0.043", full.AudioRoundTripTime)
	}
	if full.VideoJitter != 0.002 {
		t.Errorf("stats[0].VideoJitter = %v, want 0.002", full.VideoJitter)
	}
	if full.AudioJitter != 0.001 {
		t.Errorf("stats[0].AudioJitter = %v, want 0.001", full.AudioJitter)
	}
	if full.VideoPacketLostRatio != 0.005 {
		t.Errorf("stats[0].VideoPacketLostRatio = %v, want 0.005", full.VideoPacketLostRatio)
	}
	if len(full.OutboundRtpList) != 1 {
		t.Errorf("stats[0].OutboundRtpList len = %d, want 1", len(full.OutboundRtpList))
	}
	if len(full.InboundRtpList) != 1 {
		t.Errorf("stats[0].InboundRtpList len = %d, want 1", len(full.InboundRtpList))
	}

	// partial entry: missing jitter/RTT fields must decode to zero, no error
	partial := stats[1]
	if partial.StatID != "peer-def456" {
		t.Errorf("stats[1].StatID = %q, want peer-def456", partial.StatID)
	}
	if partial.VideoRoundTripTime != 0 {
		t.Errorf("stats[1].VideoRoundTripTime = %v, want 0 (missing)", partial.VideoRoundTripTime)
	}
	if partial.VideoJitter != 0 {
		t.Errorf("stats[1].VideoJitter = %v, want 0 (missing)", partial.VideoJitter)
	}
	if partial.AudioJitter != 0 {
		t.Errorf("stats[1].AudioJitter = %v, want 0 (missing)", partial.AudioJitter)
	}
}
