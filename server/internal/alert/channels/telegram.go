package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TelegramConfig is the configuration for a Telegram bot notification channel.
// Requires a bot token from @BotFather and a chat ID (user, group, or channel).
type TelegramConfig struct {
	// BotToken is the Telegram bot API token (from @BotFather). SENSITIVE: encrypted at rest.
	BotToken string
	// ChatID is the target chat ID (can be a user ID, group ID, or "@channel_name").
	ChatID string
}

// TelegramChannel sends alert notifications via Telegram Bot API.
// API: https://core.telegram.org/bots/api#sendmessage
type TelegramChannel struct {
	cfg    TelegramConfig
	client *http.Client
	apiURL string // format: "https://api.telegram.org/bot%s/sendMessage"; overridable for tests
}

// NewTelegramChannel creates a new Telegram channel.
func NewTelegramChannel(cfg TelegramConfig) *TelegramChannel {
	return &TelegramChannel{
		cfg:    cfg,
		apiURL: "https://api.telegram.org/bot%s/sendMessage",
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NewTelegramChannelWithURL creates a Telegram channel with a custom API URL template.
// Used for testing with httptest fakes. The URL template must contain %s for the bot token.
func NewTelegramChannelWithURL(cfg TelegramConfig, apiURLTemplate string) *TelegramChannel {
	return &TelegramChannel{
		cfg:    cfg,
		apiURL: apiURLTemplate,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns "telegram".
func (t *TelegramChannel) Name() string { return "telegram" }

// redact removes the bot token from an error's text so the secret never reaches
// logs. Returns a plain error (unwrappable chain intentionally dropped) because
// the underlying *url.Error embeds the token-bearing URL in its message.
func (t *TelegramChannel) redact(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if t.cfg.BotToken != "" {
		msg = strings.ReplaceAll(msg, t.cfg.BotToken, "REDACTED")
	}
	return errors.New(msg)
}

// Send delivers a notification via Telegram Bot API sendMessage.
func (t *TelegramChannel) Send(ctx context.Context, payload []byte) error {
	var n map[string]any
	_ = json.Unmarshal(payload, &n)

	text := buildTelegramMessage(n)

	reqBody := map[string]any{
		"chat_id":    t.cfg.ChatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("telegram channel: marshal: %w", err)
	}

	urlTemplate := t.apiURL
	if urlTemplate == "" {
		urlTemplate = "https://api.telegram.org/bot%s/sendMessage"
	}
	apiURL := fmt.Sprintf(urlTemplate, t.cfg.BotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("telegram channel: build request: %w", t.redact(err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		// t.redact strips the bot token: client.Do returns a *url.Error whose text
		// embeds the full request URL (…/bot<token>/sendMessage), which would
		// otherwise leak the secret to whatever logs this returned error.
		return fmt.Errorf("telegram channel: send: %w", t.redact(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiResp map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&apiResp)
		return fmt.Errorf("telegram channel: API returned %d: %v", resp.StatusCode, apiResp)
	}
	return nil
}

func buildTelegramMessage(n map[string]any) string {
	var b strings.Builder

	state, _ := n["state"].(string)
	title, _ := n["title"].(string)
	severity, _ := n["severity"].(string)
	isTest, _ := n["test"].(bool)

	stateIcon := map[string]string{
		"firing":   "🔴",
		"resolved": "🟢",
	}
	icon := stateIcon[state]
	if icon == "" {
		icon = "🟡"
	}

	if isTest {
		b.WriteString("🧪 <b>TEST NOTIFICATION</b>\n")
	}
	b.WriteString(fmt.Sprintf("%s <b>%s</b> [%s]\n", icon, escapeHTML(title), strings.ToUpper(escapeHTML(severity))))

	if metric, ok := n["metric"].(string); ok {
		b.WriteString(fmt.Sprintf("Metric: <code>%s</code>", escapeHTML(metric)))
		if value, ok := n["value"].(float64); ok {
			b.WriteString(fmt.Sprintf(" = <code>%.4g</code>", value))
		}
		b.WriteString("\n")
	}
	if threshold, ok := n["threshold"].(float64); ok {
		b.WriteString(fmt.Sprintf("Threshold: <code>%.4g</code>\n", threshold))
	}
	if scope, ok := n["scope"].(map[string]any); ok {
		var parts []string
		for k, v := range scope {
			if v != nil && v != "" {
				parts = append(parts, fmt.Sprintf("%s=%v", k, v))
			}
		}
		if len(parts) > 0 {
			b.WriteString(fmt.Sprintf("Scope: <code>%s</code>\n", escapeHTML(strings.Join(parts, ", "))))
		}
	}
	if url, ok := n["dashboard_url"].(string); ok && url != "" {
		b.WriteString(fmt.Sprintf("<a href=\"%s\">Open Dashboard</a>\n", escapeHTMLAttr(url)))
	}

	return b.String()
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// escapeHTMLAttr escapes a value for use inside a double-quoted HTML attribute
// (Telegram HTML parse mode). escapeHTML covers &,<,>; this also escapes the
// double-quote so the value cannot break out of the href="…" attribute. Defense
// in depth: dashboard_url is currently operator-derived (baseURL+"/alerts"), but
// it is the one field rendered into markup unescaped.
func escapeHTMLAttr(s string) string {
	return strings.ReplaceAll(escapeHTML(s), "\"", "&quot;")
}
