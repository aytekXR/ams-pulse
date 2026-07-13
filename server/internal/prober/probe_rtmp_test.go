// Tests for probeRTMP.
// Internal test package (package prober) so the unexported probeRTMP method is
// accessible without going through the scheduler dispatch (wiring is added by a
// separate wiring author in the "rtmp" case of executeProbe).
package prober

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// ─── Test helpers for AMF0 chunk exchange ─────────────────────────────────────

// readRTMPMsgTest reads one complete RTMP message from conn (test-side helper).
// Handles fmt=0 first chunk and fmt=3 continuations at 128-byte boundaries.
// Returns msgType and the full payload.  Returns ("", nil, nil) on EOF.
func readRTMPMsgTest(conn net.Conn) (msgType byte, payload []byte, err error) {
	// Read basic header
	var bh [1]byte
	if _, err := io.ReadFull(conn, bh[:]); err != nil {
		return 0, nil, err // may be EOF if prober already closed
	}
	fmt0 := (bh[0] >> 6) & 0x03
	if fmt0 != 0 {
		return 0, nil, fmt.Errorf("readRTMPMsgTest: expected fmt=0 for first chunk, got %d", fmt0)
	}
	// 11-byte message header (fmt=0)
	var mh [11]byte
	if _, err := io.ReadFull(conn, mh[:]); err != nil {
		return 0, nil, fmt.Errorf("readRTMPMsgTest: message header: %v", err)
	}
	msgLen := int(mh[3])<<16 | int(mh[4])<<8 | int(mh[5])
	msgType = mh[6]

	payload = make([]byte, 0, msgLen)
	rem := msgLen
	firstChunk := true
	for rem > 0 {
		if !firstChunk {
			// Continuation chunk basic header (fmt=3)
			var cb [1]byte
			if _, err := io.ReadFull(conn, cb[:]); err != nil {
				return 0, nil, fmt.Errorf("readRTMPMsgTest: continuation header: %v", err)
			}
		}
		firstChunk = false
		n := 128
		if n > rem {
			n = rem
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return 0, nil, fmt.Errorf("readRTMPMsgTest: payload read: %v", err)
		}
		payload = append(payload, buf...)
		rem -= n
	}
	return msgType, payload, nil
}

