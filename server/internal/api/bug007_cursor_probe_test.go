// Package api_test — TDD probes for BUG-007 cursor threading (S22 / F3 / D-084).
//
// BUG-007: GET /alerts/history and GET /probes/{probeId}/results both declared
// ?cursor in the OpenAPI spec but silently dropped the value before passing to
// the store / query-service layer. Callers could never page past the first limit
// results.
//
// This file provides:
//  1. recordingProbeResultQuerier — captures the cursor arg at the qsvc boundary.
//  2. setupProbeResultsCursorServer — Business-tier server with recording querier wired.
//  3. probeAlertHistoryCursor — probe body for GET /alerts/history ?cursor.
//  4. probeProbeResultsCursor — probe body for GET /probes/{probeId}/results ?cursor.
//  5. Standalone TestBUG007_* tests so the TDD red → green cycle is independently
//     reproducible without running the full conformance gate.
//
// TDD discipline (per WO-C / F3 instructions):
//
//	RED  — probeProbeResultsCursor: remove qsvc.SetProbeResultQuerier(querier)
//	        from setupProbeResultsCursorServer → querier.calls == 0 → gate fails.
//	        probeAlertHistoryCursor: before BUG-007 fix, handleAlertHistory did not
//	        call q.Get("cursor"); page 2 returned same first-page item → id1 == id2.
//	GREEN — both fixes committed in WO-C / S22 / D-084; both probes pass here.
package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// ─── Recording fake ──────────────────────────────────────────────────────────

// recordingProbeResultQuerier captures the cursor argument from each
// QueryProbeResults call and returns a fixed slice of results. It is safe for
// concurrent use (the server runs in a goroutine).
//
// Store-level SQL cursor handling for QueryProbeResults is already covered by
// the ClickHouse integration tests in server/internal/query/query_conn_test.go.
// This fake targets the handler→qsvc boundary only.
type recordingProbeResultQuerier struct {
	mu             sync.Mutex
	capturedCursor string
	calls          int
	// returnResults is returned on every call. Set to limit+1 items to trigger
	// next_cursor emission from handleProbeResults.
	returnResults []domain.ProbeResult
}

func (r *recordingProbeResultQuerier) QueryProbeResults(
	_ context.Context, _ string, _, _ time.Time, _ int, cursor string,
) ([]domain.ProbeResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capturedCursor = cursor
	r.calls++
	out := make([]domain.ProbeResult, len(r.returnResults))
	copy(out, r.returnResults)
	return out, nil
}

// ─── Server setup ─────────────────────────────────────────────────────────────

// setupProbeResultsCursorServer builds a Business-tier httptest.Server with the
// given recording querier wired into qsvc, and a probe row pre-created in the
// meta store so the handler's GetProbe existence-check succeeds.
//
// Returns (ts, token, probeID, cleanup).
func setupProbeResultsCursorServer(t *testing.T, querier *recordingProbeResultQuerier) (
	ts *httptest.Server, tok string, probeID string, cleanup func(),
) {
	t.Helper()
	licKey, licCleanup := makeTestBusinessLicense(t)

	ctx := context.Background()
	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		licCleanup()
		t.Fatalf("meta DDL not found (D-028 repo-root mount required): %v", err)
	}
	ms, err := meta.New(ctx, "sqlite", ":memory:", "bug007-probe-secret")
	if err != nil {
		licCleanup()
		t.Fatalf("meta.New: %v", err)
	}
	if err := ms.MigrateEmbedded(ctx, string(ddl)); err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("migrate: %v", err)
	}

	tok = "plt_bug007_probe_cursor_test"
	if err := ms.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "bug007-probe-admin",
		TokenHash: hashToken(tok),
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("CreateToken: %v", err)
	}

	lic, err := license.New(licKey, "")
	if err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("license.New: %v", err)
	}

	// Pre-create a probe row so the handler's GetProbe existence-check passes.
	probeRow, err := ms.CreateProbe(ctx, meta.ProbeRow{
		Name: "bug007-cursor-test-probe", URL: "http://example.com/stream.m3u8",
		Protocol: "hls", IntervalS: 30, TimeoutS: 10, Enabled: true,
	})
	if err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("CreateProbe: %v", err)
	}
	probeID = probeRow.ID

	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)
	// Wire the recording fake BEFORE building the server.
	// TDD RED (pre-BUG-007-fix): without cursor threading in handleProbeResults,
	// querier.capturedCursor == "" regardless of the URL param; also demonstrated
	// by removing this line → querier.calls == 0 → gate fails (red captured above).
	// TDD GREEN: cursor is now read and passed → querier.capturedCursor matches URL.
	qsvc.SetProbeResultQuerier(querier)

	srv := api.New(api.Config{ListenAddr: ":0"}, ms, live, qsvc, lic, nil)
	ts = httptest.NewServer(srv.Handler())

	cleanup = func() {
		ts.Close()
		ms.Close()
		licCleanup()
	}
	return ts, tok, probeID, cleanup
}

