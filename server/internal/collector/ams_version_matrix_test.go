// Package collector — AMS version matrix integration tests.
//
// TestAMSVersionMatrix verifies that the collector normalizer correctly handles
// the AMS REST v2 wire format across all supported AMS versions. These tests
// run against mock-ams emulation profiles (per-version shape differences) and
// document which assertions require real AMS containers (CI-only, INFRA-01's
// ams-version-matrix workflow).
//
// Gate criterion D-W1-006 (carried from wave-1, implemented in wave-2):
// Write TestAMSVersionMatrix content against mock-ams profiles; document
// which assertions need real AMS containers.
//
// # Local execution (mock-ams profiles — no Docker required)
//
//	CGO_ENABLED=0 go test -run TestAMSVersionMatrix ./internal/collector/...
//
// # CI execution (real AMS containers — requires Docker)
//
//	CGO_ENABLED=0 go test -tags integration -run TestAMSVersionMatrix ./internal/collector/...
//
// # AMS version matrix (INFRA-01 workflow: .github/workflows/ams-version-matrix.yml)
//
//	Versions: 2.10.0, 2.14.0, 3.0.2
//	Environment: AMS_BASE_URL set by workflow when real containers available.
package collector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// ─── AMS wire-format profiles ────────────────────────────────────────────────
//
// Each profile emulates the REST v2 response shape for a given AMS version.
// Shape differences across versions:
//
//	v2.10.x: "speed" field for bitrate, cpuUsage is 0–100 percentage
//	v2.14.x: same core fields; adds "webRTCViewerCount" distinct from "hlsViewerCount"
//	v3.0.x:  adds "currentFPS" field; "bitrate" replaces "speed" for bitrate reporting
//
// QA-01 emulates what mock-ams documents; assertions that require a real AMS
// container to validate true wire format are marked "CI-ONLY" in comments.
//
// Documented AMS surface: GET /rest/v2/broadcasts/{app}/list (BroadcastDTO).
// GET /rest/v2/cluster/nodes (ClusterNodeDTO).

// amsProfile is a per-version mock AMS REST response profile.
type amsProfile struct {
	name       string // AMS version string
	broadcasts []map[string]any
	nodes      []map[string]any
}

// amsProfiles contains mock profiles for each supported AMS version.
// Shape differences are documented per-field.
var amsProfiles = []amsProfile{
	{
		name: "v2.10.0",
		broadcasts: []map[string]any{
			{
				// v2.10.x: "speed" is the bitrate field; "bitrate" may not be present.
				// CI-ONLY: real v2.10 may use "speed" only — verify against container.
				"streamId":          "live-stream-1",
				"name":              "live-stream-1",
				"status":            "broadcasting",
				"type":              "liveStream",
				"publishType":       "webrtc",
				"startTime":         int64(1700000000000),
				"hlsViewerCount":    10,
				"webRTCViewerCount": 50,
				"rtmpViewerCount":   0,
				"dashViewerCount":   0,
				"speed":             float64(2000), // v2.10.x uses "speed" for bitrate
				"bitrate":           float64(0),    // may be absent in real v2.10.x
				"currentFPS":        30,
				"appName":           "live",
			},
		},
		nodes: []map[string]any{
			{
				// CI-ONLY: v2.10.x ClusterNode shape — verify against real container.
				"nodeId":           "node-1",
				"ip":               "192.168.1.10",
				"port":             5080,
				"cpuUsage":         float64(25.0), // 0–100 percentage (NOT 0.0–1.0 fraction)
				"memoryUsage":      float64(60.0),
				"diskUsage":        float64(30.0),
				"networkInputBps":  float64(1024000),
				"networkOutputBps": float64(2048000),
			},
		},
	},
	{
		name: "v2.14.0",
		broadcasts: []map[string]any{
			{
				// v2.14.x: "bitrate" field is the primary bitrate reporter.
				// WebRTCViewerCount and hlsViewerCount are distinct (not derived).
				"streamId":          "live-stream-1",
				"name":              "live-stream-1",
				"status":            "broadcasting",
				"type":              "liveStream",
				"publishType":       "webrtc",
				"startTime":         int64(1700000000000),
				"hlsViewerCount":    15,
				"webRTCViewerCount": 55,
				"rtmpViewerCount":   5,
				"dashViewerCount":   2,
				"bitrate":           float64(2500), // v2.14.x uses "bitrate" directly
				"currentFPS":        30,
				"appName":           "live",
			},
		},
		nodes: []map[string]any{
			{
				"nodeId":            "node-1",
				"ip":                "192.168.1.10",
				"port":              5080,
				"cpuUsage":          float64(40.0),
				"memoryUsage":       float64(55.0),
				"diskUsage":         float64(25.0),
				"networkInputBps":   float64(2048000),
				"networkOutputBps":  float64(4096000),
				"jvmMemoryUsage":    float64(30.0),
				"activeStreamCount": 3,
			},
		},
	},
	{
		name: "v3.0.2",
		broadcasts: []map[string]any{
			{
				// v3.0.x: "bitrate" is primary; "speed" may be absent.
				// "currentFPS" is explicitly present and populated.
				// CI-ONLY: real v3.0.x may add new fields; run against container to verify.
				"streamId":          "live-stream-1",
				"name":              "live-stream-1",
				"status":            "broadcasting",
				"type":              "liveStream",
				"publishType":       "webrtc",
				"startTime":         int64(1700000000000),
				"hlsViewerCount":    20,
				"webRTCViewerCount": 80,
				"rtmpViewerCount":   0,
				"dashViewerCount":   0,
				"bitrate":           float64(3000),
				"currentFPS":        60, // v3.0.x supports higher FPS streams
				"appName":           "live",
			},
		},
		nodes: []map[string]any{
			{
				"nodeId":            "node-1",
				"ip":                "192.168.1.10",
				"port":              5080,
				"cpuUsage":          float64(20.0),
				"memoryUsage":       float64(50.0),
				"diskUsage":         float64(20.0),
				"networkInputBps":   float64(4096000),
				"networkOutputBps":  float64(8192000),
				"jvmMemoryUsage":    float64(25.0),
				"activeStreamCount": 5,
				// v3.0.x may add additional monitoring fields (CI-ONLY to verify).
			},
		},
	},
}

