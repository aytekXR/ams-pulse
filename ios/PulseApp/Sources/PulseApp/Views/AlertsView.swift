import SwiftUI
import PulseKit

// MARK: - Alerts View
//
// Displays recent alert history from the Pulse server.
//
// Alerts are styled by severity (info, warning, critical) using brand colors.
// Each alert shows:
//   - Severity indicator and icon
//   - Metric name and breach details (value vs threshold)
//   - Firing timestamp as relative time
//   - State (firing, resolved, delivery_failure)
//   - Scope context (node, app, stream, tenant)
//
// The list supports pull-to-refresh but does not auto-refresh like LiveView,
// since alerts are less time-sensitive and the user can refresh manually.

struct AlertsView: View {

    // MARK: - Environment

    @Environment(\.appState) private var appState

    // MARK: - State

    @State private var alerts: [AlertHistoryEntry] = []
    @State private var isLoading: Bool = false
    @State private var error: PulseAPIError?

    // MARK: - Body

    var body: some View {
        NavigationStack {
            Group {
                if isLoading && alerts.isEmpty {
                    loadingView
                } else if alerts.isEmpty {
                    emptyView
                } else {
                    alertsList
                }
            }
            .background(BrandColors.bg.ignoresSafeArea())
            .navigationTitle("Alerts")
            .navigationBarTitleDisplayMode(.large)
            .refreshable {
                await refresh()
            }
            .toolbar {
                ToolbarItem(placement: .navigationBarTrailing) {
                    if isLoading && !alerts.isEmpty {
                        ProgressView()
                    }
                }
            }
        }
        .task {
            await refresh()
        }
    }

    // MARK: - Subviews

