// probe_rtmp.go — pure-Go stdlib RTMP handshake + AMF0 connect probe (D-073/D-090).
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
	"math"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// rtmpDefaultPort is used when no port is present in the rtmp:// URL.
const rtmpDefaultPort = "1935"

// rtmpMaxMsgSize caps the per-message accumulation buffer in the chunk demuxer.
const rtmpMaxMsgSize = 64 * 1024

// rtmpDefaultChunkSz is the RTMP default incoming chunk size (per spec §5.4.1).
const rtmpDefaultChunkSz = 128

// rtmpCSIDState holds per-chunk-stream-ID message state for the chunk demuxer.
type rtmpCSIDState struct {
	timestamp uint32
	length    uint32
	typeID    byte
	streamID  uint32
	buf       []byte // accumulated payload bytes for the in-progress message
}

// probeRTMP performs an RTMP handshake + AMF0 connect probe.
//
// URL convention: rtmp://host[:port][/app[/...]] — default port 1935 when absent.
//
// Steps:
//  1. Parse URL; non-rtmp:// or empty host → rtmp_error immediately.
//  2. Dial TCP with the probe's ctx.
//  3. Write C0 (version 0x03) + C1 (1536 bytes).
//  4. Read S0 (validate == 0x03), S1, S2 via io.ReadFull.
//  5. Strict S2 echo check.
//  6. Write C2 best-effort.
//  7. If URL has NO app path segment → success=true, handshake_complete (legacy path).
//  8. Otherwise → send AMF0 "connect" chunk, read response via chunk demuxer:
//     - "_result"  → success=true,  SignalingState="app_accepted",  ConnectTimeMs=dial→_result
//     - "_error"   → success=false, SignalingState="app_rejected",  ConnectTimeMs still set
//     - deadline   → success=false, ErrorCode="rtmp_connect_timeout", SignalingState="handshake_complete"
//     - garbage    → success=false, ErrorCode="rtmp_error",           SignalingState="handshake_complete"
//
// Error codes:
//   - "rtmp_refused"       : TCP connection refused.
//   - "rtmp_timeout"       : context deadline during handshake.
//   - "rtmp_error"         : any other failure (bad URL, protocol violation, AMF0 garbage).
//   - "rtmp_connect_timeout": AMF0 connect exchange timed out after handshake.
//   - "rtmp_app_rejected"  : server responded with AMF0 _error.
func (r *Runner) probeRTMP(ctx context.Context, p domain.ProbeConfig, result domain.ProbeResult) domain.ProbeResult {
	// ── 1. Parse URL ─────────────────────────────────────────────────────────
	host, port, app, err := parseRTMPURL(p.URL)
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
	// respect the per-probe timeout — mirrors how probeWebRTC bounds its calls.
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
	if !bytes.Equal(s2[0:4], c1[0:4]) || !bytes.Equal(s2[8:], c1[8:]) {
		result.Success = false
		result.ErrorCode = "rtmp_error"
		result.ErrorMsg = "rtmp S2 echo mismatch: server did not correctly echo C1 (non-RTMP endpoint?)"
		result.SignalingState = "rtmp_error"
		return result
	}

	// ── 6. Write C2 best-effort ───────────────────────────────────────────────
	// C2 echoes S1: S1 timestamp at [0:4], current time2 at [4:8], S1 random at [8:].
	// Success determination and ConnectTimeMs accounting happen after this.
	c2 := make([]byte, chunkLen)
	copy(c2[0:4], s1[0:4])
	binary.BigEndian.PutUint32(c2[4:8], uint32(time.Now().UnixMilli()))
	copy(c2[8:], s1[8:])
	_, _ = conn.Write(c2)

	// ── 7. Legacy no-app path ─────────────────────────────────────────────────
	// URL without an app segment → skip AMF0 entirely; preserve phase-1 behavior.
	if app == "" {
		elapsed := uint32(time.Since(dialStart).Milliseconds())
		if elapsed == 0 {
			elapsed = 1
		}
		result.Success = true
		result.ErrorCode = ""
		result.ErrorMsg = ""
		result.ConnectTimeMs = &elapsed
		result.SignalingState = "handshake_complete"
		return result
	}

	// ── 8. AMF0 connect exchange ──────────────────────────────────────────────
	// Build and send the connect command chunk.
	tcURL := "rtmp://" + addr + "/" + app
	connectChunk := buildConnectChunk(app, tcURL)
	if _, err := conn.Write(connectChunk); err != nil {
		code, _ := classifyRTMPNetError(ctx, err)
		result.Success = false
		result.ErrorCode = code
		result.ErrorMsg = fmt.Sprintf("rtmp write connect: %v", err)
		result.SignalingState = "handshake_complete"
		return result
	}

	// Read the server's AMF0 response via the minimal chunk demuxer.
	cmdName, err := readAMF0Command(conn)
	if err != nil {
		// Distinguish deadline/timeout from protocol garbage.
		code := "rtmp_error"
		if ctx.Err() == context.DeadlineExceeded || ctx.Err() == context.Canceled {
			code = "rtmp_connect_timeout"
		} else if strings.Contains(err.Error(), "timeout") ||
			strings.Contains(err.Error(), "deadline") ||
			strings.Contains(err.Error(), "i/o timeout") {
			code = "rtmp_connect_timeout"
		}
		result.Success = false
		result.ErrorCode = code
		result.ErrorMsg = fmt.Sprintf("rtmp connect response: %v", err)
		result.SignalingState = "handshake_complete" // honest partial: handshake succeeded
		return result
	}

	// ConnectTimeMs widened to dial → _result/_error received.
	elapsed := uint32(time.Since(dialStart).Milliseconds())
	if elapsed == 0 {
		elapsed = 1
	}

	switch cmdName {
	case "_result":
		result.Success = true
		result.ErrorCode = ""
		result.ErrorMsg = ""
		result.ConnectTimeMs = &elapsed
		result.SignalingState = "app_accepted"
	case "_error":
		result.Success = false
		result.ErrorCode = "rtmp_app_rejected"
		result.ErrorMsg = "rtmp connect: server returned _error (app rejected)"
		result.ConnectTimeMs = &elapsed // rejection is a valid timing sample
		result.SignalingState = "app_rejected"
	default:
		result.Success = false
		result.ErrorCode = "rtmp_error"
		result.ErrorMsg = fmt.Sprintf("rtmp connect: unexpected command %q", cmdName)
		result.SignalingState = "handshake_complete"
	}
	return result
}

