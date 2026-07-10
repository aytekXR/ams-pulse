// Package prober implements the F10 synthetic probe runner.
//
// The runner periodically issues playback checks against configured stream URLs,
// measures TTFB and estimated bitrate, and writes results to both ClickHouse
// (time-series) and the ProbeConfigSource (denormalized last_* fields).
//
// Protocol coverage:
//   - HLS (minimum, fully working): GET manifest + first segment; measures TTFB,
//     parses #EXTM3U, fetches first media URI, computes bitrate_kbps.
//   - WebRTC (phase-2a ICE path): dials wss?://{host}/{app}/websocket?streamId=<id>,
//     sends play command, measures time to first takeConfiguration/offer (signaling),
//     then continues into full ICE negotiation via pion (pure-Go, no external STUN).
//     Outcome: success=true + signaling_state="offer_received" once the offer is parsed;
//     ice_state="connected"|"failed"|"timeout" records the terminal ICE state.
//     ICE errors additionally set error_code="ice_failed"|"ice_timeout".
//     streamId MUST be present as a URL query param; missing → ws_error.
//   - RTMP (phase-1 handshake): stdlib-only TCP C0/C1 → S0/S1/S2 → C2 with strict
//     S2-echo validation; measures connect_time_ms (dial → S2 fully read);
//     signaling_state=handshake_complete; errors rtmp_timeout/rtmp_refused/rtmp_error.
//   - DASH: GET MPD + first segment (mirrors the HLS measurement shape); parses
//     SegmentTemplate/SegmentList via encoding/xml, chains BaseURL per spec,
//     computes timescale-adjusted bitrate_kbps.
//   - Unknown protocols (e.g. srt): minimal-but-honest reachability check —
//     records a "not_probed" error_code. No faked success.
//
// Remaining roadmap: WebRTC RTCP receiver stats (pion phase-2b — S14 yield to S15).
package prober

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// segBodyCapBytes is the maximum number of bytes read from a media segment body
// (HLS or DASH).  Any segment body larger than this cap is reported as
// error_code="segment_too_large" with Success=true (the manifest was OK) and
// BitrateKbps=0 — truncation never silently corrupts the bitrate computation.
// 32 MiB provides 2× headroom over the worst-case 6 s × 20 Mbps = 15 MB
// segment and is 640× the largest test fixture (D-074, WO-F).
const segBodyCapBytes = 32 << 20

// ResultStore is the ClickHouse writer the runner depends on.
// Implemented by store/clickhouse.Store.InsertProbeResult.
type ResultStore interface {
	InsertProbeResult(ctx context.Context, r domain.ProbeResult) error
}

// Config holds probe runner configuration.
type Config struct {
	// Workers is the size of the concurrency pool for parallel probe execution.
	// Default: 4.
	Workers int

	// MaxJitterFraction is the maximum jitter applied to each probe interval as
	// a fraction of interval_s, to spread load.  Default: 0.10 (10%).
	MaxJitterFraction float64

	// HTTPUserAgent is the User-Agent sent in probe requests.
	HTTPUserAgent string
}

// Clock abstracts time for deterministic testing.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// realClock is the default clock using wall time.
type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// probeExecRequest is sent from per-probe goroutines to the worker pool.
type probeExecRequest struct {
	probe domain.ProbeConfig
}

// Runner is the probe scheduler and executor.
type Runner struct {
	cfg    Config
	source domain.ProbeConfigSource
	store  ResultStore
	logger *slog.Logger
	clock  Clock
	client *http.Client
}

