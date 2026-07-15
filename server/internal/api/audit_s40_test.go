// Package api_test — S40 (D-102) audit-trail capture + read endpoint tests.
//
// Proves end-to-end that a mutating admin call records who/what/when and that
// GET /api/v1/admin/audit-log reads it back. The test server mints an admin token
// named "test-admin" (setupTestServer), so a captured entry must name that actor.
package api_test

import (
	"net/http"
	"testing"
)

// getAuditLog GETs the newest audit-log page.
func getAuditLog(t *testing.T, base, token string) jsonResp {
	t.Helper()
	return doJSON(t, http.DefaultClient, http.MethodGet, base+"/api/v1/admin/audit-log", token, nil)
}

// findAuditEntry returns the first audit item matching action+objectID, or nil.
func findAuditEntry(items []any, action, objectID string) map[string]any {
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		if m["action"] == action && m["object_id"] == objectID {
			return m
		}
	}
	return nil
}

// TestAudit_UserCreate_Recorded is the end-to-end capture+read proof: creating a
// user writes an audit entry that names the actor, action and object.
//
// Mutation proof: delete the `s.audit(...)` call from handleCreateUser and this
// test goes RED (no user.create entry is found) while the S38 user tests stay green.
func TestAudit_UserCreate_Recorded(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	created := createUser(t, ts.URL, token, "audit-alice", "admin")
	if created.status != http.StatusCreated {
		t.Fatalf("create user: expected 201, got %d: %s", created.status, created.body)
	}
	uid, _ := created.json["id"].(string)
	if uid == "" {
		t.Fatalf("no id in create response: %s", created.body)
	}

	al := getAuditLog(t, ts.URL, token)
	if al.status != http.StatusOK {
		t.Fatalf("audit-log GET: expected 200, got %d: %s", al.status, al.body)
	}
	items, _ := al.json["items"].([]any)
	entry := findAuditEntry(items, "user.create", uid)
	if entry == nil {
		t.Fatalf("no user.create audit entry for %s; body=%s", uid, al.body)
	}
	if entry["object_type"] != "user" {
		t.Errorf("object_type: got %v, want user", entry["object_type"])
	}
	if entry["actor_name"] != "test-admin" {
		t.Errorf("actor_name: got %v, want test-admin", entry["actor_name"])
	}
	if tid, _ := entry["actor_token_id"].(string); tid == "" {
		t.Error("actor_token_id empty — the caller identity was not captured")
	}
	// The detail carries the non-sensitive summary (username/role), never a secret.
	if detail, ok := entry["detail"].(map[string]any); ok {
		if detail["username"] != "audit-alice" {
			t.Errorf("detail.username: got %v, want audit-alice", detail["username"])
		}
	}
}

// TestAudit_UpdateAndDelete_Recorded: update then delete each leave their own entry.
func TestAudit_UpdateAndDelete_Recorded(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	created := createUser(t, ts.URL, token, "audit-bob", "admin")
	uid, _ := created.json["id"].(string)

	upd := doJSON(t, http.DefaultClient, http.MethodPut, ts.URL+"/api/v1/admin/users/"+uid, token,
		map[string]any{"username": "audit-bob", "role": "viewer"})
	if upd.status != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", upd.status, upd.body)
	}
	del := doJSON(t, http.DefaultClient, http.MethodDelete, ts.URL+"/api/v1/admin/users/"+uid, token, nil)
	if del.status != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", del.status, del.body)
	}

	al := getAuditLog(t, ts.URL, token)
	items, _ := al.json["items"].([]any)
	if findAuditEntry(items, "user.update", uid) == nil {
		t.Errorf("no user.update audit entry for %s; body=%s", uid, al.body)
	}
	if findAuditEntry(items, "user.delete", uid) == nil {
		t.Errorf("no user.delete audit entry for %s; body=%s", uid, al.body)
	}
}

// TestAudit_ReadRequiresAuth: the read endpoint is behind bearer auth like every
// other /api/v1 route — no token → 401.
func TestAudit_ReadRequiresAuth(t *testing.T) {
	ts, _, cleanup := setupTestServer(t)
	defer cleanup()

	al := doJSON(t, http.DefaultClient, http.MethodGet, ts.URL+"/api/v1/admin/audit-log", "", nil)
	if al.status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated audit-log GET: expected 401, got %d: %s", al.status, al.body)
	}
}
