// system_resources_test.go — D-179 / external review round 6, H-08.
//
// LIM-01 said the Fleet CPU/memory/disk gauges are blank on a standalone AMS and
// need Kafka. That was true of /rest/v2/system-status, which returns identity
// only — but a live probe of AMS 3.0.3 Enterprise (2026-07-27) showed
// /rest/v2/system-resources returns all three, fully populated, on the same
// standalone node.
//
// These tests pin the resulting poller contract:
//   - system-resources available  → cpu_pct / mem_pct / disk_pct are emitted
//   - system-resources 404 (older AMS) → silent fallback to system-status,
//     identity still emitted, gauges still honestly ABSENT
//   - both routes down → the D-087 failure-streak event, not a phantom success
package restpoller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/collector/restpoller"
	"github.com/aytekXR/ams-pulse/server/internal/domain"
	"github.com/aytekXR/ams-pulse/server/pkg/amsclient"
)

// runStandalonePoller wires a poller against a standalone-AMS mock and returns
// the first node_stats event it emits.
func runStandalonePoller(t *testing.T, handler http.HandlerFunc) *domain.ServerEvent {
	t.Helper()
	mockAMS := httptest.NewServer(handler)
	defer mockAMS.Close()

	client := amsclient.New(amsclient.Config{BaseURL: mockAMS.URL, Timeout: 3 * time.Second})
	sink := newMockSink()
	poller := restpoller.New(
		restpoller.Config{NodeID: "standalone-node", PollInterval: 100 * time.Millisecond},
		client, sink, nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { _ = poller.Run(ctx) }()

	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-sink.notify:
			if ev := findNodeStats(sink); ev != nil {
				return ev
			}
		case <-deadline.C:
			t.Fatal("no node_stats event within 3s")
			return nil
		case <-ctx.Done():
			t.Fatal("context cancelled before a node_stats event")
			return nil
		}
	}
}

// standaloneRoutes serves the routes every standalone-path test needs; extra
// returns the handler for the system endpoints under test.
func standaloneRoutes(extra func(w http.ResponseWriter, r *http.Request) bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v2/applications":
			json.NewEncoder(w).Encode(map[string]any{"applications": []map[string]any{}})
			return
		case "/rest/v2/cluster-mode-status":
			json.NewEncoder(w).Encode(map[string]any{"success": false})
			return
		}
		if extra(w, r) {
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

// The headline H-08 assertion: a standalone AMS that serves
// /rest/v2/system-resources produces populated gauges, and the poller does NOT
// need /rest/v2/system-status or /rest/v2/version to do it.
func TestStandalone_SystemResources_EmitsCPUMemDisk(t *testing.T) {
	body := loadFixture(t, "system_resources_real_v303.json")
	var statusHits, versionHits int32

	ev := runStandalonePoller(t, standaloneRoutes(func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/rest/v2/system-resources":
			w.Write(body)
			return true
		case "/rest/v2/system-status":
			atomic.AddInt32(&statusHits, 1)
			return false
		case "/rest/v2/version":
			atomic.AddInt32(&versionHits, 1)
			return false
		}
		return false
	}))

	for _, key := range []string{"cpu_pct", "mem_pct", "disk_pct"} {
		v, ok := ev.Data[key].(float64)
		if !ok {
			t.Errorf("%s missing — LIM-01 is not closed (Data=%v)", key, ev.Data)
			continue
		}
		if v <= 0 || v > 100 {
			t.Errorf("%s = %v, want a plausible percentage in (0,100]", key, v)
		}
	}
	// cpu_pct is the captured value, not a rescaled one (the 3400% trap).
	if got := ev.Data["cpu_pct"]; got != float64(34) {
		t.Errorf("cpu_pct = %v, want 34 exactly (systemCPULoad is already a percent)", got)
	}
	// Identity and version still arrive, from the same single call.
	if got := ev.Data["os_name"]; got != "Linux" {
		t.Errorf("os_name = %v, want Linux", got)
	}
	if got := ev.Data["version"]; got != "3.0.3" {
		t.Errorf("version = %v, want 3.0.3 (from softwareVersion, no /rest/v2/version call)", got)
	}
	if ev.NodeID != "standalone-node" {
		t.Errorf("NodeID = %q, want standalone-node", ev.NodeID)
	}
	// api_latency_ms must still be stamped on this path (D-087).
	if _, ok := ev.Data["api_latency_ms"].(float64); !ok {
		t.Errorf("api_latency_ms missing on the system-resources path: %v", ev.Data)
	}
	if n := atomic.LoadInt32(&statusHits); n > 0 {
		t.Errorf("system-status was called %d times; system-resources should have sufficed", n)
	}
	if n := atomic.LoadInt32(&versionHits); n > 0 {
		t.Errorf("/rest/v2/version was called %d times; softwareVersion should have sufficed", n)
	}
}

