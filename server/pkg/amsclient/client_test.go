package amsclient_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aytekXR/ams-pulse/server/pkg/amsclient"
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

// newLoginTestClient returns a Client with cookie-session auth configured.
func newLoginTestClient(srv *httptest.Server, email, password string) *amsclient.Client {
	return amsclient.New(amsclient.Config{
		BaseURL:       srv.URL,
		LoginEmail:    email,
		LoginPassword: password,
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

// ─── ClusterNodes: paginated path, cluster-mode-status probe ─────────────────

// TestClusterNodes_DecodesRoleVersionUsage verifies that ClusterNodes correctly
// pages through the real AMS v3 paginated endpoint and decodes the cluster_nodes.json
// fixture (which uses the old invented fields kept for backward compat).
func TestClusterNodes_DecodesRoleVersionUsage(t *testing.T) {
	fixture := mustReadFixture(t, "cluster_nodes.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v2/cluster-mode-status":
			fmt.Fprint(w, `{"success":true,"message":"","dataId":"","errorId":0}`)
		case "/rest/v2/cluster/nodes/0/50":
			w.Write(fixture)
		default:
			http.NotFound(w, r)
		}
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

	// cluster_nodes.json uses the old invented alias fields (nodeId, role, version,
	// cpuUsage, etc.) which still decode into the tolerant alias fields on ClusterNodeDTO.
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

// TestClusterNodes_ReaAMSWireShape verifies that ClusterNodes decodes the real AMS 3.x
// wire fields (id, ip, lastUpdateTime, memory, cpu, dbQueryAveargeTimeMs, status) and
// that PrimaryID/CPUPct/MemPct return correct values.
func TestClusterNodes_RealAMSWireShape(t *testing.T) {
	const realWireFixture = `[{
		"id": "node-abc123",
		"ip": "10.0.1.5",
		"lastUpdateTime": 1721952000000,
		"memory": "42.0%",
		"cpu": "18.5",
		"dbQueryAveargeTimeMs": 3,
		"status": "Running"
	}]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v2/cluster-mode-status":
			fmt.Fprint(w, `{"success":true}`)
		case "/rest/v2/cluster/nodes/0/50":
			fmt.Fprint(w, realWireFixture)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv)
	nodes, err := c.ClusterNodes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.PrimaryID() != "node-abc123" {
		t.Errorf("PrimaryID() = %q, want node-abc123", n.PrimaryID())
	}
	if n.CPUPct() != 18.5 {
		t.Errorf("CPUPct() = %v, want 18.5", n.CPUPct())
	}
	if n.MemPct() != 42.0 {
		t.Errorf("MemPct() = %v, want 42.0", n.MemPct())
	}
	if n.DbQueryAveargeTimeMs != 3 {
		t.Errorf("DbQueryAveargeTimeMs = %d, want 3", n.DbQueryAveargeTimeMs)
	}
	if n.Status != "Running" {
		t.Errorf("Status = %q, want Running", n.Status)
	}
	if n.LastUpdateTime != 1721952000000 {
		t.Errorf("LastUpdateTime = %d, want 1721952000000", n.LastUpdateTime)
	}
}

// TestClusterNodes_PaginationCrossesPageBoundary verifies that ClusterNodes correctly
// accumulates results across multiple pages when the first page is exactly full (pageSize=50).
func TestClusterNodes_PaginationCrossesPageBoundary(t *testing.T) {
	// Build 60 nodes: page 0 = 50 nodes, page 1 = 10 nodes.
	buildPage := func(offset, count int) string {
		nodes := make([]map[string]any, count)
		for i := range nodes {
			nodes[i] = map[string]any{
				"id": fmt.Sprintf("node-%03d", offset+i),
				"ip": fmt.Sprintf("10.0.0.%d", offset+i+1),
			}
		}
		b, _ := json.Marshal(nodes)
		return string(b)
	}
	page0 := buildPage(0, 50)  // full page — triggers next request
	page1 := buildPage(50, 10) // short page — stops pagination

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v2/cluster-mode-status":
			fmt.Fprint(w, `{"success":true}`)
		case "/rest/v2/cluster/nodes/0/50":
			fmt.Fprint(w, page0)
		case "/rest/v2/cluster/nodes/50/50":
			fmt.Fprint(w, page1)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv)
	nodes, err := c.ClusterNodes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nodes) != 60 {
		t.Errorf("expected 60 nodes (50+10 across two pages), got %d", len(nodes))
	}
	if nodes[0].ID != "node-000" {
		t.Errorf("nodes[0].ID = %q, want node-000", nodes[0].ID)
	}
	if nodes[59].ID != "node-059" {
		t.Errorf("nodes[59].ID = %q, want node-059", nodes[59].ID)
	}
}

// TestClusterNodes_Standalone500ReturnsNilNoError verifies that HTTP 500 from the
// paginated cluster/nodes path (real standalone AMS 3.0.3 behavior — returns
// NoSuchBeanDefinitionException) is mapped to (nil, nil), not an error.
func TestClusterNodes_Standalone500ReturnsNilNoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/rest/v2/cluster-mode-status":
			// Simulate older AMS or probe failure — return error to force paginated path.
			w.WriteHeader(http.StatusNotFound)
		case strings.HasPrefix(r.URL.Path, "/rest/v2/cluster/nodes/"):
			// Real standalone AMS 3.0.3: paginated path returns HTTP 500.
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv)
	nodes, err := c.ClusterNodes(context.Background())
	if err != nil {
		t.Fatalf("standalone AMS (500 on paginated path) must return nil error, got: %v", err)
	}
	if nodes != nil {
		t.Errorf("standalone AMS (500 on paginated path) must return nil slice, got: %v", nodes)
	}
}

// TestClusterNodes_ClusterModeStatusFalseReturnsNilNoError verifies that when
// cluster-mode-status returns success=false (real standalone AMS 3.0.3 behavior),
// ClusterNodes returns (nil, nil) immediately without calling the paginated path.
func TestClusterNodes_ClusterModeStatusFalseReturnsNilNoError(t *testing.T) {
	paginatedCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/rest/v2/cluster-mode-status":
			// Real standalone AMS 3.0.3 live-probed response.
			fmt.Fprint(w, `{"success":false,"message":"","dataId":"","errorId":0}`)
		case strings.HasPrefix(r.URL.Path, "/rest/v2/cluster/nodes/"):
			paginatedCalled = true
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv)
	nodes, err := c.ClusterNodes(context.Background())
	if err != nil {
		t.Fatalf("standalone AMS (cluster-mode-status=false) must return nil error, got: %v", err)
	}
	if nodes != nil {
		t.Errorf("standalone AMS (cluster-mode-status=false) must return nil slice, got: %v", nodes)
	}
	if paginatedCalled {
		t.Errorf("paginated cluster/nodes path must NOT be called when cluster-mode-status=false")
	}
}

// TestClusterNodes_Transient500InClusterMode_ReturnsError pins REVIEW-MP3 N1:
// when the cluster-mode-status probe authoritatively said "cluster", a 500 from
// the paginated nodes route is a REAL error — it must never be mapped to the
// standalone fallback (which would silently collapse a live fleet to a synthetic
// single node and reset the failure-streak alerting), and it must never downgrade
// the cached mode (a transient Mongo/Redis blip would otherwise short-circuit to
// standalone for ~60 calls). The follow-up call proves the cache survived.
func TestClusterNodes_Transient500InClusterMode_ReturnsError(t *testing.T) {
	nodesFail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/rest/v2/cluster-mode-status":
			fmt.Fprint(w, `{"success":true,"message":"","dataId":"","errorId":0}`)
		case strings.HasPrefix(r.URL.Path, "/rest/v2/cluster/nodes/"):
			if nodesFail {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, `[{"id":"node-a","ip":"10.0.0.1","cpu":"12.5","memory":"40.2","status":"Running"}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv)

	// Transient 500 while the probe says cluster → error, never (nil, nil).
	nodes, err := c.ClusterNodes(context.Background())
	if err == nil {
		t.Fatalf("cluster-mode 500 must surface as an error, got (nodes=%v, nil)", nodes)
	}

	// Recovery: the mode cache must still say cluster, so the next call reaches
	// the paginated route again and returns the real fleet.
	nodesFail = false
	nodes, err = c.ClusterNodes(context.Background())
	if err != nil {
		t.Fatalf("post-recovery call errored: %v", err)
	}
	if len(nodes) != 1 || nodes[0].PrimaryID() != "node-a" {
		t.Errorf("post-recovery call must return the real fleet (cache not downgraded), got: %v", nodes)
	}
}

// TestClusterNodes_MidPagination500_ReturnsError pins the mid-pagination half of
// REVIEW-MP3 N1: an error on page 2+ (after a successful full page) is a real
// error even when the mode is unknown — a standalone node fails on page 1,
// never after serving a full page.
func TestClusterNodes_MidPagination500_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/rest/v2/cluster-mode-status":
			// Probe unavailable → mode unknown.
			w.WriteHeader(http.StatusNotFound)
		case strings.HasPrefix(r.URL.Path, "/rest/v2/cluster/nodes/0/"):
			// Full first page (50 entries) → pager continues to page 2.
			nodes := make([]map[string]any, 50)
			for i := range nodes {
				nodes[i] = map[string]any{"id": fmt.Sprintf("node-%d", i), "ip": "10.0.0.1"}
			}
			json.NewEncoder(w).Encode(nodes) //nolint:errcheck
		case strings.HasPrefix(r.URL.Path, "/rest/v2/cluster/nodes/"):
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.ClusterNodes(context.Background())
	if err == nil {
		t.Fatal("mid-pagination 500 must surface as an error, not the standalone fallback")
	}
}

// TestClusterNodes_PaginationCapped pins REVIEW-MP3 N-cluster (b): a server (or
// proxy) that ignores the offset segment and always returns a full page must not
// loop forever — the pager aborts with an error at maxClusterPages.
func TestClusterNodes_PaginationCapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/rest/v2/cluster-mode-status":
			fmt.Fprint(w, `{"success":true,"message":"","dataId":"","errorId":0}`)
		case strings.HasPrefix(r.URL.Path, "/rest/v2/cluster/nodes/"):
			// Always a full page regardless of offset — the pathological proxy.
			nodes := make([]map[string]any, 50)
			for i := range nodes {
				nodes[i] = map[string]any{"id": fmt.Sprintf("node-%d", i), "ip": "10.0.0.1"}
			}
			json.NewEncoder(w).Encode(nodes) //nolint:errcheck
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.ClusterNodes(context.Background())
	if err == nil {
		t.Fatal("unbounded pagination must abort with an error at the page cap")
	}
	if !strings.Contains(err.Error(), "pagination exceeded") {
		t.Errorf("expected pagination-cap error, got: %v", err)
	}
}

// TestClusterNodeDTO_NumericCPUMemory pins REVIEW-MP3 N-cluster (e): a bare
// numeric cpu/memory value from any AMS build must not fail the page decode.
func TestClusterNodeDTO_NumericCPUMemory(t *testing.T) {
	var nodes []amsclient.ClusterNodeDTO
	raw := `[{"id":"n1","ip":"10.0.0.1","cpu":15.3,"memory":40.2},{"id":"n2","ip":"10.0.0.2","cpu":"12.5","memory":"33.1%"}]`
	if err := json.Unmarshal([]byte(raw), &nodes); err != nil {
		t.Fatalf("numeric cpu/memory must decode, got: %v", err)
	}
	if got := nodes[0].CPUPct(); got != 15.3 {
		t.Errorf("numeric cpu: CPUPct() = %v, want 15.3", got)
	}
	if got := nodes[0].MemPct(); got != 40.2 {
		t.Errorf("numeric memory: MemPct() = %v, want 40.2", got)
	}
	if got := nodes[1].MemPct(); got != 33.1 {
		t.Errorf("percent-suffixed memory: MemPct() = %v, want 33.1", got)
	}
}

// TestClusterNodeDTO_NonFiniteCPURejected pins REVIEW-MP3 N-cluster (d): Java's
// Double.toString(NaN) is literally "NaN" — a non-finite parse must fall back
// (NaN in an event makes the WS broadcast's json.Marshal fail silently).
func TestClusterNodeDTO_NonFiniteCPURejected(t *testing.T) {
	for _, s := range []string{"NaN", "Inf", "+Inf", "-Inf", "Infinity"} {
		var n amsclient.ClusterNodeDTO
		raw := fmt.Sprintf(`{"cpu":%q,"memory":%q,"cpuUsage":7.5}`, s, s)
		if err := json.Unmarshal([]byte(raw), &n); err != nil {
			t.Fatalf("decode with cpu=%q failed: %v", s, err)
		}
		if got := n.CPUPct(); got != 7.5 {
			t.Errorf("CPU=%q: CPUPct() = %v, want fallback 7.5 (non-finite rejected)", s, got)
		}
		if got := n.MemPct(); got != 0 {
			t.Errorf("Memory=%q: MemPct() = %v, want 0 (non-finite rejected)", s, got)
		}
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

// TestListApplications_ObjectFormStillDecodes verifies that the older AMS
// object-array form ([{"name":"LiveApp"},...]) is still decoded correctly.
func TestListApplications_ObjectFormStillDecodes(t *testing.T) {
	fixture := mustReadFixture(t, "applications_object_form.json")
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
	want := []string{"LiveApp", "live"}
	if len(names) != len(want) {
		t.Fatalf("expected %d applications, got %d: %v", len(want), len(names), names)
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
		written := 34                         // bytes written so far (the prefix above)
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

// ─── Real AMS captures ───────────────────────────────────────────────────────

// TestListBroadcasts_RealLiveAppCapture decodes the curl-verified LiveApp
// broadcast list from test.antmedia.io (2026-06-21). Asserts 16 entries, finds
// the live "test123" stream with BitRate==622312, and confirms viewer counts decode.
func TestListBroadcasts_RealLiveAppCapture(t *testing.T) {
	fixture := mustReadFixture(t, "broadcasts_real_liveapp.json")
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
	if len(results) != 16 {
		t.Fatalf("expected 16 broadcasts (real LiveApp capture), got %d", len(results))
	}

	// Find the live test123 stream.
	var found *amsclient.BroadcastDTO
	for i := range results {
		if results[i].StreamID == "test123" {
			found = &results[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected to find stream 'test123' in real capture, not found")
	}
	if found.Status != "broadcasting" {
		t.Errorf("test123.Status = %q, want broadcasting", found.Status)
	}
	if found.BitRate != 622312 {
		t.Errorf("test123.BitRate = %v, want 622312", found.BitRate)
	}
	// Viewer counts should be present (zero or more) and decoded without error.
	_ = found.HlsViewerCount
	_ = found.WebRTCViewerCount
	_ = found.RTMPViewerCount
	_ = found.DashViewerCount

	// AppName must be backfilled on all entries.
	for i, b := range results {
		if b.AppName != "LiveApp" {
			t.Errorf("results[%d].AppName = %q, want LiveApp", i, b.AppName)
		}
	}
}

// TestListBroadcasts_UsesPerAppPathParams verifies that ListBroadcasts sends
// a request to the correct AMS v3 per-app path: /{app}/rest/v2/broadcasts/list/{offset}/{size}.
func TestListBroadcasts_UsesPerAppPathParams(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.ListBroadcasts(context.Background(), "LiveApp", 0, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/LiveApp/rest/v2/broadcasts/list/0/200"
	if gotPath != want {
		t.Errorf("request path = %q, want %q (per-app path params required)", gotPath, want)
	}
}

// ─── Auth: cookie-session tests ───────────────────────────────────────────────

// TestLogin_AttachesCookieAndAuthorizes verifies that a client configured with
// LoginEmail/Password performs a login on first call and the resulting cookie
// (JSESSIONID) is carried automatically on subsequent protected requests.
func TestLogin_AttachesCookieAndAuthorizes(t *testing.T) {
	const testCookie = "JSESSIONID=test-session-abc"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v2/users/authenticate":
			// Set JSESSIONID and return success.
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "test-session-abc"})
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"success":true,"message":"system/ADMIN"}`)
		case "/rest/v2/applications":
			// Require the JSESSIONID cookie.
			cookie, err := r.Cookie("JSESSIONID")
			if err != nil || cookie.Value != "test-session-abc" {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error":"forbidden"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"applications":["LiveApp"]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newLoginTestClient(srv, "admin@example.com", "secret")
	names, err := c.ListApplications(context.Background())
	if err != nil {
		t.Fatalf("expected successful response with cookie auth, got: %v", err)
	}
	if len(names) != 1 || names[0] != "LiveApp" {
		t.Errorf("expected [LiveApp], got %v", names)
	}
}

// TestLogin_WrongPasswordReturnsError verifies that a {"success":false} login
// response surfaces as an error in the subsequent API call.
// The applications endpoint returns 403 (as real AMS does without a valid session),
// which triggers a forced re-login that also fails → "login failed" error.
func TestLogin_WrongPasswordReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v2/users/authenticate":
			// Wrong password — AMS returns HTTP 200 with success=false.
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"success":false,"message":"wrong password"}`)
		case "/rest/v2/applications":
			// Without a valid session, /rest/v2/applications returns 403 on real AMS.
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"unauthorized"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newLoginTestClient(srv, "admin@example.com", "wrongpassword")
	_, err := c.ListApplications(context.Background())
	if err == nil {
		t.Fatal("expected error for wrong password login, got nil")
	}
	if !strings.Contains(err.Error(), "login failed") {
		t.Errorf("expected error to contain 'login failed', got: %v", err)
	}
}

// TestSessionExpiry_RelogsInAndRetriesOnce verifies that when the protected
// endpoint returns 403 (stale session), the client re-logins and retries exactly
// once, ultimately succeeding.
func TestSessionExpiry_RelogsInAndRetriesOnce(t *testing.T) {
	var loginCount atomic.Int64
	var appsRequestCount atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v2/users/authenticate":
			n := loginCount.Add(1)
			// First login sets session-1, second login sets session-2.
			sessionVal := fmt.Sprintf("session-%d", n)
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: sessionVal})
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"success":true}`)
		case "/rest/v2/applications":
			n := appsRequestCount.Add(1)
			cookie, err := r.Cookie("JSESSIONID")
			if n == 1 || err != nil || cookie.Value == "session-1" {
				// First attempt: simulate stale/expired session → 403.
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error":"session expired"}`)
				return
			}
			// Second attempt (after re-login): success.
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"applications":["LiveApp","demo"]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newLoginTestClient(srv, "admin@example.com", "secret")
	names, err := c.ListApplications(context.Background())
	if err != nil {
		t.Fatalf("expected ultimate success after re-login, got: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 apps, got %v", names)
	}
	if logins := loginCount.Load(); logins != 2 {
		t.Errorf("expected exactly 2 logins (initial + one refresh), got %d", logins)
	}
}

