package tenant

import (
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

func rows() []meta.TenantRow {
	return []meta.TenantRow{
		{ID: "t1", Name: "acme", StreamPattern: "acme-*"},
		{ID: "t2", Name: "globex", StreamPattern: "gx_%"},
		{ID: "t3", Name: "meta-tenant", MetaTagKey: "org", MetaTagValue: "umbrella"},
	}
}

func TestMatcher_Resolve(t *testing.T) {
	m := NewMatcher(rows())
	cases := []struct {
		name     string
		streamID string
		meta     map[string]string
		want     string
	}{
		{"glob star prefix", "acme-live1", nil, "acme"},
		{"glob star case-insensitive", "ACME-Live1", nil, "acme"},
		{"glob percent", "gx_room42", nil, "globex"},
		{"no match → unassigned", "random-stream", nil, ""},
		{"meta-tag wins over no glob", "random-stream", map[string]string{"org": "umbrella"}, "meta-tenant"},
		{"meta-tag precedence over glob", "acme-live1", map[string]string{"org": "umbrella"}, "meta-tenant"},
		{"meta present but no rule match falls to glob", "acme-live1", map[string]string{"org": "other"}, "acme"},
		{"underscore matches one char", "gx_a", nil, "globex"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := m.Resolve(tc.streamID, tc.meta); got != tc.want {
				t.Fatalf("Resolve(%q, %v) = %q, want %q", tc.streamID, tc.meta, got, tc.want)
			}
		})
	}
}

func TestMatcher_EmptyRegistry(t *testing.T) {
	if got := NewMatcher(nil).Resolve("anything", map[string]string{"k": "v"}); got != "" {
		t.Fatalf("empty matcher resolved to %q, want empty", got)
	}
}

// A tenant row with only a meta-tag rule must NOT match a live stream (nil meta).
func TestMatcher_MetaOnlyRule_DoesNotMatchLiveStream(t *testing.T) {
	m := NewMatcher([]meta.TenantRow{{ID: "t", Name: "x", MetaTagKey: "org", MetaTagValue: "y"}})
	if got := m.Resolve("stream-1", nil); got != "" {
		t.Fatalf("meta-only rule matched a live (no-meta) stream: %q", got)
	}
}
