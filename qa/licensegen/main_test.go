package main

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
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
