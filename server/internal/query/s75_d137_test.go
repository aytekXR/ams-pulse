// Package query — S73 (D-137) tenant-isolation regression guard for IngestTimeseries.
//
// IngestTimeseries omitted the `AND tenant = ?` filter that its sibling analytics
// queries (AudienceAnalytics/GeoBreakdown/DeviceBreakdown/QoeSummary) all apply, so a
// caller scoping to ?tenant=X received ingest metrics blended across EVERY tenant
// sharing an (app, stream_id) — a cross-tenant data leak (same class as S48/D-110).
// This asserts the tenant value actually reaches the query args (→ the WHERE clause),
// mirroring TestAudienceAnalytics_TenantFilter.
//
// Mutation proof: remove the `if p.Tenant != "" { ... }` block in IngestTimeseries
// → the tenant value never reaches the args → this test goes RED.
package query

import (
	"context"
	"testing"
)

func TestIngestTimeseries_TenantFilter(t *testing.T) {
	const tenant = "acme-tenant-s73"
	conn := newFakeConn().withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	if _, err := svc.IngestTimeseries(context.Background(), IngestTimeseriesParams{Tenant: tenant}); err != nil {
		t.Fatalf("IngestTimeseries: %v", err)
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
		t.Errorf("tenant %q did not reach the query args — IngestTimeseries is not tenant-scoped (cross-tenant leak); captured=%v",
			tenant, conn.capturedArgs)
	}
}

// TestIngestTimeseries_TenantAndScopeFilters: the tenant filter composes with the
// existing app/stream/node filters (all four reach the args).
func TestIngestTimeseries_TenantAndScopeFilters(t *testing.T) {
	conn := newFakeConn().withQuery(newFakeRows(), nil)
	svc := New(nilSnapLive{}, conn, nil)

	if _, err := svc.IngestTimeseries(context.Background(),
		IngestTimeseriesParams{App: "live", StreamID: "s1", NodeID: "n1", Tenant: "t-9"}); err != nil {
		t.Fatalf("IngestTimeseries: %v", err)
	}

	want := map[string]bool{"live": false, "s1": false, "n1": false, "t-9": false}
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
