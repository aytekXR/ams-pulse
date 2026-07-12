package restpoller

// restpoller_vods_test.go — TDD suite for VoD REST polling (BUG-002, S23 author-P).
//
// Tests are in the INTERNAL package (package restpoller, not restpoller_test) so
// poll() and pollVods() can be driven directly without a real ticker or sleeps.
//
// fakeVodState is a thread-safe in-memory VodStateStore with injectable errors.
// Mock AMS is served via httptest with an atomic vods/list hit counter.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

// ─── fakeVodState ─────────────────────────────────────────────────────────────

type fakeVodState struct {
	mu        sync.Mutex
	seen      map[string]map[string]struct{} // app → vodID → {}
	listErr   error
	markErrs  []error // markErrs[i] returned on the i-th MarkVodSeen call (nil = success)
	markCount int
	Marks     []markCall // record of successful MarkVodSeen calls
}

type markCall struct {
	App       string
	VodID     string
	CreatedMS int64
}

func newFakeVodState() *fakeVodState {
	return &fakeVodState{
		seen: make(map[string]map[string]struct{}),
	}
}

func (f *fakeVodState) ListSeenVodIDs(ctx context.Context, app string) (map[string]struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make(map[string]struct{})
	for id := range f.seen[app] {
		out[id] = struct{}{}
	}
	return out, nil
}

func (f *fakeVodState) MarkVodSeen(ctx context.Context, app, vodID string, createdMS int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	idx := f.markCount
	f.markCount++
	if idx < len(f.markErrs) && f.markErrs[idx] != nil {
		return f.markErrs[idx]
	}
	if f.seen[app] == nil {
		f.seen[app] = make(map[string]struct{})
	}
	f.seen[app][vodID] = struct{}{}
	f.Marks = append(f.Marks, markCall{App: app, VodID: vodID, CreatedMS: createdMS})
	return nil
}

func (f *fakeVodState) seenCount(app string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.seen[app])
}

// ─── mockEventSink (follows latency_test.go pattern) ──────────────────────────

type mockVodSink struct {
	mu     sync.Mutex
	events []domain.ServerEvent
	notify chan struct{}
}

func newMockVodSink() *mockVodSink {
	return &mockVodSink{notify: make(chan struct{}, 200)}
}

func (m *mockVodSink) WriteServerEvent(ev domain.ServerEvent) {
	m.mu.Lock()
	m.events = append(m.events, ev)
	m.mu.Unlock()
	select {
	case m.notify <- struct{}{}:
	default:
	}
}

func (m *mockVodSink) WriteBeaconEvent(_ domain.BeaconEvent)     {}
func (m *mockVodSink) WriteViewerSession(_ domain.ViewerSession) {}

func (m *mockVodSink) countAll() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *mockVodSink) copyEvents() []domain.ServerEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.ServerEvent, len(m.events))
	copy(out, m.events)
	return out
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// newVodHandler returns an http.HandlerFunc serving the minimal AMS routes needed
// by poll() and pollVods() tests. vodsByApp is used to serve the vods/list endpoint;
// vodHits counts calls to any vods/list route.
//
// Unregistered routes return 404 (cluster/nodes → nil,nil in ClusterNodes;
// system-status → warn-and-continue; version → version="" fallback).
func newVodHandler(vodsByApp map[string][]amsclient.VodDTO, vodHits *atomic.Int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		// AMS cluster/nodes — standalone: return 404 so ClusterNodes yields (nil,nil).
		if path == "/rest/v2/cluster/nodes" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// broadcasts/list — return empty list so pollApp is a no-op.
		// Matches /{app}/rest/v2/broadcasts/list/{offset}/{size}.
		if len(path) > 30 && path[len(path)-len("/0/200"):] == "/0/200" {
			if contains(path, "/broadcasts/list/") {
				_ = json.NewEncoder(w).Encode([]any{})
				return
			}
			// vods/list
			if contains(path, "/vods/list/") {
				if vodHits != nil {
					vodHits.Add(1)
				}
				// Extract app name: /{app}/rest/v2/vods/list/0/200
				app := extractApp(path)
				list := vodsByApp[app]
				if list == nil {
					list = []amsclient.VodDTO{}
				}
				_ = json.NewEncoder(w).Encode(list)
				return
			}
		}

		// Fallthrough → 404.
		w.WriteHeader(http.StatusNotFound)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexStr(s, sub) >= 0)
}

