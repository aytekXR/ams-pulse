package main

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runLicensegen invokes the licensegen binary with the given flags and returns
// (stdout, stderr, exit-code-zero).
func runLicensegen(t *testing.T, args ...string) (stdout, stderr string, ok bool) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.String(), errBuf.String(), err == nil
}

// parseEnvLines parses a KEY=value env-file payload from stdout and returns
// a map of all declared keys.
func parseEnvLines(t *testing.T, stdout string) map[string]string {
	t.Helper()
	m := make(map[string]string)
	sc := bufio.NewScanner(strings.NewReader(strings.TrimSpace(stdout)))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			t.Fatalf("env-file line has no '=': %q", line)
		}
		m[line[:idx]] = line[idx+1:]
	}
	return m
}

// TestOutputTwoLines checks that stdout is exactly two KEY=value lines with the
// expected names.
func TestOutputTwoLines(t *testing.T) {
	stdout, stderr, ok := runLicensegen(t, "-tier", "pro")
	if !ok {
		t.Fatalf("licensegen exited non-zero\nstderr: %s", stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected exactly 2 lines of stdout, got %d:\n%s", len(lines), stdout)
	}
	env := parseEnvLines(t, stdout)
	if _, ok := env["PULSE_LICENSE_KEY"]; !ok {
		t.Error("stdout missing PULSE_LICENSE_KEY")
	}
	if _, ok := env["PULSE_LICENSE_PUBKEY"]; !ok {
		t.Error("stdout missing PULSE_LICENSE_PUBKEY")
	}
}

// TestSignatureVerifies checks that the ed25519 signature in the license key is
// valid against the published public key.
func TestSignatureVerifies(t *testing.T) {
	stdout, stderr, ok := runLicensegen(t, "-tier", "pro")
	if !ok {
		t.Fatalf("licensegen exited non-zero\nstderr: %s", stderr)
	}
	env := parseEnvLines(t, stdout)

	pubKeyHex := env["PULSE_LICENSE_PUBKEY"]
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		t.Fatalf("hex-decode public key: %v", err)
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		t.Fatalf("public key wrong size: got %d, want %d", len(pubKeyBytes), ed25519.PublicKeySize)
	}
	pubKey := ed25519.PublicKey(pubKeyBytes)

	licKey := env["PULSE_LICENSE_KEY"]
	parts := strings.SplitN(licKey, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("PULSE_LICENSE_KEY not in <claims>.<sig> format: %q", licKey)
	}
	claimsData, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("base64-decode claims: %v", err)
	}
	sig, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("base64-decode signature: %v", err)
	}

	if !ed25519.Verify(pubKey, claimsData, sig) {
		t.Error("ed25519 signature verification FAILED")
	}
}

// TestClaimsContent verifies that decoded claims carry the expected tier and
// data_api fields.
func TestClaimsContent(t *testing.T) {
	cases := []struct {
		tier    string
		dataAPI bool
	}{
		{"pro", true},
		{"business", true},
		{"enterprise", true},
		{"free", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.tier, func(t *testing.T) {
			stdout, stderr, ok := runLicensegen(t, "-tier", tc.tier)
			if !ok {
				t.Fatalf("licensegen exited non-zero for tier %q\nstderr: %s", tc.tier, stderr)
			}
			env := parseEnvLines(t, stdout)

			parts := strings.SplitN(env["PULSE_LICENSE_KEY"], ".", 2)
			if len(parts) != 2 {
				t.Fatalf("invalid key format")
			}
			claimsData, err := base64.StdEncoding.DecodeString(parts[0])
			if err != nil {
				t.Fatalf("base64-decode claims: %v", err)
			}

			var c struct {
				Tier    string `json:"tier"`
				DataAPI bool   `json:"data_api"`
			}
			if err := json.Unmarshal(claimsData, &c); err != nil {
				t.Fatalf("unmarshal claims: %v", err)
			}
			if c.Tier != tc.tier {
				t.Errorf("claims tier: got %q, want %q", c.Tier, tc.tier)
			}
			if c.DataAPI != tc.dataAPI {
				t.Errorf("claims data_api: got %v, want %v", c.DataAPI, tc.dataAPI)
			}
		})
	}
}

