package amsclient

// transport_d185_internal_test.go — white-box assertions on the poll client's
// transport (D-185).
//
// These are deliberately internal. The behavioural black-box test for the proxy
// ruling cannot exist: Go's ProxyFromEnvironment never proxies loopback, so an
// httptest-based "did it hit the proxy" test passes whether or not Proxy is
// disabled — it looks like a test and asserts nothing. That vacuity was caught by
// re-running the suite against a reverted tree, which is the only way to tell.
// Reading the field is the honest check.

import (
	"net/http"
	"testing"
)

// TestNew_TransportIsGuarded asserts the constructor's transport shape: a custom
// dialer (Control installed — proven behaviourally in ssrf_d185_test.go) and no
// proxy. With an egress proxy honoured, the transport dials the PROXY, which
// passes the resolved-IP guard, and the proxy then fetches the real target —
// bypassing the guard entirely. Same ruling as prober (D-130) and S3 (D-184).
func TestNew_TransportIsGuarded(t *testing.T) {
	c := New(Config{BaseURL: "http://ams.example.com:5080"})

	tr, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T — the guard wiring is gone", c.httpClient.Transport)
	}
	if tr.Proxy != nil {
		t.Error("transport honours a proxy: an egress proxy would resolve and connect on our behalf, bypassing ssrfguard")
	}
	if tr.DialContext == nil {
		t.Error("transport has no custom DialContext, so ssrfguard.DialControl cannot be installed")
	}
}

// TestNew_BaseURLTrailingSlashNormalized guards the one behaviour the transport
// rewrite sits next to, so a future edit to New cannot quietly drop it.
func TestNew_BaseURLTrailingSlashNormalized(t *testing.T) {
	if got := New(Config{BaseURL: "http://ams.example.com:5080/"}).BaseURL(); got != "http://ams.example.com:5080" {
		t.Errorf("trailing slash not normalized: %q", got)
	}
}
