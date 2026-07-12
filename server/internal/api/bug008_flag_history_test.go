// Package api_test — TDD pins for BUG-008 Group B / ADR-0009 flag-history routing (S24 / D-086).
//
// Tests cover the GET /anomalies ?from/?to routing branch introduced by
// handleAnomalies (wave3.go): directing time-range queries to FlagHistoryQuerier
// instead of the point-in-time ComputeFlags path.
//
// ADR-0009 §6 specifies the routing rules and cursor namespace.
// ADR AMENDMENT (D-086): QueryFlagHistory carries metric + minSigma.
//
// TDD discipline: all tests in this file were RED before the handleAnomalies
// routing changes in wave3.go. RED evidence captured pre-implementation.
package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Recording double ─────────────────────────────────────────────────────────

// flagHistoryCall captures all 9 arguments received by QueryFlagHistory.
type flagHistoryCall struct {
	From, To time.Time
	Metric   string
	App      string
	Stream   string
	MinSigma float64
	Limit    int
	Cursor   string
}

// recordingFlagHistoryQuerier is a test double that records every
// QueryFlagHistory call. It returns r.page (or r.err if non-nil).
// Thread-safe to allow use inside HTTP handlers.
type recordingFlagHistoryQuerier struct {
	mu    sync.Mutex
	calls []flagHistoryCall
	page  api.FlagHistoryPage // what to return on success
	err   error               // if non-nil, returned instead of page
}

func (r *recordingFlagHistoryQuerier) QueryFlagHistory(
	_ context.Context,
	from, to time.Time,
	metric, app, stream string,
	minSigma float64,
	limit int,
	cursor string,
) (api.FlagHistoryPage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, flagHistoryCall{
		From:     from,
		To:       to,
		Metric:   metric,
		App:      app,
		Stream:   stream,
		MinSigma: minSigma,
		Limit:    limit,
		Cursor:   cursor,
	})
	if r.err != nil {
		return api.FlagHistoryPage{}, r.err
	}
	return r.page, nil
}

func (r *recordingFlagHistoryQuerier) snapshot() []flagHistoryCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]flagHistoryCall, len(r.calls))
	copy(out, r.calls)
	return out
}

// ─── Server setup ─────────────────────────────────────────────────────────────

// setupEnterpriseAnomalyServerWithHistory creates an Enterprise httptest.Server
// with a fakeAnomalyDetector (stdFakeFlags) AND the supplied recordingFlagHistoryQuerier
// wired via SetFlagHistoryQuerier. Pass rec=nil to leave flagHistoryQuerier unset.
func setupEnterpriseAnomalyServerWithHistory(t *testing.T, rec *recordingFlagHistoryQuerier) (ts *httptest.Server, tok string, cleanup func()) {
	t.Helper()
	licKey, licCleanup := makeTestEnterpriseLicense(t)

	ctx := context.Background()
	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		licCleanup()
		t.Fatalf("meta DDL not found (D-028 repo-root mount required): %v", err)
	}
	ms, err := meta.New(ctx, "sqlite", ":memory:", "bug009-test-secret")
	if err != nil {
		licCleanup()
		t.Fatalf("meta.New: %v", err)
	}
	if err := ms.MigrateEmbedded(ctx, string(ddl)); err != nil {
		ms.Close()
		licCleanup()
		t.Fatalf("migrate: %v", err)
	}

	tok = "plt_bug009_flag_history_test"
	tokenHash := hashToken(tok)
	if err := ms.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "bug009-admin",
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
	if rec != nil {
		srv.SetFlagHistoryQuerier(rec)
	}
	ts = httptest.NewServer(srv.Handler())

	cleanup = func() {
		ts.Close()
		ms.Close()
		licCleanup()
	}
	return ts, tok, cleanup
}

// anomalyRaw sends GET /anomalies and returns status + raw body.
func anomalyRaw(t *testing.T, baseURL, tok, rawQuery string) (status int, body []byte) {
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
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, body
}

