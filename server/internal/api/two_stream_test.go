// Package api_test — two-stream live provider fixture for BUG-009 cursor probe.
package api_test

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// fakeTwoStreamLiveProvider returns a snapshot with exactly 2 active streams
// (stream-a and stream-b). Used to prove that the cursor advances past page 1.
type fakeTwoStreamLiveProvider struct{}

func (f *fakeTwoStreamLiveProvider) CurrentSnapshot() *domain.LiveSnapshot {
	return &domain.LiveSnapshot{
		ActiveStreams: 2,
		Streams: map[string]*domain.LiveStream{
			"stream-a": {StreamID: "stream-a", App: "live", NodeID: "node-1", Active: true},
			"stream-b": {StreamID: "stream-b", App: "live", NodeID: "node-1", Active: true},
		},
		AppViewers: map[string]int{},
		Nodes:      map[string]*domain.LiveNodeStats{},
	}
}

func (f *fakeTwoStreamLiveProvider) Subscribe() (<-chan *domain.LiveSnapshot, func()) {
	ch := make(chan *domain.LiveSnapshot, 1)
	return ch, func() { close(ch) }
}

// setupTwoStreamServer mirrors setupHealthyTestServer but wires fakeTwoStreamLiveProvider.
// Returns (ts, adminToken, cleanup). The server uses a Business-tier license.
func setupTwoStreamServer(t *testing.T) (ts *httptest.Server, adminToken string, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	licKey, licCleanup := makeTestBusinessLicense(t)

	ddlPath := metaDDLPath(t)
	ddl, err := os.ReadFile(ddlPath)
	if err != nil {
		licCleanup()
		t.Fatalf("meta DDL not found: %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "two-stream-test-secret")
	if err != nil {
		licCleanup()
		t.Fatalf("meta.New: %v", err)
	}
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		store.Close()
		licCleanup()
		t.Fatalf("migrate: %v", err)
	}

	adminToken = "plt_twostream_testtoken_abcdef"
	tokenHash := hashToken(adminToken)
	if err := store.CreateToken(ctx, meta.APIToken{
		Kind:      "api",
		Name:      "two-stream-admin",
		TokenHash: tokenHash,
		Scopes:    []string{"admin"},
		CreatedAt: 1000,
	}); err != nil {
		store.Close()
		licCleanup()
		t.Fatalf("CreateToken: %v", err)
	}

	lic, err := license.New(licKey, "")
	if err != nil {
		store.Close()
		licCleanup()
		t.Fatalf("license.New (business): %v", err)
	}
	live := &fakeTwoStreamLiveProvider{}
	qsvc := query.New(live, nil, lic)

	apiCfg := api.Config{ListenAddr: ":0"}
	srv := api.New(apiCfg, store, live, qsvc, lic, nil)

	ts = httptest.NewServer(srv.Handler())
	cleanup = func() {
		ts.Close()
		store.Close()
		licCleanup()
	}
	return ts, adminToken, cleanup
}
