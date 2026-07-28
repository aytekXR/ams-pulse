// livecheck: live-server contract validation for PulseKit.
//
// Calls every GET endpoint PulseAPIClient exposes against a real Pulse server.
// Prints PASS/FAIL per endpoint and exits non-zero on any failure.
//
// SECURITY:
//   - Reads PULSE_URL and PULSE_TOKEN from environment variables ONLY.
//   - Never accepts credentials as command-line arguments (they land in shell
//     history and process lists).
//   - Refuses to run any method that is not a GET.
//
// Usage:
//   export PULSE_URL="https://pulse.example.com"
//   export PULSE_TOKEN="plt_abc123..."
//   swift run livecheck

import Foundation
import PulseKit

// MARK: - Configuration

struct Config {
    let baseURL: URL
    let token: String

    static func fromEnvironment() throws -> Config {
        guard let urlString = ProcessInfo.processInfo.environment["PULSE_URL"],
              !urlString.isEmpty else {
            throw ConfigError.missingEnv("PULSE_URL")
        }

        guard let url = URL(string: urlString) else {
            throw ConfigError.invalidURL(urlString)
        }

        guard let token = ProcessInfo.processInfo.environment["PULSE_TOKEN"],
              !token.isEmpty else {
            throw ConfigError.missingEnv("PULSE_TOKEN")
        }

        return Config(baseURL: url, token: token)
    }

    enum ConfigError: Error, CustomStringConvertible {
        case missingEnv(String)
        case invalidURL(String)

        var description: String {
            switch self {
            case .missingEnv(let name):
                return "Required environment variable \(name) is not set"
            case .invalidURL(let value):
                return "PULSE_URL is not a valid URL: \(value)"
            }
        }
    }
}

// MARK: - Check Result

enum CheckResult {
    case pass
    case fail(String)

    var passed: Bool {
        if case .pass = self { return true }
        return false
    }
}

// MARK: - Endpoint Checks

/// Run all GET endpoint checks against the server.
/// Returns the count of failures.
func runChecks(client: PulseAPIClient) async -> Int {
    var failures = 0

    // 1. GET /healthz (unauthenticated)
    let healthResult = await checkHealth(client: client)
    report("GET /healthz", healthResult)
    if !healthResult.passed { failures += 1 }

    // 2. GET /auth/me
    let authResult = await checkAuthMe(client: client)
    report("GET /auth/me", authResult)
    if !authResult.passed { failures += 1 }

    // 3. GET /api/v1/live/overview
    let overviewResult = await checkLiveOverview(client: client)
    report("GET /api/v1/live/overview", overviewResult)
    if !overviewResult.passed { failures += 1 }

    // 4. GET /api/v1/live/streams
    let streamsResult = await checkLiveStreams(client: client)
    report("GET /api/v1/live/streams", streamsResult)
    if !streamsResult.passed { failures += 1 }

    // 5. GET /api/v1/alerts/history
    let alertsResult = await checkAlertHistory(client: client)
    report("GET /api/v1/alerts/history", alertsResult)
    if !alertsResult.passed { failures += 1 }

    // 6. GET /api/v1/fleet/nodes
    let fleetResult = await checkFleetNodes(client: client)
    report("GET /api/v1/fleet/nodes", fleetResult)
    if !fleetResult.passed { failures += 1 }

    // 7. GET /api/v1/anomalies
    let anomaliesResult = await checkAnomalies(client: client)
    report("GET /api/v1/anomalies", anomaliesResult)
    if !anomaliesResult.passed { failures += 1 }

    // 8. GET /api/v1/qoe/summary
    let qoeResult = await checkQoeSummary(client: client)
    report("GET /api/v1/qoe/summary", qoeResult)
    if !qoeResult.passed { failures += 1 }

    // 9. GET /api/v1/admin/license
    let licenseResult = await checkLicense(client: client)
    report("GET /api/v1/admin/license", licenseResult)
    if !licenseResult.passed { failures += 1 }

    return failures
}

