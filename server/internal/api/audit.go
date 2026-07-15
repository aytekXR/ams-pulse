package api

// audit.go — audit-trail capture helper + read endpoint (S40 / D-102).
//
// Every mutating admin/config handler calls s.audit(...) after its store write
// succeeds, recording who made the change. The actor is already in the request
// context (bearerAuthMiddleware stashes the *meta.APIToken under ctxTokenKey), so
// no new middleware is needed. GET /api/v1/admin/audit-log reads the trail back.
//
// Scope (v1, deliberate — NOT silent gaps):
//   Covered: create/update/delete of alert rules & channels, users, tokens,
//     probes, report schedules, AMS sources, tenants; plus licence activation.
//   Out of scope, by design:
//     - Test/connectivity actions (POST .../channels/{id}/test, .../sources/{id}/test)
//       — they fire a probe, they do not change stored state.
//     - Session logout (POST /auth/oidc/logout) — no resource is mutated.
//     - OIDC auto-provisioning (oidc.go creates a user on first SSO login) — the
//       actor there is the SSO flow, not an admin token, so it needs a distinct
//       actor model. This is the top Phase-2 follow-up (see D-102).

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// actorFrom extracts the audit actor from the request context. Every mutating
// route runs behind bearerAuthMiddleware, so the token is present in practice; if
// it is somehow absent the actor fields are left empty — losing the actor is
// still better than losing the record, so the audit row is written regardless.
func actorFrom(r *http.Request) (tokenID, userID, name, remoteAddr string) {
	remoteAddr = r.RemoteAddr
	if tok, _ := r.Context().Value(ctxTokenKey).(*meta.APIToken); tok != nil {
		tokenID = tok.ID
		userID = tok.UserID
		name = tok.Name
	}
	return tokenID, userID, name, remoteAddr
}

// audit records one audit_log entry for a successful mutating operation.
//
// It is best-effort: a failure to write the audit row is logged at ERROR but does
// NOT fail the caller's request. The mutation has already committed by the time
// audit is called, so failing here would report a false error for a change that
// actually happened; audit availability must not become a write-path SPOF.
//
// The insert uses a cancel-detached context so a client that disconnects the
// instant after its write still leaves a durable audit record.
//
// detail is an optional value serialised to JSON ("" when nil or unmarshalable).
func (s *Server) audit(r *http.Request, action, objectType, objectID string, detail any) {
	tokenID, userID, name, remoteAddr := actorFrom(r)
	detailJSON := ""
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			detailJSON = string(b)
		}
	}
	e := meta.AuditEntry{
		ActorTokenID: tokenID,
		ActorUserID:  userID,
		ActorName:    name,
		Action:       action,
		ObjectType:   objectType,
		ObjectID:     objectID,
		RemoteAddr:   remoteAddr,
		DetailJSON:   detailJSON,
	}
	// Detach from request cancellation (a client that disconnects the instant after
	// its write must still leave a durable record) but keep a bound so a stalled
	// store write cannot hang the response indefinitely.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	if err := s.store.CreateAuditLog(ctx, e); err != nil {
		slog.Error("api: audit log write failed",
			"error", err, "action", action, "object_type", objectType, "object_id", objectID)
	}
}

// handleListAuditLog returns audit entries newest-first with opaque keyset
// pagination, matching the {items, meta:{next_cursor}} shape of the other list
// endpoints.
//
//	GET /api/v1/admin/audit-log?limit=&cursor=
func (s *Server) handleListAuditLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	cursor := q.Get("cursor")
	entries, err := s.store.ListAuditLog(r.Context(), limit+1, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	var nextCursor *string
	if len(entries) > limit {
		entries = entries[:limit]
		last := entries[len(entries)-1]
		c := fmt.Sprintf("%d:%s", last.TS, last.ID)
		nextCursor = &c
	}
	items := make([]any, 0, len(entries))
	for _, e := range entries {
		items = append(items, auditEntryToAPI(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"meta":  map[string]any{"next_cursor": nextCursor},
	})
}

// auditEntryToAPI maps a stored audit entry to its JSON shape. detail is emitted
// only when present, and as raw JSON so it round-trips as an object, not a string.
func auditEntryToAPI(e meta.AuditEntry) map[string]any {
	m := map[string]any{
		"id":             e.ID,
		"ts":             e.TS,
		"actor_token_id": e.ActorTokenID,
		"actor_user_id":  e.ActorUserID,
		"actor_name":     e.ActorName,
		"action":         e.Action,
		"object_type":    e.ObjectType,
		"object_id":      e.ObjectID,
		"remote_addr":    e.RemoteAddr,
	}
	if e.DetailJSON != "" {
		m["detail"] = json.RawMessage(e.DetailJSON)
	}
	return m
}
