package meta

// audit.go — append-only audit trail (S40 / D-102).
//
// Records "who changed what, when" for every mutating admin/config API call.
// Rows are only ever INSERTed; the store exposes no update or delete. The table
// has NO foreign keys to api_tokens/users so a row survives token revocation and
// user deletion — precisely the moments an audit trail must remain intact.

import (
	"context"
	"fmt"
)

// AuditEntry is one row of the audit_log table.
type AuditEntry struct {
	ID           string // UUID (assigned by CreateAuditLog when empty)
	TS           int64  // Unix epoch ms (assigned by CreateAuditLog when zero)
	ActorTokenID string // api_tokens.id of the caller
	ActorUserID  string // users.id for OIDC sessions; "" for service tokens
	ActorName    string // token display name
	Action       string // e.g. "alert_rule.create"
	ObjectType   string // e.g. "alert_rule"
	ObjectID     string // affected resource id ("" for none)
	RemoteAddr   string // request source IP
	DetailJSON   string // optional JSON context ("" when absent)
}

// CreateAuditLog inserts one audit entry. ID and TS are filled in when unset so
// callers only need to populate the actor/action/object fields.
func (s *Store) CreateAuditLog(ctx context.Context, e AuditEntry) error {
	if e.ID == "" {
		e.ID = newUUID()
	}
	if e.TS == 0 {
		e.TS = nowMS()
	}
	_, err := s.execContext(ctx,
		`INSERT INTO audit_log
		   (id, ts, actor_token_id, actor_user_id, actor_name, action, object_type, object_id, remote_addr, detail_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.TS, e.ActorTokenID, e.ActorUserID, e.ActorName,
		e.Action, e.ObjectType, e.ObjectID, e.RemoteAddr, e.DetailJSON)
	return err
}

// ListAuditLog returns audit entries newest-first (ts DESC, id DESC) using the
// same opaque "ts:id" keyset cursor as the other list endpoints. limit<=0 means
// no LIMIT; cursor="" means the first (newest) page.
func (s *Store) ListAuditLog(ctx context.Context, limit int, cursor string) ([]AuditEntry, error) {
	q := `SELECT id, ts, actor_token_id, actor_user_id, actor_name, action, object_type, object_id, remote_addr, detail_json
	      FROM audit_log WHERE 1=1`
	var args []any
	// Newest-first: the cursor bounds the page from above, so the predicate is
	// "strictly older than the last row we returned" — the DESC mirror of the
	// ASC (created_at > ?) pattern used by ListUsers / ListAlertChannels.
	if ts, id := parseKeysetCursor(cursor); id != "" {
		q += " AND (ts < ? OR (ts = ? AND id < ?))"
		args = append(args, ts, ts, id)
	}
	q += " ORDER BY ts DESC, id DESC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.TS, &e.ActorTokenID, &e.ActorUserID, &e.ActorName,
			&e.Action, &e.ObjectType, &e.ObjectID, &e.RemoteAddr, &e.DetailJSON); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
