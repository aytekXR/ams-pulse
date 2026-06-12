// export_test.go exposes internal functions for testing.
package logtail

import (
	"log/slog"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// NewForTest creates a Tailer with the given domain.EventSink for white-box testing.
func NewForTest(cfg Config, sink domain.EventSink, logger *slog.Logger) *Tailer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Tailer{cfg: cfg, sink: sink, logger: logger}
}

// ProcessLineForTest exposes processLine for direct testing.
func (t *Tailer) ProcessLineForTest(line []byte) {
	t.processLine(line)
}
