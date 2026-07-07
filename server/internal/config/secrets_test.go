// Package config — tests for GetSecret (_FILE convention, Item 5).
package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetSecret covers the full resolution matrix for the _FILE convention.
func TestGetSecret(t *testing.T) {
	cases := []struct {
		name        string
		envVal      string // set if non-empty
		fileContent string // written to tmp file if fileSet=true
		fileSet     bool   // set ${name}_FILE env var pointing at the tmp file
		missingFile bool   // set ${name}_FILE to a non-existent path
		wantVal     string
		wantErr     bool
	}{
		{
			name:    "env-only: plain env var returned",
			envVal:  "from-env",
			wantVal: "from-env",
		},
		{
			name:        "file-only: file content returned, newline trimmed",
			fileContent: "from-file\n",
			fileSet:     true,
			wantVal:     "from-file",
		},
		{
			name:        "file-only: CRLF newline trimmed",
			fileContent: "crlf-secret\r\n",
			fileSet:     true,
			wantVal:     "crlf-secret",
		},
		{
			// FILE wins — the operator explicitly chose file delivery.
			// The plain env var (populated e.g. by the hardened overlay's :? guard
			// with an empty override) is ignored.
			name:        "both set: FILE wins over plain env",
			envVal:      "from-env",
			fileContent: "from-file\n",
			fileSet:     true,
			wantVal:     "from-file",
		},
		{
			// An empty file → empty secret → validate() catches it for SecretKey.
			name:        "empty file: returns empty string (not an error)",
			fileContent: "",
			fileSet:     true,
			wantVal:     "",
		},
		{
			// Missing file while _FILE is set must be a hard error (fail-closed).
			name:        "missing file: hard error",
			missingFile: true,
			wantErr:     true,
		},
		{
			name:    "neither set: empty string returned",
			wantVal: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const varName = "PULSE_TEST_SECRET_GETX"

			// Ensure both env vars start clean.
			t.Setenv(varName, "")
			t.Setenv(varName+"_FILE", "")

			if tc.envVal != "" {
				t.Setenv(varName, tc.envVal)
			}

			if tc.missingFile {
				t.Setenv(varName+"_FILE", "/nonexistent/path/does-not-exist.txt")
			} else if tc.fileSet {
				dir := t.TempDir()
				p := filepath.Join(dir, "secret.txt")
				if err := os.WriteFile(p, []byte(tc.fileContent), 0600); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				t.Setenv(varName+"_FILE", p)
			}

			got, err := GetSecret(varName)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("GetSecret() returned nil error; want error for missing file")
				}
				return
			}
			if err != nil {
				t.Fatalf("GetSecret() returned unexpected error: %v", err)
			}
			if got != tc.wantVal {
				t.Errorf("GetSecret() = %q; want %q", got, tc.wantVal)
			}
		})
	}
}
