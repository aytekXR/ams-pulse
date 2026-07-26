package webhook

// webhook_errors_test.go — coverage for AMS error-action webhook mapping.
// Verifies that endpointFailed, publishTimeoutError, and encoderNotOpenedError
// are mapped to EventStreamIngestError in both JSON and form-urlencoded payloads,
// and that unrecognised actions return an empty event list (previously silent drop;
// now promoted to Info log level).

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/aytekXR/ams-pulse/server/internal/domain"
)

// ─── translateWebhook: error actions (JSON) ──────────────────────────────────

func TestTranslateWebhook_ErrorActions_JSON(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)

	cases := []struct {
		action string
	}{
		{"endpointFailed"},
		{"publishTimeoutError"},
		{"encoderNotOpenedError"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.action, func(t *testing.T) {
			payload := `{"action":"` + tc.action + `","streamId":"s1","streamName":"my-stream","app":"live"}`
			var raw map[string]json.RawMessage
			if err := json.Unmarshal([]byte(payload), &raw); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			evs := h.translateWebhook(raw)
			if len(evs) != 1 {
				t.Fatalf("action %q: expected 1 event, got %d", tc.action, len(evs))
			}
			ev := evs[0]
			if ev.Type != domain.EventStreamIngestError {
				t.Errorf("action %q: Type = %q, want %q", tc.action, ev.Type, domain.EventStreamIngestError)
			}
			if ev.Source != domain.SourceWebhook {
				t.Errorf("action %q: Source = %q, want %q", tc.action, ev.Source, domain.SourceWebhook)
			}
			gotAction, _ := ev.Data["action"].(string)
			if gotAction != tc.action {
				t.Errorf("action %q: Data[action] = %q, want %q", tc.action, gotAction, tc.action)
			}
			gotName, _ := ev.Data["stream_name"].(string)
			if gotName != "my-stream" {
				t.Errorf("action %q: Data[stream_name] = %q, want %q", tc.action, gotName, "my-stream")
			}
			if ev.Data["metadata"] == nil {
				t.Errorf("action %q: Data[metadata] is nil", tc.action)
			}
		})
	}
}

// ─── parseFormWebhook: error actions (form-urlencoded) ───────────────────────

func TestParseFormWebhook_ErrorActions(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)

	cases := []struct {
		action string
	}{
		{"endpointFailed"},
		{"publishTimeoutError"},
		{"encoderNotOpenedError"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.action, func(t *testing.T) {
			form := url.Values{
				"action":     {tc.action},
				"streamId":   {"s2"},
				"streamName": {"form-stream"},
				"app":        {"live"},
			}
			body := []byte(form.Encode())
			evs, err := h.parseFormWebhook(body)
			if err != nil {
				t.Fatalf("action %q: parseFormWebhook error: %v", tc.action, err)
			}
			if len(evs) != 1 {
				t.Fatalf("action %q: expected 1 event, got %d", tc.action, len(evs))
			}
			ev := evs[0]
			if ev.Type != domain.EventStreamIngestError {
				t.Errorf("action %q: Type = %q, want %q", tc.action, ev.Type, domain.EventStreamIngestError)
			}
			gotAction, _ := ev.Data["action"].(string)
			if gotAction != tc.action {
				t.Errorf("action %q: Data[action] = %q, want %q", tc.action, gotAction, tc.action)
			}
		})
	}
}

// ─── unrecognised action: no events returned ─────────────────────────────────

func TestTranslateWebhook_UnrecognisedAction_NoEvents(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)

	payload := `{"action":"someNewFutureAction","streamId":"s3","app":"live"}`
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	evs := h.translateWebhook(raw)
	if len(evs) != 0 {
		t.Errorf("unrecognised action: expected 0 events, got %d", len(evs))
	}
}

// TestParseWebhookArray_ErrorActionMixed verifies that an array payload
// containing a mix of known error actions, known start/end actions, and
// unknown actions produces the correct number of events.
func TestParseWebhookArray_ErrorActionMixed(t *testing.T) {
	h, _ := newTestHandler(t, testSecret)

	body := strings.NewReader(`[
		{"action":"liveStreamStarted","streamId":"s1","app":"live"},
		{"action":"endpointFailed","streamId":"s1","streamName":"s1","app":"live"},
		{"action":"unknownFutureAction","streamId":"s1","app":"live"}
	]`)
	var arr []map[string]json.RawMessage
	if err := json.NewDecoder(body).Decode(&arr); err != nil {
		t.Fatalf("json.Decode: %v", err)
	}
	var evs []domain.ServerEvent
	for _, item := range arr {
		evs = append(evs, h.translateWebhook(item)...)
	}
	if len(evs) != 2 {
		t.Errorf("mixed array: expected 2 events (start + error), got %d", len(evs))
	}
	if evs[0].Type != domain.EventStreamPublishStart {
		t.Errorf("evs[0].Type = %q, want %q", evs[0].Type, domain.EventStreamPublishStart)
	}
	if evs[1].Type != domain.EventStreamIngestError {
		t.Errorf("evs[1].Type = %q, want %q", evs[1].Type, domain.EventStreamIngestError)
	}
}
