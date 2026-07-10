// probe_rtmp.go — pure-Go stdlib RTMP handshake probe (phase 1, D-073 ruling).
//
// Mirrors probeWebRTC's signature, result construction, timeout/context pattern,
// and 1ms-floor convention exactly.  Wiring (case "rtmp": r.probeRTMP) is added
// by a separate wiring author; this file is self-contained.
package prober

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// rtmpDefaultPort is used when no port is present in the rtmp:// URL.
const rtmpDefaultPort = "1935"

// probeRTMP performs an RTMP phase-1 handshake probe.
//
// URL convention: rtmp://host[:port]/... — default port 1935 when absent.
//
// Steps:
//  1. Parse URL; non-rtmp:// or empty host → rtmp_error immediately.
//  2. Dial TCP with the probe's ctx (context timeout already applied by the
//     scheduler — mirrors how probeWebRTC bounds its websocket.Dial call).
//  3. Write C0 (version 0x03) + C1 (1536 bytes: 4-byte big-endian ms timestamp,
//     4 zero bytes, 1528 crypto/rand bytes).
//  4. Read S0 (validate == 0x03), S1 (1536 bytes), S2 (1536 bytes) via io.ReadFull.
//  5. Strict S2 echo check: S2[0:4] must equal C1 timestamp AND S2[8:] must equal
//     C1 random; mismatch → rtmp_error (catches non-RTMP TCP endpoints).
//  6. On success: set ConnectTimeMs (≥1 ms, same floor as probeWebRTC),
//     SignalingState="handshake_complete".
//  7. Write C2 (echo of S1) best-effort — success is already recorded before C2.
//
// Error codes (SignalingState mirrors error code on failure, exactly as probeWebRTC does):
//   - "rtmp_refused"  : TCP connection refused.
//   - "rtmp_timeout"  : context deadline exceeded.
//   - "rtmp_error"    : any other failure (bad URL, protocol violation, S2 mismatch).
func (r *Runner) probeRTMP(ctx context.Context, p domain.ProbeConfig, result domain.ProbeResult) domain.ProbeResult {
	// ── 1. Parse URL ─────────────────────────────────────────────────────────
	host, port, err := parseRTMPURL(p.URL)
	if err != nil {
		result.Success = false
		result.ErrorCode = "rtmp_error"
		result.ErrorMsg = fmt.Sprintf("rtmp parse url: %v", err)
		result.SignalingState = "rtmp_error"
		return result
	}
	addr := net.JoinHostPort(host, port)

	// ── 2. Dial ──────────────────────────────────────────────────────────────
	dialStart := time.Now()
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		code, sigState := classifyRTMPNetError(ctx, err)
		result.Success = false
		result.ErrorCode = code
		result.ErrorMsg = fmt.Sprintf("rtmp dial: %v", err)
		result.SignalingState = sigState
		return result
	}
	defer conn.Close()

	// Propagate the context deadline onto the connection so reads/writes
	// respect the per-probe timeout — mirrors the deadline the scheduler
	// applies via context.WithTimeout in executeProbe.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	// ── 3. Build and send C0 + C1 ────────────────────────────────────────────
	// C0: 1 byte (RTMP version = 0x03).
	// C1: 1536 bytes = [time(4)] [zeros(4)] [random(1528)].
	const chunkLen = 1536
	c1 := make([]byte, chunkLen)
	ts := uint32(time.Now().UnixMilli())
	binary.BigEndian.PutUint32(c1[0:4], ts)
	// c1[4:8] remain zero.
	if _, err := io.ReadFull(rand.Reader, c1[8:]); err != nil {
		result.Success = false
		result.ErrorCode = "rtmp_error"
		result.ErrorMsg = fmt.Sprintf("rtmp rand fill C1: %v", err)
		result.SignalingState = "rtmp_error"
		return result
	}

	c0c1 := make([]byte, 1+chunkLen)
	c0c1[0] = 0x03
	copy(c0c1[1:], c1)
	if _, err := conn.Write(c0c1); err != nil {
		code, sigState := classifyRTMPNetError(ctx, err)
		result.Success = false
		result.ErrorCode = code
		result.ErrorMsg = fmt.Sprintf("rtmp write C0+C1: %v", err)
		result.SignalingState = sigState
		return result
	}

	// ── 4. Read S0 + S1 + S2 ─────────────────────────────────────────────────
	// S0: 1 byte — must be 0x03.
	s0 := make([]byte, 1)
	if _, err := io.ReadFull(conn, s0); err != nil {
		code, sigState := classifyRTMPNetError(ctx, err)
		result.Success = false
		result.ErrorCode = code
		result.ErrorMsg = fmt.Sprintf("rtmp read S0: %v", err)
		result.SignalingState = sigState
		return result
	}
	if s0[0] != 0x03 {
		result.Success = false
		result.ErrorCode = "rtmp_error"
		result.ErrorMsg = fmt.Sprintf("rtmp S0 version mismatch: got 0x%02x, want 0x03", s0[0])
		result.SignalingState = "rtmp_error"
		return result
	}

	// S1: 1536 bytes.
	s1 := make([]byte, chunkLen)
	if _, err := io.ReadFull(conn, s1); err != nil {
		code, sigState := classifyRTMPNetError(ctx, err)
		result.Success = false
		result.ErrorCode = code
		result.ErrorMsg = fmt.Sprintf("rtmp read S1: %v", err)
		result.SignalingState = sigState
		return result
	}

	// S2: 1536 bytes.
	s2 := make([]byte, chunkLen)
	if _, err := io.ReadFull(conn, s2); err != nil {
		code, sigState := classifyRTMPNetError(ctx, err)
		result.Success = false
		result.ErrorCode = code
		result.ErrorMsg = fmt.Sprintf("rtmp read S2: %v", err)
		result.SignalingState = sigState
		return result
	}

	// ── 5. Strict S2 echo check ───────────────────────────────────────────────
	// S2 must echo C1: S2[0:4] == C1 timestamp, S2[8:] == C1 random bytes.
	// S2[4:8] is the server's time2 (S1 send time) — not checked.
	// Mismatch → rtmp_error (catches non-RTMP TCP endpoints that happen to
	// accept the connection and write 3073 bytes of arbitrary content).
	if !bytes.Equal(s2[0:4], c1[0:4]) || !bytes.Equal(s2[8:], c1[8:]) {
		result.Success = false
		result.ErrorCode = "rtmp_error"
		result.ErrorMsg = "rtmp S2 echo mismatch: server did not correctly echo C1 (non-RTMP endpoint?)"
		result.SignalingState = "rtmp_error"
		return result
	}

	// ── 6. Record success ─────────────────────────────────────────────────────
	elapsed := uint32(time.Since(dialStart).Milliseconds())
	if elapsed == 0 {
		elapsed = 1 // floor at 1 ms — same pattern as HLS TTFB and WebRTC ConnectTimeMs
	}

	// ── 7. Write C2 best-effort ───────────────────────────────────────────────
	// C2 echoes S1: S1 timestamp at [0:4], current time2 at [4:8], S1 random at [8:].
	// Success is already determined; C2 write errors are intentionally ignored.
	c2 := make([]byte, chunkLen)
	copy(c2[0:4], s1[0:4])
	binary.BigEndian.PutUint32(c2[4:8], uint32(time.Now().UnixMilli()))
	copy(c2[8:], s1[8:])
	_, _ = conn.Write(c2)

	result.Success = true
	result.ErrorCode = ""
	result.ErrorMsg = ""
	result.ConnectTimeMs = &elapsed
	result.SignalingState = "handshake_complete"
	return result
}

