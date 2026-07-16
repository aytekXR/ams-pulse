// Package query — S48 (D-110) tenant-isolation regression guard.
//
// AudienceAnalytics omitted the `AND tenant = ?` filter that its sibling analytics
// queries (GeoBreakdown/DeviceBreakdown/QoeSummary) all apply, so a caller scoping
// to ?tenant=X received EVERY tenant's audience rollups — a cross-tenant data leak.
// The rollup_audience tables carry a tenant column. This asserts the tenant value
// actually reaches the query args (→ the WHERE clause), mirroring the S37 retention
// tests' capturedArgs technique.
//
// Mutation proof: remove the `if p.Tenant != "" { ... }` block in AudienceAnalytics
// → the tenant value never reaches the args → this test goes RED.
package query

import (
	"context"
	"testing"
)

func TestAudienceAnalytics_TenantFilter(t *testing.T) {
	const tenant = "acme-tenant-s48"
	conn := newFakeConn().withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	if _, err := svc.AudienceAnalytics(context.Background(), AudienceParams{Tenant: tenant}); err != nil {
		t.Fatalf("AudienceAnalytics: %v", err)
	}

	found := false
	for _, call := range conn.capturedArgs {
		for _, a := range call {
			if s, ok := a.(string); ok && s == tenant {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("tenant %q did not reach the query args — AudienceAnalytics is not tenant-scoped (cross-tenant leak); captured=%v",
			tenant, conn.capturedArgs)
	}
}

// TestAudienceAnalytics_TenantAndAppAndStream: the tenant filter composes with the
// existing app/stream filters (all three reach the args).
func TestAudienceAnalytics_TenantAndAppAndStream(t *testing.T) {
	conn := newFakeConn().withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	if _, err := svc.AudienceAnalytics(context.Background(),
		AudienceParams{App: "live", Stream: "s1", Tenant: "t-9"}); err != nil {
		t.Fatalf("AudienceAnalytics: %v", err)
	}

	want := map[string]bool{"live": false, "s1": false, "t-9": false}
	for _, call := range conn.capturedArgs {
		for _, a := range call {
			if s, ok := a.(string); ok {
				if _, tracked := want[s]; tracked {
					want[s] = true
				}
			}
		}
	}
	for v, seen := range want {
		if !seen {
			t.Errorf("filter value %q did not reach the query args; captured=%v", v, conn.capturedArgs)
		}
	}
}
