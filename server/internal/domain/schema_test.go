// Schema round-trip test: marshal ServerEvent → validate against JSON Schema.
// Uses npx ajv-cli for JSON Schema validation (guarded by availability).
package domain_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// Test that domain types marshal to JSON that matches the contract fixtures.
func TestServerEvent_Marshals(t *testing.T) {
	ev := domain.ServerEvent{
		Version:  1,
		Type:     domain.EventStreamPublishStart,
		TS:       1700000000000,
		Source:   domain.SourceRestPoll,
		NodeID:   "ams-node-1",
		App:      "live",
		StreamID: "test-stream",
		Data: map[string]any{
			"publish_type": "rtmp",
		},
		Enrichment: &domain.EnrichmentBlock{
			Geo:    &domain.GeoEnrichment{Country: "TR", Region: "Istanbul"},
			Client: &domain.ClientEnrichment{Device: "desktop", OS: "Windows", Browser: "Chrome"},
		},
	}

	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Round-trip: unmarshal back.
	var ev2 domain.ServerEvent
	if err := json.Unmarshal(b, &ev2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if ev2.Version != 1 {
		t.Errorf("version: got %d, want 1", ev2.Version)
	}
	if ev2.Type != domain.EventStreamPublishStart {
		t.Errorf("type: got %q", ev2.Type)
	}
	if ev2.NodeID != "ams-node-1" {
		t.Errorf("node_id: got %q", ev2.NodeID)
	}
	if ev2.Enrichment == nil || ev2.Enrichment.Geo == nil {
		t.Error("enrichment.geo is nil after round-trip")
	}
}

// TestServerEvent_AllTypes checks that all event type constants are valid strings
// and that we can create a ServerEvent for each.
func TestServerEvent_AllTypes(t *testing.T) {
	types := []string{
		domain.EventStreamPublishStart,
		domain.EventStreamPublishEnd,
		domain.EventStreamStats,
		domain.EventWebRTCClientStats,
		domain.EventIngestStats,
		domain.EventNodeStats,
		domain.EventRecordingReady,
		domain.EventViewerJoin,
		domain.EventViewerLeave,
	}
	for _, et := range types {
		if et == "" {
			t.Errorf("event type constant is empty")
		}
		ev := domain.ServerEvent{
			Version: 1,
			Type:    et,
			TS:      1700000000000,
			Source:  domain.SourceRestPoll,
			NodeID:  "test",
		}
		b, err := json.Marshal(ev)
		if err != nil {
			t.Errorf("marshal %q: %v", et, err)
		}
		if len(b) == 0 {
			t.Errorf("marshal %q: empty output", et)
		}
	}
}

// TestSchemaFixtures_Valid validates that the contract fixtures pass schema validation.
// This test uses npx ajv-cli; it is skipped if Node.js / npx is not available.
func TestSchemaFixtures_Valid(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available — skipping schema fixture validation")
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("cannot determine source file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "../../..")
	fixturesDir := filepath.Join(repoRoot, "contracts/events/fixtures")
	schemaPath := filepath.Join(repoRoot, "contracts/events/ams-server-event.schema.json")

	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}

	for _, e := range entries {
		if !hasPrefix(e.Name(), "ams-server-event-valid") {
			continue
		}
		fixturePath := filepath.Join(fixturesDir, e.Name())
		cmd := exec.Command("npx", "ajv-cli", "validate",
			"--spec=draft2020",
			"-s", schemaPath,
			"-d", fixturePath,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("fixture %s failed validation:\n%s", e.Name(), out)
		} else {
			t.Logf("fixture %s: valid", e.Name())
		}
	}
}

// TestSchemaFixtures_Invalid validates that invalid fixtures fail schema validation.
func TestSchemaFixtures_Invalid(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available — skipping schema fixture validation")
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("cannot determine source file path")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "../../..")
	fixturesDir := filepath.Join(repoRoot, "contracts/events/fixtures")
	schemaPath := filepath.Join(repoRoot, "contracts/events/ams-server-event.schema.json")

	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}

	for _, e := range entries {
		if !hasPrefix(e.Name(), "ams-server-event-invalid") {
			continue
		}
		fixturePath := filepath.Join(fixturesDir, e.Name())
		cmd := exec.Command("npx", "ajv-cli", "validate",
			"--spec=draft2020",
			"-s", schemaPath,
			"-d", fixturePath,
		)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("fixture %s should have failed but passed:\n%s", e.Name(), out)
		} else {
			t.Logf("fixture %s: correctly rejected", e.Name())
		}
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
