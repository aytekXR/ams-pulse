// Package channels defines the notification channel adapter interface and its
// implementations: email (MVP), slack (MVP), telegram, pagerduty, and generic
// webhook (Phase 2). Adapters are pluggable (PRD F5 technical notes) — adding
// a channel type means one new file implementing Channel, no evaluator changes.
package channels

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

// Channel delivers notifications to one configured destination.
type Channel interface {
	// Name returns the channel type identifier (email, slack, ...).
	Name() string
	// Send delivers one notification; must be idempotent per alert_id+state.
	Send(ctx context.Context, payload []byte) error
}

// Registry maps channel IDs to their Channel implementations.
type Registry struct {
	channels map[string]Channel
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{channels: make(map[string]Channel)}
}

// Register adds a channel to the registry.
func (r *Registry) Register(id string, ch Channel) {
	r.channels[id] = ch
}

// Get fetches a channel by ID.
func (r *Registry) Get(id string) (Channel, bool) {
	ch, ok := r.channels[id]
	return ch, ok
}

// Remove removes a channel from the registry.
func (r *Registry) Remove(id string) {
	delete(r.channels, id)
}

// ─── Email channel ────────────────────────────────────────────────────────────

// EmailConfig is the configuration for an email notification channel.
type EmailConfig struct {
	// SMTPAddr is host:port of the SMTP server (default: localhost:587).
	SMTPAddr string
	// From is the sender address.
	From string
	// To is the recipient address.
	To string
	// Username for SMTP AUTH (optional).
	Username string
	// Password for SMTP AUTH (optional).
	Password string
	// STARTTLS enables TLS upgrade (default true for port 587).
	STARTTLS bool
}

// EmailChannel sends alert notifications via SMTP.
type EmailChannel struct {
	cfg EmailConfig
}

// NewEmailChannel creates a new email channel.
func NewEmailChannel(cfg EmailConfig) *EmailChannel {
	if cfg.SMTPAddr == "" {
		cfg.SMTPAddr = "localhost:587"
	}
	if cfg.From == "" {
		cfg.From = "pulse-alerts@localhost"
	}
	return &EmailChannel{cfg: cfg}
}

// Name returns "email".
func (e *EmailChannel) Name() string { return "email" }

// Send delivers a notification via SMTP.
// The payload is a JSON-encoded alert notification (alert-notification.schema.json).
func (e *EmailChannel) Send(ctx context.Context, payload []byte) error {
	// Parse payload to build subject.
	var n map[string]any
	_ = json.Unmarshal(payload, &n)
	title, _ := n["title"].(string)
	state, _ := n["state"].(string)
	severity, _ := n["severity"].(string)
	isTest, _ := n["test"].(bool)

	subject := fmt.Sprintf("[Pulse Alert] %s", title)
	if isTest {
		subject = "[Pulse TEST] " + subject
	}
	body := buildEmailBody(n)

	msg := strings.Builder{}
	msg.WriteString(fmt.Sprintf("From: %s\r\n", e.cfg.From))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", e.cfg.To))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)
	msg.WriteString("\r\n")
	_ = state
	_ = severity

	host, _, _ := net.SplitHostPort(e.cfg.SMTPAddr)

	// Dial with deadline from context.
	deadline, ok := ctx.Deadline()
	dialer := &net.Dialer{}
	if ok {
		dialer.Deadline = deadline
	}
	conn, err := dialer.DialContext(ctx, "tcp", e.cfg.SMTPAddr)
	if err != nil {
		return fmt.Errorf("email channel: dial %s: %w", e.cfg.SMTPAddr, err)
	}

	var smtpClient *smtp.Client
	smtpClient, err = smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("email channel: smtp client: %w", err)
	}
	defer smtpClient.Close()

	// STARTTLS if configured.
	if e.cfg.STARTTLS {
		tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		if err := smtpClient.StartTLS(tlsCfg); err != nil {
			// Non-fatal if TLS not supported (e.g. test SMTP server).
			_ = err
		}
	}

	// AUTH if credentials provided.
	if e.cfg.Username != "" && e.cfg.Password != "" {
		auth := smtp.PlainAuth("", e.cfg.Username, e.cfg.Password, host)
		if err := smtpClient.Auth(auth); err != nil {
			return fmt.Errorf("email channel: smtp auth: %w", err)
		}
	}

	if err := smtpClient.Mail(e.cfg.From); err != nil {
		return fmt.Errorf("email channel: MAIL FROM: %w", err)
	}
	if err := smtpClient.Rcpt(e.cfg.To); err != nil {
		return fmt.Errorf("email channel: RCPT TO: %w", err)
	}
	wc, err := smtpClient.Data()
	if err != nil {
		return fmt.Errorf("email channel: DATA: %w", err)
	}
	if _, err := fmt.Fprint(wc, msg.String()); err != nil {
		wc.Close()
		return fmt.Errorf("email channel: write body: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("email channel: close DATA: %w", err)
	}
	_ = smtpClient.Quit()
	return nil
}

