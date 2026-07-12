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
//	(c) minProbes = 35 — at least this many probe subtests must actually run
//	    (37 probes as of S24/D-086; floor = 37 − 2 for minor-evolution headroom).
//	(d) Reverse-check: t.Logf warning for any registry key absent from spec.
//
// Known-violation entries (BUG-006, BUG-007, BUG-008, BUG-009) log without
// failing — they make debt visible but do not block CI until intentionally
// fixed. Filed: S21 / D-083 (2026-07-12). Link: docs/assessment/bugs/.
package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
			// Handler correctly reads and passes tenant, but domain.LiveStream has no TenantID
			// field and domain.LiveSnapshot carries no tenant assignment. reports.TenantMatcher
			// operates in the historical-report layer only and is not wired into query.Service.
			// Fix requires adding TenantID to the live data model (F6 multi-tenancy backlog).
			// Cursor for LiveStreams is now fixed in S22 (see ?cursor entry above).
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
			// Same as live/overview ?tenant: no TenantID on domain.LiveStream.
			// Cursor is now fixed (see separate ?cursor entry). Tenant remains pinned.
			disp:   paramKnownViolation,
			bugRef: "BUG-009",
		},

		"GET /live/streams ?limit": {
			disp:         paramExempt,
			exemptReason: "fakeHealthyLiveProvider exposes exactly 1 stream; limit=1 and limit=50 produce identical 1-item responses; no 2-stream fixture available; scout confirmed param is passed to qsvc.LiveStreams correctly.",
		},

		"GET /live/streams ?cursor": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				ts2, tok2, cleanup2 := setupTwoStreamServer(t)
				defer cleanup2()
				// Page 1: limit=1, no cursor -> 1 item, next_cursor non-nil.
				req1, _ := http.NewRequest(http.MethodGet, ts2.URL+"/api/v1/live/streams?limit=1", nil)
				req1.Header.Set("Authorization", authHeader(tok2))
				resp1, err := http.DefaultClient.Do(req1)
				if err != nil {
					t.Fatalf("page1: %v", err)
				}
				b1, _ := io.ReadAll(resp1.Body)
				resp1.Body.Close()
				if resp1.StatusCode != http.StatusOK {
					t.Fatalf("page1: want 200, got %d: %s", resp1.StatusCode, b1)
				}
				var r1 map[string]any
				json.Unmarshal(b1, &r1)
				items1 := r1["items"].([]any)
				if len(items1) != 1 {
					t.Fatalf("page1: want 1 item, got %d", len(items1))
				}
				cursor, ok := r1["meta"].(map[string]any)["next_cursor"].(string)
				if !ok || cursor == "" {
					t.Fatalf("page1: next_cursor missing — cursor not emitted?")
				}
				// Page 2: cursor must advance.
				req2, _ := http.NewRequest(http.MethodGet,
					ts2.URL+"/api/v1/live/streams?limit=1&cursor="+url.QueryEscape(cursor), nil)
				req2.Header.Set("Authorization", authHeader(tok2))
				resp2, err := http.DefaultClient.Do(req2)
				if err != nil {
					t.Fatalf("page2: %v", err)
				}
				b2, _ := io.ReadAll(resp2.Body)
				resp2.Body.Close()
				if resp2.StatusCode != http.StatusOK {
					t.Fatalf("page2: want 200, got %d: %s", resp2.StatusCode, b2)
				}
				var r2 map[string]any
				json.Unmarshal(b2, &r2)
				items2 := r2["items"].([]any)
				if len(items2) == 0 {
					t.Fatalf("page2: got 0 items — cursor ignored?")
				}
				sid1 := items1[0].(map[string]any)["stream_id"].(string)
				sid2 := items2[0].(map[string]any)["stream_id"].(string)
				if sid1 == sid2 {
					t.Errorf("cursor not advancing: same stream_id %s on both pages", sid1)
				} else {
					t.Logf("PASS: %s -> %s", sid1, sid2)
				}
			},
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
		// BUG-010 fixed S22/D-084: contract now declares ?format; handler already implemented CSV.
		"GET /analytics/audience ?format": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				// Response-differential: format=json -> Content-Type: application/json;
				// format=csv -> Content-Type: text/csv.
				// nil ClickHouse returns empty timeseries; the CSV branch fires before
				// JSON serialization, so Content-Type is observable regardless of data.
				for _, tc := range []struct {
					format string
					wantCT string
				}{
					{"json", "application/json"},
					{"csv", "text/csv"},
				} {
					u := healthyTs.URL + "/api/v1/analytics/audience?format=" + tc.format
					req, _ := http.NewRequest(http.MethodGet, u, nil)
					req.Header.Set("Authorization", authHeader(healthyTok))
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("GET ?format=%s: %v", tc.format, err)
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("?format=%s: want 200, got %d", tc.format, resp.StatusCode)
					}
					ct := resp.Header.Get("Content-Type")
					if ct == "" {
						ct = "application/json"
					}
					if !strings.Contains(ct, tc.wantCT) {
						t.Errorf("?format=%s: Content-Type=%q, want to contain %q", tc.format, ct, tc.wantCT)
					} else {
						t.Logf("PASS ?format=%s: Content-Type=%q", tc.format, ct)
					}
				}
			},
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
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/alerts/rules", bizTok, map[string]any{
						"name": fmt.Sprintf("rule-lim-%d", i), "metric": "bitrate",
						"operator": "lt", "threshold": 100.0,
					})
				}
				items, nc := getListPage(t, bizTs.URL, "/api/v1/alerts/rules?limit=1", bizTok)
				if len(items) != 1 {
					t.Errorf("?limit=1: got %d items, want 1", len(items))
				} else {
					t.Logf("PASS ?limit=1: 1 item returned")
				}
				if nc == "" {
					t.Errorf("?limit=1: next_cursor empty, want non-empty (more items exist)")
				} else {
					t.Logf("PASS ?limit=1: next_cursor=%q", nc)
				}
			},
		},
		"GET /alerts/rules ?cursor": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/alerts/rules", bizTok, map[string]any{
						"name": fmt.Sprintf("rule-cur-%d", i), "metric": "bitrate",
						"operator": "lt", "threshold": 100.0,
					})
				}
				_, nc := getListPage(t, bizTs.URL, "/api/v1/alerts/rules?limit=1", bizTok)
				if nc == "" {
					t.Fatal("?cursor: no next_cursor from page 1; need >= 2 items")
				}
				items2, _ := getListPage(t, bizTs.URL, "/api/v1/alerts/rules?limit=50&cursor="+url.QueryEscape(nc), bizTok)
				if len(items2) < 1 {
					t.Errorf("?cursor: page2 has %d items, want >= 1 (cursor not advancing)", len(items2))
				} else {
					t.Logf("PASS ?cursor: page2 has %d item(s)", len(items2))
				}
			},
		},

		// ── GET /alerts/channels ─────────────────────────────────────────────────
		"GET /alerts/channels ?limit": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/alerts/channels", bizTok, map[string]any{
						"type": "webhook", "name": fmt.Sprintf("chan-lim-%d", i),
					})
				}
				items, nc := getListPage(t, bizTs.URL, "/api/v1/alerts/channels?limit=1", bizTok)
				if len(items) != 1 {
					t.Errorf("?limit=1: got %d items, want 1", len(items))
				} else {
					t.Logf("PASS ?limit=1: 1 item returned")
				}
				if nc == "" {
					t.Errorf("?limit=1: next_cursor empty, want non-empty")
				} else {
					t.Logf("PASS ?limit=1: next_cursor=%q", nc)
				}
			},
		},
		"GET /alerts/channels ?cursor": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/alerts/channels", bizTok, map[string]any{
						"type": "webhook", "name": fmt.Sprintf("chan-cur-%d", i),
					})
				}
				_, nc := getListPage(t, bizTs.URL, "/api/v1/alerts/channels?limit=1", bizTok)
				if nc == "" {
					t.Fatal("?cursor: no next_cursor from page 1; need >= 2 items")
				}
				items2, _ := getListPage(t, bizTs.URL, "/api/v1/alerts/channels?limit=50&cursor="+url.QueryEscape(nc), bizTok)
				if len(items2) < 1 {
					t.Errorf("?cursor: page2 has %d items, want >= 1", len(items2))
				} else {
					t.Logf("PASS ?cursor: page2 has %d item(s)", len(items2))
				}
			},
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
			// BUG-007 fix verified S22/D-084: cursor now threaded through handleAlertHistory →
			// store.ListAlertHistory (keyset pagination). Probe seeds >= 3 rows via the
			// meta store (the setupAlertHistoryServer helper returns the store directly),
			// pages with limit=1, and asserts page 2 returns a distinct item from page 1.
			// Reverting cursor threading causes page 2 to repeat page 1 → id1 == id2 → fail.
			disp:      paramProbe,
			probeFunc: func(t *testing.T) { probeAlertHistoryCursor(t) },
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
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/reports/schedules", bizTok, map[string]any{
						"cron": "0 6 * * *", "format": "csv",
					})
				}
				items, nc := getListPage(t, bizTs.URL, "/api/v1/reports/schedules?limit=1", bizTok)
				if len(items) != 1 {
					t.Errorf("?limit=1: got %d items, want 1", len(items))
				} else {
					t.Logf("PASS ?limit=1: 1 item returned")
				}
				if nc == "" {
					t.Errorf("?limit=1: next_cursor empty, want non-empty")
				} else {
					t.Logf("PASS ?limit=1: next_cursor=%q", nc)
				}
			},
		},
		"GET /reports/schedules ?cursor": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/reports/schedules", bizTok, map[string]any{
						"cron": "0 8 * * *", "format": "csv",
					})
				}
				_, nc := getListPage(t, bizTs.URL, "/api/v1/reports/schedules?limit=1", bizTok)
				if nc == "" {
					t.Fatal("?cursor: no next_cursor from page 1; need >= 2 items")
				}
				items2, _ := getListPage(t, bizTs.URL, "/api/v1/reports/schedules?limit=50&cursor="+url.QueryEscape(nc), bizTok)
				if len(items2) < 1 {
					t.Errorf("?cursor: page2 has %d items, want >= 1", len(items2))
				} else {
					t.Logf("PASS ?cursor: page2 has %d item(s)", len(items2))
				}
			},
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
		//
		// ── BUG-008 triage (S22 / D-084 → S24 / D-086) ─────────────────────────
		// Group A (app, stream, limit, cursor): handler-only fixes delivered S22/D-084.
		// Each probe spins up a fresh Enterprise server with fakeAnomalyDetector
		// (stdFakeFlags: 6 flags across 2 apps × 3 streams) so the response-differential
		// is observable without a ClickHouse baseline store.
		//
		// Group B (from, to): promoted from known-violation to probe in S24/D-086.
		// Fix: ADR-0009 anomaly_flag_events store; FlagHistoryQuerier wired into
		// handleAnomalies. Each probe injects a recordingFlagHistoryQuerier via
		// SetFlagHistoryQuerier and asserts the parsed time.Time reaches the querier.
		"GET /anomalies ?from": {
			// BUG-008 Group B — fixed S24/D-086 (ADR-0009 flag-event store).
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				epochMs := int64(1_700_000_000_000) // 2023-11-14T22:13:20Z
				rec := &recordingFlagHistoryQuerier{}
				ants, anTok, anCleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
				defer anCleanup()

				req, _ := http.NewRequest(http.MethodGet,
					ants.URL+"/api/v1/anomalies?from="+strconv.FormatInt(epochMs, 10), nil)
				req.Header.Set("Authorization", authHeader(anTok))
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("GET /anomalies?from=...: %v", err)
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("want 200, got %d", resp.StatusCode)
				}
				calls := rec.snapshot()
				if len(calls) == 0 {
					t.Fatal("?from: QueryFlagHistory not called (flagHistoryQuerier not wired?)")
				}
				wantFrom := time.UnixMilli(epochMs)
				if !calls[0].From.Equal(wantFrom) {
					t.Errorf("From=%v, want %v", calls[0].From, wantFrom)
				} else {
					t.Logf("PASS: From=%v", calls[0].From)
				}
				// Non-overlapping-window differential (ADR-0009 §8): two disjoint
				// ranges both route through, confirming args are not silently dropped.
				rec2 := &recordingFlagHistoryQuerier{}
				ants2, anTok2, anCleanup2 := setupEnterpriseAnomalyServerWithHistory(t, rec2)
				defer anCleanup2()
				for _, qs := range []string{
					"from=1700000000000&to=1700050000000",
					"from=1700100000000&to=1700150000000",
				} {
					r2, _ := http.NewRequest(http.MethodGet, ants2.URL+"/api/v1/anomalies?"+qs, nil)
					r2.Header.Set("Authorization", authHeader(anTok2))
					rsp2, _ := http.DefaultClient.Do(r2)
					io.Copy(io.Discard, rsp2.Body)
					rsp2.Body.Close()
				}
				if calls2 := rec2.snapshot(); len(calls2) != 2 {
					t.Errorf("non-overlapping-window: want 2 querier calls, got %d", len(calls2))
				} else {
					t.Logf("PASS: non-overlapping-window differential: %d calls", len(calls2))
				}
			},
		},
		"GET /anomalies ?to": {
			// BUG-008 Group B — fixed S24/D-086 (ADR-0009 flag-event store).
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				epochMs := int64(1_700_100_000_000)
				rec := &recordingFlagHistoryQuerier{}
				ants, anTok, anCleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
				defer anCleanup()

				req, _ := http.NewRequest(http.MethodGet,
					ants.URL+"/api/v1/anomalies?to="+strconv.FormatInt(epochMs, 10), nil)
				req.Header.Set("Authorization", authHeader(anTok))
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("GET /anomalies?to=...: %v", err)
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("want 200, got %d", resp.StatusCode)
				}
				calls := rec.snapshot()
				if len(calls) == 0 {
					t.Fatal("?to: QueryFlagHistory not called")
				}
				wantTo := time.UnixMilli(epochMs)
				if !calls[0].To.Equal(wantTo) {
					t.Errorf("To=%v, want %v", calls[0].To, wantTo)
				} else {
					t.Logf("PASS: To=%v", calls[0].To)
				}
			},
		},
		"GET /anomalies ?app": {
			// BUG-008 Group A — fixed S22/D-084: handler post-filter on scope.App.
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				ants, anTok, anCleanup := setupEnterpriseAnomalyServer(t)
				defer anCleanup()
				// app=app-A → 3 items; app=ghost → 0 items (response differential).
				for _, tc := range []struct {
					app  string
					want int
				}{
					{"app-A", 3},
					{"ghost", 0},
				} {
					items, _ := anomalyItems(t, ants.URL, anTok, "app="+tc.app)
					if len(items) != tc.want {
						t.Errorf("?app=%s: items=%d want %d (filter not applied)", tc.app, len(items), tc.want)
					} else {
						t.Logf("PASS ?app=%s: %d items", tc.app, len(items))
					}
				}
			},
		},
		"GET /anomalies ?stream": {
			// BUG-008 Group A — fixed S22/D-084: handler post-filter on scope.StreamID.
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				ants, anTok, anCleanup := setupEnterpriseAnomalyServer(t)
				defer anCleanup()
				// stream=stream-1 → 2 items (one per app); stream=ghost → 0.
				for _, tc := range []struct {
					stream string
					want   int
				}{
					{"stream-1", 2},
					{"ghost", 0},
				} {
					items, _ := anomalyItems(t, ants.URL, anTok, "stream="+tc.stream)
					if len(items) != tc.want {
						t.Errorf("?stream=%s: items=%d want %d (filter not applied)", tc.stream, len(items), tc.want)
					} else {
						t.Logf("PASS ?stream=%s: %d items", tc.stream, len(items))
					}
				}
			},
		},
		"GET /anomalies ?limit": {
			// BUG-008 Group A — fixed S22/D-084: in-memory slice window.
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				ants, anTok, anCleanup := setupEnterpriseAnomalyServer(t)
				defer anCleanup()
				// limit=2 on 6 flags → 2 items + next_cursor non-empty.
				items, cursor := anomalyItems(t, ants.URL, anTok, "limit=2")
				if len(items) != 2 {
					t.Errorf("?limit=2: items=%d want 2 (limit not applied)", len(items))
				} else {
					t.Logf("PASS ?limit=2: %d items", len(items))
				}
				if cursor == "" {
					t.Errorf("?limit=2: next_cursor empty, want non-empty (more items exist)")
				} else {
					t.Logf("PASS ?limit=2: next_cursor=%q", cursor)
				}
			},
		},
		"GET /anomalies ?cursor": {
			// BUG-008 Group A — fixed S22/D-084: decimal-offset in-memory cursor.
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				ants, anTok, anCleanup := setupEnterpriseAnomalyServer(t)
				defer anCleanup()
				// Page 1 (limit=2) → 2 items + cursor. Page 2 → different items.
				items1, cursor := anomalyItems(t, ants.URL, anTok, "limit=2")
				if len(items1) != 2 {
					t.Fatalf("page1: want 2 items, got %d (limit not applied?)", len(items1))
				}
				if cursor == "" {
					t.Fatal("page1: next_cursor empty — cursor not emitted")
				}
				items2, _ := anomalyItems(t, ants.URL, anTok, "limit=2&cursor="+url.QueryEscape(cursor))
				if len(items2) == 0 {
					t.Fatal("page2: 0 items — cursor ignored?")
				}
				id1 := items1[0].(map[string]any)["id"].(string)
				id2 := items2[0].(map[string]any)["id"].(string)
				if id1 == id2 {
					t.Errorf("cursor not advancing: same id %s on both pages", id1)
				} else {
					t.Logf("PASS: page1[0].id=%s → page2[0].id=%s (cursor advanced)", id1, id2)
				}
			},
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
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/probes", bizTok, map[string]any{
						"name": fmt.Sprintf("probe-lim-%d", i), "url": "http://example.com",
						"interval_s": 30,
					})
				}
				items, nc := getListPage(t, bizTs.URL, "/api/v1/probes?limit=1", bizTok)
				if len(items) != 1 {
					t.Errorf("?limit=1: got %d items, want 1", len(items))
				} else {
					t.Logf("PASS ?limit=1: 1 item returned")
				}
				if nc == "" {
					t.Errorf("?limit=1: next_cursor empty, want non-empty")
				} else {
					t.Logf("PASS ?limit=1: next_cursor=%q", nc)
				}
			},
		},
		"GET /probes ?cursor": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/probes", bizTok, map[string]any{
						"name": fmt.Sprintf("probe-cur-%d", i), "url": "http://example.com",
						"interval_s": 30,
					})
				}
				_, nc := getListPage(t, bizTs.URL, "/api/v1/probes?limit=1", bizTok)
				if nc == "" {
					t.Fatal("?cursor: no next_cursor from page 1; need >= 2 items")
				}
				items2, _ := getListPage(t, bizTs.URL, "/api/v1/probes?limit=50&cursor="+url.QueryEscape(nc), bizTok)
				if len(items2) < 1 {
					t.Errorf("?cursor: page2 has %d items, want >= 1", len(items2))
				} else {
					t.Logf("PASS ?cursor: page2 has %d item(s)", len(items2))
				}
			},
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
			// BUG-007 fix verified S22/D-084: cursor now threaded through handleProbeResults →
			// qsvc.QueryProbeResults. Probe uses a recording fake ProbeResultQuerier
			// (wired via qsvc.SetProbeResultQuerier before server start) to assert:
			//   (a) the cursor VALUE sent in the URL arrives at the querier, and
			//   (b) the handler emits next_cursor from the fake-returned results.
			// Removes the querier wiring → querier.calls == 0 → probe fails (red captured).
			// Store-level SQL cursor (clickhouse.QueryProbeResults) is covered by
			// server/internal/query/query_conn_test.go — this probe targets handler→qsvc only.
			disp:      paramProbe,
			probeFunc: func(t *testing.T) { probeProbeResultsCursor(t) },
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
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/admin/sources", bizTok, map[string]any{
						"name": fmt.Sprintf("src-lim-%d", i), "type": "rest",
					})
				}
				items, nc := getListPage(t, bizTs.URL, "/api/v1/admin/sources?limit=1", bizTok)
				if len(items) != 1 {
					t.Errorf("?limit=1: got %d items, want 1", len(items))
				} else {
					t.Logf("PASS ?limit=1: 1 item returned")
				}
				if nc == "" {
					t.Errorf("?limit=1: next_cursor empty, want non-empty")
				} else {
					t.Logf("PASS ?limit=1: next_cursor=%q", nc)
				}
			},
		},
		"GET /admin/sources ?cursor": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/admin/sources", bizTok, map[string]any{
						"name": fmt.Sprintf("src-cur-%d", i), "type": "rest",
					})
				}
				_, nc := getListPage(t, bizTs.URL, "/api/v1/admin/sources?limit=1", bizTok)
				if nc == "" {
					t.Fatal("?cursor: no next_cursor from page 1; need >= 2 items")
				}
				items2, _ := getListPage(t, bizTs.URL, "/api/v1/admin/sources?limit=50&cursor="+url.QueryEscape(nc), bizTok)
				if len(items2) < 1 {
					t.Errorf("?cursor: page2 has %d items, want >= 1", len(items2))
				} else {
					t.Logf("PASS ?cursor: page2 has %d item(s)", len(items2))
				}
			},
		},

		// ── GET /admin/tokens ────────────────────────────────────────────────────
		// bizTs is pre-seeded with 1 api token (kind=api, CreatedAt=1000).
		// Tokens list in DESC order; new tokens (higher CreatedAt) appear first.
		"GET /admin/tokens ?limit": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				// Create 1 extra token so we have at least 2 (pre-seeded + new).
				postConformanceItem(t, bizTs.URL, "/api/v1/admin/tokens", bizTok, map[string]any{
					"kind": "api", "name": "tok-lim-extra", "scopes": []string{"read"},
				})
				items, nc := getListPage(t, bizTs.URL, "/api/v1/admin/tokens?limit=1", bizTok)
				if len(items) != 1 {
					t.Errorf("?limit=1: got %d items, want 1", len(items))
				} else {
					t.Logf("PASS ?limit=1: 1 item returned")
				}
				if nc == "" {
					t.Errorf("?limit=1: next_cursor empty, want non-empty")
				} else {
					t.Logf("PASS ?limit=1: next_cursor=%q", nc)
				}
			},
		},
		"GET /admin/tokens ?cursor": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				// Create 1 extra token; combined with pre-seeded and any ?limit probe tokens we have >= 2.
				postConformanceItem(t, bizTs.URL, "/api/v1/admin/tokens", bizTok, map[string]any{
					"kind": "api", "name": "tok-cur-extra", "scopes": []string{"read"},
				})
				_, nc := getListPage(t, bizTs.URL, "/api/v1/admin/tokens?limit=1", bizTok)
				if nc == "" {
					t.Fatal("?cursor: no next_cursor from page 1; need >= 2 tokens")
				}
				items2, _ := getListPage(t, bizTs.URL, "/api/v1/admin/tokens?limit=50&cursor="+url.QueryEscape(nc), bizTok)
				if len(items2) < 1 {
					t.Errorf("?cursor: page2 has %d items, want >= 1", len(items2))
				} else {
					t.Logf("PASS ?cursor: page2 has %d item(s)", len(items2))
				}
			},
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
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/admin/users", bizTok, map[string]any{
						"username": fmt.Sprintf("user-lim-%d", i), "role": "viewer", "password": "x",
					})
				}
				items, nc := getListPage(t, bizTs.URL, "/api/v1/admin/users?limit=1", bizTok)
				if len(items) != 1 {
					t.Errorf("?limit=1: got %d items, want 1", len(items))
				} else {
					t.Logf("PASS ?limit=1: 1 item returned")
				}
				if nc == "" {
					t.Errorf("?limit=1: next_cursor empty, want non-empty")
				} else {
					t.Logf("PASS ?limit=1: next_cursor=%q", nc)
				}
			},
		},
		"GET /admin/users ?cursor": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/admin/users", bizTok, map[string]any{
						"username": fmt.Sprintf("user-cur-%d", i), "role": "viewer", "password": "x",
					})
				}
				_, nc := getListPage(t, bizTs.URL, "/api/v1/admin/users?limit=1", bizTok)
				if nc == "" {
					t.Fatal("?cursor: no next_cursor from page 1; need >= 2 users")
				}
				items2, _ := getListPage(t, bizTs.URL, "/api/v1/admin/users?limit=50&cursor="+url.QueryEscape(nc), bizTok)
				if len(items2) < 1 {
					t.Errorf("?cursor: page2 has %d items, want >= 1", len(items2))
				} else {
					t.Logf("PASS ?cursor: page2 has %d item(s)", len(items2))
				}
			},
		},

		// ── GET /admin/tenants ───────────────────────────────────────────────────
		"GET /admin/tenants ?limit": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/admin/tenants", bizTok, map[string]any{
						"name": fmt.Sprintf("tenant-lim-%d", i), "stream_pattern": fmt.Sprintf("lim%d/*", i),
					})
				}
				items, nc := getListPage(t, bizTs.URL, "/api/v1/admin/tenants?limit=1", bizTok)
				if len(items) != 1 {
					t.Errorf("?limit=1: got %d items, want 1", len(items))
				} else {
					t.Logf("PASS ?limit=1: 1 item returned")
				}
				if nc == "" {
					t.Errorf("?limit=1: next_cursor empty, want non-empty")
				} else {
					t.Logf("PASS ?limit=1: next_cursor=%q", nc)
				}
			},
		},
		"GET /admin/tenants ?cursor": {
			disp: paramProbe,
			probeFunc: func(t *testing.T) {
				t.Helper()
				for i := 1; i <= 2; i++ {
					postConformanceItem(t, bizTs.URL, "/api/v1/admin/tenants", bizTok, map[string]any{
						"name": fmt.Sprintf("tenant-cur-%d", i), "stream_pattern": fmt.Sprintf("cur%d/*", i),
					})
				}
				_, nc := getListPage(t, bizTs.URL, "/api/v1/admin/tenants?limit=1", bizTok)
				if nc == "" {
					t.Fatal("?cursor: no next_cursor from page 1; need >= 2 tenants")
				}
				items2, _ := getListPage(t, bizTs.URL, "/api/v1/admin/tenants?limit=50&cursor="+url.QueryEscape(nc), bizTok)
				if len(items2) < 1 {
					t.Errorf("?cursor: page2 has %d items, want >= 1", len(items2))
				} else {
					t.Logf("PASS ?cursor: page2 has %d item(s)", len(items2))
				}
			},
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
	//     time (S21/D-083); bumped to 86 when BUG-010 added ?format to audience
	//     (S22/D-084). If kin-openapi ever silently under-enumerates
	//     (e.g. a $ref refactor that doc.Validate does not catch), the gate
	//     must go loud instead of vacuously passing. Lower this constant only
	//     for an intentional spec shrink.
	const minSpecParams = 86
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
	// minProbes census (S24/D-086 update):
	//   29 probes at S21 baseline
	//  + 4 BUG-008 Group A (anomalies ?app/?stream/?limit/?cursor) added by F2 (S22/D-084)
	//  + 2 BUG-007 cursor probes (alerts/history ?cursor, probes/results ?cursor) added by F3 (S22/D-084)
	//  + 2 BUG-008 Group B (anomalies ?from/?to) promoted from known-violation S24/D-086
	//  = 37 total probes. Floor = 37 - 2 = 35.
	const minProbes = 35
	if probesRan < minProbes {
		t.Errorf("param-conformance: only %d probe(s) ran (need >= %d); "+
			"check that probe entries are not all skipping", probesRan, minProbes)
	} else {
		t.Logf("param-conformance: %d probe(s) ran, gate satisfied", probesRan)
	}
}

