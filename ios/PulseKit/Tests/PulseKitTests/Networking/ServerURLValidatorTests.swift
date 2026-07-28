import Foundation
import Testing
@testable import PulseKit

/// Tests for ServerURLValidator - security rule for user-entered server URLs.
/// Table-driven tests from PULSEKIT-CONTRACT.md section E.
@Suite("ServerURLValidator Tests")
struct ServerURLValidatorTests {

    // MARK: - HTTPS (always accepted)

    @Test("HTTPS any host is accepted")
    func httpsAnyHost_accepted() {
        let result = validateServerURL("https://pulse.example.com")
        guard case .valid(let url) = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
        #expect(url.absoluteString == "https://pulse.example.com")
    }

    @Test("HTTPS private IP is accepted")
    func httpsPrivateIP_accepted() {
        let result = validateServerURL("https://10.0.0.5:8090")
        guard case .valid(let url) = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
        #expect(url.host == "10.0.0.5")
    }

    @Test("HTTPS IPv6 loopback is accepted")
    func httpsIPv6Loopback_accepted() {
        let result = validateServerURL("https://[::1]:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    // MARK: - HTTP Loopback (accepted)

    @Test("HTTP 127.0.0.1 loopback is accepted")
    func httpLoopback127_accepted() {
        let result = validateServerURL("http://127.0.0.1:8090")
        guard case .valid(let url) = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
        #expect(url.host == "127.0.0.1")
    }

    @Test("HTTP 127.x.x.x loopback range is accepted")
    func httpLoopback127Range_accepted() {
        // All of 127.0.0.0/8 is loopback
        let result = validateServerURL("http://127.255.255.1:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    @Test("HTTP localhost is accepted")
    func httpLoopbackLocalhost_accepted() {
        let result = validateServerURL("http://localhost:8090")
        guard case .valid(let url) = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
        #expect(url.host == "localhost")
    }

    @Test("HTTP IPv6 loopback ::1 is accepted")
    func httpLoopbackIPv6_accepted() {
        let result = validateServerURL("http://[::1]:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    // MARK: - HTTP RFC1918 Private (accepted)

    @Test("HTTP 10.x.x.x is accepted (RFC1918)")
    func httpRFC1918_10_accepted() {
        let result = validateServerURL("http://10.0.0.5:8090")
        guard case .valid(let url) = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
        #expect(url.host == "10.0.0.5")
    }

    @Test("HTTP 10.255.255.255 edge case is accepted")
    func httpRFC1918_10_edge_accepted() {
        let result = validateServerURL("http://10.255.255.255:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    @Test("HTTP 172.16.x.x is accepted (RFC1918)")
    func httpRFC1918_172_16_accepted() {
        let result = validateServerURL("http://172.16.0.1:8090")
        guard case .valid(let url) = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
        #expect(url.host == "172.16.0.1")
    }

    @Test("HTTP 172.31.x.x is accepted (RFC1918 upper bound)")
    func httpRFC1918_172_31_accepted() {
        let result = validateServerURL("http://172.31.255.255:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    @Test("HTTP 172.15.x.x is rejected (below RFC1918 range)")
    func httpRFC1918_172_15_rejected() {
        let result = validateServerURL("http://172.15.0.1:8090")
        guard case .insecureTransport(let host) = result else {
            Issue.record("Expected .insecureTransport, got \(result)")
            return
        }
        #expect(host == "172.15.0.1")
    }

    @Test("HTTP 172.32.x.x is rejected (above RFC1918 range)")
    func httpRFC1918_172_32_rejected() {
        let result = validateServerURL("http://172.32.0.1:8090")
        guard case .insecureTransport(let host) = result else {
            Issue.record("Expected .insecureTransport, got \(result)")
            return
        }
        #expect(host == "172.32.0.1")
    }

    @Test("HTTP 192.168.x.x is accepted (RFC1918)")
    func httpRFC1918_192_accepted() {
        let result = validateServerURL("http://192.168.1.100:8090")
        guard case .valid(let url) = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
        #expect(url.host == "192.168.1.100")
    }

    @Test("HTTP 192.167.x.x is rejected (not RFC1918)")
    func httpRFC1918_192_167_rejected() {
        let result = validateServerURL("http://192.167.1.1:8090")
        guard case .insecureTransport = result else {
            Issue.record("Expected .insecureTransport, got \(result)")
            return
        }
    }

    // MARK: - HTTP .local (accepted)

    @Test("HTTP .local suffix is accepted")
    func httpDotLocal_accepted() {
        let result = validateServerURL("http://mynas.local:8090")
        guard case .valid(let url) = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
        #expect(url.host == "mynas.local")
    }

    @Test("HTTP printer.local is accepted")
    func httpDotLocal_printer_accepted() {
        let result = validateServerURL("http://printer.local")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    // MARK: - HTTP RFC4193 IPv6 Unique-Local (accepted)

    @Test("HTTP fc00:: unique-local IPv6 is accepted")
    func httpRFC4193_fc00_accepted() {
        let result = validateServerURL("http://[fc00::1]:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    @Test("HTTP fd00:: unique-local IPv6 is accepted")
    func httpRFC4193_fd00_accepted() {
        let result = validateServerURL("http://[fd12:3456:789a::1]:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    // MARK: - HTTP IPv4-mapped IPv6 (accepted if underlying IPv4 is private)

    @Test("HTTP IPv4-mapped IPv6 with private IP is accepted")
    func httpMappedIPv4Private_accepted() {
        let result = validateServerURL("http://[::ffff:10.0.0.1]:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    @Test("HTTP IPv4-mapped IPv6 with 192.168 is accepted")
    func httpMappedIPv4_192_168_accepted() {
        let result = validateServerURL("http://[::ffff:192.168.1.1]:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    @Test("HTTP IPv4-mapped IPv6 with loopback is accepted")
    func httpMappedIPv4_loopback_accepted() {
        let result = validateServerURL("http://[::ffff:127.0.0.1]:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    @Test("HTTP IPv4-mapped IPv6 with public IP is rejected")
    func httpMappedIPv4Public_rejected() {
        let result = validateServerURL("http://[::ffff:8.8.8.8]:8090")
        guard case .insecureTransport = result else {
            Issue.record("Expected .insecureTransport, got \(result)")
            return
        }
    }

    // MARK: - HTTP Link-Local 169.254.x.x (accepted - differs from server SSRF guard)

    @Test("HTTP 169.254.x.x link-local is accepted")
    func httpLinkLocal169_accepted() {
        // Note: This differs from server-side ssrfguard which blocks 169.254.0.0/16
        // Client-side allows it because user might connect to their own AMS on link-local
        let result = validateServerURL("http://169.254.169.254")
        guard case .valid = result else {
            Issue.record("Expected .valid for 169.254.x.x, got \(result)")
            return
        }
    }

    @Test("HTTP 169.254.1.1 is accepted")
    func httpLinkLocal169_1_1_accepted() {
        let result = validateServerURL("http://169.254.1.1:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    // MARK: - HTTP Public (rejected)

    @Test("HTTP public domain is rejected")
    func httpPublicDomain_rejected() {
        let result = validateServerURL("http://pulse.example.com")
        guard case .insecureTransport(let host) = result else {
            Issue.record("Expected .insecureTransport, got \(result)")
            return
        }
        #expect(host == "pulse.example.com")
    }

    @Test("HTTP public IPv4 is rejected")
    func httpPublicIPv4_rejected() {
        let result = validateServerURL("http://8.8.8.8:8090")
        guard case .insecureTransport(let host) = result else {
            Issue.record("Expected .insecureTransport, got \(result)")
            return
        }
        #expect(host == "8.8.8.8")
    }

    @Test("HTTP public IPv6 is rejected")
    func httpPublicIPv6_rejected() {
        let result = validateServerURL("http://[2001:db8::1]:8090")
        guard case .insecureTransport = result else {
            Issue.record("Expected .insecureTransport, got \(result)")
            return
        }
    }

    @Test("HTTP Cloudflare DNS is rejected")
    func httpPublicCloudflare_rejected() {
        let result = validateServerURL("http://1.1.1.1:8090")
        guard case .insecureTransport = result else {
            Issue.record("Expected .insecureTransport, got \(result)")
            return
        }
    }

    // MARK: - Invalid URLs (rejected)

    @Test("FTP scheme is rejected")
    func ftpScheme_rejected() {
        let result = validateServerURL("ftp://pulse.example.com")
        guard case .invalidURL = result else {
            Issue.record("Expected .invalidURL, got \(result)")
            return
        }
    }

    @Test("Invalid URL string is rejected")
    func invalidURL_rejected() {
        let result = validateServerURL("not-a-url")
        guard case .invalidURL = result else {
            Issue.record("Expected .invalidURL, got \(result)")
            return
        }
    }

    @Test("Empty string is rejected")
    func emptyString_rejected() {
        let result = validateServerURL("")
        guard case .invalidURL = result else {
            Issue.record("Expected .invalidURL, got \(result)")
            return
        }
    }

    @Test("URL without scheme is rejected")
    func noScheme_rejected() {
        let result = validateServerURL("pulse.example.com")
        guard case .invalidURL = result else {
            Issue.record("Expected .invalidURL, got \(result)")
            return
        }
    }

    @Test("File scheme is rejected")
    func fileScheme_rejected() {
        let result = validateServerURL("file:///etc/passwd")
        guard case .invalidURL = result else {
            Issue.record("Expected .invalidURL, got \(result)")
            return
        }
    }

    // MARK: - Edge cases

    @Test("HTTP with path is handled correctly")
    func httpWithPath_handled() {
        let result = validateServerURL("http://192.168.1.1:8090/api/v1")
        guard case .valid(let url) = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
        #expect(url.path == "/api/v1")
    }

    @Test("HTTPS with query string is handled correctly")
    func httpsWithQuery_handled() {
        let result = validateServerURL("https://example.com?debug=true")
        guard case .valid(let url) = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
        #expect(url.query == "debug=true")
    }

    @Test("URL with username/password is handled")
    func urlWithCredentials_handled() {
        // Even with credentials, the security check should work
        let result = validateServerURL("http://user:pass@192.168.1.1:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    @Test("HTTPS uppercase is accepted")
    func httpsUppercase_accepted() {
        let result = validateServerURL("HTTPS://pulse.example.com")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    @Test("HTTP uppercase is handled correctly")
    func httpUppercase_handled() {
        let result = validateServerURL("HTTP://192.168.1.1:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    // MARK: - IPv6 edge cases

    @Test("HTTP fe80:: link-local IPv6 is accepted")
    func httpIPv6LinkLocal_accepted() {
        // Link-local IPv6 (fe80::/10) should be allowed
        let result = validateServerURL("http://[fe80::1]:8090")
        guard case .valid = result else {
            Issue.record("Expected .valid, got \(result)")
            return
        }
    }

    @Test("HTTP global IPv6 address is rejected")
    func httpIPv6Global_rejected() {
        // 2001:4860:4860::8888 is Google's public DNS
        let result = validateServerURL("http://[2001:4860:4860::8888]:8090")
        guard case .insecureTransport = result else {
            Issue.record("Expected .insecureTransport, got \(result)")
            return
        }
    }
}