// New creates a Runner. Pass a custom clock (e.g. FakeClock) for testing.
// If clock is nil, wall time is used.
func New(cfg Config, source domain.ProbeConfigSource, store ResultStore, logger *slog.Logger, clock Clock) *Runner {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.MaxJitterFraction < 0 {
		cfg.MaxJitterFraction = 0.10
	}
	// MaxJitterFraction == 0 is valid: means no jitter (useful for testing).
	if cfg.HTTPUserAgent == "" {
		cfg.HTTPUserAgent = "Pulse-Prober/1.0"
	}
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = realClock{}
	}
	return &Runner{
		cfg:    cfg,
		source: source,
		store:  store,
		logger: logger,
		clock:  clock,
		client: &http.Client{
			// Timeout is overridden per-probe using the context deadline.
			Timeout: 0,
		},
	}
}

// Run starts the probe scheduler. It blocks until ctx is cancelled.
// Each enabled probe is scheduled independently; all probes run in the shared
// worker pool (bounded concurrency).
func (r *Runner) Run(ctx context.Context) error {
	// Semaphore for bounded concurrency.
	sem := make(chan struct{}, r.cfg.Workers)

	var wg sync.WaitGroup

	// Initial load of probes.
	probes, err := r.source.ListEnabled(ctx)
	if err != nil {
		r.logger.Warn("prober: initial ListEnabled failed", "error", err)
		// Continue — will retry on next refresh tick.
		probes = nil
	}

	// Channel for per-probe scheduler goroutines to request execution.
	execCh := make(chan probeExecRequest, maxInt(len(probes), 1)+4)

	// refreshTicker reloads the probe list every minute so new probes are
	// picked up without restart.
	refreshTicker := time.NewTicker(60 * time.Second)
	defer refreshTicker.Stop()

	// Track per-probe goroutine lifecycle.
	type probeEntry struct {
		cancel context.CancelFunc
	}
	running := make(map[string]probeEntry)

	spawnProbe := func(p domain.ProbeConfig) {
		if e, ok := running[p.ID]; ok {
			// Cancel the old scheduler goroutine and respawn with updated config.
			e.cancel()
		}
		pCtx, pCancel := context.WithCancel(ctx)
		running[p.ID] = probeEntry{cancel: pCancel}

		wg.Add(1)
		go func(probe domain.ProbeConfig, probeCtx context.Context) {
			defer wg.Done()
			r.runProbeScheduler(probeCtx, probe, execCh)
		}(p, pCtx)
	}

	// Spawn initial probes.
	for _, p := range probes {
		spawnProbe(p)
	}

	// Worker pool: drain execCh and execute probes concurrently.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case req, ok := <-execCh:
				if !ok {
					return
				}
				// Acquire semaphore slot.
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				wg.Add(1)
				go func(probe domain.ProbeConfig) {
					defer wg.Done()
					defer func() { <-sem }()
					r.executeProbe(ctx, probe)
				}(req.probe)
			}
		}
	}()

	// Refresh loop — also the main goroutine's select.
	for {
		select {
		case <-ctx.Done():
			// Cancel all per-probe goroutines.
			for _, e := range running {
				e.cancel()
			}
			wg.Wait()
			return ctx.Err()

		case <-refreshTicker.C:
			newProbes, err := r.source.ListEnabled(ctx)
			if err != nil {
				r.logger.Warn("prober: ListEnabled refresh failed", "error", err)
				continue
			}
			// Build a set of new IDs.
			newIDs := make(map[string]struct{}, len(newProbes))
			for _, p := range newProbes {
				newIDs[p.ID] = struct{}{}
				spawnProbe(p)
			}
			// Cancel removed probes.
			for id, e := range running {
				if _, ok := newIDs[id]; !ok {
					e.cancel()
					delete(running, id)
				}
			}
		}
	}
}