// ── parseRTMPURL ──────────────────────────────────────────────────────────────

// parseRTMPURL extracts host, port, and the first app path segment from an
// rtmp:// URL.  Port defaults to 1935 when absent.  app is empty when the URL
// has no path segment (or an empty path), which triggers the legacy handshake-
// only path in probeRTMP.
func parseRTMPURL(rawURL string) (host, port, app string, err error) {
	u, parseErr := url.Parse(rawURL)
	if parseErr != nil {
		return "", "", "", fmt.Errorf("invalid URL %q: %v", rawURL, parseErr)
	}
	if !strings.EqualFold(u.Scheme, "rtmp") {
		return "", "", "", fmt.Errorf("expected rtmp:// scheme, got scheme=%q in URL %q", u.Scheme, rawURL)
	}
	h := u.Hostname()
	if h == "" {
		return "", "", "", fmt.Errorf("empty host in RTMP URL %q", rawURL)
	}
	p := u.Port()
	if p == "" {
		p = rtmpDefaultPort
	}
	// Extract first non-empty path segment as app name.
	// rtmp://host/LiveApp       → app = "LiveApp"
	// rtmp://host/LiveApp/stream → app = "LiveApp"
	// rtmp://host/              → app = ""
	// rtmp://host               → app = ""
	appName := strings.TrimPrefix(u.Path, "/")
	if idx := strings.IndexByte(appName, '/'); idx >= 0 {
		appName = appName[:idx]
	}
	return h, p, appName, nil
}

