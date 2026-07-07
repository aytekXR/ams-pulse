package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PagerDutyConfig is the configuration for a PagerDuty Events API v2 channel.
type PagerDutyConfig struct {
	// RoutingKey is the PagerDuty Events API v2 integration key. SENSITIVE: encrypted at rest.
	RoutingKey string
	// Severity overrides the default severity mapping (optional).
	// If empty, Pulse alert severity maps directly: critical→critical, warning→warning, info→info.
	Severity string
}

// PagerDutyChannel sends alert notifications via PagerDuty Events API v2.
// Spec: https://developer.pagerduty.com/docs/ZG9jOjExMDI5NTgw-send-an-alert-event
type PagerDutyChannel struct {
	cfg    PagerDutyConfig
	client *http.Client
	apiURL string // overridable for tests
}

// NewPagerDutyChannel creates a new PagerDuty channel.
func NewPagerDutyChannel(cfg PagerDutyConfig) *PagerDutyChannel {
	return &PagerDutyChannel{
		cfg:    cfg,
		apiURL: "https://events.pagerduty.com/v2/enqueue",
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Name returns "pagerduty".
func (p *PagerDutyChannel) Name() string { return "pagerduty" }

// Send delivers a notification via PagerDuty Events API v2.
// firing → trigger event; resolved → resolve event.
func (p *PagerDutyChannel) Send(ctx context.Context, payload []byte) error {
	var n map[string]any
	_ = json.Unmarshal(payload, &n)

	state, _ := n["state"].(string)
	title, _ := n["title"].(string)
	alertID, _ := n["alert_id"].(string)
	severity, _ := n["severity"].(string)
	isTest, _ := n["test"].(bool)

	// Map Pulse state to PagerDuty event action.
	action := "trigger"
	if state == "resolved" {
		action = "resolve"
	}

	// Map Pulse severity to PD severity.
	pdSeverity := mapPDSeverity(severity)
	if p.cfg.Severity != "" {
		pdSeverity = p.cfg.Severity
	}

	summary := title
	if isTest {
		summary = "[TEST] " + summary
	}

	// Build PagerDuty Events API v2 payload.
	event := map[string]any{
		"routing_key":  p.cfg.RoutingKey,
		"event_action": action,
		"dedup_key":    alertID, // idempotency: same alert_id = same PD incident
		"payload": map[string]any{
			"summary":        summary,
			"severity":       pdSeverity,
			"source":         "pulse-analytics",
			"component":      buildPDComponent(n),
			"custom_details": buildPDDetails(n),
		},
	}

	if url, ok := n["dashboard_url"].(string); ok && url != "" {
		event["links"] = []map[string]string{
			{"href": url, "text": "Pulse Dashboard"},
		}
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("pagerduty channel: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("pagerduty channel: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("pagerduty channel: send: %w", err)
	}
	defer resp.Body.Close()

	// PD returns 202 Accepted on success.
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		var apiResp map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&apiResp)
		return fmt.Errorf("pagerduty channel: API returned %d: %v", resp.StatusCode, apiResp)
	}
	return nil
}

// SetAPIURL overrides the PagerDuty API URL (for testing with httptest fakes).
func (p *PagerDutyChannel) SetAPIURL(url string) {
	p.apiURL = url
}

func mapPDSeverity(s string) string {
	switch strings.ToLower(s) {
	case "critical":
		return "critical"
	case "warning":
		return "warning"
	case "info":
		return "info"
	default:
		return "error"
	}
}

func buildPDComponent(n map[string]any) string {
	if metric, ok := n["metric"].(string); ok && metric != "" {
		return "pulse:" + metric
	}
	return "pulse"
}

func buildPDDetails(n map[string]any) map[string]any {
	details := map[string]any{}
	if metric, ok := n["metric"].(string); ok {
		details["metric"] = metric
	}
	if value, ok := n["value"].(float64); ok {
		details["value"] = fmt.Sprintf("%.4g", value)
	}
	if threshold, ok := n["threshold"].(float64); ok {
		details["threshold"] = fmt.Sprintf("%.4g", threshold)
	}
	if scope, ok := n["scope"].(map[string]any); ok {
		for k, v := range scope {
			if v != nil && v != "" {
				details["scope_"+k] = v
			}
		}
	}
	if groupKey, ok := n["group_key"].(string); ok && groupKey != "" {
		details["group_key"] = groupKey
	}
	return details
}
