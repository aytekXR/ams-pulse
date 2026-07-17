// F6 Phase 2 (D-149): tenant-scoped QoE alert rules — closes S73 finding [5].
// The evaluator must thread the rule's AlertScope.Tenant through to the QoEReader
// so a QoE rule reads only that tenant's rebuffer/error metrics (not a blend of
// every tenant reusing the same app+stream). These are unit tests over the
// FakeQoEReader.LastTenant capture; the ClickHouse WHERE tenant=? SQL that
// actually applies the scope is covered by the QoeSummary tenant-filter tests
// (S73 [1] / D-137).
package alert_test

import (
	"context"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

func evalWithScopeTenant(t *testing.T, scopeJSON string) *alert.FakeQoEReader {
	t.Helper()
	store := openTestStore(t)
	ctx := context.Background()

	row := meta.AlertRuleRow{
		Name:               "tenant-scoped-rebuffer",
		Metric:             "rebuffer_ratio",
		Operator:           "gt",
		Threshold:          0.05,
		WindowS:            5,
		ScopeJSON:          scopeJSON,
		Severity:           "warning",
		CooldownS:          300,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	}
	if _, err := store.CreateAlertRule(ctx, row); err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live := newFakeLive()
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"shared-stream": {StreamID: "shared-stream", App: "live", Active: true},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	})

	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	reader := &alert.FakeQoEReader{RebufferRatio: 0.1}
	ev.SetQoEReader(reader)

	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	return reader
}

// A rule scoped to tenant=acme must call the QoE reader with tenant="acme" so
// the per-stream read is isolated to that tenant.
func TestQoERule_TenantScope_ThreadedToReader(t *testing.T) {
	reader := evalWithScopeTenant(t, `{"tenant":"acme"}`)
	if reader.LastTenant != "acme" {
		t.Fatalf("QoEReader called with tenant=%q, want acme (rule scope tenant not threaded — S73 [5] regression)", reader.LastTenant)
	}
}

// An unscoped rule (no tenant) must call the reader with tenant="" (all tenants),
// preserving pre-F6 behavior for existing rules.
func TestQoERule_NoTenantScope_ReaderGetsEmpty(t *testing.T) {
	reader := evalWithScopeTenant(t, `{}`)
	if reader.LastTenant != "" {
		t.Fatalf("unscoped rule called reader with tenant=%q, want empty (all tenants)", reader.LastTenant)
	}
}
