// Package api_test — TDD pins for BUG-008 Group A (S22 / D-084).
//
// Tests cover the four handler-only fixes:
//   - ?app    — filter by scope.App
//   - ?stream — filter by scope.StreamID
//   - ?limit  — in-memory slice window (default 50, max 500)
//   - ?cursor — decimal-offset opaque cursor; invalid cursor → first page
//
// ?from and ?to remain known-violation (no 501 guard in S22; see triage doc §3).
//
// TDD discipline: all tests in this file were RED before the handleAnomalies
// changes in wave3.go. GREEN evidence captured after Group A fix.
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
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Fake anomaly detector ────────────────────────────────────────────────────

// fakeAnomalyDetector is a deterministic api.AnomalyDetector for BUG-008 tests.
// It returns a fixed set of flags regardless of sigmaThreshold.
type fakeAnomalyDetector struct {
	flags []api.AnomalyFlagAPI
}

func (f *fakeAnomalyDetector) ComputeFlags(_ context.Context, _ float64) ([]api.AnomalyFlagAPI, error) {
	// Return a copy so handler mutations (sort) don't affect the fixture.
	out := make([]api.AnomalyFlagAPI, len(f.flags))
	copy(out, f.flags)
	return out, nil
}

// stdFakeFlags returns 6 deterministic AnomalyFlagAPI values across 2 apps
// and 3 stream IDs with strictly increasing timestamps for stable sort order.
//
// Layout:
//
//	app-A: stream-1 (ts=1000), stream-2 (ts=2000), stream-3 (ts=3000)
//	app-B: stream-1 (ts=4000), stream-2 (ts=5000), stream-3 (ts=6000)
func stdFakeFlags() []api.AnomalyFlagAPI {
	return []api.AnomalyFlagAPI{
		{ID: "flag-001", Metric: "viewers", Scope: domain.AlertScope{App: "app-A", StreamID: "stream-1"}, Observed: 10, Expected: 5, Sigma: 3.0, TS: 1000},
		{ID: "flag-002", Metric: "viewers", Scope: domain.AlertScope{App: "app-A", StreamID: "stream-2"}, Observed: 10, Expected: 5, Sigma: 3.0, TS: 2000},
		{ID: "flag-003", Metric: "viewers", Scope: domain.AlertScope{App: "app-A", StreamID: "stream-3"}, Observed: 10, Expected: 5, Sigma: 3.0, TS: 3000},
		{ID: "flag-004", Metric: "viewers", Scope: domain.AlertScope{App: "app-B", StreamID: "stream-1"}, Observed: 10, Expected: 5, Sigma: 3.0, TS: 4000},
		{ID: "flag-005", Metric: "viewers", Scope: domain.AlertScope{App: "app-B", StreamID: "stream-2"}, Observed: 10, Expected: 5, Sigma: 3.0, TS: 5000},
		{ID: "flag-006", Metric: "viewers", Scope: domain.AlertScope{App: "app-B", StreamID: "stream-3"}, Observed: 10, Expected: 5, Sigma: 3.0, TS: 6000},
	}
}

// setupEnterpriseAnomalyServer creates an Enterprise httptest.Server with a
// fakeAnomalyDetector returning stdFakeFlags(). Used by BUG-008 filter and
// pagination tests and by the conformance registry probes.
func setupEnterpriseAnomalyServer(t *testing.T) (ts *httptest.Server, tok string, cleanup func()) {
	t.Helper()
	licKey, licCleanup := makeTestEnterpriseLicense(t)

	ctx := context.Background()
	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		licCleanup()
		t.Fatalf("meta DDL not found (D-028 repo-root mount required): %v", err)
	}
	ms, err := meta.New(ctx, "sqlite", ":memory:", "bug008-test-secret")
	if err != nil {
		licCleanup()
		t.Fatalf("meta.New: %v", err)
	}
	if err := ms.MigrateEmbedded(ctx, string(ddl)); err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("migrate: %v", err)
	}

	tok = "plt_bug008_anomaly_test"
	tokenHash := hashToken(tok)
	if err := ms.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "bug008-admin",
		TokenHash: tokenHash,
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
	if lic.Tier() != license.TierEnterprise {
		ms.Close()
		licCleanup()
		t.Fatalf("expected enterprise tier, got %q", lic.Tier())
	}

	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)
	srv := api.New(api.Config{ListenAddr: ":0"}, ms, live, qsvc, lic, nil)
	srv.SetAnomalyDetector(&fakeAnomalyDetector{flags: stdFakeFlags()})
	ts = httptest.NewServer(srv.Handler())

	cleanup = func() {
		ts.Close()
		ms.Close()
		licCleanup()
	}
	return ts, tok, cleanup
}

