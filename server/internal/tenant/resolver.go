package tenant

import (
	"context"
	"sync"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// Loader loads the current tenant rows from the registry (typically
// meta.Store.ListTenants). Called at most once per CachedResolver ttl.
type Loader func(context.Context) ([]meta.TenantRow, error)

// CachedResolver implements query.TenantResolver: it resolves stream→tenant with
// a Matcher rebuilt from the tenant registry at most once per ttl, so the hot
// live-dashboard path does not hit the meta store on every request. Safe for
// concurrent use.
//
// Failure policy: on a load error it keeps serving the previous (stale) matcher
// rather than dropping tenant assignments to "unassigned" — a transient meta-store
// hiccup must not silently widen a tenant-scoped view. Before the first successful
// load it resolves everything to "" (unassigned).
type CachedResolver struct {
	load Loader
	ttl  time.Duration
	now  func() time.Time

	mu       sync.Mutex
	matcher  *Matcher
	loadedAt time.Time
	loaded   bool
}

// NewCachedResolver builds a resolver that refreshes its rules at most once per
// ttl via load. A ttl <= 0 reloads on every call.
func NewCachedResolver(load Loader, ttl time.Duration) *CachedResolver {
	return &CachedResolver{load: load, ttl: ttl, now: time.Now, matcher: NewMatcher(nil)}
}

// ResolveTenant resolves a live stream's tenant (glob only — live streams carry
// no beacon meta).
func (r *CachedResolver) ResolveTenant(streamID string) string {
	return r.current().Resolve(streamID, nil)
}

func (r *CachedResolver) current() *Matcher {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.loaded || r.now().Sub(r.loadedAt) >= r.ttl {
		rows, err := r.load(context.Background())
		if err == nil {
			r.matcher = NewMatcher(rows)
			r.loadedAt = r.now()
			r.loaded = true
		}
		// On error: keep the existing matcher (stale-but-safe). The initial
		// matcher is a non-nil empty NewMatcher(nil), so this never returns nil.
	}
	return r.matcher
}
