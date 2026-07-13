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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// minLoginInterval is the shortest interval between two forced re-logins.
// A permanent IP-block 403 must not cause a login storm.
const minLoginInterval = 3 * time.Second

// ─── simpleCookieJar ─────────────────────────────────────────────────────────
// The standard net/http/cookiejar (RFC 6265) does not store cookies for bare
// IP addresses, but AMS test environments (and integration tests using httptest)
// are accessed via IP. This minimal jar stores all cookies keyed by
// "scheme://host" and sends them on matching requests, covering both hostnames
// and bare IPs. It is intentionally narrow: no domain/path matching subtleties,
// just enough to carry a JSESSIONID from a login response to subsequent GETs.

type simpleCookieJar struct {
	mu      sync.Mutex
	cookies map[string][]*http.Cookie // key: "scheme://host"
}

func newSimpleCookieJar() *simpleCookieJar {
	return &simpleCookieJar{cookies: make(map[string][]*http.Cookie)}
}

func (j *simpleCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	key := u.Scheme + "://" + u.Host
	j.mu.Lock()
	defer j.mu.Unlock()
	// Merge: replace any cookie with the same name, append new ones.
	existing := j.cookies[key]
	for _, nc := range cookies {
		replaced := false
		for i, ec := range existing {
			if ec.Name == nc.Name {
				existing[i] = nc
				replaced = true
				break
			}
		}
		if !replaced {
			existing = append(existing, nc)
		}
	}
	j.cookies[key] = existing
}

func (j *simpleCookieJar) Cookies(u *url.URL) []*http.Cookie {
	key := u.Scheme + "://" + u.Host
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.cookies[key]
}

// ─── Wire DTOs (AMS REST API v2 response shapes) ─────────────────────────────
// These are AMS-specific. Nothing downstream should import these types.
// The collector translates them to domain types.

// BroadcastDTO is the AMS REST v2 broadcast object.
// Fields are tolerant: unknown fields are ignored, missing fields zero.
type BroadcastDTO struct {
	StreamID          string  `json:"streamId"`
	Name              string  `json:"name"`
	Status            string  `json:"status"`      // created|broadcasting|finished
	Type              string  `json:"type"`        // liveStream|ipCamera|streamSource|VOD
	PublishType       string  `json:"publishType"` // webrtc|rtmp|hls
	StartTime         int64   `json:"startTime"`   // Unix epoch ms
	EndTime           int64   `json:"endTime"`
	HlsViewerCount    int     `json:"hlsViewerCount"`
	WebRTCViewerCount int     `json:"webRTCViewerCount"`
	RTMPViewerCount   int     `json:"rtmpViewerCount"`
	DashViewerCount   int     `json:"dashViewerCount"`
	Speed             float64 `json:"speed"`      // realtime ingest speed RATIO (1.0≈realtime); NOT a bitrate
	BitRate           float64 `json:"bitrate"`    // ingest bitrate in BITS/sec (curl-verified AMS 3.0.3); /1000 → kbps
	CurrentFPS        int     `json:"currentFPS"` // ingest FPS; ABSENT from AMS 3.0.3 REST broadcast list (decodes to 0)
	// Ingest-side QoE fields present in the real AMS 3.0.3 broadcast object
	// (curl-verified 2026-06-21). Units: packetLostRatio is a 0..1 fraction
	// (×100 → percent); jitterMs/rttMs are already in milliseconds.
	PacketLostRatio float64 `json:"packetLostRatio"`
	PacketsLost     int     `json:"packetsLost"`
	JitterMs        float64 `json:"jitterMs"`
	RttMs           float64 `json:"rttMs"`
	AppName         string  `json:"appName"` // populated from API path context
}

// BroadcastListResponse wraps the paged list response.
type BroadcastListResponse []BroadcastDTO

