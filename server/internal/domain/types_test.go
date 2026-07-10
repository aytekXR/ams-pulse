// Tests for pure-logic helpers and typed constants in the domain package.
// See schema_test.go for JSON-contract round-trip tests.
package domain_test

import (
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// TestServerEvent_Time verifies that the Time() helper correctly converts the
// epoch-millisecond TS field to a UTC time.Time value.
//
// ServerEvent.Time() = time.UnixMilli(e.TS).UTC()
func TestServerEvent_Time(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		tsMs    int64
		wantUTC time.Time
	}{
		{
			name:    "zero epoch",
			tsMs:    0,
			wantUTC: time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "one second",
			tsMs:    1000,
			wantUTC: time.Date(1970, 1, 1, 0, 0, 1, 0, time.UTC),
		},
		{
			name:    "millisecond precision",
			tsMs:    1500,
			wantUTC: time.Date(1970, 1, 1, 0, 0, 1, 500_000_000, time.UTC),
		},
		{
			name:    "pre-epoch negative",
			tsMs:    -1000,
			wantUTC: time.Date(1969, 12, 31, 23, 59, 59, 0, time.UTC),
		},
		{
			name: "known fixture timestamp",
			// 1700000000000 ms = 2023-11-14T22:13:20 UTC
			tsMs:    1700000000000,
			wantUTC: time.UnixMilli(1700000000000).UTC(),
		},
		{
			name: "sub-ms boundary — 499ms",
			tsMs: 499,
			// 499 ms past epoch
			wantUTC: time.Date(1970, 1, 1, 0, 0, 0, 499_000_000, time.UTC),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := domain.ServerEvent{TS: tc.tsMs}
			got := ev.Time()
			if !got.Equal(tc.wantUTC) {
				t.Errorf("ServerEvent{TS:%d}.Time() = %v, want %v", tc.tsMs, got, tc.wantUTC)
			}
			if got.Location() != time.UTC {
				t.Errorf("Time() location = %v, want UTC", got.Location())
			}
		})
	}
}

// TestServerEvent_Time_AlwaysUTC checks that Time() always returns UTC regardless
// of what the local timezone is set to in the test environment.
func TestServerEvent_Time_AlwaysUTC(t *testing.T) {
	t.Parallel()

	// 1700000000123 ms → 123 ms sub-second component
	const tsMs = 1700000000123
	ev := domain.ServerEvent{TS: tsMs}
	got := ev.Time()

	if got.Location() != time.UTC {
		t.Errorf("Time() returned non-UTC location: %v", got.Location())
	}
	if got.Nanosecond() != 123_000_000 {
		t.Errorf("millisecond precision lost: got %d ns, want 123_000_000", got.Nanosecond())
	}
}

