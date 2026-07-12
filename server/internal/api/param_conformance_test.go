// Package api_test — Parameter-conformance gate (S21 / D-083).
//
// TestParamConformance ensures that every query parameter declared in
// contracts/openapi/pulse-api.yaml has a corresponding registry entry:
// either a live probe that verifies the handler honours the param,
// an exemption with an explicit reason citing the fixture gap, or a
// known-violation that pins the debt against a bug file without blocking CI.
//
// Design: WO-C (S21 / D-083) — closes the class of invisible declared-but-
// ignored params that caused BUG-004 and BUG-005 to escape CI undetected.
//
// Non-vacuity guards:
//
//	(a) t.Fatal if the spec cannot be loaded (never t.Skip).
//	(b) t.Errorf for every unregistered param (all gaps reported in one run).
//	(c) minProbes = 8 — at least this many probe subtests must actually run.
//	(d) Reverse-check: t.Logf warning for any registry key absent from spec.
//
// Known-violation entries (BUG-006, BUG-007, BUG-008, BUG-009) log without
// failing — they make debt visible but do not block CI until intentionally
// fixed. Filed: S21 / D-083 (2026-07-12). Link: docs/assessment/bugs/.
package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

// ─── Registry types ───────────────────────────────────────────────────────────

type paramDisposition string

const (
	paramProbe          paramDisposition = "probe"
	paramExempt         paramDisposition = "exempt"
	paramKnownViolation paramDisposition = "known-violation"
)

type paramEntry struct {
	disp         paramDisposition
	exemptReason string           // non-empty iff disp == exempt
	bugRef       string           // non-empty iff disp == known-violation; e.g. "BUG-006"
	probeFunc    func(*testing.T) // non-nil iff disp == probe
}

// ─── Main gate ────────────────────────────────────────────────────────────────

