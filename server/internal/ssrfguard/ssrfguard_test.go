package ssrfguard

import (
	"net"
	"strings"
	"testing"
)

// TestIsDenied_S68 pins the address policy (D-130 [21]): link-local (incl. the
// IMDSv4 metadata address), unspecified, link-local multicast, and IMDSv6 are
// denied; loopback and RFC-1918 / ULA private ranges and public addresses are
// allowed (self-hosted AMS is routinely on an internal network — B4/A6 ruling).
func TestIsDenied_S68(t *testing.T) {
	cases := []struct {
		ip     string
		denied bool
		why    string
	}{
		// Denied.
		{"169.254.169.254", true, "IMDSv4 cloud metadata (link-local)"},
		{"169.254.0.1", true, "link-local unicast"},
		{"0.0.0.0", true, "unspecified v4"},
		{"::", true, "unspecified v6"},
		{"fe80::1", true, "link-local unicast v6"},
		{"ff02::1", true, "link-local multicast v6"},
		{"224.0.0.1", true, "link-local multicast v4"},
		{"fd00:ec2::254", true, "IMDSv6 cloud metadata"},
		{"::ffff:169.254.169.254", true, "IPv4-mapped link-local"},
		{"64:ff9b::169.254.169.254", true, "NAT64-embedded link-local (RFC 6052)"},
		{"64:ff9b::0.0.0.0", true, "NAT64-embedded unspecified"},
		{"::169.254.169.254", true, "IPv4-compatible link-local (deprecated)"},
		// Allowed.
		{"127.0.0.1", false, "loopback allowed (internal AMS / test servers)"},
		{"::1", false, "loopback v6 allowed"},
		{"10.0.0.5", false, "RFC-1918 allowed (internal AMS)"},
		{"172.16.0.1", false, "RFC-1918 allowed"},
		{"192.168.1.1", false, "RFC-1918 allowed"},
		{"fd12:3456:789a::1", false, "ULA (non-IMDS) allowed"},
		{"8.8.8.8", false, "public allowed"},
		{"1.1.1.1", false, "public allowed"},
		{"64:ff9b::8.8.8.8", false, "NAT64-embedded public allowed (judged by embedded v4)"},
		{"64:ff9b::10.0.0.5", false, "NAT64-embedded RFC-1918 allowed (internal AMS)"},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("test bug: %q did not parse as an IP", c.ip)
		}
		if got := IsDenied(ip); got != c.denied {
			t.Errorf("IsDenied(%s) = %v, want %v (%s)", c.ip, got, c.denied, c.why)
		}
	}
	// A nil IP must fail closed (denied).
	if !IsDenied(nil) {
		t.Errorf("IsDenied(nil) = false, want true (must fail closed)")
	}
}

// TestDialControl_S68 proves the net.Dialer.Control hook refuses a restricted
// resolved address and permits an allowed one — the enforcement seam shared by
// every prober dial path.
func TestDialControl_S68(t *testing.T) {
	denied := []string{
		"169.254.169.254:80",
		"0.0.0.0:80",
		"[fe80::1]:80",
		"[fd00:ec2::254]:80",
		"[64:ff9b::169.254.169.254]:80", // NAT64-embedded link-local
		"not-an-ip:80",                  // unparseable host → fail closed
		"noport",                        // no host:port → fail closed
	}
	for _, addr := range denied {
		if err := DialControl("tcp", addr, nil); err == nil {
			t.Errorf("DialControl(%q) = nil, want error", addr)
		}
	}
	allowed := []string{
		"10.0.0.5:5080",
		"127.0.0.1:8090",
		"[::1]:80",
		"8.8.8.8:443",
	}
	for _, addr := range allowed {
		if err := DialControl("tcp", addr, nil); err != nil {
			t.Errorf("DialControl(%q) = %v, want nil", addr, err)
		}
	}
}

// TestValidateProbeURL_S68 pins the API-boundary policy: scheme allowlist across
// every real probe protocol, and IP-literal rejection for restricted hosts.
func TestValidateProbeURL_S68(t *testing.T) {
	valid := []string{
		"http://example.com/live.m3u8",
		"https://example.com",
		"ws://10.0.0.5:5080/LiveApp/websocket?streamId=x", // ws + RFC-1918 allowed
		"wss://cdn.example.com/signal",
		"rtmp://10.0.0.5:1935/live",
		"rtmps://example.com/live",
		"http://10.0.0.5:5080/live.m3u8", // RFC-1918 allowed (internal AMS)
		"http://127.0.0.1:8090/x",        // loopback allowed
		"http://[::1]:80/x",              // loopback v6 allowed
	}
	for _, u := range valid {
		if err := ValidateProbeURL(u); err != nil {
			t.Errorf("ValidateProbeURL(%q) = %v, want nil", u, err)
		}
	}
	invalid := []string{
		"file:///etc/passwd",
		"gopher://169.254.169.254/",
		"ftp://example.com/x",
		"dict://localhost:11211/",
		"http://169.254.169.254/latest/meta-data/",
		"http://169.254.169.254",
		"https://169.254.169.254/",
		"http://user@169.254.169.254/",       // userinfo-in-authority trick
		"http://[fd00:ec2::254]/",            // IMDSv6
		"http://[64:ff9b::169.254.169.254]/", // NAT64-embedded link-local
		"http://[::169.254.169.254]/",        // IPv4-compatible link-local
		"http://[fe80::1]/",                  // link-local v6
		"http://0.0.0.0/",                    // unspecified
		"http://",                            // no host
		"",                                   // no scheme/host
		"://noscheme",                        // empty scheme
	}
	for _, u := range invalid {
		if err := ValidateProbeURL(u); err == nil {
			t.Errorf("ValidateProbeURL(%q) = nil, want error", u)
		}
	}
	// The error for a restricted IP literal should name the host (UX for the 422).
	if err := ValidateProbeURL("http://169.254.169.254/"); err == nil ||
		!strings.Contains(err.Error(), "169.254.169.254") {
		t.Errorf("expected error naming 169.254.169.254, got %v", err)
	}
}
