package main

// meta_backend_test.go — table tests for resolveMetaBackend helper (D-072 WO-A wiring).
//
// resolveMetaBackend is a pure function that maps env vars to (backend, dsn)
// ready for meta.New. Tests run without a database or network.
//
// TDD red evidence: file written before resolveMetaBackend was defined; the
// package failed to compile with "undefined: resolveMetaBackend" until the
// helper was added to serve.go.

import (
	"testing"
)

func TestResolveMetaBackend(t *testing.T) {
	cases := []struct {
		name        string
		env         map[string]string
		wantBackend string
		wantDSN     string
	}{
		{
			name:        "defaults: no env set",
			env:         map[string]string{},
			wantBackend: "sqlite",
			wantDSN:     "pulse_meta.db",
		},
		{
			name:        "PULSE_META_DSN overrides default DSN",
			env:         map[string]string{"PULSE_META_DSN": "/data/meta.db"},
			wantBackend: "sqlite",
			wantDSN:     "/data/meta.db",
		},
		{
			name: "PULSE_META selects postgres with explicit DSN",
			env: map[string]string{
				"PULSE_META":     "postgres",
				"PULSE_META_DSN": "postgres://host/db",
			},
			wantBackend: "postgres",
			wantDSN:     "postgres://host/db",
		},
		{
			name: "PULSE_POSTGRES_DSN convenience: overrides backend and DSN",
			env: map[string]string{
				"PULSE_POSTGRES_DSN": "postgres://localhost/pulse_meta",
			},
			wantBackend: "postgres",
			wantDSN:     "postgres://localhost/pulse_meta",
		},
		{
			name: "PULSE_POSTGRES_DSN wins over PULSE_META_DSN",
			env: map[string]string{
				"PULSE_META_DSN":     "some.db",
				"PULSE_POSTGRES_DSN": "postgres://pg/pulse",
			},
			wantBackend: "postgres",
			wantDSN:     "postgres://pg/pulse",
		},
		{
			name: "PULSE_POSTGRES_DSN wins even when PULSE_META is explicitly sqlite",
			env: map[string]string{
				"PULSE_META":         "sqlite",
				"PULSE_POSTGRES_DSN": "postgres://pg/pulse",
			},
			wantBackend: "postgres",
			wantDSN:     "postgres://pg/pulse",
		},
		{
			name:        "PULSE_META empty falls back to sqlite default",
			env:         map[string]string{"PULSE_META": ""},
			wantBackend: "sqlite",
			wantDSN:     "pulse_meta.db",
		},
		{
			name:        "memory DSN preserved for sqlite",
			env:         map[string]string{"PULSE_META_DSN": ":memory:"},
			wantBackend: "sqlite",
			wantDSN:     ":memory:",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(key string) string { return tc.env[key] }
			backend, dsn := resolveMetaBackend(getenv)
			if backend != tc.wantBackend {
				t.Errorf("backend = %q, want %q", backend, tc.wantBackend)
			}
			if dsn != tc.wantDSN {
				t.Errorf("dsn = %q, want %q", dsn, tc.wantDSN)
			}
		})
	}
}
