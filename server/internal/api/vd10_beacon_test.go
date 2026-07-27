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
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/aytekXR/ams-pulse/server/internal/api"
	"github.com/aytekXR/ams-pulse/server/internal/domain"
	"github.com/aytekXR/ams-pulse/server/internal/license"
	"github.com/aytekXR/ams-pulse/server/internal/query"
	"github.com/aytekXR/ams-pulse/server/internal/store/meta"
)

// testEventSink is a thread-safe in-memory event sink for tests.
type testEventSink struct {
	mu           sync.Mutex
	beaconEvents []domain.BeaconEvent
}

func (s *testEventSink) WriteServerEvent(_ domain.ServerEvent)     {}
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

// makeTestProLicense generates a valid test Pro-tier license key using a freshly
// generated ed25519 key pair. Sets PULSE_LICENSE_PUBKEY so license.New accepts it.
// Returns the license key and a cleanup function that restores the env var.
func makeTestProLicense(t *testing.T) (key string, cleanup func()) {
	t.Helper()
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate license key pair: %v", err)
	}
	claims := map[string]any{
		"tier":           "pro",
		"max_nodes":      10,
		"retention_days": 90,
		"data_api":       true,
		"white_label":    false,
	}
	claimsJSON, _ := json.Marshal(claims)
	claimsB64 := base64.StdEncoding.EncodeToString(claimsJSON)
	sig := ed25519.Sign(privKey, claimsJSON)
	sigB64 := base64.StdEncoding.EncodeToString(sig)
	key = claimsB64 + "." + sigB64

	orig := os.Getenv("PULSE_LICENSE_PUBKEY")
	os.Setenv("PULSE_LICENSE_PUBKEY", hex.EncodeToString(pubKey))
	return key, func() {
		os.Setenv("PULSE_LICENSE_PUBKEY", orig)
	}
}

// setupServerWithSink creates an API test server with a wired testEventSink.
// VD-15: uses a Pro-tier license so beacon ingest is permitted.
// Returns the server, an ingest token (raw), a test event sink, and cleanup.
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

	// VD-15: beacon ingest requires Pro tier or higher. Use a test Pro license.
	proKey, licCleanup := makeTestProLicense(t)
	lic, err := license.New(proKey, "")
	if err != nil {
		licCleanup()
		t.Fatalf("license.New (pro tier): %v", err)
	}
	if lic.Tier() != license.TierPro {
		licCleanup()
		t.Fatalf("expected pro tier, got %q", lic.Tier())
	}

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
		licCleanup()
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

// TestVD10_BeaconPOST_NoSink_StillAccepts verifies that on Pro tier (beacon allowed)
// and without a wired sink, the handler still returns 202 (graceful degradation).
// Uses the Pro-tier setup from setupServerWithSink but without the event sink.
func TestVD10_BeaconPOST_NoSink_StillAccepts(t *testing.T) {
	ctx := context.Background()

	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "nosink-test-secret")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer store.Close()

	// Create an ingest token.
	rawIngest := "pit_nosink_test_abcdef1234567890"
	ingestHash := hashToken(rawIngest)
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "ingest",
		Name:      "nosink-ingest",
		TokenHash: ingestHash,
		Scopes:    []string{"ingest"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken (ingest): %v", err)
	}

	// VD-15: use Pro tier so beacon ingest is permitted.
	proKey, licCleanup := makeTestProLicense(t)
	defer licCleanup()
	lic, err := license.New(proKey, "")
	if err != nil {
		t.Fatalf("license.New (pro tier): %v", err)
	}

	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)

	// Intentionally do NOT wire an event sink.
	srv := api.New(api.Config{ListenAddr: ":0"}, store, live, qsvc, lic, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	batch := map[string]any{
		"version":    1,
		"session_id": "nosink-session",
		"stream_id":  "nosink-stream",
		"events": []any{
			map[string]any{"type": "session_start", "ts": int64(1700000000000), "data": map[string]any{}},
		},
	}
	batchBody, _ := json.Marshal(batch)
	beaconReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/ingest/beacon", bytes.NewReader(batchBody))
	beaconReq.Header.Set("Content-Type", "application/json")
	beaconReq.Header.Set("X-Pulse-Ingest-Token", rawIngest)

	beaconResp, err := http.DefaultClient.Do(beaconReq)
	if err != nil {
		t.Fatalf("POST /ingest/beacon: %v", err)
	}
	defer beaconResp.Body.Close()

	if beaconResp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(beaconResp.Body)
		t.Errorf("expected 202, got %d: %s", beaconResp.StatusCode, b)
	} else {
		t.Logf("PASS: Pro tier + no sink → handler still returns 202 (graceful degradation)")
	}
}

// ─── A10 field limits parity tests ────────────────────────────────────────────
//
// The dedicated beacon handler (collector/beacon) applies A10 field limits:
// - tenant truncated to maxTenantLen (64)
// - string values in event Data truncated to maxStringDataValueLen (64)
// Both truncations use truncateUTF8 which preserves UTF-8 validity.
//
// Before D-184 the main-port /ingest/beacon handler applied NEITHER truncation,
// so the two implementations diverged. These tests verify the main-port handler
// applies identical limits, using the same exported helper from the beacon
// package so they can never diverge again.

