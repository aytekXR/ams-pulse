package webhook

// webhook_more_test.go — additional coverage for parseWebhook, translateWebhook,
// json helpers, normalizePublishType, Name, and handleWebhook HTTP paths.
// Complements webhook_test.go without duplicating its cases.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// ─── Name ────────────────────────────────────────────────────────────────────

func TestName(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)
	// newTestHandler creates with ListenAddr ":0"; Name() wraps that in "webhook(...)".
	want := "webhook(:0)"
	got := h.Name()
	if got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

// ─── parseWebhook ────────────────────────────────────────────────────────────

func TestParseWebhookSingleObject(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)
	body := []byte(`{"action":"liveStreamStarted","streamId":"s1","app":"live","publishType":"rtmp"}`)
	events, err := h.parseWebhook(body)
	if err != nil {
		t.Fatalf("parseWebhook single object: unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != domain.EventStreamPublishStart {
		t.Errorf("Type = %q, want %q", events[0].Type, domain.EventStreamPublishStart)
	}
	if events[0].StreamID != "s1" {
		t.Errorf("StreamID = %q, want %q", events[0].StreamID, "s1")
	}
	if events[0].App != "live" {
		t.Errorf("App = %q, want %q", events[0].App, "live")
	}
}

func TestParseWebhookArrayMixedActions(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)
	// Array with three elements: known-start, unknown action, known-end.
	// Only the two known actions should produce events.
	body := []byte(`[
		{"action":"liveStreamStarted","streamId":"s1","app":"live"},
		{"action":"unknownAction","streamId":"s2","app":"live"},
		{"action":"liveStreamEnded","streamId":"s3","app":"live","duration":120}
	]`)
	events, err := h.parseWebhook(body)
	if err != nil {
		t.Fatalf("parseWebhook array: unexpected error: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events from mixed-action array, got %d", len(events))
	}
}

func TestParseWebhookInvalidJSON(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)
	body := []byte(`not valid json at all {]`)
	events, err := h.parseWebhook(body)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if events != nil {
		t.Errorf("expected nil events on parse error, got %v", events)
	}
}

func TestParseWebhookArrayZeroEvents(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)
	// Array of unknown actions → each translateWebhook returns nil → aggregate 0 events.
	body := []byte(`[{"action":"unknownFoo"},{"action":"unknownBar"}]`)
	events, err := h.parseWebhook(body)
	if err != nil {
		t.Fatalf("parseWebhook zero-events array: unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events from all-unknown array, got %d", len(events))
	}
}

// ─── translateWebhook: all action aliases ─────────────────────────────────────

func TestTranslateWebhookPublishStartAliases(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)

	cases := []struct {
		name    string
		payload string
		wantPT  string
	}{
		{
			name:    "liveStreamStarted with publishType webrtc",
			payload: `{"action":"liveStreamStarted","streamId":"s1","app":"live","publishType":"webrtc"}`,
			wantPT:  "webrtc",
		},
		{
			name:    "liveStreamStarted without publishType defaults to other",
			payload: `{"action":"liveStreamStarted","streamId":"s1","app":"live"}`,
			wantPT:  "other",
		},
		{
			name:    "startBroadcast alias",
			payload: `{"action":"startBroadcast","streamId":"s1","app":"live"}`,
			wantPT:  "other",
		},
		{
			name:    "publish_started alias",
			payload: `{"action":"publish_started","streamId":"s1","app":"live"}`,
			wantPT:  "other",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.payload), &raw); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			evs := h.translateWebhook(raw)
			if len(evs) != 1 {
				t.Fatalf("expected 1 event, got %d", len(evs))
			}
			if evs[0].Type != domain.EventStreamPublishStart {
				t.Errorf("Type = %q, want %q", evs[0].Type, domain.EventStreamPublishStart)
			}
			pt, _ := evs[0].Data["publish_type"].(string)
			if pt != tc.wantPT {
				t.Errorf("publish_type = %q, want %q", pt, tc.wantPT)
			}
		})
	}
}

func TestTranslateWebhookPublishEndAliases(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)

	cases := []struct {
		name    string
		payload string
		wantDur int
	}{
		{
			name:    "liveStreamEnded with duration",
			payload: `{"action":"liveStreamEnded","streamId":"s1","app":"live","duration":300}`,
			wantDur: 300,
		},
		{
			name:    "liveStreamEnded without duration",
			payload: `{"action":"liveStreamEnded","streamId":"s1","app":"live"}`,
			wantDur: 0,
		},
		{
			name:    "stopBroadcast alias",
			payload: `{"action":"stopBroadcast","streamId":"s1","app":"live"}`,
			wantDur: 0,
		},
		{
			name:    "publish_ended alias",
			payload: `{"action":"publish_ended","streamId":"s1","app":"live"}`,
			wantDur: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.payload), &raw); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			evs := h.translateWebhook(raw)
			if len(evs) != 1 {
				t.Fatalf("expected 1 event, got %d", len(evs))
			}
			if evs[0].Type != domain.EventStreamPublishEnd {
				t.Errorf("Type = %q, want %q", evs[0].Type, domain.EventStreamPublishEnd)
			}
			dur, _ := evs[0].Data["duration_s"].(int)
			if dur != tc.wantDur {
				t.Errorf("duration_s = %d, want %d", dur, tc.wantDur)
			}
		})
	}
}

