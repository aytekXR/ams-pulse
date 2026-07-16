// S47 (D-109) — audit-integrity + hardening cluster (final S44-audit findings).
//
// Findings pinned here (1a/1b/3 are mutation-proven; 2 is happy-path only — see
// the scope note on TestAudit_TokenCreate_Recorded and D-109):
//
//	1a. handleDeleteUser audited a phantom user.delete for a non-existent id.
//	1b. handleRevokeToken audited a phantom token.revoke for a non-existent id.
//	2.  handleCreateToken/handleCreateUser now audit the committed create BEFORE the
//	    response re-fetch (S40 class). This file adds the previously-missing
//	    create-token audit coverage (user.create is covered by audit_s40_test.go).
//	3.  handleCreateToken accepted an arbitrary kind (no allowlist).
//
// The delete/revoke routes are IDEMPOTENT by contract (204 for a missing id — see
// the OpenAPI descriptions), so the fix keeps the 204 and only suppresses the
// phantom audit entry. Each negative case is paired with a positive control so the
// "no phantom entry" assertion can't pass vacuously.
package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// createToken POSTs a token and returns the response (id + raw token in body).
func createToken(t *testing.T, base, token, kind, name string) jsonResp {
	t.Helper()
	return doJSON(t, http.DefaultClient, http.MethodPost, base+"/api/v1/admin/tokens", token,
		map[string]any{"kind": kind, "name": name})
}

// TestAudit_DeleteMissingUser_NoPhantomEntry: deleting a non-existent user is 204
// (idempotent) but must NOT write a user.delete audit entry; deleting a real user
// (positive control) MUST. Mutation: drop the `if err == nil` guard in
// handleDeleteUser → the phantom entry appears → the negative assertion goes RED.
func TestAudit_DeleteMissingUser_NoPhantomEntry(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	const bogus = "00000000-0000-4000-8000-000000000000"
	del := doJSON(t, http.DefaultClient, http.MethodDelete, ts.URL+"/api/v1/admin/users/"+bogus, token, nil)
	if del.status != http.StatusNoContent {
		t.Fatalf("delete missing user: expected 204 (idempotent per contract), got %d: %s", del.status, del.body)
	}

	// Positive control: a real user delete DOES audit.
	created := createUser(t, ts.URL, token, "s47-del-alice", "admin")
	uid, _ := created.json["id"].(string)
	if uid == "" {
		t.Fatalf("no id in create response: %s", created.body)
	}
	realDel := doJSON(t, http.DefaultClient, http.MethodDelete, ts.URL+"/api/v1/admin/users/"+uid, token, nil)
	if realDel.status != http.StatusNoContent {
		t.Fatalf("delete real user: expected 204, got %d: %s", realDel.status, realDel.body)
	}

	al := getAuditLog(t, ts.URL, token)
	items, _ := al.json["items"].([]any)
	if findAuditEntry(items, "user.delete", bogus) != nil {
		t.Errorf("phantom user.delete audit entry recorded for a non-existent id %s; body=%s", bogus, al.body)
	}
	if findAuditEntry(items, "user.delete", uid) == nil {
		t.Errorf("positive control failed: no user.delete entry for the real delete %s — the negative assertion is vacuous; body=%s", uid, al.body)
	}
}

// TestAudit_RevokeMissingToken_NoPhantomEntry: same shape for token revoke (S44
// finding 1b — the split-verdict one, confirmed real). Mutation: drop the
// `if err == nil` guard in handleRevokeToken → phantom token.revoke → RED.
func TestAudit_RevokeMissingToken_NoPhantomEntry(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	const bogus = "11111111-1111-4111-8111-111111111111"
	rev := doJSON(t, http.DefaultClient, http.MethodDelete, ts.URL+"/api/v1/admin/tokens/"+bogus, token, nil)
	if rev.status != http.StatusNoContent {
		t.Fatalf("revoke missing token: expected 204 (idempotent per contract), got %d: %s", rev.status, rev.body)
	}

	// Positive control: mint a real token, revoke it — that DOES audit.
	ct := createToken(t, ts.URL, token, "api", "s47-revoke-target")
	if ct.status != http.StatusCreated {
		t.Fatalf("create token: expected 201, got %d: %s", ct.status, ct.body)
	}
	tid, _ := ct.json["id"].(string)
	if tid == "" {
		t.Fatalf("no id in token create response: %s", ct.body)
	}
	realRev := doJSON(t, http.DefaultClient, http.MethodDelete, ts.URL+"/api/v1/admin/tokens/"+tid, token, nil)
	if realRev.status != http.StatusNoContent {
		t.Fatalf("revoke real token: expected 204, got %d: %s", realRev.status, realRev.body)
	}

	al := getAuditLog(t, ts.URL, token)
	items, _ := al.json["items"].([]any)
	if findAuditEntry(items, "token.revoke", bogus) != nil {
		t.Errorf("phantom token.revoke audit entry recorded for a non-existent id %s; body=%s", bogus, al.body)
	}
	if findAuditEntry(items, "token.revoke", tid) == nil {
		t.Errorf("positive control failed: no token.revoke entry for the real revoke %s — the negative assertion is vacuous; body=%s", tid, al.body)
	}
}