// runProbeScheduler manages the timing loop for a single probe. It sends probe
// execution requests to execCh at every interval, with random jitter to spread
// load.
func (r *Runner) runProbeScheduler(ctx context.Context, p domain.ProbeConfig, execCh chan<- probeExecRequest) {
	intervalS := p.IntervalS
	if intervalS <= 0 {
		intervalS = 60
	}
	interval := time.Duration(intervalS) * time.Second

	// Initial jitter: stagger startup to avoid thundering herd.
	jitter := r.jitter(interval)

	// Wait for initial jitter before the first fire.
	select {
	case <-ctx.Done():
		return
	case <-r.clock.After(jitter):
	}

	// First fire.
	select {
	case execCh <- probeExecRequest{probe: p}:
	case <-ctx.Done():
		return
	}

	// Subsequent fires.
	for {
		wait := interval + r.jitter(interval)
		select {
		case <-ctx.Done():
			return
		case <-r.clock.After(wait):
		}
		select {
		case execCh <- probeExecRequest{probe: p}:
		case <-ctx.Done():
			return
		}
	}
}

// jitter returns a random duration in [0, MaxJitterFraction * interval).
func (r *Runner) jitter(interval time.Duration) time.Duration {
	maxJitterMs := int64(float64(interval.Milliseconds()) * r.cfg.MaxJitterFraction)
	if maxJitterMs <= 0 {
		return 0
	}
	// #nosec G404 — non-security random for scheduling jitter.
	return time.Duration(rand.Int63n(maxJitterMs)) * time.Millisecond //nolint:gosec
}

// executeProbe runs a single probe check and writes the result.
func (r *Runner) executeProbe(ctx context.Context, p domain.ProbeConfig) {
	timeoutS := p.TimeoutS
	if timeoutS <= 0 {
		timeoutS = 10
	}

	probeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutS)*time.Second)
	defer cancel()

	result := domain.ProbeResult{
		ID:      uuid.New().String(),
		ProbeID: p.ID,
		TS:      r.clock.Now().UTC(),
	}

	switch strings.ToLower(p.Protocol) {
	case "hls", "":
		// HLS: full manifest + first-segment check.
		result = r.probeHLS(probeCtx, p, result)
	case "webrtc":
		// WebRTC phase-2a (D-074): signaling via nhooyr.io/websocket, then
		// pion ICE negotiation (continueWebRTCICE) annotating ice_state.
		// URL convention: ws(s)://host/{app}/websocket?streamId=<id>
		result = r.probeWebRTC(probeCtx, p, result)
	case "rtmp":
		// RTMP: TCP handshake (C0+C1 / S0+S1+S2 / C2) probe.
		result = r.probeRTMP(probeCtx, p, result)
	case "dash":
		// DASH: MPD manifest fetch + first-segment TTFB/bitrate probe.
		result = r.probeDASH(probeCtx, p, result)
	default:
		// Unknown protocol (e.g. "srt"): minimal honest reachability check.
		// Returns error_code="not_probed" as a forward-compat stub until a
		// dedicated probe is implemented. Does NOT cover rtmp or dash.
		result = r.probeReachability(probeCtx, p, result)
	}

	// Write to ClickHouse.
	if err := r.store.InsertProbeResult(ctx, result); err != nil {
		r.logger.Warn("prober: InsertProbeResult failed",
			"probe_id", p.ID,
			"error", err,
		)
	}

	// Update last_* denorm fields in meta store.
	if err := r.source.RecordResult(ctx, result); err != nil {
		r.logger.Warn("prober: RecordResult failed",
			"probe_id", p.ID,
			"error", err,
		)
	}
}