func TestTranslateWebhookVodAliases(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)

	cases := []struct {
		name     string
		payload  string
		wantPath string
		wantSize int64
	}{
		{
			name:     "vodReady with vodName and vodSize",
			payload:  `{"action":"vodReady","streamId":"s1","app":"live","vodName":"/recordings/s1.mp4","vodSize":12345}`,
			wantPath: "/recordings/s1.mp4",
			wantSize: 12345,
		},
		{
			name:     "recording_ready alias without vodName/vodSize",
			payload:  `{"action":"recording_ready","streamId":"s1","app":"live"}`,
			wantPath: "",
			wantSize: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.payload), &raw); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			evs := h.translateWebhook(raw)
			if len(evs) != 1 {
				t.Fatalf("expected 1 event, got %d", len(evs))
			}
			if evs[0].Type != domain.EventRecordingReady {
				t.Errorf("Type = %q, want %q", evs[0].Type, domain.EventRecordingReady)
			}
			path, _ := evs[0].Data["path"].(string)
			if path != tc.wantPath {
				t.Errorf("path = %q, want %q", path, tc.wantPath)
			}
			size, _ := evs[0].Data["size_bytes"].(int64)
			if size != tc.wantSize {
				t.Errorf("size_bytes = %d, want %d", size, tc.wantSize)
			}
		})
	}
}

func TestTranslateWebhookActionFallbackChain(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)

	// "event" key is the first fallback when "action" is absent.
	t.Run("action from event key", func(t *testing.T) {
		var raw map[string]json.RawMessage
		json.Unmarshal([]byte(`{"event":"liveStreamStarted","streamId":"s1","app":"live"}`), &raw) //nolint:errcheck
		evs := h.translateWebhook(raw)
		if len(evs) != 1 || evs[0].Type != domain.EventStreamPublishStart {
			t.Errorf("expected EventStreamPublishStart from 'event' key, got %v", evs)
		}
	})

	// "type" key is the second fallback when both "action" and "event" are absent.
	t.Run("action from type key", func(t *testing.T) {
		var raw map[string]json.RawMessage
		json.Unmarshal([]byte(`{"type":"liveStreamStarted","streamId":"s1","app":"live"}`), &raw) //nolint:errcheck
		evs := h.translateWebhook(raw)
		if len(evs) != 1 || evs[0].Type != domain.EventStreamPublishStart {
			t.Errorf("expected EventStreamPublishStart from 'type' key, got %v", evs)
		}
	})

	// "appName" is the fallback when "app" is absent.
	t.Run("app from appName key", func(t *testing.T) {
		var raw map[string]json.RawMessage
		json.Unmarshal([]byte(`{"action":"liveStreamStarted","streamId":"s1","appName":"myApp"}`), &raw) //nolint:errcheck
		evs := h.translateWebhook(raw)
		if len(evs) != 1 || evs[0].App != "myApp" {
			t.Errorf("expected App = 'myApp', got %v", evs)
		}
	})
}

func TestTranslateWebhookUnknownAndEmptyAction(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)

	t.Run("unknown action returns nil", func(t *testing.T) {
		var raw map[string]json.RawMessage
		json.Unmarshal([]byte(`{"action":"unknownXYZ","streamId":"s1","app":"live"}`), &raw) //nolint:errcheck
		evs := h.translateWebhook(raw)
		if len(evs) != 0 {
			t.Errorf("expected empty events for unknown action, got %v", evs)
		}
	})

	t.Run("empty action returns nil", func(t *testing.T) {
		var raw map[string]json.RawMessage
		json.Unmarshal([]byte(`{"streamId":"s1","app":"live"}`), &raw) //nolint:errcheck
		evs := h.translateWebhook(raw)
		if len(evs) != 0 {
			t.Errorf("expected empty events for missing action, got %v", evs)
		}
	})
}

// ─── jsonInt ─────────────────────────────────────────────────────────────────