func checkHealth(client: PulseAPIClient) async -> CheckResult {
    do {
        let health = try await client.getHealth()
        // The health endpoint returned and decoded. We do not assert on status
        // because the server may be degraded but reachable.
        _ = health.status
        return .pass
    } catch {
        return .fail(describeError(error))
    }
}

func checkAuthMe(client: PulseAPIClient) async -> CheckResult {
    do {
        let auth = try await client.getAuthMe()
        // Verify we got a name back.
        guard !auth.name.isEmpty else {
            return .fail("response decoded but name is empty")
        }
        return .pass
    } catch {
        return .fail(describeError(error))
    }
}

func checkLiveOverview(client: PulseAPIClient) async -> CheckResult {
    do {
        let overview = try await client.getLiveOverview()
        // Verify timestamp is plausible (positive epoch ms).
        guard overview.ts > 0 else {
            return .fail("response decoded but ts <= 0")
        }
        return .pass
    } catch {
        return .fail(describeError(error))
    }
}

func checkLiveStreams(client: PulseAPIClient) async -> CheckResult {
    do {
        let streams = try await client.getLiveStreams()
        // The array may be empty or populated; decoding is the test.
        _ = streams.items.count
        return .pass
    } catch {
        return .fail(describeError(error))
    }
}

func checkAlertHistory(client: PulseAPIClient) async -> CheckResult {
    do {
        let alerts = try await client.getAlertHistory()
        _ = alerts.items.count
        return .pass
    } catch {
        return .fail(describeError(error))
    }
}

func checkFleetNodes(client: PulseAPIClient) async -> CheckResult {
    do {
        let nodes = try await client.getFleetNodes()
        _ = nodes.items.count
        return .pass
    } catch {
        return .fail(describeError(error))
    }
}

func checkAnomalies(client: PulseAPIClient) async -> CheckResult {
    do {
        let anomalies = try await client.getAnomalies()
        _ = anomalies.items.count
        return .pass
    } catch {
        return .fail(describeError(error))
    }
}

func checkQoeSummary(client: PulseAPIClient) async -> CheckResult {
    do {
        let qoe = try await client.getQoeSummary()
        // QoE summary always has a totals object.
        _ = qoe.totals
        return .pass
    } catch {
        return .fail(describeError(error))
    }
}

func checkLicense(client: PulseAPIClient) async -> CheckResult {
    do {
        let license = try await client.getLicense()
        // License has a tier.
        _ = license.tier
        return .pass
    } catch {
        return .fail(describeError(error))
    }
}

// MARK: - Output

func report(_ endpoint: String, _ result: CheckResult) {
    switch result {
    case .pass:
        print("PASS  \(endpoint)")
    case .fail(let reason):
        print("FAIL  \(endpoint): \(reason)")
    }
}

func describeError(_ error: Error) -> String {
    if let apiError = error as? PulseAPIError {
        return apiError.localizedDescription
    }
    return error.localizedDescription
}

// MARK: - Main

@main
struct LiveCheck {
    static func main() async {
        print("livecheck: PulseKit live-server contract validation")
        print("=========================================================")
        print("")

        // Load configuration from environment.
        let config: Config
        do {
            config = try Config.fromEnvironment()
        } catch {
            print("Configuration error: \(error)")
            print("")
            print("Usage:")
            print("  export PULSE_URL=\"https://pulse.example.com\"")
            print("  export PULSE_TOKEN=\"plt_abc123...\"")
            print("  swift run livecheck")
            exit(1)
        }

        print("Server: \(config.baseURL.absoluteString)")
        print("")

        // Create the client.
        let client = PulseAPIClient(
            baseURL: config.baseURL,
            tokenProvider: { config.token }
        )

        // Run all checks.
        let failures = await runChecks(client: client)

        print("")
        print("=========================================================")
        if failures == 0 {
            print("All checks passed.")
            exit(0)
        } else {
            print("\(failures) check(s) failed.")
            exit(1)
        }
    }
}