// ─── Probe bodies ─────────────────────────────────────────────────────────────

// probeAlertHistoryCursor is the probe body for "GET /alerts/history ?cursor".
//
// Strategy: seed >= 3 alert_history rows via the meta store (rows ordered DESC
// by ts). Page 1 (limit=1) returns the highest-ts row and a non-nil next_cursor.
// Page 2 passes that cursor and must return a different row — which proves that
// the handler reads cursor and passes it to store.ListAlertHistory (keyset
// pagination). If cursor is dropped, page 2 repeats page 1 → id1 == id2 → fail.
//
// TDD RED  (pre-BUG-007-fix): handleAlertHistory did not call q.Get("cursor");
// store.ListAlertHistory had no cursor parameter; page 2 always equalled page 1.
// TDD GREEN (S22/D-084): cursor is read and threaded through; keyset pagination works.
func probeAlertHistoryCursor(t *testing.T) {
	t.Helper()

	histTS, histTok, histStore, cleanup := setupAlertHistoryServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create a rule (foreign-key requirement for alert_history rows).
	rule, err := histStore.CreateAlertRule(ctx, meta.AlertRuleRow{
		Name: "ah-cursor-probe-rule", Metric: "bitrate", Operator: "lt",
		Threshold: 1, WindowS: 5, ScopeJSON: "{}", Severity: "warning",
		CooldownS: 300, Enabled: true, Muted: false,
		MaintenanceWindows: "[]", ChannelIDs: "[]",
	})
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	// Seed 3 rows with current timestamps (parseTimeRange defaults from=now-7d,
	// so epoch-ms values like 1000/2000/3000 predate the filter window).
	// Rows are 3 s, 2 s, 1 s before now; ListAlertHistory orders DESC by ts:
	//   page 1: most-recent row (delta= -1s);
	//   page 2 (with cursor): next row (delta= -2s).
	baseTS := time.Now().UnixMilli()
	for i, delta := range []int64{-3000, -2000, -1000} {
		rowTS := baseTS + delta
		if err := histStore.CreateAlertHistory(ctx, meta.AlertHistoryRow{
			AlertID:   fmt.Sprintf("ah-cursor-probe-alert-%d", i+1),
			RuleID:    rule.ID,
			State:     "firing",
			Severity:  "warning",
			TS:        rowTS,
			Metric:    "bitrate",
			Value:     0.5,
			Threshold: 1.0,
			ScopeJSON: "{}",
		}); err != nil {
			t.Fatalf("CreateAlertHistory[delta=%d]: %v", delta, err)
		}
	}

	// Page 1: limit=1, no cursor → 1 item + non-nil next_cursor.
	p1Items, nc := getListPage(t, histTS.URL, "/api/v1/alerts/history?limit=1", histTok)
	if len(p1Items) != 1 {
		t.Fatalf("page1: want 1 item, got %d", len(p1Items))
	}
	if nc == "" {
		t.Fatal("page1: next_cursor empty — handler did not emit cursor (3 rows seeded, limit=1)")
	}
	t.Logf("page1: 1 item, next_cursor=%q", nc)

	// Page 2: send cursor from page 1 → must return a DIFFERENT item.
	p2Items, _ := getListPage(t, histTS.URL,
		"/api/v1/alerts/history?limit=1&cursor="+url.QueryEscape(nc), histTok)
	if len(p2Items) == 0 {
		t.Fatal("page2: 0 items — cursor may be ignored (handler not threading cursor to store)")
	}
	id1 := p1Items[0].(map[string]any)["id"].(string)
	id2 := p2Items[0].(map[string]any)["id"].(string)
	if id1 == id2 {
		t.Errorf("cursor not advancing: page1 id=%s == page2 id=%s "+
			"(cursor dropped before store.ListAlertHistory)", id1, id2)
	} else {
		t.Logf("PASS: page1 id=%s → page2 id=%s (cursor advanced, different rows)", id1, id2)
	}
}