// probeHLS performs a full HLS playback check:
//  1. GET manifest URL, measure TTFB.
//  2. Parse #EXTM3U — extract first media segment URI.
//  3. GET first segment, compute bitrate_kbps = bytes / segment_duration.
//
// On any failure, returns an honest error_code and empty success.
func (r *Runner) probeHLS(ctx context.Context, p domain.ProbeConfig, result domain.ProbeResult) domain.ProbeResult {
	// Step 1: fetch manifest.
	manifestStart := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		result.Success = false
		result.ErrorCode = "parse"
		result.ErrorMsg = fmt.Sprintf("build request: %v", err)
		return result
	}
	req.Header.Set("User-Agent", r.cfg.HTTPUserAgent)

	resp, err := r.client.Do(req)
	ttfbMs := uint32(time.Since(manifestStart).Milliseconds())
	// Ensure at least 1 ms is reported for a successful HTTP round trip.
	// time.Since().Milliseconds() rounds down; on localhost this can produce 0
	// even for a real TCP connection (sub-millisecond). Any actual network round
	// trip takes >0 µs, so 1 ms is the correct floor for uint32 ms resolution.
	if ttfbMs == 0 {
		ttfbMs = 1
	}
	result.TTFBMs = ttfbMs

	if err != nil {
		result.Success = false
		result.ErrorCode = classifyHTTPError(err)
		result.ErrorMsg = err.Error()
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 == 4 {
		result.Success = false
		result.ErrorCode = "http_4xx"
		result.ErrorMsg = fmt.Sprintf("manifest HTTP %d", resp.StatusCode)
		return result
	}
	if resp.StatusCode/100 == 5 {
		result.Success = false
		result.ErrorCode = "http_5xx"
		result.ErrorMsg = fmt.Sprintf("manifest HTTP %d", resp.StatusCode)
		return result
	}
	if resp.StatusCode/100 != 2 {
		result.Success = false
		result.ErrorCode = fmt.Sprintf("http_%d", resp.StatusCode)
		result.ErrorMsg = fmt.Sprintf("manifest HTTP %d", resp.StatusCode)
		return result
	}

	// Step 2: parse manifest.
	segmentURI, segmentDurationS, isMaster, variantURI, err := parseHLSManifest(resp.Body, p.URL)
	if err != nil {
		result.Success = false
		result.ErrorCode = "parse"
		result.ErrorMsg = fmt.Sprintf("parse manifest: %v", err)
		return result
	}

	if isMaster {
		// This is a master playlist. Follow the first variant to obtain bitrate data.
		// Cap at ONE level of indirection — master → variant → segment.
		if variantURI == "" {
			// Master with no variants (empty/malformed) — reachability pass, bitrate = 0.
			result.Success = true
			result.BitrateKbps = 0
			return result
		}

		// Fetch the variant playlist.
		varReq, err := http.NewRequestWithContext(ctx, http.MethodGet, variantURI, nil)
		if err != nil {
			result.Success = false
			result.ErrorCode = "parse"
			result.ErrorMsg = fmt.Sprintf("variant request: %v", err)
			return result
		}
		varReq.Header.Set("User-Agent", r.cfg.HTTPUserAgent)

		varResp, err := r.client.Do(varReq)
		if err != nil {
			result.Success = false
			result.ErrorCode = classifyHTTPError(err)
			result.ErrorMsg = fmt.Sprintf("variant fetch: %v", err)
			return result
		}
		defer varResp.Body.Close()

		if varResp.StatusCode/100 != 2 {
			result.Success = false
			result.ErrorCode = fmt.Sprintf("http_%d", varResp.StatusCode)
			result.ErrorMsg = fmt.Sprintf("variant HTTP %d", varResp.StatusCode)
			return result
		}

		// Parse the variant playlist — it must be a media playlist (not another master).
		var isMasterAgain bool
		segmentURI, segmentDurationS, isMasterAgain, _, err = parseHLSManifest(varResp.Body, variantURI)
		if err != nil {
			result.Success = false
			result.ErrorCode = "parse"
			result.ErrorMsg = fmt.Sprintf("parse variant manifest: %v", err)
			return result
		}
		if isMasterAgain {
			// Malformed: master → master — refuse to recurse.
			result.Success = false
			result.ErrorCode = "parse"
			result.ErrorMsg = "variant playlist is also a master (malformed HLS)"
			return result
		}
		if segmentURI == "" {
			// Variant has no segments yet (live edge case) — reachability pass.
			result.Success = true
			result.BitrateKbps = 0
			return result
		}
		// segmentURI and segmentDurationS are now updated from the variant playlist;
		// fall through to Step 3 below.
	}

	// Step 3: fetch first media segment and measure bitrate.
	segReq, err := http.NewRequestWithContext(ctx, http.MethodGet, segmentURI, nil)
	if err != nil {
		// Manifest was valid; segment build error = parse.
		result.Success = true // manifest OK
		result.ErrorCode = "parse"
		result.ErrorMsg = fmt.Sprintf("segment request: %v", err)
		return result
	}
	segReq.Header.Set("User-Agent", r.cfg.HTTPUserAgent)

	segStart := time.Now()
	segResp, err := r.client.Do(segReq)
	segTTFBMs := uint32(time.Since(segStart).Milliseconds())
	// Apply same 1ms floor as manifest TTFB (D-013): localhost sub-ms rounds to 0.
	if segTTFBMs == 0 {
		segTTFBMs = 1
	}
	if err != nil {
		result.Success = true // manifest OK; segment is bonus measurement
		result.ErrorCode = classifyHTTPError(err)
		result.ErrorMsg = fmt.Sprintf("segment: %v", err)
		return result
	}
	defer segResp.Body.Close()

	if segResp.StatusCode/100 != 2 {
		result.Success = true // manifest OK
		result.ErrorCode = fmt.Sprintf("http_%d", segResp.StatusCode)
		result.ErrorMsg = fmt.Sprintf("segment HTTP %d", segResp.StatusCode)
		return result
	}

	// Record segment TTFB on successful 2xx response.
	result.SegmentTTFBMs = segTTFBMs

	segBytes, err := io.ReadAll(io.LimitReader(segResp.Body, segBodyCapBytes+1))
	if err != nil {
		result.Success = true
		result.ErrorCode = "read"
		result.ErrorMsg = fmt.Sprintf("read segment: %v", err)
		return result
	}

	// Enforce the segment body cap: a segment larger than segBodyCapBytes is
	// flagged as segment_too_large.  BitrateKbps stays 0 — a bitrate computed
	// from a truncated body would be meaningless.  Success=true because the
	// manifest was valid ("segment is bonus measurement" invariant, D-074 WO-F).
	if len(segBytes) > segBodyCapBytes {
		result.Success = true
		result.ErrorCode = "segment_too_large"
		result.ErrorMsg = fmt.Sprintf("segment body exceeds %d-byte cap (%d bytes read via LimitReader)",
			segBodyCapBytes, len(segBytes))
		return result
	}

	// Compute bitrate: segment_bytes * 8 bits / segment_duration_s / 1000.
	if segmentDurationS > 0 {
		result.BitrateKbps = float32(float64(len(segBytes)) * 8.0 / segmentDurationS / 1000.0)
	}

	result.Success = true
	result.ErrorCode = ""
	result.ErrorMsg = ""
	return result
}

