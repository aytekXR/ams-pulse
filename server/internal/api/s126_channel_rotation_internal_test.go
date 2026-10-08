package api

import (
	"context"
	"strings"
	"testing"

	"github.com/aytekXR/ams-pulse/server/internal/store/meta"
)

// After a PULSE_SECRET_KEY rotation a channel's stored secrets no longer decrypt. Editing the
// channel and re-entering them is how it is repaired — the merge-on-update must not turn
// that edit into a 422 (S126 red team, from a reviewer's "safe" claim).
func TestS126_ChannelEditAfterKeyRotation(t *testing.T) {
	ctx := context.Background()
	oldStore, err := meta.New(ctx, "sqlite", ":memory:", "s126-old-secret-key")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	defer oldStore.Close()
	newStore, err := meta.New(ctx, "sqlite", ":memory:", "s126-new-secret-key")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	defer newStore.Close()

	stored, err := alertChannelFromAPI(map[string]any{
		"type": "slack", "name": "ops",
		"config": map[string]any{"slack_webhook_url": "https://hooks.slack.com/services/OLD", "slack_channel": "#ops"},
	}, oldStore, nil)
	if err != nil || stored.ConfigEnc == "" {
		t.Fatalf("setup: %v (enc %q)", err, stored.ConfigEnc)
	}
	if _, err := newStore.Decrypt(stored.ConfigEnc); err == nil {
		t.Fatalf("setup: the new key must not decrypt the old secret")
	}

	// The repair: re-enter the secret. Public keys still carry over.
	repaired, err := alertChannelFromAPI(map[string]any{
		"type": "slack", "name": "ops",
		"config": map[string]any{"slack_webhook_url": "https://hooks.slack.com/services/NEW"},
	}, newStore, &stored)
	if err != nil {
		t.Fatalf("re-entering the secret after a key rotation was refused: %v", err)
	}
	plain, err := newStore.Decrypt(repaired.ConfigEnc)
	if err != nil || !strings.Contains(plain, "services/NEW") {
		t.Errorf("repaired secret = %q, %v; want the new URL under the new key", plain, err)
	}
	if !strings.Contains(repaired.ConfigPublic, "#ops") {
		t.Errorf("public config not carried over: %s", repaired.ConfigPublic)
	}

	// An edit that does not re-enter it is told what is missing — not a decrypt error.
	_, err = alertChannelFromAPI(map[string]any{"type": "slack", "name": "renamed", "config": map[string]any{}}, newStore, &stored)
	if err == nil || !strings.Contains(err.Error(), "config.slack_webhook_url is required") {
		t.Errorf("edit without the undecryptable secret: err = %v, want the required-key message", err)
	}
}
