// Command mock-ams is a lightweight HTTP server that emulates the AMS REST v2
// surface used by the Pulse restpoller, logtail, and webhook collector.
//
// It runs configurable scenarios: streams publish/unpublish on a timeline,
// viewer counts evolve, and a stream can be forced to "fail".
//
// Usage:
//
//	mock-ams [-addr :9090] [-scenario N] [-app live] [-log-dir /tmp/ams-logs]
//
// REST paths emulated (matching amsclient/client.go exactly):
//
//	GET /rest/v2/applications               → {"applications":[{"name":"live"}]}
//	GET /rest/v2/broadcasts/{app}/list      → []BroadcastDTO (paginated)
//	GET /rest/v2/cluster/nodes              → []ClusterNodeDTO
//	GET /rest/v2/broadcasts/{app}/{id}/webrtc-client-stats/0/100 → []WebRTCStatsDTO
//
// WebRTC signaling (WO-B):
//
//	GET /{app}/websocket (WS upgrade)  — on {"command":"play","streamId":"..."}
//	    Phase-1 (default, -webrtc-ice=false): replies with a static offer and closes.
//	    Phase-2a (-webrtc-ice=true): pion offerer, full ICE candidate exchange,
//	                                 WS kept open until ICE completes or deadline.
//
// Control endpoints (test driver uses these):
//
//	POST /control/publish       {"stream_id":"x","viewers":N[,"bitrate":N]}
//	POST /control/unpublish     {"stream_id":"x"}
//	POST /control/set_viewers   {"stream_id":"x","viewers":N}
//	POST /control/set_bitrate   {"stream_id":"x","bitrate":N}
//	  bitrate is the raw AMS wire value in bits/sec (Pulse's normalize.go divides
//	  by 1000 to produce kbps). Example: 2000000 → 2000 kbps seen by Pulse.
//	  Returns 400 on bad JSON or missing stream_id; 404 if stream not found.
//	POST /control/bulk_publish  {"count":N,"prefix":"str-","viewers_each":0}
//	  Publishes N streams with IDs "<prefix>0001".."<prefix>000N" in a single call.
//	  Returns {"status":"ok","count":N}. 400 on bad JSON or count <= 0.
//	GET  /truth/viewers/{id}    → {"stream_id":"x","viewers":N}  (truth for assertions)
//	GET  /healthz               → {"status":"ok"}
package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// ─── Config ──────────────────────────────────────────────────────────────────

type Config struct {
	Addr      string
	RtmpAddr  string
	WebRTCICE bool // -webrtc-ice: enable pion ICE offerer on /{app}/websocket
	LogDir    string
	Scenario  int
	AppName   string
}

// dashSegmentData is a 50000-byte static payload served by the DASH segment route.
// Deterministic across requests: byte[i] = i % 256.
// Expected bitrate at a 2 s segment duration: 50000*8/2/1000 = 200 kbps.
var dashSegmentData = func() []byte {
	b := make([]byte, 50000)
	for i := range b {
		b[i] = byte(i % 256)
	}
	return b
}()

// ─── Broadcast (AMS wire shape) ───────────────────────────────────────────────

type Broadcast struct {
	StreamID          string  `json:"streamId"`
	Name              string  `json:"name"`
	Status            string  `json:"status"` // broadcasting | finished | created
	Type              string  `json:"type"`
	PublishType       string  `json:"publishType"`
	StartTime         int64   `json:"startTime"`
	EndTime           int64   `json:"endTime"`
	HlsViewerCount    int     `json:"hlsViewerCount"`
	WebRTCViewerCount int     `json:"webRTCViewerCount"`
	RTMPViewerCount   int     `json:"rtmpViewerCount"`
	DashViewerCount   int     `json:"dashViewerCount"`
	Speed             float64 `json:"speed"`
	BitRate           float64 `json:"bitrate"`
	CurrentFPS        int     `json:"currentFPS"`
	AppName           string  `json:"appName"`
}

