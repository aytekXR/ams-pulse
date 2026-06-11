// Package channels defines the notification channel adapter interface and its
// implementations: email (MVP), slack (MVP), telegram, pagerduty, and generic
// webhook (Phase 2). Adapters are pluggable (PRD F5 technical notes) — adding
// a channel type means one new file implementing Channel, no evaluator changes.
package channels

import "context"

// Channel delivers notifications to one configured destination.
type Channel interface {
	// Name returns the channel type identifier (email, slack, ...).
	Name() string
	// Send delivers one notification; must be idempotent per alert_id+state.
	Send(ctx context.Context, payload []byte) error
}

// TODO(BE-02): email.go, slack.go (MVP); telegram.go, pagerduty.go,
// genericwebhook.go (Phase 2). Each with a TestFire path for the
// per-channel test button.
