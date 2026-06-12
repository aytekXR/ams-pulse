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
// Control endpoints (test driver uses these):
//
//	POST /control/publish       {"stream_id":"x","viewers":N}
//	POST /control/unpublish     {"stream_id":"x"}
//	POST /control/set_viewers   {"stream_id":"x","viewers":N}
//	GET  /truth/viewers/{id}    → {"stream_id":"x","viewers":N}  (truth for assertions)
//	GET  /healthz               → {"status":"ok"}
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─── Config ──────────────────────────────────────────────────────────────────

type Config struct {
	Addr     string
	LogDir   string
	Scenario int
	AppName  string
}

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

	// GET /rest/v2/broadcasts/{app}/list (paged)
	// amsclient.ListBroadcastsPaged / ListBroadcasts expects: []BroadcastDTO
	s.mux.HandleFunc("/rest/v2/broadcasts/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Match /rest/v2/broadcasts/{app}/list or /rest/v2/broadcasts/{app}/list/0/200
		if strings.Contains(path, "/list") {
			broadcasts := s.state.List()
			if broadcasts == nil {
				broadcasts = []Broadcast{}
			}
			writeJSON(w, broadcasts)
			return
		}
		// Match /rest/v2/broadcasts/{app}/{streamID}/webrtc-client-stats/0/100
		if strings.Contains(path, "/webrtc-client-stats") {
			writeJSON(w, []map[string]any{})
			return
		}
		// Match /rest/v2/broadcasts/{app}/{streamID}/statistics
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
	})

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
	s.mux.HandleFunc("/control/publish", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			StreamID string `json:"stream_id"`
			Viewers  int    `json:"viewers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.StreamID == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s.state.Publish(body.StreamID, body.Viewers)
		log.Printf("mock-ams: published %s viewers=%d", body.StreamID, body.Viewers)
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
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.Addr, "addr", ":9090", "listen address")
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

	srv := NewServer(cfg, state)

	log.Printf("mock-ams: listening on %s (app=%s)", cfg.Addr, cfg.AppName)
	if err := http.ListenAndServe(cfg.Addr, srv); err != nil {
		log.Fatalf("mock-ams: %v", err)
	}
}