// ─── State ───────────────────────────────────────────────────────────────────

type State struct {
	mu         sync.RWMutex
	broadcasts map[string]*Broadcast
	appName    string
	logFile    *os.File
}

func NewState(appName string) *State {
	return &State{
		broadcasts: make(map[string]*Broadcast),
		appName:    appName,
	}
}

// Publish adds or updates a broadcast to "broadcasting".
func (s *State) Publish(id string, viewers int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	if b, ok := s.broadcasts[id]; ok {
		b.Status = "broadcasting"
		b.WebRTCViewerCount = viewers
		b.HlsViewerCount = viewers / 3
	} else {
		s.broadcasts[id] = &Broadcast{
			StreamID:          id,
			Name:              id,
			Status:            "broadcasting",
			Type:              "liveStream",
			PublishType:       "webrtc",
			StartTime:         now,
			WebRTCViewerCount: viewers,
			HlsViewerCount:    viewers / 3,
			BitRate:           2000,
			CurrentFPS:        30,
			AppName:           s.appName,
		}
	}
	s.writeLogEvent("stream_publish_start", id, viewers)
}

// Unpublish marks a broadcast as finished.
func (s *State) Unpublish(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.broadcasts[id]; ok {
		b.Status = "finished"
		b.EndTime = time.Now().UnixMilli()
		s.writeLogEvent("stream_publish_end", id, 0)
	}
}

// SetViewers updates viewer counts.
func (s *State) SetViewers(id string, viewers int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.broadcasts[id]; ok {
		b.WebRTCViewerCount = viewers
		b.HlsViewerCount = viewers / 3
		s.writeLogEvent("stream_stats", id, viewers)
	}
}

// SetBitRate updates the BitRate field for a stream.
// bitrate is the raw AMS wire value in bits/sec — Pulse's normalize.go divides by 1000
// to produce kbps (e.g. pass 2000000 to produce 2000 kbps as seen by Pulse).
// Returns false if the stream does not exist.
func (s *State) SetBitRate(id string, bitrate float64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.broadcasts[id]; ok {
		b.BitRate = bitrate
		return true
	}
	return false
}

// TruthViewers returns the true total viewer count for a stream (for assertions).
func (s *State) TruthViewers(id string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if b, ok := s.broadcasts[id]; ok {
		return b.WebRTCViewerCount + b.HlsViewerCount + b.RTMPViewerCount
	}
	return 0
}

// List returns a copy of all broadcasting streams (non-finished).
func (s *State) List() []Broadcast {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Broadcast
	for _, b := range s.broadcasts {
		cp := *b
		result = append(result, cp)
	}
	return result
}

// ListActive returns only "broadcasting" streams.
func (s *State) ListActive() []Broadcast {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Broadcast
	for _, b := range s.broadcasts {
		if b.Status == "broadcasting" {
			cp := *b
			result = append(result, cp)
		}
	}
	return result
}

func (s *State) writeLogEvent(eventType, streamID string, viewers int) {
	if s.logFile == nil {
		return
	}
	event := map[string]any{
		"type":      eventType,
		"stream_id": streamID,
		"app":       s.appName,
		"ts":        time.Now().UnixMilli(),
		"data": map[string]any{
			"viewer_count": viewers,
		},
	}
	line, _ := json.Marshal(event)
	_, _ = fmt.Fprintf(s.logFile, "%s\n", string(line))
}

// ─── HTTP Handlers ────────────────────────────────────────────────────────────

type Server struct {
	cfg   Config
	state *State
	mux   *http.ServeMux
}

