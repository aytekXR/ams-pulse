package meta_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// TestVodPollState_SeenSet exercises the seen-set deduplication methods:
//   - MarkVodSeen is idempotent (second call with same key is a no-op)
//   - ListSeenVodIDs returns exactly one entry per unique (app, vod_id)
//   - Different apps are isolated (app-A's IDs don't appear under app-B)
//   - Unknown app returns empty non-nil map, no error
func TestVodPollState_SeenSet(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	// ── 1. Idempotency: same (app, vod_id) twice → exactly 1 entry ─────────────
	const app1 = "app-alpha"
	const vodID1 = "vod-001"

	if err := s.MarkVodSeen(ctx, app1, vodID1, 1000); err != nil {
		t.Fatalf("MarkVodSeen first call: %v", err)
	}
	if err := s.MarkVodSeen(ctx, app1, vodID1, 2000); err != nil {
		t.Fatalf("MarkVodSeen second call (same key): %v", err)
	}

	seen, err := s.ListSeenVodIDs(ctx, app1)
	if err != nil {
		t.Fatalf("ListSeenVodIDs after double-mark: %v", err)
	}
	if len(seen) != 1 {
		t.Errorf("expected exactly 1 entry after double-mark, got %d", len(seen))
	}
	if _, ok := seen[vodID1]; !ok {
		t.Errorf("expected vod_id %q in seen set, got %v", vodID1, seen)
	}

	// ── 2. Multiple distinct VoDs for same app ───────────────────────────────────
	const vodID2 = "vod-002"
	if err := s.MarkVodSeen(ctx, app1, vodID2, 3000); err != nil {
		t.Fatalf("MarkVodSeen second distinct vod: %v", err)
	}

	seen2, err := s.ListSeenVodIDs(ctx, app1)
	if err != nil {
		t.Fatalf("ListSeenVodIDs after second vod: %v", err)
	}
	if len(seen2) != 2 {
		t.Errorf("expected 2 entries after marking two vods, got %d", len(seen2))
	}
	if _, ok := seen2[vodID1]; !ok {
		t.Errorf("expected vod_id %q still present", vodID1)
	}
	if _, ok := seen2[vodID2]; !ok {
		t.Errorf("expected vod_id %q present", vodID2)
	}

	// ── 3. App isolation ─────────────────────────────────────────────────────────
	// Same vod_id under a different app should be independent.
	const app2 = "app-beta"
	if err := s.MarkVodSeen(ctx, app2, vodID1, 5000); err != nil {
		t.Fatalf("MarkVodSeen for app2: %v", err)
	}

	seenApp1, err := s.ListSeenVodIDs(ctx, app1)
	if err != nil {
		t.Fatalf("ListSeenVodIDs app1 after app2 insert: %v", err)
	}
	if len(seenApp1) != 2 {
		t.Errorf("app1 should still have 2 entries (isolated from app2), got %d", len(seenApp1))
	}

	seenApp2, err := s.ListSeenVodIDs(ctx, app2)
	if err != nil {
		t.Fatalf("ListSeenVodIDs app2: %v", err)
	}
	if len(seenApp2) != 1 {
		t.Errorf("app2 should have 1 entry, got %d", len(seenApp2))
	}
	if _, ok := seenApp2[vodID1]; !ok {
		t.Errorf("expected vod_id %q under app2", vodID1)
	}

	// ── 4. Unknown app → empty non-nil map, no error ─────────────────────────────
	seenUnknown, err := s.ListSeenVodIDs(ctx, "app-unknown-xyz")
	if err != nil {
		t.Fatalf("ListSeenVodIDs unknown app: %v", err)
	}
	if seenUnknown == nil {
		t.Error("expected non-nil map for unknown app, got nil")
	}
	if len(seenUnknown) != 0 {
		t.Errorf("expected empty map for unknown app, got %v", seenUnknown)
	}
}

// TestEmbeddedDDLPostgres_ContainsVodPollState pins the embed_pg.go chain:
// removing embeddedPGDDL0003 from EmbeddedDDLPostgres would ship a Postgres
// meta store without the vod_poll_state table (first MarkVodSeen call would
// fail with "no such table") while every SQLite-path test stays green. This
// guard catches the chain omission without needing a live Postgres.
func TestEmbeddedDDLPostgres_ContainsVodPollState(t *testing.T) {
	if !strings.Contains(meta.EmbeddedDDLPostgres, "vod_poll_state") {
		t.Fatal("EmbeddedDDLPostgres does not contain vod_poll_state — " +
			"embeddedPGDDL0003 missing from the embed_pg.go concatenation chain")
	}
}