// anomalyPage sends GET /anomalies and decodes {items, meta:{next_cursor}}.
// Fatals on non-200 status.
func anomalyPage(t *testing.T, baseURL, tok, rawQuery string) (items []any, nextCursor *string) {
	t.Helper()
	status, body := anomalyRaw(t, baseURL, tok, rawQuery)
	if status != http.StatusOK {
		t.Fatalf("GET anomalies?%s: want 200, got %d: %s", rawQuery, status, body)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	items, _ = result["items"].([]any)
	metaBlock, _ := result["meta"].(map[string]any)
	if nc, ok := metaBlock["next_cursor"].(string); ok {
		nextCursor = &nc
	}
	return items, nextCursor
}

// ─── Routing unit tests ───────────────────────────────────────────────────────

// TestFlagHistory_NilQuerier_From_400 verifies that ?from with nil querier returns 400.
//
// RED: handler takes ComputeFlags path → 200.
// GREEN: handler detects nil querier on ?from branch → 400 FLAG_STORE_NOT_CONFIGURED.
func TestFlagHistory_NilQuerier_From_400(t *testing.T) {
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, nil)
	defer cleanup()

	epochMs := int64(1_700_000_000_000)
	qs := fmt.Sprintf("from=%d", epochMs)
	status, body := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusBadRequest {
		t.Errorf("?from with nil querier: want 400, got %d body=%s", status, body)
	} else {
		t.Logf("PASS: nil querier + ?from → 400: %s", body)
	}
	// Confirm error code.
	var errResp map[string]any
	if err := json.Unmarshal(body, &errResp); err == nil {
		if code, ok := errResp["code"].(string); ok {
			if code != "FLAG_STORE_NOT_CONFIGURED" {
				t.Errorf("want code FLAG_STORE_NOT_CONFIGURED, got %q", code)
			}
		}
	}
}

// TestFlagHistory_ToOnly_Routes verifies that ?to alone routes to the querier.
//
// RED: handler ignores ?to; querier is never called → 0 calls recorded.
// GREEN: handler routes ?to to querier → 1 call recorded.
func TestFlagHistory_ToOnly_Routes(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	epochMs := int64(1_700_100_000_000)
	qs := fmt.Sprintf("to=%d", epochMs)
	status, body := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusOK {
		t.Fatalf("?to with querier: want 200, got %d body=%s", status, body)
	}
	calls := rec.snapshot()
	if len(calls) != 1 {
		t.Errorf("?to routing: want 1 querier call, got %d (handler not routing ?to)", len(calls))
	} else {
		t.Logf("PASS: ?to → querier called: To=%v", calls[0].To)
	}
}

// TestFlagHistory_MalformedFrom_400 verifies that a present-but-unparseable ?from → 400.
//
// RED: handler ignores ?from; goes to ComputeFlags → 200.
// GREEN: handler detects malformed from → 400 BAD_REQUEST.
func TestFlagHistory_MalformedFrom_400(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	status, body := anomalyRaw(t, ts.URL, tok, "from=not-a-timestamp")
	if status != http.StatusBadRequest {
		t.Errorf("malformed ?from: want 400, got %d body=%s", status, body)
	} else {
		t.Logf("PASS: malformed ?from → 400: %s", body)
	}
	if calls := rec.snapshot(); len(calls) != 0 {
		t.Errorf("malformed ?from: querier should not be called, got %d call(s)", len(calls))
	}
}

// TestFlagHistory_EpochMs_Parsed verifies that ?from as epoch-milliseconds is
// correctly parsed and forwarded to the querier as time.Time.
//
// RED: querier is never called → captured From is zero.
// GREEN: querier called with From == time.UnixMilli(epochMs).
func TestFlagHistory_EpochMs_Parsed(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	epochMs := int64(1_700_000_000_000)
	qs := fmt.Sprintf("from=%d", epochMs)
	status, body := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusOK {
		t.Fatalf("epoch-ms ?from: want 200, got %d body=%s", status, body)
	}
	calls := rec.snapshot()
	if len(calls) == 0 {
		t.Fatalf("epoch-ms ?from: querier not called")
	}
	want := time.UnixMilli(epochMs)
	if !calls[0].From.Equal(want) {
		t.Errorf("epoch-ms ?from: captured From=%v, want %v", calls[0].From, want)
	} else {
		t.Logf("PASS: epoch-ms ?from → From=%v", calls[0].From)
	}
}

