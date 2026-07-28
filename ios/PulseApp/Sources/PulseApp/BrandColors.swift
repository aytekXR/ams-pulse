import SwiftUI

// MARK: - Brand Colors
//
// Source of truth: brandkit/design-system/tokens.json
// These values are authoritative; do not invent colors.
//
// The design system uses semantic color names that adapt to light/dark mode.
// SwiftUI's Color does not support automatic scheme adaptation via hex, so we
// use UIColor.init(dynamicProvider:) on iOS and NSColor on macOS (wrapped in
// Color.init for SwiftUI).

/// Brand color palette from tokens.json.
///
/// All colors adapt automatically to light/dark mode. Use these exclusively
/// for the Pulse brand — do not create ad-hoc colors.
public enum BrandColors {

    // MARK: - Dark Mode Hex Values
    // From tokens.json > color > dark

    private static let darkBg = "#0A0E14"
    private static let darkSurface = "#10161D"
    private static let darkRaised = "#161E27"
    private static let darkBorder = "#1E2833"
    private static let darkBorderStrong = "#2B3947"
    private static let darkTextPrimary = "#E8EEF4"
    private static let darkTextSecondary = "#9FB0C0"
    private static let darkTextMuted = "#5C6F80"
    private static let darkSignal = "#2CE5A7"
    private static let darkSignalHover = "#4FEDB9"
    private static let darkOnSignal = "#0A0E14"
    private static let darkHealthy = "#2CE5A7"
    private static let darkWarning = "#FFB224"
    private static let darkCritical = "#FF5C68"
    private static let darkNeutral = "#8296A8"
    private static let darkInfo = "#58A6FF"

    // MARK: - Light Mode Hex Values
    // From tokens.json > color > light

    private static let lightBg = "#F7F9FA"
    private static let lightSurface = "#FFFFFF"
    private static let lightRaised = "#F0F4F7"
    private static let lightBorder = "#D7DEE5"
    private static let lightBorderStrong = "#C7D1DA"
    private static let lightTextPrimary = "#10181F"
    private static let lightTextSecondary = "#4A5B6B"
    private static let lightTextMuted = "#6B7B88"
    private static let lightSignal = "#087A59"
    private static let lightSignalHover = "#07684C"
    private static let lightOnSignal = "#FFFFFF"
    private static let lightHealthy = "#0BA678"
    private static let lightWarning = "#B45309"
    private static let lightCritical = "#DC2626"
    private static let lightNeutral = "#64748B"
    private static let lightInfo = "#1B5EAD"

    // MARK: - Semantic Colors

    /// Main background color (deepest layer).
    public static var bg: Color {
        adaptiveColor(light: lightBg, dark: darkBg)
    }

    /// Surface color for cards and containers.
    public static var surface: Color {
        adaptiveColor(light: lightSurface, dark: darkSurface)
    }

    /// Raised surface for emphasis.
    public static var raised: Color {
        adaptiveColor(light: lightRaised, dark: darkRaised)
    }

    /// Border color (standard).
    public static var border: Color {
        adaptiveColor(light: lightBorder, dark: darkBorder)
    }

    /// Border color (stronger emphasis).
    public static var borderStrong: Color {
        adaptiveColor(light: lightBorderStrong, dark: darkBorderStrong)
    }

    /// Primary text color.
    public static var textPrimary: Color {
        adaptiveColor(light: lightTextPrimary, dark: darkTextPrimary)
    }

    /// Secondary text color.
    public static var textSecondary: Color {
        adaptiveColor(light: lightTextSecondary, dark: darkTextSecondary)
    }

    /// Muted text color.
    public static var textMuted: Color {
        adaptiveColor(light: lightTextMuted, dark: darkTextMuted)
    }

    /// Primary brand signal color (the teal/green accent).
    public static var signal: Color {
        adaptiveColor(light: lightSignal, dark: darkSignal)
    }

    /// Signal color hover state.
    public static var signalHover: Color {
        adaptiveColor(light: lightSignalHover, dark: darkSignalHover)
    }