func NewServer(cfg Config, state *State) *Server {
	s := &Server{cfg: cfg, state: state, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	// GET /rest/v2/applications
	// amsclient.ListApplications expects: {"applications":[{"name":"live"}]}
	s.mux.HandleFunc("/rest/v2/applications", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"applications": []map[string]string{{"name": s.cfg.AppName}},
		})
	})

	// broadcastsHandler handles all /rest/v2/broadcasts/... sub-paths regardless of
	// whether the URL includes the app prefix (/{app}/rest/v2/broadcasts/...) or not
	// (/rest/v2/broadcasts/...). Path matching uses strings.Contains so it is
	// insensitive to offset/size suffixes and app-name placement.
	broadcastsHandler := func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Match .../list or .../list/{offset}/{size}
		if strings.Contains(path, "/list") {
			broadcasts := s.state.List()
			if broadcasts == nil {
				broadcasts = []Broadcast{}
			}
			// Sort by StreamID for deterministic pagination across separate HTTP requests.
			// Go map iteration is non-deterministic: without sorting, page 0/page 1/page 2
			// each iterate in a different order and the union may not cover all streams.
			sort.Slice(broadcasts, func(i, j int) bool {
				return broadcasts[i].StreamID < broadcasts[j].StreamID
			})
			// Parse optional {offset}/{size} from path suffix: .../list/{offset}/{size}
			// Default size=200 matches amsclient pageSize; bare /list keeps working.
			offset, size := 0, 200
			if parts := strings.SplitN(path, "/list/", 2); len(parts) == 2 {
				fmt.Sscanf(parts[1], "%d/%d", &offset, &size)
			}
			if size <= 0 {
				size = 200
			}
			if offset >= len(broadcasts) {
				writeJSON(w, []Broadcast{})
				return
			}
			end := offset + size
			if end > len(broadcasts) {
				end = len(broadcasts)
			}
			writeJSON(w, broadcasts[offset:end])
			return
		}
		// Match .../{streamID}/webrtc-client-stats/0/100
		if strings.Contains(path, "/webrtc-client-stats") {
			writeJSON(w, []map[string]any{})
			return
		}
		// Match .../{streamID}/broadcast-statistics
		if strings.Contains(path, "/statistics") {
			writeJSON(w, map[string]any{
				"totalHLSWatchTime":      0,
				"totalWebRTCWatchTime":   0,
				"totalHlsViewerCount":    0,
				"totalWebRTCViewerCount": 0,
			})
			return
		}
		writeJSON(w, []map[string]any{})
	}

	// App-prefixed route: /{app}/rest/v2/broadcasts/...
	// amsclient (D-029+) calls /{app}/rest/v2/broadcasts/list/{offset}/{size} and
	// /{app}/rest/v2/broadcasts/{streamID}/webrtc-client-stats/0/100.
	s.mux.HandleFunc("/"+s.cfg.AppName+"/rest/v2/broadcasts/", broadcastsHandler)

	// Legacy un-prefixed route: /rest/v2/broadcasts/...
	// Kept for backward compatibility with older test drivers and direct curl usage.
	s.mux.HandleFunc("/rest/v2/broadcasts/", broadcastsHandler)

	// GET /rest/v2/cluster/nodes
	// amsclient.ClusterNodes expects: []ClusterNodeDTO
	s.mux.HandleFunc("/rest/v2/cluster/nodes", func(w http.ResponseWriter, r *http.Request) {
		active := s.state.ListActive()
		writeJSON(w, []map[string]any{
			{
				"nodeId":            "standalone",
				"ip":                "127.0.0.1",
				"port":              9090,
				"role":              "origin",
				"cpuUsage":          15.0,
				"memoryUsage":       40.0,
				"diskUsage":         20.0,
				"networkInputBps":   1024.0,
				"networkOutputBps":  2048.0,
				"jvmMemoryUsage":    25.0,
				"activeStreamCount": len(active),
			},
		})
	})

	// WebRTC signaling handler.
	// Handles WS upgrade on /{app}/websocket (any app prefix, e.g. /live/websocket).
	// Path pattern: "/{app}/websocket" where {app} matches the configured AppName.
	//
	// Phase-1 (default, -webrtc-ice=false):
	//   On {"command":"play","streamId":"..."} → replies with a static
	//   takeConfiguration/offer and closes the WS.  Deterministic and instant.
	//
	// Phase-2a (-webrtc-ice=true):
	//   Delegates to runPionOfferer (webrtc_ice.go): real pion offer, full
	//   candidate exchange, WS kept open until ICE completes or deadline fires.
	wsSignalingHandler := func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true, // CI/local: no cert verification needed
		})
		if err != nil {
			log.Printf("mock-ams: ws accept error: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")

		// 10 s is enough for the play-command read in both modes.
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		// Read the play command.
		var playMsg map[string]json.RawMessage
		if err := wsjson.Read(ctx, conn, &playMsg); err != nil {
			log.Printf("mock-ams: ws read error: %v", err)
			return
		}

		// Extract command and streamId.
		var cmd, streamID string
		if raw, ok := playMsg["command"]; ok {
			_ = json.Unmarshal(raw, &cmd)
		}
		if raw, ok := playMsg["streamId"]; ok {
			_ = json.Unmarshal(raw, &streamID)
		}

		if cmd != "play" {
			log.Printf("mock-ams: ws unexpected command %q (expected play)", cmd)
			return
		}

		// Real AMS 3.0.3 sends a notification (subtrackAdded) BEFORE the offer
		// (live-captured 2026-07-10, D-074) — mirror it in BOTH modes so CI
		// exercises the probe's notification-skip path.
		notification := map[string]interface{}{
			"command":    "notification",
			"definition": "subtrackAdded",
			"trackId":    streamID,
			"mainTrack":  streamID,
		}
		if err := wsjson.Write(r.Context(), conn, notification); err != nil {
			log.Printf("mock-ams: ws signaling: send notification: %v", err)
			return
		}

		// Phase-2a: delegate to pion offerer (keeps WS open for ICE exchange).
		if s.cfg.WebRTCICE {
			runPionOfferer(r.Context(), conn, streamID)
			return
		}

		// Phase-1: reply with a static, minimal-but-valid AMS-shaped offer and
		// close immediately (from real-AMS capture, see
		// agents/handoffs/real-ams-captures/webrtc-signaling-play-offer.json).
		offer := map[string]interface{}{
			"command":  "takeConfiguration",
			"streamId": streamID,
			"type":     "offer",
			"sdp": "v=0\r\n" +
				"o=- 4611731400430051336 2 IN IP4 127.0.0.1\r\n" +
				"s=-\r\n" +
				"t=0 0\r\n" +
				"a=group:BUNDLE 0\r\n" +
				"m=video 9 UDP/TLS/RTP/SAVPF 96\r\n" +
				"c=IN IP4 0.0.0.0\r\n" +
				"a=rtcp:9 IN IP4 0.0.0.0\r\n" +
				"a=ice-ufrag:mock\r\n" +
				"a=ice-pwd:mockpassword12345678901\r\n" +
				"a=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\n" +
				"a=setup:actpass\r\n" +
				"a=mid:0\r\n" +
				"a=recvonly\r\n" +
				"a=rtcp-mux\r\n" +
				"a=rtpmap:96 VP8/90000\r\n",
		}
		if err := wsjson.Write(ctx, conn, offer); err != nil {
			log.Printf("mock-ams: ws write offer error: %v", err)
			return
		}
		log.Printf("mock-ams: ws signaling: sent takeConfiguration/offer for streamId=%q", streamID)
	}
	s.mux.HandleFunc("/"+s.cfg.AppName+"/websocket", wsSignalingHandler)

	// DASH MPD + segment routes — AMS URL convention /{app}/streams/{streamId}.mpd
	// and /{app}/streams/{streamId}-seg-N.m4s.
	//
	// MPD: timescale=90000, duration=180000 (= 2 s per segment at 90 kHz), startNumber=1.
	// Segment: exactly 50000 bytes (bitrate = 50000*8/2/1000 = 200 kbps).
	// Media URL is relative to the MPD directory so a standards-based resolver
	// lands on the same /{app}/streams/ prefix.
	// Unknown streamIds return 200 (consistent with other read-only mock routes).
	streamsPrefix := "/" + s.cfg.AppName + "/streams/"
	s.mux.HandleFunc(streamsPrefix, func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, streamsPrefix)
		switch {
		case strings.HasSuffix(rest, ".mpd"):
			streamID := strings.TrimSuffix(rest, ".mpd")
			w.Header().Set("Content-Type", "application/dash+xml")
			mpd := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT4S" minBufferTime="PT2S" profiles="urn:mpeg:dash:profile:isoff-on-demand:2011">
  <Period duration="PT4S">
    <AdaptationSet mimeType="video/mp4" segmentAlignment="true">
      <Representation id="1" bandwidth="200000">
        <SegmentTemplate timescale="90000" duration="180000" startNumber="1" media="%s-seg-$Number$.m4s"/>
      </Representation>
    </AdaptationSet>
  </Period>
</MPD>
`, streamID)
			_, _ = w.Write([]byte(mpd))
		case strings.HasSuffix(rest, ".m4s"):
			w.Header().Set("Content-Type", "video/iso.segment")
			_, _ = w.Write(dashSegmentData)
		default:
			http.NotFound(w, r)
		}
	})

	// Health check for mock itself.
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// GET /truth/viewers/{id} — returns true viewer counts for test assertions.
	s.mux.HandleFunc("/truth/viewers/", func(w http.ResponseWriter, r *http.Request) {
		id := filepath.Base(r.URL.Path)
		writeJSON(w, map[string]any{
			"stream_id": id,
			"viewers":   s.state.TruthViewers(id),
		})
	})

	// POST /control/publish — drive scenario streams.
	// Optional "bitrate" field (bits/sec, AMS wire value) defaults to 2000 for backward compat.
	s.mux.HandleFunc("/control/publish", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			StreamID string  `json:"stream_id"`
			Viewers  int     `json:"viewers"`
			BitRate  float64 `json:"bitrate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.StreamID == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s.state.Publish(body.StreamID, body.Viewers)
		if body.BitRate != 0 {
			s.state.SetBitRate(body.StreamID, body.BitRate)
		}
		log.Printf("mock-ams: published %s viewers=%d bitrate=%.0f", body.StreamID, body.Viewers, body.BitRate)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// POST /control/unpublish
	s.mux.HandleFunc("/control/unpublish", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			StreamID string `json:"stream_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.StreamID == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s.state.Unpublish(body.StreamID)
		log.Printf("mock-ams: unpublished %s", body.StreamID)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// POST /control/set_viewers
	s.mux.HandleFunc("/control/set_viewers", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			StreamID string `json:"stream_id"`
			Viewers  int    `json:"viewers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.StreamID == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s.state.SetViewers(body.StreamID, body.Viewers)
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// POST /control/set_bitrate — update the AMS wire bitrate for a published stream.
	// "bitrate" is in bits/sec (raw AMS wire value); Pulse's normalize.go divides by 1000
	// to produce kbps. Example: {"stream_id":"s1","bitrate":2000000} → 2000 kbps in Pulse.
	// 400 on bad JSON or missing stream_id; 404 if the stream does not exist.
	s.mux.HandleFunc("/control/set_bitrate", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			StreamID string  `json:"stream_id"`
			BitRate  float64 `json:"bitrate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.StreamID == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if !s.state.SetBitRate(body.StreamID, body.BitRate) {
			http.Error(w, "stream not found", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// POST /control/bulk_publish — seed N streams in a single HTTP call.
	// Body: {"count":N,"prefix":"str-","viewers_each":0}
	// Each stream is published with ID "<prefix><zero-padded-index>" (4 digits).
	// Returns {"status":"ok","count":N}. 400 on bad JSON or count <= 0.
	// Each Publish call acquires the state lock individually (no long-held bulk lock).
	s.mux.HandleFunc("/control/bulk_publish", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Count       int    `json:"count"`
			Prefix      string `json:"prefix"`
			ViewersEach int    `json:"viewers_each"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Count <= 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if body.Prefix == "" {
			body.Prefix = "stream-"
		}
		for i := 1; i <= body.Count; i++ {
			s.state.Publish(fmt.Sprintf("%s%04d", body.Prefix, i), body.ViewersEach)
		}
		log.Printf("mock-ams: bulk published %d streams (prefix=%q)", body.Count, body.Prefix)
		writeJSON(w, map[string]any{"status": "ok", "count": body.Count})
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ─── RTMP Handshake + AMF0 Connect Listener ──────────────────────────────────

// startRTMPListener binds addr, logs the effective address, then calls
// serveRTMPOnListener. Calls log.Fatalf on bind failure; call in a goroutine.
func startRTMPListener(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("mock-ams: rtmp listen %s: %v", addr, err)
	}
	log.Printf("mock-ams: RTMP listener on %s", ln.Addr())
	serveRTMPOnListener(ln)
}

// serveRTMPOnListener accepts connections from ln and spawns a goroutine per
// connection. Returns when ln is closed.
func serveRTMPOnListener(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleRTMPConn(conn)
	}
}

// handleRTMPConn completes an RTMP handshake and processes an optional AMF0
// connect command, replying with _result or _error.
//
// Protocol:
//
//	C0 (1 B, must be 0x03) + C1 (1536 B)
//	→ S0 (0x03) + S1 (1536 B) + S2 (echo of C1)
//	→ C2 (1536 B)
//	→ AMF0 connect command (optional, if app segment in URL)
//	  → _result (app != "rejected") or _error (app == "rejected")
//	→ close
//
// Deterministic rejection hook: when the connect command carries app="rejected",
// the mock replies with an AMF0 _error so probe tests can exercise that path.
//
// A bad C0 version byte or any I/O error causes immediate close without reply.
func handleRTMPConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	// Read C0: version byte must be 0x03.
	var c0 [1]byte
	if _, err := io.ReadFull(conn, c0[:]); err != nil || c0[0] != 0x03 {
		return
	}

	// Read C1: 4-byte timestamp + 4 zero bytes + 1528 random bytes.
	var c1 [1536]byte
	if _, err := io.ReadFull(conn, c1[:]); err != nil {
		return
	}

	// Build response: S0 (1 B) + S1 (1536 B) + S2 (1536 B) = 3073 B.
	var response [3073]byte

	// S0.
	response[0] = 0x03

	// S1: 4-byte BE timestamp + 4 zero bytes + 1528 random bytes.
	binary.BigEndian.PutUint32(response[1:5], uint32(time.Now().UnixMilli()))
	// response[5:9] remains zero.
	if _, err := io.ReadFull(rand.Reader, response[9:1537]); err != nil {
		return
	}

	// S2 echoes C1: C1 timestamp | ack time | C1's 1528 random bytes.
	copy(response[1537:1541], c1[0:4])
	binary.BigEndian.PutUint32(response[1541:1545], uint32(time.Now().UnixMilli()))
	copy(response[1545:3073], c1[8:1536])

	if _, err := conn.Write(response[:]); err != nil {
		return
	}

	// Read C2 (client echo of S1).
	var c2 [1536]byte
	if _, err := io.ReadFull(conn, c2[:]); err != nil {
		return
	}

	log.Printf("mock-ams: rtmp handshake complete from %s", conn.RemoteAddr())

	// Read the optional AMF0 connect command. If none arrives within the deadline
	// (e.g. probe only does handshake — no-app URL), the read will timeout and
	// we close silently.
	app, err := rtmpReadAMF0App(conn)
	if err != nil {
		// Client didn't send connect, or I/O error — close quietly.
		return
	}

	// Deterministic rejection hook: app name "rejected" → _error.
	cmdName := "_result"
	if app == "rejected" {
		cmdName = "_error"
	}

	if err := rtmpSendAMF0Response(conn, cmdName); err != nil {
		return
	}
	log.Printf("mock-ams: rtmp AMF0 connect: app=%q → %s from %s", app, cmdName, conn.RemoteAddr())
}

