package alert

// BuildChannelFromRow constructs a channels.Channel from a stored meta.AlertChannelRow.
//
// It decrypts ConfigEnc (secret fields) and merges with ConfigPublic (non-secret
// fields), then maps the unified config to the typed channels constructor for row.Type.
//
// This is the single authoritative factory shared by:
//   - the alert evaluator (syncRegistryFromStore, called on every tick)
//   - the API test-fire handler (server.go handleTestAlertChannel)
//
// Error policy: decrypt failure or unknown type returns a non-nil error. The
// caller must log + skip the channel — never crash the evaluator tick.

import (
	"encoding/json"
	"fmt"

	"github.com/pulse-analytics/pulse/server/internal/alert/channels"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// BuildChannelFromRow is the shared channel factory. See file-level doc for usage.
func BuildChannelFromRow(store *meta.Store, row *meta.AlertChannelRow) (channels.Channel, error) {
	// Parse public (non-secret) config fields.
	publicMap := map[string]any{}
	if row.ConfigPublic != "" && row.ConfigPublic != "{}" && row.ConfigPublic != "null" {
		if err := json.Unmarshal([]byte(row.ConfigPublic), &publicMap); err != nil {
			return nil, fmt.Errorf("parse public config: %w", err)
		}
	}

	// Decrypt and parse secret config fields.
	secretMap := map[string]any{}
	if row.ConfigEnc != "" {
		decrypted, err := store.Decrypt(row.ConfigEnc)
		if err != nil {
			return nil, fmt.Errorf("decrypt channel config: %w", err)
		}
		if err := json.Unmarshal([]byte(decrypted), &secretMap); err != nil {
			return nil, fmt.Errorf("parse secret config: %w", err)
		}
	}

	// Merge: secret fields override public in case of collision (should not happen).
	merged := make(map[string]any, len(publicMap)+len(secretMap))
	for k, v := range publicMap {
		merged[k] = v
	}
	for k, v := range secretMap {
		merged[k] = v
	}

	str := func(key string) string {
		v, _ := merged[key].(string)
		return v
	}

	switch row.Type {
	case "webhook":
		cfg := channels.WebhookConfig{
			URL:    str("webhook_url"), // contract key: webhook_url (was "url" — FIXED)
			Secret: str("webhook_secret"),
		}
		return channels.NewWebhookChannel(cfg), nil
	case "slack":
		cfg := channels.SlackConfig{
			WebhookURL: str("slack_webhook_url"),
			Channel:    str("slack_channel"), // contract key: slack_channel (was "channel" — FIXED)
		}
		return channels.NewSlackChannel(cfg), nil
	case "email":
		cfg := channels.EmailConfig{
			SMTPAddr: str("smtp_addr"),
			From:     str("from"),
			To:       str("email_to"), // contract key: email_to (was "to" — FIXED)
			Username: str("username"),
			Password: str("password"),
		}
		if v, ok := merged["starttls"].(bool); ok {
			cfg.STARTTLS = v
		}
		return channels.NewEmailChannel(cfg), nil
	case "telegram":
		cfg := channels.TelegramConfig{
			BotToken: str("telegram_bot_token"),
			ChatID:   str("telegram_chat_id"), // contract key: telegram_chat_id (was "chat_id" — FIXED)
		}
		return channels.NewTelegramChannel(cfg), nil
	case "pagerduty":
		cfg := channels.PagerDutyConfig{
			RoutingKey: str("pagerduty_routing_key"),
			Severity:   str("pagerduty_severity"), // contract key: pagerduty_severity (was "severity" — FIXED)
		}
		return channels.NewPagerDutyChannel(cfg), nil
	default:
		return nil, fmt.Errorf("unknown channel type %q", row.Type)
	}
}