// WebRTCClientStatsDTO holds per-WebRTC-peer quality stats.
type WebRTCClientStatsDTO struct {
	StatID               string           `json:"statId"`
	VideoRoundTripTime   float64          `json:"videoRoundTripTime"`
	AudioRoundTripTime   float64          `json:"audioRoundTripTime"`
	VideoJitter          float64          `json:"videoJitter"`
	AudioJitter          float64          `json:"audioJitter"`
	VideoPacketLostRatio float64          `json:"videoPacketLostRatio"`
	AudioPacketLostRatio float64          `json:"audioPacketLostRatio"`
	OutboundRtpList      []map[string]any `json:"outboundRtpList"`
	InboundRtpList       []map[string]any `json:"inboundRtpList"`
}

// ClusterNodeDTO is one entry in the cluster nodes list.
type ClusterNodeDTO struct {
	NodeID string `json:"nodeId"`
	IP     string `json:"ip"`
	Port   int    `json:"port"`
	Role   string `json:"role"` // origin|edge
	// Version is the AMS server version string (e.g. "2.8.3") returned by
	// the cluster nodes endpoint. VD-40: populated so FleetPage can render it.
	Version           string  `json:"version"`
	CPUUsage          float64 `json:"cpuUsage"`
	MemoryUsage       float64 `json:"memoryUsage"`
	DiskUsage         float64 `json:"diskUsage"`
	NetworkInputBps   float64 `json:"networkInputBps"`
	NetworkOutputBps  float64 `json:"networkOutputBps"`
	JvmMemoryUsage    float64 `json:"jvmMemoryUsage"`
	ActiveStreamCount int     `json:"activeStreamCount"`
}

// ApplicationDTO represents one AMS application (object form, older AMS versions).
type ApplicationDTO struct {
	Name string `json:"name"`
}

// ─── Typed HTTP error ─────────────────────────────────────────────────────────

// httpStatusError is returned by getJSON on non-2xx responses so callers can
// branch on HTTP status (e.g. to make 404 tolerant for standalone AMS nodes).
type httpStatusError struct {
	Path   string
	Status int
	Body   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("amsclient: GET %s: HTTP %d: %s", e.Path, e.Status, e.Body)
}

// ─── Client ──────────────────────────────────────────────────────────────────

// Client talks to one AMS node's REST API v2.
// All methods are read-only, context-aware and tolerant of unknown/missing
// JSON fields. Bearer token auth and/or cookie-session auth are both optional.
type Client struct {
	baseURL    string
	httpClient *http.Client
	authHeader string // "Bearer <token>" or empty

	// Cookie-session auth fields (AMS v3: no JWT, only login + JSESSIONID).
	loginEmail    string
	loginPassword string
	loginMu       sync.Mutex
	lastLogin     time.Time
	loggedIn      bool
}

// Config holds Client construction options.
type Config struct {
	// BaseURL is the AMS REST base, e.g. "http://localhost:5080".
	// Trailing slash is normalized.
	BaseURL string

	// AuthToken is the AMS JWT/bearer token (optional — some local AMS installs
	// have auth disabled).
	AuthToken string

	// LoginEmail and LoginPassword enable cookie-session auth (AMS v3 default,
	// jwtServerControlEnabled=false). When set, the client logs in via
	// POST /rest/v2/users/authenticate and carries the JSESSIONID cookie.
	// Both AuthToken and LoginEmail/Password can coexist.
	LoginEmail    string // AMS console email for cookie-session auth (optional)
	LoginPassword string // AMS console password (optional)

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

	hc := &http.Client{Timeout: timeout}
	if cfg.LoginEmail != "" {
		// Use simpleCookieJar instead of the standard cookiejar: the standard
		// net/http/cookiejar (RFC 6265 §5.1.3) does not store cookies for bare
		// IP addresses, but AMS instances are often accessed by IP (and tests
		// always use httptest IPs). simpleCookieJar covers both cases.
		hc.Jar = newSimpleCookieJar()
	}

	return &Client{
		baseURL:       base,
		httpClient:    hc,
		authHeader:    auth,
		loginEmail:    cfg.LoginEmail,
		loginPassword: cfg.LoginPassword,
	}
}

// BaseURL returns the base URL for this client (useful for logging).
func (c *Client) BaseURL() string { return c.baseURL }