// ── AMF0 helpers ──────────────────────────────────────────────────────────────

// amf0EncodeString returns an AMF0 string encoding (type 0x02 + uint16 length + UTF-8 bytes).
func amf0EncodeString(s string) []byte {
	b := make([]byte, 3+len(s))
	b[0] = 0x02
	binary.BigEndian.PutUint16(b[1:3], uint16(len(s)))
	copy(b[3:], s)
	return b
}

// amf0EncodeNumber returns an AMF0 number encoding (type 0x00 + 8-byte IEEE 754 BE).
func amf0EncodeNumber(n float64) []byte {
	b := make([]byte, 9)
	b[0] = 0x00
	binary.BigEndian.PutUint64(b[1:], math.Float64bits(n))
	return b
}

// amf0DecodeString decodes an AMF0 string from data starting at offset.
// Returns the string and the next byte offset, or an error.
func amf0DecodeString(data []byte, offset int) (string, int, error) {
	if offset >= len(data) {
		return "", offset, fmt.Errorf("amf0: offset %d beyond data", offset)
	}
	if data[offset] != 0x02 {
		return "", offset, fmt.Errorf("amf0: expected string type 0x02 at offset %d, got 0x%02x", offset, data[offset])
	}
	offset++
	if offset+2 > len(data) {
		return "", offset, fmt.Errorf("amf0: short buffer for string length")
	}
	slen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+slen > len(data) {
		return "", offset, fmt.Errorf("amf0: short buffer for string data (want %d bytes)", slen)
	}
	return string(data[offset : offset+slen]), offset + slen, nil
}

// ── RTMP chunk builder ────────────────────────────────────────────────────────

// buildConnectChunk builds the RTMP chunk stream carrying the AMF0 "connect"
// command (fmt=0, csid=3, type=0x14, msg_stream_id=0).
//
// The payload carries:
//
//	["connect", 1.0, {app: <app>, tcUrl: <tcURL>, flashVer: "FMLE/3.0"}]
//
// The payload is split into rtmpDefaultChunkSz (128) byte pieces; subsequent
// pieces carry a fmt=3 continuation header (1 byte).
func buildConnectChunk(app, tcURL string) []byte {
	// Build AMF0 payload
	var payload []byte
	payload = append(payload, amf0EncodeString("connect")...)
	payload = append(payload, amf0EncodeNumber(1.0)...)
	payload = append(payload, 0x03) // AMF0 object start

	appendProp := func(key string, val []byte) {
		// Object property key: uint16 length + key bytes (no type byte)
		payload = append(payload, byte(len(key)>>8), byte(len(key)))
		payload = append(payload, []byte(key)...)
		payload = append(payload, val...)
	}
	appendProp("app", amf0EncodeString(app))
	appendProp("tcUrl", amf0EncodeString(tcURL))
	appendProp("flashVer", amf0EncodeString("FMLE/3.0"))
	payload = append(payload, 0x00, 0x00, 0x09) // AMF0 object end marker

	msgLen := len(payload)

	// First chunk: fmt=0 header (1 basic + 11 message = 12 bytes)
	var out []byte
	out = append(out, 0x03)                                            // basic header: fmt=0, csid=3
	out = append(out, 0x00, 0x00, 0x00)                                // timestamp = 0
	out = append(out, byte(msgLen>>16), byte(msgLen>>8), byte(msgLen)) // message length
	out = append(out, 0x14)                                            // type_id: AMF0 command
	out = append(out, 0x00, 0x00, 0x00, 0x00)                          // msg_stream_id = 0 (LE)

	// Payload split across rtmpDefaultChunkSz-byte chunks
	remaining := payload
	for i := 0; len(remaining) > 0; i++ {
		n := rtmpDefaultChunkSz
		if n > len(remaining) {
			n = len(remaining)
		}
		if i > 0 {
			// Continuation chunk basic header: fmt=3, csid=3 → 0xC3
			out = append(out, 0xC3)
		}
		out = append(out, remaining[:n]...)
		remaining = remaining[n:]
	}
	return out
}