func TestJsonInt(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
		want int
	}{
		{"nil raw", nil, 0},
		{"valid number", json.RawMessage(`42`), 42},
		{"malformed string", json.RawMessage(`"not a number"`), 0},
		{"zero", json.RawMessage(`0`), 0},
		{"negative", json.RawMessage(`-5`), -5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jsonInt(tc.raw)
			if got != tc.want {
				t.Errorf("jsonInt(%s) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

// ─── jsonInt64 ───────────────────────────────────────────────────────────────

func TestJsonInt64(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
		want int64
	}{
		{"nil raw", nil, 0},
		{"large number", json.RawMessage(`9999999999`), 9999999999},
		{"malformed string", json.RawMessage(`"not a number"`), 0},
		{"zero", json.RawMessage(`0`), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jsonInt64(tc.raw)
			if got != tc.want {
				t.Errorf("jsonInt64(%s) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

// ─── jsonString ──────────────────────────────────────────────────────────────

func TestJsonString(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{"nil raw", nil, ""},
		{"valid string", json.RawMessage(`"hello"`), "hello"},
		{"number where string expected", json.RawMessage(`42`), ""},
		{"empty string", json.RawMessage(`""`), ""},
		{"boolean", json.RawMessage(`true`), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := jsonString(tc.raw)
			if got != tc.want {
				t.Errorf("jsonString(%s) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// ─── normalizePublishType ─────────────────────────────────────────────────────

func TestNormalizePublishType(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"webrtc", "webrtc"},
		{"WebRTC", "webrtc"},
		{"WEBRTC", "webrtc"},
		{"rtmp", "rtmp"},
		{"RTMP", "rtmp"},
		{"hls", "hls"},
		{"HLS", "hls"},
		{"mp4", "other"},
		{"unknown", "other"},
		{"", "other"},
		{"dash", "other"},
	}
	for _, tc := range cases {
		t.Run(tc.input+"_->_"+tc.want, func(t *testing.T) {
			got := normalizePublishType(tc.input)
			if got != tc.want {
				t.Errorf("normalizePublishType(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ─── handleWebhook HTTP paths ─────────────────────────────────────────────────

func TestHandleWebhookWrongMethod(t *testing.T) {
	h, sink := newTestHandler(t, testSecret)
	req := httptest.NewRequest(http.MethodGet, "/webhook/ams", nil)
	rr := httptest.NewRecorder()
	h.HTTPHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /webhook/ams should return 405, got %d", rr.Code)
	}
	if sink.Len() != 0 {
		t.Errorf("expected 0 events for wrong method, got %d", sink.Len())
	}
}

func TestHandleWebhookEmptySecretFail(t *testing.T) {
	// An empty shared secret must reject every request (fail-closed).
	h, sink := newTestHandler(t, "")
	body := []byte(`{"action":"liveStreamStarted","streamId":"s1","app":"live"}`)
	// Even if caller somehow provides a signature, the empty-secret check fires first.
	rr := post(t, h, body, hmacSign(body, ""))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("empty secret should return 401, got %d", rr.Code)
	}
	if sink.Len() != 0 {
		t.Errorf("expected 0 events with empty secret, got %d", sink.Len())
	}
}

func TestHandleWebhookInvalidJSONValidHMAC(t *testing.T) {
	// An HMAC-valid request whose body is not parseable JSON should return 200
	// (AMS may retry on non-2xx; we swallow the error rather than cause retries).
	h, sink := newTestHandler(t, testSecret)
	body := []byte(`this is not JSON {[`)
	sig := hmacSign(body, testSecret)
	rr := post(t, h, body, sig)
	if rr.Code != http.StatusOK {
		t.Errorf("invalid JSON with valid HMAC should return 200, got %d", rr.Code)
	}
	if sink.Len() != 0 {
		t.Errorf("expected 0 events for invalid JSON body, got %d", sink.Len())
	}
}

func TestHandleWebhookSinkContents(t *testing.T) {
	// Assert that sink contains the correct fields after a valid POST.
	h, sink := newTestHandler(t, testSecret)
	body := []byte(`{"action":"liveStreamEnded","streamId":"stream42","app":"myApp","duration":180}`)
	sig := hmacSign(body, testSecret)
	rr := post(t, h, body, sig)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if sink.Len() != 1 {
		t.Fatalf("expected 1 event in sink, got %d", sink.Len())
	}
	ev := sink.events[0]
	if ev.Type != domain.EventStreamPublishEnd {
		t.Errorf("Type = %q, want %q", ev.Type, domain.EventStreamPublishEnd)
	}
	if ev.StreamID != "stream42" {
		t.Errorf("StreamID = %q, want %q", ev.StreamID, "stream42")
	}
	if ev.App != "myApp" {
		t.Errorf("App = %q, want %q", ev.App, "myApp")
	}
	if ev.Source != domain.SourceWebhook {
		t.Errorf("Source = %q, want %q", ev.Source, domain.SourceWebhook)
	}
	if ev.NodeID != "test-node" {
		t.Errorf("NodeID = %q, want %q", ev.NodeID, "test-node")
	}
	dur, _ := ev.Data["duration_s"].(int)
	if dur != 180 {
		t.Errorf("duration_s = %d, want 180", dur)
	}
}

func TestHandleWebhookArrayBodySinkContents(t *testing.T) {
	// Verify that an array-form webhook body delivers the correct number of events.
	h, sink := newTestHandler(t, testSecret)
	body := []byte(`[
		{"action":"liveStreamStarted","streamId":"sA","app":"live"},
		{"action":"liveStreamEnded","streamId":"sB","app":"live","duration":60}
	]`)
	sig := hmacSign(body, testSecret)
	rr := post(t, h, body, sig)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if sink.Len() != 2 {
		t.Errorf("expected 2 events from array body, got %d", sink.Len())
	}
}
