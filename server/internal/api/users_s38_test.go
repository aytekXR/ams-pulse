// Package api_test — S38 (D-100) /admin/users correctness tests.
//
// The user CRUD handlers had three real bugs that this suite pins:
//   - handleUpdateUser did an unconditional SET username=?, role=? — a role-only
//     edit blanked the username (and vice-versa), never 404'd a missing id, and
//     returned an echo of the request with a fabricated created_at:0.
//   - handleCreateUser / handleUpdateUser accepted any role string.
//   - a duplicate username surfaced as 500 instead of 409.
//
// (The stored role is a display/registry value — OIDC computes the session scope
// from IdP groups on each login and never reads it — so these are correctness/UX
// fixes, not an authz change.)
package api_test

import (
	"net/http"
	"testing"
)

func createUser(t *testing.T, base, token, username, role string) jsonResp {
	t.Helper()
	return doJSON(t, http.DefaultClient, http.MethodPost, base+"/api/v1/admin/users", token, map[string]any{
		"username": username, "role": role, "password": "pw",
	})
}

// TestUsers_UpdateMissingField_400: a role-only PUT (username omitted) must 400 —
// this is what prevents the old blanking bug. UserWrite declares
// required:[username,role], so PUT is a full replace and both fields are required.
// Mutation proof: the old handler accepted it (200) and set username="".
func TestUsers_UpdateMissingField_400(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	created := createUser(t, ts.URL, token, "alice", "admin")
	if created.status != http.StatusCreated {
		t.Fatalf("create user: expected 201, got %d: %s", created.status, created.body)
	}
	id, _ := created.json["id"].(string)

	upd := doJSON(t, http.DefaultClient, http.MethodPut, ts.URL+"/api/v1/admin/users/"+id, token,
		map[string]any{"role": "viewer"}) // username omitted
	if upd.status != http.StatusBadRequest {
		t.Fatalf("role-only update: expected 400, got %d: %s", upd.status, upd.body)
	}
}

// TestUsers_Update_FullReplace_HonestResponse: a full PUT (both fields) updates the
// role and returns the REAL stored row — username intact and a real created_at, not
// the old fabricated echo with created_at:0.
func TestUsers_Update_FullReplace_HonestResponse(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	created := createUser(t, ts.URL, token, "alice", "admin")
	id, _ := created.json["id"].(string)
	if id == "" {
		t.Fatalf("no id in create response: %s", created.body)
	}

	upd := doJSON(t, http.DefaultClient, http.MethodPut, ts.URL+"/api/v1/admin/users/"+id, token,
		map[string]any{"username": "alice", "role": "viewer"})
	if upd.status != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", upd.status, upd.body)
	}
	if upd.json["username"] != "alice" {
		t.Errorf("username: got %q, want \"alice\"", upd.json["username"])
	}
	if upd.json["role"] != "viewer" {
		t.Errorf("role not updated: got %q, want \"viewer\"", upd.json["role"])
	}
	if ca, _ := upd.json["created_at"].(float64); ca == 0 {
		t.Errorf("response has fabricated created_at:0 instead of the stored value")
	}
}

// TestUsers_UpdateMissing_404: updating a non-existent id must 404, not 200.
func TestUsers_UpdateMissing_404(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	upd := doJSON(t, http.DefaultClient, http.MethodPut, ts.URL+"/api/v1/admin/users/does-not-exist", token,
		map[string]any{"role": "viewer"})
	if upd.status != http.StatusNotFound {
		t.Fatalf("update missing user: expected 404, got %d: %s", upd.status, upd.body)
	}
	if upd.json["code"] != "NOT_FOUND" {
		t.Errorf("expected code=NOT_FOUND, got %v", upd.json["code"])
	}
}

// TestUsers_CreateInvalidRole_400: an unknown role is a client error.
func TestUsers_CreateInvalidRole_400(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	res := createUser(t, ts.URL, token, "bob", "superadmin")
	if res.status != http.StatusBadRequest {
		t.Fatalf("create invalid role: expected 400, got %d: %s", res.status, res.body)
	}
	if res.json["code"] != "INVALID_PARAM" {
		t.Errorf("expected code=INVALID_PARAM, got %v", res.json["code"])
	}
}

// TestUsers_UpdateInvalidRole_400: an unknown role on update is rejected before persisting.
func TestUsers_UpdateInvalidRole_400(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	created := createUser(t, ts.URL, token, "carol", "admin")
	id, _ := created.json["id"].(string)

	upd := doJSON(t, http.DefaultClient, http.MethodPut, ts.URL+"/api/v1/admin/users/"+id, token,
		map[string]any{"role": "root"})
	if upd.status != http.StatusBadRequest {
		t.Fatalf("update invalid role: expected 400, got %d: %s", upd.status, upd.body)
	}
}

// TestUsers_CreateDuplicate_409: a duplicate username is a 409, not a 500.
func TestUsers_CreateDuplicate_409(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	if first := createUser(t, ts.URL, token, "dave", "viewer"); first.status != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", first.status, first.body)
	}
	dup := createUser(t, ts.URL, token, "dave", "viewer")
	if dup.status != http.StatusConflict {
		t.Fatalf("duplicate create: expected 409, got %d: %s", dup.status, dup.body)
	}
	if dup.json["code"] != "CONFLICT" {
		t.Errorf("expected code=CONFLICT, got %v", dup.json["code"])
	}
}
