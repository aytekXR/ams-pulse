package api_test

// s126_input_validation_test.go — request bodies that used to be stored and silently
// ignored (or coerced to zero) are refused with a 422 that names the field.
//
// Found by the S126 live campaign against v0.5.0:
//   - "threshold": "90" was stored as 0, so a "CPU above 90" rule fired on every sample;
//   - a maintenance window written as the alerting runbook showed it ({cron_expr, …}) was
//     stored, never matched, and notifications went out during the window;
//   - a report schedule written as the reports runbook showed it ("app_filter": "live")
//     ran unscoped — a "tenant-a" statement covered every app and tenant;
//   - a webhook channel written with "url" (not "webhook_url") was created and bound to
//     rules, and every alert it carried failed with `Post "": unsupported protocol scheme`.

import (
	"net/http"
	"strings"
	"testing"
)

func TestS126_AlertRule_RefusesWhatItWouldIgnore(t *testing.T) {
	ts, token, cleanup := setupTestServerWithLicense(t, allFreeLicense(t)) // the shipped configuration
	defer cleanup()

	cases := []struct {
		name  string
		field string
		val   any
		want  string // substring of the 422 message
	}{
		{"threshold as a string", "threshold", "90", "threshold must be a number"},
		{"window_s as a string", "window_s", "60", "window_s must be a number"},
		{"enabled as a string", "enabled", "yes", "enabled must be true or false"},
		{"muted as a number", "muted", 1.0, "muted must be true or false"},
		{"name as a number", "name", 7.0, "name must be a string"},
		{"scope as a string", "scope", "live", "scope must be an object"},
		{"scope with an unknown key", "scope", map[string]any{"stream": "s1"}, `unknown field "stream"`},
		{"scope value not a string", "scope", map[string]any{"app": 5.0}, "scope.app must be a string"},
		{"channel_ids as a string", "channel_ids", "ch-1", "channel_ids must be an array"},
		{"channel_ids with a number", "channel_ids", []any{1.0}, "channel_ids[0] must be a string"},
		{"singular maintenance_window", "maintenance_window", map[string]any{"cron_expr": "0 2 *", "duration_s": 3600.0}, `"maintenance_windows"`},
		{"window written with cron_expr", "maintenance_windows", []any{map[string]any{"cron_expr": "0 2 *", "duration_s": 3600.0}}, "the field is start_cron"},
		{"window longer than a day", "maintenance_windows", []any{map[string]any{"start_cron": "0 22 6", "duration_s": 172800.0}}, "duration_s must be a whole number"},
		{"window with a fractional duration", "maintenance_windows", []any{map[string]any{"start_cron": "0 2 *", "duration_s": 90.5}}, "duration_s must be a whole number"},
		{"window cron out of range", "maintenance_windows", []any{map[string]any{"start_cron": "0 25 *", "duration_s": 3600.0}}, "hour must be a number 0-23"},
		{"window weekday out of range", "maintenance_windows", []any{map[string]any{"start_cron": "0 2 7", "duration_s": 3600.0}}, "weekday 7 out of range"},
		{"window as an object", "maintenance_windows", map[string]any{"start_cron": "0 2 *", "duration_s": 3600.0}, "must be an array"},
		{"fractional window_s", "window_s", 60.5, "window_s must be a whole number"},
		{"negative cooldown", "cooldown_s", -5.0, "cooldown_s must be 0 or more"},
		{"zero sigma", "sigma", 0.0, "sigma must be a positive number"},
		{"unknown rule_type", "rule_type", "ratio", "rule_type must be threshold or anomaly"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := mutateBody(baseValidRule("s126-"+tc.field), tc.field, tc.val)
			r := doJSON(t, http.DefaultClient, http.MethodPost, ts.URL+"/api/v1/alerts/rules", token, body)
			if r.status != http.StatusUnprocessableEntity {
				t.Fatalf("want 422, got %d: %s", r.status, r.body)
			}
			if msg, _ := r.json["message"].(string); !strings.Contains(msg, tc.want) {
				t.Errorf("message %q does not contain %q", msg, tc.want)
			}
		})
	}
}