// parseRTMPURL extracts host and port from an rtmp:// URL.
// Returns rtmp_error descriptions for: wrong scheme, empty host.
// Port defaults to 1935 when absent.
func parseRTMPURL(rawURL string) (host, port string, err error) {
	u, parseErr := url.Parse(rawURL)
	if parseErr != nil {
		return "", "", fmt.Errorf("invalid URL %q: %v", rawURL, parseErr)
	}
	if !strings.EqualFold(u.Scheme, "rtmp") {
		return "", "", fmt.Errorf("expected rtmp:// scheme, got scheme=%q in URL %q", u.Scheme, rawURL)
	}
	h := u.Hostname()
	if h == "" {
		return "", "", fmt.Errorf("empty host in RTMP URL %q", rawURL)
	}
	p := u.Port()
	if p == "" {
		p = rtmpDefaultPort
	}
	return h, p, nil
}

// classifyRTMPNetError maps a network/IO error to an RTMP error code and
// signaling state string.  Mirrors the classification block in probeWebRTC:
// ctx.Err() is checked first (nhooyr.io/websocket wraps context errors into
// CloseErrors; net.Conn deadline errors are an i/o timeout string — same idea).
func classifyRTMPNetError(ctx context.Context, err error) (code, sigState string) {
	// Prefer ctx.Err() as the authoritative signal — the connection deadline is
	// derived from the context deadline, so both fire at the same moment.
	if ctx.Err() != nil {
		switch ctx.Err() {
		case context.DeadlineExceeded:
			return "rtmp_timeout", "rtmp_timeout"
		case context.Canceled:
			return "rtmp_error", "rtmp_error"
		}
	}
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "connection refused"):
		return "rtmp_refused", "rtmp_refused"
	case strings.Contains(errStr, "context deadline exceeded") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "timeout"):
		return "rtmp_timeout", "rtmp_timeout"
	default:
		return "rtmp_error", "rtmp_error"
	}
}