// sendAMF0CmdChunkTest sends a minimal AMF0 command chunk (fmt=0, csid=3, type=0x14).
// Payload: command name string + txid number (1.0) + null (info placeholder).
// Fits in a single 128-byte chunk for any realistic command name.
func sendAMF0CmdChunkTest(conn net.Conn, cmdName string) error {
	var payload []byte
	// AMF0 string: type 0x02 + uint16 length + bytes
	payload = append(payload, 0x02, byte(len(cmdName)>>8), byte(len(cmdName)))
	payload = append(payload, []byte(cmdName)...)
	// AMF0 number: type 0x00 + 8-byte IEEE 754 BE (1.0 = 0x3FF0_0000_0000_0000)
	payload = append(payload, 0x00, 0x3F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
	// AMF0 null
	payload = append(payload, 0x05)

	msgLen := len(payload)
	// Single chunk: fmt=0, csid=3 → basic header byte 0x03
	chunk := make([]byte, 1+11+msgLen)
	chunk[0] = 0x03
	// timestamp = 0
	chunk[1], chunk[2], chunk[3] = 0, 0, 0
	// length
	chunk[4] = byte(msgLen >> 16)
	chunk[5] = byte(msgLen >> 8)
	chunk[6] = byte(msgLen)
	// type_id = 0x14 (AMF0 command)
	chunk[7] = 0x14
	// msg_stream_id = 0 (LE)
	chunk[8], chunk[9], chunk[10], chunk[11] = 0, 0, 0, 0
	copy(chunk[12:], payload)

	if _, err := conn.Write(chunk); err != nil {
		return fmt.Errorf("sendAMF0CmdChunkTest write: %v", err)
	}
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// rtmpTestRunner returns a minimal Runner sufficient for probeRTMP unit tests.
// probeRTMP does not use r.cfg, r.store, r.source, r.logger, or r.client.
func rtmpTestRunner() *Runner {
	return &Runner{}
}

// rtmpConfig returns a ProbeConfig for the given rtmp:// URL.
func rtmpConfig(url string) domain.ProbeConfig {
	return domain.ProbeConfig{
		ID:        "test-probe-id",
		Name:      "test-rtmp",
		URL:       url,
		Protocol:  "rtmp",
		IntervalS: 60,
		TimeoutS:  5,
	}
}

// rtmpResult returns an initial ProbeResult for direct probeRTMP calls.
func rtmpResult() domain.ProbeResult {
	return domain.ProbeResult{
		ID:      "test-result-id",
		ProbeID: "test-probe-id",
		TS:      time.Now().UTC(),
	}
}

// rtmpListener wraps a TCP listener and a completion channel for one-shot servers.
type rtmpListener struct {
	ln   net.Listener
	done chan error
}

// newRTMPListener starts a TCP listener on 127.0.0.1:0.
func newRTMPListener(t *testing.T) *rtmpListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return &rtmpListener{ln: ln, done: make(chan error, 1)}
}

// url returns an rtmp:// URL pointing at the listener, with the given app path.
func (l *rtmpListener) url(app string) string {
	return "rtmp://" + l.ln.Addr().String() + "/" + app
}

// close shuts down the listener.
func (l *rtmpListener) close() {
	l.ln.Close()
}

// serveOnce accepts one connection, runs fn(conn), and sends the result to done.
// The accepted conn is closed via defer after fn returns.
func (l *rtmpListener) serveOnce(fn func(conn net.Conn) error) {
	go func() {
		conn, err := l.ln.Accept()
		if err != nil {
			l.done <- err
			return
		}
		defer conn.Close()
		l.done <- fn(conn)
	}()
}

// wait checks that the server goroutine completed without error within 5 seconds.
func (l *rtmpListener) wait(t *testing.T) {
	t.Helper()
	select {
	case err := <-l.done:
		if err != nil {
			t.Errorf("server goroutine error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("server goroutine timed out waiting to finish")
	}
}

// readC0C1 reads C0 (1 byte, must be 0x03) + C1 (1536 bytes) from conn.
// Returns the 1536 C1 bytes or an error.
func readC0C1(conn net.Conn) (c1 []byte, err error) {
	buf := make([]byte, 1537)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, fmt.Errorf("read C0+C1: %v", err)
	}
	if buf[0] != 0x03 {
		return nil, fmt.Errorf("C0 version mismatch: got 0x%02x want 0x03", buf[0])
	}
	return buf[1:], nil
}

// sendS0S1S2 sends S0 + S1 + S2 (3073 bytes total) on conn.
//
//   - s0Version: byte sent as S0; use 0x03 for valid, anything else to test bad-version.
//   - c1: the C1 bytes received from the client (used to construct a correct S2 echo).
//   - corruptS2: if true, fills S2 with 0xFF bytes instead of echoing C1.
func sendS0S1S2(conn net.Conn, s0Version byte, c1 []byte, corruptS2 bool) error {
	const chunkLen = 1536
	out := make([]byte, 1+chunkLen+chunkLen)
	out[0] = s0Version

	// S1: minimal — just set timestamp at [0:4]; rest zeros (probe doesn't validate S1).
	s1 := out[1 : 1+chunkLen]
	binary.BigEndian.PutUint32(s1[0:4], uint32(time.Now().UnixMilli()))

	// S2: echo of C1 (timestamp at [0:4], time2 at [4:8], C1 random at [8:]).
	s2 := out[1+chunkLen:]
	if corruptS2 {
		for i := range s2 {
			s2[i] = 0xFF // clearly not a C1 echo
		}
	} else {
		copy(s2[0:4], c1[0:4])                                              // C1 timestamp
		binary.BigEndian.PutUint32(s2[4:8], uint32(time.Now().UnixMilli())) // time2
		copy(s2[8:], c1[8:])                                                // C1 random
	}

	if _, err := conn.Write(out); err != nil {
		return fmt.Errorf("write S0+S1+S2: %v", err)
	}
	return nil
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestProbeRTMP_Success verifies the full C0+C1 → S0+S1+S2 → C2 → AMF0 connect
// → _result happy path.  The mock server reads the connect command and replies
// with AMF0 _result, so the prober should return SignalingState="app_accepted".
// ConnectTimeMs is widened to dial→_result parsed (≥1 ms).
func TestProbeRTMP_Success(t *testing.T) {
	srv := newRTMPListener(t)
	defer srv.close()

	srv.serveOnce(func(conn net.Conn) error {
		conn.SetDeadline(time.Now().Add(5 * time.Second))

		c1, err := readC0C1(conn)
		if err != nil {
			return err
		}
		if err := sendS0S1S2(conn, 0x03, c1, false); err != nil {
			return err
		}
		// Read C2 (1536 bytes).
		c2 := make([]byte, 1536)
		if _, err := io.ReadFull(conn, c2); err != nil {
			return fmt.Errorf("read C2: %v", err)
		}
		// Read AMF0 connect command from probe.
		// Ignore error: before the implementation lands, the prober closes here
		// and the read returns EOF — the RED failure is in the assertion below.
		_, _, _ = readRTMPMsgTest(conn)
		// Send _result.
		return sendAMF0CmdChunkTest(conn, "_result")
	})

	r := rtmpTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := r.probeRTMP(ctx, rtmpConfig(srv.url("live")), rtmpResult())
	srv.wait(t)

	t.Logf("app-accepted result: success=%v code=%q state=%q connect_ms=%v msg=%q",
		result.Success, result.ErrorCode, result.SignalingState, result.ConnectTimeMs, result.ErrorMsg)

	if !result.Success {
		t.Fatalf("expected Success=true, got false: code=%q msg=%q",
			result.ErrorCode, result.ErrorMsg)
	}
	if result.ConnectTimeMs == nil {
		t.Fatal("expected ConnectTimeMs != nil on success")
	}
	if *result.ConnectTimeMs < 1 {
		t.Errorf("expected ConnectTimeMs >= 1 ms (1ms floor), got %d", *result.ConnectTimeMs)
	}
	// RED assertion: expects "app_accepted" (fails against current impl which returns "handshake_complete").
	if result.SignalingState != "app_accepted" {
		t.Errorf("expected SignalingState=app_accepted, got %q", result.SignalingState)
	}
	if result.ErrorCode != "" {
		t.Errorf("expected empty ErrorCode on success, got %q", result.ErrorCode)
	}
	if result.ErrorMsg != "" {
		t.Errorf("expected empty ErrorMsg on success, got %q", result.ErrorMsg)
	}
}

// TestProbeRTMP_NoAppSegment pins the legacy behavior: when the RTMP URL has no
// app path segment, the probe succeeds at handshake completion and returns
// SignalingState="handshake_complete" without sending an AMF0 connect command.
// Backward-compatibility test — must stay GREEN before and after implementation.
func TestProbeRTMP_NoAppSegment(t *testing.T) {
	srv := newRTMPListener(t)
	defer srv.close()

	srv.serveOnce(func(conn net.Conn) error {
		conn.SetDeadline(time.Now().Add(5 * time.Second))

		c1, err := readC0C1(conn)
		if err != nil {
			return err
		}
		if err := sendS0S1S2(conn, 0x03, c1, false); err != nil {
			return err
		}
		// Read C2 best-effort (probe may or may not send it before closing).
		c2 := make([]byte, 1536)
		_, _ = io.ReadFull(conn, c2)
		// No AMF0 exchange — legacy path.
		return nil
	})

	r := rtmpTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// URL with no app path segment — triggers legacy handshake-only path.
	noAppURL := "rtmp://" + srv.ln.Addr().String()
	result := r.probeRTMP(ctx, rtmpConfig(noAppURL), rtmpResult())
	srv.wait(t)

	t.Logf("no-app result: success=%v code=%q state=%q connect_ms=%v",
		result.Success, result.ErrorCode, result.SignalingState, result.ConnectTimeMs)

	if !result.Success {
		t.Fatalf("expected Success=true on no-app path, got false: code=%q", result.ErrorCode)
	}
	if result.ConnectTimeMs == nil {
		t.Fatal("expected ConnectTimeMs != nil on handshake success")
	}
	if *result.ConnectTimeMs < 1 {
		t.Errorf("expected ConnectTimeMs >= 1 ms, got %d", *result.ConnectTimeMs)
	}
	if result.SignalingState != "handshake_complete" {
		t.Errorf("expected SignalingState=handshake_complete (legacy no-app path), got %q", result.SignalingState)
	}
}

// TestProbeRTMP_AppRejected verifies that an AMF0 _error response maps to
// success=false, ErrorCode="rtmp_app_rejected", SignalingState="app_rejected",
// and ConnectTimeMs is still recorded (a rejection is a valid timing sample).
func TestProbeRTMP_AppRejected(t *testing.T) {
	srv := newRTMPListener(t)
	defer srv.close()

	srv.serveOnce(func(conn net.Conn) error {
		conn.SetDeadline(time.Now().Add(5 * time.Second))

		c1, err := readC0C1(conn)
		if err != nil {
			return err
		}
		if err := sendS0S1S2(conn, 0x03, c1, false); err != nil {
			return err
		}
		c2 := make([]byte, 1536)
		if _, err := io.ReadFull(conn, c2); err != nil {
			return fmt.Errorf("read C2: %v", err)
		}
		// Read connect command (ignore error in RED: prober doesn't send it yet).
		_, _, _ = readRTMPMsgTest(conn)
		// Send _error — app "rejected" triggers this path.
		return sendAMF0CmdChunkTest(conn, "_error")
	})

	r := rtmpTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := r.probeRTMP(ctx, rtmpConfig(srv.url("rejected")), rtmpResult())
	srv.wait(t)

	t.Logf("app-rejected result: success=%v code=%q state=%q connect_ms=%v msg=%q",
		result.Success, result.ErrorCode, result.SignalingState, result.ConnectTimeMs, result.ErrorMsg)

	// RED: current impl returns success=true; after implementation: success=false.
	if result.Success {
		t.Error("expected Success=false on _error response")
	}
	if result.ErrorCode != "rtmp_app_rejected" {
		t.Errorf("expected error_code=rtmp_app_rejected, got %q", result.ErrorCode)
	}
	if result.SignalingState != "app_rejected" {
		t.Errorf("expected SignalingState=app_rejected, got %q", result.SignalingState)
	}
	// ConnectTimeMs must be set even on rejection (timing sample for the rejected path).
	if result.ConnectTimeMs == nil {
		t.Error("expected ConnectTimeMs != nil on app_rejected (rejection is a valid timing sample)")
	}
}

// TestProbeRTMP_ConnectTimeout verifies that a server that completes the
// handshake but never sends an AMF0 response causes ErrorCode="rtmp_connect_timeout"
// and SignalingState="handshake_complete" (honest partial — handshake succeeded).
func TestProbeRTMP_ConnectTimeout(t *testing.T) {
	srv := newRTMPListener(t)
	defer srv.close()

	srv.serveOnce(func(conn net.Conn) error {
		conn.SetDeadline(time.Now().Add(5 * time.Second))

		c1, err := readC0C1(conn)
		if err != nil {
			return err
		}
		if err := sendS0S1S2(conn, 0x03, c1, false); err != nil {
			return err
		}
		// Read C2
		c2 := make([]byte, 1536)
		if _, err := io.ReadFull(conn, c2); err != nil {
			return fmt.Errorf("read C2: %v", err)
		}
		// Read connect chunk (if it arrives) but send NO AMF0 response.
		// Drain remaining writes so the prober blocks on reading, not writing.
		_, _ = io.Copy(io.Discard, conn)
		return nil
	})

	r := rtmpTestRunner()
	// Short timeout: probe must fire before the server writes anything.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := r.probeRTMP(ctx, rtmpConfig(srv.url("live")), rtmpResult())
	elapsed := time.Since(start)

	t.Logf("connect-timeout result: success=%v code=%q state=%q elapsed=%v",
		result.Success, result.ErrorCode, result.SignalingState, elapsed)

	// RED: current impl returns success=true immediately after C2; after
	// implementation: probe blocks waiting for _result and times out.
	if result.Success {
		t.Error("expected Success=false on connect timeout")
	}
	if result.ErrorCode != "rtmp_connect_timeout" {
		t.Errorf("expected error_code=rtmp_connect_timeout, got %q", result.ErrorCode)
	}
	if result.SignalingState != "handshake_complete" {
		t.Errorf("expected SignalingState=handshake_complete (honest partial), got %q", result.SignalingState)
	}
	if result.ConnectTimeMs != nil {
		t.Errorf("expected ConnectTimeMs=nil on connect timeout, got %d", *result.ConnectTimeMs)
	}
	if elapsed > 5*time.Second {
		t.Errorf("test took too long (%v); expected to complete within 5 s", elapsed)
	}

	select {
	case <-srv.done:
	case <-time.After(3 * time.Second):
		t.Log("warning: server goroutine did not exit within 3 s (non-fatal)")
	}
}

// TestProbeRTMP_Refused verifies that a connection-refused error maps to rtmp_refused.
// Approach: listen on :0, note the port, close, then run the probe.
func TestProbeRTMP_Refused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close() // close immediately → ECONNREFUSED on dial

	url := "rtmp://" + addr + "/live"
	r := rtmpTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := r.probeRTMP(ctx, rtmpConfig(url), rtmpResult())
	t.Logf("refused result: success=%v code=%q state=%q msg=%q",
		result.Success, result.ErrorCode, result.SignalingState, result.ErrorMsg)

	if result.Success {
		t.Error("expected Success=false on connection refused")
	}
	// rtmp_refused is the primary classification; rtmp_error is accepted on platforms
	// where the OS returns a different error string (mirrors WebRTC ws_refused test).
	if result.ErrorCode != "rtmp_refused" && result.ErrorCode != "rtmp_error" {
		t.Errorf("expected error_code rtmp_refused or rtmp_error, got %q", result.ErrorCode)
	}
	if result.ConnectTimeMs != nil {
		t.Errorf("expected ConnectTimeMs=nil on failure, got %d", *result.ConnectTimeMs)
	}
}

// TestProbeRTMP_Timeout verifies that a server that accepts but never writes
// causes an rtmp_timeout once the probe's context deadline fires.
// The server drains the client's writes via io.Copy(io.Discard) so the C0+C1
// write succeeds; the probe then blocks on reading S0 until the deadline.
// Total test wall-time ≪ 5 s (probe timeout = 500 ms).
func TestProbeRTMP_Timeout(t *testing.T) {
	srv := newRTMPListener(t)
	defer srv.close()

	// Server accepts and consumes whatever the probe sends, but never writes back.
	// io.Copy blocks until the probe closes its end (on deadline expiry).
	srv.serveOnce(func(conn net.Conn) error {
		_, _ = io.Copy(io.Discard, conn) // drain writes, never reply
		return nil
	})

	r := rtmpTestRunner()
	// Short timeout: probe must time out before the server writes anything.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := r.probeRTMP(ctx, rtmpConfig(srv.url("live")), rtmpResult())
	elapsed := time.Since(start)

	t.Logf("timeout result: success=%v code=%q state=%q elapsed=%v msg=%q",
		result.Success, result.ErrorCode, result.SignalingState, elapsed, result.ErrorMsg)

	if result.Success {
		t.Error("expected Success=false on timeout")
	}
	if result.ErrorCode != "rtmp_timeout" {
		t.Errorf("expected error_code=rtmp_timeout, got %q", result.ErrorCode)
	}
	if result.SignalingState != "rtmp_timeout" {
		t.Errorf("expected SignalingState=rtmp_timeout, got %q", result.SignalingState)
	}
	if result.ConnectTimeMs != nil {
		t.Errorf("expected ConnectTimeMs=nil on timeout, got %d", *result.ConnectTimeMs)
	}
	// Sanity: test finished well within 5 s (probe timeout was 500 ms).
	if elapsed > 5*time.Second {
		t.Errorf("test took too long (%v); probe timeout was 500 ms", elapsed)
	}

	// Let the server goroutine drain. Since the probe closed its conn on timeout,
	// io.Copy returns promptly — no sleep needed.
	select {
	case <-srv.done:
	case <-time.After(3 * time.Second):
		t.Log("warning: server goroutine did not exit within 3 s (non-fatal)")
	}
}

// TestProbeRTMP_BadVersion verifies that S0 != 0x03 maps to rtmp_error.
func TestProbeRTMP_BadVersion(t *testing.T) {
	srv := newRTMPListener(t)
	defer srv.close()

	srv.serveOnce(func(conn net.Conn) error {
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		c1, err := readC0C1(conn)
		if err != nil {
			return err
		}
		// Send S0 with wrong version byte (0x06).
		if err := sendS0S1S2(conn, 0x06, c1, false); err != nil {
			return err
		}
		return nil
	})

	r := rtmpTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := r.probeRTMP(ctx, rtmpConfig(srv.url("live")), rtmpResult())
	srv.wait(t)

	t.Logf("bad-version result: success=%v code=%q state=%q msg=%q",
		result.Success, result.ErrorCode, result.SignalingState, result.ErrorMsg)

	if result.Success {
		t.Error("expected Success=false on bad S0 version")
	}
	if result.ErrorCode != "rtmp_error" {
		t.Errorf("expected error_code=rtmp_error on bad S0, got %q", result.ErrorCode)
	}
	if result.SignalingState != "rtmp_error" {
		t.Errorf("expected SignalingState=rtmp_error, got %q", result.SignalingState)
	}
	if result.ConnectTimeMs != nil {
		t.Errorf("expected ConnectTimeMs=nil on failure, got %d", *result.ConnectTimeMs)
	}
}

// TestProbeRTMP_S2EchoMismatch verifies that an incorrect S2 echo maps to rtmp_error.
// The server sends S2 filled with 0xFF bytes instead of echoing C1.
func TestProbeRTMP_S2EchoMismatch(t *testing.T) {
	srv := newRTMPListener(t)
	defer srv.close()

	srv.serveOnce(func(conn net.Conn) error {
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		c1, err := readC0C1(conn)
		if err != nil {
			return err
		}
		// corruptS2=true: S2 is all 0xFF — clearly not an echo of C1.
		if err := sendS0S1S2(conn, 0x03, c1, true); err != nil {
			return err
		}
		// Probe will close the connection on mismatch; drain to avoid EPIPE.
		_, _ = io.Copy(io.Discard, conn)
		return nil
	})

	r := rtmpTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := r.probeRTMP(ctx, rtmpConfig(srv.url("live")), rtmpResult())

	// Server goroutine may already be done (probe closed conn after detecting mismatch).
	select {
	case <-srv.done:
	case <-time.After(3 * time.Second):
		t.Log("warning: server goroutine still running after 3 s (non-fatal)")
	}

	t.Logf("s2-mismatch result: success=%v code=%q state=%q msg=%q",
		result.Success, result.ErrorCode, result.SignalingState, result.ErrorMsg)

	if result.Success {
		t.Error("expected Success=false on S2 echo mismatch")
	}
	if result.ErrorCode != "rtmp_error" {
		t.Errorf("expected error_code=rtmp_error on S2 mismatch, got %q", result.ErrorCode)
	}
	if result.SignalingState != "rtmp_error" {
		t.Errorf("expected SignalingState=rtmp_error, got %q", result.SignalingState)
	}
	if result.ConnectTimeMs != nil {
		t.Errorf("expected ConnectTimeMs=nil on failure, got %d", *result.ConnectTimeMs)
	}
}

// TestProbeRTMP_MalformedURL verifies that non-rtmp:// URLs return rtmp_error immediately
// without attempting a network connection.
func TestProbeRTMP_MalformedURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"no-scheme", "not-an-rtmp-url"},
		{"http-scheme", "http://example.com/stream"},
		{"empty", ""},
		{"rtmp-empty-host", "rtmp:///app/stream"},
	}

	r := rtmpTestRunner()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result := r.probeRTMP(ctx, rtmpConfig(tc.url), rtmpResult())
			t.Logf("[%s] result: success=%v code=%q state=%q msg=%q",
				tc.name, result.Success, result.ErrorCode, result.SignalingState, result.ErrorMsg)

			if result.Success {
				t.Errorf("[%s] expected Success=false on malformed URL", tc.name)
			}
			if result.ErrorCode != "rtmp_error" {
				t.Errorf("[%s] expected error_code=rtmp_error, got %q", tc.name, result.ErrorCode)
			}
			if result.SignalingState != "rtmp_error" {
				t.Errorf("[%s] expected SignalingState=rtmp_error, got %q", tc.name, result.SignalingState)
			}
			if result.ConnectTimeMs != nil {
				t.Errorf("[%s] expected ConnectTimeMs=nil on failure", tc.name)
			}
		})
	}
}

