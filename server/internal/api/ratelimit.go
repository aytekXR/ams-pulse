// Package api — per-key token-bucket rate limiter.
//
// Mirrors the hand-rolled tokenBucket in internal/collector/beacon/beacon.go
// (no new dependency; x/time/rate is NOT in go.mod).
package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Rate constants mirror the dedicated beacon server (serve.go:326 +
// beacon defaults: RateLimitPerTokenRPS=100, RateBurst=200).
// A7 metrics scrape defaults: 10 rps / burst 20.
const (
	mainBeaconRateRPS = 100.0 // per-token RPS for main-port beacon ingest (A2)
	mainBeaconBurst   = 200.0 // burst size matching dedicated beacon server
	metricsRateRPS    = 10.0  // per-IP RPS for /metrics scrape (A7)
	metricsBurst      = 20.0  // burst size for /metrics
)

// rlBucket holds the state for one key in the keyed limiter.
type rlBucket struct {
	tokens   float64
	lastFill time.Time
}

// keyedLimiter is a per-key token-bucket rate limiter.
// It is safe for concurrent use; every field access is under l.mu.
type keyedLimiter struct {
	rate  float64 // tokens per second
	burst float64 // max tokens (= initial tokens per bucket)

	mu      sync.Mutex
	buckets map[string]*rlBucket

	// eviction control
	stopEvict chan struct{}
}

// newKeyedLimiter creates a ready-to-use keyed limiter.
// The eviction goroutine is NOT started here; call l.startEviction / l.stopEviction
// in Server.Start / Server.Stop to avoid goroutine leaks in tests that only
// use Handler() and never call Start().
func newKeyedLimiter(rate, burst float64) *keyedLimiter {
	return &keyedLimiter{
		rate:    rate,
		burst:   burst,
		buckets: make(map[string]*rlBucket),
	}
}

// Allow returns true and consumes one token for key if within the rate limit.
// If the key has no bucket yet it is created with tokens=burst (full).
// No external/blocking calls are made while holding l.mu (D-021 lesson).
func (l *keyedLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		// Lazily create bucket pre-filled to burst.
		b = &rlBucket{tokens: l.burst, lastFill: now}
		l.buckets[key] = b
	}

	// Refill tokens proportional to elapsed time.
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.lastFill = now

	if b.tokens >= 1.0 {
		b.tokens--
		return true
	}
	return false
}

// evictOnce removes buckets idle for longer than idleTTL.
// Called periodically by the background goroutine started via startEviction.
func (l *keyedLimiter) evictOnce(idleTTL time.Duration) {
	cutoff := time.Now().Add(-idleTTL)
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, b := range l.buckets {
		if b.lastFill.Before(cutoff) {
			delete(l.buckets, key)
		}
	}
}

// startEviction launches the background eviction goroutine.
// Stops when the returned stop function is called.
// interval is how often to sweep; idleTTL is how long a bucket must be
// idle before eviction.
func (l *keyedLimiter) startEviction(interval, idleTTL time.Duration) (stop func()) {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				l.evictOnce(idleTTL)
			case <-done:
				return
			}
		}
	}()
	return func() { close(done) }
}

// clientIP returns the host part of r.RemoteAddr (via net.SplitHostPort; falls
// back to the raw RemoteAddr string on error). It deliberately reads only
// RemoteAddr and never parses X-Forwarded-For itself.
//
// NOTE on the chi RealIP middleware: the router installs middleware.RealIP
// (server.go), which rewrites r.RemoteAddr from X-Forwarded-For / X-Real-IP
// BEFORE handlers run. So in production behind a reverse proxy this key reflects
// the proxy-reported client IP (good: the real viewer, not the proxy), but that
// value is ultimately client-influenceable and not a spoofing-proof identity.
// That is acceptable for A7's threat model — bounding load from a misconfigured
// scraper, not defeating a determined adversary. A spoofing-resistant key would
// require trusted-proxy XFF parsing or running the limiter ahead of RealIP,
// which is out of scope for this low-severity guard.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