// ── Minimal chunk demuxer ─────────────────────────────────────────────────────

// readAMF0Command reads RTMP chunks from r until a complete AMF0 command message
// (type 0x14) is assembled, then returns the command name (first AMF0 string value).
//
// Protocol behaviors:
//   - Honors incoming SetChunkSize (type 0x01) to adjust the read window.
//   - Handles extended timestamps (0xFFFFFF marker) for fmt 0–2.
//   - Reassembles fragmented messages per csid, up to a rtmpMaxMsgSize (64 KB) cap.
//   - Skips WindowAckSize (5), SetPeerBandwidth (6), UserControl (4), and all
//     other non-AMF0-command message types silently.
//   - Returns the first AMF0 command name; the caller decides on "_result"/"_error".
func readAMF0Command(r io.Reader) (string, error) {
	chunkSize := rtmpDefaultChunkSz
	states := make(map[uint32]*rtmpCSIDState)

	const extTS = uint32(0xFFFFFF)

	for {
		// ── Basic header ─────────────────────────────────────────────────
		var b0 [1]byte
		if _, err := io.ReadFull(r, b0[:]); err != nil {
			return "", fmt.Errorf("rtmp chunk: basic header: %w", err)
		}
		fmt_ := (b0[0] >> 6) & 0x03
		csidRaw := uint32(b0[0] & 0x3F)

		var csid uint32
		switch csidRaw {
		case 0: // 2-byte form: csid = 64 + next byte
			var x [1]byte
			if _, err := io.ReadFull(r, x[:]); err != nil {
				return "", fmt.Errorf("rtmp chunk: csid 2-byte: %w", err)
			}
			csid = 64 + uint32(x[0])
		case 1: // 3-byte form: csid = 64 + uint8 + 256*uint8
			var x [2]byte
			if _, err := io.ReadFull(r, x[:]); err != nil {
				return "", fmt.Errorf("rtmp chunk: csid 3-byte: %w", err)
			}
			csid = 64 + uint32(x[0]) + 256*uint32(x[1])
		default:
			csid = csidRaw
		}

		st, ok := states[csid]
		if !ok {
			st = new(rtmpCSIDState)
			states[csid] = st
		}

		// ── Message header ───────────────────────────────────────────────
		switch fmt_ {
		case 0: // 11 bytes — full header, starts new message
			var h [11]byte
			if _, err := io.ReadFull(r, h[:]); err != nil {
				return "", fmt.Errorf("rtmp chunk: fmt0 header: %w", err)
			}
			rawTS := uint32(h[0])<<16 | uint32(h[1])<<8 | uint32(h[2])
			st.length = uint32(h[3])<<16 | uint32(h[4])<<8 | uint32(h[5])
			st.typeID = h[6]
			// msg_stream_id is little-endian
			st.streamID = uint32(h[7]) | uint32(h[8])<<8 | uint32(h[9])<<16 | uint32(h[10])<<24
			if rawTS == extTS {
				var ext [4]byte
				if _, err := io.ReadFull(r, ext[:]); err != nil {
					return "", fmt.Errorf("rtmp chunk: ext timestamp fmt0: %w", err)
				}
				st.timestamp = binary.BigEndian.Uint32(ext[:])
			} else {
				st.timestamp = rawTS
			}
			st.buf = st.buf[:0] // start fresh for this message

		case 1: // 7 bytes — new message, inherits stream ID
			var h [7]byte
			if _, err := io.ReadFull(r, h[:]); err != nil {
				return "", fmt.Errorf("rtmp chunk: fmt1 header: %w", err)
			}
			delta := uint32(h[0])<<16 | uint32(h[1])<<8 | uint32(h[2])
			st.length = uint32(h[3])<<16 | uint32(h[4])<<8 | uint32(h[5])
			st.typeID = h[6]
			if delta == extTS {
				var ext [4]byte
				if _, err := io.ReadFull(r, ext[:]); err != nil {
					return "", fmt.Errorf("rtmp chunk: ext timestamp fmt1: %w", err)
				}
				st.timestamp += binary.BigEndian.Uint32(ext[:])
			} else {
				st.timestamp += delta
			}
			st.buf = st.buf[:0]

		case 2: // 3 bytes — continuation, same length/type/stream
			var h [3]byte
			if _, err := io.ReadFull(r, h[:]); err != nil {
				return "", fmt.Errorf("rtmp chunk: fmt2 header: %w", err)
			}
			delta := uint32(h[0])<<16 | uint32(h[1])<<8 | uint32(h[2])
			if delta == extTS {
				var ext [4]byte
				if _, err := io.ReadFull(r, ext[:]); err != nil {
					return "", fmt.Errorf("rtmp chunk: ext timestamp fmt2: %w", err)
				}
				st.timestamp += binary.BigEndian.Uint32(ext[:])
			} else {
				st.timestamp += delta
			}
			// length, typeID, streamID unchanged

		case 3: // 0 bytes — all fields from previous chunk on this csid
		}

		// ── Oversized message guard ───────────────────────────────────────
		if st.length > rtmpMaxMsgSize {
			// Drain this chunk's slice to stay byte-synchronized; bail out.
			already := len(st.buf)
			left := int(st.length) - already
			if left < 0 {
				left = 0
			}
			n := chunkSize
			if n > left {
				n = left
			}
			if _, err := io.CopyN(io.Discard, r, int64(n)); err != nil {
				return "", fmt.Errorf("rtmp chunk: drain oversized: %w", err)
			}
			return "", fmt.Errorf("rtmp chunk: message too large (%d bytes)", st.length)
		}

		// ── Read this chunk's payload bytes ──────────────────────────────
		already := len(st.buf)
		msgRemain := int(st.length) - already
		if msgRemain < 0 {
			msgRemain = 0
		}
		toRead := chunkSize
		if toRead > msgRemain {
			toRead = msgRemain
		}
		if toRead > 0 {
			// Grow st.buf if needed
			need := already + toRead
			if cap(st.buf) < need {
				grown := make([]byte, already, need)
				copy(grown, st.buf)
				st.buf = grown
			}
			st.buf = st.buf[:already+toRead]
			if _, err := io.ReadFull(r, st.buf[already:]); err != nil {
				return "", fmt.Errorf("rtmp chunk: payload: %w", err)
			}
		}

		// ── Check if message is complete ──────────────────────────────────
		if len(st.buf) < int(st.length) {
			continue // more chunks needed for this message
		}

		// Complete — process and reset accumulator
		msgType := st.typeID
		msgBuf := make([]byte, len(st.buf))
		copy(msgBuf, st.buf)
		st.buf = st.buf[:0]

		switch msgType {
		case 0x01: // SetChunkSize
			if len(msgBuf) >= 4 {
				n := int(binary.BigEndian.Uint32(msgBuf[:4]) & 0x7FFFFFFF)
				if n > 0 && n <= rtmpMaxMsgSize {
					chunkSize = n
				}
			}

		case 0x14: // AMF0 Command (invoke) — what we're waiting for
			if len(msgBuf) == 0 {
				return "", fmt.Errorf("rtmp: empty AMF0 command payload")
			}
			name, _, err := amf0DecodeString(msgBuf, 0)
			if err != nil {
				return "", fmt.Errorf("rtmp: AMF0 command name: %w", err)
			}
			return name, nil

			// Types 0x04 (UserControl), 0x05 (WindowAckSize), 0x06 (SetPeerBandwidth)
			// and all other types: skip silently.
		}
	}
}

// ── classifyRTMPNetError ──────────────────────────────────────────────────────

// classifyRTMPNetError maps a network/IO error to an RTMP error code and
// signaling state string.  Mirrors the classification block in probeWebRTC.
func classifyRTMPNetError(ctx context.Context, err error) (code, sigState string) {
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
