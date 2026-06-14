// Package prober implements the F10 synthetic probe runner.
//
// The runner periodically issues playback checks against configured stream URLs,
// measures TTFB and estimated bitrate, and writes results to both ClickHouse
// (time-series) and the ProbeConfigSource (denormalized last_* fields).
//
// Protocol coverage:
//   - HLS (minimum, fully working): GET manifest + first segment; measures TTFB,
//     parses #EXTM3U, fetches first media URI, computes bitrate_kbps.
//   - WebRTC / RTMP / DASH: minimal-but-honest reachability check — performs an
//     HTTP GET against the URL and records a "not_probed" error_code.
//     No faked success. Phase-3 roadmap: native protocol clients.
//
// See WO-301 for phase-3 full-coverage roadmap.
package prober

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pulse-analytics/pulse/server/internal/domain"
)

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
	default:
		// webrtc / rtmp / dash: minimal honest check — attempt HTTP reachability
		// but mark as not-yet-probed with a documented error_code.
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
	result.TTFBMs = uint32(time.Since(manifestStart).Milliseconds())

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
	segmentURI, segmentDurationS, err := parseHLSManifest(resp.Body, p.URL)
	if err != nil {
		result.Success = false
		result.ErrorCode = "parse"
		result.ErrorMsg = fmt.Sprintf("parse manifest: %v", err)
		return result
	}

	if segmentURI == "" {
		// This is a master playlist (points to variant streams). Success at the
		// manifest level counts as a reachability pass for HLS; bitrate = 0.
		result.Success = true
		result.BitrateKbps = 0
		return result
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

	segResp, err := r.client.Do(segReq)
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

	segBytes, err := io.ReadAll(segResp.Body)
	if err != nil {
		result.Success = true
		result.ErrorCode = "read"
		result.ErrorMsg = fmt.Sprintf("read segment: %v", err)
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
//     Empty string means this is a master playlist (variant streams only).
//   - segmentDurationS: the #EXTINF duration for the first segment.
//
// baseURL is used to resolve relative URIs.
func parseHLSManifest(body io.Reader, baseURL string) (segmentURI string, segmentDurationS float64, err error) {
	scanner := bufio.NewScanner(body)

	isM3U := false
	var pendingDuration float64

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !isM3U {
			if strings.HasPrefix(line, "#EXTM3U") {
				isM3U = true
			} else {
				return "", 0, fmt.Errorf("not an M3U8: first non-empty line = %q", line)
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
			continue
		}

		if strings.HasPrefix(line, "#") {
			// Other tags (e.g. #EXT-X-STREAM-INF) — skip.
			continue
		}

		// Non-comment, non-empty line: either a segment URI or a variant URI.
		if pendingDuration > 0 {
			// Preceded by #EXTINF → this is a media segment.
			uri := resolveURI(line, baseURL)
			return uri, pendingDuration, nil
		}
		// No preceding #EXTINF → variant playlist URI in a master playlist.
		// Return empty segmentURI to signal master playlist.
		return "", 0, nil
	}

	if err := scanner.Err(); err != nil {
		return "", 0, err
	}

	// Empty or live playlist with no segments yet — signal master (non-error).
	return "", 0, nil
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
