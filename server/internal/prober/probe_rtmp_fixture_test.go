// probe_rtmp_fixture_test.go — replays captured AMS RTMP connect bytes through
// the chunk demuxer to guard against regressions.
//
// Provenance: AMS 3.0.3 Enterprise build 20260504_1443, captured 2026-07-14
// against rtmp://127.0.0.1:1935/LiveApp.  The fixture contains the raw bytes
// received from AMS after C2 was written (281 bytes; byte-parse verified S29):
//   - WindowAckSize (type 0x05, csid=2, fmt=0, 10,000,000)
//   - SetPeerBandwidth (type 0x06, csid=2, fmt=1, 10,000,000 Dynamic)
//   - UserControl/StreamBegin (type 0x04, csid=2, fmt=1, stream 0)
//   - AMF0 _result command (type 0x14, csid=3, fmt=0, 225 bytes fragmented at
//     the 128-byte default: 128 + fmt=3 continuation of 97)
//
// NOTE: AMS does NOT renegotiate chunk size in this exchange — the demuxer's
// SetChunkSize handling is pinned separately by
// TestReadAMF0Command_HonorsSetChunkSize (probe_rtmp_test.go).
//
// The test does NOT require network access.
package prober

import (
	"bytes"
	"os"
	"testing"
)

// TestProbeRTMP_FixtureReplay verifies that readAMF0Command correctly demuxes
// the captured AMS RTMP response and returns "_result".
func TestProbeRTMP_FixtureReplay(t *testing.T) {
	data, err := os.ReadFile("testdata/ams-connect-response.bin")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("fixture is empty")
	}

	t.Logf("replaying %d fixture bytes (AMS 3.0.3 Enterprise 20260504_1443)", len(data))

	cmdName, err := readAMF0Command(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("readAMF0Command on fixture: %v", err)
	}
	if cmdName != "_result" {
		t.Errorf("expected _result from AMS fixture, got %q", cmdName)
	}
	t.Logf("fixture replay: cmdName=%q OK", cmdName)
}
