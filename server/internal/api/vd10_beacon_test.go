// Package api_test — VD-10 regression test:
//
// The main-port /ingest/beacon handler MUST write events to the EventSink
// when one is wired. Before VD-10 the handler authenticated the token and
// decoded the body but silently discarded all events (no sink call).
//
// This test:
//  1. Builds an API server with a test EventSink.
//  2. Creates an ingest token in the meta store.
//  3. POSTs a valid beacon batch to the main-port /ingest/beacon.
//  4. Asserts (a) 202 Accepted, (b) the test EventSink received the event.
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// testEventSink is a thread-safe in-memory event sink for tests.
type testEventSink struct {
	mu           sync.Mutex
	beaconEvents []domain.BeaconEvent
}

func (s *testEventSink) WriteServerEvent(_ domain.ServerEvent)    {}
func (s *testEventSink) WriteViewerSession(_ domain.ViewerSession) {}
func (s *testEventSink) WriteBeaconEvent(ev domain.BeaconEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beaconEvents = append(s.beaconEvents, ev)
}

func (s *testEventSink) BeaconCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.beaconEvents)
}

func (s *testEventSink) LastEvent() (domain.BeaconEvent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.beaconEvents) == 0 {
		return domain.BeaconEvent{}, false
	}
	return s.beaconEvents[len(s.beaconEvents)-1], true
}

// setupServerWithSink creates an API test server with a wired testEventSink.
// Returns the server, an ingest token (raw), a test admin token, and cleanup.
func setupServerWithSink(t *testing.T) (ts *httptest.Server, ingestToken string, sink *testEventSink, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "beacon-test-secret")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Create an ingest token.
	rawIngest := "pit_testingest_abcdef1234567890"
	ingestHash := hashToken(rawIngest)
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "ingest",
		Name:      "test-ingest",
		TokenHash: ingestHash,
		Scopes:    []string{"ingest"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken (ingest): %v", err)
	}

	lic, _ := license.New("", "")
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)

	sink = &testEventSink{}

	apiCfg := api.Config{ListenAddr: ":0"}
	srv := api.New(apiCfg, store, live, qsvc, lic, nil)
	srv.SetEventSink(sink)

	ts = httptest.NewServer(srv.Handler())
	cleanup = func() {
		ts.Close()
		store.Close()
	}
	return ts, rawIngest, sink, cleanup
}

// TestVD10_BeaconPOST_PersistsToSink verifies that a valid beacon POST to the
// main API port is written to the wired EventSink.
// This is the regression test for VD-10 (handler was silently discarding events).
func TestVD10_BeaconPOST_PersistsToSink(t *testing.T) {
	ts, ingestToken, sink, cleanup := setupServerWithSink(t)
	defer cleanup()

	batch := map[string]any{
		"version":    1,
		"session_id": "test-session-abc123",
		"stream_id":  "test-stream-xyz",
		"app":        "live",
		"events": []any{
			map[string]any{
				"type": "session_start",
				"ts":   1700000000000,
				"data": map[string]any{},
			},
		},
	}
	body, _ := json.Marshal(batch)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/ingest/beacon", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pulse-Ingest-Token", ingestToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /ingest/beacon: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", resp.StatusCode, respBody)
	}

	// Verify response shape.
	var respJSON map[string]any
	if err := json.Unmarshal(respBody, &respJSON); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	accepted, _ := respJSON["accepted"].(float64)
	if int(accepted) != 1 {
		t.Errorf("expected accepted=1, got %v", accepted)
	}

	// VD-10: verify the event was written to the sink.
	// The handler does `go s.eventSink.WriteBeaconEvent(evt)` so we need to
	// wait briefly for the goroutine to run. Spin-wait bounded to 1 second.
	var lastEvt domain.BeaconEvent
	var got bool
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		lastEvt, got = sink.LastEvent()
		if got {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !got {
		t.Fatal("VD-10 FAIL: EventSink never received a BeaconEvent (handler silently discarded the event)")
	}
	if lastEvt.SessionID != "test-session-abc123" {
		t.Errorf("expected session_id=%q, got %q", "test-session-abc123", lastEvt.SessionID)
	}
	if lastEvt.StreamID != "test-stream-xyz" {
		t.Errorf("expected stream_id=%q, got %q", "test-stream-xyz", lastEvt.StreamID)
	}
	if len(lastEvt.Events) != 1 {
		t.Errorf("expected 1 beacon item, got %d", len(lastEvt.Events))
	}
	t.Logf("PASS VD-10: BeaconEvent delivered to sink (session=%s, events=%d)",
		lastEvt.SessionID, len(lastEvt.Events))
}

// TestVD10_BeaconPOST_64KB_Cap verifies the body size limit is 64 KB
// (aligned to the spec, VD-10 fix from 256 KB).
func TestVD10_BeaconPOST_64KB_Cap(t *testing.T) {
	ts, ingestToken, _, cleanup := setupServerWithSink(t)
	defer cleanup()

	// 70 KB body — must return 413.
	oversized := make([]byte, 70*1024)
	for i := range oversized {
		oversized[i] = 'x'
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/ingest/beacon", bytes.NewReader(oversized))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pulse-Ingest-Token", ingestToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /ingest/beacon: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 413, got %d: %s", resp.StatusCode, body)
	} else {
		t.Logf("PASS: 70 KB body correctly returns 413")
	}
}

// TestVD10_BeaconPOST_NoSink_StillAccepts verifies that without a wired sink
// (legacy / test scenario) the handler still returns 202 (graceful degradation).
func TestVD10_BeaconPOST_NoSink_StillAccepts(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()

	// The default setupTestServer does NOT wire an event sink.
	// We need an ingest token — create one using the admin token.
	createBody, _ := json.Marshal(map[string]any{
		"name": "test-ingest-nosink",
		"kind": "ingest",
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tokens",
		bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create ingest token: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Skipf("cannot create ingest token (status %d): %s", resp.StatusCode, respBody)
	}

	var tokenResp map[string]any
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		t.Skipf("cannot parse token response: %v", err)
	}
	rawToken, _ := tokenResp["token"].(string)
	if rawToken == "" {
		t.Skip("token not in response — skipping no-sink test")
	}

	batch := map[string]any{
		"version":    1,
		"session_id": "nosink-session",
		"stream_id":  "nosink-stream",
		"events": []any{
			map[string]any{"type": "session_start", "ts": int64(1700000000000), "data": map[string]any{}},
		},
	}
	batchBody, _ := json.Marshal(batch)
	beaconReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/ingest/beacon",
		bytes.NewReader(batchBody))
	beaconReq.Header.Set("Content-Type", "application/json")
	beaconReq.Header.Set("X-Pulse-Ingest-Token", rawToken)

	beaconResp, err := http.DefaultClient.Do(beaconReq)
	if err != nil {
		t.Fatalf("POST /ingest/beacon: %v", err)
	}
	defer beaconResp.Body.Close()

	if beaconResp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(beaconResp.Body)
		t.Errorf("expected 202, got %d: %s", beaconResp.StatusCode, b)
	} else {
		t.Logf("PASS: without sink the handler still returns 202 (graceful degradation)")
	}
}
