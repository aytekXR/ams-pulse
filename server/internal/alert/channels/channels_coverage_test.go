package channels // internal — required to reach unexported helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ─── Registry ─────────────────────────────────────────────────────────────────

func TestRegistry_RegisterGetRemove(t *testing.T) {
	r := NewRegistry()
	noop := &NoopChannel{}

	// Nothing registered yet.
	if _, ok := r.Get("ch1"); ok {
		t.Fatal("Get on empty registry returned ok=true")
	}

	r.Register("ch1", noop)
	got, ok := r.Get("ch1")
	if !ok {
		t.Fatal("Get after Register returned ok=false")
	}
	if got != noop {
		t.Errorf("Get returned wrong channel: got %v, want %v", got, noop)
	}

	r.Remove("ch1")
	if _, ok := r.Get("ch1"); ok {
		t.Error("Get after Remove returned ok=true; Remove did not delete the entry")
	}
}

// ─── Name() for each channel type ─────────────────────────────────────────────

func TestChannelNames(t *testing.T) {
	cases := []struct {
		ch   Channel
		want string
	}{
		{NewSlackChannel(SlackConfig{}), "slack"},
		{NewWebhookChannel(WebhookConfig{}), "webhook"},
		{NewPagerDutyChannel(PagerDutyConfig{}), "pagerduty"},
		{NewTelegramChannel(TelegramConfig{}), "telegram"},
		{NewEmailChannel(EmailConfig{}), "email"},
		{&NoopChannel{}, "noop"},
	}
	for _, tc := range cases {
		got := tc.ch.Name()
		if got != tc.want {
			t.Errorf("%T.Name() = %q, want %q", tc.ch, got, tc.want)
		}
	}
}

// ─── buildEmailBody ───────────────────────────────────────────────────────────

func TestBuildEmailBody_AllFields(t *testing.T) {
	n := map[string]any{
		"title":         "CPU high",
		"state":         "firing",
		"severity":      "critical",
		"metric":        "cpu_usage",
		"value":         float64(95.5),
		"threshold":     float64(90.0),
		"scope":         map[string]any{"host": "server1"},
		"test":          true,
		"dashboard_url": "http://localhost:8090/dashboard",
	}
	body := buildEmailBody(n)

	must := []struct {
		label   string
		contain string
	}{
		{"header", "Pulse Alert Notification"},
		{"separator", "========================================"},
		{"title line", "Title:     CPU high"},
		{"state line", "State:     firing"},
		{"severity line", "Severity:  critical"},
		{"metric line", "Metric:    cpu_usage"},
		{"value line", "Value:     95.5"},
		{"threshold line", "Threshold: 90"},
		{"scope key", "host"},
		{"scope value", "server1"},
		{"test notice", "[This is a test notification]"},
		{"dashboard url", "http://localhost:8090/dashboard"},
	}
	for _, c := range must {
		if !strings.Contains(body, c.contain) {
			t.Errorf("buildEmailBody: missing %s — want %q in:\n%s", c.label, c.contain, body)
		}
	}
}

func TestBuildEmailBody_EmptyMap(t *testing.T) {
	body := buildEmailBody(map[string]any{})
	if !strings.Contains(body, "Pulse Alert Notification") {
		t.Errorf("buildEmailBody(empty): missing header, got:\n%s", body)
	}
	// Optional field lines must NOT appear when absent.
	for _, absent := range []string{"Title:", "State:", "Severity:", "Metric:", "Value:", "Threshold:", "Dashboard:"} {
		if strings.Contains(body, absent) {
			t.Errorf("buildEmailBody(empty): unexpected field %q in output:\n%s", absent, body)
		}
	}
}

func TestBuildEmailBody_NoTestFlag(t *testing.T) {
	n := map[string]any{
		"title": "real alert",
		"test":  false,
	}
	body := buildEmailBody(n)
	if strings.Contains(body, "test notification") {
		t.Errorf("buildEmailBody with test=false should not include test notice; got:\n%s", body)
	}
}

// ─── mapPDSeverity ────────────────────────────────────────────────────────────