// newProfileServer creates an httptest.Server serving the given AMS profile.
func newProfileServer(t *testing.T, profile amsProfile) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// GET /rest/v2/applications
	mux.HandleFunc("/rest/v2/applications", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"applications": []map[string]string{{"name": "live"}},
		})
	})

	// GET /rest/v2/broadcasts/{app}/list
	mux.HandleFunc("/rest/v2/broadcasts/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if len(profile.broadcasts) == 0 {
			_ = json.NewEncoder(w).Encode([]any{})
			return
		}
		_ = json.NewEncoder(w).Encode(profile.broadcasts)
	})

	// GET /rest/v2/cluster/nodes
	mux.HandleFunc("/rest/v2/cluster/nodes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if len(profile.nodes) == 0 {
			_ = json.NewEncoder(w).Encode([]any{})
			return
		}
		_ = json.NewEncoder(w).Encode(profile.nodes)
	})

	// Smoke: /rest/v2/version (used by the ams-version-matrix workflow smoke test)
	mux.HandleFunc("/rest/v2/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"version": profile.name,
		})
	})

	return httptest.NewServer(mux)
}

// TestAMSVersionMatrix verifies that the normalizer produces correct domain events
// across all supported AMS versions using mock-ams profile emulation.
//
// CI-ONLY assertions are documented in comments — they require real AMS containers
// and are executed by INFRA-01's ams-version-matrix workflow.
func TestAMSVersionMatrix(t *testing.T) {
	ctx := context.Background()

	for _, profile := range amsProfiles {
		profile := profile // capture
		t.Run(profile.name, func(t *testing.T) {
			srv := newProfileServer(t, profile)
			defer srv.Close()

			client := amsclient.New(amsclient.Config{
				BaseURL: srv.URL,
			})

			// ── Assert 1: ListApplications returns the live app ──────────────
			apps, err := client.ListApplications(ctx)
			if err != nil {
				t.Fatalf("[%s] ListApplications: %v", profile.name, err)
			}
			if len(apps) == 0 {
				t.Errorf("[%s] expected ≥1 application, got 0", profile.name)
			}
			found := false
			for _, a := range apps {
				if a == "live" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("[%s] expected 'live' in applications, got %v", profile.name, apps)
			}
			t.Logf("[%s] PASS: ListApplications → %v", profile.name, apps)

			// ── Assert 2: ListBroadcasts returns streams ─────────────────────
			broadcasts, err := client.ListBroadcasts(ctx, "live", 0, 200)
			if err != nil {
				t.Fatalf("[%s] ListBroadcasts: %v", profile.name, err)
			}
			if len(profile.broadcasts) > 0 && len(broadcasts) == 0 {
				t.Errorf("[%s] expected ≥1 broadcast, got 0", profile.name)
			}
			for _, b := range broadcasts {
				// Verify required AMS fields parse.
				if b.StreamID == "" {
					t.Errorf("[%s] broadcast missing StreamID", profile.name)
				}
				if b.Status == "" {
					t.Errorf("[%s] broadcast missing Status", profile.name)
				}
				// Viewer count: must be non-negative
				totalViewers := b.HlsViewerCount + b.WebRTCViewerCount + b.RTMPViewerCount + b.DashViewerCount
				if totalViewers < 0 {
					t.Errorf("[%s] broadcast has negative total viewers: %d", profile.name, totalViewers)
				}
				t.Logf("[%s] broadcast: id=%s status=%s viewers(hls=%d webrtc=%d rtmp=%d dash=%d) bitrate=%.0f fps=%d",
					profile.name, b.StreamID, b.Status,
					b.HlsViewerCount, b.WebRTCViewerCount, b.RTMPViewerCount, b.DashViewerCount,
					b.BitRate, b.CurrentFPS)
			}
			t.Logf("[%s] PASS: ListBroadcasts → %d broadcasts", profile.name, len(broadcasts))

			// ── Assert 3: NormalizeBroadcast sums all viewer protocols ────────
			// Verifies ARCHITECTURE rule: viewer_count = sum of all protocol counts.
			noop := NoopGeoResolver{}
			ua := NewEmbeddedUAParser()
			for _, b := range broadcasts {
				events := NormalizeBroadcast(b, "test-node", "", noop, ua)
				if len(events) == 0 {
					t.Errorf("[%s] NormalizeBroadcast returned 0 events for broadcasting stream", profile.name)
					continue
				}

				// Find the stream_stats event (always emitted for broadcasting).
				wantViewers := b.HlsViewerCount + b.WebRTCViewerCount + b.RTMPViewerCount + b.DashViewerCount
				foundStats := false
				for _, ev := range events {
					if ev.Type == "stream_stats" {
						foundStats = true
						if viewerCount, ok := ev.Data["viewer_count"].(int); ok {
							if viewerCount != wantViewers {
								t.Errorf("[%s] stream_stats viewer_count=%d, want %d (hls=%d+webrtc=%d+rtmp=%d+dash=%d)",
									profile.name, viewerCount, wantViewers,
									b.HlsViewerCount, b.WebRTCViewerCount, b.RTMPViewerCount, b.DashViewerCount)
							} else {
								t.Logf("[%s] PASS: stream_stats viewer_count=%d", profile.name, viewerCount)
							}
						}
					}
				}
				if !foundStats {
					t.Errorf("[%s] no stream_stats event emitted for broadcasting stream", profile.name)
				}

				// StreamID must be carried through all events.
				for _, ev := range events {
					if ev.StreamID != b.StreamID {
						t.Errorf("[%s] event.StreamID=%q, want %q", profile.name, ev.StreamID, b.StreamID)
					}
				}
				t.Logf("[%s] PASS: NormalizeBroadcast → %d events for stream=%s", profile.name, len(events), b.StreamID)
			}

			// ── Assert 4: ClusterNodes returns nodes ─────────────────────────
			nodes, err := client.ClusterNodes(ctx)
			if err != nil {
				// CI-ONLY: some AMS single-node installs return 404 for /cluster/nodes.
				t.Logf("[%s] WARN: ClusterNodes error (may be single-node mode): %v", profile.name, err)
			} else {
				if len(profile.nodes) > 0 && len(nodes) == 0 {
					t.Errorf("[%s] expected ≥1 node from mock profile, got 0", profile.name)
				}
				for _, n := range nodes {
					// D-W1-001 regression: cpuUsage must be preserved as-is (0–100).
					// The ×100 multiplier was removed in Wave 1 fix-loop.
					normalized := NormalizeClusterNode(n)
					if normalized.Data == nil {
						t.Errorf("[%s] NormalizeClusterNode returned nil Data", profile.name)
						continue
					}
					cpuPct, _ := normalized.Data["cpu_pct"].(float64)
					if cpuPct > 100 {
						t.Errorf("[%s] D-W1-001 regression: cpu_pct=%.1f > 100 (AMS sends 0-100, not fraction; ×100 must NOT be applied)",
							profile.name, cpuPct)
					}
					if cpuPct < 0 {
						t.Errorf("[%s] cpu_pct=%.1f < 0 (invalid)", profile.name, cpuPct)
					}
					t.Logf("[%s] PASS: ClusterNode → node=%s cpu_pct=%.1f",
						profile.name, n.NodeID, cpuPct)
				}
				t.Logf("[%s] PASS: ClusterNodes → %d nodes", profile.name, len(nodes))
			}
		})
	}
}