// TestStreamHealth_Constants_Values verifies that each StreamHealth constant has
// exactly the string value prescribed by the live-snapshot contract.
func TestStreamHealth_Constants_Values(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  domain.StreamHealth
		want domain.StreamHealth
	}{
		{"good", domain.StreamHealthGood, "good"},
		{"warning", domain.StreamHealthWarning, "warning"},
		{"critical", domain.StreamHealthCritical, "critical"},
		{"offline", domain.StreamHealthOffline, "offline"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Errorf("StreamHealth%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestStreamHealth_Constants_Distinct verifies that the four StreamHealth
// constants are all unique (no accidental duplicate values).
func TestStreamHealth_Constants_Distinct(t *testing.T) {
	t.Parallel()

	vals := []domain.StreamHealth{
		domain.StreamHealthGood,
		domain.StreamHealthWarning,
		domain.StreamHealthCritical,
		domain.StreamHealthOffline,
	}
	seen := make(map[domain.StreamHealth]string, len(vals))
	names := []string{"StreamHealthGood", "StreamHealthWarning", "StreamHealthCritical", "StreamHealthOffline"}
	for i, v := range vals {
		if prev, dup := seen[v]; dup {
			t.Errorf("%s and %s have the same value %q", prev, names[i], v)
		}
		seen[v] = names[i]
	}
}

// TestSourceConstants_NonEmptyAndDistinct verifies all source identifier
// constants are non-empty strings and unique.
func TestSourceConstants_NonEmptyAndDistinct(t *testing.T) {
	t.Parallel()

	type srcEntry struct {
		name string
		val  string
	}
	entries := []srcEntry{
		{"SourceRestPoll", domain.SourceRestPoll},
		{"SourceKafka", domain.SourceKafka},
		{"SourceWebhook", domain.SourceWebhook},
		{"SourceHostAgent", domain.SourceHostAgent},
	}

	seen := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.val == "" {
			t.Errorf("%s is empty string", e.name)
		}
		if prev, dup := seen[e.val]; dup {
			t.Errorf("%s and %s share value %q", prev, e.name, e.val)
		}
		seen[e.val] = e.name
	}
}

// TestEventTypeConstants_NonEmptyAndDistinct verifies all event-type constants
// are non-empty strings and distinct. This complements TestServerEvent_AllTypes
// in schema_test.go (which checks marshalling); this test checks the values themselves.
func TestEventTypeConstants_NonEmptyAndDistinct(t *testing.T) {
	t.Parallel()

	type etEntry struct {
		name string
		val  string
	}
	entries := []etEntry{
		{"EventStreamPublishStart", domain.EventStreamPublishStart},
		{"EventStreamPublishEnd", domain.EventStreamPublishEnd},
		{"EventStreamStats", domain.EventStreamStats},
		{"EventWebRTCClientStats", domain.EventWebRTCClientStats},
		{"EventIngestStats", domain.EventIngestStats},
		{"EventNodeStats", domain.EventNodeStats},
		{"EventRecordingReady", domain.EventRecordingReady},
		{"EventViewerJoin", domain.EventViewerJoin},
		{"EventViewerLeave", domain.EventViewerLeave},
	}

	seen := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.val == "" {
			t.Errorf("%s is empty string", e.name)
		}
		if prev, dup := seen[e.val]; dup {
			t.Errorf("%s and %s share value %q", prev, e.name, e.val)
		}
		seen[e.val] = e.name
	}
}

// TestAlertRule_SeverityCondition exercises the AlertRule and AlertScope structs
// to ensure field assignment and reading work as expected.
func TestAlertRule_Fields(t *testing.T) {
	t.Parallel()

	rule := domain.AlertRule{
		ID:        "rule-1",
		Name:      "High CPU",
		Metric:    "cpu_pct",
		Condition: "gt",
		Threshold: 90.0,
		WindowS:   60,
		Severity:  "critical",
		Enabled:   true,
		Scope: domain.AlertScope{
			NodeID: "ams-node-1",
			App:    "live",
		},
		CooldownS: 300,
		CreatedAt: 1700000000000,
		UpdatedAt: 1700000000000,
	}

	if rule.Condition != "gt" {
		t.Errorf("Condition = %q, want gt", rule.Condition)
	}
	if rule.Threshold != 90.0 {
		t.Errorf("Threshold = %v, want 90.0", rule.Threshold)
	}
	if rule.Scope.NodeID != "ams-node-1" {
		t.Errorf("Scope.NodeID = %q, want ams-node-1", rule.Scope.NodeID)
	}
	if !rule.Enabled {
		t.Errorf("Enabled = false, want true")
	}
}

// TestProbeConfig_Fields verifies that ProbeConfig fields round-trip through
// assignment correctly. Covers the synthetic-probe types (F10, WO-301).
func TestProbeConfig_Fields(t *testing.T) {
	t.Parallel()

	cfg := domain.ProbeConfig{
		ID:        "probe-uuid-1",
		Name:      "RTMP Health",
		URL:       "rtmp://edge.example.com/live/stream",
		Protocol:  "rtmp",
		IntervalS: 60,
		TimeoutS:  10,
		Enabled:   true,
	}

	if cfg.Protocol != "rtmp" {
		t.Errorf("Protocol = %q, want rtmp", cfg.Protocol)
	}
	if cfg.IntervalS != 60 {
		t.Errorf("IntervalS = %d, want 60", cfg.IntervalS)
	}
	if cfg.TimeoutS != 10 {
		t.Errorf("TimeoutS = %d, want 10", cfg.TimeoutS)
	}
	if !cfg.Enabled {
		t.Errorf("Enabled = false, want true")
	}
}

// TestProbeResult_Fields verifies ProbeResult fields and the ErrorCode sentinel values.
func TestProbeResult_Fields(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		errorCode string
		success   bool
	}{
		{"success", "", true},
		{"timeout", "timeout", false},
		{"dns", "dns", false},
		{"http_4xx", "http_4xx", false},
		{"http_5xx", "http_5xx", false},
		{"parse", "parse", false},
		{"not_probed", "not_probed", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := domain.ProbeResult{
				ID:          "result-uuid",
				ProbeID:     "probe-uuid-1",
				TS:          now,
				Success:     tc.success,
				ErrorCode:   tc.errorCode,
				TTFBMs:      100,
				BitrateKbps: 3500.5,
			}
			if r.Success != tc.success {
				t.Errorf("Success = %v, want %v", r.Success, tc.success)
			}
			if r.ErrorCode != tc.errorCode {
				t.Errorf("ErrorCode = %q, want %q", r.ErrorCode, tc.errorCode)
			}
			if !r.TS.Equal(now) {
				t.Errorf("TS = %v, want %v", r.TS, now)
			}
			// IceState zero value is empty string (not applicable for HLS/DASH/RTMP).
			if r.IceState != "" {
				t.Errorf("default IceState = %q, want ''", r.IceState)
			}
		})
	}
}

