import SwiftUI
import AVKit
import PulseKit
import PulseBeacon

// MARK: - Player View
//
// HLS video player with full PulseBeacon QoE instrumentation.
//
// This is the key deliverable for roadmap item 2.12 (iOS Phase 2): a sample
// demonstrating PulseBeacon integration in a real iOS player.
//
// Instrumentation events emitted:
//   - sessionStart: when the view appears and playback is attempted
//   - startupComplete: when AVPlayer first transitions to .playing, with startup_ms
//   - heartbeat: every 10s while playing, with watch_ms
//   - rebufferStart: when timeControlStatus transitions to .waitingToPlayAtSpecifiedRate
//   - rebufferEnd: when playback resumes, with duration_ms of the stall
//   - error: on AVPlayerItem.failedToPlayToEndTime or asset load failure
//   - sessionEnd: when the view disappears, with total watch_ms and reason
//
// Design notes:
//   1. Uses AVPlayerViewController for native system player UI (scrubbing, PiP, etc.).
//   2. Observes AVPlayer.timeControlStatus for play/pause/buffering state.
//   3. Observes AVPlayerItem notifications for errors.
//   4. Tracks startup time from asset loading start to first .playing state.
//   5. Beacon is disposed on view disappear to ensure session_end is sent.
//
// Swift 6 concurrency: Uses @MainActor isolated playback state to ensure all
// AVPlayer observation and beacon calls happen on the main thread. The heartbeat
// and rebuffer checks run in an async task loop rather than Timer.scheduledTimer.

struct PlayerView: View {

    // MARK: - Environment

    @Environment(\.appState) private var appState
    @Environment(\.dismiss) private var dismiss

    // MARK: - Input

    /// The stream to play. Used for UI display and HLS URL construction.
    let stream: LiveStream

    // MARK: - State

    /// The AVPlayer instance.
    @State private var player: AVPlayer?

    /// Playback error message.
    @State private var errorMessage: String?

    /// PulseBeacon instance for QoE telemetry.
    @State private var beacon: PulseBeacon?

    // MARK: - Tracking State

    /// True once startupComplete has been emitted.
    @State private var hasEmittedStartup: Bool = false

    /// Timestamp when asset loading began (for startup_ms calculation).
    @State private var loadStartTime: Date?

    /// Timestamp when a rebuffer started (for duration_ms calculation).
    @State private var rebufferStartTime: Date?

    /// Total watch time in milliseconds (for heartbeat and session_end).
    @State private var totalWatchMs: Int = 0

    /// Last heartbeat time.
    @State private var lastHeartbeatTime: Date?

    /// Last observed timeControlStatus for detecting transitions.
    @State private var lastTimeControlStatus: AVPlayer.TimeControlStatus?

    // MARK: - Constants

    /// Heartbeat interval in nanoseconds (10 seconds).
    private let heartbeatInterval: UInt64 = 10_000_000_000

    // MARK: - Body

    var body: some View {
        VStack(spacing: 0) {
            // Player.
            if let player = player {
                VideoPlayer(player: player)
                    .ignoresSafeArea(.all, edges: .horizontal)
            } else {
                loadingView
            }

            // Error overlay.
            if let errorMessage = errorMessage {
                errorView(message: errorMessage)
            }

            // Stream info bar.
            streamInfoBar
        }
        .background(Color.black)
        .navigationTitle(stream.streamId)
        .navigationBarTitleDisplayMode(.inline)
        .onAppear {
            setupPlayer()
        }
        .onDisappear {
            teardownPlayer()
        }
        .task {
            // Heartbeat and playback state monitoring loop.
            await playbackMonitoringLoop()
        }
    }

    // MARK: - Subviews

    private var loadingView: some View {
        VStack(spacing: 16) {
            ProgressView()
                .scaleEffect(1.5)
                .tint(.white)

            Text("Loading stream...")
                .font(.system(size: 14, weight: .medium))
                .foregroundColor(.white.opacity(0.8))
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Color.black)
    }

