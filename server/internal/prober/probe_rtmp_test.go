// Tests for probeRTMP.
// Internal test package (package prober) so the unexported probeRTMP method is
// accessible without going through the scheduler dispatch (wiring is added by a
// separate wiring author in the "rtmp" case of executeProbe).
package prober

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

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

// TestProbeRTMP_Success verifies the full C0+C1 → S0+S1+S2 → C2 happy path.
// Assertions: Success=true, ConnectTimeMs>=1, SignalingState="handshake_complete", ErrorCode="".
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
		// Read C2 (1536 bytes) — probe sends this best-effort; validate it arrived.
		c2 := make([]byte, 1536)
		if _, err := io.ReadFull(conn, c2); err != nil {
			return fmt.Errorf("read C2: %v", err)
		}
		return nil
	})

	r := rtmpTestRunner()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := r.probeRTMP(ctx, rtmpConfig(srv.url("live")), rtmpResult())
	srv.wait(t)

	t.Logf("success result: success=%v code=%q state=%q connect_ms=%v msg=%q",
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
	if result.SignalingState != "handshake_complete" {
		t.Errorf("expected SignalingState=handshake_complete, got %q", result.SignalingState)
	}
	if result.ErrorCode != "" {
		t.Errorf("expected empty ErrorCode on success, got %q", result.ErrorCode)
	}
	if result.ErrorMsg != "" {
		t.Errorf("expected empty ErrorMsg on success, got %q", result.ErrorMsg)
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