func buildEmailBody(n map[string]any) string {
	var b strings.Builder
	b.WriteString("Pulse Alert Notification\n")
	b.WriteString(strings.Repeat("=", 40) + "\n\n")
	if title, ok := n["title"].(string); ok {
		b.WriteString(fmt.Sprintf("Title:     %s\n", title))
	}
	if state, ok := n["state"].(string); ok {
		b.WriteString(fmt.Sprintf("State:     %s\n", state))
	}
	if severity, ok := n["severity"].(string); ok {
		b.WriteString(fmt.Sprintf("Severity:  %s\n", severity))
	}
	if metric, ok := n["metric"].(string); ok {
		b.WriteString(fmt.Sprintf("Metric:    %s\n", metric))
	}
	if value, ok := n["value"].(float64); ok {
		b.WriteString(fmt.Sprintf("Value:     %.4g\n", value))
	}
	if threshold, ok := n["threshold"].(float64); ok {
		b.WriteString(fmt.Sprintf("Threshold: %.4g\n", threshold))
	}
	if scope, ok := n["scope"].(map[string]any); ok {
		b.WriteString("Scope:\n")
		for k, v := range scope {
			b.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
	}
	if isTest, ok := n["test"].(bool); ok && isTest {
		b.WriteString("\n[This is a test notification]\n")
	}
	if url, ok := n["dashboard_url"].(string); ok && url != "" {
		b.WriteString(fmt.Sprintf("\nDashboard: %s\n", url))
	}
	return b.String()
}

// ─── Slack channel ────────────────────────────────────────────────────────────

// SlackConfig is the configuration for a Slack incoming webhook channel.
type SlackConfig struct {
	// WebhookURL is the Slack incoming webhook URL.
	WebhookURL string
	// Channel is the Slack channel name (for display only).
	Channel string
}

// SlackChannel sends alert notifications via Slack incoming webhooks.
type SlackChannel struct {
	cfg    SlackConfig
	client *http.Client
}

// NewSlackChannel creates a new Slack channel.
func NewSlackChannel(cfg SlackConfig) *SlackChannel {
	return &SlackChannel{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns "slack".
func (s *SlackChannel) Name() string { return "slack" }

// Send delivers a notification via Slack incoming webhook.
func (s *SlackChannel) Send(ctx context.Context, payload []byte) error {
	var n map[string]any
	_ = json.Unmarshal(payload, &n)

	title, _ := n["title"].(string)
	state, _ := n["state"].(string)
	severity, _ := n["severity"].(string)
	isTest, _ := n["test"].(bool)

	// Build a simple Slack message.
	stateEmoji := map[string]string{
		"firing":   ":red_circle:",
		"resolved": ":large_green_circle:",
	}
	emoji := stateEmoji[state]
	if emoji == "" {
		emoji = ":large_yellow_circle:"
	}

	text := fmt.Sprintf("%s *%s* [%s]", emoji, title, strings.ToUpper(severity))
	if isTest {
		text = ":test_tube: " + text + " *(TEST)*"
	}

	if metric, ok := n["metric"].(string); ok {
		text += fmt.Sprintf("\nMetric: `%s`", metric)
	}
	if value, ok := n["value"].(float64); ok {
		text += fmt.Sprintf(" | Value: `%.4g`", value)
	}
	if threshold, ok := n["threshold"].(float64); ok {
		text += fmt.Sprintf(" | Threshold: `%.4g`", threshold)
	}
	if scope, ok := n["scope"].(map[string]any); ok {
		var scopeParts []string
		for k, v := range scope {
			if v != nil && v != "" {
				scopeParts = append(scopeParts, fmt.Sprintf("%s=%v", k, v))
			}
		}
		if len(scopeParts) > 0 {
			text += fmt.Sprintf("\nScope: `%s`", strings.Join(scopeParts, ", "))
		}
	}
	if url, ok := n["dashboard_url"].(string); ok && url != "" {
		text += fmt.Sprintf("\n<%s|Open Dashboard>", url)
	}

	slackBody := map[string]any{
		"text": text,
	}
	if s.cfg.Channel != "" {
		slackBody["channel"] = s.cfg.Channel
	}

	body, err := json.Marshal(slackBody)
	if err != nil {
		return fmt.Errorf("slack channel: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack channel: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack channel: send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack channel: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ─── Noop channel (for testing) ───────────────────────────────────────────────

// NoopChannel discards all notifications. Used for testing.
// It is safe for concurrent use (delivery goroutines call Send concurrently).
type NoopChannel struct {
	mu       sync.Mutex
	Received [][]byte
}

// Name returns "noop".
func (n *NoopChannel) Name() string { return "noop" }

// Send records the payload without delivering it.
func (n *NoopChannel) Send(_ context.Context, payload []byte) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Received = append(n.Received, payload)
	return nil
}
