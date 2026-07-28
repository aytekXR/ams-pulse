import XCTest

/// UI tests that drive the Pulse app through every screen via XCUITest.
///
/// RATIONALE (D-188/D-189): The app only ever reaches the CONNECT screen in CI
/// because nothing supplies a server. Every other screen — live dashboard,
/// streams, alerts, settings — has NEVER BEEN SEEN BY ANYONE. These tests point
/// the app at a local fixture server (fixture_server.py on localhost:8090) and
/// navigate through every tab, taking screenshots as evidence.
///
/// CONSTRAINT: No runtime backdoor in the app binary. The fixture server is the
/// only instrumentation; the app code path is identical to production.
///
/// FIXTURE SERVER: Must be running on localhost:8090 before tests start.
/// CI starts it in the "Start fixture server" workflow step. The server is a
/// Python script (ios/PulseApp/UITests/Fixtures/fixture_server.py) that serves
/// realistic JSON for all PulseAPIClient endpoints.
///
/// NSAllowsLocalNetworking in Info.plist permits plain HTTP to localhost.
/// PulseKit's ServerURLValidator accepts HTTP for loopback addresses.
///
/// SCREENSHOTS: Each test takes screenshots using XCTAttachment. These are
/// extracted from the .xcresult bundle and uploaded as workflow artifacts.
final class PulseUITests: XCTestCase {

    // MARK: - Properties

    private var app: XCUIApplication!

    /// Fixture server URL — matches fixture_server.py's default port.
    /// NSAllowsLocalNetworking in Info.plist permits plain HTTP to localhost.
    private let fixtureServerURL = "http://localhost:8090"

    /// Dummy token — fixture server accepts any non-empty Bearer token.
    private let fixtureToken = "plt_fixture_test_token_12345"

