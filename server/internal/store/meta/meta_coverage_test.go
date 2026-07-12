package meta_test

// meta_coverage_test.go — additional unit tests to raise coverage of
// internal/store/meta beyond the 6 existing round-trip tests.
//
// Tests added here:
//   - TestMetaStore_Probes_RoundTrip         — probe CRUD + scanProbe
//   - TestMetaStore_ProbeConfigSource         — NewProbeConfigSource, ListEnabled, RecordResult
//   - TestMetaStore_AnomalyBaselines_RoundTrip — anomaly CRUD (insert + upsert-update + delete)
//   - TestMetaStore_Tenants_RoundTrip          — tenant CRUD + GetTenantByName
//   - TestMetaStore_Token_Revoke               — ListTokens (kind filter), TouchToken, DeleteToken
//   - TestMetaStore_ConcurrentInsert_Race      — goroutines + -race for data-race detection

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/anomaly"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Probes ──────────────────────────────────────────────────────────────────

// TestMetaStore_Probes_RoundTrip covers CreateProbe / GetProbe / ListProbes /
// UpdateProbe / DeleteProbe and the scanProbe helper (via Get).
func TestMetaStore_Probes_RoundTrip(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	// Create — verify defaults are NOT applied when caller supplies values.
	p := meta.ProbeRow{
		Name:      "hls-live-probe",
		URL:       "https://origin.example.com/live/stream.m3u8",
		Protocol:  "hls",
		IntervalS: 30,
		TimeoutS:  5,
		Enabled:   true,
	}
	created, err := s.CreateProbe(ctx, p)
	if err != nil {
		t.Fatalf("CreateProbe: %v", err)
	}
	if created.ID == "" {
		t.Fatal("CreateProbe: expected non-empty ID")
	}
	if created.CreatedAt == 0 {
		t.Fatal("CreateProbe: expected non-zero created_at")
	}
	if created.IntervalS != 30 {
		t.Errorf("CreateProbe: IntervalS got %d, want 30", created.IntervalS)
	}
	if created.TimeoutS != 5 {
		t.Errorf("CreateProbe: TimeoutS got %d, want 5", created.TimeoutS)
	}

	// GetProbe — verify all scalar fields round-trip correctly.
	got, err := s.GetProbe(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetProbe: %v", err)
	}
	if got == nil {
		t.Fatal("GetProbe: expected probe, got nil")
	}
	if got.URL != p.URL {
		t.Errorf("GetProbe: URL got %q, want %q", got.URL, p.URL)
	}
	if got.Protocol != "hls" {
		t.Errorf("GetProbe: Protocol got %q, want hls", got.Protocol)
	}
	if !got.Enabled {
		t.Error("GetProbe: expected Enabled=true")
	}
	if got.LastResultID.Valid {
		t.Error("GetProbe: expected LastResultID to be NULL before any result")
	}

	// ListProbes — should contain exactly the one we created.
	probes, err := s.ListProbes(ctx, 0, "")
	if err != nil {
		t.Fatalf("ListProbes: %v", err)
	}
	if len(probes) != 1 {
		t.Fatalf("ListProbes: expected 1, got %d", len(probes))
	}

	// UpdateProbe — change interval and toggle enabled.
	got.IntervalS = 120
	got.Enabled = false
	if err := s.UpdateProbe(ctx, *got); err != nil {
		t.Fatalf("UpdateProbe: %v", err)
	}
	updated, err := s.GetProbe(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetProbe after UpdateProbe: %v", err)
	}
	if updated.IntervalS != 120 {
		t.Errorf("UpdateProbe: IntervalS got %d, want 120", updated.IntervalS)
	}
	if updated.Enabled {
		t.Error("UpdateProbe: expected Enabled=false after update")
	}

	// DeleteProbe — GetProbe must return nil afterwards.
	if err := s.DeleteProbe(ctx, created.ID); err != nil {
		t.Fatalf("DeleteProbe: %v", err)
	}
	gone, err := s.GetProbe(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetProbe after DeleteProbe: %v", err)
	}
	if gone != nil {
		t.Errorf("DeleteProbe: expected nil, got %+v", gone)
	}
}