// TestFlagHistory_RFC3339_Parsed verifies that ?to as RFC3339 is correctly parsed.
//
// RED: querier is never called → captured To is zero.
// GREEN: querier called with To == parsed RFC3339 time.
func TestFlagHistory_RFC3339_Parsed(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	ts3339 := "2023-11-14T22:13:20Z"
	want, _ := time.Parse(time.RFC3339, ts3339)

	qs := "to=" + ts3339
	status, body := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusOK {
		t.Fatalf("RFC3339 ?to: want 200, got %d body=%s", status, body)
	}
	calls := rec.snapshot()
	if len(calls) == 0 {
		t.Fatalf("RFC3339 ?to: querier not called")
	}
	if !calls[0].To.Equal(want) {
		t.Errorf("RFC3339 ?to: captured To=%v, want %v", calls[0].To, want)
	} else {
		t.Logf("PASS: RFC3339 ?to → To=%v", calls[0].To)
	}
}

// TestFlagHistory_AbsentTo_ZeroTime verifies that absent ?to is forwarded as
// zero time (= unbounded upper side), NOT a 7-day default.
//
// RED: querier not called at all.
// GREEN: querier called with To.IsZero() == true.
func TestFlagHistory_AbsentTo_ZeroTime(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	epochMs := int64(1_700_000_000_000)
	qs := fmt.Sprintf("from=%d", epochMs) // no ?to
	status, body := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusOK {
		t.Fatalf("absent ?to: want 200, got %d body=%s", status, body)
	}
	calls := rec.snapshot()
	if len(calls) == 0 {
		t.Fatalf("absent ?to: querier not called")
	}
	if !calls[0].To.IsZero() {
		t.Errorf("absent ?to: captured To=%v, want zero (unbounded)", calls[0].To)
	} else {
		t.Logf("PASS: absent ?to → To=zero (unbounded)")
	}
}

// TestFlagHistory_MetricForwarded verifies that ?metric is forwarded to the querier.
//
// RED: querier not called → Metric == "".
// GREEN: querier called with Metric == "cpu_pct".
func TestFlagHistory_MetricForwarded(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	epochMs := int64(1_700_000_000_000)
	qs := fmt.Sprintf("from=%d&metric=cpu_pct", epochMs)
	status, body := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusOK {
		t.Fatalf("?metric: want 200, got %d body=%s", status, body)
	}
	calls := rec.snapshot()
	if len(calls) == 0 {
		t.Fatalf("?metric: querier not called")
	}
	if calls[0].Metric != "cpu_pct" {
		t.Errorf("?metric: captured Metric=%q, want %q", calls[0].Metric, "cpu_pct")
	} else {
		t.Logf("PASS: ?metric=cpu_pct forwarded")
	}
}

// TestFlagHistory_AppStreamForwarded verifies that ?app and ?stream are forwarded.
//
// RED: querier not called.
// GREEN: querier called with App="app-A" Stream="stream-1".
func TestFlagHistory_AppStreamForwarded(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	epochMs := int64(1_700_000_000_000)
	qs := fmt.Sprintf("from=%d&app=app-A&stream=stream-1", epochMs)
	status, body := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusOK {
		t.Fatalf("?app/?stream: want 200, got %d body=%s", status, body)
	}
	calls := rec.snapshot()
	if len(calls) == 0 {
		t.Fatalf("?app/?stream: querier not called")
	}
	if calls[0].App != "app-A" {
		t.Errorf("?app: captured App=%q, want %q", calls[0].App, "app-A")
	}
	if calls[0].Stream != "stream-1" {
		t.Errorf("?stream: captured Stream=%q, want %q", calls[0].Stream, "stream-1")
	}
	t.Logf("PASS: ?app=%s ?stream=%s forwarded", calls[0].App, calls[0].Stream)
}

