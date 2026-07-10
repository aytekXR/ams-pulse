// export_test.go — exposes unexported package-level test hooks to the
// external test package (prober_test).  This file is compiled ONLY during
// test builds (Go's _test.go convention) so it does not ship in production.
package prober

import "time"

// SetTestRTPStatsHoldOverride overrides the RTP stats hold duration used in
// continueWebRTCICE.  Pass 0 to restore the production constant.
//
// The store is atomic, so the write never races with a probe goroutine's
// read; tests using it should still avoid t.Parallel() so the override cannot
// leak into a concurrently running probe test, and must restore the zero
// value via t.Cleanup.
func SetTestRTPStatsHoldOverride(d time.Duration) {
	testRTPStatsHoldOverride.Store(int64(d))
}
