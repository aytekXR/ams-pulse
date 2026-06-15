// Package amsclient is a typed, read-only client for the Ant Media Server REST
// API v2 (broadcasts, broadcast-statistics, WebRTC client stats, cluster nodes,
// applications). It is the ONLY package allowed to speak AMS wire formats for
// REST; raw responses are translated to domain types at this boundary.
//
// In pkg/ (not internal/) deliberately: it is a candidate for open-sourcing
// alongside the beacon SDK as community surface area (PRD §7.12 GTM).
//
// Compatibility: AMS API/log formats vary across versions (PRD §7.13 risk).
// This package carries a version matrix and is exercised by the
// ams-version-matrix CI workflow against released AMS containers.
package amsclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ─── Wire DTOs (AMS REST API v2 response shapes) ─────────────────────────────
// These are AMS-specific. Nothing downstream should import these types.
// The collector translates them to domain types.

// BroadcastDTO is the AMS REST v2 broadcast object.
// Fields are tolerant: unknown fields are ignored, missing fields zero.
type BroadcastDTO struct {
	StreamID        string  `json:"streamId"`
	Name            string  `json:"name"`
	Status          string  `json:"status"`      // created|broadcasting|finished
	Type            string  `json:"type"`        // liveStream|ipCamera|streamSource|VOD
	PublishType     string  `json:"publishType"` // webrtc|rtmp|hls
	StartTime       int64   `json:"startTime"`   // Unix epoch ms
	EndTime         int64   `json:"endTime"`
	HlsViewerCount  int     `json:"hlsViewerCount"`
	WebRTCViewerCount int   `json:"webRTCViewerCount"`
	RTMPViewerCount int     `json:"rtmpViewerCount"`
	DashViewerCount int     `json:"dashViewerCount"`
	Speed           float64 `json:"speed"` // read speed kbps
	BitRate         float64 `json:"bitrate"`
	CurrentFPS      int     `json:"currentFPS"`
	AppName         string  `json:"appName"` // populated from API path context
}

// BroadcastListResponse wraps the paged list response.
type BroadcastListResponse []BroadcastDTO

// BroadcastStatisticsDTO holds per-broadcast stats from the statistics endpoint.
type BroadcastStatisticsDTO struct {
	TotalHLSWatchTime   int64 `json:"totalHLSWatchTime"`
	TotalWebRTCWatchTime int64 `json:"totalWebRTCWatchTime"`
	TotalPlaylistWatchTime int64 `json:"totalPlaylistWatchTime"`
	TotalHLSViewerCount  int  `json:"totalHlsViewerCount"`
	TotalWebRTCViewerCount int `json:"totalWebRTCViewerCount"`
}

// WebRTCClientStatsDTO holds per-WebRTC-peer quality stats.
type WebRTCClientStatsDTO struct {
	StatID         string  `json:"statId"`
	VideoRoundTripTime float64 `json:"videoRoundTripTime"`
	AudioRoundTripTime float64 `json:"audioRoundTripTime"`
	VideoJitter    float64 `json:"videoJitter"`
	AudioJitter    float64 `json:"audioJitter"`
	VideoPacketLostRatio float64 `json:"videoPacketLostRatio"`
	AudioPacketLostRatio float64 `json:"audioPacketLostRatio"`
	OutboundRtpList []map[string]any `json:"outboundRtpList"`
	InboundRtpList  []map[string]any `json:"inboundRtpList"`
}

// ClusterNodeDTO is one entry in the cluster nodes list.
type ClusterNodeDTO struct {
	NodeID    string `json:"nodeId"`
	IP        string `json:"ip"`
	Port      int    `json:"port"`
	Role      string `json:"role"` // origin|edge
	// Version is the AMS server version string (e.g. "2.8.3") returned by
	// the cluster nodes endpoint. VD-40: populated so FleetPage can render it.
	Version   string  `json:"version"`
	CPUUsage  float64 `json:"cpuUsage"`
	MemoryUsage float64 `json:"memoryUsage"`
	DiskUsage float64 `json:"diskUsage"`
	NetworkInputBps  float64 `json:"networkInputBps"`
	NetworkOutputBps float64 `json:"networkOutputBps"`
	JvmMemoryUsage   float64 `json:"jvmMemoryUsage"`
	ActiveStreamCount int    `json:"activeStreamCount"`
}

// ApplicationDTO represents one AMS application.
type ApplicationDTO struct {
	Name string `json:"name"`
}

// ─── Client ──────────────────────────────────────────────────────────────────

// Client talks to one AMS node's REST API v2.
// All methods are read-only, context-aware and tolerant of unknown/missing
// JSON fields. Bearer token auth is optional.
type Client struct {
	baseURL    string
	httpClient *http.Client
	authHeader string // "Bearer <token>" or empty
}