// TestAudit_TokenCreate_Recorded: creating a token writes a token.create entry
// naming the actor/object, and never leaks the raw token. Mutation: delete the
// s.audit call in handleCreateToken → RED.
//
// SCOPE NOTE (S47 review): this proves creates ARE audited — it does NOT
// discriminate the audit-BEFORE-re-fetch ordering (finding 2). In the happy path
// (LookupToken succeeds) both orderings write the same entry, so this test stays
// green either way. The ordering's only observable difference is when the re-fetch
// returns nil after a committed create (a concurrent-delete race), which is not
// reachable through the HTTP surface with a concrete *meta.Store. The reorder
// mirrors the already-proven S40 update-path fix; see D-109 for why it is not
// independently mutation-provable here.
func TestAudit_TokenCreate_Recorded(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	ct := createToken(t, ts.URL, token, "api", "s47-created-token")
	if ct.status != http.StatusCreated {
		t.Fatalf("create token: expected 201, got %d: %s", ct.status, ct.body)
	}
	tid, _ := ct.json["id"].(string)
	if tid == "" {
		t.Fatalf("no id in token create response: %s", ct.body)
	}

	al := getAuditLog(t, ts.URL, token)
	items, _ := al.json["items"].([]any)
	entry := findAuditEntry(items, "token.create", tid)
	if entry == nil {
		t.Fatalf("no token.create audit entry for %s; body=%s", tid, al.body)
	}
	if entry["object_type"] != "token" {
		t.Errorf("object_type: got %v, want token", entry["object_type"])
	}
	// The detail carries name/kind — never the raw token or its hash.
	if detail, ok := entry["detail"].(map[string]any); ok {
		if detail["name"] != "s47-created-token" {
			t.Errorf("detail.name: got %v, want s47-created-token", detail["name"])
		}
		if _, leaked := detail["token"]; leaked {
			t.Error("audit detail leaked the raw token")
		}
	}
}

// TestCreateToken_KindAllowlist: kind is a positive allowlist (D-098). Only "api"
// and "ingest" are storable; anything else is 422. Mutation: remove the allowlist
// block → kind:"superadmin" returns 201 (a dead token row) → RED.
func TestCreateToken_KindAllowlist(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	for _, kind := range []string{"api", "ingest"} {
		resp := createToken(t, ts.URL, token, kind, "ok-"+kind)
		if resp.status != http.StatusCreated {
			t.Errorf("kind=%q: expected 201, got %d: %s", kind, resp.status, resp.body)
		}
	}

	for _, bad := range []string{"superadmin", "root", "API", "admin"} {
		resp := createToken(t, ts.URL, token, bad, "bad-"+bad)
		if resp.status != http.StatusUnprocessableEntity {
			t.Errorf("kind=%q: expected 422 (allowlist), got %d: %s", bad, resp.status, resp.body)
		}
	}

	// Empty kind is still a 400 (required-field), distinct from the 422 allowlist.
	empty := doJSON(t, http.DefaultClient, http.MethodPost, ts.URL+"/api/v1/admin/tokens", token,
		map[string]any{"name": "no-kind"})
	if empty.status != http.StatusBadRequest {
		t.Errorf("empty kind: expected 400, got %d: %s", empty.status, empty.body)
	}
}

// TestCreateUser_RejectsOverlongPassword: a password past bcrypt's 72-byte limit
// is a 422, not a silently-created user with an unusable (empty) hash (D-109 —
// CodeQL-surfaced). Mutation: remove the length guard in handleCreateUser → the
// over-long create returns 201 → RED.
func TestCreateUser_RejectsOverlongPassword(t *testing.T) {
	ts, token, cleanup := setupTestServer(t)
	defer cleanup()

	over := doJSON(t, http.DefaultClient, http.MethodPost, ts.URL+"/api/v1/admin/users", token,
		map[string]any{"username": "s47-longpw", "role": "viewer", "password": strings.Repeat("x", 100)})
	if over.status != http.StatusUnprocessableEntity {
		t.Errorf("over-long password: expected 422, got %d: %s", over.status, over.body)
	}

	// A normal password still creates the user (positive control).
	ok := doJSON(t, http.DefaultClient, http.MethodPost, ts.URL+"/api/v1/admin/users", token,
		map[string]any{"username": "s47-okpw", "role": "viewer", "password": "fine-password"})
	if ok.status != http.StatusCreated {
		t.Errorf("normal password: expected 201, got %d: %s", ok.status, ok.body)
	}
}
