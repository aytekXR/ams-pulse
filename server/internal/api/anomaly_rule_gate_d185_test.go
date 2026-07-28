package api_test

// anomaly_rule_gate_d185_test.go — D-185: anomaly ALERT RULES are Enterprise-only.
//
// Found while gating the anomaly baseline loop (round 11's M-03 "the class is now
// uniform" reassurance turned out to have a fourth member). Anomaly detection is
// advertised Enterprise-only (PRD §7.11, docs/overview.md tier table,
// docs/marketplace/listing.md), and GET /anomalies enforced that via
// CheckAnomalies — but alert-rule create/update never did, so ANY tier could
// configure rule_type=anomaly and receive Enterprise-only alerts off the same
// baselines.
//
// The two halves are load-bearing on each other: gating only the detector loop
// would have left these rules in place, reading progressively staler baselines
// and quietly ceasing to fire. A silent stop is a worse defect than the leak.

import (
	"net/http"
	"testing"
)

// anomalyRuleBody is a valid anomaly rule (window_s=3600 is required by
// ValidateAnomalyRule, and viewers is a supported anomaly metric).
func anomalyRuleBody(name string) map[string]any {
	// Shape copied from the existing contract test (api_anomaly_contract_test.go)
	// so this test exercises the TIER gate, not the shape validator.
	return map[string]any{
		"name":        name,
		"metric":      "viewer_count",
		"rule_type":   "anomaly",
		"window_s":    3600,
		"sigma":       2.5,
		"min_samples": 5,
		"severity":    "warning",
		"operator":    "gt",
		"threshold":   0,
	}
}

// TestAnomalyRule_CreateBlockedBelowEnterprise is the gate. A Pro tenant must be
// refused with 403 LICENSE_REQUIRED, exactly as GET /anomalies refuses them.
func TestAnomalyRule_CreateBlockedBelowEnterprise(t *testing.T) {
	base, _, token, cleanup := setupProTierServer(t)
	defer cleanup()

	res := doJSON(t, http.DefaultClient, http.MethodPost, base+"/api/v1/alerts/rules", token, anomalyRuleBody("anom-1"))
	if res.status != http.StatusForbidden {
		t.Fatalf("expected 403 for an anomaly rule on Pro tier, got %d: %s", res.status, res.body)
	}
	if code, _ := res.json["code"].(string); code != "LICENSE_REQUIRED" {
		// The body shape is {error:{code,message}} in some handlers; accept either.
		if errObj, ok := res.json["error"].(map[string]any); ok {
			if c, _ := errObj["code"].(string); c == "LICENSE_REQUIRED" {
				return
			}
		}
		t.Fatalf("expected LICENSE_REQUIRED, got body: %s", res.body)
	}
}

// TestThresholdRule_StillAllowedBelowEnterprise is the negative control, and the
// one that matters most: the gate must be scoped to rule_type=anomaly. Alerting
// itself is not Enterprise-only, and a gate that caught ordinary threshold rules
// would break every non-Enterprise tenant's alerting — a far worse regression
// than the leak being closed.
func TestThresholdRule_StillAllowedBelowEnterprise(t *testing.T) {
	base, _, token, cleanup := setupProTierServer(t)
	defer cleanup()

	res := doJSON(t, http.DefaultClient, http.MethodPost, base+"/api/v1/alerts/rules", token, map[string]any{
		"name":      "threshold-still-works",
		"metric":    "stream_offline",
		"operator":  "gt",
		"threshold": 0,
		"window_s":  60,
		"severity":  "warning",
		"scope":     map[string]any{},
	})
	if res.status != http.StatusCreated {
		t.Fatalf("a threshold rule must still be creatable on Pro tier: got %d: %s", res.status, res.body)
	}
}

// TestAnomalyRule_UpdateBlockedBelowEnterprise closes the other half of the
// boundary: create and update are separate handlers, and a gate on one only is
// the shape of defect this whole review chain keeps finding.
func TestAnomalyRule_UpdateBlockedBelowEnterprise(t *testing.T) {
	base, _, token, cleanup := setupProTierServer(t)
	defer cleanup()

	// A Pro tenant CAN create a threshold rule.
	created := doJSON(t, http.DefaultClient, http.MethodPost, base+"/api/v1/alerts/rules", token, map[string]any{
		"name":      "to-be-upgraded",
		"metric":    "stream_offline",
		"operator":  "gt",
		"threshold": 0,
		"window_s":  60,
		"severity":  "warning",
		"scope":     map[string]any{},
	})
	if created.status != http.StatusCreated {
		t.Fatalf("setup: create threshold rule: %d %s", created.status, created.body)
	}
	id, _ := created.json["id"].(string)
	if id == "" {
		t.Fatalf("setup: no id in %s", created.body)
	}

	// Converting it to an anomaly rule must be refused.
	upd := doJSON(t, http.DefaultClient, http.MethodPut, base+"/api/v1/alerts/rules/"+id, token, anomalyRuleBody("now-anomaly"))
	if upd.status != http.StatusForbidden {
		t.Fatalf("expected 403 converting a rule to rule_type=anomaly on Pro tier, got %d: %s", upd.status, upd.body)
	}
}