// anomalyItems fetches GET /anomalies with rawQuery appended and returns
// (items, nextCursor). Fatals on network error or non-200 status.
func anomalyItems(t *testing.T, baseURL, tok, rawQuery string) (items []any, nextCursor string) {
	t.Helper()
	u := baseURL + "/api/v1/anomalies"
	if rawQuery != "" {
		u += "?" + rawQuery
	}
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("Authorization", authHeader(tok))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET anomalies?%s: %v", rawQuery, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET anomalies?%s: want 200, got %d: %s", rawQuery, resp.StatusCode, body)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode GET anomalies?%s: %v", rawQuery, err)
	}
	items, _ = result["items"].([]any)
	metaBlock, _ := result["meta"].(map[string]any)
	nextCursor, _ = metaBlock["next_cursor"].(string)
	return items, nextCursor
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestAnomalies_BUG008_AppFilter verifies that ?app filters flags by scope.App.
//
// RED: handler ignores ?app, returns all 6 flags regardless of value.
// GREEN: ?app=app-A → 3 items; ?app=app-B → 3 items; ?app=ghost → 0 items.
func TestAnomalies_BUG008_AppFilter(t *testing.T) {
	ts, tok, cleanup := setupEnterpriseAnomalyServer(t)
	defer cleanup()

	for _, tc := range []struct {
		app  string
		want int
	}{
		{"app-A", 3},
		{"app-B", 3},
		{"ghost", 0},
	} {
		items, _ := anomalyItems(t, ts.URL, tok, "app="+tc.app)
		if len(items) != tc.want {
			t.Errorf("BUG-008 ?app=%s: items len=%d, want %d (filter not applied)", tc.app, len(items), tc.want)
		} else {
			t.Logf("PASS ?app=%s: %d items", tc.app, len(items))
		}
	}
}

// TestAnomalies_BUG008_StreamFilter verifies that ?stream filters by scope.StreamID.
//
// RED: handler ignores ?stream, returns all 6 flags.
// GREEN: ?stream=stream-1 → 2 (one per app); ?stream=ghost → 0.
func TestAnomalies_BUG008_StreamFilter(t *testing.T) {
	ts, tok, cleanup := setupEnterpriseAnomalyServer(t)
	defer cleanup()

	for _, tc := range []struct {
		stream string
		want   int
	}{
		{"stream-1", 2}, // one per app (flag-001 + flag-004)
		{"stream-2", 2},
		{"ghost", 0},
	} {
		items, _ := anomalyItems(t, ts.URL, tok, "stream="+tc.stream)
		if len(items) != tc.want {
			t.Errorf("BUG-008 ?stream=%s: items len=%d, want %d (filter not applied)", tc.stream, len(items), tc.want)
		} else {
			t.Logf("PASS ?stream=%s: %d items", tc.stream, len(items))
		}
	}
}

// TestAnomalies_BUG008_LimitTruncation verifies slice-window pagination.
//
// RED: handler ignores ?limit, returns all 6 flags with next_cursor=nil.
// GREEN: ?limit=2 → 2 items + next_cursor; ?limit=10 → 6 items + no cursor.
func TestAnomalies_BUG008_LimitTruncation(t *testing.T) {
	ts, tok, cleanup := setupEnterpriseAnomalyServer(t)
	defer cleanup()

	for _, tc := range []struct {
		limit     int
		wantItems int
		wantMore  bool // next_cursor should be non-empty
	}{
		{2, 2, true},   // 6 flags → page of 2 with cursor
		{3, 3, true},   // 6 flags → page of 3 with cursor
		{10, 6, false}, // limit > total → all items, no cursor
	} {
		qs := fmt.Sprintf("limit=%d", tc.limit)
		items, cursor := anomalyItems(t, ts.URL, tok, qs)
		if len(items) != tc.wantItems {
			t.Errorf("BUG-008 ?%s: items len=%d, want %d", qs, len(items), tc.wantItems)
		} else {
			t.Logf("PASS ?%s: %d items", qs, len(items))
		}
		if tc.wantMore && cursor == "" {
			t.Errorf("BUG-008 ?%s: next_cursor empty, want non-empty (more items exist)", qs)
		} else if !tc.wantMore && cursor != "" {
			t.Errorf("BUG-008 ?%s: next_cursor=%q, want empty (all items on page)", qs, cursor)
		} else {
			t.Logf("PASS ?%s: next_cursor=%q (wantMore=%v)", qs, cursor, tc.wantMore)
		}
	}
}

