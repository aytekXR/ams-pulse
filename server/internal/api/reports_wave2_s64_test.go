// S64 (D-126) — end-to-end proofs for the reports_wave2 re-fetch cluster,
// exercised through the full HTTP stack (auth + license + routing).
//
//   - handleUpdateReportSchedule ([6]): the redundant post-update re-fetch was
//     dropped; the response is now rendered from the in-hand row. Asserts the PUT
//     response body reflects the updated cron/format and keeps a schedulable
//     next_run_at — i.e. the drop renders correct data, not a stale/empty body.
//   - handleUpdateTenant ([5]): the guarded re-fetch still returns the
//     DB-authoritative row. Asserts the PUT response reflects the new name and
//     carries updated_at (stamped inside UpdateTenant, visible only via the
//     re-fetch — which is why the re-fetch is kept here, unlike [6]).
//   - [19] positive control: a genuinely missing schedule still yields 404, not
//     500 — the err/nil split must not turn ErrNoRows into an INTERNAL_ERROR.
//     (The tenant missing→404 path is already covered by TestTenant_* and stays
//     green under the split.)
package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// putJSONReq issues an authenticated JSON PUT and returns the response.
func putJSONReq(t *testing.T, url, token string, body []byte) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", url, err)
	}
	return resp
}

func TestUpdateReportSchedule_ResponseRenderedFromRow_S64(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	// Create a schedule.
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
		t.Fatalf("create: want 201 got %d: %s", resp.StatusCode, b)
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("no id in create response")
	}

	// Update cron + format; the RESPONSE (rendered from row, no re-fetch) must reflect both.
	up, _ := json.Marshal(map[string]any{"cron": "0 7 * * 2", "format": "pdf"})
	uresp := putJSONReq(t, ts.URL+"/api/v1/reports/schedules/"+id, token, up)
	defer uresp.Body.Close()
	if uresp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(uresp.Body)
		t.Fatalf("update: want 200 got %d: %s", uresp.StatusCode, b)
	}
	var got map[string]any
	if err := json.NewDecoder(uresp.Body).Decode(&got); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if got["id"] != id {
		t.Errorf("id: want %s got %v", id, got["id"])
	}
	if got["cron"] != "0 7 * * 2" {
		t.Errorf("cron not reflected in update response: got %v", got["cron"])
	}
	if got["format"] != "pdf" {
		t.Errorf("format not reflected in update response: got %v", got["format"])
	}
	if _, ok := got["next_run_at"]; !ok {
		t.Error("next_run_at absent from update response — edited schedule must stay schedulable")
	}
}

func TestUpdateTenant_ResponseHasFreshUpdatedAt_S64(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	// Create a tenant.
	resp := createTenantReq(t, ts.URL, token, map[string]any{"name": "Orig Co", "stream_pattern": "orig/%"})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create tenant: want 201 got %d: %s", resp.StatusCode, b)
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("no id in create response")
	}
	createdAt, _ := created["updated_at"].(float64)

	// Update the name; the guarded re-fetch must return the fresh, DB-authoritative row.
	ub, _ := json.Marshal(map[string]any{"name": "Renamed Co", "stream_pattern": "orig/%"})
	uresp := putJSONReq(t, ts.URL+"/api/v1/admin/tenants/"+id, token, ub)
	defer uresp.Body.Close()
	if uresp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(uresp.Body)
		t.Fatalf("update tenant: want 200 got %d: %s", uresp.StatusCode, b)
	}
	var got map[string]any
	if err := json.NewDecoder(uresp.Body).Decode(&got); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if got["name"] != "Renamed Co" {
		t.Errorf("name not reflected in update response: got %v", got["name"])
	}
	ua, ok := got["updated_at"].(float64)
	if !ok {
		t.Fatalf("updated_at absent/not-numeric in update response: %v", got["updated_at"])
	}
	if ua < createdAt {
		t.Errorf("updated_at regressed after update: got %.0f, create had %.0f", ua, createdAt)
	}
}

func TestUpdateReportSchedule_MissingRow_Returns404_S64(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	// A genuine missing row (ErrNoRows → nil,nil) must stay 404 after the err/nil split.
	up, _ := json.Marshal(map[string]any{"cron": "0 7 * * 2", "format": "pdf"})
	resp := putJSONReq(t, ts.URL+"/api/v1/reports/schedules/does-not-exist", token, up)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("missing schedule: want 404 got %d: %s", resp.StatusCode, b)
	}
	var got map[string]any
	json.NewDecoder(resp.Body).Decode(&got)
	if got["code"] != "NOT_FOUND" {
		t.Errorf("want NOT_FOUND code, got %v", got["code"])
	}
}