// TestUnknownTierErrors verifies that an invalid -tier value causes a non-zero
// exit code.
func TestUnknownTierErrors(t *testing.T) {
	_, _, ok := runLicensegen(t, "-tier", "platinum")
	if ok {
		t.Error("expected non-zero exit for unknown tier, got zero")
	}
}

// TestPrivkeyValidFile verifies that -privkey loads a hex-encoded private key,
// signs the claims with it, and prints the matching public key.
func TestPrivkeyValidFile(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	keyFile := filepath.Join(t.TempDir(), "vendor.priv")
	if err := os.WriteFile(keyFile, []byte(hex.EncodeToString(priv)), 0600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	stdout, stderr, ok := runLicensegen(t, "-privkey", keyFile, "-tier", "pro")
	if !ok {
		t.Fatalf("licensegen exited non-zero\nstderr: %s", stderr)
	}

	env := parseEnvLines(t, stdout)

	if got, want := env["PULSE_LICENSE_PUBKEY"], hex.EncodeToString(pub); got != want {
		t.Errorf("PULSE_LICENSE_PUBKEY mismatch\ngot:  %s\nwant: %s", got, want)
	}

	parts := strings.SplitN(env["PULSE_LICENSE_KEY"], ".", 2)
	if len(parts) != 2 {
		t.Fatalf("invalid key format")
	}
	claimsData, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("base64-decode claims: %v", err)
	}
	sig, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("base64-decode sig: %v", err)
	}
	if !ed25519.Verify(pub, claimsData, sig) {
		t.Error("signature verification failed with provided public key")
	}
}

// TestPrivkeyMissingFile verifies that a non-existent privkey path causes
// a non-zero exit with "privkey" in stderr.
func TestPrivkeyMissingFile(t *testing.T) {
	_, stderr, ok := runLicensegen(t, "-privkey", "/nonexistent/vendor.priv")
	if ok {
		t.Fatal("expected non-zero exit for missing privkey file")
	}
	if !strings.Contains(stderr, "privkey") {
		t.Errorf("stderr does not mention 'privkey': %s", stderr)
	}
}