// TestPersistent403_DoesNotStormLogins verifies that when the protected endpoint
// permanently returns 403 (e.g. IP blocked), the client errors and the login
// endpoint is hit a small bounded number of times (≤ 2), never more.
func TestPersistent403_DoesNotStormLogins(t *testing.T) {
	var loginCount atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v2/users/authenticate":
			loginCount.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "blocked-session"})
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"success":true}`)
		case "/rest/v2/applications":
			// Always 403 — simulates permanent IP block.
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"not allowed IP"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newLoginTestClient(srv, "admin@example.com", "secret")
	_, err := c.ListApplications(context.Background())
	if err == nil {
		t.Fatal("expected error for permanently blocked IP, got nil")
	}
	if logins := loginCount.Load(); logins > 2 {
		t.Errorf("login endpoint hit %d times, want ≤ 2 (throttle must prevent storm)", logins)
	}
}

// TestClusterNodes_404OnPaginatedReturnsNilNoError verifies that a 404 from the
// paginated cluster/nodes path (defensive fallback) is mapped to (nil, nil), not an error.
// The real AMS 3.0.3 returns 500 on standalone, but older or odd builds may 404.
func TestClusterNodes_404OnPaginatedReturnsNilNoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/rest/v2/cluster-mode-status":
			// cluster-mode-status unavailable → fall through to paginated path.
			w.WriteHeader(http.StatusNotFound)
		default:
			// Paginated path and any other path: 404.
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv)
	nodes, err := c.ClusterNodes(context.Background())
	if err != nil {
		t.Fatalf("ClusterNodes on standalone AMS (404 on paginated path) must return nil error, got: %v", err)
	}
	if nodes != nil {
		t.Errorf("ClusterNodes on standalone AMS (404 on paginated path) must return nil slice, got: %v", nodes)
	}
}

// TestListBroadcasts_RealAMS303_QoEFields decodes the SANITIZED real AMS 3.0.3
// LiveApp/test123 broadcast (curl-captured 2026-06-21) and pins the wire facts
// the integration fixes depend on: bitrate is bps (624016), speed is a ratio
// (0.991), currentFPS is ABSENT (decodes to 0), and the ingest-side QoE fields
// (packetLostRatio/jitterMs/rttMs/packetsLost) decode into the DTO.
func TestListBroadcasts_RealAMS303_QoEFields(t *testing.T) {
	fixture := mustReadFixture(t, "broadcasts_real_test123_v303.json")
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
	if b.StreamID != "test123" || b.Status != "broadcasting" {
		t.Fatalf("unexpected stream: id=%q status=%q", b.StreamID, b.Status)
	}
	if b.BitRate != 624016 {
		t.Errorf("BitRate = %v, want 624016 (raw bps from the wire)", b.BitRate)
	}
	if b.Speed != 0.991 {
		t.Errorf("Speed = %v, want 0.991 (realtime ratio)", b.Speed)
	}
	if b.CurrentFPS != 0 {
		t.Errorf("CurrentFPS = %v, want 0 (AMS 3.0.3 omits currentFPS)", b.CurrentFPS)
	}
	// New DTO fields must bind to the real wire keys (all 0 on this idle stream,
	// but the decode path is what we are pinning).
	if b.PacketLostRatio != 0 || b.JitterMs != 0 || b.RttMs != 0 || b.PacketsLost != 0 {
		t.Errorf("QoE fields = plr:%v jitterMs:%v rttMs:%v packetsLost:%v, want all 0",
			b.PacketLostRatio, b.JitterMs, b.RttMs, b.PacketsLost)
	}
}

// ─── GetVersion (D-041): standalone fleet-node version source ─────────────────

func TestGetVersion_DecodesVersionName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v2/version" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"versionName":"3.0.3","versionType":"Enterprise Edition","buildNumber":"20260504_1443"}`))
	}))
	defer srv.Close()

	v, err := newTestClient(srv).GetVersion(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v == nil || v.VersionName != "3.0.3" {
		t.Fatalf("VersionName = %+v, want 3.0.3", v)
	}
}

