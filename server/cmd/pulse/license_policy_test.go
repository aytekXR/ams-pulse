package main

// D-194 (v0.5.0): every Pulse server runs with the all-features-free policy on —
// Pulse is free, every feature, commercial use included. newLicenseManager is
// the only place serve() builds the license manager, so pinning it pins what
// production runs. Every load outcome is covered: no key, a key that fails to
// verify, and an offline file that cannot be read must all end up fully open,
// never back on Free-tier enforcement.

import (
	"io"
	"log/slog"
	"testing"
)

func TestNewLicenseManager_AllFeaturesFreeOnEveryLoadPath(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range []struct{ name, key, file string }{
		{"no key", "", ""},
		{"key fails verification", "bm90LWEta2V5.bm90LWEtc2ln", ""},
		{"offline file unreadable", "", "/nonexistent/pulse-license.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lic := newLicenseManager(tc.key, tc.file, logger)
			if lic == nil {
				t.Fatal("newLicenseManager returned nil")
			}
			if !lic.AllFeaturesFree() {
				t.Fatal("serve's license manager must run with the all-features-free policy on")
			}
			// The strictest gates under enforcement: Enterprise-only SSO and the node limit.
			if err := lic.CheckSSO(); err != nil {
				t.Errorf("CheckSSO: %v", err)
			}
			if err := lic.CheckNodeLimit(500); err != nil {
				t.Errorf("CheckNodeLimit(500): %v", err)
			}
		})
	}
}
