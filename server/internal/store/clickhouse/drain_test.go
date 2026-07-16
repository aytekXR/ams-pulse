// Package clickhouse — unit tests for Store.Close() graceful drain.
//
// TDD: each test in this file was written against the *original* sleep-based
// Close() and confirmed to fail (RED) before the WaitGroup drain was added.
// The tests are race-clean: run with -race -count=4.
package clickhouse

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/column"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// ─── Minimal mock implementing driver.Conn + driver.Batch ────────────────────

// mockConn records inserts and tracks call ordering for drain assertions.
// Thread-safe: all mutable fields guarded by mu.
type mockConn struct {
	mu sync.Mutex

	// closed is set to true by Close(). PrepareBatch returns an error when
	// closed==true, making it impossible for goroutines to accidentally count
	// events after conn.Close() has been called. This gives TestDrain_NoLoss
	// a deterministic RED guarantee: in the old (sleep-based) Close() the
	// connection is closed before goroutines can drain the channel, so any
	// drain attempt fails at PrepareBatch rather than succeeding by luck.
	closed bool

	// Per-channel insert counters (incremented on batch.Send()).
	serverRows int
	beaconRows int
	viewerRows int

	// callLog records "send" and "close" in order — used by ordering tests.
	callLog []string

	// sendDelay is an artificial delay applied inside Send() after unblocking.
	sendDelay time.Duration

	// blockSend, when non-nil, blocks every Send() call until closed.
	// Close the channel to unblock all pending Send() calls simultaneously.
	blockSend <-chan struct{}

	// beforeSend, when non-nil, receives one struct{}{} just before each Send()
	// call blocks/delays. Buffered (capacity >= expected signals) so senders
	// never block on it.
	beforeSend chan<- struct{}

	// Send-failure injection (finding [13]): when sendErr != nil, the sendFailOnCall-th
	// (1-indexed) Send() returns sendErr WITHOUT committing its rows. Default zero
	// value never fails, so existing tests are unaffected.
	sendCalls      int
	sendFailOnCall int
	sendErr        error
}

func newMockConn() *mockConn { return &mockConn{} }

// totalRows returns the sum across all channels.
func (m *mockConn) totalRows() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.serverRows + m.beaconRows + m.viewerRows
}

// rowsByType returns a snapshot of per-channel counts.
func (m *mockConn) rowsByType() (server, beacon, viewer int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.serverRows, m.beaconRows, m.viewerRows
}

// callLogSnapshot returns a copy of the call log.
func (m *mockConn) callLogSnapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.callLog))
	copy(out, m.callLog)
	return out
}

// ── driver.Conn interface ─────────────────────────────────────────────────────

func (m *mockConn) PrepareBatch(ctx context.Context, query string, opts ...driver.PrepareBatchOption) (driver.Batch, error) {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		// Simulate the real driver: operations on a closed connection fail.
		// In TestDrain_NoLoss this ensures that goroutines which accidentally
		// reach a drain Send() after the old impl's conn.Close() cannot
		// increment the row counters, giving a deterministic RED guarantee.
		return nil, fmt.Errorf("clickhouse: connection already closed")
	}
	return &mockBatch{conn: m, query: query}, nil
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	m.closed = true
	m.callLog = append(m.callLog, "close")
	m.mu.Unlock()
	return nil
}

func (m *mockConn) Contributors() []string                                    { return nil }
func (m *mockConn) ServerVersion() (*driver.ServerVersion, error)             { return nil, nil }
func (m *mockConn) Select(_ context.Context, _ any, _ string, _ ...any) error { return nil }
func (m *mockConn) Query(_ context.Context, _ string, _ ...any) (driver.Rows, error) {
	return nil, nil
}
func (m *mockConn) QueryRow(_ context.Context, _ string, _ ...any) driver.Row { return nil }
func (m *mockConn) Exec(_ context.Context, _ string, _ ...any) error          { return nil }
func (m *mockConn) AsyncInsert(_ context.Context, _ string, _ bool, _ ...any) error {
	return nil
}
func (m *mockConn) Ping(_ context.Context) error { return nil }
func (m *mockConn) Stats() driver.Stats          { return driver.Stats{} }

// ── mockBatch ─────────────────────────────────────────────────────────────────

// mockBatch accumulates row counts and applies blocking/delay on Send().
type mockBatch struct {
	conn     *mockConn
	query    string // query string identifies the target table
	rowCount int
}

func (b *mockBatch) Append(_ ...any) error {
	b.rowCount++
	return nil
}