func TestS126_AlertRule_ValidWindowsAndRoundTrip(t *testing.T) {
	ts, token, cleanup := setupTestServerWithLicense(t, allFreeLicense(t)) // the shipped configuration
	defer cleanup()

	ch := doJSON(t, http.DefaultClient, http.MethodPost, ts.URL+"/api/v1/alerts/channels", token, map[string]any{
		"type": "webhook", "name": "s126-hook", "config": map[string]any{"webhook_url": "https://example.com/hook"},
	})
	if ch.status != http.StatusCreated {
		t.Fatalf("create channel: %d %s", ch.status, ch.body)
	}
	chID, _ := ch.json["id"].(string)

	body := baseValidRule("s126-roundtrip")
	body["channel_ids"] = []any{chID}
	body["scope"] = map[string]any{"app": "live", "stream_id": nil}
	// Two windows (multi-day maintenance is one window per day); the cron is stored
	// whitespace-normalised.
	body["maintenance_windows"] = []any{
		map[string]any{"start_cron": "0  22  6", "duration_s": 7200.0},
		map[string]any{"start_cron": "0 0 0", "duration_s": 86400.0},
	}
	created := doJSON(t, http.DefaultClient, http.MethodPost, ts.URL+"/api/v1/alerts/rules", token, body)
	if created.status != http.StatusCreated {
		t.Fatalf("create: %d %s", created.status, created.body)
	}
	mw, _ := created.json["maintenance_windows"].([]any)
	if len(mw) != 2 {
		t.Fatalf("maintenance_windows = %v, want 2 windows", created.json["maintenance_windows"])
	}
	if w0, _ := mw[0].(map[string]any); w0["start_cron"] != "0 22 6" || w0["duration_s"] != 7200.0 {
		t.Errorf("window 0 stored as %v, want start_cron \"0 22 6\", duration_s 7200", w0)
	}

	// A listed rule — read-only keys and all — can be edited and PUT back whole: that is
	// how a client changes one field (there is no PATCH), and how the web UI keeps the
	// channels and windows it does not edit.
	list := doJSON(t, http.DefaultClient, http.MethodGet, ts.URL+"/api/v1/alerts/rules?limit=200", token, nil)
	var listed map[string]any
	items, _ := list.json["items"].([]any)
	for _, it := range items {
		if m, _ := it.(map[string]any); m["id"] == created.json["id"] {
			listed = m
		}
	}
	if listed == nil {
		t.Fatalf("created rule not in the list: %s", list.body)
	}
	listed["muted"] = true
	id, _ := listed["id"].(string)
	put := doJSON(t, http.DefaultClient, http.MethodPut, ts.URL+"/api/v1/alerts/rules/"+id, token, listed)
	if put.status != http.StatusOK {
		t.Fatalf("PUT of a listed rule: %d %s", put.status, put.body)
	}
	if put.json["muted"] != true {
		t.Errorf("muted not updated: %v", put.json["muted"])
	}
	if ids, _ := put.json["channel_ids"].([]any); len(ids) != 1 || ids[0] != chID {
		t.Errorf("channel_ids after round trip = %v, want [%s]", put.json["channel_ids"], chID)
	}
	if got, _ := put.json["maintenance_windows"].([]any); len(got) != 2 {
		t.Errorf("maintenance_windows after round trip = %v, want 2", put.json["maintenance_windows"])
	}
}