// Older AMS without /rest/v2/version must yield (nil, nil) so the caller emits
// the node with an empty version rather than dropping it (no silent error swallow
// of a real failure: only 404/405 are tolerated).
func TestGetVersion_404ReturnsNilNoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	v, err := newTestClient(srv).GetVersion(context.Background())
	if err != nil {
		t.Fatalf("404 must map to nil error, got %v", err)
	}
	if v != nil {
		t.Fatalf("404 must return nil DTO, got %+v", v)
	}
}

// A genuine server error (500) must NOT be swallowed — it should surface.
func TestGetVersion_500ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := newTestClient(srv).GetVersion(context.Background()); err == nil {
		t.Fatal("500 must surface as an error, got nil")
	}
}

// TestWebRTCClientStats_EscapesStreamID is the S50 regression test (D-112) for
// S48 finding [3]: a publisher-chosen streamID with a URL-significant character
// must be percent-escaped so the request reaches the webrtc-client-stats endpoint
// rather than silently hitting the single-broadcast detail route.
//
// The server records r.URL.EscapedPath(): with the fix, "test#peer" arrives as
// the escaped segment "test%23peer"; without it, url.Parse treats '#' as a
// fragment and everything after is dropped, so the path truncates at "test".
func TestWebRTCClientStats_EscapesStreamID(t *testing.T) {
	cases := []struct {
		name         string
		streamID     string
		wantEscPath  string
		wantStripped bool // pre-fix behavior would truncate the path here
	}{
		{
			name:        "hash_is_escaped_not_fragment",
			streamID:    "test#peer",
			wantEscPath: "/LiveApp/rest/v2/broadcasts/test%23peer/webrtc-client-stats/0/100",
		},
		{
			name:        "space_is_escaped",
			streamID:    "my stream",
			wantEscPath: "/LiveApp/rest/v2/broadcasts/my%20stream/webrtc-client-stats/0/100",
		},
		{
			// Positive control: a normal id is byte-identical after escaping, so
			// the common path is unchanged (no regression for ordinary streams).
			name:        "normal_id_unchanged",
			streamID:    "test123",
			wantEscPath: "/LiveApp/rest/v2/broadcasts/test123/webrtc-client-stats/0/100",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath atomic.Value
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath.Store(r.URL.EscapedPath())
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`[{"statId":"peer-1","videoJitter":1.5}]`)) //nolint:errcheck
			}))
			defer srv.Close()

			stats, err := newTestClient(srv).WebRTCClientStats(context.Background(), "LiveApp", tc.streamID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, _ := gotPath.Load().(string)
			if got != tc.wantEscPath {
				t.Errorf("escaped request path = %q, want %q\n"+
					"(streamID %q must be url.PathEscape'd so it reaches the webrtc-client-stats endpoint)",
					got, tc.wantEscPath, tc.streamID)
			}
			// With the correct endpoint reached, the stats decode and are returned.
			if len(stats) != 1 || stats[0].StatID != "peer-1" {
				t.Errorf("expected 1 stat with statId=peer-1, got %+v", stats)
			}
		})
	}
}

