// Package config — tests for PULSE_* environment variable parsing (C2).
package config

import (
	"os"
	"testing"
)

// TestLoad_AllowedWSOrigins verifies that PULSE_ALLOWED_WS_ORIGINS is parsed
// correctly: comma-separated values trimmed of spaces, empties dropped (C2).
func TestLoad_AllowedWSOrigins(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		want    []string
		wantNil bool
	}{
		{
			name:    "unset env → nil/empty",
			envVal:  "",
			wantNil: true,
		},
		{
			name:   "two origins, spaces trimmed",
			envVal: "https://a.com, https://b.com",
			want:   []string{"https://a.com", "https://b.com"},
		},
		{
			name:   "single origin",
			envVal: "https://example.com",
			want:   []string{"https://example.com"},
		},
		{
			name:   "trailing comma (empty entry dropped)",
			envVal: "https://a.com,",
			want:   []string{"https://a.com"},
		},
		{
			name:   "only commas/spaces",
			envVal: " , , ",
			want:   nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use in-memory DSN to exempt from PULSE_SECRET_KEY validation in tests.
			t.Setenv("PULSE_META_DSN", ":memory:")
			if tc.envVal != "" {
				t.Setenv("PULSE_ALLOWED_WS_ORIGINS", tc.envVal)
			} else {
				os.Unsetenv("PULSE_ALLOWED_WS_ORIGINS")
			}

			cfg, err := Load(nil)
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}

			if tc.wantNil {
				if len(cfg.AllowedWSOrigins) != 0 {
					t.Errorf("AllowedWSOrigins = %v; want nil/empty for unset env", cfg.AllowedWSOrigins)
				}
				return
			}

			if len(cfg.AllowedWSOrigins) != len(tc.want) {
				t.Fatalf("AllowedWSOrigins = %v (len %d); want %v (len %d)",
					cfg.AllowedWSOrigins, len(cfg.AllowedWSOrigins),
					tc.want, len(tc.want))
			}
			for i, got := range cfg.AllowedWSOrigins {
				if got != tc.want[i] {
					t.Errorf("AllowedWSOrigins[%d] = %q; want %q", i, got, tc.want[i])
				}
			}
		})
	}
}