// TestMetaStore_ProbeConfigSource covers NewProbeConfigSource, ListEnabled
// (enabled-only filtering) and RecordResult (last_* denorm write-back).
func TestMetaStore_ProbeConfigSource(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	// Two probes: one enabled, one disabled.
	enabledRow, err := s.CreateProbe(ctx, meta.ProbeRow{
		Name:     "probe-on",
		URL:      "https://cdn.example.com/live.m3u8",
		Protocol: "hls",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("CreateProbe (enabled): %v", err)
	}
	_, err = s.CreateProbe(ctx, meta.ProbeRow{
		Name:     "probe-off",
		URL:      "https://cdn.example.com/other.m3u8",
		Protocol: "hls",
		Enabled:  false,
	})
	if err != nil {
		t.Fatalf("CreateProbe (disabled): %v", err)
	}

	src := meta.NewProbeConfigSource(s)

	// ListEnabled must return only the enabled probe — not the disabled one.
	configs, err := src.ListEnabled(ctx)
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("ListEnabled: expected 1 enabled probe, got %d", len(configs))
	}
	if configs[0].ID != enabledRow.ID {
		t.Errorf("ListEnabled: ID got %q, want %q", configs[0].ID, enabledRow.ID)
	}
	if !configs[0].Enabled {
		t.Error("ListEnabled: returned probe has Enabled=false")
	}
	if configs[0].URL == "" {
		t.Error("ListEnabled: returned probe has empty URL")
	}

	// RecordResult must write last_result_id, last_success, last_run_at.
	const resultID = "result-uuid-aabb1122"
	err = src.RecordResult(ctx, domain.ProbeResult{
		ID:      resultID,
		ProbeID: enabledRow.ID,
		TS:      time.Now(),
		Success: true,
	})
	if err != nil {
		t.Fatalf("RecordResult: %v", err)
	}

	afterResult, err := s.GetProbe(ctx, enabledRow.ID)
	if err != nil {
		t.Fatalf("GetProbe after RecordResult: %v", err)
	}
	if !afterResult.LastResultID.Valid {
		t.Error("RecordResult: LastResultID not set")
	}
	if afterResult.LastResultID.String != resultID {
		t.Errorf("RecordResult: LastResultID got %q, want %q",
			afterResult.LastResultID.String, resultID)
	}
	if !afterResult.LastSuccess.Valid {
		t.Error("RecordResult: LastSuccess not set")
	}
	if afterResult.LastSuccess.Int64 != 1 {
		t.Errorf("RecordResult: LastSuccess got %d, want 1 (success=true)",
			afterResult.LastSuccess.Int64)
	}
	if !afterResult.LastRunAt.Valid {
		t.Error("RecordResult: LastRunAt not set")
	}

	// RecordResult with Success=false must write last_success=0.
	err = src.RecordResult(ctx, domain.ProbeResult{
		ID:      "result-fail-001",
		ProbeID: enabledRow.ID,
		TS:      time.Now(),
		Success: false,
	})
	if err != nil {
		t.Fatalf("RecordResult (failure): %v", err)
	}
	afterFail, err := s.GetProbe(ctx, enabledRow.ID)
	if err != nil {
		t.Fatalf("GetProbe after failed RecordResult: %v", err)
	}
	if afterFail.LastSuccess.Int64 != 0 {
		t.Errorf("RecordResult failure: LastSuccess got %d, want 0",
			afterFail.LastSuccess.Int64)
	}
}

// ─── Anomaly baselines ───────────────────────────────────────────────────────

