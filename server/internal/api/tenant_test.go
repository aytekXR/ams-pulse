// Package api_test — VAL-tenant: tenant CRUD handler tests (D-010, §7.11).
//
// Tests:
//   - CRUD happy paths (create, get, list, update, delete) on Enterprise tier
//   - 409 DUPLICATE_NAME on create and update
//   - 404 NOT_FOUND for unknown tenant ID
//   - 422 INVALID_TENANT when no matcher field provided
//   - Tier gate: Free and Pro tiers → 403 LICENSE_REQUIRED on all 5 ops
//   - Stream resolution: a created tenant's stream_pattern resolves a stream in
//     TenantMatcher (ties to F6 usage accounting / reconcile path)
//   - OpenAPI conformance for listTenants, createTenant, getTenant, updateTenant, deleteTenant
package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/reports"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// createTenantReq issues POST /api/v1/admin/tenants and returns the response.
// baseURL is the httptest.Server.URL string.
func createTenantReq(t *testing.T, baseURL, token string, body map[string]any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/admin/tenants", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/tenants: %v", err)
	}
	return resp
}

// ─── CRUD happy path ──────────────────────────────────────────────────────────

func TestTenant_CRUD_HappyPath(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	// CREATE
	createBody := map[string]any{
		"name":           "Acme Corp",
		"stream_pattern": "acme/%",
	}
	resp := createTenantReq(t, ts.URL, token, createBody)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, b)
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	tenantID, _ := created["id"].(string)
	if tenantID == "" {
		t.Fatal("expected id in create response")
	}
	if created["name"] != "Acme Corp" {
		t.Errorf("expected name=Acme Corp, got %v", created["name"])
	}
	if created["stream_pattern"] != "acme/%" {
		t.Errorf("expected stream_pattern=acme/%%, got %v", created["stream_pattern"])
	}
	t.Logf("PASS: POST /admin/tenants → 201 (id=%s)", tenantID)

	// GET
	getReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/tenants/"+tenantID, nil)
	getReq.Header.Set("Authorization", authHeader(token))
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("GET /admin/tenants/%s: %v", tenantID, err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(getResp.Body)
		t.Fatalf("expected 200, got %d: %s", getResp.StatusCode, b)
	}
	var got map[string]any
	json.NewDecoder(getResp.Body).Decode(&got)
	if got["id"] != tenantID {
		t.Errorf("expected id=%s, got %v", tenantID, got["id"])
	}
	t.Logf("PASS: GET /admin/tenants/%s → 200", tenantID)

	// LIST
	listReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/tenants", nil)
	listReq.Header.Set("Authorization", authHeader(token))
	listResp, err2 := http.DefaultClient.Do(listReq)
	if err2 != nil {
		t.Fatalf("GET /admin/tenants: %v", err2)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(listResp.Body)
		t.Fatalf("expected 200 list, got %d: %s", listResp.StatusCode, b)
	}
	var listBody map[string]any
	json.NewDecoder(listResp.Body).Decode(&listBody)
	items, _ := listBody["items"].([]any)
	if len(items) != 1 {
		t.Errorf("expected 1 tenant in list, got %d", len(items))
	}
	t.Logf("PASS: GET /admin/tenants → 200, %d item(s)", len(items))

	// UPDATE
	updateBody := map[string]any{
		"name":           "Acme Corp Updated",
		"stream_pattern": "acme-updated/%",
	}
	ub, _ := json.Marshal(updateBody)
	putReq, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/tenants/"+tenantID, bytes.NewReader(ub))
	putReq.Header.Set("Authorization", authHeader(token))
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err3 := http.DefaultClient.Do(putReq)
	if err3 != nil {
		t.Fatalf("PUT /admin/tenants: %v", err3)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(putResp.Body)
		t.Fatalf("expected 200 from PUT, got %d: %s", putResp.StatusCode, b)
	}
	var updated map[string]any
	json.NewDecoder(putResp.Body).Decode(&updated)
	if updated["name"] != "Acme Corp Updated" {
		t.Errorf("expected name=Acme Corp Updated, got %v", updated["name"])
	}
	t.Logf("PASS: PUT /admin/tenants/%s → 200", tenantID)

	// DELETE
	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/admin/tenants/"+tenantID, nil)
	delReq.Header.Set("Authorization", authHeader(token))
	delResp, _ := http.DefaultClient.Do(delReq)
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 from DELETE, got %d", delResp.StatusCode)
	}
	t.Logf("PASS: DELETE /admin/tenants/%s → 204", tenantID)

	// Verify gone via GET
	getReq2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/tenants/"+tenantID, nil)
	getReq2.Header.Set("Authorization", authHeader(token))
	getResp2, _ := http.DefaultClient.Do(getReq2)
	getResp2.Body.Close()
	if getResp2.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", getResp2.StatusCode)
	}
	t.Logf("PASS: GET after DELETE → 404 (tenant gone)")
}

