package license

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestMintPath_LicensegenToEntitlements is the end-to-end pin the earlier
// regressions lacked (REVIEW-MP3, Issue-F residue): it runs the REAL
// qa/licensegen tool for every tier, feeds the minted key through the real
// license verification, and asserts the resulting server-side entitlements
// match the published ladder (Free 1 node/7 d · Pro 10/90 d · Business 50/396 d ·
// Enterprise unlimited). The licensegen-side unit test only base64-decodes the
// claims it minted — a claims-vs-parser drift (exactly the shape of the
// Business 5-node mint bug, D-166/D-173) passes that test and fails this one.
func TestMintPath_LicensegenToEntitlements(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not in PATH — mint-path e2e needs `go run` for qa/licensegen")
	}

	// Locate qa/licensegen relative to this source file (repo layout is fixed).
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	licensegenDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "qa", "licensegen")
	if _, err := os.Stat(filepath.Join(licensegenDir, "main.go")); err != nil {
		t.Skipf("qa/licensegen not present at %s: %v", licensegenDir, err)
	}

	cases := []struct {
		tier          string
		wantNodes     int
		wantRetention int
		wantDataAPI   bool
	}{
		{"free", 1, 7, false},
		{"pro", 10, 90, true},
		{"business", 50, 396, true},
		{"enterprise", -1, -1, true}, // unlimited
	}

	for _, tc := range cases {
		t.Run(tc.tier, func(t *testing.T) {
			cmd := exec.Command(goBin, "run", ".", "-tier", tc.tier)
			cmd.Dir = licensegenDir
			cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
			out, err := cmd.Output()
			if err != nil {
				stderr := ""
				if ee, ok := err.(*exec.ExitError); ok {
					stderr = string(ee.Stderr)
				}
				t.Fatalf("licensegen -tier %s failed: %v\n%s", tc.tier, err, stderr)
			}

			// stdout contract: exactly two GITHUB_ENV lines.
			var key, pubkey string
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				switch {
				case strings.HasPrefix(line, "PULSE_LICENSE_KEY="):
					key = strings.TrimPrefix(line, "PULSE_LICENSE_KEY=")
				case strings.HasPrefix(line, "PULSE_LICENSE_PUBKEY="):
					pubkey = strings.TrimPrefix(line, "PULSE_LICENSE_PUBKEY=")
				}
			}
			if key == "" || pubkey == "" {
				t.Fatalf("licensegen stdout missing env lines; got:\n%s", out)
			}

			t.Setenv("PULSE_LICENSE_PUBKEY", pubkey)
			m, err := New(key, "")
			if err != nil {
				t.Fatalf("license.New on minted %s key: %v", tc.tier, err)
			}
			if got := string(m.Tier()); got != tc.tier {
				t.Fatalf("server resolved tier %q from a %q mint (activation failed → fell open to free?)", got, tc.tier)
			}
			e := m.Entitlements()
			if e.MaxNodes != tc.wantNodes {
				t.Errorf("%s: server MaxNodes = %d, want %d (published ladder)", tc.tier, e.MaxNodes, tc.wantNodes)
			}
			if e.RetentionDays != tc.wantRetention {
				t.Errorf("%s: server RetentionDays = %d, want %d (published ladder)", tc.tier, e.RetentionDays, tc.wantRetention)
			}
			if e.DataAPI != tc.wantDataAPI {
				t.Errorf("%s: server DataAPI = %v, want %v", tc.tier, e.DataAPI, tc.wantDataAPI)
			}
		})
	}
}