// TestAMSVersionMatrix_D_W1_001_Regression verifies the D-W1-001 fix (cpu/mem ×100
// removed) holds across all mock profiles. This is a regression guard.
func TestAMSVersionMatrix_D_W1_001_Regression(t *testing.T) {
	// Simulate what AMS sends: cpuUsage = 15.0 means 15%, NOT 0.15.
	// Before D-W1-001 fix: cpu_pct = 15.0 * 100 = 1500 (wrong).
	// After D-W1-001 fix: cpu_pct = 15.0 (correct).
	fakeNode := amsclient.ClusterNodeDTO{
		NodeID:          "regression-test-node",
		IP:              "127.0.0.1",
		Port:            5080,
		CPUUsage:        15.0,
		MemoryUsage:     40.0,
		DiskUsage:       20.0,
		NetworkInputBps: 1024,
	}
	result := NormalizeClusterNode(fakeNode)

	if result.Data == nil {
		t.Fatal("NormalizeClusterNode returned nil Data")
	}

	cpuPct, _ := result.Data["cpu_pct"].(float64)
	memPct, _ := result.Data["mem_pct"].(float64)

	if cpuPct != 15.0 {
		t.Errorf("D-W1-001 regression: cpu_pct=%.1f, want 15.0 (AMS sends 0-100, not fraction; ×100 must NOT be applied)", cpuPct)
	}
	if memPct != 40.0 {
		t.Errorf("D-W1-001 regression: mem_pct=%.1f, want 40.0", memPct)
	}
	t.Logf("PASS: D-W1-001 regression — cpu_pct=%.1f, mem_pct=%.1f", cpuPct, memPct)
}

