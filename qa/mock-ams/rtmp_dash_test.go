package main

import (
	"encoding/binary"
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ─── DASH MPD + Segment tests ─────────────────────────────────────────────────

// TestDASHMPD_HappyPath verifies GET /{app}/streams/{streamId}.mpd returns 200,
// Content-Type application/dash+xml, valid XML root element MPD, a SegmentTemplate
// element, timescale="90000" (non-1), and a media attribute embedding the streamId.
func TestDASHMPD_HappyPath(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/live/streams/test-stream.mpd")
	if err != nil {
		t.Fatalf("GET /live/streams/test-stream.mpd: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/dash+xml") {
		t.Errorf("want Content-Type application/dash+xml, got %q", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	// Must parse as valid XML with root element MPD.
	var doc struct {
		XMLName xml.Name `xml:"MPD"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("body is not valid XML MPD: %v\nbody: %s", err, body)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "SegmentTemplate") {
		t.Error("MPD does not contain SegmentTemplate element")
	}
	if !strings.Contains(bodyStr, `timescale="90000"`) {
		t.Errorf("MPD SegmentTemplate does not use timescale=90000; body:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, "test-stream-seg-$Number$.m4s") {
		t.Errorf("MPD media attribute does not embed streamId; body:\n%s", bodyStr)
	}
}

// TestDASHSegment_HappyPath verifies GET /{app}/streams/{streamId}-seg-1.m4s returns
// 200, Content-Type video/iso.segment, and exactly 50000 bytes.
func TestDASHSegment_HappyPath(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/live/streams/test-stream-seg-1.m4s")
	if err != nil {
		t.Fatalf("GET /live/streams/test-stream-seg-1.m4s: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "video/iso.segment") {
		t.Errorf("want Content-Type video/iso.segment, got %q", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) != 50000 {
		t.Errorf("want exactly 50000 bytes, got %d", len(body))
	}
}

// TestDASHMPD_UnknownStream verifies that the MPD route returns 200 for any
// streamId (consistent with existing mock conventions that do not validate stream existence).
func TestDASHMPD_UnknownStream(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/live/streams/nonexistent-stream.mpd")
	if err != nil {
		t.Fatalf("GET /live/streams/nonexistent-stream.mpd: %v", err)
	}
	defer resp.Body.Close()

	// Unknown streams still return 200 — consistent with existing handlers that
	// return empty data rather than 404 for unregistered stream IDs.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for unknown stream (consistent with existing mock conventions), got %d", resp.StatusCode)
	}
}

// ─── RTMP Handshake tests ─────────────────────────────────────────────────────

// TestRTMPHandshake_HappyPath performs the full C0/C1 → S0/S1/S2 → C2 TCP exchange
// against a real net.Listener on an ephemeral port. Verifies:
//   - S0 == 0x03
//   - S2 correctly echoes C1 (C1 timestamp in S2[0:4], C1 random in S2[8:1536])
//   - Server closes the connection after reading C2.
func TestRTMPHandshake_HappyPath(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer ln.Close()
	go serveRTMPOnListener(ln)

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}

	// Build C1 with a known timestamp and recognisable random payload.
	const c1Ts uint32 = 0xDEADBEEF
	var c1 [1536]byte
	binary.BigEndian.PutUint32(c1[0:4], c1Ts)
	// c1[4:8] = 0 (already)
	for i := 8; i < 1536; i++ {
		c1[i] = byte(i % 251) // non-trivial pattern, prime modulus
	}

	// Send C0 + C1.
	if _, err := conn.Write([]byte{0x03}); err != nil {
		t.Fatalf("write C0: %v", err)
	}
	if _, err := conn.Write(c1[:]); err != nil {
		t.Fatalf("write C1: %v", err)
	}

	// Read S0.
	var s0 [1]byte
	if _, err := io.ReadFull(conn, s0[:]); err != nil {
		t.Fatalf("read S0: %v", err)
	}
	if s0[0] != 0x03 {
		t.Errorf("S0 = 0x%02x, want 0x03", s0[0])
	}

	// Read S1 (1536 bytes) — we need it to build a proper C2 reply.
	var s1 [1536]byte
	if _, err := io.ReadFull(conn, s1[:]); err != nil {
		t.Fatalf("read S1: %v", err)
	}

	// Read S2 (1536 bytes) and verify it echoes C1.
	var s2 [1536]byte
	if _, err := io.ReadFull(conn, s2[:]); err != nil {
		t.Fatalf("read S2: %v", err)
	}
	s2Ts := binary.BigEndian.Uint32(s2[0:4])
	if s2Ts != c1Ts {
		t.Errorf("S2 timestamp = 0x%08x, want C1 timestamp 0x%08x", s2Ts, c1Ts)
	}
	for i := 8; i < 1536; i++ {
		if s2[i] != c1[i] {
			t.Errorf("S2[%d] = 0x%02x, want C1[%d] = 0x%02x (first mismatch)", i, s2[i], i, c1[i])
			break
		}
	}

	// Send C2: echo of S1 (S1 timestamp + ack time + S1 random).
	var c2 [1536]byte
	copy(c2[0:4], s1[0:4])
	binary.BigEndian.PutUint32(c2[4:8], uint32(time.Now().UnixMilli()))
	copy(c2[8:1536], s1[8:1536])
	if _, err := conn.Write(c2[:]); err != nil {
		t.Fatalf("write C2: %v", err)
	}

	// Server should close after reading C2; a subsequent read must return an error.
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetDeadline for close check: %v", err)
	}
	var buf [1]byte
	if _, readErr := conn.Read(buf[:]); readErr == nil {
		t.Error("expected server to close connection after C2, but read returned data")
	}
}

// TestRTMPHandshake_BadVersion verifies that a client sending C0 version byte 0x02
// (not 0x03) causes the server to close the connection without sending S0.
func TestRTMPHandshake_BadVersion(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer ln.Close()
	go serveRTMPOnListener(ln)

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}

	// Send bad version byte (0x02 instead of 0x03).
	if _, err := conn.Write([]byte{0x02}); err != nil {
		t.Fatalf("write bad C0: %v", err)
	}

	// Server must close without sending S0; any read should return a non-nil error.
	var buf [1]byte
	if _, readErr := conn.Read(buf[:]); readErr == nil {
		t.Error("expected server to close on bad version byte, but read returned data")
	}
}