// parseHLSManifest reads an HLS playlist body and returns:
//   - segmentURI: the first .ts/.m4s/.fmp4 media segment URL (absolute).
//     Empty string with isMaster=true means a master playlist; variantURI is populated.
//   - segmentDurationS: the #EXTINF duration for the first segment.
//   - isMaster: true when the playlist is a master (EXT-X-STREAM-INF present); in that
//     case segmentURI is empty and variantURI is the first variant playlist URL (absolute).
//   - variantURI: the first variant playlist URL when isMaster=true; empty otherwise.
//
// baseURL is used to resolve relative URIs.
func parseHLSManifest(body io.Reader, baseURL string) (segmentURI string, segmentDurationS float64, isMaster bool, variantURI string, err error) {
	scanner := bufio.NewScanner(body)

	isM3U := false
	var pendingDuration float64
	pendingVariant := false // true when the previous line was #EXT-X-STREAM-INF

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !isM3U {
			if strings.HasPrefix(line, "#EXTM3U") {
				isM3U = true
			} else {
				return "", 0, false, "", fmt.Errorf("not an M3U8: first non-empty line = %q", line)
			}
			continue
		}

		if strings.HasPrefix(line, "#EXTINF:") {
			// #EXTINF:<duration>, title
			info := strings.TrimPrefix(line, "#EXTINF:")
			info = strings.SplitN(info, ",", 2)[0]
			var dur float64
			fmt.Sscanf(info, "%f", &dur) //nolint:errcheck
			pendingDuration = dur
			pendingVariant = false
			continue
		}

		if strings.HasPrefix(line, "#EXT-X-STREAM-INF") {
			// Master playlist stream entry — next non-comment line is the variant URI.
			pendingVariant = true
			continue
		}

		if strings.HasPrefix(line, "#") {
			// Other tags — skip, but don't clear pendingVariant.
			continue
		}

		// Non-comment, non-empty line: either a segment URI or a variant URI.
		if pendingDuration > 0 {
			// Preceded by #EXTINF → this is a media segment.
			uri := resolveURI(line, baseURL)
			return uri, pendingDuration, false, "", nil
		}
		if pendingVariant {
			// Preceded by #EXT-X-STREAM-INF → this is a variant playlist URI in a master.
			uri := resolveURI(line, baseURL)
			return "", 0, true, uri, nil
		}
		// No preceding #EXTINF or #EXT-X-STREAM-INF → treat as master (non-error).
		return "", 0, true, "", nil
	}

	if err := scanner.Err(); err != nil {
		return "", 0, false, "", err
	}

	// Empty or live playlist with no segments yet — signal master (non-error).
	return "", 0, true, "", nil
}