    // MARK: - Setup

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        // Launch with a clean slate.
        app.launchArguments = ["-ApplePersistenceIgnoreState", "YES"]
        app.launch()
    }

    override func tearDownWithError() throws {
        app = nil
    }

    // MARK: - Tests

    /// Full navigation test: connect, visit every tab, take screenshots.
    ///
    /// This is the primary UI test that proves every screen renders. It:
    /// 1. Connects to the fixture server via the ConnectView form
    /// 2. Verifies the Live dashboard loads with data
    /// 3. Navigates to Alerts tab and takes a screenshot
    /// 4. Navigates to Settings tab and takes a screenshot
    /// 5. Returns to Live tab for a final screenshot
    ///
    /// Each screenshot is attached to the test result for artifact upload.
    ///
    /// Fixture data expectations (from fixture_server.py):
    /// - total_viewers = 2847 -> formatted as "2.8K"
    /// - total_publishers = 42
    /// - 6 streams with different states
    /// - 5 alerts (2 firing, 2 resolved, 1 delivery_failure)
    /// - healthz status = "ok"
    func testFullAppNavigation() throws {
        // ────────────────────────────────────────────────────────────────────
        // Step 1: Connect to the fixture server
        // ────────────────────────────────────────────────────────────────────

        // The app should start on the ConnectView.
        let connectTitle = app.navigationBars["Connect to Pulse"]
        XCTAssertTrue(connectTitle.waitForExistence(timeout: 5),
                      "App should start on ConnectView")
        takeScreenshot(name: "01-connect-empty")

        // Enter the fixture server URL.
        // Find the text field — there should be one for the URL.
        let serverURLField = app.textFields.firstMatch
        XCTAssertTrue(serverURLField.waitForExistence(timeout: 5),
                      "Server URL field should exist")
        serverURLField.tap()
        serverURLField.typeText(fixtureServerURL)

        // Enter the fixture token.
        // The token field is a SecureField, which XCUITest sees as a secureTextField.
        let tokenField = app.secureTextFields.firstMatch
        XCTAssertTrue(tokenField.waitForExistence(timeout: 5),
                      "Token field should exist")
        tokenField.tap()
        tokenField.typeText(fixtureToken)

        takeScreenshot(name: "02-connect-filled")

        // Tap the Connect button.
        let connectButton = app.buttons["Connect"]
        XCTAssertTrue(connectButton.exists, "Connect button should exist")
        XCTAssertTrue(connectButton.isEnabled, "Connect button should be enabled")
        connectButton.tap()

        // ────────────────────────────────────────────────────────────────────
        // Step 2: Live dashboard should load
        // ────────────────────────────────────────────────────────────────────

        // Wait for the tab bar to appear (indicates we're past the connect screen).
        let tabBar = app.tabBars.firstMatch
        XCTAssertTrue(tabBar.waitForExistence(timeout: 10),
                      "Tab bar should appear after connecting")

        // The Live tab should be selected by default.
        let liveTab = tabBar.buttons["Live"]
        XCTAssertTrue(liveTab.exists, "Live tab should exist")

        // Wait a moment for data to load.
        sleep(2)

        takeScreenshot(name: "03-live-dashboard")

        // Assert the SPECIFIC values the fixture serves, located by identifier.
        //
        // The previous assertion here matched any label containing a digit or the
        // letter K. That passes on a screen showing "OK", on a screen showing a
        // stray "1", and on most broken screens — which makes it worse than no
        // assertion, because it reports coverage that does not exist.
        //
        // Fixture: total_viewers 2847, total_publishers 42.
        let viewersTile = app.otherElements[AccessibilityID.live_viewersTile]
        XCTAssertTrue(viewersTile.waitForExistence(timeout: 10),
                      "Live dashboard did not render its viewers tile")
        XCTAssertTrue(descendantLabels(of: viewersTile).contains { $0.contains("2.8K") || $0.contains("2847") },
                      "Viewers tile should show the fixture's 2847 viewers; saw: \(descendantLabels(of: viewersTile))")

        let publishersTile = app.otherElements[AccessibilityID.live_publishersTile]
        XCTAssertTrue(publishersTile.exists, "Live dashboard did not render its publishers tile")
        XCTAssertTrue(descendantLabels(of: publishersTile).contains { $0.contains("42") },
                      "Publishers tile should show the fixture's 42 publishers; saw: \(descendantLabels(of: publishersTile))")

        // And a specific stream from the fixture must be listed by name.
        XCTAssertTrue(app.staticTexts["keynote-2026-main"].waitForExistence(timeout: 5),
                      "The stream list should contain the fixture stream keynote-2026-main")

        // ────────────────────────────────────────────────────────────────────
        // Step 3: Navigate to Alerts tab
        // ────────────────────────────────────────────────────────────────────

        let alertsTab = tabBar.buttons["Alerts"]
        XCTAssertTrue(alertsTab.exists, "Alerts tab should exist")
        alertsTab.tap()

        // Wait for content to load.
        sleep(2)

        takeScreenshot(name: "04-alerts")

        // A screenshot is NOT an assertion. The previous version computed a
        // boolean and then printed a warning, so this step passed on an empty
        // list, a permanent spinner, and an error state alike — the exact
        // "harness that silently skips" shape this repo treats as no
        // verification at all.
        let alertsList = app.otherElements[AccessibilityID.alerts_list]
        XCTAssertTrue(alertsList.waitForExistence(timeout: 10),
                      "Alerts screen did not render its list container")
        XCTAssertFalse(app.otherElements[AccessibilityID.alerts_emptyView].exists,
                       "Alerts showed its empty state, but the fixture serves 5 alerts")
        XCTAssertFalse(app.otherElements[AccessibilityID.alerts_loadingView].exists,
                       "Alerts was still loading after 10s")
        XCTAssertTrue(app.staticTexts["rebuffer_ratio"].waitForExistence(timeout: 5),
                      "Alerts should list the fixture's rebuffer_ratio alert")

        // ────────────────────────────────────────────────────────────────────
        // Step 4: Navigate to Settings tab
        // ────────────────────────────────────────────────────────────────────

        let settingsTab = tabBar.buttons["Settings"]
        XCTAssertTrue(settingsTab.exists, "Settings tab should exist")
        settingsTab.tap()

        // Wait for content to load.
        sleep(2)

        takeScreenshot(name: "05-settings")

        let serverRow = app.otherElements[AccessibilityID.settings_serverRow]
        XCTAssertTrue(serverRow.waitForExistence(timeout: 10),
                      "Settings did not render the server row")
        XCTAssertTrue(descendantLabels(of: serverRow).contains { $0.contains("localhost") },
                      "Settings should show the connected server; saw: \(descendantLabels(of: serverRow))")
        // The fixture's /healthz reports every component ok — so the component
        // rows must be present, not merely the section heading.
        XCTAssertTrue(app.otherElements[AccessibilityID.settings_componentClickhouse].exists,
                      "Settings should show the ClickHouse component health row")

        // ────────────────────────────────────────────────────────────────────
        // Step 5: Return to Live tab (round trip complete)
        // ────────────────────────────────────────────────────────────────────

        liveTab.tap()
        sleep(1)

        takeScreenshot(name: "06-live-return")
    }

    /// Test the connect view error state with an invalid server URL.
    ///
    /// This test verifies that the error UI renders correctly when
    /// the server is unreachable. Uses a URL that will never connect.
    func testConnectErrorState() throws {
        // Enter a URL that won't resolve — a port with no server.
        let serverURLField = app.textFields.firstMatch
        XCTAssertTrue(serverURLField.waitForExistence(timeout: 5))
        serverURLField.tap()
        serverURLField.typeText("http://localhost:19999")

        let tokenField = app.secureTextFields.firstMatch
        XCTAssertTrue(tokenField.waitForExistence(timeout: 5))
        tokenField.tap()
        tokenField.typeText("invalid_token")

        let connectButton = app.buttons["Connect"]
        connectButton.tap()

        // Wait for the error to appear. Connection failure takes a few seconds.
        sleep(10)

        takeScreenshot(name: "07-connect-error")

        // A test named testConnectErrorState that cannot fail is not a test of
        // error state. The error banner has an identifier precisely so this can
        // be asserted rather than guessed at from free text.
        let banner = app.otherElements[AccessibilityID.connect_errorBanner]
        XCTAssertTrue(banner.waitForExistence(timeout: 15),
                      "Connecting to an unreachable host must surface the error banner")
        // And we must still be on the connect screen — a failed connection that
        // navigates onward would be a far worse defect than a missing message.
        XCTAssertFalse(app.tabBars.firstMatch.exists,
                       "A failed connection must not navigate into the dashboard")
    }

    /// Capture the same journey a second time.
    ///
    /// ⚠ NAME AND CONTENT NOTE. This was called testLightModeAppearance and it
    /// neither set the appearance nor asserted anything — it would have passed
    /// with every screen black. XCUITest cannot change the simulator's system
    /// appearance from inside the test; that is done by `xcrun simctl ui booted
    /// appearance light` on the host, which the CI job does around its own
    /// captures. So this test is honest about what it is: a second pass through
    /// the same screens, with the same assertions, producing a second set of
    /// captures. It is named for what it does.
    func testSecondPassCaptures() throws {
        // Connect first.
        let serverURLField = app.textFields.firstMatch
        XCTAssertTrue(serverURLField.waitForExistence(timeout: 5))
        serverURLField.tap()
        serverURLField.typeText(fixtureServerURL)

        let tokenField = app.secureTextFields.firstMatch
        tokenField.tap()
        tokenField.typeText(fixtureToken)

        app.buttons["Connect"].tap()

        // Wait for dashboard.
        let tabBar = app.tabBars.firstMatch
        XCTAssertTrue(tabBar.waitForExistence(timeout: 10))

        sleep(2)

        XCTAssertTrue(app.otherElements[AccessibilityID.live_viewersTile].waitForExistence(timeout: 10),
                      "Second pass: live dashboard did not render")
        takeScreenshot(name: "08-live-second-pass")

        tabBar.buttons["Alerts"].tap()
        XCTAssertTrue(app.otherElements[AccessibilityID.alerts_list].waitForExistence(timeout: 10),
                      "Second pass: alerts did not render")
        takeScreenshot(name: "09-alerts-second-pass")

        tabBar.buttons["Settings"].tap()
        XCTAssertTrue(app.otherElements[AccessibilityID.settings_serverRow].waitForExistence(timeout: 10),
                      "Second pass: settings did not render")
        takeScreenshot(name: "10-settings-second-pass")
    }

    // MARK: - Assertion helpers

    /// All labels beneath an element, for asserting on a composed tile rather
    /// than on whatever free text happens to be on screen.
    private func descendantLabels(of element: XCUIElement) -> [String] {
        let texts = element.descendants(matching: .staticText)
        return (0..<texts.count).map { texts.element(boundBy: $0).label }
    }

    // MARK: - Helpers

    /// Takes a screenshot and attaches it to the test result.
    ///
    /// Screenshots are extracted from the .xcresult bundle by CI and uploaded
    /// as workflow artifacts. The name should be descriptive and sortable
    /// (use numeric prefixes like "01-", "02-").
    private func takeScreenshot(name: String) {
        let screenshot = app.screenshot()
        let attachment = XCTAttachment(screenshot: screenshot)
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