func TestS126_ReportSchedule_RefusesWhatItWouldIgnore(t *testing.T) {
	ts, token, cleanup := setupTestServerWithLicense(t, allFreeLicense(t)) // the shipped configuration
	defer cleanup()

	base := func() map[string]any {
		return map[string]any{"cron": "0 6 1 * *", "format": "pdf", "scope": map[string]any{"app": "live", "tenant": nil}}
	}
	cases := []struct {
		name string
		mut  func(map[string]any)
		want string
	}{
		{"unparseable cron", func(b map[string]any) { b["cron"] = "invalid cron" }, "cron:"},
		{"cron out of range", func(b map[string]any) { b["cron"] = "0 6 32 * *" }, "out of range"},
		{"cron that never fires", func(b map[string]any) { b["cron"] = "0 0 31 2 *" }, "never fires"},
		{"runbook cron_expr", func(b map[string]any) { delete(b, "cron"); b["cron_expr"] = "0 6 1 * *" }, `"cron"`},
		{"runbook app_filter", func(b map[string]any) { b["app_filter"] = "live" }, `"scope": {"app"`},
		{"runbook tenant_filter", func(b map[string]any) { b["tenant_filter"] = "tenant-a" }, `"scope": {"tenant"`},
		{"scope unknown key", func(b map[string]any) { b["scope"] = map[string]any{"stream": "x"} }, `unknown field "stream"`},
		{"format as a number", func(b map[string]any) { b["format"] = 1.0 }, "format must be a string"},
		{"whitelabel unknown key", func(b map[string]any) { b["whitelabel_header"] = map[string]any{"company": "Acme"} }, `unknown field "company"`},
		{"whitelabel without a name", func(b map[string]any) { b["whitelabel_header"] = map[string]any{"address": "1 Main St"} }, "whitelabel_header.name is required"},
		{"whitelabel logo_path", func(b map[string]any) {
			b["whitelabel_header"] = map[string]any{"name": "Acme", "logo_path": "/etc/logo.png"}
		}, "PULSE_REPORT_LOGO_PATH"},
		{"whitelabel as a string", func(b map[string]any) { b["whitelabel_header"] = "Acme" }, "whitelabel_header must be an object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := base()
			tc.mut(b)
			r := doJSON(t, http.DefaultClient, http.MethodPost, ts.URL+"/api/v1/reports/schedules", token, b)
			if r.status != http.StatusUnprocessableEntity {
				t.Fatalf("want 422, got %d: %s", r.status, r.body)
			}
			if msg, _ := r.json["message"].(string); !strings.Contains(msg, tc.want) {
				t.Errorf("message %q does not contain %q", msg, tc.want)
			}
		})
	}

	// The valid shape, and a GET → PUT round trip of what it returns.
	b := base()
	b["whitelabel_header"] = map[string]any{"name": "Acme Streaming", "address": "1 Main St\nSpringfield"}
	created := doJSON(t, http.DefaultClient, http.MethodPost, ts.URL+"/api/v1/reports/schedules", token, b)
	if created.status != http.StatusCreated {
		t.Fatalf("valid schedule: %d %s", created.status, created.body)
	}
	id, _ := created.json["id"].(string)
	created.json["format"] = "csv"
	put := doJSON(t, http.DefaultClient, http.MethodPut, ts.URL+"/api/v1/reports/schedules/"+id, token, created.json)
	if put.status != http.StatusOK {
		t.Fatalf("PUT of a returned schedule: %d %s", put.status, put.body)
	}
	if put.json["format"] != "csv" {
		t.Errorf("format not updated: %v", put.json["format"])
	}
}

