// Package reports — logo resolution for PDF report branding (S11 WO-A).
package reports

import (
	_ "embed"
	"log/slog"
	"os"
)

//go:embed assets/default_logo.png
var defaultLogoBytes []byte

// ResolveLogo returns logo bytes for PDF embedding.
// If logoPath is empty or the file is unreadable, returns the embedded default.
// The file is read at PDF-generation time (no caching — operator can replace it
// between report runs without restarting the server).
func ResolveLogo(logoPath string) []byte {
	if logoPath == "" {
		return defaultLogoBytes
	}
	data, err := os.ReadFile(logoPath)
	if err != nil {
		return defaultLogoBytes
	}
	return data
}

// ValidateLogoPath probes logoPath at boot time.
// If the path is set but unreadable, or set but not a PNG/JPEG (detected by
// magic bytes), a WARN is logged. Never panics.
// Returns the resolved logo bytes (embedded default on any failure).
// Call from serve.go's newServer() as a non-fatal boot check.
func ValidateLogoPath(path string, logger *slog.Logger) []byte {
	if path == "" {
		return defaultLogoBytes
	}
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Warn("reports: PULSE_REPORT_LOGO_PATH set but file is not readable, using embedded default",
			"path", path, "error", err)
		return defaultLogoBytes
	}
	// Warn if the format is not PNG or JPEG (magic bytes check).
	if !isPNG(data) && !isJPEG(data) {
		logger.Warn("reports: PULSE_REPORT_LOGO_PATH is not a PNG or JPEG file, using embedded default",
			"path", path)
		return defaultLogoBytes
	}
	return data
}

// isPNG returns true if data starts with the PNG magic bytes.
func isPNG(data []byte) bool {
	return len(data) >= 4 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G'
}

// isJPEG returns true if data starts with the JPEG magic bytes.
func isJPEG(data []byte) bool {
	return len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8
}