// TestReadAMF0Command_HonorsSetChunkSize pins the demuxer's SetChunkSize
// (type 0x01) handling: after the server renegotiates a larger chunk size,
// a command longer than the 128-byte default arrives as a SINGLE chunk. A
// demuxer stuck at the default expects a continuation header at byte 128,
// desyncs into the payload, and fails (S29 V1 mutation catch — the live AMS
// fixture happens not to renegotiate, so only this test covers the handler).
func TestReadAMF0Command_HonorsSetChunkSize(t *testing.T) {
	var buf bytes.Buffer

	// SetChunkSize(256): csid=2 fmt=0, ts=0, len=4, type=0x01, msg stream id=0.
	buf.Write([]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00})
	buf.Write([]byte{0x00, 0x00, 0x01, 0x00}) // 256 big-endian

	// AMF0 _result command longer than the default chunk size — sent as a
	// single chunk, valid only under the renegotiated size.
	body := amf0EncodeString("_result")
	body = append(body, amf0EncodeNumber(1)...)
	body = append(body, amf0EncodeString(strings.Repeat("x", 150))...)
	if len(body) <= rtmpDefaultChunkSz {
		t.Fatalf("test body must exceed the default chunk size, got %d bytes", len(body))
	}
	buf.Write([]byte{0x03, 0x00, 0x00, 0x00, 0x00, byte(len(body) >> 8), byte(len(body)), 0x14, 0x00, 0x00, 0x00, 0x00})
	buf.Write(body)

	name, err := readAMF0Command(&buf)
	if err != nil {
		t.Fatalf("readAMF0Command with renegotiated chunk size: %v", err)
	}
	if name != "_result" {
		t.Fatalf("expected _result, got %q", name)
	}
}
