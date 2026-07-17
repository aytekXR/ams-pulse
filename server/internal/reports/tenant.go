// Package reports — tenant mapping (F6 WO-204 item 2).
//
// The tenant→stream resolution logic was relocated to internal/tenant (F6
// Phase 1) so the live query path and the alert evaluator can reuse it. These
// aliases keep the reports-package API (TenantMatcher / NewTenantMatcher)
// unchanged for the Accountant. See internal/tenant for the resolution
// precedence (meta-tag > stream-name glob > unassigned).
package reports

import (
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
	"github.com/pulse-analytics/pulse/server/internal/tenant"
)

// TenantMatcher is the shared tenant.Matcher (relocated to internal/tenant).
type TenantMatcher = tenant.Matcher

// NewTenantMatcher creates a matcher from tenant rows loaded from the meta store.
func NewTenantMatcher(tenants []meta.TenantRow) *TenantMatcher {
	return tenant.NewMatcher(tenants)
}