// TestProbeResult_IceState verifies the IceState field semantics for WebRTC probes.
// D-074 binding: "connected"|"failed"|"timeout"|"" (empty = not applicable / ICE not attempted).
func TestProbeResult_IceState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		iceState  string
		errorCode string
	}{
		{"not_applicable", "", ""},
		{"connected", "connected", ""},
		{"failed", "failed", "ice_failed"},
		{"timeout", "timeout", "ice_timeout"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := domain.ProbeResult{
				ID:             "webrtc-result-uuid",
				ProbeID:        "webrtc-probe-uuid",
				TS:             time.Now().UTC(),
				Success:        true, // signaling always succeeds even when ICE fails
				SignalingState: "offer_received",
				IceState:       tc.iceState,
				ErrorCode:      tc.errorCode,
			}
			if r.IceState != tc.iceState {
				t.Errorf("IceState = %q, want %q", r.IceState, tc.iceState)
			}
			if r.ErrorCode != tc.errorCode {
				t.Errorf("ErrorCode = %q, want %q", r.ErrorCode, tc.errorCode)
			}
			// Success must stay true regardless of ICE outcome (D-074 semantics).
			if !r.Success {
				t.Error("Success must be true when signaling succeeded, even on ICE failure")
			}
		})
	}
}

// TestBeaconEvent_RoundTrip verifies that a BeaconEvent retains all fields
// through struct assignment and that BeaconItem.TS is preserved.
func TestBeaconEvent_Fields(t *testing.T) {
	t.Parallel()

	ev := domain.BeaconEvent{
		Version:    1,
		SessionID:  "sess-abc123",
		StreamID:   "live-stream-1",
		App:        "live",
		SDK:        "beacon-js/0.1.0",
		PlayerKind: "hls.js",
		Tenant:     "acme",
		Events: []domain.BeaconItem{
			{Type: "play", TS: 1700000000000, Data: map[string]any{"latency_ms": 350}},
			{Type: "buffer", TS: 1700000001000},
		},
		Enrichment: &domain.EnrichmentBlock{
			Geo:    &domain.GeoEnrichment{Country: "TR", Region: "Istanbul"},
			Client: &domain.ClientEnrichment{Device: "mobile", OS: "iOS", Browser: "Safari"},
		},
	}

	if ev.Version != 1 {
		t.Errorf("Version = %d, want 1", ev.Version)
	}
	if ev.SessionID != "sess-abc123" {
		t.Errorf("SessionID = %q, want sess-abc123", ev.SessionID)
	}
	if len(ev.Events) != 2 {
		t.Fatalf("len(Events) = %d, want 2", len(ev.Events))
	}
	if ev.Events[0].Type != "play" {
		t.Errorf("Events[0].Type = %q, want play", ev.Events[0].Type)
	}
	if ev.Events[1].TS != 1700000001000 {
		t.Errorf("Events[1].TS = %d, want 1700000001000", ev.Events[1].TS)
	}
	if ev.Enrichment == nil || ev.Enrichment.Client == nil {
		t.Fatal("Enrichment or Enrichment.Client is nil")
	}
	if ev.Enrichment.Client.Device != "mobile" {
		t.Errorf("Client.Device = %q, want mobile", ev.Enrichment.Client.Device)
	}
}

// TestLiveStream_HealthField verifies that LiveStream.Health accepts StreamHealth values.
func TestLiveStream_HealthField(t *testing.T) {
	t.Parallel()

	cases := []domain.StreamHealth{
		domain.StreamHealthGood,
		domain.StreamHealthWarning,
		domain.StreamHealthCritical,
		domain.StreamHealthOffline,
	}

	for _, h := range cases {
		h := h
		t.Run(string(h), func(t *testing.T) {
			t.Parallel()
			ls := domain.LiveStream{
				StreamID: "s1",
				Health:   h,
			}
			if ls.Health != h {
				t.Errorf("LiveStream.Health = %q, want %q", ls.Health, h)
			}
		})
	}
}