func (b *mockBatch) Send() error {
	// Signal that we are about to block/delay.
	if b.conn.beforeSend != nil {
		select {
		case b.conn.beforeSend <- struct{}{}:
		default:
		}
	}
	// Block until the test releases us.
	if b.conn.blockSend != nil {
		<-b.conn.blockSend
	}
	// Artificial delay (applied after unblocking so timing is predictable).
	if b.conn.sendDelay > 0 {
		time.Sleep(b.conn.sendDelay)
	}

	b.conn.mu.Lock()
	b.conn.sendCalls++
	if b.conn.sendErr != nil && b.conn.sendCalls == b.conn.sendFailOnCall {
		b.conn.callLog = append(b.conn.callLog, "send-fail")
		b.conn.mu.Unlock()
		return b.conn.sendErr
	}
	switch {
	case strings.Contains(b.query, "server_events"):
		b.conn.serverRows += b.rowCount
	case strings.Contains(b.query, "beacon_events"):
		b.conn.beaconRows += b.rowCount
	case strings.Contains(b.query, "viewer_sessions"):
		b.conn.viewerRows += b.rowCount
	}
	b.conn.callLog = append(b.conn.callLog, "send")
	b.conn.mu.Unlock()

	return nil
}

func (b *mockBatch) Abort() error                    { return nil }
func (b *mockBatch) AppendStruct(_ any) error        { return nil }
func (b *mockBatch) Column(_ int) driver.BatchColumn { return nil }
func (b *mockBatch) Flush() error                    { return nil }
func (b *mockBatch) IsSent() bool                    { return false }
func (b *mockBatch) Rows() int                       { return b.rowCount }
func (b *mockBatch) Columns() []column.Interface     { return nil }
func (b *mockBatch) Close() error                    { return nil }

// ─── Test helpers ─────────────────────────────────────────────────────────────

// newTestStore constructs a Store with an injected mock connection.
// BatchSize and FlushInterval are configurable; the default FlushInterval is
// very long (60s) to prevent periodic flushes from interfering with tests.
func newTestStore(conn *mockConn, batchSize int) *Store {
	if batchSize <= 0 {
		batchSize = 10
	}
	return &Store{
		cfg: Config{
			BatchSize:     batchSize,
			FlushInterval: 60 * time.Second,
			Database:      "test",
		},
		conn:          conn,
		db:            "test",
		serverEventCh: make(chan domain.ServerEvent, batchSize*2),
		beaconEventCh: make(chan domain.BeaconEvent, batchSize*2),
		viewerSessCh:  make(chan domain.ViewerSession, batchSize*2),
		done:          make(chan struct{}),
	}
}

func makeServerEvent(i int) domain.ServerEvent {
	return domain.ServerEvent{
		Version:  1,
		Type:     domain.EventStreamStats,
		TS:       time.Now().UnixMilli(),
		Source:   domain.SourceRestPoll,
		NodeID:   "node-1",
		App:      "live",
		StreamID: fmt.Sprintf("stream-%d", i),
		Data:     map[string]any{"viewer_count": i},
	}
}

func makeBeaconEvent(i int) domain.BeaconEvent {
	return domain.BeaconEvent{
		Version:   1,
		SessionID: fmt.Sprintf("sess-%d", i),
		StreamID:  fmt.Sprintf("stream-%d", i),
		App:       "live",
		// One item per event so count == BeaconEvents queued.
		Events: []domain.BeaconItem{
			{
				Type: "heartbeat",
				TS:   time.Now().UnixMilli(),
				Data: map[string]any{"watch_ms": float64(1000 * i)},
			},
		},
	}
}

func makeViewerSession(i int) domain.ViewerSession {
	now := time.Now().UTC()
	return domain.ViewerSession{
		SessionID: fmt.Sprintf("sess-%d", i),
		StreamID:  fmt.Sprintf("stream-%d", i),
		App:       "live",
		NodeID:    "node-1",
		StartedAt: now,
		EndedAt:   now,
		UpdatedAt: now,
	}
}

// ─── TestDrain_NoLoss ─────────────────────────────────────────────────────────

