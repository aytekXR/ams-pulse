import Foundation

// MARK: - AccessibilityIdentifiers
//
// Centralized registry of accessibility identifiers for XCUITest automation
// and VoiceOver support.
//
// NAMING CONVENTION:
//   - Flat namespace: <screen>_<element>
//   - Snake_case with underscores (UITest assertions read better than camelCase)
//   - Stable across releases — changing an ID is a breaking change for any
//     external UI tests.
//
// These identifiers are applied via .accessibilityIdentifier() in the SwiftUI
// views. The tests duplicate these strings (UI test targets should not import
// the app module).

public enum AccessibilityID {

    // MARK: - Connect Screen

    public static let connect_serverURLField = "connect_server_url_field"
    public static let connect_tokenField = "connect_token_field"
    public static let connect_showTokenButton = "connect_show_token_button"
    public static let connect_connectButton = "connect_connect_button"
    public static let connect_errorBanner = "connect_error_banner"

    // MARK: - Tab Bar

    public static let tabBar = "main_tab_bar"
    public static let tabBar_live = "tab_live"
    public static let tabBar_alerts = "tab_alerts"
    public static let tabBar_settings = "tab_settings"

    // MARK: - Live Screen

    public static let live_viewersTile = "live_viewers_tile"
    public static let live_publishersTile = "live_publishers_tile"
    public static let live_streamsTile = "live_streams_tile"
    public static let live_protocolMixTile = "live_protocol_mix_tile"
    public static let live_streamsList = "live_streams_list"
    public static let live_emptyStreamsView = "live_empty_streams_view"
    /// Prefix for stream rows: live_stream_row_<streamId>
    public static func live_streamRow(id: String) -> String {
        "live_stream_row_\(id)"
    }

    // MARK: - Alerts Screen

    public static let alerts_list = "alerts_list"
    public static let alerts_emptyView = "alerts_empty_view"
    public static let alerts_loadingView = "alerts_loading_view"
    /// Prefix for alert rows: alerts_row_<alertId>
    public static func alerts_row(id: String) -> String {
        "alerts_row_\(id)"
    }

    // MARK: - Settings Screen

    public static let settings_serverRow = "settings_server_row"
    public static let settings_statusRow = "settings_status_row"
    public static let settings_versionRow = "settings_version_row"
    public static let settings_buildRow = "settings_build_row"
    public static let settings_signOutButton = "settings_sign_out_button"
    public static let settings_componentClickhouse = "settings_component_clickhouse"
    public static let settings_componentMetaStore = "settings_component_meta_store"
    public static let settings_componentCollector = "settings_component_collector"
    public static let settings_componentKafka = "settings_component_kafka"

    // MARK: - Player Screen

    public static let player_videoView = "player_video_view"
    public static let player_loadingView = "player_loading_view"
    public static let player_errorView = "player_error_view"
    public static let player_infoBar = "player_info_bar"
}