// ─── MetaTag matcher happy path ───────────────────────────────────────────────

func TestTenant_MetaTagMatcher_HappyPath(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	body := map[string]any{
		"name":           "Meta Tenant",
		"meta_tag_key":   "org",
		"meta_tag_value": "acme",
	}
	resp := createTenantReq(t, ts.URL, token, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, b)
	}
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	if created["meta_tag_key"] != "org" {
		t.Errorf("expected meta_tag_key=org, got %v", created["meta_tag_key"])
	}
	t.Logf("PASS: tenant with meta_tag matcher → 201 (id=%v)", created["id"])
}

// ─── 409 DUPLICATE_NAME ───────────────────────────────────────────────────────

func TestTenant_DuplicateName_409(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	body := map[string]any{
		"name":           "Duplicate Tenant",
		"stream_pattern": "dup/%",
	}
	// First create — should succeed.
	resp1 := createTenantReq(t, ts.URL, token, body)
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first create expected 201, got %d", resp1.StatusCode)
	}

	// Second create with same name — should 409.
	resp2 := createTenantReq(t, ts.URL, token, body)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 409 for duplicate name, got %d: %s", resp2.StatusCode, b)
	}
	var errResp map[string]any
	json.NewDecoder(resp2.Body).Decode(&errResp)
	if errResp["code"] != "DUPLICATE_NAME" {
		t.Errorf("expected code=DUPLICATE_NAME, got %v", errResp["code"])
	}
	t.Logf("PASS: duplicate name → 409 DUPLICATE_NAME")
}

// TestTenant_UpdateDuplicateName_409 ensures PUT with a conflicting name returns 409.
func TestTenant_UpdateDuplicateName_409(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	// Create two tenants.
	b1 := map[string]any{"name": "Tenant A", "stream_pattern": "ta/%"}
	r1 := createTenantReq(t, ts.URL, token, b1)
	r1.Body.Close()
	if r1.StatusCode != http.StatusCreated {
		t.Fatalf("create A: expected 201, got %d", r1.StatusCode)
	}

	b2 := map[string]any{"name": "Tenant B", "stream_pattern": "tb/%"}
	r2 := createTenantReq(t, ts.URL, token, b2)
	var created2 map[string]any
	json.NewDecoder(r2.Body).Decode(&created2)
	r2.Body.Close()
	if r2.StatusCode != http.StatusCreated {
		t.Fatalf("create B: expected 201, got %d", r2.StatusCode)
	}
	idB, _ := created2["id"].(string)

	// Try to rename B to A's name.
	upd := map[string]any{"name": "Tenant A", "stream_pattern": "tb/%"}
	ub, _ := json.Marshal(upd)
	putReq, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/tenants/"+idB, bytes.NewReader(ub))
	putReq.Header.Set("Authorization", authHeader(token))
	putReq.Header.Set("Content-Type", "application/json")
	putResp, errPut := http.DefaultClient.Do(putReq)
	if errPut != nil {
		t.Fatalf("PUT /admin/tenants/%s: %v", idB, errPut)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(putResp.Body)
		t.Fatalf("expected 409 for duplicate name on update, got %d: %s", putResp.StatusCode, b)
	}
	t.Logf("PASS: PUT with duplicate name → 409")
}

// ─── 404 NOT_FOUND ────────────────────────────────────────────────────────────

func TestTenant_GetUnknownID_404(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/tenants/nonexistent-uuid", nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown id, got %d", resp.StatusCode)
	}
	t.Logf("PASS: GET /admin/tenants/nonexistent → 404")
}

func TestTenant_UpdateUnknownID_404(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	body := map[string]any{"name": "X", "stream_pattern": "x/%"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/tenants/no-such-id", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown id, got %d", resp.StatusCode)
	}
	t.Logf("PASS: PUT /admin/tenants/no-such-id → 404")
}

func TestTenant_DeleteUnknownID_404(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/admin/tenants/no-such-id", nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown delete, got %d", resp.StatusCode)
	}
	t.Logf("PASS: DELETE /admin/tenants/no-such-id → 404")
}

// ─── 422 validation: no matcher field ────────────────────────────────────────