func TestMapPDSeverity_AllCases(t *testing.T) {
	cases := []struct{ in, want string }{
		{"critical", "critical"},
		{"CRITICAL", "critical"}, // ToLower
		{"warning", "warning"},
		{"Warning", "warning"}, // ToLower
		{"info", "info"},
		{"INFO", "info"}, // ToLower
		{"", "error"},    // default
		{"unknown", "error"},
		{"debug", "error"},
	}
	for _, tc := range cases {
		got := mapPDSeverity(tc.in)
		if got != tc.want {
			t.Errorf("mapPDSeverity(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ─── buildPDComponent ─────────────────────────────────────────────────────────

func TestBuildPDComponent(t *testing.T) {
	// With a non-empty metric.
	got := buildPDComponent(map[string]any{"metric": "stream_offline"})
	if got != "pulse:stream_offline" {
		t.Errorf("buildPDComponent with metric: got %q, want %q", got, "pulse:stream_offline")
	}

	// Without metric key — must fall back to "pulse".
	got2 := buildPDComponent(map[string]any{})
	if got2 != "pulse" {
		t.Errorf("buildPDComponent without metric: got %q, want %q", got2, "pulse")
	}

	// Empty string metric — treated as missing, falls back to "pulse".
	got3 := buildPDComponent(map[string]any{"metric": ""})
	if got3 != "pulse" {
		t.Errorf("buildPDComponent empty metric: got %q, want %q", got3, "pulse")
	}
}

// ─── buildPDDetails ───────────────────────────────────────────────────────────

func TestBuildPDDetails_FullPayload(t *testing.T) {
	n := map[string]any{
		"metric":    "p95_latency",
		"value":     float64(450.0),
		"threshold": float64(200.0),
		"scope": map[string]any{
			"app":       "live",
			"stream_id": "auction",
		},
		"group_key": "live-auction",
	}
	d := buildPDDetails(n)

	if d["metric"] != "p95_latency" {
		t.Errorf("buildPDDetails: metric = %v, want p95_latency", d["metric"])
	}
	if d["value"] != "450" {
		t.Errorf("buildPDDetails: value = %v, want '450'", d["value"])
	}
	if d["threshold"] != "200" {
		t.Errorf("buildPDDetails: threshold = %v, want '200'", d["threshold"])
	}
	if d["scope_app"] != "live" {
		t.Errorf("buildPDDetails: scope_app = %v, want 'live'", d["scope_app"])
	}
	if d["scope_stream_id"] != "auction" {
		t.Errorf("buildPDDetails: scope_stream_id = %v, want 'auction'", d["scope_stream_id"])
	}
	if d["group_key"] != "live-auction" {
		t.Errorf("buildPDDetails: group_key = %v, want 'live-auction'", d["group_key"])
	}
}

func TestBuildPDDetails_NilAndEmptyScopeExcluded(t *testing.T) {
	n := map[string]any{
		"scope": map[string]any{
			"host": nil,
			"app":  "",
		},
	}
	d := buildPDDetails(n)
	if _, ok := d["scope_host"]; ok {
		t.Error("buildPDDetails: nil scope value must not appear in details")
	}
	if _, ok := d["scope_app"]; ok {
		t.Error("buildPDDetails: empty scope value must not appear in details")
	}
}

func TestBuildPDDetails_EmptyGroupKeyExcluded(t *testing.T) {
	n := map[string]any{"group_key": ""}
	d := buildPDDetails(n)
	if _, ok := d["group_key"]; ok {
		t.Error("buildPDDetails: empty group_key must not appear in details")
	}
}

// ─── Telegram: exact bot-API request assertions ───────────────────────────────

func TestTelegramChannel_ExactRequest(t *testing.T) {
	var (
		capturedPath        string
		capturedContentType string
		capturedBody        []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedContentType = r.Header.Get("Content-Type")
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": 7}}) //nolint:errcheck
	}))
	defer srv.Close()

	ch := NewTelegramChannelWithURL(TelegramConfig{
		BotToken: "MY-BOT-TOKEN",
		ChatID:   "-100987654321",
	}, srv.URL+"/bot%s/sendMessage")

	n := map[string]any{
		"title":         "High latency",
		"state":         "firing",
		"severity":      "critical",
		"metric":        "p95_latency",
		"value":         float64(500.0),
		"threshold":     float64(200.0),
		"dashboard_url": "http://localhost:8090/alerts",
		"test":          false,
	}
	payload, _ := json.Marshal(n)
	if err := ch.Send(context.Background(), payload); err != nil {
		t.Fatalf("telegram.Send: %v", err)
	}

	// 1. URL path must embed the bot token exactly.
	wantPath := "/botMY-BOT-TOKEN/sendMessage"
	if capturedPath != wantPath {
		t.Errorf("telegram request path: got %q, want %q", capturedPath, wantPath)
	}

	// 2. Content-Type must be application/json.
	if !strings.HasPrefix(capturedContentType, "application/json") {
		t.Errorf("telegram Content-Type: got %q, want application/json", capturedContentType)
	}

	// 3. Body fields: chat_id, parse_mode, and text.
	var body map[string]any
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("telegram body not JSON: %v", err)
	}
	if body["chat_id"] != "-100987654321" {
		t.Errorf("telegram chat_id: got %v, want -100987654321", body["chat_id"])
	}
	if body["parse_mode"] != "HTML" {
		t.Errorf("telegram parse_mode: got %v, want HTML", body["parse_mode"])
	}
	text, _ := body["text"].(string)
	if text == "" {
		t.Fatal("telegram text: must be a non-empty string")
	}
	if !strings.Contains(text, "High latency") {
		t.Errorf("telegram text must contain alert title 'High latency'; got:\n%s", text)
	}
}

func TestTelegramChannel_Non2xx_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "description": "Unauthorized"}) //nolint:errcheck
	}))
	defer srv.Close()

	ch := NewTelegramChannelWithURL(TelegramConfig{
		BotToken: "bad-token",
		ChatID:   "123",
	}, srv.URL+"/bot%s/sendMessage")

	payload, _ := json.Marshal(map[string]any{"title": "test", "state": "firing"})
	err := ch.Send(context.Background(), payload)
	if err == nil {
		t.Fatal("telegram.Send: expected error for 401 response, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("telegram error should mention status 401; got: %v", err)
	}
}

