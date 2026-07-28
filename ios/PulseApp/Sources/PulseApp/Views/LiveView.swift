import SwiftUI
import PulseKit

// MARK: - Live View
//
// The main dashboard screen showing real-time overview tiles and stream list.
//
// Features:
//   - Overview tiles: viewers, publishers, protocol mix
//   - Active streams list with health indicators
//   - Pull-to-refresh for manual updates
//   - Auto-refresh via async task loop while foregrounded (10s interval per PRD F1)
//   - Tap a stream to navigate to the player
//
// Swift 6 concurrency: Auto-refresh uses a `.task` modifier with an async sleep
// loop, which is automatically cancelled when the view disappears. This avoids
// the strict concurrency issues of Timer.scheduledTimer capturing self.

struct LiveView: View {

    // MARK: - Environment

    @Environment(\.appState) private var appState
    @Environment(\.scenePhase) private var scenePhase

    // MARK: - State

    @State private var overview: LiveOverview?
    @State private var streams: [LiveStream] = []
    @State private var isLoading: Bool = false
    @State private var error: PulseAPIError?

    /// Controls whether the auto-refresh loop runs.
    @State private var isAutoRefreshActive: Bool = true

    // MARK: - Constants

    /// Refresh interval in seconds. Matches PRD F1's ~10s lag expectation.
    private let refreshInterval: UInt64 = 10_000_000_000 // 10 seconds in nanoseconds