    private func errorView(message: String) -> some View {
        HStack(spacing: 12) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundColor(BrandColors.critical)

            Text(message)
                .font(.system(size: 14, weight: .regular))
                .foregroundColor(.white)
                .multilineTextAlignment(.leading)

            Spacer()
        }
        .padding(16)
        .background(Color.red.opacity(0.3))
    }

    private var streamInfoBar: some View {
        HStack(spacing: 16) {
            // Health score.
            HStack(spacing: 8) {
                Circle()
                    .fill(healthColor(for: stream.healthScore))
                    .frame(width: 10, height: 10)
                Text(Formatters.healthScoreDisplay(stream.healthScore))
                    .font(.system(size: 12, weight: .medium))
                    .foregroundColor(.white)
            }

            Divider()
                .frame(height: 16)
                .background(Color.white.opacity(0.3))

            // Viewer count.
            Label(Formatters.abbreviatedCount(stream.viewers), systemImage: "person.2")
                .font(.system(size: 12, weight: .medium))
                .foregroundColor(.white.opacity(0.8))

            Spacer()

            // App name.
            Text(stream.app)
                .font(.system(size: 12, weight: .medium))
                .foregroundColor(.white.opacity(0.6))
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .background(Color.black.opacity(0.9))
    }

    private func healthColor(for score: Double) -> Color {
        if score >= 90 { return BrandColors.healthy }
        else if score >= 70 { return BrandColors.warning }
        else { return BrandColors.critical }
    }

    // MARK: - Player Setup

    private func setupPlayer() {
        // Build HLS URL from stream info.
        // Standard AMS HLS endpoint: /<app>/streams/<streamId>.m3u8
        guard let appState = appState,
              case .signedIn(let serverURL) = appState.authState,
              let baseURL = URL(string: serverURL) else {
            errorMessage = "Not connected to a server"
            return
        }

        // NOTE: The HLS URL construction assumes the AMS server is accessible from the device.
        // In production, this might need to be configurable or use a different URL scheme.
        // For self-hosted Pulse + AMS setups, the AMS server URL is typically different
        // from the Pulse server URL. This is a simplification for the sample app.
        //
        // A real implementation would:
        // 1. Get the AMS server URL from the Pulse API (not currently exposed).
        // 2. Or use a configurable AMS base URL in settings.
        // 3. Or the LiveStream model could include the playback URL directly.
        //
        // For this sample, we construct a URL relative to the Pulse server, which
        // would only work if Pulse is proxying the AMS streams (uncommon).
        let hlsPath = "/\(stream.app)/streams/\(stream.streamId).m3u8"
        let hlsURL = baseURL.appendingPathComponent(hlsPath)

        // Record load start time for startup_ms.
        loadStartTime = Date()

        // Initialize PulseBeacon.
        //
        // The beacon needs:
        //   - ingestURL: The Pulse server's beacon ingest endpoint.
        //   - token: Not the API token — the beacon uses a separate ingest token.
        //            For this sample, we'll use an empty token which may work if
        //            the server allows anonymous ingest. A production app would
        //            need a proper ingest token from the Pulse admin.
        //   - streamID: The AMS stream ID (for correlation with server metrics).
        //   - app: The AMS app name.
        //
        // TODO: In production, the beacon ingest token should be separate from the
        // API token and stored/retrieved appropriately.
        let beaconConfig = PulseBeacon.Config(
            ingestURL: serverURL,
            token: "", // Anonymous ingest; see note above.
            streamID: stream.streamId,
            app: stream.app,
            metadata: nil,
            sampleRate: 1.0, // Always report for this sample app.
            playerKind: .native
        )
        beacon = PulseBeacon(config: beaconConfig)

        // Emit session start.
        beacon?.sessionStart(autoplay: true)

        // Create player and item.
        let item = AVPlayerItem(url: hlsURL)
        let avPlayer = AVPlayer(playerItem: item)

        // Start playback.
        avPlayer.play()

        self.player = avPlayer
        self.lastHeartbeatTime = Date()
    }

    private func teardownPlayer() {
        // Stop playback.
        player?.pause()
        player = nil

        // Emit session end.
        beacon?.sessionEnd(watchMs: totalWatchMs, reason: "view_closed")
        beacon?.dispose()
        beacon = nil
    }

    // MARK: - Playback Monitoring Loop

    /// Async loop that monitors playback state and emits heartbeats.
    /// Cancelled automatically when the view disappears (task cancellation).
    private func playbackMonitoringLoop() async {
        // Check interval (1 second) for responsive state change detection.
        let checkInterval: UInt64 = 1_000_000_000 // 1 second
        var lastHeartbeat = Date()

        while !Task.isCancelled {
            do {
                try await Task.sleep(nanoseconds: checkInterval)
                // Not `await`: both helpers are synchronous and already on this
                // isolation domain. Swift 6 warns "no async operations occur
                // within 'await' expression", and a needless await here would
                // read as if it could suspend mid-playback-check, which it cannot.
                checkPlaybackState()

                // Emit heartbeat every 10 seconds while playing.
                let now = Date()
                if now.timeIntervalSince(lastHeartbeat) >= 10 {
                    emitHeartbeatIfPlaying()
                    lastHeartbeat = now
                }
            } catch {
                // Task was cancelled (view disappeared).
                break
            }
        }
    }

    @MainActor
    private func checkPlaybackState() {
        guard let player = player else { return }

        let currentStatus = player.timeControlStatus
        let previousStatus = lastTimeControlStatus
        lastTimeControlStatus = currentStatus

        // Detect state transitions.
        switch (previousStatus, currentStatus) {
        case (_, .playing):
            // Transitioned to playing.
            if !hasEmittedStartup {
                // First time playing — emit startup complete.
                if let loadStart = loadStartTime {
                    let startupMs = Int(Date().timeIntervalSince(loadStart) * 1000)
                    beacon?.startupComplete(startupMs: startupMs)
                }
                hasEmittedStartup = true
                lastHeartbeatTime = Date()
            }

            // Check if recovering from rebuffer.
            if let rebufferStart = rebufferStartTime {
                let durationMs = Int(Date().timeIntervalSince(rebufferStart) * 1000)
                beacon?.rebufferEnd(durationMs: durationMs)
                rebufferStartTime = nil
            }

        case (.playing?, .waitingToPlayAtSpecifiedRate):
            // Transitioned from playing to waiting — rebuffer start.
            if rebufferStartTime == nil {
                rebufferStartTime = Date()
                beacon?.rebufferStart()
            }

        default:
            break
        }

        // Check for errors on the player item.
        if let error = player.currentItem?.error {
            let message = error.localizedDescription
            if errorMessage == nil {
                errorMessage = "Playback error: \(message)"
                beacon?.error(code: "playback_failed", message: message, fatal: true)
            }
        }
    }

    @MainActor
    private func emitHeartbeatIfPlaying() {
        guard let player = player,
              player.timeControlStatus == .playing,
              let lastBeat = lastHeartbeatTime else {
            return
        }

        // Calculate watch time since last heartbeat.
        let now = Date()
        let watchMs = Int(now.timeIntervalSince(lastBeat) * 1000)
        totalWatchMs += watchMs
        lastHeartbeatTime = now

        // Emit heartbeat.
        beacon?.heartbeat(watchMs: watchMs)
    }
}

// MARK: - Preview
//
// Previews require a LiveStream instance, but PulseKit's LiveStream is Codable-only
// without a public memberwise initializer. We decode from JSON for previews.

#Preview {
    NavigationStack {
        PlayerView(stream: PreviewData.sampleStream)
            .environment(\.appState, AppState())
    }
}

/// Preview data for SwiftUI previews.
/// Uses JSON decoding because PulseKit models are Codable-only (no public init).
private enum PreviewData {
    static let sampleStream: LiveStream = {
        let json = """
        {
            "stream_id": "preview-stream",
            "app": "live",
            "viewers": 42,
            "publisher_state": "publishing",
            "health_score": 95.0,
            "bitrate_kbps": 2500
        }
        """
        return try! JSONDecoder().decode(LiveStream.self, from: json.data(using: .utf8)!)
    }()
}
