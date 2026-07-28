import Foundation

// MARK: - ServerURLValidation

/// Result of validating a user-entered server URL.
public enum ServerURLValidation: Sendable, Equatable {
    /// The URL is valid and safe to use.
    case valid(URL)

    /// The URL string could not be parsed.
    case invalidURL

    /// The URL uses HTTP on a non-local host.
    case insecureTransport(host: String)
}

// MARK: - validateServerURL

/// Validate a user-entered server URL for security.
///
/// Pulse is self-hosted: users enter their own server URL. This function protects
/// the user's bearer token from being sent over an unencrypted channel to an
/// untrusted network.
///
/// ## Policy
///
/// - **HTTPS**: Accept for any host.
/// - **HTTP**: Accept ONLY for hosts that are:
///   - Loopback: `127.0.0.0/8`, `::1`, `localhost`
///   - Link-local: `.local` suffix, `169.254.0.0/16`, `fe80::/10`
///   - RFC1918 private: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`
///   - RFC4193 unique-local: `fc00::/7`
///   - IPv4-mapped IPv6 forms of the above: `::ffff:10.0.0.1`
///
/// ## Difference from server-side ssrfguard
///
/// The server's ssrfguard DENIES 169.254.0.0/16 (cloud IMDS) because it protects
/// server-to-server SSRF. This client-side rule ALLOWS 169.254 because the user
/// might legitimately connect to their own AMS on a link-local address. The
/// client-side rule protects the bearer token, not cloud metadata.
///
/// - Parameter urlString: The user-entered URL string.
/// - Returns: The validation result.
public func validateServerURL(_ urlString: String) -> ServerURLValidation {
    // Parse the URL
    guard let url = URL(string: urlString),
          let scheme = url.scheme?.lowercased(),
          let host = url.host else {
        return .invalidURL
    }

    // Only HTTP and HTTPS are valid schemes
    guard scheme == "http" || scheme == "https" else {
        return .invalidURL
    }

    // HTTPS is always allowed
    if scheme == "https" {
        return .valid(url)
    }

    // HTTP requires a local/private host
    if isLocalOrPrivate(host: host) {
        return .valid(url)
    }

    return .insecureTransport(host: host)
}

// MARK: - Private Helpers

/// Check if a host is a local or private address where HTTP is acceptable.
private func isLocalOrPrivate(host: String) -> Bool {
    let lowercased = host.lowercased()

    // "localhost" is always local
    if lowercased == "localhost" {
        return true
    }

    // .local suffix (mDNS/Bonjour)
    if lowercased.hasSuffix(".local") {
        return true
    }

    // Check if it's an IPv6 address (bracketed in URLs, but host property strips brackets)
    if let ipv6Range = parseIPv6(host) {
        return isPrivateIPv6(ipv6Range)
    }

    // Check if it's an IPv4 address
    if let octets = parseIPv4(host) {
        return isPrivateIPv4(octets)
    }

    // Not a recognized IP address format - must be a domain name
    // Domain names other than localhost/.local are not considered private
    return false
}

/// Parse an IPv4 address into its four octets.
private func parseIPv4(_ host: String) -> (UInt8, UInt8, UInt8, UInt8)? {
    let parts = host.split(separator: ".")
    guard parts.count == 4 else { return nil }

    var octets: [UInt8] = []
    for part in parts {
        guard let value = UInt8(part) else { return nil }
        octets.append(value)
    }

    return (octets[0], octets[1], octets[2], octets[3])
}

/// Check if an IPv4 address is private/local.
private func isPrivateIPv4(_ octets: (UInt8, UInt8, UInt8, UInt8)) -> Bool {
    let (a, b, _, _) = octets

    // Loopback: 127.0.0.0/8
    if a == 127 {
        return true
    }

    // RFC1918 10.0.0.0/8
    if a == 10 {
        return true
    }

    // RFC1918 172.16.0.0/12 (172.16.0.0 - 172.31.255.255)
    if a == 172 && b >= 16 && b <= 31 {
        return true
    }

    // RFC1918 192.168.0.0/16
    if a == 192 && b == 168 {
        return true
    }

    // Link-local 169.254.0.0/16 (allowed client-side, unlike server ssrfguard)
    if a == 169 && b == 254 {
        return true
    }

    return false
}

