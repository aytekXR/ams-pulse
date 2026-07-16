package prober

// D-130 [21]: prove ssrfguard.DialControl is actually wired into every prober
// dial path (HTTP client, RTMP raw dialer, WebRTC WS handshake). The guard fires
// after DNS resolution but before connect, so a link-local literal (the cloud
// metadata address) is refused without any socket ever reaching the network —
// these tests are hermetic and assert on the guard's own message so they cannot
// pass by coincidental network failure.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

func TestGuardedClient_RefusesMetadata_S68(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"http://169.254.169.254/latest/meta-data/", nil)
	_, err := newGuardedClient().Do(req)
	if err == nil {
		t.Fatal("guarded client connected to 169.254.169.254; want refusal")
	}
	if !strings.Contains(err.Error(), "restricted address") {
		t.Errorf("expected ssrfguard refusal, got %v", err)
	}
}

// TestGuardedClient_ProxyDisabled_S68 pins the D-130 review fix: the guarded
// transport must NOT honor HTTP(S)_PROXY, or an egress proxy would resolve and
// dial the probe's real destination, bypassing the resolved-IP guard.
func TestGuardedClient_ProxyDisabled_S68(t *testing.T) {
	tr, ok := newGuardedClient().Transport.(*http.Transport)
	if !ok {
		t.Fatal("guarded client transport is not *http.Transport")
	}
	if tr.Proxy != nil {
		t.Error("guarded transport has a Proxy set; want nil (SSRF proxy-bypass guard)")
	}
}

func TestGuardedClient_AllowsLoopback_S68(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := newGuardedClient().Do(req)
	if err != nil {
		t.Fatalf("guarded client refused a loopback test server: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("loopback GET → %d, want 200", resp.StatusCode)
	}
}

func TestProbeRTMP_RefusesMetadata_S68(t *testing.T) {
	r := New(Config{}, nil, nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res := r.probeRTMP(ctx, domain.ProbeConfig{ID: "p", URL: "rtmp://169.254.169.254:1935/live"}, domain.ProbeResult{})
	if res.Success {
		t.Fatal("probeRTMP connected to link-local metadata; want failure")
	}
	if !strings.Contains(res.ErrorMsg, "restricted address") {
		t.Errorf("expected ssrfguard refusal in ErrorMsg, got %q (code=%s)", res.ErrorMsg, res.ErrorCode)
	}
}

func TestProbeWebRTC_RefusesMetadata_S68(t *testing.T) {
	r := New(Config{}, nil, nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res := r.probeWebRTC(ctx, domain.ProbeConfig{ID: "p", URL: "ws://169.254.169.254/LiveApp/websocket?streamId=x"}, domain.ProbeResult{})
	if res.Success {
		t.Fatal("probeWebRTC connected to link-local metadata; want failure")
	}
	if !strings.Contains(res.ErrorMsg, "restricted address") {
		t.Errorf("expected ssrfguard refusal in ErrorMsg, got %q (code=%s)", res.ErrorMsg, res.ErrorCode)
	}
}