// Config holds Client construction options.
type Config struct {
	// BaseURL is the AMS REST base, e.g. "http://localhost:5080".
	// Trailing slash is normalized.
	BaseURL string

	// AuthToken is the AMS JWT/bearer token (optional — some local AMS installs
	// have auth disabled).
	AuthToken string

	// Timeout for individual HTTP requests. Default: 10 s.
	Timeout time.Duration
}

// New creates an amsclient.Client.
func New(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	// Normalize trailing slash
	base := cfg.BaseURL
	if len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	auth := ""
	if cfg.AuthToken != "" {
		auth = "Bearer " + cfg.AuthToken
	}
	return &Client{
		baseURL: base,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		authHeader: auth,
	}
}

// BaseURL returns the base URL for this client (useful for logging).
func (c *Client) BaseURL() string { return c.baseURL }

// ─── Internal helpers ─────────────────────────────────────────────────────────

// getJSON performs a GET request with tolerant JSON decoding.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("amsclient: build request: %w", err)
	}
	if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("amsclient: GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("amsclient: GET %s: HTTP %d: %s", path, resp.StatusCode, body)
	}

	// Use standard Decoder — unknown fields silently ignored (tolerant).
	return json.NewDecoder(resp.Body).Decode(out)
}

// ─── API methods ──────────────────────────────────────────────────────────────

// ListApplications returns all application names on this AMS node.
func (c *Client) ListApplications(ctx context.Context) ([]string, error) {
	var payload struct {
		Applications []ApplicationDTO `json:"applications"`
	}
	if err := c.getJSON(ctx, "/rest/v2/applications", nil, &payload); err != nil {
		return nil, err
	}
	names := make([]string, len(payload.Applications))
	for i, a := range payload.Applications {
		names[i] = a.Name
	}
	return names, nil
}

// ListBroadcasts returns all broadcasts for an application, paged.
// offset/size follow AMS pagination; size=0 uses AMS default (typically 50).
func (c *Client) ListBroadcasts(ctx context.Context, app string, offset, size int) ([]BroadcastDTO, error) {
	q := url.Values{}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	if size > 0 {
		q.Set("size", strconv.Itoa(size))
	}
	path := fmt.Sprintf("/rest/v2/broadcasts/%s/list", app)
	if offset == 0 && size == 0 {
		path = fmt.Sprintf("/rest/v2/broadcasts/%s/list/0/200", app)
		q = nil
	}
	var result []BroadcastDTO
	if err := c.getJSON(ctx, path, q, &result); err != nil {
		return nil, err
	}
	// Backfill AppName from the request context.
	for i := range result {
		result[i].AppName = app
	}
	return result, nil
}

// ListBroadcastsPaged fetches all pages and returns the complete broadcast list.
func (c *Client) ListBroadcastsPaged(ctx context.Context, app string) ([]BroadcastDTO, error) {
	const pageSize = 200
	var all []BroadcastDTO
	offset := 0
	for {
		q := url.Values{}
		q.Set("offset", strconv.Itoa(offset))
		q.Set("size", strconv.Itoa(pageSize))
		path := fmt.Sprintf("/rest/v2/broadcasts/%s/list", app)
		var page []BroadcastDTO
		if err := c.getJSON(ctx, path, q, &page); err != nil {
			return all, err
		}
		for i := range page {
			page[i].AppName = app
		}
		all = append(all, page...)
		if len(page) < pageSize {
			break
		}
		offset += pageSize
	}
	return all, nil
}

// BroadcastStatistics returns statistics for a specific broadcast.
func (c *Client) BroadcastStatistics(ctx context.Context, app, streamID string) (*BroadcastStatisticsDTO, error) {
	path := fmt.Sprintf("/rest/v2/broadcasts/%s/%s/statistics", app, streamID)
	var result BroadcastStatisticsDTO
	if err := c.getJSON(ctx, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WebRTCClientStats returns per-peer WebRTC quality stats for a stream.
func (c *Client) WebRTCClientStats(ctx context.Context, app, streamID string) ([]WebRTCClientStatsDTO, error) {
	path := fmt.Sprintf("/rest/v2/broadcasts/%s/%s/webrtc-client-stats/0/100", app, streamID)
	var result []WebRTCClientStatsDTO
	if err := c.getJSON(ctx, path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ClusterNodes returns the list of cluster nodes (only meaningful on origin nodes).
func (c *Client) ClusterNodes(ctx context.Context) ([]ClusterNodeDTO, error) {
	var result []ClusterNodeDTO
	if err := c.getJSON(ctx, "/rest/v2/cluster/nodes", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// NodeInfo returns info about one specific node.
func (c *Client) NodeInfo(ctx context.Context, nodeID string) (*ClusterNodeDTO, error) {
	path := fmt.Sprintf("/rest/v2/cluster/nodes/%s", nodeID)
	var result ClusterNodeDTO
	if err := c.getJSON(ctx, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SystemStats returns aggregate system statistics from the AMS node.
// Returns a raw map since the shape varies by AMS version.
func (c *Client) SystemStats(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	if err := c.getJSON(ctx, "/rest/v2/system/stats", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}