// TestPrivkeyMalformedKey verifies that invalid hex and wrong-length keys
// each cause a non-zero exit with "privkey" in stderr.
func TestPrivkeyMalformedKey(t *testing.T) {
	dir := t.TempDir()

	t.Run("invalid_hex", func(t *testing.T) {
		keyFile := filepath.Join(dir, "bad_hex.priv")
		if err := os.WriteFile(keyFile, []byte("not-valid-hex"), 0600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		_, stderr, ok := runLicensegen(t, "-privkey", keyFile)
		if ok {
			t.Fatal("expected non-zero exit for invalid hex")
		}
		if !strings.Contains(stderr, "privkey") {
			t.Errorf("stderr does not mention 'privkey': %s", stderr)
		}
	})

	t.Run("wrong_length", func(t *testing.T) {
		// 32 bytes → 64 hex chars; ed25519 private key needs 64 bytes (128 hex).
		shortKey := make([]byte, 32)
		keyFile := filepath.Join(dir, "short.priv")
		if err := os.WriteFile(keyFile, []byte(hex.EncodeToString(shortKey)), 0600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		_, stderr, ok := runLicensegen(t, "-privkey", keyFile)
		if ok {
			t.Fatal("expected non-zero exit for wrong-length key")
		}
		if !strings.Contains(stderr, "privkey") {
			t.Errorf("stderr does not mention 'privkey': %s", stderr)
		}
	})
}

// TestExpiresPositiveDays verifies that -expires 30 sets expires_at within
// the expected range (now+29d to now+31d) in milliseconds.
func TestExpiresPositiveDays(t *testing.T) {
	before := time.Now().UTC()
	stdout, stderr, ok := runLicensegen(t, "-expires", "30", "-tier", "pro")
	after := time.Now().UTC()
	if !ok {
		t.Fatalf("licensegen exited non-zero\nstderr: %s", stderr)
	}

	env := parseEnvLines(t, stdout)
	parts := strings.SplitN(env["PULSE_LICENSE_KEY"], ".", 2)
	if len(parts) != 2 {
		t.Fatalf("invalid key format")
	}
	claimsData, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("base64-decode claims: %v", err)
	}

	var c struct {
		ExpiresAt *int64 `json:"expires_at"`
	}
	if err := json.Unmarshal(claimsData, &c); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if c.ExpiresAt == nil {
		t.Fatal("claims missing expires_at")
	}

	minMs := before.Add(29 * 24 * time.Hour).UnixMilli()
	maxMs := after.Add(31 * 24 * time.Hour).UnixMilli()
	if *c.ExpiresAt < minMs || *c.ExpiresAt > maxMs {
		t.Errorf("expires_at=%d out of expected range [%d, %d]", *c.ExpiresAt, minMs, maxMs)
	}
}

// TestExpiresZeroRejected verifies that -expires 0 causes a non-zero exit.
func TestExpiresZeroRejected(t *testing.T) {
	_, _, ok := runLicensegen(t, "-expires", "0")
	if ok {
		t.Fatal("expected non-zero exit for -expires 0")
	}
}

// TestExpiresNegativeRejected verifies that a negative -expires value causes
// a non-zero exit.
func TestExpiresNegativeRejected(t *testing.T) {
	// Use the flag=value form to avoid ambiguity with flag parser.
	_, _, ok := runLicensegen(t, "-expires=-5")
	if ok {
		t.Fatal("expected non-zero exit for -expires=-5")
	}
}

// TestPrivkeyWithExpires verifies that combining -privkey, -expires, and -tier
// produces a valid signature, the correct pubkey, the right tier claim, and an
// expires_at close to now+365d.
func TestPrivkeyWithExpires(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	keyFile := filepath.Join(t.TempDir(), "vendor.priv")
	if err := os.WriteFile(keyFile, []byte(hex.EncodeToString(priv)), 0600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	before := time.Now().UTC()
	stdout, stderr, ok := runLicensegen(t, "-privkey", keyFile, "-expires", "365", "-tier", "enterprise")
	after := time.Now().UTC()
	if !ok {
		t.Fatalf("licensegen exited non-zero\nstderr: %s", stderr)
	}

	env := parseEnvLines(t, stdout)

	if got, want := env["PULSE_LICENSE_PUBKEY"], hex.EncodeToString(pub); got != want {
		t.Errorf("PULSE_LICENSE_PUBKEY mismatch\ngot:  %s\nwant: %s", got, want)
	}

	parts := strings.SplitN(env["PULSE_LICENSE_KEY"], ".", 2)
	if len(parts) != 2 {
		t.Fatalf("invalid key format")
	}
	claimsData, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("base64-decode claims: %v", err)
	}
	sig, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("base64-decode sig: %v", err)
	}
	if !ed25519.Verify(pub, claimsData, sig) {
		t.Error("signature verification failed")
	}

	var c struct {
		Tier      string `json:"tier"`
		ExpiresAt *int64 `json:"expires_at"`
	}
	if err := json.Unmarshal(claimsData, &c); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if c.Tier != "enterprise" {
		t.Errorf("tier: got %q, want enterprise", c.Tier)
	}
	if c.ExpiresAt == nil {
		t.Fatal("claims missing expires_at")
	}
	minMs := before.Add(364 * 24 * time.Hour).UnixMilli()
	maxMs := after.Add(366 * 24 * time.Hour).UnixMilli()
	if *c.ExpiresAt < minMs || *c.ExpiresAt > maxMs {
		t.Errorf("expires_at=%d out of expected range [%d, %d]", *c.ExpiresAt, minMs, maxMs)
	}
}

// TestBackcompatNoFlags verifies that running with no arguments still exits
// cleanly and produces exactly two stdout lines (regression guard).
func TestBackcompatNoFlags(t *testing.T) {
	stdout, stderr, ok := runLicensegen(t)
	if !ok {
		t.Fatalf("licensegen exited non-zero\nstderr: %s", stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected exactly 2 stdout lines, got %d:\n%s", len(lines), stdout)
	}
}
