// Package config loads and validates Pulse server configuration.
//
// Sources, in precedence order: flags > environment (PULSE_*) > YAML file
// (deploy/config/pulse.example.yaml documents the full surface). Pre-tuned
// defaults matter commercially: the Free/Pro tiers must run on a 2-vCPU
// sidecar with zero tuning (PRD §7.13).
package config

// Config is the root configuration shared by all run modes.
type Config struct {
	// TODO(BE-01): HTTP listen addrs (UI/API, beacon ingest), AMS source defs,
	// ClickHouse + meta-store DSNs, retention windows, license key path,
	// sampling rate, log level.
}

// Load resolves configuration from flags/env/file.
func Load(args []string) (*Config, error) {
	// TODO(BE-01)
	return nil, errNotImplemented
}