// TestFlagHistory_MinSigmaForwarded verifies that ?min_sigma is forwarded.
//
// RED: querier not called → MinSigma == 0.
// GREEN: querier called with MinSigma == 3.5.
func TestFlagHistory_MinSigmaForwarded(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	epochMs := int64(1_700_000_000_000)
	qs := fmt.Sprintf("from=%d&min_sigma=3.5", epochMs)
	status, body := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusOK {
		t.Fatalf("?min_sigma: want 200, got %d body=%s", status, body)
	}
	calls := rec.snapshot()
	if len(calls) == 0 {
		t.Fatalf("?min_sigma: querier not called")
	}
	if calls[0].MinSigma != 3.5 {
		t.Errorf("?min_sigma: captured MinSigma=%v, want 3.5", calls[0].MinSigma)
	} else {
		t.Logf("PASS: ?min_sigma=3.5 forwarded")
	}
}

// TestFlagHistory_LimitClamp verifies that limit<=0→50, >500→500 (existing clamp).
//
// RED: querier not called → Limit == 0.
// GREEN: querier called with clamped limit.
func TestFlagHistory_LimitClamp(t *testing.T) {
	for _, tc := range []struct {
		name       string
		limitParam string
		wantLimit  int
	}{
		{"absent-default", "", 50},
		{"zero-default", "0", 50},
		{"negative-default", "-1", 50},
		{"normal", "10", 10},
		{"over-max", "600", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingFlagHistoryQuerier{}
			ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
			defer cleanup()

			epochMs := int64(1_700_000_000_000)
			qs := fmt.Sprintf("from=%d", epochMs)
			if tc.limitParam != "" {
				qs += "&limit=" + tc.limitParam
			}
			status, body := anomalyRaw(t, ts.URL, tok, qs)
			if status != http.StatusOK {
				t.Fatalf("limit %s: want 200, got %d body=%s", tc.limitParam, status, body)
			}
			calls := rec.snapshot()
			if len(calls) == 0 {
				t.Fatalf("limit %s: querier not called", tc.limitParam)
			}
			if calls[0].Limit != tc.wantLimit {
				t.Errorf("limit %s: captured Limit=%d, want %d", tc.limitParam, calls[0].Limit, tc.wantLimit)
			} else {
				t.Logf("PASS: limit=%s → captured Limit=%d", tc.limitParam, calls[0].Limit)
			}
		})
	}
}

// TestFlagHistory_CursorPassedThrough verifies that cursor is passed raw (not Atoi'd).
//
// RED: querier not called → Cursor == "".
// GREEN: querier called with Cursor == "base64value==".
func TestFlagHistory_CursorPassedThrough(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	rawCursor := "dGVzdGN1cnNvcg==" // base64("testcursor")
	epochMs := int64(1_700_000_000_000)
	qs := fmt.Sprintf("from=%d&cursor=%s", epochMs, rawCursor)
	status, body := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusOK {
		t.Fatalf("cursor passthrough: want 200, got %d body=%s", status, body)
	}
	calls := rec.snapshot()
	if len(calls) == 0 {
		t.Fatalf("cursor passthrough: querier not called")
	}
	if calls[0].Cursor != rawCursor {
		t.Errorf("cursor: captured %q, want %q (cursor must not be Atoi'd)", calls[0].Cursor, rawCursor)
	} else {
		t.Logf("PASS: cursor passed raw: %q", calls[0].Cursor)
	}
}