/// Parse an IPv6 address (without brackets).
/// Returns the normalized address for checking.
private func parseIPv6(_ host: String) -> [UInt16]? {
    // Handle IPv4-mapped IPv6 addresses like ::ffff:10.0.0.1
    if host.lowercased().hasPrefix("::ffff:") {
        let ipv4Part = String(host.dropFirst(7))
        if let octets = parseIPv4(ipv4Part) {
            // Convert to IPv6 representation
            // ::ffff:a.b.c.d -> [0, 0, 0, 0, 0, 0xffff, (a<<8)|b, (c<<8)|d]
            let high = (UInt16(octets.0) << 8) | UInt16(octets.1)
            let low = (UInt16(octets.2) << 8) | UInt16(octets.3)
            return [0, 0, 0, 0, 0, 0xFFFF, high, low]
        }
        return nil
    }

    // Parse standard IPv6 format
    var groups: [UInt16] = []
    var hasDoubleColon = false
    var doubleColonIndex = 0

    // Handle leading ::
    var workingHost = host.lowercased()
    if workingHost.hasPrefix("::") {
        hasDoubleColon = true
        doubleColonIndex = 0
        workingHost = String(workingHost.dropFirst(2))
    }

    if workingHost.isEmpty {
        // Just "::" - all zeros
        return [0, 0, 0, 0, 0, 0, 0, 0]
    }

    let parts = workingHost.split(separator: ":", omittingEmptySubsequences: false)

    for (_, part) in parts.enumerated() {
        if part.isEmpty {
            // This is where :: appears
            if hasDoubleColon {
                // Can't have two ::
                return nil
            }
            hasDoubleColon = true
            doubleColonIndex = groups.count
        } else {
            guard let value = UInt16(part, radix: 16) else { return nil }
            groups.append(value)
        }
    }

    // Expand :: to fill 8 groups
    if hasDoubleColon {
        let zerosNeeded = 8 - groups.count
        var expanded: [UInt16] = []
        for (index, group) in groups.enumerated() {
            if index == doubleColonIndex {
                for _ in 0..<zerosNeeded {
                    expanded.append(0)
                }
            }
            expanded.append(group)
        }
        // Handle case where :: is at the end
        if doubleColonIndex == groups.count {
            for _ in 0..<zerosNeeded {
                expanded.append(0)
            }
        }
        groups = expanded
    }

    guard groups.count == 8 else { return nil }
    return groups
}

/// Check if an IPv6 address is private/local.
private func isPrivateIPv6(_ groups: [UInt16]) -> Bool {
    guard groups.count == 8 else { return false }

    // Loopback ::1
    if groups[0] == 0 && groups[1] == 0 && groups[2] == 0 && groups[3] == 0 &&
       groups[4] == 0 && groups[5] == 0 && groups[6] == 0 && groups[7] == 1 {
        return true
    }

    // IPv4-mapped address ::ffff:x.x.x.x
    if groups[0] == 0 && groups[1] == 0 && groups[2] == 0 && groups[3] == 0 &&
       groups[4] == 0 && groups[5] == 0xFFFF {
        // Extract IPv4 and check
        let a = UInt8(groups[6] >> 8)
        let b = UInt8(groups[6] & 0xFF)
        let c = UInt8(groups[7] >> 8)
        let d = UInt8(groups[7] & 0xFF)
        return isPrivateIPv4((a, b, c, d))
    }

    // RFC4193 Unique Local fc00::/7 (fc00:: - fdff::)
    let firstByte = groups[0] >> 8
    if firstByte == 0xFC || firstByte == 0xFD {
        return true
    }

    // Link-local fe80::/10
    if (groups[0] & 0xFFC0) == 0xFE80 {
        return true
    }

    return false
}
