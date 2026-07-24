package main

// base_url_test.go — unit tests for resolveAlertBaseURL (D-165 fix).
//
// resolveAlertBaseURL is a pure function: no network, no env, no external deps.
// Tests run without a database or cluster.
//
// TDD pin: referencing resolveAlertBaseURL here causes a compile failure until
// serve.go defines the function — analogous to beaconListenerConfig / D-058.

import "testing"

func TestResolveAlertBaseURL(t *testing.T) {
	cases := []struct {
		name         string
		pulseBaseURL string
		listenAddr   string
		want         string
	}{
		{
			name:         "PULSE_BASE_URL unset: falls back to http://+listenAddr",
			pulseBaseURL: "",
			listenAddr:   ":8090",
			want:         "http://:8090",
		},
		{
			name:         "PULSE_BASE_URL http URL: returned as-is",
			pulseBaseURL: "http://pulse.example.com",
			listenAddr:   ":8090",
			want:         "http://pulse.example.com",
		},
		{
			name:         "PULSE_BASE_URL https URL: returned as-is",
			pulseBaseURL: "https://pulse.example.com",
			listenAddr:   ":8090",
			want:         "https://pulse.example.com",
		},
		{
			name:         "PULSE_BASE_URL with path: returned as-is",
			pulseBaseURL: "https://pulse.example.com/pulse",
			listenAddr:   ":8090",
			want:         "https://pulse.example.com/pulse",
		},
		{
			// Trailing slash is stripped by loadEnvConfig before PulseBaseURL is set;
			// resolveAlertBaseURL itself simply returns whatever was stored.
			name:         "trailing-slash already stripped by loadEnvConfig",
			pulseBaseURL: "https://pulse.example.com",
			listenAddr:   ":8090",
			want:         "https://pulse.example.com",
		},
		{
			// Fallback with explicit host:port (prod-behind-proxy without PULSE_BASE_URL).
			name:         "PULSE_BASE_URL unset: non-loopback listenAddr fallback",
			pulseBaseURL: "",
			listenAddr:   "0.0.0.0:8090",
			want:         "http://0.0.0.0:8090",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := resolveAlertBaseURL(tc.pulseBaseURL, tc.listenAddr)
			if got != tc.want {
				t.Errorf("resolveAlertBaseURL(%q, %q) = %q, want %q",
					tc.pulseBaseURL, tc.listenAddr, got, tc.want)
			}
		})
	}
}