    private var loadingView: some View {
        VStack(spacing: 16) {
            ProgressView()
                .scaleEffect(1.2)

            Text("Loading alerts...")
                .font(.system(size: 14, weight: .medium))
                .foregroundColor(BrandColors.textSecondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var emptyView: some View {
        VStack(spacing: 16) {
            Image(systemName: "bell.slash")
                .font(.system(size: 48, weight: .light))
                .foregroundColor(BrandColors.textMuted)

            Text("No Recent Alerts")
                .font(.system(size: 18, weight: .semibold))
                .foregroundColor(BrandColors.textSecondary)

            Text("Alert history will appear here when rules fire.")
                .font(.system(size: 14, weight: .regular))
                .foregroundColor(BrandColors.textMuted)
                .multilineTextAlignment(.center)
                .padding(.horizontal, 32)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var alertsList: some View {
        ScrollView {
            LazyVStack(spacing: 12) {
                // Error banner (if any).
                if let error = error {
                    errorBanner(error: error)
                }

                // Alert rows.
                ForEach(alerts, id: \.id) { alert in
                    alertRow(alert: alert)
                }
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 16)
        }
    }

    private func alertRow(alert: AlertHistoryEntry) -> some View {
        HStack(alignment: .top, spacing: 16) {
            // Severity indicator.
            severityIndicator(severity: alert.severity)

            // Content.
            VStack(alignment: .leading, spacing: 8) {
                // Header: metric and timestamp.
                HStack {
                    Text(alert.metric)
                        .font(.system(size: 14, weight: .semibold))
                        .foregroundColor(BrandColors.textPrimary)

                    Spacer()

                    Text(Formatters.relativeTime(fromEpochMs: alert.ts, to: Date()))
                        .font(.system(size: 12, weight: .regular))
                        .foregroundColor(BrandColors.textMuted)
                }

                // Breach details.
                HStack(spacing: 4) {
                    Text(formatValue(alert.value))
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(severityColor(alert.severity))

                    Text("breached threshold")
                        .font(.system(size: 12, weight: .regular))
                        .foregroundColor(BrandColors.textSecondary)

                    Text(formatValue(alert.threshold))
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(BrandColors.textSecondary)
                }

                // State badge.
                HStack(spacing: 8) {
                    stateBadge(state: alert.state)

                    // Scope context (if any).
                    if let scope = alert.scope {
                        scopeLabel(scope: scope)
                    }
                }
            }
        }
        .padding(16)
        .background(BrandColors.surface)
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(severityBorderColor(alert.severity), lineWidth: 1)
        )
    }

    private func severityIndicator(severity: AlertSeverity) -> some View {
        let icon: String
        let color: Color

        switch severity {
        case .critical:
            icon = "exclamationmark.triangle.fill"
            color = BrandColors.critical
        case .warning:
            icon = "exclamationmark.circle.fill"
            color = BrandColors.warning
        case .info:
            icon = "info.circle.fill"
            color = BrandColors.info
        case .unknown:
            // A severity this build does not know. A circle is the shape used
            // nowhere else here, so an operator can tell it apart at a glance —
            // status is always shape AND colour in this product, never colour
            // alone (brandkit design-rationale section 2, the CVD rule).
            icon = "questionmark.circle.fill"
            color = BrandColors.neutral
        }

        return Image(systemName: icon)
            .font(.system(size: 24, weight: .regular))
            .foregroundColor(color)
            .frame(width: 32, height: 32)
    }

    private func severityColor(_ severity: AlertSeverity) -> Color {
        switch severity {
        case .critical: return BrandColors.critical
        case .warning: return BrandColors.warning
        case .info: return BrandColors.info
        case .unknown: return BrandColors.neutral
        }
    }

    private func severityBorderColor(_ severity: AlertSeverity) -> Color {
        switch severity {
        case .critical: return BrandColors.critical.opacity(0.3)
        case .warning: return BrandColors.warning.opacity(0.3)
        case .info: return BrandColors.border
        case .unknown: return BrandColors.border
        }
    }

    private func stateBadge(state: AlertState) -> some View {
        let (text, bgColor, fgColor): (String, Color, Color) = {
            switch state {
            case .firing:
                return ("Firing", BrandColors.critical.opacity(0.15), BrandColors.critical)
            case .resolved:
                return ("Resolved", BrandColors.healthy.opacity(0.15), BrandColors.healthy)
            case .deliveryFailure:
                return ("Delivery Failed", BrandColors.warning.opacity(0.15), BrandColors.warning)
            case .unknown(let raw):
                // Show the server's own word. An alert in a state we cannot
                // interpret is exactly when hiding the raw value costs most.
                return (raw.uppercased(), BrandColors.neutral.opacity(0.15), BrandColors.neutral)
            }
        }()

        return Text(text)
            .font(.system(size: 11, weight: .medium))
            .foregroundColor(fgColor)
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .background(bgColor)
            .cornerRadius(4)
    }

    private func scopeLabel(scope: AlertScope) -> some View {
        // Show the most specific scope level.
        let text: String? = {
            if let streamId = scope.streamId {
                return "stream: \(streamId)"
            } else if let app = scope.app {
                return "app: \(app)"
            } else if let nodeId = scope.nodeId {
                return "node: \(nodeId)"
            } else if let tenant = scope.tenant {
                return "tenant: \(tenant)"
            }
            return nil
        }()

        return Group {
            if let text = text {
                Text(text)
                    .font(.system(size: 11, weight: .medium))
                    .foregroundColor(BrandColors.textMuted)
                    .padding(.horizontal, 8)
                    .padding(.vertical, 4)
                    .background(BrandColors.raised)
                    .cornerRadius(4)
            }
        }
    }

    private func formatValue(_ value: Double) -> String {
        // Format metric value for display.
        // Use percentageRaw if it looks like a percentage (0-100).
        if value >= 0 && value <= 100 {
            return Formatters.percentageRaw(value)
        } else {
            // General number formatting.
            if value == value.rounded() {
                return "\(Int(value))"
            } else {
                return String(format: "%.2f", value)
            }
        }
    }

    private func errorBanner(error: PulseAPIError) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundColor(BrandColors.warning)

            VStack(alignment: .leading, spacing: 4) {
                Text("Failed to load alerts")
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
            let result = try await api.getAlertHistory()
            self.alerts = result.items
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
    AlertsView()
        .environment(\.appState, AppState())
}