// rtmpReadAMF0App reads one RTMP chunk carrying the AMF0 "connect" command from
// conn and returns the value of the "app" property from the command object.
// Returns ("", error) if the chunk cannot be read or parsed.
// Handles the prober's chunked format: fmt=0 first chunk + fmt=3 continuations
// at 128-byte boundaries.
func rtmpReadAMF0App(conn net.Conn) (string, error) {
	// Basic header (1 byte: fmt=0, csid=3 → 0x03)
	var bh [1]byte
	if _, err := io.ReadFull(conn, bh[:]); err != nil {
		return "", fmt.Errorf("rtmp: connect basic header: %v", err)
	}

	// Message header (11 bytes for fmt=0)
	var mh [11]byte
	if _, err := io.ReadFull(conn, mh[:]); err != nil {
		return "", fmt.Errorf("rtmp: connect message header: %v", err)
	}
	msgLen := int(mh[3])<<16 | int(mh[4])<<8 | int(mh[5])
	if msgLen <= 0 || msgLen > 4096 {
		return "", fmt.Errorf("rtmp: connect payload length %d out of range", msgLen)
	}

	// Read payload, possibly fragmented at 128-byte chunk boundaries.
	payload := make([]byte, 0, msgLen)
	rem := msgLen
	first := true
	for rem > 0 {
		if !first {
			// Continuation chunk basic header (fmt=3, 1 byte)
			var cb [1]byte
			if _, err := io.ReadFull(conn, cb[:]); err != nil {
				return "", fmt.Errorf("rtmp: connect continuation header: %v", err)
			}
		}
		first = false
		n := 128
		if n > rem {
			n = rem
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", fmt.Errorf("rtmp: connect payload: %v", err)
		}
		payload = append(payload, buf...)
		rem -= n
	}

	return rtmpAMF0ExtractApp(payload)
}