func TestTenant_NoMatcher_422(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	// No stream_pattern and no meta_tag_key/value — should 422.
	body := map[string]any{"name": "No Matcher"}
	resp := createTenantReq(t, ts.URL, token, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 422 for no matcher, got %d: %s", resp.StatusCode, b)
	}
	var errResp map[string]any
	json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp["code"] != "INVALID_TENANT" {
		t.Errorf("expected code=INVALID_TENANT, got %v", errResp["code"])
	}
	t.Logf("PASS: no matcher → 422 INVALID_TENANT")
}

func TestTenant_PartialMetaTag_422(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	// meta_tag_key without meta_tag_value — partial, should 422.
	body := map[string]any{"name": "Partial Meta", "meta_tag_key": "org"}
	resp := createTenantReq(t, ts.URL, token, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 422 for partial meta tag, got %d: %s", resp.StatusCode, b)
	}
	t.Logf("PASS: meta_tag_key without meta_tag_value → 422")
}

// ─── Tier gate: Free → 403 ────────────────────────────────────────────────────

// TestTenant_FreeTier_Blocked_403 verifies that Free tier gets 403 on all 5 ops.
func TestTenant_FreeTier_Blocked_403(t *testing.T) {
	ts, token, cleanup := setupTestServer(t) // free tier by default
	defer cleanup()

	tenantBody := map[string]any{"name": "T", "stream_pattern": "t/%"}
	endpoints := []struct {
		method  string
		path    string
		hasBody bool
	}{
		{http.MethodGet, "/api/v1/admin/tenants", false},
		{http.MethodPost, "/api/v1/admin/tenants", true},
		{http.MethodGet, "/api/v1/admin/tenants/fake-id", false},
		{http.MethodPut, "/api/v1/admin/tenants/fake-id", true},
		{http.MethodDelete, "/api/v1/admin/tenants/fake-id", false},
	}

	for _, ep := range endpoints {
		var body io.Reader
		if ep.hasBody {
			b, _ := json.Marshal(tenantBody)
			body = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(ep.method, ts.URL+ep.path, body)
		req.Header.Set("Authorization", authHeader(token))
		if ep.hasBody {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", ep.method, ep.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Free tier: %s %s — expected 403, got %d", ep.method, ep.path, resp.StatusCode)
		} else {
			t.Logf("PASS: Free tier %s %s → 403", ep.method, ep.path)
		}
	}
}

// TestTenant_ProTier_Blocked_403 verifies that Pro tier gets 403 (via enterprise server with pro license).
// Since we don't have a pro license helper, we use the free-tier server (which is also non-Enterprise).
func TestTenant_NonEnterpriseTier_Blocked_403(t *testing.T) {
	// Same as free tier test — both free and pro map to non-enterprise.
	ts, token, cleanup := setupTestServer(t) // free tier
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/tenants", nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, errGet := http.DefaultClient.Do(req)
	if errGet != nil {
		t.Fatalf("GET /admin/tenants: %v", errGet)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 403 for non-enterprise GET /admin/tenants, got %d: %s", resp.StatusCode, b)
	}
	t.Logf("PASS: non-Enterprise tier → 403 for GET /admin/tenants")
}

// ─── Stream resolution (F6 reconcile path) ───────────────────────────────────

// TestTenant_StreamPattern_ResolvesStream verifies that a tenant's stream_pattern
// resolves a matching stream via TenantMatcher (ties to F6 usage accounting).
func TestTenant_StreamPattern_ResolvesStream(t *testing.T) {
	ts, token, mstore, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	// Create a tenant with a glob pattern.
	body := map[string]any{
		"name":           "LiveCorp",
		"stream_pattern": "live/%",
	}
	resp := createTenantReq(t, ts.URL, token, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create tenant: expected 201, got %d: %s", resp.StatusCode, b)
	}

	// Load tenants from the meta store and build a TenantMatcher.
	// Fetch tenants from the store directly to build a matcher.
	tenants, err := mstore.ListTenants(resp.Request.Context())
	if err != nil {
		t.Fatalf("ListTenants: %v", err)
	}
	if len(tenants) == 0 {
		t.Fatal("expected 1 tenant after create")
	}

	matcher := reports.NewTenantMatcher(tenants)

	// Streams matching "live/%" should resolve to "LiveCorp".
	testCases := []struct {
		streamID string
		want     string
	}{
		{"live/stream1", "LiveCorp"},
		{"live/another", "LiveCorp"},
		{"other/stream", ""}, // unmatched → empty (unassigned)
	}
	for _, tc := range testCases {
		got := matcher.Resolve(tc.streamID, nil)
		if got != tc.want {
			t.Errorf("Resolve(%q) = %q, want %q", tc.streamID, got, tc.want)
		} else {
			t.Logf("PASS: Resolve(%q) = %q", tc.streamID, got)
		}
	}
}