// TestDrain_NoLoss verifies that every event queued before Close() returns is
// durably inserted. The test is designed to FAIL with the old sleep-based
// Close() (which drops events still buffered in the channels) and PASS after
// the WaitGroup drain is implemented.
//
// Strategy: fill each channel with 2×BatchSize events. Block the first batch's
// Send() until the channels hold the second batch, then start Close() and
// wait 200 ms before releasing the block.
//
// Why 200 ms is deterministic for the RED case: the old Close() did
// close(done); sleep(100ms); conn.Close(). By t=200ms the old impl has already
// called conn.Close(), which sets mockConn.closed=true. Any goroutine that
// subsequently tries to drain the channel and call PrepareBatch receives an
// error — so the second-batch events can never be counted by accident regardless
// of how the Go scheduler orders the select cases. This removes the ~0.2%
// false-pass probability that a plain timing-based check has.
//
// In the new impl, Close() blocks in wg.Wait() until the goroutines drain their
// channels. Releasing blockSend at t=200ms lets all three flushers complete
// their first-batch Send, enter the drain loop, consume the second batch (while
// closed=false), and only then does conn.Close() set closed=true.
//
// Expected RED evidence (old impl, batchSize=3): got 3 rows per channel
// (first batch only), want 6. Total: 9/18 rows inserted.
// Expected GREEN evidence (new impl): got 6 rows per channel, total 18/18.
func TestDrain_NoLoss(t *testing.T) {
	const batchSize = 3

	// blockSend gates every Send() call; close it to release all blocked calls.
	blockSend := make(chan struct{})
	// beforeSend receives one signal per flusher when they enter Send().
	// Capacity 10 to absorb all signals without blocking.
	beforeSendCh := make(chan struct{}, 10)

	mock := newMockConn()
	mock.blockSend = blockSend
	mock.beforeSend = beforeSendCh

	store := newTestStore(mock, batchSize)

	// Phase 1: queue the first batch (BatchSize per channel) BEFORE starting so
	// the goroutines find events waiting and immediately trigger a flush.
	for i := 0; i < batchSize; i++ {
		store.serverEventCh <- makeServerEvent(i)
		store.beaconEventCh <- makeBeaconEvent(i)
		store.viewerSessCh <- makeViewerSession(i)
	}

	ctx := context.Background()
	store.Start(ctx)

	// Wait for all three flushers to enter Send() (and block there).
	// This ensures the channels are empty and the flushers are definitely
	// inside Send() when we queue the second batch.
	for i := 0; i < 3; i++ {
		select {
		case <-beforeSendCh:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for flusher goroutines to enter Send()")
		}
	}

	// Phase 2: queue the second batch while flushers are blocked.
	for i := batchSize; i < 2*batchSize; i++ {
		store.serverEventCh <- makeServerEvent(i)
		store.beaconEventCh <- makeBeaconEvent(i)
		store.viewerSessCh <- makeViewerSession(i)
	}

	// Phase 3: start Close() concurrently, then release the block after 200 ms.
	// 200 ms is past the 100 ms sleep in the old (pre-fix) Close(), so by the
	// time we release blockSend, old impl has already called conn.Close()
	// (setting mockConn.closed=true). Any goroutine that then picks the channel
	// case in its select will hit PrepareBatch→error and cannot count events.
	// New impl is still blocked in wg.Wait() at t=200ms (goroutines are in Send());
	// releasing blockSend lets them drain and then proceed to conn.Close().
	closeDone := make(chan struct{})
	go func() {
		store.Close()
		close(closeDone)
	}()

	time.Sleep(200 * time.Millisecond)
	close(blockSend)

	// Wait for Close() to return (up to 10s).
	select {
	case <-closeDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Close() did not return within 10s; possible deadlock")
	}

	// Assert: ALL 2*batchSize events per channel were inserted.
	// With old sleep-based Close(): only the first batch (batchSize each) is
	// inserted — the second batch stays in the channel and is dropped.
	serverRows, beaconRows, viewerRows := mock.rowsByType()

	const want = 2 * batchSize
	if serverRows != want {
		t.Errorf("server_events: got %d rows, want %d (events in channel were dropped by Close())", serverRows, want)
	}
	if beaconRows != want {
		// Each BeaconEvent has 1 item, so beaconRows should equal events queued.
		t.Errorf("beacon_events: got %d rows, want %d (events in channel were dropped by Close())", beaconRows, want)
	}
	if viewerRows != want {
		t.Errorf("viewer_sessions: got %d rows, want %d (events in channel were dropped by Close())", viewerRows, want)
	}
}

// ─── TestDrain_Ordering ───────────────────────────────────────────────────────

