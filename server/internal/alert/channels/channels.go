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

	"github.com/pulse-analytics/pulse/server/internal/ssrfguard"
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
	// STARTTLS controls the TLS upgrade for the SMTP session. It is NOT
	// auto-enabled (the zero value is false); set it explicitly:
	//   false — no STARTTLS; the session is plaintext. Use this for a local /
	//     loopback relay that does not offer TLS (and knows it is plaintext).
	//   true  — STARTTLS is MANDATORY (D-125): if the upgrade fails, Send returns
	//     an error and does NOT fall back to plaintext, so the message body and any
	//     SMTP AUTH credentials are never sent in cleartext after a TLS downgrade.
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
	var n map[string]any
	_ = json.Unmarshal(payload, &n)
	msg := buildEmailMessage(e.cfg, n)

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
			// Fail closed: the operator requested STARTTLS, so a failed upgrade must
			// abort rather than silently continue on a plaintext connection — which
			// would transmit the message body (and any SMTP AUTH credentials) in
			// cleartext after a TLS downgrade.
			return fmt.Errorf("email channel: STARTTLS upgrade failed: %w", err)
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
	if _, err := fmt.Fprint(wc, msg); err != nil {
		wc.Close()
		return fmt.Errorf("email channel: write body: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("email channel: close DATA: %w", err)
	}
	_ = smtpClient.Quit()
	return nil
}

// buildEmailMessage renders the RFC822 message for an alert notification. Header
// values derived from the (publisher-influenced) payload — notably the Subject,
// which embeds the alert title and thus the stream_id scope — are stripped of CR
// and LF via sanitizeHeaderValue to prevent SMTP header injection (a stream named
// "x\r\nBcc: attacker@evil" must not inject a Bcc header).
func buildEmailMessage(cfg EmailConfig, n map[string]any) string {
	title, _ := n["title"].(string)
	isTest, _ := n["test"].(bool)

	subject := fmt.Sprintf("[Pulse Alert] %s", title)
	if isTest {
		subject = "[Pulse TEST] " + subject
	}
	body := buildEmailBody(n)

	msg := strings.Builder{}
	msg.WriteString(fmt.Sprintf("From: %s\r\n", sanitizeHeaderValue(cfg.From)))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", sanitizeHeaderValue(cfg.To)))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", sanitizeHeaderValue(subject)))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)
	msg.WriteString("\r\n")
	return msg.String()
}

// sanitizeHeaderValue strips CR and LF so a payload-derived value cannot inject
// additional SMTP headers. RFC 5322 header values are single-line by definition.
func sanitizeHeaderValue(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
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
// The HTTP client is guarded by ssrfguard.DialControl (Fix E): a webhook URL
// that resolves to a link-local or IMDS address is refused at dial time.
func NewSlackChannel(cfg SlackConfig) *SlackChannel {
	return &SlackChannel{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Control: ssrfguard.DialControl,
				}).DialContext,
			},
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