func TestS126_AlertChannel_ConfigValidatedAndKeptOnEdit(t *testing.T) {
	ts, token, cleanup := setupTestServerWithLicense(t, allFreeLicense(t)) // the shipped configuration
	defer cleanup()
	url := ts.URL + "/api/v1/alerts/channels"

	refused := []struct {
		name string
		body map[string]any
		want string
	}{
		{"webhook written with url", map[string]any{"type": "webhook", "name": "x", "config": map[string]any{"url": "https://example.com/h"}}, `unknown key "url"`},
		{"webhook without a URL", map[string]any{"type": "webhook", "name": "x", "config": map[string]any{}}, "config.webhook_url is required"},
		{"webhook URL without a scheme", map[string]any{"type": "webhook", "name": "x", "config": map[string]any{"webhook_url": "example.com/h"}}, "must be an http(s) URL"},
		{"slack written with webhook_url", map[string]any{"type": "slack", "name": "x", "config": map[string]any{"webhook_url": "https://hooks.slack.com/x"}}, `unknown key "webhook_url"`},
		{"telegram without a chat", map[string]any{"type": "telegram", "name": "x", "config": map[string]any{"telegram_bot_token": "123:abc"}}, "config.telegram_chat_id is required"},
		{"pagerduty without a key", map[string]any{"type": "pagerduty", "name": "x", "config": map[string]any{}}, "config.pagerduty_routing_key is required"},
		{"email written with to", map[string]any{"type": "email", "name": "x", "config": map[string]any{"to": "a@example.com"}}, `unknown key "to"`},
		{"email smtp_addr without a port", map[string]any{"type": "email", "name": "x", "config": map[string]any{"email_to": "a@example.com", "smtp_addr": "smtp.example.com"}}, "must be host:port"},
		{"email starttls as a string", map[string]any{"type": "email", "name": "x", "config": map[string]any{"email_to": "a@example.com", "starttls": "true"}}, "starttls must be true or false"},
		{"email not an address", map[string]any{"type": "email", "name": "x", "config": map[string]any{"email_to": "ops"}}, "must be a plain e-mail address"},
		{"email with a display name", map[string]any{"type": "email", "name": "x", "config": map[string]any{"email_to": "Ops <ops@example.com>"}}, "must be a plain e-mail address"},
		{"unknown type", map[string]any{"type": "sms", "name": "x", "config": map[string]any{}}, "type must be one of"},
		{"config as a string", map[string]any{"type": "webhook", "name": "x", "config": "https://example.com"}, "config must be an object"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			r := doJSON(t, http.DefaultClient, http.MethodPost, url, token, tc.body)
			if r.status != http.StatusUnprocessableEntity {
				t.Fatalf("want 422, got %d: %s", r.status, r.body)
			}
			if msg, _ := r.json["message"].(string); !strings.Contains(msg, tc.want) {
				t.Errorf("message %q does not contain %q", msg, tc.want)
			}
		})
	}

	// Edit keeps what the body omits. The web form shows the URL but never the signing
	// secret (secrets are write-only); before S126 saving the form erased the secret.
	created := doJSON(t, http.DefaultClient, http.MethodPost, url, token, map[string]any{
		"type": "webhook", "name": "signed", "config": map[string]any{"webhook_url": "https://example.com/a", "webhook_secret": "s3cret"},
	})
	if created.status != http.StatusCreated || created.json["credential_set"] != true {
		t.Fatalf("create signed webhook: %d %s", created.status, created.body)
	}
	id, _ := created.json["id"].(string)
	edited := doJSON(t, http.DefaultClient, http.MethodPut, url+"/"+id, token, map[string]any{
		"type": "webhook", "name": "signed (renamed)", "config": map[string]any{"webhook_url": "https://example.com/b"},
	})
	if edited.status != http.StatusOK {
		t.Fatalf("edit: %d %s", edited.status, edited.body)
	}
	if edited.json["credential_set"] != true {
		t.Errorf("the signing secret was dropped by an edit that did not mention it")
	}
	if cs, _ := edited.json["config_summary"].(map[string]any); cs["webhook_url"] != "https://example.com/b" {
		t.Errorf("webhook_url not updated: %v", edited.json["config_summary"])
	}
	// A key sent empty is removed.
	cleared := doJSON(t, http.DefaultClient, http.MethodPut, url+"/"+id, token, map[string]any{
		"type": "webhook", "name": "signed", "config": map[string]any{"webhook_secret": ""},
	})
	if cleared.status != http.StatusOK || cleared.json["credential_set"] != false {
		t.Errorf("an empty webhook_secret should remove it: %d %s", cleared.status, cleared.body)
	}
	// A type change starts from an empty config: the old type's keys do not carry over.
	switched := doJSON(t, http.DefaultClient, http.MethodPut, url+"/"+id, token, map[string]any{
		"type": "slack", "name": "signed", "config": map[string]any{},
	})
	if switched.status != http.StatusUnprocessableEntity {
		t.Errorf("type change without the new type's required key: want 422, got %d %s", switched.status, switched.body)
	}
}

// An install that serves Pulse directly (the quickstart) gets the hardening headers the
// production proxies add — before S126 it had none and the admin UI could be framed.
func TestS126_SecurityHeadersWithoutAProxy(t *testing.T) {
	ts, token, cleanup := setupTestServerWithLicense(t, allFreeLicense(t))
	defer cleanup()
	for _, path := range []string{"/healthz", "/api/v1/alerts/rules", "/"} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		for k, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "strict-origin-when-cross-origin",
			"Permissions-Policy":     "camera=(), microphone=(), geolocation=()",
		} {
			if got := resp.Header.Values(k); len(got) != 1 || got[0] != want {
				t.Errorf("GET %s: %s = %q, want exactly [%q]", path, k, got, want)
			}
		}
	}
}
