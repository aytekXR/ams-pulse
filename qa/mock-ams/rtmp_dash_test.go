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

// rtmpDoHandshake performs the C0+C1 → S0+S1+S2 → C2 RTMP handshake on conn.
// Returns c1 bytes (needed to verify S2 echo) and s1 bytes (needed for C2).
// Fatals on any error.
func rtmpDoHandshake(t *testing.T, conn net.Conn) (c1, s1 [1536]byte) {
	t.Helper()

	const c1Ts uint32 = 0xDEADBEEF
	binary.BigEndian.PutUint32(c1[0:4], c1Ts)
	for i := 8; i < 1536; i++ {
		c1[i] = byte(i % 251) // non-trivial pattern, prime modulus
	}

	if _, err := conn.Write([]byte{0x03}); err != nil {
		t.Fatalf("write C0: %v", err)
	}
	if _, err := conn.Write(c1[:]); err != nil {
		t.Fatalf("write C1: %v", err)
	}

	var s0 [1]byte
	if _, err := io.ReadFull(conn, s0[:]); err != nil {
		t.Fatalf("read S0: %v", err)
	}
	if s0[0] != 0x03 {
		t.Errorf("S0 = 0x%02x, want 0x03", s0[0])
	}

	if _, err := io.ReadFull(conn, s1[:]); err != nil {
		t.Fatalf("read S1: %v", err)
	}

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

	var c2 [1536]byte
	copy(c2[0:4], s1[0:4])
	binary.BigEndian.PutUint32(c2[4:8], uint32(time.Now().UnixMilli()))
	copy(c2[8:1536], s1[8:1536])
	if _, err := conn.Write(c2[:]); err != nil {
		t.Fatalf("write C2: %v", err)
	}
	return c1, s1
}

// sendAMF0ConnectTest builds and sends a minimal AMF0 connect chunk with the
// given app name (fmt=0, csid=3, type=0x14, txid=1.0).
func sendAMF0ConnectTest(t *testing.T, conn net.Conn, app string) {
	t.Helper()
	tcURL := "rtmp://" + conn.RemoteAddr().String() + "/" + app
	var payload []byte
	// "connect" string
	payload = append(payload, 0x02, 0x00, 0x07)
	payload = append(payload, []byte("connect")...)
	// txid number 1.0
	payload = append(payload, 0x00, 0x3F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
	// object start
	payload = append(payload, 0x03)
	appendProp := func(key string, val string) {
		payload = append(payload, byte(len(key)>>8), byte(len(key)))
		payload = append(payload, []byte(key)...)
		payload = append(payload, 0x02, byte(len(val)>>8), byte(len(val)))
		payload = append(payload, []byte(val)...)
	}
	appendProp("app", app)
	appendProp("tcUrl", tcURL)
	appendProp("flashVer", "FMLE/3.0")
	payload = append(payload, 0x00, 0x00, 0x09) // object end

	msgLen := len(payload)
	chunk := make([]byte, 1+11+msgLen)
	chunk[0] = 0x03
	chunk[4] = byte(msgLen >> 16)
	chunk[5] = byte(msgLen >> 8)
	chunk[6] = byte(msgLen)
	chunk[7] = 0x14
	copy(chunk[12:], payload)
	if _, err := conn.Write(chunk); err != nil {
		t.Fatalf("write AMF0 connect: %v", err)
	}
}

// readAMF0CmdTest reads one AMF0 command chunk from conn and returns the command name.
func readAMF0CmdTest(t *testing.T, conn net.Conn) string {
	t.Helper()
	var bh [1]byte
	if _, err := io.ReadFull(conn, bh[:]); err != nil {
		t.Fatalf("read basic header: %v", err)
	}
	var mh [11]byte
	if _, err := io.ReadFull(conn, mh[:]); err != nil {
		t.Fatalf("read message header: %v", err)
	}
	msgLen := int(mh[3])<<16 | int(mh[4])<<8 | int(mh[5])
	payload := make([]byte, msgLen)
	if _, err := io.ReadFull(conn, payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	if len(payload) < 3 || payload[0] != 0x02 {
		t.Fatalf("expected AMF0 string type 0x02, got 0x%02x", payload[0])
	}
	slen := int(payload[1])<<8 | int(payload[2])
	if 3+slen > len(payload) {
		t.Fatalf("AMF0 string length %d overruns payload", slen)
	}
	return string(payload[3 : 3+slen])
}

// TestRTMPHandshake_HappyPath performs the full C0/C1 → S0/S1/S2 → C2 TCP exchange
// followed by an AMF0 connect command and verifies the mock returns _result.
// Verifies:
//   - S0 == 0x03
//   - S2 correctly echoes C1 (C1 timestamp in S2[0:4], C1 random in S2[8:1536])
//   - Server replies with AMF0 _result after connect command.
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

	rtmpDoHandshake(t, conn)

	// Send AMF0 connect with app "live".
	sendAMF0ConnectTest(t, conn, "live")

	// Server should reply with AMF0 _result.
	cmdName := readAMF0CmdTest(t, conn)
	if cmdName != "_result" {
		t.Errorf("expected _result from mock, got %q", cmdName)
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

// TestRTMPConnect_AppAccepted performs a full RTMP handshake + AMF0 connect
// with app="live" against the mock and verifies the mock returns _result.
func TestRTMPConnect_AppAccepted(t *testing.T) {
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

	rtmpDoHandshake(t, conn)
	sendAMF0ConnectTest(t, conn, "live")

	cmdName := readAMF0CmdTest(t, conn)
	if cmdName != "_result" {
		t.Errorf("expected _result from mock for app=live, got %q", cmdName)
	}
}

// TestRTMPConnect_AppRejected performs a full RTMP handshake + AMF0 connect
// with app="rejected" and verifies the mock returns _error (rejection hook).
func TestRTMPConnect_AppRejected(t *testing.T) {
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

	rtmpDoHandshake(t, conn)
	sendAMF0ConnectTest(t, conn, "rejected")

	cmdName := readAMF0CmdTest(t, conn)
	if cmdName != "_error" {
		t.Errorf("expected _error from mock for app=rejected, got %q", cmdName)
	}
}
