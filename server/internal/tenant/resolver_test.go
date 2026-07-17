package tenant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

func TestCachedResolver_CachesWithinTTL(t *testing.T) {
	var loads int
	load := func(context.Context) ([]meta.TenantRow, error) {
		loads++
		return []meta.TenantRow{{Name: "acme", StreamPattern: "acme-*"}}, nil
	}
	r := NewCachedResolver(load, time.Minute)
	now := time.Unix(1000, 0)
	r.now = func() time.Time { return now }

	for i := 0; i < 5; i++ {
		if got := r.ResolveTenant("acme-1"); got != "acme" {
			t.Fatalf("resolve #%d = %q, want acme", i, got)
		}
	}
	if loads != 1 {
		t.Fatalf("loads = %d, want 1 (cached within ttl)", loads)
	}
}

func TestCachedResolver_ReloadsAfterTTL(t *testing.T) {
	rows := []meta.TenantRow{{Name: "acme", StreamPattern: "acme-*"}}
	var loads int
	load := func(context.Context) ([]meta.TenantRow, error) {
		loads++
		return rows, nil
	}
	r := NewCachedResolver(load, 30*time.Second)
	now := time.Unix(1000, 0)
	r.now = func() time.Time { return now }

	_ = r.ResolveTenant("acme-1") // load 1
	now = now.Add(10 * time.Second)
	_ = r.ResolveTenant("acme-1") // cached
	if loads != 1 {
		t.Fatalf("loads = %d after 10s, want 1", loads)
	}

	// New tenant appears; advance past ttl → reload picks it up.
	rows = append(rows, meta.TenantRow{Name: "globex", StreamPattern: "gx-*"})
	now = now.Add(30 * time.Second)
	if got := r.ResolveTenant("gx-1"); got != "globex" {
		t.Fatalf("after ttl reload, resolve gx-1 = %q, want globex", got)
	}
	if loads != 2 {
		t.Fatalf("loads = %d, want 2 (reloaded after ttl)", loads)
	}
}

func TestCachedResolver_KeepsStaleMatcherOnLoadError(t *testing.T) {
	good := []meta.TenantRow{{Name: "acme", StreamPattern: "acme-*"}}
	fail := false
	load := func(context.Context) ([]meta.TenantRow, error) {
		if fail {
			return nil, errors.New("meta down")
		}
		return good, nil
	}
	r := NewCachedResolver(load, time.Second)
	now := time.Unix(1000, 0)
	r.now = func() time.Time { return now }

	if got := r.ResolveTenant("acme-1"); got != "acme" {
		t.Fatalf("initial resolve = %q, want acme", got)
	}
	// TTL expires and the reload fails — must keep serving the last good matcher,
	// NOT drop to unassigned (which would silently widen a tenant-scoped view).
	fail = true
	now = now.Add(2 * time.Second)
	if got := r.ResolveTenant("acme-1"); got != "acme" {
		t.Fatalf("after failed reload, resolve = %q, want acme (stale-but-safe)", got)
	}
}

func TestCachedResolver_EmptyBeforeFirstLoad(t *testing.T) {
	load := func(context.Context) ([]meta.TenantRow, error) {
		return nil, errors.New("down at startup")
	}
	r := NewCachedResolver(load, time.Second)
	r.now = func() time.Time { return time.Unix(1000, 0) }
	if got := r.ResolveTenant("acme-1"); got != "" {
		t.Fatalf("resolve before any successful load = %q, want empty", got)
	}
}
