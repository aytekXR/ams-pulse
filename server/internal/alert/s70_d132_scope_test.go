package alert

// s70_d132_scope_test.go — internal (white-box) regression test for D-132 [18].
// The alert evaluator's baseline-lookup scope key MUST be byte-identical to the
// key the anomaly Detector stores, including the [18] escaping of quotes /
// backslashes / control bytes. scopeJSONAnomaly delegates to anomaly.ScopeJSON to
// guarantee that; this pins the concrete escaped output so a future hand-rolled
// raw-concat copy (which would silently miss baselines for special-char IDs and
// stop the alert rule firing) is caught.

import (
	"encoding/json"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/anomaly"
)

func TestScopeJSONAnomaly_EscapesAndMatchesDetectorKey(t *testing.T) {
	cases := []struct {
		nodeID, streamID, want string
	}{
		{"", "s1", `{"stream_id":"s1"}`},
		{"node-1", "", `{"node_id":"node-1"}`},
		{"", `foo"bar`, `{"stream_id":"foo\"bar"}`}, // [18]: quote escaped, not truncated
		{"", `foo\bar`, `{"stream_id":"foo\\bar"}`}, // backslash escaped
		{`n"1`, "", `{"node_id":"n\"1"}`},           // quote in node id
	}
	for _, tc := range cases {
		got := scopeJSONAnomaly(tc.nodeID, tc.streamID)
		if got != tc.want {
			t.Errorf("scopeJSONAnomaly(%q,%q) = %q, want %q", tc.nodeID, tc.streamID, got, tc.want)
		}
		if !json.Valid([]byte(got)) {
			t.Errorf("scopeJSONAnomaly(%q,%q) = %q — not valid JSON", tc.nodeID, tc.streamID, got)
		}
		// Cross-package contract: must equal the Detector's canonical builder exactly,
		// or GetAnomalyBaseline looks up a key that no stored row matches.
		if want := anomaly.ScopeJSON(tc.nodeID, "", tc.streamID); got != want {
			t.Errorf("scopeJSONAnomaly(%q,%q)=%q diverges from anomaly.ScopeJSON=%q", tc.nodeID, tc.streamID, got, want)
		}
	}
}