// TestFlagHistory_NoFromTo_ComputeFlagsPath verifies that absence of ?from and ?to
// does NOT route to the querier — the ComputeFlags path must remain unchanged.
//
// This test is GREEN before the routing change (querier is never called when
// no from/to is present) and must stay GREEN after.
func TestFlagHistory_NoFromTo_ComputeFlagsPath(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	// No from/to → ComputeFlags path; 6 flags returned by fakeAnomalyDetector.
	items, _ := anomalyPage(t, ts.URL, tok, "")
	if len(items) != 6 {
		t.Errorf("no from/to: want 6 items from ComputeFlags, got %d", len(items))
	}
	if calls := rec.snapshot(); len(calls) != 0 {
		t.Errorf("no from/to: querier must NOT be called, got %d call(s)", len(calls))
	}
	t.Logf("PASS: no from/to → ComputeFlags path (0 querier calls, %d items)", len(items))
}

// TestFlagHistory_NextCursorNull verifies that page.NextCursor=="" serializes as JSON null.
//
// RED: querier not called → response is from ComputeFlags (not controlled by page.NextCursor).
// GREEN: querier returns empty NextCursor → next_cursor is null in JSON.
func TestFlagHistory_NextCursorNull(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{
		page: api.FlagHistoryPage{
			Items:      nil,
			NextCursor: "", // empty = last page → null
		},
	}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	epochMs := int64(1_700_000_000_000)
	qs := fmt.Sprintf("from=%d", epochMs)
	status, rawBody := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusOK {
		t.Fatalf("next_cursor null: want 200, got %d body=%s", status, rawBody)
	}

	var result map[string]any
	if err := json.Unmarshal(rawBody, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	metaBlock, ok := result["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta field missing: %s", rawBody)
	}
	// next_cursor must be JSON null (nil in Go after unmarshal), not empty string "".
	if nc, exists := metaBlock["next_cursor"]; !exists {
		t.Errorf("next_cursor field absent from meta")
	} else if nc != nil {
		t.Errorf("next_cursor: want JSON null (nil), got %v (type %T)", nc, nc)
	} else {
		t.Logf("PASS: NextCursor==\"\" → next_cursor=null in JSON")
	}
}

