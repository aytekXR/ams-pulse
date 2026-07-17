// s81_d143_test.go — S81 (D-143): PULSE_REPORT_ARTIFACT_RETENTION_DAYS env
// wiring for report-artifact retention pruning. Guards the env-var NAME and the
// default so a typo (which would silently disable operator control) is caught.
package main

import "testing"

func TestReportArtifactRetentionDaysConfig(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want int
	}{
		{"default when unset", "", 90},
		{"operator override", "30", 30},
		{"zero disables", "0", 0},
		{"negative disables", "-1", -1},
		// Whitespace-padded values (k8s --from-file trailing newline / Docker
		// --env-file trailing space) must not silently fall back to the default.
		{"trailing newline zero", "0\n", 0},
		{"padded override", "  30  ", 30},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("PULSE_REPORT_ARTIFACT_RETENTION_DAYS", c.val)
			cfg, err := loadEnvConfig()
			if err != nil {
				t.Fatalf("loadEnvConfig() error: %v", err)
			}
			if cfg.ReportArtifactRetentionDays != c.want {
				t.Fatalf("ReportArtifactRetentionDays = %d, want %d", cfg.ReportArtifactRetentionDays, c.want)
			}
		})
	}
}