// TestMainPort_BeaconPOST_TenantTruncation verifies that a tenant value longer
// than 64 bytes is truncated to 64 bytes by the main-port handler (A10 parity).
// Before D-184 this passed the full tenant through unchanged.
func TestMainPort_BeaconPOST_TenantTruncation(t *testing.T) {
	ts, ingestToken, sink, cleanup := setupServerWithSink(t)
	defer cleanup()

	// 200-byte tenant — must be truncated to 64 bytes.
	longTenant := strings.Repeat("a", 200)
	batch := map[string]any{
		"version":    1,
		"session_id": "truncate-tenant-session",
		"stream_id":  "truncate-tenant-stream",
		"meta":       map[string]string{"tenant": longTenant},
		"events": []any{
			map[string]any{
				"type": "heartbeat",
				"ts":   int64(1700000000000),
				"data": map[string]any{"watch_ms": 1000},
			},
		},
	}
	body, _ := json.Marshal(batch)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/ingest/beacon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pulse-Ingest-Token", ingestToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /ingest/beacon: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 202, got %d: %s", resp.StatusCode, b)
	}

	// Wait for async sink write.
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
		t.Fatal("EventSink never received a BeaconEvent")
	}

	// A10 parity: tenant must be truncated to 64 bytes, same as dedicated handler.
	if len(lastEvt.Tenant) > 64 {
		t.Errorf("FAIL A10 parity: main-port tenant not truncated: len=%d, want <=64", len(lastEvt.Tenant))
	} else {
		t.Logf("PASS A10: main-port tenant truncated to %d bytes", len(lastEvt.Tenant))
	}
}

// TestMainPort_BeaconPOST_DataStringTruncation verifies that string values in
// event Data longer than 64 bytes are truncated by the main-port handler.
// Before D-184 string values were passed through unchanged.
func TestMainPort_BeaconPOST_DataStringTruncation(t *testing.T) {
	ts, ingestToken, sink, cleanup := setupServerWithSink(t)
	defer cleanup()

	// 200-byte string value in Data — must be truncated to 64 bytes.
	longValue := strings.Repeat("x", 200)
	batch := map[string]any{
		"version":    1,
		"session_id": "truncate-data-session",
		"stream_id":  "truncate-data-stream",
		"events": []any{
			map[string]any{
				"type": "error",
				"ts":   int64(1700000000000),
				"data": map[string]any{
					"code":    "MEDIA_ERR_DECODE",
					"message": longValue, // must be truncated
				},
			},
		},
	}
	body, _ := json.Marshal(batch)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/ingest/beacon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pulse-Ingest-Token", ingestToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /ingest/beacon: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 202, got %d: %s", resp.StatusCode, b)
	}

	// Wait for async sink write.
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
		t.Fatal("EventSink never received a BeaconEvent")
	}

	if len(lastEvt.Events) == 0 {
		t.Fatal("no events in BeaconEvent")
	}
	msg, ok := lastEvt.Events[0].Data["message"].(string)
	if !ok {
		t.Fatalf("expected Data[message] to be string, got %T", lastEvt.Events[0].Data["message"])
	}
	if len(msg) > 64 {
		t.Errorf("FAIL A10 parity: main-port Data string not truncated: len=%d, want <=64", len(msg))
	} else {
		t.Logf("PASS A10: main-port Data string truncated to %d bytes", len(msg))
	}
}

// TestMainPort_BeaconPOST_UTF8SafeTruncation verifies that multi-byte UTF-8
// runes are not split by truncation in the main-port handler.
// This test uses a string that would be split mid-rune at 64 bytes.
func TestMainPort_BeaconPOST_UTF8SafeTruncation(t *testing.T) {
	ts, ingestToken, sink, cleanup := setupServerWithSink(t)
	defer cleanup()

	// Build a string where byte 64 falls in the middle of a multi-byte rune.
	// "a" repeated 62 times + "日本語" (3 chars, 9 bytes) = 71 bytes.
	// truncateUTF8(s, 64) should return 62 "a"s + the first 2 bytes of "日" which
	// is invalid UTF-8, so it backtracks to 62 "a"s.
	// Actually: "日" is 3 bytes, so bytes 62..64 is incomplete — the result is 62 "a"s.
	value := strings.Repeat("a", 62) + "日本語"
	if len(value) != 71 {
		t.Fatalf("test setup: expected 71 bytes, got %d", len(value))
	}

	batch := map[string]any{
		"version":    1,
		"session_id": "utf8-safe-session",
		"stream_id":  "utf8-safe-stream",
		"meta":       map[string]string{"tenant": value}, // truncate tenant
		"events": []any{
			map[string]any{
				"type": "heartbeat",
				"ts":   int64(1700000000000),
				"data": map[string]any{"watch_ms": 1000},
			},
		},
	}
	body, _ := json.Marshal(batch)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/ingest/beacon", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pulse-Ingest-Token", ingestToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /ingest/beacon: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 202, got %d: %s", resp.StatusCode, b)
	}

	// Wait for async sink write.
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
		t.Fatal("EventSink never received a BeaconEvent")
	}

	// The truncated string must be valid UTF-8 (no split runes).
	if !utf8.ValidString(lastEvt.Tenant) {
		t.Errorf("FAIL A10 parity: truncated tenant is not valid UTF-8: %q", lastEvt.Tenant)
	}
	// Should be 62 "a"s — the rune was incomplete so truncateUTF8 backtracked.
	if lastEvt.Tenant != strings.Repeat("a", 62) {
		t.Errorf("UTF-8 safe truncation: got %q, want %q", lastEvt.Tenant, strings.Repeat("a", 62))
	}
	t.Logf("PASS A10 UTF-8: main-port truncation is UTF-8 safe (%d bytes)", len(lastEvt.Tenant))
}
