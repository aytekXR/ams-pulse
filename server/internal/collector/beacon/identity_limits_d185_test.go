package beacon

// identity_limits_d185_test.go — D-185: bound the beacon identity fields.
//
// Round 11 re-confirmed M-04 (unbounded session_id/stream_id) as "an honest
// deferral", on the reasoning that truncating a session_id corrupts
// uniq(session_id). The reasoning is right about truncation and does not reach
// rejection, which has no such effect — and the deferral was priced without the
// amplification: the identity fields are copied onto EVERY row a batch produces,
// so one 64 KB request inside the body cap expands to megabytes of storage.
//
// These tests pin the rejection, the generous headroom for real clients, and the
// fact that a conforming batch is untouched.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aytekXR/ams-pulse/server/internal/domain"
)

func rawBatch(t *testing.T, sessionID, streamID, app string) []byte {
	t.Helper()
	b := map[string]any{
		"version":    1,
		"session_id": sessionID,
		"stream_id":  streamID,
		"events":     []map[string]any{{"type": "heartbeat", "ts": 1700000000000, "data": map[string]any{"watch_ms": 1000}}},
	}
	if app != "" {
		b["app"] = app
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// TestValidate_RejectsOversizedIdentityFields is the amplification fix. A 30 KB
// session_id fits inside the 64 KB body cap and would be written onto all 100
// rows of a full batch.
func TestValidate_RejectsOversizedIdentityFields(t *testing.T) {
	cases := []struct {
		name                 string
		session, stream, app string
		wantField            string
	}{
		{"session_id", strings.Repeat("a", maxIdentityLen+1), "s1", "", "session_id"},
		{"stream_id", "sess-1", strings.Repeat("b", maxIdentityLen+1), "", "stream_id"},
		{"app", "sess-1", "s1", strings.Repeat("c", maxIdentityLen+1), "app"},
		{"realistic amplification payload", strings.Repeat("a", 30000), "s1", "", "session_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jsonErr, errs := ValidateRawBatch(rawBatch(t, tc.session, tc.stream, tc.app))
			if jsonErr != nil {
				t.Fatalf("body should still decode: %v", jsonErr)
			}
			if len(errs) == 0 {
				t.Fatalf("oversized %s was accepted", tc.wantField)
			}
			found := false
			for _, e := range errs {
				if strings.Contains(e, tc.wantField) && strings.Contains(e, "must not exceed") {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected a length error naming %s, got %v", tc.wantField, errs)
			}
		})
	}
}

// TestValidate_AcceptsAtCapAndTypicalIDs pins the headroom: the cap is ~7× a
// UUIDv4, so no conforming client can reach it, and exactly-at-cap is valid
// (an off-by-one here would reject real traffic).
func TestValidate_AcceptsAtCapAndTypicalIDs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		session string
	}{
		{"uuidv4", "3f2504e0-4f89-11d3-9a0c-0305e82c3301"},
		{"exactly at cap", strings.Repeat("a", maxIdentityLen)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jsonErr, errs := ValidateRawBatch(rawBatch(t, tc.session, "stream-1", "LiveApp"))
			if jsonErr != nil {
				t.Fatalf("decode: %v", jsonErr)
			}
			if len(errs) != 0 {
				t.Fatalf("conforming batch rejected: %v", errs)
			}
		})
	}
}

// TestValidate_RejectionIsNotTruncation is the property M-04 actually cared
// about: two distinct sessions must never collapse onto one key. Rejection keeps
// uniq(session_id) exact by construction — nothing oversized is ever stored.
func TestValidate_RejectionIsNotTruncation(t *testing.T) {
	a := strings.Repeat("x", maxIdentityLen) + "-alpha"
	b := strings.Repeat("x", maxIdentityLen) + "-beta"
	if a == b {
		t.Fatal("test setup: the two ids must differ only past the cap")
	}
	for _, id := range []string{a, b} {
		_, errs := ValidateRawBatch(rawBatch(t, id, "s1", ""))
		if len(errs) == 0 {
			t.Fatalf("id past the cap must be rejected, not stored: %q", id[:20]+"…")
		}
	}
}