// rtmpAMF0ExtractApp parses the AMF0 payload of a "connect" command and returns
// the "app" property value. The payload structure is:
//
//	string "connect" + number txid + object {app:string, tcUrl:string, ...} + end
func rtmpAMF0ExtractApp(payload []byte) (string, error) {
	// Skip command name string (type 0x02 + uint16 len + bytes)
	if len(payload) < 3 || payload[0] != 0x02 {
		return "", fmt.Errorf("amf0: expected string type at offset 0")
	}
	cmdLen := int(binary.BigEndian.Uint16(payload[1:3]))
	offset := 3 + cmdLen

	// Skip txid number (type 0x00 + 8 bytes)
	if offset+9 > len(payload) || payload[offset] != 0x00 {
		return "", fmt.Errorf("amf0: expected number type for txid at offset %d", offset)
	}
	offset += 9

	// Expect AMF0 object (type 0x03)
	if offset >= len(payload) || payload[offset] != 0x03 {
		return "", fmt.Errorf("amf0: expected object type 0x03 at offset %d", offset)
	}
	offset++

	// Iterate properties until end marker 0x00 0x00 0x09
	for offset+2 < len(payload) {
		if payload[offset] == 0x00 && payload[offset+1] == 0x00 && payload[offset+2] == 0x09 {
			break // end marker
		}

		// Key: uint16 length + bytes (no type byte)
		if offset+2 > len(payload) {
			return "", fmt.Errorf("amf0: key length truncated at offset %d", offset)
		}
		keyLen := int(binary.BigEndian.Uint16(payload[offset : offset+2]))
		offset += 2
		if offset+keyLen > len(payload) {
			return "", fmt.Errorf("amf0: key data truncated")
		}
		key := string(payload[offset : offset+keyLen])
		offset += keyLen

		// Value: type byte + data
		if offset >= len(payload) {
			return "", fmt.Errorf("amf0: value type missing for key %q", key)
		}
		valType := payload[offset]
		offset++

		switch valType {
		case 0x02: // string
			if offset+2 > len(payload) {
				return "", fmt.Errorf("amf0: string length truncated for key %q", key)
			}
			vLen := int(binary.BigEndian.Uint16(payload[offset : offset+2]))
			offset += 2
			if offset+vLen > len(payload) {
				return "", fmt.Errorf("amf0: string data truncated for key %q", key)
			}
			val := string(payload[offset : offset+vLen])
			offset += vLen
			if key == "app" {
				return val, nil
			}
		case 0x00: // number (8 bytes)
			offset += 8
		case 0x01: // boolean (1 byte)
			offset++
		case 0x05: // null (no data)
		default:
			// Unknown type — cannot parse further; "app" not found
			return "", nil
		}
	}
	return "", nil // "app" key not present
}

