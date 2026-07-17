// Package tenant resolves which tenant owns a stream, from the tenant registry
// (the meta-store `tenants` table). It is the single server-side tenant→stream
// mapping shared by reports (billing) and query (the live dashboard) — F6
// multi-tenancy. Relocated from internal/reports so the live query path and the
// alert evaluator can reuse the exact same resolution logic.
//
// Resolution precedence (documented):
//  1. Meta-tag match: if a session's beacon meta contains the meta_tag_key/value
//     pair defined on a Tenant row, that tenant wins.
//  2. Stream-name glob match: if the stream ID matches the tenant's stream_pattern
//     (SQL LIKE-style: % or * = any sequence, _ = any char; case-insensitive).
//  3. Unassigned: the empty string "". Callers display as "unassigned".
//
// Live-dashboard streams come from the AMS REST poller and carry no beacon meta,
// so for them only the stream_pattern (glob) rule applies; pass metaTags=nil.
// If multiple tenants match, the first wins (meta-tag before glob; within each
// type the order is undefined — operators must not configure overlapping rules).
package tenant

import (
	"strings"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// Matcher holds loaded tenant rules and resolves stream→tenant.
type Matcher struct {
	tenants []meta.TenantRow
}

// NewMatcher creates a matcher from tenant rows loaded from the meta store.
// A nil/empty slice yields a matcher that resolves everything to "" (unassigned).
func NewMatcher(tenants []meta.TenantRow) *Matcher {
	return &Matcher{tenants: tenants}
}

// Resolve returns the tenant name for a stream ID and optional beacon meta tags
// (metaTags may be nil). Returns "" if no tenant matches.
func (m *Matcher) Resolve(streamID string, metaTags map[string]string) string {
	// Precedence 1: meta-tag match.
	if len(metaTags) > 0 {
		for _, t := range m.tenants {
			if t.MetaTagKey != "" && t.MetaTagValue != "" {
				if v, ok := metaTags[t.MetaTagKey]; ok && v == t.MetaTagValue {
					return t.Name
				}
			}
		}
	}
	// Precedence 2: stream-name glob match.
	for _, t := range m.tenants {
		if t.StreamPattern != "" && globMatch(t.StreamPattern, streamID) {
			return t.Name
		}
	}
	return "" // unassigned
}

// globMatch returns true if pattern (SQL LIKE-style: % or * = any, _ = one char)
// matches the full string s (case-insensitive).
func globMatch(pattern, s string) bool {
	// Normalise: treat * as % for SQL LIKE compatibility.
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