// TestFlagHistory_NextCursorSet verifies that a non-empty page.NextCursor serializes
// as a JSON string.
//
// RED: querier not called → next_cursor is from ComputeFlags pagination (not our page).
// GREEN: querier returns NextCursor="abc123" → next_cursor="abc123" in JSON.
func TestFlagHistory_NextCursorSet(t *testing.T) {
	nc := "dGVzdGN1cnNvcg=="
	rec := &recordingFlagHistoryQuerier{
		page: api.FlagHistoryPage{
			Items:      []api.AnomalyFlagAPI{},
			NextCursor: nc,
		},
	}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	epochMs := int64(1_700_000_000_000)
	qs := fmt.Sprintf("from=%d", epochMs)
	status, rawBody := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusOK {
		t.Fatalf("next_cursor set: want 200, got %d body=%s", status, rawBody)
	}

	var result map[string]any
	if err := json.Unmarshal(rawBody, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	metaBlock, _ := result["meta"].(map[string]any)
	got, _ := metaBlock["next_cursor"].(string)
	if got != nc {
		t.Errorf("next_cursor: want %q, got %q", nc, got)
	} else {
		t.Logf("PASS: NextCursor=%q serialized correctly", nc)
	}
}

// TestFlagHistory_BadCursorError_400 verifies that a querier error wrapping
// ErrBadCursor maps to HTTP 400 (not 500).
//
// RED: querier not called → no error path reached at all.
// GREEN: querier returns ErrBadCursor → 400 BAD_REQUEST.
func TestFlagHistory_BadCursorError_400(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{err: fmt.Errorf("cursor decode failed: %w", api.ErrBadCursor)}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	epochMs := int64(1_700_000_000_000)
	qs := fmt.Sprintf("from=%d&cursor=BADC==", epochMs)
	status, body := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusBadRequest {
		t.Errorf("bad cursor error: want 400, got %d body=%s", status, body)
	} else {
		t.Logf("PASS: ErrBadCursor → 400: %s", body)
	}
}

// TestFlagHistory_OtherError_500 verifies that non-cursor querier errors map to 500.
//
// RED: querier not called → no error path reached.
// GREEN: querier returns generic error → 500 INTERNAL_ERROR.
func TestFlagHistory_OtherError_500(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{err: fmt.Errorf("clickhouse connection refused")}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	epochMs := int64(1_700_000_000_000)
	qs := fmt.Sprintf("from=%d", epochMs)
	status, body := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusInternalServerError {
		t.Errorf("non-cursor error: want 500, got %d body=%s", status, body)
	} else {
		t.Logf("PASS: non-cursor error → 500: %s", body)
	}
}

// TestFlagHistory_ItemsNilEmpty verifies that a nil page.Items serializes as [].
//
// RED: querier not called.
// GREEN: querier returns nil Items → items=[] in JSON (never null).
func TestFlagHistory_ItemsNilEmpty(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{
		page: api.FlagHistoryPage{Items: nil, NextCursor: ""},
	}
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	epochMs := int64(1_700_000_000_000)
	qs := fmt.Sprintf("from=%d", epochMs)
	status, rawBody := anomalyRaw(t, ts.URL, tok, qs)
	if status != http.StatusOK {
		t.Fatalf("nil items: want 200, got %d body=%s", status, rawBody)
	}
	var result map[string]any
	if err := json.Unmarshal(rawBody, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	items, _ := result["items"].([]any)
	if items == nil {
		t.Errorf("items must be [] (non-null array), got null")
	} else {
		t.Logf("PASS: nil Items → items=[] (len=%d)", len(items))
	}
}

// TestFlagHistory_NonOverlappingWindow verifies the ADR §8 differential sub-case:
// two disjoint [from, to] ranges both route through the querier, confirming
// the args are forwarded and not silently dropped (empty pages are acceptable).
//
// RED: querier not called for either request.
// GREEN: querier called twice with distinct From/To.
func TestFlagHistory_NonOverlappingWindow(t *testing.T) {
	rec := &recordingFlagHistoryQuerier{} // returns empty page
	ts, tok, cleanup := setupEnterpriseAnomalyServerWithHistory(t, rec)
	defer cleanup()

	window1From := int64(1_700_000_000_000)
	window1To := int64(1_700_050_000_000)
	window2From := int64(1_700_100_000_000)
	window2To := int64(1_700_150_000_000)

	for _, tc := range []struct {
		qs       string
		wantFrom time.Time
		wantTo   time.Time
	}{
		{
			fmt.Sprintf("from=%d&to=%d", window1From, window1To),
			time.UnixMilli(window1From), time.UnixMilli(window1To),
		},
		{
			fmt.Sprintf("from=%d&to=%d", window2From, window2To),
			time.UnixMilli(window2From), time.UnixMilli(window2To),
		},
	} {
		status, body := anomalyRaw(t, ts.URL, tok, tc.qs)
		if status != http.StatusOK {
			t.Fatalf("non-overlapping window %s: want 200, got %d body=%s", tc.qs, status, body)
		}
	}

	calls := rec.snapshot()
	if len(calls) != 2 {
		t.Fatalf("non-overlapping window: want 2 querier calls, got %d", len(calls))
	}
	if !calls[0].From.Equal(time.UnixMilli(window1From)) {
		t.Errorf("call[0].From=%v, want %v", calls[0].From, time.UnixMilli(window1From))
	}
	if !calls[0].To.Equal(time.UnixMilli(window1To)) {
		t.Errorf("call[0].To=%v, want %v", calls[0].To, time.UnixMilli(window1To))
	}
	if !calls[1].From.Equal(time.UnixMilli(window2From)) {
		t.Errorf("call[1].From=%v, want %v", calls[1].From, time.UnixMilli(window2From))
	}
	if !calls[1].To.Equal(time.UnixMilli(window2To)) {
		t.Errorf("call[1].To=%v, want %v", calls[1].To, time.UnixMilli(window2To))
	}
	t.Logf("PASS: two disjoint windows both routed (calls[0].From=%v calls[1].From=%v)",
		calls[0].From, calls[1].From)
}
