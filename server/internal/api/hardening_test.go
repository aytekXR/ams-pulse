// Package api_test — regression tests for the security-hardening batch:
//
//   - B6  handleTestSource decrypts stored credential before SetBasicAuth
//   - A2  handleIngestBeacon per-token rate limit (100 rps / 200 burst)
//   - A7  handleMetrics per-IP rate limit (10 rps / 20 burst)
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── B6: source-test decrypts stored credential ──────────────────────────────

// TestB6_SourceTest_DecryptsCredential verifies that POST /admin/sources/{id}/test
// sends the stored (encrypted) password — not an empty string — to the upstream
// AMS REST endpoint.
//
// This test FAILS against the old SetBasicAuth(user, "") code and passes only
// after the B6 fix (decrypt src.CredentialEnc before SetBasicAuth).
func TestB6_SourceTest_DecryptsCredential(t *testing.T) {
	// 1. Fake AMS server that records the Basic-Auth credentials it receives.
	var receivedUser, receivedPass atomic.Value
	fakeAMS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		receivedUser.Store(u)
		receivedPass.Store(p)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"version":"2.x"}`)
	}))
	defer fakeAMS.Close()

	// 2. Stand up the API server with a full meta store.
	ctx := context.Background()
	ddlPath := metaDDLPath(t)
	ddl, err := readMetaDDL(t)
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	_ = ddlPath
	store, err := meta.New(ctx, "sqlite", ":memory:", "b6-test-secret-key")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer store.Close()

	adminRaw := "plt_b6test_abcdef1234567890xxxxx"
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "b6-admin",
		TokenHash: hashToken(adminRaw),
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	lic, _ := license.New("", "")
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)

	srv := api.New(api.Config{ListenAddr: ":0"}, store, live, qsvc, lic, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 3. Create a source with rest_user and rest_password (triggers Encrypt).
	createBody, _ := json.Marshal(map[string]any{
		"name":          "b6-source",
		"type":          "antmedia",
		"rest_url":      fakeAMS.URL,
		"rest_user":     "amsuser",
		"rest_password": "amspass-secret",
	})
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/sources",
		bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+adminRaw)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("POST /admin/sources: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		t.Fatalf("expected 201, got %d: %s", createResp.StatusCode, body)
	}
	var created map[string]any
	json.NewDecoder(createResp.Body).Decode(&created)
	sourceID, _ := created["id"].(string)
	if sourceID == "" {
		t.Fatal("no source id in create response")
	}

	// 4. Trigger the connectivity test.
	testReq, _ := http.NewRequest(http.MethodPost,
		ts.URL+"/api/v1/admin/sources/"+sourceID+"/test", nil)
	testReq.Header.Set("Authorization", "Bearer "+adminRaw)
	testResp, err := http.DefaultClient.Do(testReq)
	if err != nil {
		t.Fatalf("POST /sources/{id}/test: %v", err)
	}
	defer testResp.Body.Close()

	if testResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(testResp.Body)
		t.Fatalf("expected 200, got %d: %s", testResp.StatusCode, body)
	}
	var result map[string]any
	json.NewDecoder(testResp.Body).Decode(&result)

	// 5. Assert the fake AMS observed the correct credentials.
	gotUser, _ := receivedUser.Load().(string)
	gotPass, _ := receivedPass.Load().(string)
	if gotUser != "amsuser" {
		t.Errorf("B6: expected username %q, got %q", "amsuser", gotUser)
	}
	if gotPass != "amspass-secret" {
		t.Errorf("B6: expected password %q, got %q (old code sends empty string; fix must decrypt)", "amspass-secret", gotPass)
	}

	// 6. Assert reachable=true and status=ok.
	reachable, _ := result["reachable"].(bool)
	if !reachable {
		t.Errorf("B6: expected reachable=true, got result=%v", result)
	}
	status, _ := result["status"].(string)
	if status != "ok" {
		t.Errorf("B6: expected status=%q, got %q", "ok", status)
	}
}

// ─── A2: beacon ingest per-token rate limit ──────────────────────────────────

// TestA2_BeaconIngest_RateLimit verifies that rapid-fire POSTs to /ingest/beacon
// with the same ingest token eventually receive HTTP 429 with code RATE_LIMITED.
//
// Design note: the production defaults are 100 rps / 200 burst. Under the race
// detector (used by CI) the DB-backed round-trip latency is 1–3 ms per request,
// which means ~150–300 tokens refill into the bucket during the time it takes to
// dispatch 300 concurrent goroutines. This makes the concurrent-HTTP approach
// unreliable under -race: all 300 requests pass the limiter without 429.
//
// Fix: use Config.BeaconBurstOverride / BeaconRateRPSOverride to configure a
// tiny bucket (burst=2, rate=0.001 rps ≈ negligible refill) for this test.
// With burst=2 the first two sequential requests are allowed and all subsequent
// ones must receive 429 — no timing dependency, no goroutines needed.
func TestA2_BeaconIngest_RateLimit(t *testing.T) {
	// Build a dedicated server with an overridden (tiny) beacon rate limit so
	// the bucket is exhausted after just 2 requests regardless of race-detector
	// overhead or wall-clock timing.
	ctx := context.Background()
	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		t.Skipf("meta DDL not found: %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "a2-rate-test-secret")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer store.Close()

	rawIngest := "pit_a2ratetest_abcdef1234567890"
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "ingest",
		Name:      "a2-rate-ingest",
		TokenHash: hashToken(rawIngest),
		Scopes:    []string{"ingest"},
		CreatedAt: 1000,
	}); err != nil {
		t.Fatalf("CreateToken (ingest): %v", err)
	}

	// Pro-tier license required for beacon ingest (VD-15).
	proKey, licCleanup := makeTestProLicense(t)
	defer licCleanup()
	lic, err := license.New(proKey, "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}

	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)

	// BeaconBurstOverride=2, BeaconRateRPSOverride=0.001: the bucket starts with
	// 2 tokens and refills at ~1 token per 1000 s — negligible during the test.
	srv := api.New(api.Config{
		ListenAddr:            ":0",
		BeaconRateRPSOverride: 0.001,
		BeaconBurstOverride:   2,
	}, store, live, qsvc, lic, nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	beaconBody := []byte(`{
		"version":1,
		"session_id":"rl-session",
		"stream_id":"rl-stream",
		"app":"live",
		"events":[{"type":"session_start","ts":1700000000000,"data":{}}]
	}`)

	// Send 5 sequential requests. The first 2 should be allowed (burst=2);
	// requests 3–5 must return 429 RATE_LIMITED.
	const total = 5
	got429 := false
	var first429Body []byte
	for i := 0; i < total; i++ {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/ingest/beacon",
			bytes.NewReader(beaconBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Pulse-Ingest-Token", rawIngest)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("A2: request %d: %v", i, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			if !got429 {
				first429Body = body
			}
			got429 = true
		}
	}

	if !got429 {
		t.Fatalf("A2: sent %d requests with burst=2 but never received 429", total)
	}
	var errResp map[string]any
	if err := json.Unmarshal(first429Body, &errResp); err != nil {
		t.Errorf("A2: 429 body not valid JSON: %v", err)
	} else {
		code, _ := errResp["code"].(string)
		if code != "RATE_LIMITED" {
			t.Errorf("A2: expected code RATE_LIMITED, got %q", code)
		}
	}
}

// ─── A7: metrics per-IP rate limit ──────────────────────────────────────────

// TestA7_Metrics_RateLimit verifies that rapid-fire GETs to /metrics from the
// test client (same loopback IP, burst=20) eventually receive HTTP 429.
func TestA7_Metrics_RateLimit(t *testing.T) {
	ts, adminToken, cleanup := setupTestServer(t)
	defer cleanup()
	_ = adminToken // /metrics is unauthenticated when MetricsToken is empty

	const total = 25 // burst=20 → at least the ~21st fires 429
	got429 := false
	for i := 0; i < total; i++ {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/metrics", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			var errResp map[string]any
			json.Unmarshal(respBody, &errResp)
			code, _ := errResp["code"].(string)
			if code != "RATE_LIMITED" {
				t.Errorf("A7: expected code RATE_LIMITED, got %q", code)
			}
			got429 = true
			break
		}
	}
	if !got429 {
		t.Errorf("A7: fired %d requests with burst=20 but never received 429", total)
	}
}