// TestDrain_Ordering verifies that conn.Close() is called AFTER the last
// Send(), not during or before it.
//
// Strategy: make Send() take sendDelay=150ms. Call Close() while Send() is
// in progress. Old impl: conn.Close() fires after 100ms sleep (Send still
// running at 150ms), so "close" appears before the last "send" in callLog.
// New impl: wg.Wait() blocks until Send() finishes, so "close" is last.
func TestDrain_Ordering(t *testing.T) {
	const batchSize = 3
	const sendDelay = 150 * time.Millisecond

	blockSend := make(chan struct{})
	beforeSendCh := make(chan struct{}, 1)

	mock := newMockConn()
	mock.blockSend = blockSend
	mock.beforeSend = beforeSendCh
	mock.sendDelay = sendDelay

	store := newTestStore(mock, batchSize)

	// Queue exactly one batch for server events to trigger a flush.
	for i := 0; i < batchSize; i++ {
		store.serverEventCh <- makeServerEvent(i)
	}

	store.Start(context.Background())

	// Wait for the flusher to enter Send() (blocked + about to apply sendDelay).
	select {
	case <-beforeSendCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for flusher to enter Send()")
	}

	// Start Close() and release the block so Send() can proceed (but still takes
	// sendDelay to finish after unblocking).
	closeDone := make(chan struct{})
	go func() {
		store.Close()
		close(closeDone)
	}()

	// Give Close() time to call close(done).
	time.Sleep(10 * time.Millisecond)
	// Release Send(): it will now sleep for sendDelay (150ms).
	close(blockSend)

	// Wait for Close() + extra margin to ensure the goroutine has finished
	// even under the old (sleep-based) implementation.
	select {
	case <-closeDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Close() did not return within 10s")
	}

	// Extra wait: with old impl Close() returns before Send() finishes
	// (100ms sleep < 160ms total delay). We wait long enough to capture
	// the goroutine's final "send" entry in the callLog.
	time.Sleep(400 * time.Millisecond)

	log := mock.callLogSnapshot()
	if len(log) == 0 {
		t.Fatal("callLog is empty; no Send() or Close() was recorded")
	}
	last := log[len(log)-1]
	if last != "close" {
		// callLog ordering violation: "close" appeared before the last "send".
		t.Errorf("callLog ordering violation: last entry is %q, want \"close\"; full log: %v", last, log)
	}
	// Also verify at least one "send" happened.
	hasSend := false
	for _, entry := range log {
		if entry == "send" {
			hasSend = true
			break
		}
	}
	if !hasSend {
		t.Error("no \"send\" entry in callLog; expected at least one insert")
	}
}

// ─── TestDrain_DoubleClose ────────────────────────────────────────────────────

// TestDrain_DoubleClose verifies that calling Close() twice does not panic or
// deadlock (idempotency via once.Do).
func TestDrain_DoubleClose(t *testing.T) {
	mock := newMockConn()
	store := newTestStore(mock, 10)
	store.Start(context.Background())

	store.Close()
	store.Close() // must not panic or deadlock
}

// ─── TestDrain_CloseWithoutStart ─────────────────────────────────────────────

// TestDrain_CloseWithoutStart verifies that Close() returns promptly when
// Start() was never called (the WaitGroup counter is zero).
func TestDrain_CloseWithoutStart(t *testing.T) {
	mock := newMockConn()
	store := newTestStore(mock, 10)

	done := make(chan struct{})
	go func() {
		store.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not return within 2s when Start() was never called; deadlock?")
	}
}

// ─── TestDrain_CtxCancelThenClose ────────────────────────────────────────────

// TestDrain_CtxCancelThenClose verifies that cancelling the context (fast
// exit) and then calling Close() does not deadlock. The WaitGroup reaches
// zero when the flushers exit via ctx.Done(); wg.Wait() in Close() then
// returns immediately.
func TestDrain_CtxCancelThenClose(t *testing.T) {
	mock := newMockConn()
	store := newTestStore(mock, 10)

	ctx, cancel := context.WithCancel(context.Background())
	store.Start(ctx)

	// Cancel context: flushers enter fast-exit path (flush in-memory batch, return).
	cancel()
	// Give flushers time to exit (they should be nearly instantaneous on no
	// pending events, but give a generous budget for the scheduler).
	time.Sleep(100 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		store.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close() deadlocked after context cancellation")
	}
}

// ─── TestDrain_ConcurrentSendDuringClose ─────────────────────────────────────

// TestDrain_ConcurrentSendDuringClose verifies that calling OnServerEvent
// concurrently while Close() is in progress does not panic. Events may be
// dropped (channel full or drain already finished) but must never cause a
// panic or data race.
func TestDrain_ConcurrentSendDuringClose(t *testing.T) {
	mock := newMockConn()
	store := newTestStore(mock, 10)
	store.Start(context.Background())

	// Launch senders and Close concurrently.
	closeDone := make(chan struct{})
	go func() {
		store.Close()
		close(closeDone)
	}()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			store.OnServerEvent(makeServerEvent(n))
		}(i)
	}
	wg.Wait()

	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return within 5s during concurrent sends")
	}
	// No panics → test passes.
}