// TestApplyA10FieldLimits_TruncatesPlayerKind covers the one remaining unbounded
// per-row string. player_kind is not an identity key — no aggregate counts
// distinct player kinds — so truncation is the right treatment here, unlike the
// fields above.
func TestApplyA10FieldLimits_TruncatesPlayerKind(t *testing.T) {
	ev := &domain.BeaconEvent{PlayerKind: strings.Repeat("k", maxStringDataValueLen*4)}
	ApplyA10FieldLimits(ev)
	if len(ev.PlayerKind) > maxStringDataValueLen {
		t.Fatalf("player_kind not bounded: %d bytes", len(ev.PlayerKind))
	}
}

// TestApplyA10FieldLimits_LeavesIdentityFieldsAlone is the regression guard on
// the rule itself: if a later change starts truncating these, uniq(session_id)
// silently stops being exact and no aggregate test would notice.
func TestApplyA10FieldLimits_LeavesIdentityFieldsAlone(t *testing.T) {
	long := strings.Repeat("s", 1000)
	ev := &domain.BeaconEvent{SessionID: long, StreamID: long, App: long}
	ApplyA10FieldLimits(ev)
	for name, got := range map[string]string{"session_id": ev.SessionID, "stream_id": ev.StreamID, "app": ev.App} {
		if got != long {
			t.Errorf("%s was truncated (%d bytes) — identity fields must be rejected upstream, never truncated", name, len(got))
		}
	}
}

// TestValidate_AmplificationIsBounded states the resulting bound as an
// arithmetic fact rather than a claim in a comment.
//
// The cap is in CHARACTERS (JSON Schema maxLength semantics), so the byte bound
// is 4× that in the worst case — a 4-byte UTF-8 rune throughout. Asserting
// against maxIdentityLen alone would silently assume 1 byte per character and
// understate the true bound by 4×, which is the mistake this test exists to not
// make.
func TestValidate_AmplificationIsBounded(t *testing.T) {
	const maxBytesPerRune = 4
	const identityFields = 3 // session_id + stream_id + app
	worst := identityFields * maxIdentityLen * maxBytesPerRune * maxEventsPerBatch
	// Before D-185 this was unbounded above (limited only by the 64 KB body cap
	// times 100 rows ≈ 6 MB). The bound that matters is that it is now a small
	// multiple of the request, not that it is below it.
	if worst > 8*maxBodyBytes {
		t.Fatalf("identity amplification exceeds 8× the body cap: %d bytes from a %d byte request", worst, maxBodyBytes)
	}
	t.Logf("worst-case identity storage per request: %s", fmt.Sprintf("%d bytes, i.e. %.1f× the %d byte body cap",
		worst, float64(worst)/float64(maxBodyBytes), maxBodyBytes))
}

// TestValidate_CapIsCharactersNotBytes is the contract-consistency test. JSON
// Schema defines maxLength in characters; if the server counted bytes, a payload
// the published contract calls VALID would be rejected. A contract and its
// implementation disagreeing about what they accept is a defect even when the
// implementation is the stricter one.
func TestValidate_CapIsCharactersNotBytes(t *testing.T) {
	// 256 four-byte runes = 256 characters (valid per the contract) = 1024 bytes.
	id := strings.Repeat("𝄞", maxIdentityLen)
	if len(id) <= maxIdentityLen {
		t.Fatalf("test setup: expected a multi-byte string longer in bytes than in runes, got %d bytes", len(id))
	}
	jsonErr, errs := ValidateRawBatch(rawBatch(t, id, "s1", ""))
	if jsonErr != nil {
		t.Fatalf("decode: %v", jsonErr)
	}
	if len(errs) != 0 {
		t.Fatalf("a %d-character id is valid per contracts/events/beacon-event.schema.json (maxLength 256) but was rejected: %v",
			maxIdentityLen, errs)
	}

	// One character past the cap must still be refused.
	_, errs = ValidateRawBatch(rawBatch(t, id+"𝄞", "s1", ""))
	if len(errs) == 0 {
		t.Fatal("a 257-character id must be rejected")
	}
}
