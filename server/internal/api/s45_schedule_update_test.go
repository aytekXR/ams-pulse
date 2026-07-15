// S45 (D-107) — editing a report schedule must not silence it.
//
// BLOCKER: handleUpdateReportSchedule built the row from reportScheduleFromAPI
// (which leaves NextRunAt/LastRunAt nil) and wrote it, NULLing next_run_at.
// ListDueReportSchedules filters `next_run_at IS NOT NULL`, so an edited schedule
// would never fire again. The fix recomputes next_run_at from the (possibly new)
// cron and preserves last_run_at, mirroring the create handler.
//
// Mutation proof: remove the recompute/preserve block in handleUpdateReportSchedule
// → next_run_at is NULL after update → this test goes RED (both the NextRunAt
// assertion and the ListDueReportSchedules check).
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestUpdateReportSchedule_PreservesNextRunAndLastRun(t *testing.T) {
	ts, token, ms, cleanup := setupEnterpriseServer(t)
	defer cleanup()
	ctx := context.Background()

	// Create a weekly schedule.
	body, _ := json.Marshal(map[string]any{"cron": "0 6 * * 1", "format": "csv"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/reports/schedules", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /reports/schedules: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("POST expected 201, got %d: %s", resp.StatusCode, b)
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("no id in create response")
	}

	// Simulate the scheduler having fired once so last_run_at is set.
	const lastRun = int64(1_700_000_000_000)
	if err := ms.MarkScheduleRan(ctx, id, lastRun, 1_700_600_000_000); err != nil {
		t.Fatalf("MarkScheduleRan: %v", err)
	}

	// Edit the schedule (change cron + format).
	upBody, _ := json.Marshal(map[string]any{"cron": "0 7 * * 2", "format": "pdf"})
	ureq, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/reports/schedules/"+id, bytes.NewReader(upBody))
	ureq.Header.Set("Authorization", authHeader(token))
	ureq.Header.Set("Content-Type", "application/json")
	uresp, err := http.DefaultClient.Do(ureq)
	if err != nil {
		t.Fatalf("PUT /reports/schedules/%s: %v", id, err)
	}
	if uresp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(uresp.Body)
		uresp.Body.Close()
		t.Fatalf("PUT expected 200, got %d: %s", uresp.StatusCode, b)
	}
	uresp.Body.Close()

	// The stored row must remain schedulable.
	row, err := ms.GetReportSchedule(ctx, id)
	if err != nil || row == nil {
		t.Fatalf("GetReportSchedule: %v (row=%v)", err, row)
	}
	if row.NextRunAt == nil {
		t.Fatal("next_run_at is NULL after update — schedule permanently silenced " +
			"(ListDueReportSchedules filters `next_run_at IS NOT NULL`)")
	}
	if row.LastRunAt == nil || *row.LastRunAt != lastRun {
		t.Errorf("last_run_at not preserved across update: got %v, want %d", row.LastRunAt, lastRun)
	}

	// And it must actually be returned as due once its next_run_at arrives.
	due, err := ms.ListDueReportSchedules(ctx, *row.NextRunAt)
	if err != nil {
		t.Fatalf("ListDueReportSchedules: %v", err)
	}
	found := false
	for _, d := range due {
		if d.ID == id {
			found = true
		}
	}
	if !found {
		t.Error("edited schedule not returned by ListDueReportSchedules at its next_run_at — it will never fire")
	}
}
