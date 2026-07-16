// S66 (D-128) — the RTMP chunk demuxer must bound the number of distinct
// chunk-stream-ID (CSID) states it tracks. A hostile server that passes the
// handshake can open all 65,536 3-byte-form CSIDs, each buffering up to
// rtmpMaxMsgSize (64 KiB) → ~4 GB of heap within the probe deadline, OOM-killing
// the prober (S62 [13]). readAMF0Command now refuses past maxCSIDStates.
//
// Mutation proof: remove the `len(states) >= maxCSIDStates` guard → the demuxer
// keeps allocating states, consumes all the crafted chunks, and returns a
// basic-header EOF error instead of "too many chunk streams" → this test reddens.
package prober

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// rtmpZeroLenChunk writes one RTMP chunk with a 3-byte-form basic header (a
// distinct CSID = 64+i) and a fmt=0 message header declaring length=0 and a
// skipped message type (0x05 WindowAckSize). A zero-length skipped message
// completes immediately, so the demuxer loops on to the next CSID.
func rtmpZeroLenChunk(buf *bytes.Buffer, i int) {
	buf.Write([]byte{0x01, byte(i & 0xFF), byte(i >> 8)}) // basic header: fmt=0, 3-byte CSID form
	buf.Write([]byte{0, 0, 0})                            // timestamp
	buf.Write([]byte{0, 0, 0})                            // message length = 0
	buf.WriteByte(0x05)                                   // type_id = WindowAckSize (skipped)
	buf.Write([]byte{0, 0, 0, 0})                         // message stream id
}

func TestReadAMF0Command_CapsCSIDStates_S66(t *testing.T) {
	var buf bytes.Buffer
	// One more distinct CSID than the cap allows.
	for i := 0; i <= maxCSIDStates; i++ {
		rtmpZeroLenChunk(&buf, i)
	}
	_, err := readAMF0Command(&buf)
	if err == nil {
		t.Fatal("expected an error once distinct CSID states exceed the cap, got nil")
	}
	if !strings.Contains(err.Error(), "too many chunk streams") {
		t.Fatalf("want 'too many chunk streams' cap error, got: %v", err)
	}
}

// Positive control: a handful of distinct CSIDs (well under the cap) is fine —
// the demuxer processes the skipped messages and then errors only on EOF (a
// clean end of stream), never on the cap.
func TestReadAMF0Command_UnderCap_NoCapError_S66(t *testing.T) {
	var buf bytes.Buffer
	for i := 0; i < 8; i++ {
		rtmpZeroLenChunk(&buf, i)
	}
	_, err := readAMF0Command(&buf)
	if err == nil {
		t.Fatal("expected an EOF error at end of stream (no AMF0 command present)")
	}
	if strings.Contains(err.Error(), "too many chunk streams") {
		t.Fatalf("cap must not fire under the limit; got: %v", err)
	}
}

// rtmpMsg writes a complete fmt=0 message on the given 1-byte CSID and type,
// with a payload of payloadLen zero bytes. Assumes chunkSize >= payloadLen so the
// whole payload fits in one chunk.
func rtmpMsg(buf *bytes.Buffer, csid, typeID byte, payloadLen int) {
	buf.WriteByte(csid & 0x3F)                                                         // basic header: fmt=0, 1-byte CSID form (2..63)
	buf.Write([]byte{0, 0, 0})                                                         // timestamp
	buf.Write([]byte{byte(payloadLen >> 16), byte(payloadLen >> 8), byte(payloadLen)}) // length
	buf.WriteByte(typeID)                                                              // message type
	buf.Write([]byte{0, 0, 0, 0})                                                      // message stream id
	buf.Write(make([]byte, payloadLen))                                                // payload
}

// TestReadAMF0Command_SkippedTypesNoLargeCopy_S66 proves the demuxer no longer
// copies every completed message before the type dispatch. A hostile server can
// raise chunkSize to 64 KiB then stream large silently-skipped messages; the old
// code allocated a fresh len(payload) copy per message (sustained GC pressure)
// despite the CSID-state cap (found by the S66 adversarial review). The fix reads
// the buffer in place, so total bytes allocated stay near one message's worth.
func TestReadAMF0Command_SkippedTypesNoLargeCopy_S66(t *testing.T) {
	const n = 64
	const payloadLen = 60000 // < rtmpMaxMsgSize; fits one chunk once chunkSize is raised
	var data bytes.Buffer
	// SetChunkSize(65536) on csid=2 so each large message below is a single chunk.
	rtmpMsg(&data, 2, 0x01, 4)
	// overwrite the 4-byte SetChunkSize payload (last 4 bytes) with 65536.
	raw := data.Bytes()
	copy(raw[len(raw)-4:], []byte{0x00, 0x01, 0x00, 0x00})
	// n large skipped (WindowAckSize) messages on csid=3.
	for i := 0; i < n; i++ {
		rtmpMsg(&data, 3, 0x05, payloadLen)
	}
	raw = data.Bytes()

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)
	_, _ = readAMF0Command(bytes.NewReader(raw))
	runtime.ReadMemStats(&m2)
	allocated := m2.TotalAlloc - m1.TotalAlloc

	// Old code copied each 60000-byte message → ~n*60000 ≈ 3.8 MB. The fix reuses
	// the accumulation buffer → a few message-worths of headroom is plenty.
	const budget = 8 * payloadLen // ~480 KB, vs ~3.8 MB for the per-message copy
	if allocated > uint64(budget) {
		t.Fatalf("large per-message copy not eliminated: %d bytes allocated for %d skipped messages (want < %d)", allocated, n, budget)
	}
}
