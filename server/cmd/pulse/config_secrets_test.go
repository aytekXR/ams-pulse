// cmd/pulse — tests for _FILE secret resolution in loadEnvConfig (Item 5).
// These are table-driven behavioural tests: RED first (before the helper was
// wired), now GREEN after wiring config.GetSecret for each secret var.
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadEnvConfig_SecretFile exercises the _FILE convention for each
// secret-bearing var in the cmd layer: PULSE_AMS_LOGIN_PASSWORD,
// PULSE_WEBHOOK_SECRET, and PULSE_METRICS_TOKEN.
func TestLoadEnvConfig_SecretFile(t *testing.T) {
	type secretVarCase struct {
		varName   string
		fieldName string // human label for error messages
		getField  func(EnvConfig) string
	}

	secretVars := []secretVarCase{
		{
			varName:   "PULSE_AMS_AUTH_TOKEN",
			fieldName: "AMSAuthToken",
			getField:  func(c EnvConfig) string { return c.AMSAuthToken },
		},
		{
			varName:   "PULSE_AMS_LOGIN_PASSWORD",
			fieldName: "AMSLoginPassword",
			getField:  func(c EnvConfig) string { return c.AMSLoginPassword },
		},
		{
			varName:   "PULSE_WEBHOOK_SECRET",
			fieldName: "WebhookSharedSecret",
			getField:  func(c EnvConfig) string { return c.WebhookSharedSecret },
		},
		{
			varName:   "PULSE_METRICS_TOKEN",
			fieldName: "MetricsToken",
			getField:  func(c EnvConfig) string { return c.MetricsToken },
		},
	}

	for _, sv := range secretVars {
		sv := sv
		t.Run(sv.fieldName, func(t *testing.T) {
			subtests := []struct {
				name        string
				envVal      string
				fileContent string
				fileSet     bool
				missingFile bool
				wantVal     string
				wantErr     bool
			}{
				{
					name:    "env-only",
					envVal:  "secret-from-env",
					wantVal: "secret-from-env",
				},
				{
					name:        "file-only newline trimmed",
					fileContent: "secret-from-file\n",
					fileSet:     true,
					wantVal:     "secret-from-file",
				},
				{
					name:        "file-only CRLF trimmed",
					fileContent: "crlf-secret\r\n",
					fileSet:     true,
					wantVal:     "crlf-secret",
				},
				{
					// FILE wins when both are set.
					name:        "both set FILE wins",
					envVal:      "env-value",
					fileContent: "file-value\n",
					fileSet:     true,
					wantVal:     "file-value",
				},
				{
					name:        "missing file is hard error",
					missingFile: true,
					wantErr:     true,
				},
			}

			for _, tc := range subtests {
				tc := tc
				t.Run(tc.name, func(t *testing.T) {
					// Clean env for the vars under test.
					t.Setenv(sv.varName, "")
					t.Setenv(sv.varName+"_FILE", "")

					if tc.envVal != "" {
						t.Setenv(sv.varName, tc.envVal)
					}
					if tc.missingFile {
						t.Setenv(sv.varName+"_FILE", "/nonexistent/secret.txt")
					} else if tc.fileSet {
						dir := t.TempDir()
						p := filepath.Join(dir, "secret.txt")
						if err := os.WriteFile(p, []byte(tc.fileContent), 0600); err != nil {
							t.Fatalf("WriteFile: %v", err)
						}
						t.Setenv(sv.varName+"_FILE", p)
					}

					cfg, err := loadEnvConfig()
					if tc.wantErr {
						if err == nil {
							t.Fatalf("loadEnvConfig() returned nil error; want error for missing _FILE")
						}
						return
					}
					if err != nil {
						t.Fatalf("loadEnvConfig() unexpected error: %v", err)
					}
					if got := sv.getField(cfg); got != tc.wantVal {
						t.Errorf("%s = %q; want %q", sv.fieldName, got, tc.wantVal)
					}
				})
			}
		})
	}
}