    /// Text on signal-colored backgrounds.
    public static var onSignal: Color {
        adaptiveColor(light: lightOnSignal, dark: darkOnSignal)
    }

    /// Healthy/success status.
    public static var healthy: Color {
        adaptiveColor(light: lightHealthy, dark: darkHealthy)
    }

    /// Warning status.
    public static var warning: Color {
        adaptiveColor(light: lightWarning, dark: darkWarning)
    }

    /// Critical/error status.
    public static var critical: Color {
        adaptiveColor(light: lightCritical, dark: darkCritical)
    }

    /// Neutral status.
    public static var neutral: Color {
        adaptiveColor(light: lightNeutral, dark: darkNeutral)
    }

    /// Informational status.
    public static var info: Color {
        adaptiveColor(light: lightInfo, dark: darkInfo)
    }

    // MARK: - Dataviz Palette
    // tokens.json > color > dataviz — for charts and graphs.

    /// Data visualization palette for charts (8 colors).
    public static let dataviz: [Color] = [
        Color(hex: "#2CE5A7"),  // Teal (primary)
        Color(hex: "#58A6FF"),  // Blue
        Color(hex: "#A78BFA"),  // Purple
        Color(hex: "#F06BB2"),  // Pink
        Color(hex: "#FFB224"),  // Amber
        Color(hex: "#38D6E0"),  // Cyan
        Color(hex: "#C4B26A"),  // Olive
        Color(hex: "#7C93AD")   // Slate
    ]

    // MARK: - Helper

    /// Creates a Color that adapts to light/dark mode.
    private static func adaptiveColor(light: String, dark: String) -> Color {
        #if os(iOS)
        return Color(UIColor { traitCollection in
            switch traitCollection.userInterfaceStyle {
            case .dark:
                return UIColor(hex: dark)
            default:
                return UIColor(hex: light)
            }
        })
        #else
        // macOS fallback — use appearance name.
        return Color(NSColor(name: nil) { appearance in
            if appearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua {
                return NSColor(hex: dark)
            } else {
                return NSColor(hex: light)
            }
        })
        #endif
    }
}

// MARK: - Hex Color Extensions

extension Color {
    /// Creates a Color from a hex string (e.g., "#2CE5A7" or "2CE5A7").
    init(hex: String) {
        let hex = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
        var int: UInt64 = 0
        Scanner(string: hex).scanHexInt64(&int)
        let r, g, b: Double
        switch hex.count {
        case 6: // RGB (24-bit)
            r = Double((int >> 16) & 0xFF) / 255.0
            g = Double((int >> 8) & 0xFF) / 255.0
            b = Double(int & 0xFF) / 255.0
        default:
            r = 0
            g = 0
            b = 0
        }
        self.init(red: r, green: g, blue: b)
    }
}

#if os(iOS)
extension UIColor {
    /// Creates a UIColor from a hex string.
    convenience init(hex: String) {
        let hex = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
        var int: UInt64 = 0
        Scanner(string: hex).scanHexInt64(&int)
        let r, g, b: CGFloat
        switch hex.count {
        case 6:
            r = CGFloat((int >> 16) & 0xFF) / 255.0
            g = CGFloat((int >> 8) & 0xFF) / 255.0
            b = CGFloat(int & 0xFF) / 255.0
        default:
            r = 0
            g = 0
            b = 0
        }
        self.init(red: r, green: g, blue: b, alpha: 1.0)
    }
}
#else
extension NSColor {
    /// Creates an NSColor from a hex string.
    convenience init(hex: String) {
        let hex = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
        var int: UInt64 = 0
        Scanner(string: hex).scanHexInt64(&int)
        let r, g, b: CGFloat
        switch hex.count {
        case 6:
            r = CGFloat((int >> 16) & 0xFF) / 255.0
            g = CGFloat((int >> 8) & 0xFF) / 255.0
            b = CGFloat(int & 0xFF) / 255.0
        default:
            r = 0
            g = 0
            b = 0
        }
        self.init(red: r, green: g, blue: b, alpha: 1.0)
    }
}
#endif
