package meta_test

// audit_test.go — S40 (D-102) audit_log store round-trip, ordering and pagination.

import (
	"context"
	"fmt"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// TestAuditLog_RoundTrip: every field persists and ID/TS are auto-assigned.
func TestAuditLog_RoundTrip(t *testing.T) {
	store := openMetaTestStore(t)
	ctx := context.Background()

	in := meta.AuditEntry{
		ActorTokenID: "tok-1", ActorUserID: "user-1", ActorName: "alice",
		Action: "alert_rule.create", ObjectType: "alert_rule", ObjectID: "rule-1",
		RemoteAddr: "10.0.0.1", DetailJSON: `{"name":"cpu"}`,
	}
	if err := store.CreateAuditLog(ctx, in); err != nil {
		t.Fatalf("CreateAuditLog: %v", err)
	}
	got, err := store.ListAuditLog(ctx, 10, "")
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	g := got[0]
	if g.ID == "" {
		t.Error("ID was not auto-assigned")
	}
	if g.TS == 0 {
		t.Error("TS was not auto-assigned")
	}
	if g.Action != "alert_rule.create" || g.ObjectType != "alert_rule" || g.ObjectID != "rule-1" {
		t.Errorf("action/object mismatch: %+v", g)
	}
	if g.ActorName != "alice" || g.ActorTokenID != "tok-1" || g.ActorUserID != "user-1" {
		t.Errorf("actor mismatch: %+v", g)
	}
	if g.RemoteAddr != "10.0.0.1" || g.DetailJSON != `{"name":"cpu"}` {
		t.Errorf("remote_addr/detail mismatch: %+v", g)
	}
}

// TestAuditLog_NewestFirst: entries return in ts-descending order.
func TestAuditLog_NewestFirst(t *testing.T) {
	store := openMetaTestStore(t)
	ctx := context.Background()

	for i, ts := range []int64{100, 200, 300} {
		if err := store.CreateAuditLog(ctx, meta.AuditEntry{
			ID: fmt.Sprintf("id-%d", i), TS: ts, Action: "x.create", ObjectType: "x",
		}); err != nil {
			t.Fatalf("CreateAuditLog: %v", err)
		}
	}
	got, err := store.ListAuditLog(ctx, 10, "")
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0].TS != 300 || got[1].TS != 200 || got[2].TS != 100 {
		t.Errorf("not newest-first: %d, %d, %d", got[0].TS, got[1].TS, got[2].TS)
	}
}

// TestAuditLog_CursorPagination: the "ts:id" keyset cursor walks the full set
// newest-first with no gaps or repeats.
func TestAuditLog_CursorPagination(t *testing.T) {
	store := openMetaTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := store.CreateAuditLog(ctx, meta.AuditEntry{
			ID: fmt.Sprintf("id-%d", i), TS: int64(100 + i), Action: "x.create", ObjectType: "x",
		}); err != nil {
			t.Fatalf("CreateAuditLog: %v", err)
		}
	}

	page1, err := store.ListAuditLog(ctx, 2, "")
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 || page1[0].TS != 104 || page1[1].TS != 103 {
		t.Fatalf("page1 wrong: %+v", page1)
	}

	cursor := fmt.Sprintf("%d:%s", page1[1].TS, page1[1].ID)
	page2, err := store.ListAuditLog(ctx, 2, cursor)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 2 || page2[0].TS != 102 || page2[1].TS != 101 {
		t.Fatalf("page2 wrong: %+v", page2)
	}

	cursor2 := fmt.Sprintf("%d:%s", page2[1].TS, page2[1].ID)
	page3, err := store.ListAuditLog(ctx, 2, cursor2)
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page3) != 1 || page3[0].TS != 100 {
		t.Fatalf("page3 wrong (want single TS=100): %+v", page3)
	}
}