// resolveURI resolves a potentially relative URI against the base playlist URL.
func resolveURI(uri, base string) string {
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		return uri
	}
	// Simple path resolution: replace everything after the last '/' in base.
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		return base[:idx+1] + uri
	}
	return uri
}

// probeReachability performs a minimal HTTP GET and marks the result as
// "not_probed" with a documented error_code. This is the honest stub for
// webrtc / rtmp / dash protocols where full playback simulation requires a
// native client library (Phase 3 roadmap).
//
// We do NOT fake a success — a 200 HTTP response from a non-HLS endpoint says
// nothing about playback quality for these protocols.
func (r *Runner) probeReachability(ctx context.Context, p domain.ProbeConfig, result domain.ProbeResult) domain.ProbeResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		result.Success = false
		result.ErrorCode = "not_probed"
		result.ErrorMsg = fmt.Sprintf("protocol=%s: full probing not yet implemented (Phase 3); build request: %v", p.Protocol, err)
		return result
	}
	req.Header.Set("User-Agent", r.cfg.HTTPUserAgent)

	start := time.Now()
	resp, err := r.client.Do(req)
	result.TTFBMs = uint32(time.Since(start).Milliseconds())

	if err != nil {
		result.Success = false
		result.ErrorCode = "not_probed"
		result.ErrorMsg = fmt.Sprintf("protocol=%s: full probing not yet implemented (Phase 3); reachability: %v", p.Protocol, err)
		return result
	}
	defer resp.Body.Close()

	// Intentionally NOT marking as success — see function doc.
	result.Success = false
	result.ErrorCode = "not_probed"
	result.ErrorMsg = fmt.Sprintf("protocol=%s: full probing not yet implemented (Phase 3); HTTP %d received", p.Protocol, resp.StatusCode)
	return result
}