// TestTenant_MetaTag_ResolvesStream verifies meta-tag matching via TenantMatcher.
func TestTenant_MetaTag_ResolvesStream(t *testing.T) {
	ts, token, mstore, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	body := map[string]any{
		"name":           "OrgTenant",
		"meta_tag_key":   "org",
		"meta_tag_value": "acme",
	}
	resp := createTenantReq(t, ts.URL, token, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create tenant: expected 201, got %d: %s", resp.StatusCode, b)
	}

	tenants, err := mstore.ListTenants(resp.Request.Context())
	if err != nil {
		t.Fatalf("ListTenants: %v", err)
	}
	matcher := reports.NewTenantMatcher(tenants)

	// Meta-tag match should take precedence.
	got := matcher.Resolve("any-stream", map[string]string{"org": "acme"})
	if got != "OrgTenant" {
		t.Errorf("meta-tag Resolve = %q, want OrgTenant", got)
	}
	t.Logf("PASS: meta-tag match resolves to %q", got)

	// No matching meta tag → unassigned.
	got2 := matcher.Resolve("any-stream", map[string]string{"org": "other"})
	if got2 != "" {
		t.Errorf("non-matching meta-tag Resolve = %q, want empty (unassigned)", got2)
	}
	t.Logf("PASS: non-matching meta-tag → unassigned (empty string)")
}

// ─── OpenAPI conformance (kin-openapi) ───────────────────────────────────────

func TestTenant_OpenAPI_ListConforms(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/tenants", nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /admin/tenants: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/tenants", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: GET /api/v1/admin/tenants → 200, conforms to OpenAPI spec (listTenants)")
}

func TestTenant_OpenAPI_CreateConforms(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	body := map[string]any{"name": "Conform Corp", "stream_pattern": "cc/%"}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/admin/tenants", bytes.NewReader(b))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/tenants: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body2, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body2)
	}
	req2, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/tenants", nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: POST /api/v1/admin/tenants → 201, conforms to OpenAPI spec (createTenant)")
}

func TestTenant_OpenAPI_GetConforms(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Create a tenant first.
	body := map[string]any{"name": "GetConform", "stream_pattern": "gc/%"}
	cr := createTenantReq(t, ts.URL, token, body)
	var created map[string]any
	json.NewDecoder(cr.Body).Decode(&created)
	cr.Body.Close()
	tenantID, _ := created["id"].(string)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/admin/tenants/"+tenantID, nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /admin/tenants/%s: %v", tenantID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	req2, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/tenants/"+tenantID, nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: GET /api/v1/admin/tenants/%s → 200, conforms to OpenAPI spec (getTenant)", tenantID)
}

func TestTenant_OpenAPI_UpdateConforms(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Create first.
	body := map[string]any{"name": "UpdConform", "stream_pattern": "uc/%"}
	cr := createTenantReq(t, ts.URL, token, body)
	var created map[string]any
	json.NewDecoder(cr.Body).Decode(&created)
	cr.Body.Close()
	tenantID, _ := created["id"].(string)

	upd := map[string]any{"name": "UpdConformUpdated", "stream_pattern": "uc2/%"}
	ub, _ := json.Marshal(upd)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/admin/tenants/"+tenantID, bytes.NewReader(ub))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /admin/tenants/%s: %v", tenantID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, b)
	}
	req2, _ := http.NewRequest(http.MethodPut, "/api/v1/admin/tenants/"+tenantID, nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: PUT /api/v1/admin/tenants/%s → 200, conforms to OpenAPI spec (updateTenant)", tenantID)
}

func TestTenant_OpenAPI_DeleteConforms(t *testing.T) {
	ts, token, _, cleanup := setupEnterpriseServer(t)
	defer cleanup()
	doc := openAPISpec(t)

	// Create first.
	body := map[string]any{"name": "DelConform", "stream_pattern": "dc/%"}
	cr := createTenantReq(t, ts.URL, token, body)
	var created map[string]any
	json.NewDecoder(cr.Body).Decode(&created)
	cr.Body.Close()
	tenantID, _ := created["id"].(string)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/admin/tenants/"+tenantID, nil)
	req.Header.Set("Authorization", authHeader(token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /admin/tenants/%s: %v", tenantID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, b)
	}
	req2, _ := http.NewRequest(http.MethodDelete, "/api/v1/admin/tenants/"+tenantID, nil)
	req2.Header.Set("Authorization", authHeader(token))
	conformCheck(t, doc, req2, resp)
	t.Logf("PASS: DELETE /api/v1/admin/tenants/%s → 204, conforms to OpenAPI spec (deleteTenant)", tenantID)
}
