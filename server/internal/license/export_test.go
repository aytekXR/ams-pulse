// export_test.go — test-only hooks exported for use in *_test.go files of the
// license package. Not compiled in production builds (file ends in _test.go).
package license

import "time"

// SetNow replaces the package-level clock function used in expiry checks.
// Call t.Cleanup(func() { license.SetNow(time.Now) }) to restore after the test.
func SetNow(f func() time.Time) { now = f }