// rtmpSendAMF0Response sends a minimal AMF0 command chunk (cmdName, txid 1.0, null info).
// Single chunk: fmt=0, csid=3, type 0x14 — payload always fits in 128 bytes.
func rtmpSendAMF0Response(conn net.Conn, cmdName string) error {
	var payload []byte
	// Command name string
	payload = append(payload, 0x02, byte(len(cmdName)>>8), byte(len(cmdName)))
	payload = append(payload, []byte(cmdName)...)
	// txid number (1.0 = 0x3FF0000000000000 IEEE 754 BE)
	payload = append(payload, 0x00, 0x3F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
	// null (command info placeholder)
	payload = append(payload, 0x05)

	msgLen := len(payload)
	chunk := make([]byte, 1+11+msgLen)
	chunk[0] = 0x03 // fmt=0, csid=3
	chunk[1], chunk[2], chunk[3] = 0, 0, 0
	chunk[4] = byte(msgLen >> 16)
	chunk[5] = byte(msgLen >> 8)
	chunk[6] = byte(msgLen)
	chunk[7] = 0x14 // AMF0 command
	chunk[8], chunk[9], chunk[10], chunk[11] = 0, 0, 0, 0
	copy(chunk[12:], payload)

	_, err := conn.Write(chunk)
	return err
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.Addr, "addr", ":9090", "listen address")
	flag.StringVar(&cfg.RtmpAddr, "rtmp-addr", "", "TCP address for RTMP handshake listener (empty = disabled)")
	flag.BoolVar(&cfg.WebRTCICE, "webrtc-ice", false, "enable pion WebRTC ICE offerer on /{app}/websocket (default off = phase-1 static offer)")
	flag.StringVar(&cfg.LogDir, "log-dir", "", "directory to write AMS analytics log (empty = disable)")
	flag.IntVar(&cfg.Scenario, "scenario", 0, "auto-run scenario (0 = manual control only)")
	flag.StringVar(&cfg.AppName, "app", "live", "AMS application name")
	flag.Parse()

	state := NewState(cfg.AppName)

	// Open log file if requested.
	if cfg.LogDir != "" {
		if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
			log.Fatalf("mock-ams: create log dir: %v", err)
		}
		logPath := filepath.Join(cfg.LogDir, "ant-media-server-analytics.log")
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("mock-ams: open log: %v", err)
		}
		defer f.Close()
		state.logFile = f
		log.Printf("mock-ams: writing analytics log to %s", logPath)
	}

	if cfg.RtmpAddr != "" {
		go startRTMPListener(cfg.RtmpAddr)
	}

	srv := NewServer(cfg, state)

	log.Printf("mock-ams: listening on %s (app=%s)", cfg.Addr, cfg.AppName)
	if err := http.ListenAndServe(cfg.Addr, srv); err != nil {
		log.Fatalf("mock-ams: %v", err)
	}
}