// NOTE: This file reuses setupBusinessServer (v3b_guard_test.go), setupHealthyTestServer
// and setupIngestCaptureServer (vd20b_vd21_ingest_test.go), captureIngestQsvc, and
// authHeader (api_test.go) — all in the same package api_test.

// ─── Pagination probe helpers ─────────────────────────────────────────────────

// postConformanceItem sends an authenticated JSON POST to the given path and
// fatals if the response is not 2xx.  Used by BUG-006 pagination probes to seed
// items without boilerplate in every closure.
func postConformanceItem(t *testing.T, baseURL, path, tok string, body map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(tok))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("postConformanceItem POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		rb, _ := io.ReadAll(resp.Body)
		t.Fatalf("postConformanceItem POST %s: got %d: %s", path, resp.StatusCode, rb)
	}
	io.Copy(io.Discard, resp.Body)
}

// getListPage fetches an authenticated list page and returns (items, next_cursor).
// Fatals on network or non-200 response.
func getListPage(t *testing.T, baseURL, path, tok string) (items []any, nextCursor string) {
	t.Helper()
	// Ensure path starts with a slash so string operations below are predictable.
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	req, _ := http.NewRequest(http.MethodGet, baseURL+path, nil)
	req.Header.Set("Authorization", authHeader(tok))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("getListPage GET %s: %v", path, err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("getListPage GET %s: got %d: %s", path, resp.StatusCode, b)
	}
	var result map[string]any
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("getListPage GET %s: decode: %v", path, err)
	}
	items, _ = result["items"].([]any)
	meta2, _ := result["meta"].(map[string]any)
	nextCursor, _ = meta2["next_cursor"].(string)
	return items, nextCursor
}