    // MARK: - Body

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 24) {
                    // Error banner (if any).
                    if let error = error {
                        errorBanner(error: error)
                    }

                    // Overview tiles.
                    if let overview = overview {
                        overviewTiles(overview: overview)
                    }

                    // Streams list.
                    streamsList
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 16)
            }
            .background(BrandColors.bg.ignoresSafeArea())
            .navigationTitle("Live")
            .navigationBarTitleDisplayMode(.large)
            .refreshable {
                await refresh()
            }
            .toolbar {
                ToolbarItem(placement: .navigationBarTrailing) {
                    if isLoading {
                        ProgressView()
                    }
                }
            }
        }
        .task {
            // Initial load.
            await refresh()
        }
        .task {
            // Auto-refresh loop. Cancelled automatically when view disappears.
            await autoRefreshLoop()
        }
        .onChange(of: scenePhase) { oldPhase, newPhase in
            // Pause refresh when backgrounded, resume when foregrounded.
            switch newPhase {
            case .active:
                isAutoRefreshActive = true
                Task { await refresh() }
            case .background, .inactive:
                isAutoRefreshActive = false
            @unknown default:
                break
            }
        }
    }

    // MARK: - Auto-Refresh Loop

    /// Runs an async loop that refreshes data every 10 seconds.
    /// Automatically cancelled when the view disappears (task cancellation).
    private func autoRefreshLoop() async {
        while !Task.isCancelled {
            do {
                try await Task.sleep(nanoseconds: refreshInterval)
                if isAutoRefreshActive {
                    await refresh()
                }
            } catch {
                // Task was cancelled (view disappeared).
                break
            }
        }
    }

    // MARK: - Overview Tiles

    private func overviewTiles(overview: LiveOverview) -> some View {
        LazyVGrid(columns: [
            GridItem(.flexible(), spacing: 12),
            GridItem(.flexible(), spacing: 12)
        ], spacing: 12) {
            // Viewers tile.
            metricTile(
                title: "VIEWERS",
                value: Formatters.abbreviatedCount(overview.totalViewers),
                icon: "person.2.fill",
                color: BrandColors.signal
            )

            // Publishers tile.
            metricTile(
                title: "PUBLISHERS",
                value: Formatters.abbreviatedCount(overview.totalPublishers),
                icon: "video.fill",
                color: BrandColors.info
            )

            // Active streams tile.
            metricTile(
                title: "STREAMS",
                value: "\(streams.count)",
                icon: "play.circle.fill",
                color: BrandColors.healthy
            )

            // Protocol mix tile.
            protocolMixTile(protocolMix: overview.protocolMix)
        }
    }

    private func metricTile(title: String, value: String, icon: String, color: Color) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Image(systemName: icon)
                    .foregroundColor(color)
                    .font(.system(size: 20, weight: .semibold))
                Spacer()
            }

            Text(value)
                // tokens.json: metric size 40, weight 700.
                .font(.system(size: 40, weight: .bold))
                .foregroundColor(BrandColors.textPrimary)
                .monospacedDigit()

            Text(title)
                // tokens.json: label size 11, weight 500, uppercase.
                .font(.system(size: 11, weight: .medium))
                .foregroundColor(BrandColors.textMuted)
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(BrandColors.surface)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(BrandColors.border, lineWidth: 1)
        )
    }

    private func protocolMixTile(protocolMix: ProtocolMix) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Image(systemName: "chart.pie.fill")
                    .foregroundColor(BrandColors.neutral)
                    .font(.system(size: 20, weight: .semibold))
                Spacer()
            }

            VStack(alignment: .leading, spacing: 4) {
                protocolRow(name: "WebRTC", count: protocolMix.webrtc)
                protocolRow(name: "HLS", count: protocolMix.hls)
                protocolRow(name: "RTMP", count: protocolMix.rtmp)
            }

            Text("PROTOCOL MIX")
                .font(.system(size: 11, weight: .medium))
                .foregroundColor(BrandColors.textMuted)
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(BrandColors.surface)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(BrandColors.border, lineWidth: 1)
        )
    }

    private func protocolRow(name: String, count: Int) -> some View {
        HStack {
            Text(name)
                .font(.system(size: 12, weight: .medium))
                .foregroundColor(BrandColors.textSecondary)
            Spacer()
            Text(Formatters.abbreviatedCount(count))
                .font(.system(size: 12, weight: .semibold))
                .foregroundColor(BrandColors.textPrimary)
                .monospacedDigit()
        }
    }

    // MARK: - Streams List

    private var streamsList: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Active Streams")
                .font(.system(size: 22, weight: .semibold))
                .foregroundColor(BrandColors.textPrimary)

            if streams.isEmpty {
                emptyStreamsView
            } else {
                LazyVStack(spacing: 12) {
                    ForEach(streams, id: \.streamId) { stream in
                        NavigationLink(value: stream) {
                            streamRow(stream: stream)
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
        }
        .navigationDestination(for: LiveStream.self) { stream in
            PlayerView(stream: stream)
        }
    }

    private var emptyStreamsView: some View {
        VStack(spacing: 16) {
            Image(systemName: "video.slash")
                .font(.system(size: 48, weight: .light))
                .foregroundColor(BrandColors.textMuted)

            Text("No Active Streams")
                .font(.system(size: 16, weight: .semibold))
                .foregroundColor(BrandColors.textSecondary)

            Text("Streams will appear here when publishers go live.")
                .font(.system(size: 14, weight: .regular))
                .foregroundColor(BrandColors.textMuted)
                .multilineTextAlignment(.center)
        }
        .padding(.vertical, 48)
        .frame(maxWidth: .infinity)
        .background(BrandColors.surface)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(BrandColors.border, lineWidth: 1)
        )
    }

    private func streamRow(stream: LiveStream) -> some View {
        HStack(spacing: 16) {
            // Health indicator.
            healthIndicator(score: stream.healthScore)

            // Stream info.
            VStack(alignment: .leading, spacing: 4) {
                Text(stream.streamId)
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(BrandColors.textPrimary)
                    .lineLimit(1)

                HStack(spacing: 12) {
                    // App name.
                    Label(stream.app, systemImage: "app.fill")
                        .font(.system(size: 12, weight: .regular))
                        .foregroundColor(BrandColors.textSecondary)

                    // Viewer count.
                    Label(Formatters.abbreviatedCount(stream.viewers), systemImage: "person.2")
                        .font(.system(size: 12, weight: .regular))
                        .foregroundColor(BrandColors.textSecondary)

                    // Bitrate (if available).
                    if let bitrate = stream.bitrateKbps {
                        Label(Formatters.bitrateString(kbps: bitrate), systemImage: "speedometer")
                            .font(.system(size: 12, weight: .regular))
                            .foregroundColor(BrandColors.textSecondary)
                    }
                }
            }

            Spacer()

            // Chevron for navigation.
            Image(systemName: "chevron.right")
                .font(.system(size: 14, weight: .semibold))
                .foregroundColor(BrandColors.textMuted)
        }
        .padding(16)
        .background(BrandColors.surface)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(BrandColors.border, lineWidth: 1)
        )
    }

    private func healthIndicator(score: Double) -> some View {
        // Color based on health score thresholds.
        let color: Color = {
            if score >= 90 { return BrandColors.healthy }
            else if score >= 70 { return BrandColors.warning }
            else { return BrandColors.critical }
        }()

        return ZStack {
            Circle()
                .stroke(color.opacity(0.3), lineWidth: 3)
            Circle()
                .trim(from: 0, to: score / 100)
                .stroke(color, style: StrokeStyle(lineWidth: 3, lineCap: .round))
                .rotationEffect(.degrees(-90))
            Text(Formatters.healthScoreDisplay(score))
                .font(.system(size: 11, weight: .bold))
                .foregroundColor(color)
                .monospacedDigit()
        }
        .frame(width: 44, height: 44)
    }

    // MARK: - Error Banner

    private func errorBanner(error: PulseAPIError) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundColor(BrandColors.warning)

            VStack(alignment: .leading, spacing: 4) {
                Text("Connection Issue")
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(BrandColors.textPrimary)

                Text(error.errorDescription ?? "Unknown error")
                    .font(.system(size: 12, weight: .regular))
                    .foregroundColor(BrandColors.textSecondary)
            }

            Spacer()

            Button {
                self.error = nil
            } label: {
                Image(systemName: "xmark")
                    .foregroundColor(BrandColors.textMuted)
            }
        }
        .padding(16)
        .background(BrandColors.warning.opacity(0.1))
        .cornerRadius(12)
    }

    // MARK: - Data Loading

    private func refresh() async {
        guard let api = appState?.api else { return }

        isLoading = true

        do {
            // Fetch overview and streams in parallel.
            async let overviewTask = api.getLiveOverview()
            async let streamsTask = api.getLiveStreams()

            let (newOverview, newStreams) = try await (overviewTask, streamsTask)

            self.overview = newOverview
            self.streams = newStreams.items
            self.error = nil
        } catch let pulseError as PulseAPIError {
            self.error = pulseError
        } catch {
            self.error = .transport(description: error.localizedDescription)
        }

        isLoading = false
    }
}

// MARK: - Preview

#Preview {
    LiveView()
        .environment(\.appState, AppState())
}
