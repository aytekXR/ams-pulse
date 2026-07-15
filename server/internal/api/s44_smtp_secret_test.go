// S44 (D-106) — email/SMTP alert-channel credentials must be encrypted at rest.
//
// Before the fix, alertChannelFromAPI's secretFields allowlist omitted the email
// "password"/"username" keys, so they were serialized into config_public in
// plaintext (config_enc left empty). factory.BuildChannelFromRow merges
// public+decrypted config on read, so the send path worked either way — which is
// exactly why the leak was silent. This test posts an email channel and inspects
// the STORED row: the password must not appear in config_public, and config_enc
// must be populated.
//
// Mutation proof: remove "password"/"username" from secretFields in
// alertChannelFromAPI → the plaintext password lands in config_public → RED.
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAlertChannel_EmailCredentialsEncryptedAtRest(t *testing.T) {
	ts, token, ms, cleanup := setupEnterpriseServer(t)
	defer cleanup()

	const smtpPassword = "smtp_secret_xyz_do_not_leak"
	const smtpUsername = "smtp_user_do_not_leak"
	body, _ := json.Marshal(map[string]any{
		"type": "email",
		"name": "ops-email",
		"config": map[string]any{
			"smtp_addr": "smtp.example.com:587",
			"from":      "alerts@example.com",
			"email_to":  "ops@example.com",
			"username":  smtpUsername,
			"password":  smtpPassword,
		},
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/alerts/channels", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader(token))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /alerts/channels: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, b)
	}

	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	// The API response signals a stored credential set.
	if created["credential_set"] != true {
		t.Errorf("expected credential_set=true in response, got %v", created["credential_set"])
	}

	// Inspect the STORED row directly — the ground truth.
	rows, err := ms.ListAlertChannels(context.Background(), 100, "")
	if err != nil {
		t.Fatalf("ListAlertChannels: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 stored channel, got %d", len(rows))
	}
	row := rows[0]
	if strings.Contains(row.ConfigPublic, smtpPassword) {
		t.Errorf("SMTP password stored in plaintext config_public: %q", row.ConfigPublic)
	}
	if strings.Contains(row.ConfigPublic, smtpUsername) {
		t.Errorf("SMTP username stored in plaintext config_public: %q", row.ConfigPublic)
	}
	if row.ConfigEnc == "" {
		t.Error("config_enc is empty — email credentials were not encrypted")
	}
	// Sanity: the encrypted blob decrypts back to the credentials, proving the
	// send path (factory merge) still recovers them.
	dec, err := ms.Decrypt(row.ConfigEnc)
	if err != nil {
		t.Fatalf("Decrypt config_enc: %v", err)
	}
	if !strings.Contains(dec, smtpPassword) {
		t.Errorf("decrypted config_enc missing password; got %q", dec)
	}
}