// ─── Webhook: non-2xx and context-timeout ─────────────────────────────────────

func TestWebhookChannel_Non2xx_ReturnsError(t *testing.T) {
	for _, code := range []int{400, 401, 403, 500, 503} {
		code := code
		t.Run(fmt.Sprintf("status%d", code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer srv.Close()

			ch := NewWebhookChannel(WebhookConfig{URL: srv.URL})
			err := ch.Send(context.Background(), []byte(`{"state":"firing"}`))
			if err == nil {
				t.Fatalf("webhook.Send: expected error for HTTP %d, got nil", code)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%d", code)) {
				t.Errorf("webhook error for status %d must mention the code; got: %v", code, err)
			}
		})
	}
}

func TestWebhookChannel_ContextDeadline_ReturnsError(t *testing.T) {
	// stall blocks the handler goroutine until the test finishes; closing it
	// unblocks the handler so that srv.Close() can return without a long wait.
	stall := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-stall // released by the test cleanup below
		w.WriteHeader(http.StatusOK)
	}))
	defer func() {
		close(stall) // unblock any waiting handler goroutine
		srv.Close()
	}()

	ch := NewWebhookChannel(WebhookConfig{URL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := ch.Send(ctx, []byte(`{"state":"firing"}`))
	if err == nil {
		t.Fatal("webhook.Send: expected deadline/cancel error, got nil")
	}
}

// ─── VerifyWebhookSignature: short-signature branch ───────────────────────────

func TestVerifyWebhookSignature_TooShort(t *testing.T) {
	payload := []byte(`{"x":1}`)
	// Signatures at or below len("sha256=") = 7 must return false immediately.
	for _, sig := range []string{"", "sha256=", "sha25", "x"} {
		if VerifyWebhookSignature("any-secret", payload, sig) {
			t.Errorf("VerifyWebhookSignature(%q): expected false for too-short signature, got true", sig)
		}
	}
}
