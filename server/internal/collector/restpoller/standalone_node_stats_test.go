// standalone_node_stats_test.go — TDD test for standalone AMS node stats polling.
//
// Covers item B: when a standalone AMS (no cluster) returns 404 on
// /rest/v2/cluster/nodes, the poller must fall back to SystemStats + GetVersion
// and emit a domain.EventNodeStats event with the REAL AMS 3.x identity fields
// (os_name, os_arch, java_version, processor_count, version).
//
// AMS 3.x GET /rest/v2/system-status returns ONLY {osName, osArch, javaVersion,
// processorCount} — no cpu/mem/disk metrics. This test asserts the honest shape
// (processor_count==8, os_name=="Linux", java_version=="17", version=="3.0.3")
// and explicitly verifies that cpu_pct is ABSENT (not fabricated as zero).
package restpoller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/collector/restpoller"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// loadFixture reads a JSON file from pkg/amsclient/testdata relative to the repo root.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	candidates := []string{
		"../../../pkg/amsclient/testdata/" + name,
		"../../pkg/amsclient/testdata/" + name,
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err == nil {
			return data
		}
	}
	t.Fatalf("could not find fixture %s; checked: %v", name, candidates)
	return nil
}

// TestStandaloneNodeStats_PollEmitsNodeStatsEvent verifies that when a standalone
// AMS node returns 404 on /rest/v2/cluster/nodes, Poller.poll() calls SystemStats
// and GetVersion, then emits a domain.EventNodeStats with:
//   - processor_count == 8
//   - os_name == "Linux"
//   - java_version == "17"
//   - version == "3.0.3" (from /rest/v2/version)
//   - cpu_pct ABSENT (honest — AMS 3.x system-status has no cpu metrics)
func TestStandaloneNodeStats_PollEmitsNodeStatsEvent(t *testing.T) {
	systemStatusBody := loadFixture(t, "system_status.json")
	versionBody := loadFixture(t, "version.json")

	mockAMS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v2/applications":
			json.NewEncoder(w).Encode(map[string]any{
				"applications": []map[string]any{},
			})
		case "/rest/v2/cluster/nodes":
			// Standalone AMS: 404 tells the client there is no cluster.
			w.WriteHeader(http.StatusNotFound)
		case "/rest/v2/system-status":
			w.Header().Set("Content-Type", "application/json")
			w.Write(systemStatusBody)
		case "/rest/v2/version":
			w.Header().Set("Content-Type", "application/json")
			w.Write(versionBody)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockAMS.Close()

	client := amsclient.New(amsclient.Config{
		BaseURL: mockAMS.URL,
		Timeout: 3 * time.Second,
	})
	sink := newMockSink()
	poller := restpoller.New(
		restpoller.Config{
			NodeID:       "standalone-node",
			PollInterval: 100 * time.Millisecond,
		},
		client,
		sink,
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = poller.Run(ctx)
	}()

	// Wait for node_stats event (up to 3 s).
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()

	for {
		select {
		case <-sink.notify:
			ev := findNodeStats(sink)
			if ev == nil {
				continue
			}

			t.Logf("PASS: got node_stats event: Data=%v", ev.Data)

			// Assert real identity fields.
			if got, ok := ev.Data["os_name"].(string); !ok || got != "Linux" {
				t.Errorf("os_name = %v, want %q", ev.Data["os_name"], "Linux")
			}
			if got, ok := ev.Data["java_version"].(string); !ok || got != "17" {
				t.Errorf("java_version = %v, want %q", ev.Data["java_version"], "17")
			}
			if got, ok := ev.Data["processor_count"].(int); !ok || got != 8 {
				t.Errorf("processor_count = %v (%T), want 8", ev.Data["processor_count"], ev.Data["processor_count"])
			}
			if got, ok := ev.Data["version"].(string); !ok || got != "3.0.3" {
				t.Errorf("version = %v, want %q (from /rest/v2/version)", ev.Data["version"], "3.0.3")
			}

			// CRITICAL: cpu_pct must be absent (honest — no fabricated metric).
			if v, exists := ev.Data["cpu_pct"]; exists {
				t.Errorf("HONEST FAIL: cpu_pct must be absent for standalone AMS 3.x; got %v", v)
			}

			if ev.NodeID != "standalone-node" {
				t.Errorf("node_stats NodeID = %q, want %q", ev.NodeID, "standalone-node")
			}
			cancel()
			return

		case <-deadline.C:
			t.Errorf("FAIL: no domain.EventNodeStats received within 3s after standalone AMS SystemStats poll")
			cancel()
			return
		case <-ctx.Done():
			return
		}
	}
}

// findNodeStats scans the sink for a node_stats event.
func findNodeStats(sink *mockEventSink) *domain.ServerEvent {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	for i := range sink.events {
		if sink.events[i].Type == domain.EventNodeStats {
			return &sink.events[i]
		}
	}
	return nil
}
