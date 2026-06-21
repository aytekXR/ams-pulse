package main

import "testing"

// TestMaskDSN is the regression guard for D-030: maskDSN previously returned the
// DSN unchanged, leaking the ClickHouse password in plaintext to JSON logs on
// every migrate run and `pulse diag`. The password must be redacted; everything
// else (scheme, user, host, port, db, query) must survive.
func TestMaskDSN(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{
			"user+password redacted",
			"clickhouse://pulse:b6c23a46d4589f127915e42cd736869a@clickhouse:9000/pulse",
			"clickhouse://pulse:xxxxx@clickhouse:9000/pulse",
		},
		{
			"password with special chars redacted",
			"clickhouse://u:p%40ss%3Aword@host:9000/db",
			"clickhouse://u:xxxxx@host:9000/db",
		},
		{
			"user with no password is left as-is",
			"clickhouse://pulse@clickhouse:9000/pulse",
			"clickhouse://pulse@clickhouse:9000/pulse",
		},
		{
			"no userinfo is left as-is",
			"clickhouse://clickhouse:9000/pulse",
			"clickhouse://clickhouse:9000/pulse",
		},
		{
			"unparseable string returned unchanged (no panic)",
			"not-a-dsn",
			"not-a-dsn",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := maskDSN(tc.in)
			if got != tc.want {
				t.Errorf("maskDSN(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// Strongest invariant: the raw password must never appear in the output.
			if tc.name == "user+password redacted" && got == tc.in {
				t.Errorf("maskDSN did not redact the password: %q", got)
			}
		})
	}
}
