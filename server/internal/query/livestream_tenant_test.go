// Package query_test — F6 Phase 1: server-side tenant resolution + the ?tenant=
// filter on the live endpoints (closes BUG-009, whose tenant portion was a
// known-violation: the param was accepted and silently ignored).
package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
)

// fakeTenantResolver maps stream_id → tenant deterministically (no meta store).
type fakeTenantResolver struct{ m map[string]string }

func (f fakeTenantResolver) ResolveTenant(streamID string) string { return f.m[streamID] }

func tenantSnap() *domain.LiveSnapshot {
	mk := func(id string, viewers int) *domain.LiveStream {
		return &domain.LiveStream{
			StreamID: id, App: "live", NodeID: "n1", Active: true,
			ViewerCount: viewers, Health: domain.StreamHealthGood, StartedAt: time.Now(),
		}
	}
	return &domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"acme-1":   mk("acme-1", 3),
			"acme-2":   mk("acme-2", 5),
			"globex-1": mk("globex-1", 7),
			"orphan-1": mk("orphan-1", 9), // resolves to "" (unassigned)
		},
		Nodes:     map[string]*domain.LiveNodeStats{},
		UpdatedAt: time.Now(),
	}
}

func newTenantSvc() *query.Service {
	live := &mockLiveProvider{snap: tenantSnap()}
	lic, _ := license.New("", "")
	svc := query.New(live, nil, lic)
	svc.SetTenantResolver(fakeTenantResolver{m: map[string]string{
		"acme-1": "acme", "acme-2": "acme", "globex-1": "globex",
		// orphan-1 intentionally absent → resolves to "" (unassigned)
	}})
	return svc
}

func TestLiveStreams_TenantFilterAndPopulation(t *testing.T) {
	svc := newTenantSvc()

	// No filter → all 4, each carrying its resolved tenant.
	all, err := svc.LiveStreams(context.Background(), "", "", "", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Items) != 4 {
		t.Fatalf("no filter: got %d items, want 4", len(all.Items))
	}
	byID := map[string]string{}
	for _, it := range all.Items {
		byID[it.StreamID] = it.Tenant
	}
	if byID["acme-1"] != "acme" || byID["globex-1"] != "globex" || byID["orphan-1"] != "" {
		t.Fatalf("resolved tenants on items wrong: %v", byID)
	}

	// ?tenant=acme → exactly the two acme streams, each tagged acme.
	acme, err := svc.LiveStreams(context.Background(), "", "", "acme", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(acme.Items) != 2 {
		t.Fatalf("tenant=acme: got %d items, want 2", len(acme.Items))
	}
	for _, it := range acme.Items {
		if it.Tenant != "acme" {
			t.Fatalf("tenant=acme leaked a %q stream (%s)", it.Tenant, it.StreamID)
		}
	}

	// ?tenant=nomatch → empty (no cross-tenant leakage to a bogus tenant).
	none, err := svc.LiveStreams(context.Background(), "", "", "nomatch", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(none.Items) != 0 {
		t.Fatalf("tenant=nomatch: got %d items, want 0", len(none.Items))
	}
}

func TestLiveOverview_TenantFilter(t *testing.T) {
	svc := newTenantSvc()

	all, err := svc.LiveOverview(context.Background(), "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if all.TotalPublishers != 4 || all.TotalViewers != 24 { // 3+5+7+9
		t.Fatalf("unfiltered: pubs=%d viewers=%d, want 4/24", all.TotalPublishers, all.TotalViewers)
	}

	acme, err := svc.LiveOverview(context.Background(), "", "", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if acme.TotalPublishers != 2 || acme.TotalViewers != 8 { // 3+5
		t.Fatalf("tenant=acme: pubs=%d viewers=%d, want 2/8", acme.TotalPublishers, acme.TotalViewers)
	}
	if len(acme.Apps) != 1 || acme.Apps[0].Viewers != 8 {
		t.Fatalf("tenant=acme app aggregation wrong: %+v", acme.Apps)
	}
}

// Contract for the single-tenant default and a misconfigured/absent resolver:
// an EXPLICIT ?tenant=X fails closed (empty — no stream can belong to a tenant
// when nothing resolves), while an unfiltered request returns everything. This
// is the safe (never-leak) direction and matches the analytics tenant filters.
func TestLiveStreams_NoResolver_TenantFilterFailsClosed(t *testing.T) {
	live := &mockLiveProvider{snap: tenantSnap()}
	lic, _ := license.New("", "")
	svc := query.New(live, nil, lic) // no SetTenantResolver → resolveTenant == ""

	// Explicit tenant filter → fail closed (empty), never a cross-tenant leak.
	filtered, err := svc.LiveStreams(context.Background(), "", "", "acme", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Items) != 0 {
		t.Fatalf("no resolver + tenant=acme: got %d, want 0 (fail closed)", len(filtered.Items))
	}

	// No tenant filter → all streams, tenant field empty (feature not wired).
	unfiltered, err := svc.LiveStreams(context.Background(), "", "", "", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(unfiltered.Items) != 4 {
		t.Fatalf("no resolver, no filter: got %d, want 4", len(unfiltered.Items))
	}
	for _, it := range unfiltered.Items {
		if it.Tenant != "" {
			t.Fatalf("no resolver: item %s got tenant %q, want empty", it.StreamID, it.Tenant)
		}
	}
}