// TestMetaStore_AnomalyBaselines_RoundTrip covers UpsertAnomalyBaseline (insert
// and update paths via ON CONFLICT), GetAnomalyBaseline, ListAnomalyBaselines,
// DeleteAnomalyBaseline.
func TestMetaStore_AnomalyBaselines_RoundTrip(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	const (
		metric  = "viewers"
		scope   = `{"stream_id":"stream-abc"}`
		windowS = 3600
	)

	row := anomaly.AnomalyBaselineRow{
		Metric:      metric,
		Scope:       scope,
		WindowS:     windowS,
		Mean:        42.5,
		Stddev:      3.2,
		SampleCount: 100,
		LastUpdated: 1_700_000_000_000,
	}

	// First upsert inserts a new row.
	if err := s.UpsertAnomalyBaseline(ctx, row); err != nil {
		t.Fatalf("UpsertAnomalyBaseline (insert): %v", err)
	}

	got, err := s.GetAnomalyBaseline(ctx, metric, scope, windowS)
	if err != nil {
		t.Fatalf("GetAnomalyBaseline: %v", err)
	}
	if got == nil {
		t.Fatal("GetAnomalyBaseline: expected row, got nil")
	}
	if got.ID == "" {
		t.Error("GetAnomalyBaseline: expected non-empty ID")
	}
	if got.Mean != 42.5 {
		t.Errorf("GetAnomalyBaseline: Mean got %v, want 42.5", got.Mean)
	}
	if got.SampleCount != 100 {
		t.Errorf("GetAnomalyBaseline: SampleCount got %d, want 100", got.SampleCount)
	}
	if got.Metric != metric {
		t.Errorf("GetAnomalyBaseline: Metric got %q, want %q", got.Metric, metric)
	}

	// Second upsert on the same (metric, scope, window_s) key must UPDATE.
	row.Mean = 55.0
	row.SampleCount = 250
	row.LastUpdated = 2_000_000_000_000
	if err := s.UpsertAnomalyBaseline(ctx, row); err != nil {
		t.Fatalf("UpsertAnomalyBaseline (update): %v", err)
	}

	got2, err := s.GetAnomalyBaseline(ctx, metric, scope, windowS)
	if err != nil {
		t.Fatalf("GetAnomalyBaseline after update: %v", err)
	}
	if got2.Mean != 55.0 {
		t.Errorf("Upsert update: Mean got %v, want 55.0", got2.Mean)
	}
	if got2.SampleCount != 250 {
		t.Errorf("Upsert update: SampleCount got %d, want 250", got2.SampleCount)
	}
	if got2.LastUpdated != 2_000_000_000_000 {
		t.Errorf("Upsert update: LastUpdated got %d, want 2000000000000", got2.LastUpdated)
	}

	// ListAnomalyBaselines must surface the single row we have.
	all, err := s.ListAnomalyBaselines(ctx)
	if err != nil {
		t.Fatalf("ListAnomalyBaselines: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("ListAnomalyBaselines: expected 1, got %d", len(all))
	}

	// GetAnomalyBaseline for an absent key must return nil, nil.
	absent, err := s.GetAnomalyBaseline(ctx, "no-such-metric", scope, windowS)
	if err != nil {
		t.Fatalf("GetAnomalyBaseline (absent): %v", err)
	}
	if absent != nil {
		t.Errorf("GetAnomalyBaseline (absent): expected nil, got %+v", absent)
	}

	// DeleteAnomalyBaseline removes by primary key ID.
	if err := s.DeleteAnomalyBaseline(ctx, got2.ID); err != nil {
		t.Fatalf("DeleteAnomalyBaseline: %v", err)
	}
	gone, err := s.GetAnomalyBaseline(ctx, metric, scope, windowS)
	if err != nil {
		t.Fatalf("GetAnomalyBaseline after delete: %v", err)
	}
	if gone != nil {
		t.Errorf("DeleteAnomalyBaseline: expected nil, got %+v", gone)
	}
}

// ─── Tenants ─────────────────────────────────────────────────────────────────

// TestMetaStore_Tenants_RoundTrip covers CreateTenant / GetTenant /
// GetTenantByName / ListTenants / UpdateTenant / DeleteTenant.
func TestMetaStore_Tenants_RoundTrip(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	t1, err := s.CreateTenant(ctx, meta.TenantRow{
		Name:          "acme",
		StreamPattern: "acme-*",
		MetaTagKey:    "org",
		MetaTagValue:  "acme-corp",
	})
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	if t1.ID == "" {
		t.Fatal("CreateTenant: expected non-empty ID")
	}
	if t1.CreatedAt == 0 {
		t.Fatal("CreateTenant: expected non-zero created_at")
	}

	// GetTenant by ID.
	byID, err := s.GetTenant(ctx, t1.ID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if byID == nil {
		t.Fatal("GetTenant: expected tenant, got nil")
	}
	if byID.Name != "acme" {
		t.Errorf("GetTenant: Name got %q, want acme", byID.Name)
	}
	if byID.StreamPattern != "acme-*" {
		t.Errorf("GetTenant: StreamPattern got %q, want acme-*", byID.StreamPattern)
	}
	if byID.MetaTagKey != "org" {
		t.Errorf("GetTenant: MetaTagKey got %q, want org", byID.MetaTagKey)
	}

	// GetTenantByName — found.
	byName, err := s.GetTenantByName(ctx, "acme")
	if err != nil {
		t.Fatalf("GetTenantByName: %v", err)
	}
	if byName == nil {
		t.Fatal("GetTenantByName: expected tenant, got nil")
	}
	if byName.ID != t1.ID {
		t.Errorf("GetTenantByName: ID got %q, want %q", byName.ID, t1.ID)
	}

	// GetTenantByName — not found must return nil, nil.
	missing, err := s.GetTenantByName(ctx, "no-such-tenant")
	if err != nil {
		t.Fatalf("GetTenantByName (missing): %v", err)
	}
	if missing != nil {
		t.Errorf("GetTenantByName (missing): expected nil, got %+v", missing)
	}

	// ListTenants must return the one tenant.
	all, err := s.ListTenants(ctx, 0, "")
	if err != nil {
		t.Fatalf("ListTenants: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ListTenants: expected 1, got %d", len(all))
	}

	// UpdateTenant — change name and stream pattern.
	byID.Name = "acme-renamed"
	byID.StreamPattern = "acme-corp-*"
	if err := s.UpdateTenant(ctx, *byID); err != nil {
		t.Fatalf("UpdateTenant: %v", err)
	}
	updated, err := s.GetTenant(ctx, t1.ID)
	if err != nil {
		t.Fatalf("GetTenant after update: %v", err)
	}
	if updated.Name != "acme-renamed" {
		t.Errorf("UpdateTenant: Name got %q, want acme-renamed", updated.Name)
	}
	if updated.StreamPattern != "acme-corp-*" {
		t.Errorf("UpdateTenant: StreamPattern got %q, want acme-corp-*", updated.StreamPattern)
	}

	// DeleteTenant — GetTenant must return nil afterwards.
	if err := s.DeleteTenant(ctx, t1.ID); err != nil {
		t.Fatalf("DeleteTenant: %v", err)
	}
	gone, err := s.GetTenant(ctx, t1.ID)
	if err != nil {
		t.Fatalf("GetTenant after delete: %v", err)
	}
	if gone != nil {
		t.Errorf("DeleteTenant: expected nil, got %+v", gone)
	}
}

// ─── Token revoke ─────────────────────────────────────────────────────────────

// TestMetaStore_Token_Revoke covers the revoke path (DeleteToken) plus
// ListTokens with kind filtering, and TouchToken / LastUsedAt.
func TestMetaStore_Token_Revoke(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	// Create one api token and one ingest token.
	apiTok := meta.APIToken{
		Kind:      "api",
		Name:      "admin-key",
		TokenHash: "sha256:api-revoke-test-001",
		Scopes:    []string{"read", "write"},
		CreatedAt: 1_000,
	}
	ingestTok := meta.APIToken{
		Kind:      "ingest",
		Name:      "ingest-key",
		TokenHash: "sha256:ingest-revoke-test-001",
		Scopes:    []string{"ingest"},
		CreatedAt: 2_000,
	}
	if err := s.CreateToken(ctx, apiTok); err != nil {
		t.Fatalf("CreateToken (api): %v", err)
	}
	if err := s.CreateToken(ctx, ingestTok); err != nil {
		t.Fatalf("CreateToken (ingest): %v", err)
	}

	// ListTokens filtered by kind must return only that kind.
	apiList, err := s.ListTokens(ctx, "api", 0, "")
	if err != nil {
		t.Fatalf("ListTokens(api): %v", err)
	}
	if len(apiList) != 1 {
		t.Fatalf("ListTokens(api): expected 1, got %d", len(apiList))
	}
	if apiList[0].Kind != "api" {
		t.Errorf("ListTokens(api): Kind got %q, want api", apiList[0].Kind)
	}

	ingestList, err := s.ListTokens(ctx, "ingest", 0, "")
	if err != nil {
		t.Fatalf("ListTokens(ingest): %v", err)
	}
	if len(ingestList) != 1 {
		t.Fatalf("ListTokens(ingest): expected 1, got %d", len(ingestList))
	}

	// ListTokens with empty kind must return both.
	allList, err := s.ListTokens(ctx, "", 0, "")
	if err != nil {
		t.Fatalf("ListTokens(all): %v", err)
	}
	if len(allList) != 2 {
		t.Fatalf("ListTokens(all): expected 2, got %d", len(allList))
	}

	// TouchToken must set last_used_at (it was NULL before).
	apiID := apiList[0].ID
	s.TouchToken(ctx, apiID)

	touched, err := s.GetTokenByHash(ctx, "sha256:api-revoke-test-001")
	if err != nil {
		t.Fatalf("GetTokenByHash after TouchToken: %v", err)
	}
	if touched == nil {
		t.Fatal("GetTokenByHash after TouchToken: expected token, got nil")
	}
	if touched.LastUsedAt == nil {
		t.Error("TouchToken: LastUsedAt still nil after touch")
	}

	// Revoke the api token.
	if err := s.DeleteToken(ctx, apiID); err != nil {
		t.Fatalf("DeleteToken (revoke): %v", err)
	}

	// GetTokenByHash must return nil for the deleted hash.
	revoked, err := s.GetTokenByHash(ctx, "sha256:api-revoke-test-001")
	if err != nil {
		t.Fatalf("GetTokenByHash after revoke: %v", err)
	}
	if revoked != nil {
		t.Errorf("DeleteToken: GetTokenByHash expected nil, got %+v", revoked)
	}

	// Only the ingest token should remain.
	n, err := s.CountTokens(ctx)
	if err != nil {
		t.Fatalf("CountTokens after revoke: %v", err)
	}
	if n != 1 {
		t.Errorf("CountTokens after revoke: expected 1, got %d", n)
	}
}

// ─── Concurrent insert (race detector) ───────────────────────────────────────

// TestMetaStore_ConcurrentInsert_Race fires concurrent CreateUser calls to
// exercise the store under -race. Any unprotected shared state in the store
// or its underlying sql.DB wrapper would be flagged by the race detector.
func TestMetaStore_ConcurrentInsert_Race(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	const goroutines = 20

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if err := s.CreateUser(ctx, meta.User{
				Username: fmt.Sprintf("concurrent-user-%d", idx),
				PwHash:   "bcrypt:placeholder",
				Role:     "viewer",
			}); err != nil {
				errs <- fmt.Errorf("goroutine %d: %w", idx, err)
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	// All inserts must succeed — no concurrency error is tolerated.
	for err := range errs {
		t.Errorf("concurrent CreateUser: %v", err)
	}

	count, err := s.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers after concurrent inserts: %v", err)
	}
	if count != goroutines {
		t.Errorf("CountUsers: expected %d, got %d", goroutines, count)
	}
}
