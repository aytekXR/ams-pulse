// Package reports — tenant mapping (F6 WO-204 item 2).
//
// Resolution precedence (documented):
//  1. Meta-tag match: if a session's beacon meta contains the meta_tag_key/value
//     pair defined on a Tenant row, that tenant wins.
//  2. Stream-name glob match: if the stream ID matches the tenant's stream_pattern
//     (SQL LIKE-style: % = any sequence, _ = any char, * also treated as %).
//  3. Unassigned: the empty string "". Callers should display as "unassigned".
//
// If multiple tenants match, the first one wins (order: meta-tag > glob,
// and within each type, the order is undefined — operators should not configure
// overlapping patterns).
package reports

import (
	"strings"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// TenantMatcher holds loaded tenant rules and resolves stream→tenant.
type TenantMatcher struct {
	tenants []meta.TenantRow
}

// NewTenantMatcher creates a matcher from tenant rows loaded from the meta store.
func NewTenantMatcher(tenants []meta.TenantRow) *TenantMatcher {
	return &TenantMatcher{tenants: tenants}
}

// Resolve returns the tenant name for a given stream ID and optional meta tags.
// metaTags may be nil if no beacon meta is available.
// Returns "" if no tenant matches (callers display as "unassigned").
func (tm *TenantMatcher) Resolve(streamID string, metaTags map[string]string) string {
	// Phase 1: meta-tag match (higher precedence).
	if len(metaTags) > 0 {
		for _, t := range tm.tenants {
			if t.MetaTagKey != "" && t.MetaTagValue != "" {
				if v, ok := metaTags[t.MetaTagKey]; ok && v == t.MetaTagValue {
					return t.Name
				}
			}
		}
	}

	// Phase 2: stream-name glob match.
	for _, t := range tm.tenants {
		if t.StreamPattern != "" && globMatch(t.StreamPattern, streamID) {
			return t.Name
		}
	}

	return "" // unassigned
}

// globMatch returns true if pattern (SQL LIKE-style: % or * = any, _ = one char)
// matches the full string s (case-insensitive).
func globMatch(pattern, s string) bool {
	// Normalise: replace * with % for SQL LIKE compat.
	pattern = strings.ReplaceAll(pattern, "*", "%")
	return likeMatch(strings.ToLower(pattern), strings.ToLower(s))
}

// likeMatch is a minimal SQL LIKE pattern matcher (% = any sequence, _ = any char).
func likeMatch(pattern, s string) bool {
	if pattern == "" {
		return s == ""
	}
	if pattern == "%" {
		return true
	}
	if pattern[0] == '%' {
		for i := 0; i <= len(s); i++ {
			if likeMatch(pattern[1:], s[i:]) {
				return true
			}
		}
		return false
	}
	if s == "" {
		return false
	}
	if pattern[0] == '_' || pattern[0] == s[0] {
		return likeMatch(pattern[1:], s[1:])
	}
	return false
}