// ─── Cookie-session auth helpers ──────────────────────────────────────────────

// login POSTs to /rest/v2/users/authenticate and checks the JSON body for
// "success". On success the cookie jar (attached in New) carries the JSESSIONID.
func (c *Client) login(ctx context.Context) error {
	body, err := json.Marshal(map[string]string{
		"email":    c.loginEmail,
		"password": c.loginPassword,
	})
	if err != nil {
		return fmt.Errorf("amsclient: marshal login body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/rest/v2/users/authenticate", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("amsclient: build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("amsclient: login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("amsclient: login HTTP %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&result); err != nil {
		return fmt.Errorf("amsclient: decode login response: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("amsclient: login failed (check PULSE_AMS_LOGIN_EMAIL/PASSWORD)")
	}
	// Cookie jar already holds the JSESSIONID from the Set-Cookie header.
	return nil
}

// invalidateSession marks the session as expired so the next ensureLogin
// re-authenticates. Called from getJSON on 401/403 before a forced retry.
func (c *Client) invalidateSession() {
	c.loginMu.Lock()
	c.loggedIn = false
	c.loginMu.Unlock()
}

// ensureLogin is a no-op when loginEmail is empty (bearer-only or no-auth mode).
// Otherwise, mutex-guarded:
//   - If not yet logged in: always logs in (no throttle).
//   - If force=true and already logged in within minLoginInterval: throttled
//     no-op (prevents login storms on permanent IP-block 403s).
//   - If force=true and last login is older than minLoginInterval: re-logs in.
func (c *Client) ensureLogin(ctx context.Context, force bool) error {
	if c.loginEmail == "" {
		return nil
	}
	c.loginMu.Lock()
	defer c.loginMu.Unlock()

	if !c.loggedIn {
		// Not logged in (initial or after session invalidation): always login.
		if err := c.login(ctx); err != nil {
			return err
		}
		c.loggedIn = true
		c.lastLogin = time.Now()
		return nil
	}

	if force {
		if time.Since(c.lastLogin) < minLoginInterval {
			// Throttle: we re-logged in very recently; reuse the existing session
			// rather than hammering login. This caps re-logins at ≤ 2 per
			// minLoginInterval even if 403s keep arriving.
			return nil
		}
		if err := c.login(ctx); err != nil {
			return err
		}
		c.loggedIn = true
		c.lastLogin = time.Now()
	}
	return nil
}

// ─── Internal HTTP helpers ────────────────────────────────────────────────────

// doGet executes a GET request, setting the Bearer header (if any) and
// Accept: application/json. The caller is responsible for closing the body.
func (c *Client) doGet(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("amsclient: build request: %w", err)
	}
	if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("amsclient: GET %s: %w", path, err)
	}
	return resp, nil
}

// getJSON performs a GET request with tolerant JSON decoding.
// It best-effort logs in first (if cookie-session auth is configured).
// On 401 or 403 it attempts a single throttled re-login and retries once.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	// Best-effort login (ignore error — per-app endpoints work without auth).
	_ = c.ensureLogin(ctx, false)

	resp, err := c.doGet(ctx, path)
	if err != nil {
		return err
	}

	// On 401/403 and with cookie-session auth: invalidate the session, attempt
	// one re-login, and retry the GET exactly once. The throttle in ensureLogin
	// prevents a login storm if the 403 is due to a permanent IP block.
	if (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) &&
		c.loginEmail != "" {
		resp.Body.Close()
		// Invalidate so ensureLogin(true) will re-login even within minLoginInterval.
		c.invalidateSession()
		if loginErr := c.ensureLogin(ctx, true); loginErr != nil {
			// Re-login itself failed (wrong credentials etc.): surface that error.
			return loginErr
		}
		resp, err = c.doGet(ctx, path)
		if err != nil {
			return err
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &httpStatusError{Path: path, Status: resp.StatusCode, Body: string(body)}
	}

	// Use standard Decoder — unknown fields silently ignored (tolerant).
	// Limit the body to 10 MB to prevent a rogue AMS response from OOM-ing Pulse.
	return json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(out)
}

// ─── API methods ──────────────────────────────────────────────────────────────

// ListApplications returns all application names on this AMS node.
// Tolerates both AMS v3 string-array form (["LiveApp","live",...]) and
// older object-array form ([{"name":"LiveApp"},...]).
func (c *Client) ListApplications(ctx context.Context) ([]string, error) {
	var payload struct {
		Applications []json.RawMessage `json:"applications"`
	}
	if err := c.getJSON(ctx, "/rest/v2/applications", &payload); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(payload.Applications))
	for _, raw := range payload.Applications {
		if len(raw) == 0 {
			continue
		}
		var name string
		if raw[0] == '"' {
			// AMS v3: plain string element.
			if err := json.Unmarshal(raw, &name); err != nil {
				continue
			}
		} else {
			// Older form: object with "name" field.
			var obj ApplicationDTO
			if err := json.Unmarshal(raw, &obj); err != nil {
				continue
			}
			name = obj.Name
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// ListBroadcasts returns all broadcasts for an application, paged.
// Uses the real AMS v3 path: /{app}/rest/v2/broadcasts/list/{offset}/{size}.
// size<=0 defaults to 200.
func (c *Client) ListBroadcasts(ctx context.Context, app string, offset, size int) ([]BroadcastDTO, error) {
	if size <= 0 {
		size = 200
	}
	path := fmt.Sprintf("/%s/rest/v2/broadcasts/list/%d/%d", app, offset, size)
	var result []BroadcastDTO
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	// Backfill AppName from the request context.
	for i := range result {
		result[i].AppName = app
	}
	return result, nil
}

// ListBroadcastsPaged fetches all pages and returns the complete broadcast list.
// Uses the real AMS v3 path: /{app}/rest/v2/broadcasts/list/{offset}/{pageSize}.
func (c *Client) ListBroadcastsPaged(ctx context.Context, app string) ([]BroadcastDTO, error) {
	const pageSize = 200
	var all []BroadcastDTO
	offset := 0
	for {
		path := fmt.Sprintf("/%s/rest/v2/broadcasts/list/%d/%d", app, offset, pageSize)
		var page []BroadcastDTO
		if err := c.getJSON(ctx, path, &page); err != nil {
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

// WebRTCClientStats returns per-peer WebRTC quality stats for a stream.
// Path: /{app}/rest/v2/broadcasts/{streamID}/webrtc-client-stats/0/100
func (c *Client) WebRTCClientStats(ctx context.Context, app, streamID string) ([]WebRTCClientStatsDTO, error) {
	path := fmt.Sprintf("/%s/rest/v2/broadcasts/%s/webrtc-client-stats/0/100", app, streamID)
	var result []WebRTCClientStatsDTO
	if err := c.getJSON(ctx, path, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ClusterNodes returns the list of cluster nodes (only meaningful on origin nodes).
// Returns (nil, nil) when the AMS node is standalone (404 from the endpoint).
func (c *Client) ClusterNodes(ctx context.Context) ([]ClusterNodeDTO, error) {
	var result []ClusterNodeDTO
	if err := c.getJSON(ctx, "/rest/v2/cluster/nodes", &result); err != nil {
		var hse *httpStatusError
		if errors.As(err, &hse) && hse.Status == http.StatusNotFound {
			return nil, nil // standalone AMS — no cluster endpoint
		}
		return nil, err
	}
	return result, nil
}

// NodeInfo returns info about one specific node.
// Returns (&ClusterNodeDTO{}, nil) when the AMS node is standalone (404).
func (c *Client) NodeInfo(ctx context.Context, nodeID string) (*ClusterNodeDTO, error) {
	path := fmt.Sprintf("/rest/v2/cluster/nodes/%s", nodeID)
	var result ClusterNodeDTO
	if err := c.getJSON(ctx, path, &result); err != nil {
		var hse *httpStatusError
		if errors.As(err, &hse) && hse.Status == http.StatusNotFound {
			return &ClusterNodeDTO{}, nil // standalone AMS — no cluster endpoint
		}
		return nil, err
	}
	return &result, nil
}

// SystemStats returns aggregate system statistics from the AMS node.
// Path: /rest/v2/system-status (real AMS v3 path, curl-verified).
// Returns a raw map since the shape varies by AMS version.
func (c *Client) SystemStats(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	if err := c.getJSON(ctx, "/rest/v2/system-status", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// VersionDTO is the AMS REST v2 version response.
// Real AMS 3.x GET /rest/v2/version returns: versionName, versionType, buildNumber.
type VersionDTO struct {
	VersionName string `json:"versionName"`
	VersionType string `json:"versionType"`
	BuildNumber string `json:"buildNumber"`
}

// GetVersion returns the AMS server version information.
// Path: /rest/v2/version (AMS v3+ endpoint).
// Returns (nil, nil) if the endpoint is unavailable (older AMS versions).
func (c *Client) GetVersion(ctx context.Context) (*VersionDTO, error) {
	var result VersionDTO
	if err := c.getJSON(ctx, "/rest/v2/version", &result); err != nil {
		var hse *httpStatusError
		if errors.As(err, &hse) && (hse.Status == http.StatusNotFound || hse.Status == http.StatusMethodNotAllowed) {
			return nil, nil // older AMS without /rest/v2/version endpoint
		}
		return nil, err
	}
	return &result, nil
}

// VodDTO is the AMS REST v2 VoD list entry.
// Fields are verified against a live capture: GET /pulse-test/rest/v2/vods/list/0/5
// on AMS 3.0.3, 2026-07-12. All fields are tolerant: unknown fields silently
// ignored, missing fields zero.
//
// Unit notes (curl-verified):
//   - VodID: stable unique string id — use as dedup key, NOT StreamID
//   - FileSize: bytes (int64)
//   - CreationDate: Unix epoch MILLISECONDS
//   - Duration: MILLISECONDS (43025 for a ~43 s VoD — NOT seconds)
//   - StreamID: originating live stream id; VodName is the file name
//     (e.g. StreamID="val-vodgen-s17", VodName="val-vodgen-s17.mp4")
type VodDTO struct {
	VodID        string `json:"vodId"`        // unique stable id (dedup key)
	VodName      string `json:"vodName"`      // file name (e.g. "val-vodgen-s17.mp4")
	StreamID     string `json:"streamId"`     // originating stream id (use for attribution, not streamName)
	FilePath     string `json:"filePath"`     // relative path (e.g. "streams/val-vodgen-s17.mp4")
	FileSize     int64  `json:"fileSize"`     // bytes
	CreationDate int64  `json:"creationDate"` // Unix epoch ms
	Duration     int64  `json:"duration"`     // ms (NOT seconds)
	Type         string `json:"type"`         // e.g. "streamVod"
}

// ListVods returns a single page of VoDs for an application.
// Uses the AMS v3 path: /{app}/rest/v2/vods/list/{offset}/{size}.
// size<=0 defaults to 200.
func (c *Client) ListVods(ctx context.Context, app string, offset, size int) ([]VodDTO, error) {
	if size <= 0 {
		size = 200
	}
	path := fmt.Sprintf("/%s/rest/v2/vods/list/%d/%d", app, offset, size)
	var result []VodDTO
	return result, c.getJSON(ctx, path, &result)
}

// ListVodsPaged fetches all pages and returns the complete VoD list for an
// application. Terminates when a page contains fewer than pageSize entries.
// Uses the AMS v3 path: /{app}/rest/v2/vods/list/{offset}/200.
func (c *Client) ListVodsPaged(ctx context.Context, app string) ([]VodDTO, error) {
	const pageSize = 200
	var all []VodDTO
	offset := 0
	for {
		path := fmt.Sprintf("/%s/rest/v2/vods/list/%d/%d", app, offset, pageSize)
		var page []VodDTO
		if err := c.getJSON(ctx, path, &page); err != nil {
			return all, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			break
		}
		offset += pageSize
	}
	return all, nil
}