// TestAnomalies_BUG008_CursorAdvances verifies that the cursor paginates to
// a distinct second page.
//
// RED: handler ignores cursor; page 1 and page 2 return the same items.
// GREEN: page 1 has flag-001,flag-002; page 2 (via cursor) has flag-003,flag-004.
func TestAnomalies_BUG008_CursorAdvances(t *testing.T) {
	ts, tok, cleanup := setupEnterpriseAnomalyServer(t)
	defer cleanup()

	// Page 1: limit=2, no cursor.
	items1, cursor := anomalyItems(t, ts.URL, tok, "limit=2")
	if len(items1) != 2 {
		t.Fatalf("page1: want 2 items, got %d (limit not applied?)", len(items1))
	}
	if cursor == "" {
		t.Fatal("page1: next_cursor empty — cursor not emitted despite more items remaining")
	}
	t.Logf("page1: %d items, cursor=%q", len(items1), cursor)

	// Page 2: same limit, cursor from page 1.
	items2, _ := anomalyItems(t, ts.URL, tok, "limit=2&cursor="+url.QueryEscape(cursor))
	if len(items2) == 0 {
		t.Fatal("page2: 0 items — cursor ignored or all items on page 1?")
	}

	// The first item on each page must differ (cursor is advancing).
	id1 := items1[0].(map[string]any)["id"].(string)
	id2 := items2[0].(map[string]any)["id"].(string)
	if id1 == id2 {
		t.Errorf("BUG-008 cursor: page1[0].id=%s == page2[0].id=%s — cursor not advancing", id1, id2)
	} else {
		t.Logf("PASS: page1[0].id=%s, page2[0].id=%s (cursor advanced)", id1, id2)
	}
}

// TestAnomalies_BUG008_InvalidCursor_FirstPage verifies that a non-parseable
// cursor falls back to first page (offset 0).
//
// RED: handler ignores cursor; returns all 6 items regardless.
// GREEN: ?limit=2&cursor=invalid-xyz → 2 items (offset=0) + next_cursor set.
func TestAnomalies_BUG008_InvalidCursor_FirstPage(t *testing.T) {
	ts, tok, cleanup := setupEnterpriseAnomalyServer(t)
	defer cleanup()

	items, cursor := anomalyItems(t, ts.URL, tok, "limit=2&cursor=invalid-xyz")
	if len(items) != 2 {
		t.Errorf("BUG-008 invalid cursor: want 2 items (first page), got %d", len(items))
	} else {
		t.Logf("PASS: invalid cursor → first page: %d items", len(items))
	}
	if cursor == "" {
		t.Errorf("BUG-008 invalid cursor: next_cursor empty, want non-empty (more items remain)")
	} else {
		t.Logf("PASS: next_cursor=%q after invalid cursor input", cursor)
	}
}

// TestAnomalies_BUG008_NoFilter_AllFlags is a baseline sanity check:
// no filter params must return all 6 flags. Default limit=50 fits all 6.
//
// This verifies the fake detector is wired correctly and that unfiltered
// behavior is preserved after the Group A fix.
func TestAnomalies_BUG008_NoFilter_AllFlags(t *testing.T) {
	ts, tok, cleanup := setupEnterpriseAnomalyServer(t)
	defer cleanup()

	items, cursor := anomalyItems(t, ts.URL, tok, "")
	if len(items) != 6 {
		t.Errorf("no filter: want 6 items (all flags), got %d", len(items))
	} else {
		t.Logf("PASS: no filter → %d items", len(items))
	}
	if cursor != "" {
		t.Errorf("no filter: next_cursor=%q, want empty (6 items < default limit 50)", cursor)
	} else {
		t.Logf("PASS: no filter → next_cursor empty (6 items < default limit 50)")
	}
}

// TestAnomalies_BUG008_AppAndStreamIntersect verifies combined app+stream filter.
//
// RED: both filters ignored → all 6 items returned.
// GREEN: ?app=app-A&stream=stream-1 → exactly 1 item (flag-001).
func TestAnomalies_BUG008_AppAndStreamIntersect(t *testing.T) {
	ts, tok, cleanup := setupEnterpriseAnomalyServer(t)
	defer cleanup()

	items, _ := anomalyItems(t, ts.URL, tok, "app=app-A&stream=stream-1")
	if len(items) != 1 {
		t.Errorf("?app=app-A&stream=stream-1: want 1 item, got %d", len(items))
		return
	}
	id := items[0].(map[string]any)["id"].(string)
	if id != "flag-001" {
		t.Errorf("expected id=flag-001, got %q", id)
	} else {
		t.Logf("PASS: ?app=app-A&stream=stream-1 → 1 item (id=%s)", id)
	}
}