// probeProbeResultsCursor is the probe body for "GET /probes/{probeId}/results ?cursor".
//
// Strategy: inject a recording fake ProbeResultQuerier via qsvc.SetProbeResultQuerier
// so that the cursor value passed by the URL is observable at the service boundary.
// The fake returns limit+1 results so the handler emits next_cursor.
// Two assertions:
//
//	(a) capturedCursor == sentCursor — cursor value arrived at the querier.
//	(b) response next_cursor is non-nil — handler emits cursor from returned results.
//
// Note: Store-level SQL cursor handling (clickhouse.QueryProbeResults) is covered
// by server/internal/query/query_conn_test.go — this probe targets handler→qsvc only.
//
// TDD RED  (pre-BUG-007-fix): handleProbeResults did not call r.URL.Query().Get("cursor");
// qsvc.QueryProbeResults had no cursor parameter → capturedCursor == "" → (a) fails.
// TDD GREEN (S22/D-084): cursor read + passed through → capturedCursor == sentCursor.
func probeProbeResultsCursor(t *testing.T) {
	t.Helper()

	const sentCursor = "test-cursor-probe-xyz"

	// The fake returns 2 results for a request with ?limit=1.
	// handleProbeResults calls QueryProbeResults(ctx, id, from, to, limit+1=2, cursor).
	// len(2) > limit(1) → handler sets nextCursor from the last result.
	now := time.Now()
	querier := &recordingProbeResultQuerier{
		returnResults: []domain.ProbeResult{
			{ID: "pr-res-001", ProbeID: "placeholder", TS: now.Add(-2 * time.Second), Success: true},
			{ID: "pr-res-002", ProbeID: "placeholder", TS: now.Add(-1 * time.Second), Success: true},
		},
	}

	ts, tok, probeID, cleanup := setupProbeResultsCursorServer(t, querier)
	defer cleanup()

	u := ts.URL + "/api/v1/probes/" + probeID + "/results?limit=1&cursor=" + url.QueryEscape(sentCursor)
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Authorization", authHeader(tok))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET probe results: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, body)
	}

	querier.mu.Lock()
	capturedCursor := querier.capturedCursor
	calls := querier.calls
	querier.mu.Unlock()

	// (a) Assert the cursor VALUE arrived at the querier.
	if calls == 0 {
		t.Fatal("querier.QueryProbeResults never called — handler did not reach qsvc")
	}
	if capturedCursor != sentCursor {
		t.Errorf("FAIL (a): cursor not threaded: querier received %q, want %q "+
			"(handler not passing cursor to qsvc.QueryProbeResults)", capturedCursor, sentCursor)
	} else {
		t.Logf("PASS (a): cursor=%q arrived at querier (%d call(s))", capturedCursor, calls)
	}

	// (b) Assert the handler emits next_cursor from the fake-returned results.
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	metaBlock, _ := result["meta"].(map[string]any)
	nextCursor, _ := metaBlock["next_cursor"].(string)
	if nextCursor == "" {
		t.Errorf("FAIL (b): next_cursor empty — handler did not emit cursor " +
			"from 2 fake results with limit=1 (len(2)>limit(1) should set next_cursor)")
	} else {
		t.Logf("PASS (b): next_cursor=%q emitted by handler", nextCursor)
	}
}

// ─── Standalone TDD tests ─────────────────────────────────────────────────────

// TestBUG007_AlertHistory_CursorProbe is a standalone TDD pin for the
// GET /alerts/history ?cursor probe. Also run as a conformance registry subtest.
//
// RED  (pre-BUG-007-fix): cursor not threaded → page 2 repeats page 1.
// GREEN (S22/D-084): cursor read + passed to store → distinct page 2.
func TestBUG007_AlertHistory_CursorProbe(t *testing.T) {
	probeAlertHistoryCursor(t)
}

// TestBUG007_ProbeResults_CursorProbe is a standalone TDD pin for the
// GET /probes/{probeId}/results ?cursor probe. Also run as a conformance registry subtest.
//
// RED  (pre-BUG-007-fix): cursor not threaded → capturedCursor == "" → (a) fails.
// GREEN (S22/D-084): cursor read + passed to qsvc → capturedCursor matches URL param.
func TestBUG007_ProbeResults_CursorProbe(t *testing.T) {
	probeProbeResultsCursor(t)
}