// Older AMS: the endpoint 404s. The poller must fall back silently, keep the
// identity fields, and keep the gauges ABSENT rather than fabricating zeros.
// This is the pre-D-179 behaviour, now pinned as the fallback contract.
func TestStandalone_SystemResources404_FallsBackToSystemStatus(t *testing.T) {
	statusBody := loadFixture(t, "system_status.json")
	versionBody := loadFixture(t, "version.json")

	ev := runStandalonePoller(t, standaloneRoutes(func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/rest/v2/system-status":
			w.Write(statusBody)
			return true
		case "/rest/v2/version":
			w.Write(versionBody)
			return true
		}
		return false // system-resources → 404
	}))

	if got := ev.Data["os_name"]; got != "Linux" {
		t.Errorf("os_name = %v, want Linux via the fallback path", got)
	}
	if got := ev.Data["version"]; got != "3.0.3" {
		t.Errorf("version = %v, want 3.0.3 from /rest/v2/version", got)
	}
	for _, key := range []string{"cpu_pct", "mem_pct", "disk_pct"} {
		if v, exists := ev.Data[key]; exists {
			t.Errorf("HONEST FAIL: %s must be absent on the system-status fallback, got %v", key, v)
		}
	}
}

// A stripped or proxied system-resources response that carries no gauges must
// not cost us the identity fields: HasResourceMetrics is false, so the poller
// falls back rather than emitting a metric-less event from the new route.
func TestStandalone_SystemResourcesWithoutMetrics_FallsBack(t *testing.T) {
	statusBody := loadFixture(t, "system_status.json")
	versionBody := loadFixture(t, "version.json")

	ev := runStandalonePoller(t, standaloneRoutes(func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/rest/v2/system-resources":
			// Identity only — no cpuUsage / systemMemoryInfo / fileSystemInfo.
			json.NewEncoder(w).Encode(map[string]any{
				"systemInfo": map[string]any{"osName": "Linux"},
			})
			return true
		case "/rest/v2/system-status":
			w.Write(statusBody)
			return true
		case "/rest/v2/version":
			w.Write(versionBody)
			return true
		}
		return false
	}))

	// processor_count comes only from the fallback fixture (8), proving we took it.
	if got, ok := ev.Data["processor_count"].(int); !ok || got != 8 {
		t.Errorf("processor_count = %v, want 8 from the system-status fallback", ev.Data["processor_count"])
	}
	if got := ev.Data["version"]; got != "3.0.3" {
		t.Errorf("version = %v, want 3.0.3 from the fallback /rest/v2/version", got)
	}
}

// Both system routes failing is a real outage: the poller must emit the D-087
// failure-streak event (api_unreachable) rather than a success with no metrics.
func TestStandalone_BothSystemRoutesFail_EmitsFailureStreak(t *testing.T) {
	ev := runStandalonePoller(t, standaloneRoutes(func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/rest/v2/system-resources", "/rest/v2/system-status":
			w.WriteHeader(http.StatusInternalServerError)
			return true
		}
		return false
	}))

	if unreachable, _ := ev.Data["api_unreachable"].(bool); !unreachable {
		t.Errorf("want api_unreachable=true when both system routes 500, got Data=%v", ev.Data)
	}
	if _, ok := ev.Data["api_latency_ms"]; ok {
		t.Errorf("api_latency_ms must be ABSENT on a failure event (D-075 semantics), got %v", ev.Data["api_latency_ms"])
	}
}