func indexStr(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// extractApp extracts the first path segment: "/{app}/rest/..." → "app".
func extractApp(path string) string {
	if len(path) == 0 || path[0] != '/' {
		return ""
	}
	rest := path[1:]
	for i, c := range rest {
		if c == '/' {
			return rest[:i]
		}
	}
	return rest
}

// buildPollerFor builds a Poller using the given httptest server, VodState, and sink.
// PollInterval is set to 100 ms so the dedup window is 200 ms.
// Applications is pinned to ["testapp"] to avoid calling ListApplications.
func buildPollerFor(srv *httptest.Server, vodState VodStateStore, sink domain.EventSink) *Poller {
	client := amsclient.New(amsclient.Config{
		BaseURL: srv.URL,
		Timeout: 3 * time.Second,
	})
	return New(
		Config{
			NodeID:       "test-node",
			PollInterval: 100 * time.Millisecond,
			Applications: []string{"testapp"},
			VodState:     vodState,
		},
		client,
		sink,
		nil,
	)
}

// ─── Test 1: Backfill — 2 VoDs → 2 recording_ready events ────────────────────

func TestPollVods_Backfill(t *testing.T) {
	t.Parallel()

	vods := map[string][]amsclient.VodDTO{
		"testapp": {
			{VodID: "v1", VodName: "v1.mp4", StreamID: "stream1", FileSize: 1000, CreationDate: 1000, Duration: 43000},
			{VodID: "v2", VodName: "v2.mp4", StreamID: "stream2", FileSize: 2000, CreationDate: 2000, Duration: 0},
		},
	}
	srv := httptest.NewServer(newVodHandler(vods, nil))
	defer srv.Close()

	sink := newMockVodSink()
	fs := newFakeVodState()
	p := buildPollerFor(srv, fs, sink)

	ctx := context.Background()
	if err := p.pollVods(ctx, "testapp"); err != nil {
		t.Fatalf("pollVods: %v", err)
	}

	evs := sink.copyEvents()
	if len(evs) != 2 {
		t.Fatalf("want 2 events, got %d", len(evs))
	}

	// ── full event shape validation for event 0 (v1, sorted by CreationDate asc) ──
	e := evs[0]
	if e.Version != 1 {
		t.Errorf("Version: want 1, got %d", e.Version)
	}
	if e.Type != domain.EventRecordingReady {
		t.Errorf("Type: want %q, got %q", domain.EventRecordingReady, e.Type)
	}
	if e.Source != domain.SourceRestPoll {
		t.Errorf("Source: want %q, got %q", domain.SourceRestPoll, e.Source)
	}
	if e.NodeID != "test-node" {
		t.Errorf("NodeID: want %q, got %q", "test-node", e.NodeID)
	}
	if e.App != "testapp" {
		t.Errorf("App: want %q, got %q", "testapp", e.App)
	}
	if e.TS != 1000 {
		t.Errorf("TS: want 1000 (= CreationDate), got %d", e.TS)
	}
	if e.StreamID != "stream1" {
		t.Errorf("StreamID: want %q, got %q", "stream1", e.StreamID)
	}
	if e.Data["path"] != "v1.mp4" {
		t.Errorf("Data[path]: want %q, got %v", "v1.mp4", e.Data["path"])
	}
	if e.Data["size_bytes"] != int64(1000) {
		t.Errorf("Data[size_bytes]: want int64(1000), got %v (%T)", e.Data["size_bytes"], e.Data["size_bytes"])
	}
	// Duration 43000 ms → 43 s.
	if e.Data["duration_s"] != int64(43) {
		t.Errorf("Data[duration_s]: want int64(43), got %v (%T)", e.Data["duration_s"], e.Data["duration_s"])
	}

	// ── event 1 (v2, Duration=0) — duration_s must be absent ──
	e2 := evs[1]
	if e2.StreamID != "stream2" {
		t.Errorf("e2.StreamID: want %q, got %q", "stream2", e2.StreamID)
	}
	if _, ok := e2.Data["duration_s"]; ok {
		t.Errorf("Data[duration_s]: must be absent for zero-duration VoD, got %v", e2.Data["duration_s"])
	}

	// Both VoDs must be marked seen.
	if fs.seenCount("testapp") != 2 {
		t.Errorf("want 2 seen VoDs, got %d", fs.seenCount("testapp"))
	}
}

// ─── Test 2: Second cycle, same VoDs → 0 new events ──────────────────────────

func TestPollVods_NoDuplicateOnSecondCycle(t *testing.T) {
	t.Parallel()

	vods := map[string][]amsclient.VodDTO{
		"testapp": {
			{VodID: "v1", VodName: "v1.mp4", StreamID: "s1", FileSize: 100, CreationDate: 1000},
			{VodID: "v2", VodName: "v2.mp4", StreamID: "s2", FileSize: 200, CreationDate: 2000},
		},
	}
	srv := httptest.NewServer(newVodHandler(vods, nil))
	defer srv.Close()

	sink := newMockVodSink()
	fs := newFakeVodState()
	p := buildPollerFor(srv, fs, sink)
	ctx := context.Background()

	// Cycle 1: initial backfill.
	if err := p.pollVods(ctx, "testapp"); err != nil {
		t.Fatalf("cycle 1: %v", err)
	}
	// Cycle 2: same VoDs — must produce 0 new events.
	if err := p.pollVods(ctx, "testapp"); err != nil {
		t.Fatalf("cycle 2: %v", err)
	}

	if got := sink.countAll(); got != 2 {
		t.Errorf("want 2 total events after 2 cycles (no duplicates), got %d", got)
	}
}

// ─── Test 3: Third VoD appears → exactly 1 new event ─────────────────────────

func TestPollVods_NewVodAfterBackfill(t *testing.T) {
	t.Parallel()

	var vodsMu sync.Mutex
	vodList := []amsclient.VodDTO{
		{VodID: "v1", VodName: "v1.mp4", StreamID: "s1", FileSize: 100, CreationDate: 1000},
		{VodID: "v2", VodName: "v2.mp4", StreamID: "s2", FileSize: 200, CreationDate: 2000},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/testapp/rest/v2/vods/list/0/200" {
			vodsMu.Lock()
			list := make([]amsclient.VodDTO, len(vodList))
			copy(list, vodList)
			vodsMu.Unlock()
			_ = json.NewEncoder(w).Encode(list)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	sink := newMockVodSink()
	fs := newFakeVodState()
	p := buildPollerFor(srv, fs, sink)
	ctx := context.Background()

	// Cycle 1: 2 VoDs.
	if err := p.pollVods(ctx, "testapp"); err != nil {
		t.Fatalf("cycle 1: %v", err)
	}

	// Add third VoD.
	vodsMu.Lock()
	vodList = append(vodList, amsclient.VodDTO{
		VodID: "v3", VodName: "v3.mp4", StreamID: "s3", FileSize: 300, CreationDate: 3000,
	})
	vodsMu.Unlock()

	// Cycle 2: only v3 is new.
	if err := p.pollVods(ctx, "testapp"); err != nil {
		t.Fatalf("cycle 2: %v", err)
	}

	evs := sink.copyEvents()
	if len(evs) != 3 {
		t.Fatalf("want 3 total events (2 backfill + 1 new), got %d", len(evs))
	}
	// Third event must be v3.
	if evs[2].StreamID != "s3" {
		t.Errorf("3rd event StreamID: want %q, got %q", "s3", evs[2].StreamID)
	}
}

// ─── Test 4: RESTART-RESUME — second Poller with pre-seeded seen-set emits 0 ─

func TestPollVods_RestartResume(t *testing.T) {
	t.Parallel()

	vods := map[string][]amsclient.VodDTO{
		"testapp": {
			{VodID: "v1", VodName: "v1.mp4", StreamID: "s1", FileSize: 100, CreationDate: 1000},
			{VodID: "v2", VodName: "v2.mp4", StreamID: "s2", FileSize: 200, CreationDate: 2000},
		},
	}
	srv := httptest.NewServer(newVodHandler(vods, nil))
	defer srv.Close()

	// First Poller: emits 2 VoDs, marks both seen.
	sink1 := newMockVodSink()
	fs1 := newFakeVodState()
	p1 := buildPollerFor(srv, fs1, sink1)
	ctx := context.Background()

	if err := p1.pollVods(ctx, "testapp"); err != nil {
		t.Fatalf("p1 pollVods: %v", err)
	}
	if sink1.countAll() != 2 {
		t.Fatalf("p1: expected 2 events, got %d", sink1.countAll())
	}

	// Simulate restart: build a second fakeVodState pre-seeded with fs1's seen-set.
	fs2 := newFakeVodState()
	fs1.mu.Lock()
	for app, ids := range fs1.seen {
		fs2.seen[app] = make(map[string]struct{})
		for id := range ids {
			fs2.seen[app][id] = struct{}{}
		}
	}
	fs1.mu.Unlock()

	// Second Poller shares same seen-set → must emit 0 events (no double-count).
	sink2 := newMockVodSink()
	p2 := buildPollerFor(srv, fs2, sink2)

	if err := p2.pollVods(ctx, "testapp"); err != nil {
		t.Fatalf("p2 pollVods: %v", err)
	}
	if got := sink2.countAll(); got != 0 {
		t.Errorf("RESTART-RESUME: want 0 re-emits from second Poller, got %d", got)
	}
}

// ─── Test 5: DEDUP-BYPASS regression pin ──────────────────────────────────────
//
// Guards against routing VoD events through p.dedup. Two VoDs with the same
// StreamID and TS share a dedup key {recording_ready, nodeID, streamID, window}.
// If p.dedup.IsDuplicate is called in pollVods, the second event is silently
// dropped. Both MUST always reach the sink — the seen-set in VodState is the
// correct dedup mechanism for VoDs.
//
// The single-line change this pins: adding `if !p.dedup.IsDuplicate(ev) { continue }`
// before p.sink.WriteServerEvent in pollVods would cause this test to fail (1 event
// instead of 2). The verifier may mutation-test by adding that line.
func TestPollVods_DedupBypass_Regression(t *testing.T) {
	t.Parallel()

	const sharedStreamID = "shared-stream"
	const sharedTS = int64(1_700_000_000_000) // same TS → same dedup window

	vods := map[string][]amsclient.VodDTO{
		"testapp": {
			{VodID: "vid-a", VodName: "a.mp4", StreamID: sharedStreamID, FileSize: 100, CreationDate: sharedTS},
			{VodID: "vid-b", VodName: "b.mp4", StreamID: sharedStreamID, FileSize: 200, CreationDate: sharedTS},
		},
	}
	srv := httptest.NewServer(newVodHandler(vods, nil))
	defer srv.Close()

	sink := newMockVodSink()
	fs := newFakeVodState()

	// Construct Poller exactly as production: New() creates collector.NewDeduplicator
	// internally. This test verifies that pollVods does NOT use p.dedup.IsDuplicate.
	p := buildPollerFor(srv, fs, sink)
	ctx := context.Background()

	if err := p.pollVods(ctx, "testapp"); err != nil {
		t.Fatalf("pollVods: %v", err)
	}

	if got := sink.countAll(); got != 2 {
		t.Errorf("DEDUP-BYPASS: want 2 events (both VoDs), got %d — "+
			"VoD events must NOT be routed through p.dedup", got)
	}
}

// ─── Test 6: At-most-once — MarkVodSeen error aborts cycle ───────────────────

func TestPollVods_AtMostOnce_MarkSeenErrorAbortsEmit(t *testing.T) {
	t.Parallel()

	vods := map[string][]amsclient.VodDTO{
		"testapp": {
			{VodID: "v1", VodName: "v1.mp4", StreamID: "s1", FileSize: 100, CreationDate: 1000},
			{VodID: "v2", VodName: "v2.mp4", StreamID: "s2", FileSize: 200, CreationDate: 2000},
		},
	}
	srv := httptest.NewServer(newVodHandler(vods, nil))
	defer srv.Close()

	sink := newMockVodSink()
	fs := newFakeVodState()
	// First MarkVodSeen (v1) succeeds; second (v2) fails.
	fs.markErrs = []error{nil, errors.New("db write failed")}

	p := buildPollerFor(srv, fs, sink)
	ctx := context.Background()

	err := p.pollVods(ctx, "testapp")
	if err == nil {
		t.Fatal("expected error from pollVods when MarkVodSeen fails, got nil")
	}

	// Exactly 1 event must have been emitted (v1 only).
	if got := sink.countAll(); got != 1 {
		t.Errorf("at-most-once: want exactly 1 event emitted (v1), got %d", got)
	}
	// v2 must NOT be marked seen.
	if got := fs.seenCount("testapp"); got != 1 {
		t.Errorf("at-most-once: want 1 seen VoD (v1 only), got %d", got)
	}
}

// ─── Test 7: Cadence — poll() fires vods/list exactly on ticks 0 and 12 ──────

func TestPollVods_Cadence(t *testing.T) {
	t.Parallel()

	vodList := map[string][]amsclient.VodDTO{
		"testapp": {
			{VodID: "v1", VodName: "v1.mp4", StreamID: "s1", FileSize: 100, CreationDate: 1000},
			{VodID: "v2", VodName: "v2.mp4", StreamID: "s2", FileSize: 200, CreationDate: 2000},
			{VodID: "v3", VodName: "v3.mp4", StreamID: "s3", FileSize: 300, CreationDate: 3000},
		},
	}

	t.Run("configured", func(t *testing.T) {
		t.Parallel()

		var vodHits atomic.Int64
		srv := httptest.NewServer(newVodHandler(vodList, &vodHits))
		defer srv.Close()

		sink := newMockVodSink()
		fs := newFakeVodState()
		p := buildPollerFor(srv, fs, sink)
		ctx := context.Background()

		// 13 poll() calls: tick 0 and tick 12 fire vodDue (0%12==0, 12%12==0).
		for i := 0; i < 13; i++ {
			if err := p.poll(ctx); err != nil {
				t.Errorf("poll() #%d: %v", i, err)
			}
		}

		if hits := vodHits.Load(); hits != 2 {
			t.Errorf("cadence: want 2 vods/list hits (ticks 0 and 12), got %d", hits)
		}
	})

	t.Run("disabled_when_nil_vodstate", func(t *testing.T) {
		t.Parallel()

		var vodHits atomic.Int64
		srv := httptest.NewServer(newVodHandler(vodList, &vodHits))
		defer srv.Close()

		sink := newMockVodSink()
		// VodState = nil → VoD polling disabled.
		p := buildPollerFor(srv, nil, sink)
		ctx := context.Background()

		for i := 0; i < 13; i++ {
			if err := p.poll(ctx); err != nil {
				t.Errorf("poll() #%d: %v", i, err)
			}
		}

		if hits := vodHits.Load(); hits != 0 {
			t.Errorf("disabled: want 0 vods/list hits when VodState=nil, got %d", hits)
		}
	})
}

// ─── Test 8: Empty VodID skipped — no emit, no mark ──────────────────────────

func TestPollVods_EmptyVodIDSkipped(t *testing.T) {
	t.Parallel()

	vods := map[string][]amsclient.VodDTO{
		"testapp": {
			// Entry with empty VodID: no stable dedup key → must be skipped.
			{VodID: "", VodName: "nokey.mp4", StreamID: "s0", FileSize: 100, CreationDate: 500},
			// Valid entry: must be emitted.
			{VodID: "v2", VodName: "v2.mp4", StreamID: "s2", FileSize: 200, CreationDate: 2000},
		},
	}
	srv := httptest.NewServer(newVodHandler(vods, nil))
	defer srv.Close()

	sink := newMockVodSink()
	fs := newFakeVodState()
	p := buildPollerFor(srv, fs, sink)
	ctx := context.Background()

	if err := p.pollVods(ctx, "testapp"); err != nil {
		t.Fatalf("pollVods: %v", err)
	}

	// Only v2 emitted; empty-ID entry skipped.
	if got := sink.countAll(); got != 1 {
		t.Errorf("empty-id: want 1 event (v2 only), got %d", got)
	}
	// Empty-ID entry not marked.
	if got := fs.seenCount("testapp"); got != 1 {
		t.Errorf("empty-id: want 1 mark (v2 only), got %d", got)
	}
}