func TestParamConformance(t *testing.T) {
	// 1. Load spec — t.Fatal (never t.Skip) if missing.
	_, thisFile, _, _ := runtime.Caller(0)
	specPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..",
		"contracts", "openapi", "pulse-api.yaml"))
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Fatalf("param-conformance: spec not found at %s", specPath)
	}
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	if err != nil {
		t.Fatalf("param-conformance: load spec %s: %v", specPath, err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatalf("param-conformance: spec %s is invalid: %v", specPath, err)
	}

	// 2. Spin shared servers (deferred cleanup).
	//
	// bizTs  — Business-tier: admin/*, alerts/*, reports/* probes.
	// healthyTs — Business-tier with fakeHealthyLiveProvider: live/*, qoe/ingest
	//             response-differential probes.
	//
	// Enterprise server omitted: all anomaly and probe params are either exempt
	// (nil detector/CH) or known-violation (BUG-006/BUG-008), so no enterprise
	// probes exist in this iteration.
	bizTs, bizTok, bizCleanup := setupBusinessServer(t)
	defer bizCleanup()

	healthyTs, healthyTok, hCleanup := setupHealthyTestServer(t)
	defer hCleanup()

	// 3. Build registry — every query param declared in the spec must appear here.
	//
	// Key format: "METHOD /openapi-path-template ?paramName"
	// The /api/v1 server-base prefix is NOT part of the key (spec paths omit it);
	// requests sent to the test server DO use the full /api/v1 path.
	registry := map[string]paramEntry{

		// ── GET /live/overview ───────────────────────────────────────────────────
		"GET /live/overview ?app": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				// fakeHealthyLiveProvider has one stream: App="live", NodeID="node-1".
				// app=live → total_publishers == 1; app=ghost → total_publishers == 0.
				t.Helper()
				for _, tc := range []struct {
					app  string
					want int
				}{
					{"live", 1},
					{"ghost", 0},
				} {
					u := healthyTs.URL + "/api/v1/live/overview?app=" + tc.app
					req, _ := http.NewRequest(http.MethodGet, u, nil)
					req.Header.Set("Authorization", authHeader(healthyTok))
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("GET live/overview?app=%s: %v", tc.app, err)
					}
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
					}
					var result map[string]any
					if err := json.Unmarshal(body, &result); err != nil {
						t.Fatalf("decode: %v", err)
					}
					got := int(result["total_publishers"].(float64))
					if got != tc.want {
						t.Errorf("?app=%s: total_publishers=%d want %d", tc.app, got, tc.want)
					} else {
						t.Logf("PASS ?app=%s: total_publishers=%d", tc.app, got)
					}
				}
			},
		},

		"GET /live/overview ?node": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				// node=node-1 → 1 publisher; node=ghost → 0 publishers.
				t.Helper()
				for _, tc := range []struct {
					node string
					want int
				}{
					{"node-1", 1},
					{"ghost", 0},
				} {
					u := healthyTs.URL + "/api/v1/live/overview?node=" + tc.node
					req, _ := http.NewRequest(http.MethodGet, u, nil)
					req.Header.Set("Authorization", authHeader(healthyTok))
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("GET live/overview?node=%s: %v", tc.node, err)
					}
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
					}
					var result map[string]any
					if err := json.Unmarshal(body, &result); err != nil {
						t.Fatalf("decode: %v", err)
					}
					got := int(result["total_publishers"].(float64))
					if got != tc.want {
						t.Errorf("?node=%s: total_publishers=%d want %d", tc.node, got, tc.want)
					} else {
						t.Logf("PASS ?node=%s: total_publishers=%d", tc.node, got)
					}
				}
			},
		},

		"GET /live/overview ?tenant": {
			// Handler reads and passes tenant, but query.LiveOverview accepts the
			// parameter and never uses it (no tenant filter in the method body) —
			// the caller-visible effect is identical to BUG-004's class.
			disp:   paramKnownViolation,
			bugRef: "BUG-009",
		},

		// ── GET /live/streams ────────────────────────────────────────────────────
		"GET /live/streams ?app": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				// app=live → 1 item; app=other → 0 items.
				t.Helper()
				for _, tc := range []struct {
					app  string
					want int
				}{
					{"live", 1},
					{"other", 0},
				} {
					u := healthyTs.URL + "/api/v1/live/streams?app=" + tc.app
					req, _ := http.NewRequest(http.MethodGet, u, nil)
					req.Header.Set("Authorization", authHeader(healthyTok))
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("GET live/streams?app=%s: %v", tc.app, err)
					}
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
					}
					var result map[string]any
					if err := json.Unmarshal(body, &result); err != nil {
						t.Fatalf("decode: %v", err)
					}
					items, _ := result["items"].([]any)
					if len(items) != tc.want {
						t.Errorf("?app=%s: items len=%d want %d", tc.app, len(items), tc.want)
					} else {
						t.Logf("PASS ?app=%s: items len=%d", tc.app, len(items))
					}
				}
			},
		},

		"GET /live/streams ?node": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				// node=node-1 → 1 item; node=ghost → 0 items.
				t.Helper()
				for _, tc := range []struct {
					node string
					want int
				}{
					{"node-1", 1},
					{"ghost", 0},
				} {
					u := healthyTs.URL + "/api/v1/live/streams?node=" + tc.node
					req, _ := http.NewRequest(http.MethodGet, u, nil)
					req.Header.Set("Authorization", authHeader(healthyTok))
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("GET live/streams?node=%s: %v", tc.node, err)
					}
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
					}
					var result map[string]any
					if err := json.Unmarshal(body, &result); err != nil {
						t.Fatalf("decode: %v", err)
					}
					items, _ := result["items"].([]any)
					if len(items) != tc.want {
						t.Errorf("?node=%s: items len=%d want %d", tc.node, len(items), tc.want)
					} else {
						t.Logf("PASS ?node=%s: items len=%d", tc.node, len(items))
					}
				}
			},
		},

		"GET /live/streams ?tenant": {
			// Same as live/overview ?tenant: query.LiveStreams accepts tenant and
			// silently drops it (no filter in the method body).
			disp:   paramKnownViolation,
			bugRef: "BUG-009",
		},

		"GET /live/streams ?limit": {
			disp:         paramExempt,
			exemptReason: "fakeHealthyLiveProvider exposes exactly 1 stream; limit=1 and limit=50 produce identical 1-item responses; no 2-stream fixture available; scout confirmed param is passed to qsvc.LiveStreams correctly.",
		},

		"GET /live/streams ?cursor": {
			// query.LiveStreams explicitly stubs the cursor ("_ = cursor // wave 1:
			// ignore cursor, return first page") while honouring limit — callers
			// can never page past page 1.
			disp:   paramKnownViolation,
			bugRef: "BUG-009",
		},

		// ── GET /analytics/audience ──────────────────────────────────────────────
		"GET /analytics/audience ?from": {
			disp:         paramExempt,
			exemptReason: "nil ClickHouse conn returns empty timeseries for all queries regardless of from value; parse-to-struct wiring confirmed by existing unit tests (wo4_internal_test.go TestParseTimeRange); no CH fixture in standard test harness.",
		},
		"GET /analytics/audience ?to": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason as audience ?from.",
		},
		"GET /analytics/audience ?app": {
			disp:         paramExempt,
			exemptReason: "nil CH returns empty AudienceResult regardless of app filter; no CH fixture.",
		},
		"GET /analytics/audience ?stream": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason as audience ?app.",
		},
		"GET /analytics/audience ?node": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /analytics/audience ?tenant": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /analytics/audience ?interval": {
			disp:         paramExempt,
			exemptReason: "nil CH returns empty response regardless of interval; parse wiring confirmed by parseAudienceParams unit test path; no CH fixture to observe differential.",
		},

		// ── GET /analytics/geo ───────────────────────────────────────────────────
		"GET /analytics/geo ?from": {
			disp:         paramExempt,
			exemptReason: "nil CH returns empty []GeoRow regardless of from; no CH fixture.",
		},
		"GET /analytics/geo ?to": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason as geo ?from.",
		},
		"GET /analytics/geo ?app": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /analytics/geo ?stream": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /analytics/geo ?tenant": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /analytics/geo ?region": {
			disp:         paramExempt,
			exemptReason: "nil CH returns empty; scout confirmed reads=yes; no CH fixture.",
		},

		// ── GET /analytics/devices ───────────────────────────────────────────────
		"GET /analytics/devices ?from": {
			disp:         paramExempt,
			exemptReason: "nil CH returns empty []DeviceRow regardless of params; no CH fixture.",
		},
		"GET /analytics/devices ?to": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /analytics/devices ?app": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /analytics/devices ?stream": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /analytics/devices ?tenant": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},

		// ── GET /qoe/summary ─────────────────────────────────────────────────────
		"GET /qoe/summary ?from": {
			disp:         paramExempt,
			exemptReason: "nil CH returns empty QoeSummaryResult regardless of from; parse wiring confirmed by handler code reading parseTimeRange; no CH fixture.",
		},
		"GET /qoe/summary ?to": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /qoe/summary ?app": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /qoe/summary ?stream": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /qoe/summary ?tenant": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /qoe/summary ?interval": {
			disp:         paramExempt,
			exemptReason: "nil CH returns empty; interval parsed and passed to QoeParams.Interval; no CH fixture to observe differential.",
		},
		"GET /qoe/summary ?country": {
			disp:         paramExempt,
			exemptReason: "nil CH reason; scout confirms reads=yes.",
		},
		"GET /qoe/summary ?device": {
			disp:         paramExempt,
			exemptReason: "nil CH reason; scout confirms reads=yes.",
		},

		// ── GET /qoe/ingest ──────────────────────────────────────────────────────
		//
		// from, to, interval: use captureIngestQsvc (local fresh server per probe
		// to keep capture slices independent). app, stream, node: response
		// differential on healthyTs (fakeHealthyLiveProvider: 1 stream,
		// id=healthy-stream-1, app=live, node=node-1).

		"GET /qoe/ingest ?from": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				epochMs := int64(1_700_000_000_000) // 2023-11-14T22:13:20Z
				cap := &captureIngestQsvc{}
				ts, tok, cleanup := setupIngestCaptureServer(t, cap)
				defer cleanup()

				vals := url.Values{}
				vals.Set("from", strconv.FormatInt(epochMs, 10))
				req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/qoe/ingest?"+vals.Encode(), nil)
				req.Header.Set("Authorization", authHeader(tok))
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("GET qoe/ingest?from=...: %v", err)
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("want 200, got %d", resp.StatusCode)
				}
				if len(cap.captured) == 0 {
					t.Fatal("qoe/ingest ?from: IngestTimeseries not called (iqsvc not wired?)")
				}
				wantFrom := time.UnixMilli(epochMs)
				if !cap.captured[0].From.Equal(wantFrom) {
					t.Errorf("IngestTimeseriesParams.From = %v, want %v", cap.captured[0].From, wantFrom)
				} else {
					t.Logf("PASS: From=%v", cap.captured[0].From)
				}
			},
		},

		"GET /qoe/ingest ?to": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				epochMs := int64(1_700_100_000_000)
				cap := &captureIngestQsvc{}
				ts, tok, cleanup := setupIngestCaptureServer(t, cap)
				defer cleanup()

				vals := url.Values{}
				vals.Set("to", strconv.FormatInt(epochMs, 10))
				req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/qoe/ingest?"+vals.Encode(), nil)
				req.Header.Set("Authorization", authHeader(tok))
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("GET qoe/ingest?to=...: %v", err)
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("want 200, got %d", resp.StatusCode)
				}
				if len(cap.captured) == 0 {
					t.Fatal("qoe/ingest ?to: IngestTimeseries not called")
				}
				wantTo := time.UnixMilli(epochMs)
				if !cap.captured[0].To.Equal(wantTo) {
					t.Errorf("IngestTimeseriesParams.To = %v, want %v", cap.captured[0].To, wantTo)
				} else {
					t.Logf("PASS: To=%v", cap.captured[0].To)
				}
			},
		},

		"GET /qoe/ingest ?app": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				// Response differential: app=live → 1 stream; app=ghost → 0 streams.
				t.Helper()
				for _, tc := range []struct {
					app  string
					want int
				}{
					{"live", 1},
					{"ghost", 0},
				} {
					u := healthyTs.URL + "/api/v1/qoe/ingest?app=" + tc.app
					req, _ := http.NewRequest(http.MethodGet, u, nil)
					req.Header.Set("Authorization", authHeader(healthyTok))
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("GET qoe/ingest?app=%s: %v", tc.app, err)
					}
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
					}
					var result map[string]any
					if err := json.Unmarshal(body, &result); err != nil {
						t.Fatalf("decode: %v", err)
					}
					streams, _ := result["streams"].([]any)
					if len(streams) != tc.want {
						t.Errorf("?app=%s: streams len=%d want %d", tc.app, len(streams), tc.want)
					} else {
						t.Logf("PASS ?app=%s: streams len=%d", tc.app, len(streams))
					}
				}
			},
		},

		"GET /qoe/ingest ?stream": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				// stream=healthy-stream-1 → 1 stream; stream=ghost → 0 streams.
				t.Helper()
				for _, tc := range []struct {
					stream string
					want   int
				}{
					{"healthy-stream-1", 1},
					{"ghost", 0},
				} {
					u := healthyTs.URL + "/api/v1/qoe/ingest?stream=" + tc.stream
					req, _ := http.NewRequest(http.MethodGet, u, nil)
					req.Header.Set("Authorization", authHeader(healthyTok))
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("GET qoe/ingest?stream=%s: %v", tc.stream, err)
					}
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
					}
					var result map[string]any
					if err := json.Unmarshal(body, &result); err != nil {
						t.Fatalf("decode: %v", err)
					}
					streams, _ := result["streams"].([]any)
					if len(streams) != tc.want {
						t.Errorf("?stream=%s: streams len=%d want %d", tc.stream, len(streams), tc.want)
					} else {
						t.Logf("PASS ?stream=%s: streams len=%d", tc.stream, len(streams))
					}
				}
			},
		},

		"GET /qoe/ingest ?node": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				// node=node-1 → 1 stream; node=ghost → 0 streams.
				t.Helper()
				for _, tc := range []struct {
					node string
					want int
				}{
					{"node-1", 1},
					{"ghost", 0},
				} {
					u := healthyTs.URL + "/api/v1/qoe/ingest?node=" + tc.node
					req, _ := http.NewRequest(http.MethodGet, u, nil)
					req.Header.Set("Authorization", authHeader(healthyTok))
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("GET qoe/ingest?node=%s: %v", tc.node, err)
					}
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
					}
					var result map[string]any
					if err := json.Unmarshal(body, &result); err != nil {
						t.Fatalf("decode: %v", err)
					}
					streams, _ := result["streams"].([]any)
					if len(streams) != tc.want {
						t.Errorf("?node=%s: streams len=%d want %d", tc.node, len(streams), tc.want)
					} else {
						t.Logf("PASS ?node=%s: streams len=%d", tc.node, len(streams))
					}
				}
			},
		},

		"GET /qoe/ingest ?interval": {
			// BUG-005 live probe — GREEN after parseBucketInterval fix in server.go.
			// hour → BucketSeconds=3600; fix committed before this file (same session).
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				cap := &captureIngestQsvc{}
				ts, tok, cleanup := setupIngestCaptureServer(t, cap)
				defer cleanup()

				req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/qoe/ingest?interval=hour", nil)
				req.Header.Set("Authorization", authHeader(tok))
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("GET qoe/ingest?interval=hour: %v", err)
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("want 200, got %d", resp.StatusCode)
				}
				if len(cap.captured) == 0 {
					t.Fatal("qoe/ingest ?interval: IngestTimeseries not called")
				}
				const wantBucket = 3600
				if cap.captured[0].BucketSeconds != wantBucket {
					t.Errorf("BUG-005: BucketSeconds=%d want %d (interval=hour)",
						cap.captured[0].BucketSeconds, wantBucket)
				} else {
					t.Logf("PASS: BucketSeconds=%d (interval=hour)", cap.captured[0].BucketSeconds)
				}
			},
		},

		// ── GET /alerts/rules ────────────────────────────────────────────────────
		"GET /alerts/rules ?limit": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},
		"GET /alerts/rules ?cursor": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},

		// ── GET /alerts/channels ─────────────────────────────────────────────────
		"GET /alerts/channels ?limit": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},
		"GET /alerts/channels ?cursor": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},

		// ── GET /alerts/history ──────────────────────────────────────────────────
		"GET /alerts/history ?from": {
			disp:         paramExempt,
			exemptReason: "alert_history table is empty in test env (no fired alerts); from/to/limit/rule_id/state are all read and passed per scout (reads=yes); observable reaction requires seeded alert_history rows not available in standard test harness.",
		},
		"GET /alerts/history ?to": {
			disp:         paramExempt,
			exemptReason: "Same empty-fixture reason as alerts/history ?from.",
		},
		"GET /alerts/history ?limit": {
			disp:         paramExempt,
			exemptReason: "Same empty-fixture reason; limit IS read and passed to store.ListAlertHistory (scout reads=yes); differential requires multiple seeded history rows.",
		},
		"GET /alerts/history ?cursor": {
			// cursor threading omitted from store.ListAlertHistory — see BUG-007.
			disp:   paramKnownViolation,
			bugRef: "BUG-007",
		},
		"GET /alerts/history ?rule_id": {
			disp:         paramExempt,
			exemptReason: "Same empty-fixture reason; rule_id is read and passed to store.ListAlertHistory (scout reads=yes).",
		},
		"GET /alerts/history ?state": {
			disp:         paramExempt,
			exemptReason: "Same empty-fixture reason; state is read and passed (scout reads=yes).",
		},

		// ── GET /reports/usage ───────────────────────────────────────────────────
		"GET /reports/usage ?from": {
			disp:         paramExempt,
			exemptReason: "nil CH returns empty UsageReportResponse; no CH fixture; scout confirms reads=yes.",
		},
		"GET /reports/usage ?to": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /reports/usage ?app": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /reports/usage ?stream": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /reports/usage ?tenant": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /reports/usage ?interval": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason; scout confirms reads=yes.",
		},

		// ── GET /reports/schedules ───────────────────────────────────────────────
		"GET /reports/schedules ?limit": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},
		"GET /reports/schedules ?cursor": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},

		// ── GET /fleet/nodes ─────────────────────────────────────────────────────
		"GET /fleet/nodes ?limit": {
			disp:         paramExempt,
			exemptReason: "in-memory LiveProvider returns no nodes in test env; limit=1 and limit=50 produce identical empty node lists; scout confirmed param is passed to qsvc.FleetNodes correctly.",
		},
		"GET /fleet/nodes ?cursor": {
			disp:         paramExempt,
			exemptReason: "Same empty-nodes-fixture reason; scout confirmed param passes to qsvc.FleetNodes.",
		},

		// ── GET /anomalies ───────────────────────────────────────────────────────
		"GET /anomalies ?from": {
			disp:   paramKnownViolation,
			bugRef: "BUG-008",
		},
		"GET /anomalies ?to": {
			disp:   paramKnownViolation,
			bugRef: "BUG-008",
		},
		"GET /anomalies ?app": {
			disp:   paramKnownViolation,
			bugRef: "BUG-008",
		},
		"GET /anomalies ?stream": {
			disp:   paramKnownViolation,
			bugRef: "BUG-008",
		},
		"GET /anomalies ?limit": {
			disp:   paramKnownViolation,
			bugRef: "BUG-008",
		},
		"GET /anomalies ?cursor": {
			disp:   paramKnownViolation,
			bugRef: "BUG-008",
		},
		"GET /anomalies ?metric": {
			disp:         paramExempt,
			exemptReason: "anomalyDetector is nil in test env; handler returns early with empty items before reaching the metric post-filter; scout confirmed reads=yes but observable reaction requires a seeded baseline detector unavailable in standard test harness.",
		},
		"GET /anomalies ?min_sigma": {
			disp:         paramExempt,
			exemptReason: "Same nil-detector reason; min_sigma is read and parsed (scout reads=yes) but passed to ComputeFlags which is never reached when detector is nil.",
		},

		// ── GET /probes ──────────────────────────────────────────────────────────
		"GET /probes ?limit": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},
		"GET /probes ?cursor": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},

		// ── GET /probes/{probeId}/results ────────────────────────────────────────
		"GET /probes/{probeId}/results ?from": {
			disp:         paramExempt,
			exemptReason: "nil CH returns empty probe results regardless of from/to; no CH fixture; scout confirmed from is parsed via parseTimeRange and passed to qsvc.QueryProbeResults.",
		},
		"GET /probes/{probeId}/results ?to": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason.",
		},
		"GET /probes/{probeId}/results ?limit": {
			disp:         paramExempt,
			exemptReason: "Same nil-CH reason; limit is read and passed to QueryProbeResults (scout reads=yes).",
		},
		"GET /probes/{probeId}/results ?cursor": {
			// cursor absent from handler and qsvc.QueryProbeResults signature — see BUG-007.
			disp:   paramKnownViolation,
			bugRef: "BUG-007",
		},

		// ── GET /auth/oidc/callback ──────────────────────────────────────────────
		"GET /auth/oidc/callback ?code": {
			disp:         paramExempt,
			exemptReason: "OIDC callback requires a live OIDC provider to exchange the authorization code; handler reads r.FormValue('code') (scout confirmed reads=yes at S21/D-083); a unit probe would require a mock OIDC server not present in the standard test harness.",
		},
		"GET /auth/oidc/callback ?state": {
			disp:         paramExempt,
			exemptReason: "Same OIDC provider reason; handler reads r.FormValue('state') (scout confirmed reads=yes at S21/D-083).",
		},

		// ── GET /admin/sources ───────────────────────────────────────────────────
		"GET /admin/sources ?limit": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},
		"GET /admin/sources ?cursor": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},

		// ── GET /admin/tokens ────────────────────────────────────────────────────
		"GET /admin/tokens ?limit": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},
		"GET /admin/tokens ?cursor": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},
		"GET /admin/tokens ?kind": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				// bizTs has one token pre-seeded by setupBusinessServer: kind=api.
				// kind=api → items >= 1; kind=ingest → items == 0.
				t.Helper()
				for _, tc := range []struct {
					kind    string
					wantMin int
					wantMax int
				}{
					{"api", 1, 999},
					{"ingest", 0, 0},
				} {
					u := bizTs.URL + "/api/v1/admin/tokens?kind=" + tc.kind
					req, _ := http.NewRequest(http.MethodGet, u, nil)
					req.Header.Set("Authorization", authHeader(bizTok))
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("GET admin/tokens?kind=%s: %v", tc.kind, err)
					}
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
					}
					var result map[string]any
					if err := json.Unmarshal(body, &result); err != nil {
						t.Fatalf("decode: %v", err)
					}
					items, _ := result["items"].([]any)
					if len(items) < tc.wantMin || len(items) > tc.wantMax {
						t.Errorf("?kind=%s: items len=%d want [%d,%d]",
							tc.kind, len(items), tc.wantMin, tc.wantMax)
					} else {
						t.Logf("PASS ?kind=%s: items len=%d", tc.kind, len(items))
					}
				}
			},
		},

		// ── GET /admin/users ─────────────────────────────────────────────────────
		"GET /admin/users ?limit": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},
		"GET /admin/users ?cursor": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},

		// ── GET /admin/tenants ───────────────────────────────────────────────────
		"GET /admin/tenants ?limit": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},
		"GET /admin/tenants ?cursor": {
			disp:   paramKnownViolation,
			bugRef: "BUG-006",
		},
	}

	// 4. Enumerate spec: query params only (deterministic output via sorted loop).
	//    Collect keys first, then sort, for stable error message ordering.
	type specParam struct {
		method  string
		rawPath string
		name    string
	}
	var specParams []specParam
	for rawPath, item := range doc.Paths.Map() {
		ops := map[string]*openapi3.Operation{
			"GET":    item.Get,
			"POST":   item.Post,
			"PUT":    item.Put,
			"DELETE": item.Delete,
			"PATCH":  item.Patch,
		}
		for method, op := range ops {
			if op == nil {
				continue
			}
			for _, pRef := range op.Parameters {
				p := pRef.Value
				if p == nil || p.In != "query" {
					continue
				}
				specParams = append(specParams, specParam{method, rawPath, p.Name})
			}
		}
	}

	// 4a. Non-vacuity floor: the walk enumerated 85 query params at authoring
	//     time (S21/D-083). If kin-openapi ever silently under-enumerates
	//     (e.g. a $ref refactor that doc.Validate does not catch), the gate
	//     must go loud instead of vacuously passing. Lower this constant only
	//     for an intentional spec shrink.
	const minSpecParams = 85
	if len(specParams) < minSpecParams {
		t.Errorf("param-conformance: enumerated only %d spec query params, "+
			"expected >= %d — spec load may be incomplete", len(specParams), minSpecParams)
	}

	seen := map[string]bool{}
	var missing, probesRan int
	for _, sp := range specParams {
		key := fmt.Sprintf("%s %s ?%s", sp.method, sp.rawPath, sp.name)
		seen[key] = true
		entry, ok := registry[key]
		if !ok {
			t.Errorf("PARAM-CONFORMANCE GATE: %q is declared in OpenAPI "+
				"but has no registry entry — add probe/exempt/known-violation", key)
			missing++
			continue
		}
		switch entry.disp {
		case paramProbe:
			t.Run(key, func(t *testing.T) { entry.probeFunc(t) })
			probesRan++
		case paramExempt:
			t.Logf("EXEMPT %s: %s", key, entry.exemptReason)
		case paramKnownViolation:
			t.Logf("KNOWN-VIOLATION %s: %s (pin: see docs/assessment/bugs/%s*.md)",
				key, entry.bugRef, entry.bugRef)
		}
	}

	// 4b. Reverse-check: warn about registry keys not found in the spec.
	//     Warns (not fails) to avoid noise during spec evolution.
	for key := range registry {
		if !seen[key] {
			t.Logf("WARN param-conformance: registry key %q not found in spec — "+
				"may indicate a stale entry after a spec update", key)
		}
	}

	// 5. Non-vacuity guards.
	if missing > 0 {
		t.Errorf("param-conformance: %d unregistered param(s) found — gate open", missing)
	}
	const minProbes = 8
	if probesRan < minProbes {
		t.Errorf("param-conformance: only %d probe(s) ran (need >= %d); "+
			"check that probe entries are not all skipping", probesRan, minProbes)
	} else {
		t.Logf("param-conformance: %d probe(s) ran, gate satisfied", probesRan)
	}
}

// NOTE: This file reuses setupBusinessServer (v3b_guard_test.go), setupHealthyTestServer
// and setupIngestCaptureServer (vd20b_vd21_ingest_test.go), captureIngestQsvc, and
// authHeader (api_test.go) — all in the same package api_test. No new helpers needed.
