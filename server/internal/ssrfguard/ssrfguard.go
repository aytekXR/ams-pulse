// Package ssrfguard is a small, dependency-free SSRF policy shared by the probe
// API handlers (validate a stored URL) and the prober's network dialers (refuse
// to connect to a restricted address). It exists as a leaf package so both the
// api and prober packages can import it without a layering cycle, and so the
// policy lives in exactly one tested place.
//
// Policy (D-130 / S62 finding [21]). The prober fetches operator-stored URLs
// from inside the server's trust boundary, so an unrestricted URL is a
// server-side request forgery (SSRF) sink — most dangerously the cloud instance
// metadata endpoint, which can hand out IAM credentials even to an app-level
// admin. We therefore DENY:
//   - link-local unicast/multicast (IPv4 169.254.0.0/16 — the IMDSv4 address
//     169.254.169.254 that AWS/GCP/Azure all use — and IPv6 fe80::/10, ff02::/16),
//   - the unspecified address (0.0.0.0, ::), and
//   - the AWS IMDSv6 address fd00:ec2::254 (a carve-out from otherwise-allowed ULA).
//
// We deliberately ALLOW loopback and RFC-1918 / IPv6-ULA private ranges: a
// self-hosted AMS node is routinely on an internal network (this mirrors the
// B4/A6 ruling for the AMS source-connectivity test — see api/server.go:1910 —
// and keeps the loopback-based prober test suite valid). The IP check runs at
// dial time on the *resolved* address (see DialControl), so it is
// DNS-rebinding-safe and covers every HTTP redirect hop.
package ssrfguard

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
)

// imdsV6 is the AWS IPv6 instance-metadata address. It sits inside fc00::/7
// (unique-local), which we otherwise allow as the IPv6 analogue of RFC-1918, so
// it is denied explicitly rather than by range.
var imdsV6 = net.ParseIP("fd00:ec2::254")

// allowedSchemes is the probe URL scheme allowlist. It spans every protocol the
// prober actually dials (http/https for HLS/DASH/reachability, ws/wss for the
// WebRTC signaling handshake, rtmp/rtmps for the RTMP handshake) and nothing
// else — rejecting file://, gopher://, dict://, ftp://, data:, etc.
var allowedSchemes = map[string]bool{
	"http":  true,
	"https": true,
	"ws":    true,
	"wss":   true,
	"rtmp":  true,
	"rtmps": true,
}

// IsDenied reports whether ip is an address the prober must never connect to.
// A nil IP fails closed (denied): callers resolve to a concrete address before
// calling, so nil signals an unexpected, untrusted input.
func IsDenied(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// Reduce any IPv6 form that embeds an IPv4 address to that IPv4 so the v4
	// predicates below apply. This covers IPv4-mapped (::ffff:a.b.c.d), the NAT64
	// well-known prefix (64:ff9b::a.b.c.d, RFC 6052), and IPv4-compatible
	// (::a.b.c.d, RFC 4291 deprecated) — a kernel/NAT64 router can translate all of
	// them back to the embedded IPv4, so 64:ff9b::169.254.169.254 must be denied
	// exactly like 169.254.169.254.
	if v4 := embeddedIPv4(ip); v4 != nil {
		ip = v4
	}
	switch {
	case ip.IsUnspecified(): // 0.0.0.0, ::
		return true
	case ip.IsLinkLocalUnicast(): // 169.254.0.0/16 (IMDSv4), fe80::/10
		return true
	case ip.IsLinkLocalMulticast(): // 224.0.0.0/24, ff02::/16
		return true
	case ip.Equal(imdsV6): // AWS IMDSv6 (ULA is otherwise allowed)
		return true
	}
	return false
}

// nat64Prefix is the NAT64 well-known prefix 64:ff9b::/96 (RFC 6052): its low 32
// bits embed an IPv4 address a NAT64 router translates and forwards.
var nat64Prefix = []byte{0x00, 0x64, 0xff, 0x9b, 0, 0, 0, 0, 0, 0, 0, 0}

// v4CompatPrefix is the IPv4-compatible prefix ::/96 (RFC 4291 §2.5.5.1,
// deprecated): the low 32 bits carry an IPv4 address.
var v4CompatPrefix = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

// embeddedIPv4 returns the IPv4 address embedded in ip for the IPv4-mapped,
// NAT64, and IPv4-compatible forms, or nil when ip carries no embedded IPv4
// (a plain IPv4 is returned as-is; a genuine IPv6 address yields nil). Both
// embedding prefixes translate to their low 32 bits, so the address must be
// judged by that embedded IPv4 rather than by its (non-special) IPv6 wrapper.
func embeddedIPv4(ip net.IP) net.IP {
	if v4 := ip.To4(); v4 != nil {
		return v4 // plain IPv4 or IPv4-mapped ::ffff:a.b.c.d
	}
	if len(ip) != net.IPv6len {
		return nil
	}
	if bytes.HasPrefix(ip, nat64Prefix) || bytes.HasPrefix(ip, v4CompatPrefix) {
		return net.IP(ip[12:16]).To4()
	}
	return nil
}

// DialControl is a net.Dialer.Control hook. The dialer invokes it after DNS
// resolution but before the socket connects, with address == "ip:port" for the
// concrete resolved peer — so refusing here is DNS-rebinding-safe (the resolved
// IP is vetted, not the hostname) and, when installed on an http.Client's
// transport, re-runs for every redirect hop. Returning an error aborts the dial.
func DialControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		// Post-resolution the address is always host:port; anything else is
		// unexpected — fail closed rather than let an unparsed target through.
		return fmt.Errorf("ssrfguard: cannot parse dial address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// Control is called with a resolved IP literal; a non-IP here is
		// unexpected — fail closed.
		return fmt.Errorf("ssrfguard: dial address host %q is not a resolved IP", host)
	}
	if IsDenied(ip) {
		return fmt.Errorf("ssrfguard: refusing to dial restricted address %s", host)
	}
	return nil
}

// ValidateProbeURL is the API-boundary check for an operator-supplied probe URL.
// It rejects a disallowed scheme, a missing host, and an IP-literal host that is
// itself restricted (the common http://169.254.169.254/ case) so the create/update
// handlers return a clean 422 instead of silently accepting a URL the dialer will
// later refuse. A hostname that only resolves to a restricted address is NOT
// rejected here — resolving at validation time is both DNS-rebinding-bypassable
// and would wrongly reject a hostname that legitimately resolves to an allowed
// private AMS node; DialControl is the authoritative, rebinding-safe enforcement.
func ValidateProbeURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("url is not parseable")
	}
	if !allowedSchemes[strings.ToLower(u.Scheme)] {
		return fmt.Errorf("url scheme %q is not allowed (use http, https, ws, wss, rtmp, or rtmps)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url must include a host")
	}
	if ip := net.ParseIP(host); ip != nil && IsDenied(ip) {
		return fmt.Errorf("url host %s is a restricted address", host)
	}
	return nil
}