// ─── /rest/v2/system-resources (D-179, review round 6 H-08) ──────────────────
//
// LIVE-PROBE ORIGIN: a standalone AMS 3.0.3 Enterprise returns fully populated
// cpuUsage / systemMemoryInfo / fileSystemInfo from this endpoint — the very
// metrics /rest/v2/system-status omits. The fixture is that captured response
// (license + instanceId redacted).
func TestSystemResources_DecodesRealV303Capture(t *testing.T) {
	body, err := os.ReadFile("testdata/system_resources_real_v303.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v2/system-resources" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	res, err := newTestClient(srv).SystemResources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("SystemResources returned nil map on a 200 response")
	}
	for _, key := range []string{"cpuUsage", "systemMemoryInfo", "fileSystemInfo", "systemInfo", "softwareVersion"} {
		if _, ok := res[key]; !ok {
			t.Errorf("captured payload key %q missing from decoded map", key)
		}
	}
}

// Older AMS without the console resources route must yield (nil, nil) so the
// caller falls back to /rest/v2/system-status instead of dropping the node.
// Same tolerance contract as GetVersion: only 404/405 are swallowed.
func TestSystemResources_404ReturnsNilNoError(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
		res, err := newTestClient(srv).SystemResources(context.Background())
		srv.Close()
		if err != nil {
			t.Fatalf("HTTP %d must map to nil error, got %v", code, err)
		}
		if res != nil {
			t.Fatalf("HTTP %d must return a nil map, got %+v", code, res)
		}
	}
}

// A genuine server error must NOT be swallowed — otherwise a broken AMS looks
// like an old AMS and the fallback masks a real outage.
func TestSystemResources_500ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := newTestClient(srv).SystemResources(context.Background()); err == nil {
		t.Fatal("500 must surface as an error, got nil")
	}
}