// ─── insertBeaconEvents atomicity (finding [13] / D-118) ──────────────────────

// countSends returns how many successful "send" entries the call log holds.
func countSends(log []string) int {
	n := 0
	for _, e := range log {
		if e == "send" {
			n++
		}
	}
	return n
}

// makeBeaconEventWithItems builds one BeaconEvent carrying n heartbeat items.
func makeBeaconEventWithItems(n int) domain.BeaconEvent {
	items := make([]domain.BeaconItem, n)
	for i := range items {
		items[i] = domain.BeaconItem{Type: "heartbeat", TS: time.Now().UnixMilli()}
	}
	return domain.BeaconEvent{
		Version:   1,
		SessionID: "sess-atomic",
		StreamID:  "stream-atomic",
		App:       "live",
		Events:    items,
	}
}

// TestInsertBeaconEvents_SingleBatchPerFlush proves the whole flush uses ONE
// PrepareBatch + ONE Send (mirroring insertServerEvents), not one per item. The
// previous per-item shape issued N Sends for an N-item flush; reverting the fix makes
// countSends() != 1 and reddens this test.
func TestInsertBeaconEvents_SingleBatchPerFlush(t *testing.T) {
	conn := newMockConn()
	store := newTestStore(conn, 100)
	batch := []domain.BeaconEvent{makeBeaconEventWithItems(3)}

	if err := store.insertBeaconEvents(context.Background(), batch); err != nil {
		t.Fatalf("insertBeaconEvents: unexpected error: %v", err)
	}
	if _, beaconRows, _ := conn.rowsByType(); beaconRows != 3 {
		t.Errorf("beaconRows: got %d, want 3", beaconRows)
	}
	if got := countSends(conn.callLogSnapshot()); got != 1 {
		t.Errorf("Send() calls: got %d, want 1 (one PrepareBatch+Send per flush, not one per item)", got)
	}
}

// TestInsertBeaconEvents_NeverPartiallyCommits proves atomicity: when a Send fails
// partway through a multi-item flush, ClickHouse ends up with ALL or NONE of the rows —
// never a partial subset. The previous per-item shape committed the items before the
// failing one (a partial commit), which this test catches.
func TestInsertBeaconEvents_NeverPartiallyCommits(t *testing.T) {
	// Fail the 2nd physical Send. With the fix there is only one Send (call 1), so the
	// flush succeeds atomically (all 3 rows). With the per-item bug there are 3 Sends:
	// item 0 commits (call 1), item 1's Send fails (call 2) → a partial 1-row commit.
	conn := newMockConn()
	conn.sendErr = fmt.Errorf("simulated send failure")
	conn.sendFailOnCall = 2
	store := newTestStore(conn, 100)
	batch := []domain.BeaconEvent{makeBeaconEventWithItems(3)}

	err := store.insertBeaconEvents(context.Background(), batch)
	_, beaconRows, _ := conn.rowsByType()

	if beaconRows != 0 && beaconRows != 3 {
		t.Errorf("partial commit: got %d beacon rows, want 0 or 3 (per-item PrepareBatch commits a subset before the failing Send)", beaconRows)
	}
	if err == nil && beaconRows != 3 {
		t.Errorf("err==nil but beaconRows=%d (want 3)", beaconRows)
	}
}

// TestInsertBeaconEvents_AtomicFailureCommitsNothing is a positive control for the
// fix's failure path: when the batch Send fails, zero rows are committed and the error
// propagates so the flusher (correctly) skips crediting s.inserted.
func TestInsertBeaconEvents_AtomicFailureCommitsNothing(t *testing.T) {
	conn := newMockConn()
	conn.sendErr = fmt.Errorf("simulated send failure")
	conn.sendFailOnCall = 1 // fail the very first Send
	store := newTestStore(conn, 100)
	batch := []domain.BeaconEvent{makeBeaconEventWithItems(3)}

	if err := store.insertBeaconEvents(context.Background(), batch); err == nil {
		t.Fatal("insertBeaconEvents: expected error on Send failure, got nil")
	}
	if _, beaconRows, _ := conn.rowsByType(); beaconRows != 0 {
		t.Errorf("beaconRows after failed flush: got %d, want 0 (nothing committed)", beaconRows)
	}
}
