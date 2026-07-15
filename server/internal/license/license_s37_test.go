// Package license_test — S37 (D-099) tests for CheckSSO and CheckWhiteLabel.
//
// SSO/OIDC login is an Enterprise-only feature (PRD §7 pricing table); white-label
// report headers require the white_label entitlement (Enterprise). These checks
// back the HTTP gates added in S37 for the OIDC login/callback/status endpoints
// and the report-schedule white-label header.
package license_test

import (
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/license"
)

// ─── CheckSSO ─────────────────────────────────────────────────────────────────

func TestCheckSSO_BlockedOnFree(t *testing.T) {
	mgr, _ := license.New("", "") // free
	if err := mgr.CheckSSO(); err == nil {
		t.Error("free tier: CheckSSO must return error (Enterprise required)")
	}
}

func TestCheckSSO_BlockedOnPro(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{"tier": "pro", "data_api": true})
	if err := mgr.CheckSSO(); err == nil {
		t.Error("pro tier: CheckSSO must return error (Enterprise required)")
	}
}

func TestCheckSSO_BlockedOnBusiness(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{"tier": "business", "data_api": true})
	if err := mgr.CheckSSO(); err == nil {
		t.Error("business tier: CheckSSO must return error (Enterprise required)")
	}
}

func TestCheckSSO_AllowedOnEnterprise(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":        "enterprise",
		"data_api":    true,
		"white_label": true,
	})
	if err := mgr.CheckSSO(); err != nil {
		t.Errorf("enterprise tier: CheckSSO want nil got %v", err)
	}
}

// ─── CheckWhiteLabel ──────────────────────────────────────────────────────────

func TestCheckWhiteLabel_BlockedOnFree(t *testing.T) {
	mgr, _ := license.New("", "") // free (white_label:false)
	if err := mgr.CheckWhiteLabel(); err == nil {
		t.Error("free tier: CheckWhiteLabel must return error")
	}
}

func TestCheckWhiteLabel_BlockedOnBusiness(t *testing.T) {
	// Business does not license white-label branding (white_label:false).
	mgr := newMgr(t, map[string]interface{}{
		"tier":        "business",
		"data_api":    true,
		"white_label": false,
	})
	if err := mgr.CheckWhiteLabel(); err == nil {
		t.Error("business tier without white_label: CheckWhiteLabel must return error")
	}
}

func TestCheckWhiteLabel_AllowedWhenEntitled(t *testing.T) {
	mgr := newMgr(t, map[string]interface{}{
		"tier":        "enterprise",
		"data_api":    true,
		"white_label": true,
	})
	if err := mgr.CheckWhiteLabel(); err != nil {
		t.Errorf("enterprise tier with white_label: CheckWhiteLabel want nil got %v", err)
	}
}