// wsSignalingMsg is the AMS WS signaling message shape parsed by the WebRTC probe.
//
// Phase-1 fields (signaling): Command, Type.
// Phase-2a fields (ICE via trickle): StreamID, SDP (takeConfiguration/offer+answer),
// Label (SDPMLineIndex, int), ID (SDPMid, string), Candidate (ICE candidate string).
//
// AMS wire shapes (from real-AMS-captures/webrtc-signaling-play-offer.json):
//
//	takeConfiguration: {"command":"takeConfiguration","streamId":"...","type":"offer","sdp":"..."}
//	takeCandidate:     {"command":"takeCandidate","streamId":"...","label":1,"id":"1","candidate":"candidate:..."}
type wsSignalingMsg struct {
	Command    string `json:"command"`
	Type       string `json:"type,omitempty"`
	StreamID   string `json:"streamId,omitempty"`
	SDP        string `json:"sdp,omitempty"`
	Label      int    `json:"label,omitempty"`
	ID         string `json:"id,omitempty"`
	Candidate  string `json:"candidate,omitempty"`
	Definition string `json:"definition,omitempty"` // notification/error detail (e.g. subtrackAdded, highResourceUsage)
}

// probeWebRTC performs a WebRTC signaling + ICE probe (phase 2a, D-074).
//
// URL convention (D-072 ruling): ws(s)://host/{app}/websocket?streamId=<id>
// The streamId query parameter is REQUIRED. Missing → Success=false, error_code=ws_error.
//
// Steps:
//  1. Parse streamId from URL query string.
//  2. Dial WebSocket endpoint; classify refused/timeout/other.
//  3. Send AMS play command JSON.
//  4. Read first server message; success iff command==takeConfiguration && type==offer.
//  5. Set ConnectTimeMs = elapsed ms from dial start to first parseable message.
//  6. On signaling success, continue into pion ICE negotiation
//     (continueWebRTCICE): answer + trickle candidates → ice_state
//     connected|failed|timeout. ICE outcome never flips Success
//     (bonus-measurement semantics, D-074).
func (r *Runner) probeWebRTC(ctx context.Context, p domain.ProbeConfig, result domain.ProbeResult) domain.ProbeResult {
	// Validate streamId in URL query params.
	rawURL := p.URL
	streamID := ""
	if idx := strings.Index(rawURL, "?"); idx >= 0 {
		query := rawURL[idx+1:]
		for _, kv := range strings.Split(query, "&") {
			if strings.HasPrefix(kv, "streamId=") {
				raw := strings.TrimPrefix(kv, "streamId=")
				// Percent-decode so encoded IDs (spaces, slashes, non-ASCII)
				// reach AMS as their real value; keep the raw form if decoding
				// fails (malformed escape) rather than dropping the probe.
				if dec, decErr := url.QueryUnescape(raw); decErr == nil {
					streamID = dec
				} else {
					streamID = raw
				}
			}
		}
	}
	if streamID == "" {
		result.Success = false
		result.ErrorCode = "ws_error"
		result.ErrorMsg = "webrtc probe URL must include ?streamId=<id>; convention: ws(s)://host/{app}/websocket?streamId=<id>"
		result.SignalingState = "ws_error"
		return result
	}

	// Remove query string from WS dial URL — nhooyr.io/websocket accepts the full URL
	// including query params, so pass rawURL directly.
	dialStart := time.Now()

	conn, _, err := websocket.Dial(ctx, rawURL, &websocket.DialOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		errStr := err.Error()
		var code string
		var sigState string
		switch {
		case strings.Contains(errStr, "connection refused"):
			code = "ws_refused"
			sigState = "ws_refused"
		case strings.Contains(errStr, "context deadline exceeded") ||
			strings.Contains(errStr, "deadline exceeded") ||
			strings.Contains(errStr, "timeout"):
			code = "ws_timeout"
			sigState = "ws_timeout"
		default:
			code = "ws_error"
			sigState = "ws_error"
		}
		result.Success = false
		result.ErrorCode = code
		result.ErrorMsg = fmt.Sprintf("ws dial: %v", err)
		result.SignalingState = sigState
		return result
	}
	defer conn.Close(websocket.StatusNormalClosure, "probe done")

	// Send AMS play command.
	playCmd := map[string]interface{}{
		"command":   "play",
		"streamId":  streamID,
		"token":     "",
		"trackList": []interface{}{},
	}
	if err := wsjson.Write(ctx, conn, playCmd); err != nil {
		result.Success = false
		result.ErrorCode = "ws_error"
		result.ErrorMsg = fmt.Sprintf("ws write play: %v", err)
		result.SignalingState = "ws_error"
		return result
	}

	// Read server messages until the signaling response arrives. Real AMS
	// 3.0.3 sends notification messages (e.g. subtrackAdded) BEFORE
	// takeConfiguration — live-evidenced D-074; the probe must skip them or
	// it fails against every real AMS with a live stream. The ctx deadline
	// (TimeoutS) bounds the loop.
	for {
		var rawMsg json.RawMessage
		if err := wsjson.Read(ctx, conn, &rawMsg); err != nil {
			errStr := err.Error()
			var code string
			var sigState string
			// Check both the error string and the context state.
			// nhooyr.io/websocket may return a CloseError on ctx cancellation instead of
			// propagating the context error directly; always defer to ctx.Err() first.
			switch {
			case ctx.Err() != nil && (ctx.Err() == context.DeadlineExceeded ||
				strings.Contains(ctx.Err().Error(), "deadline exceeded")):
				code = "ws_timeout"
				sigState = "ws_timeout"
			case strings.Contains(errStr, "context deadline exceeded") ||
				strings.Contains(errStr, "deadline exceeded") ||
				strings.Contains(errStr, "StatusGoingAway") ||
				strings.Contains(errStr, "timeout"):
				code = "ws_timeout"
				sigState = "ws_timeout"
			default:
				code = "ws_error"
				sigState = "ws_error"
			}
			result.Success = false
			result.ErrorCode = code
			result.ErrorMsg = fmt.Sprintf("ws read: %v", err)
			result.SignalingState = sigState
			return result
		}

		// Parse message: success iff command==takeConfiguration && type==offer.
		var msg wsSignalingMsg
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			result.Success = false
			result.ErrorCode = "ws_error"
			result.ErrorMsg = fmt.Sprintf("ws parse signaling message: %v", err)
			result.SignalingState = "ws_error"
			return result
		}

		// Informational messages (subtrackAdded, play_started, …) are not the
		// signaling response — keep reading.
		if msg.Command == "notification" {
			continue
		}

		if msg.Command == "takeConfiguration" && msg.Type == "offer" {
			// ConnectTimeMs = dial start → offer received (notifications
			// skipped above do not count as the signaling response).
			elapsed := uint32(time.Since(dialStart).Milliseconds())
			if elapsed == 0 {
				elapsed = 1 // floor at 1ms (same pattern as HLS TTFB)
			}
			ct := elapsed
			result.Success = true
			result.ErrorCode = ""
			result.ErrorMsg = ""
			result.ConnectTimeMs = &ct
			result.SignalingState = "offer_received"
			// Phase-2a: continue into ICE negotiation on the same WS connection.
			// continueWebRTCICE is defined in probe_webrtc_ice.go; it fills
			// result.IceState (and result.ErrorCode on ICE failure/timeout).
			// Success stays true regardless of ICE outcome — signaling succeeded.
			result = continueWebRTCICE(ctx, conn, streamID, msg.SDP, result)
			return result
		}

		// A non-notification message that is not the offer (e.g. an AMS
		// error command) is terminal for the signaling check.
		result.Success = false
		result.ErrorCode = "ws_error"
		if msg.Command == "error" && msg.Definition != "" {
			result.ErrorMsg = fmt.Sprintf("ams error: %s", msg.Definition)
		} else {
			result.ErrorMsg = fmt.Sprintf("unexpected signaling message: command=%q type=%q", msg.Command, msg.Type)
		}
		result.SignalingState = "ws_error"
		return result
	}
}

// classifyHTTPError maps a Go net/http error to a probe error_code string.
func classifyHTTPError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "dns"):
		return "dns"
	case strings.Contains(msg, "connection refused"):
		return "conn_refused"
	default:
		return "network"
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