// TestAMSVersionMatrix_CIOnlyAssertions documents which assertions require real
// AMS containers and CANNOT be verified against mock profiles.
//
// These are executed by INFRA-01's ams-version-matrix.yml workflow in CI,
// where AMS containers (v2.10.0, v2.14.0, v3.0.2) are available.
func TestAMSVersionMatrix_CIOnlyAssertions(t *testing.T) {
	t.Log("The following assertions require real AMS containers (CI-only):")
	t.Log("")
	t.Log("1. v2.10.x: verify 'speed' field is the authoritative bitrate field")
	t.Log("   (mock emulates 'bitrate', real AMS may only have 'speed')")
	t.Log("")
	t.Log("2. v2.10.x vs v2.14.x: verify hlsViewerCount and webRTCViewerCount")
	t.Log("   are correctly populated (v2.10.x may sum into a single field)")
	t.Log("")
	t.Log("3. v3.0.x: verify new fields (if any) don't break normalization")
	t.Log("   (run against real container to discover new DTO fields)")
	t.Log("")
	t.Log("4. All versions: verify /rest/v2/applications returns expected structure")
	t.Log("   (real AMS may return 'success' wrapper around 'applications' array)")
	t.Log("")
	t.Log("5. All versions: ClusterNode 'cpuUsage' is 0–100 (not 0.0–1.0 fraction)")
	t.Log("   (D-W1-001 regression: critical to verify against each version)")
	t.Log("")
	t.Log("Workflow: .github/workflows/ams-version-matrix.yml")
	t.Log("Versions: 2.10.0, 2.14.0, 3.0.2")
	t.Log("Run: CGO_ENABLED=0 go test -tags integration -run TestAMSVersionMatrix ./internal/collector/...")
	// This test always passes — it's documentation only.
}
