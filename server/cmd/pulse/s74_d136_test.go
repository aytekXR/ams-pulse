package main

// s74_d136_test.go — D-136 (S74) tests for the S73-audit config-startup cluster:
//   [2] server.Stop() must drain the HTTP API server (call apiServer.Stop()).
//   [3] envBool accepts the "1" / "True" idioms, not just exact "true".
//   [6] the AMS base URL is credential-redacted before it is printed.

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// ─── [2] graceful shutdown drains the API server ────────────────────────────────

type fakeAPILifecycle struct {
	startCalled bool
	stopCalled  bool
}

func (f *fakeAPILifecycle) Start(context.Context) error { f.startCalled = true; return nil }
func (f *fakeAPILifecycle) Stop()                       { f.stopCalled = true }

func TestServerStop_DrainsAPIServer(t *testing.T) {
	fake := &fakeAPILifecycle{}
	// Every other dependency is nil; Stop() is nil-safe, so this exercises the
	// apiServer drain in isolation.
	s := &server{apiServer: fake, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

	s.Stop()

	if !fake.stopCalled {
		t.Error("server.Stop() must call apiServer.Stop() to drain the HTTP server + stop its goroutines on SIGTERM (S73/D-136 [2])")
	}
}

// ─── [3] envBool truthy idioms ──────────────────────────────────────────────────

func TestEnvBool(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"1", true}, {"true", true}, {"True", true}, {"TRUE", true}, {"tRuE", true},
		// Whitespace-padded truthy values (k8s secret --from-file trailing newline;
		// Docker --env-file trailing space) must still read as true (S73/D-136 review).
		{"true ", true}, {" true", true}, {"true\n", true}, {"1 ", true}, {"1\n", true}, {"\t1\t", true},
		{"0", false}, {"false", false}, {"", false}, {"yes", false}, {"2", false}, {"on", false}, {"  ", false},
	}
	const key = "PULSE_TEST_ENVBOOL"
	for _, c := range cases {
		t.Setenv(key, c.val)
		if got := envBool(key); got != c.want {
			t.Errorf("envBool(%q) = %v, want %v", c.val, got, c.want)
		}
	}
}

// ─── [6] AMS URL credential redaction ───────────────────────────────────────────

func TestRedactURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"http://admin:s3cr3t@ams.internal:5080", "http://admin:xxxxx@ams.internal:5080"},
		{"https://ams.internal:5443", "https://ams.internal:5443"},
		{"", ""},
	}
	for _, c := range cases {
		if got := redactURL(c.in); got != c.want {
			t.Errorf("redactURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if out := redactURL("http://u:topsecret@h:1/x"); strings.Contains(out, "topsecret") {
		t.Errorf("redactURL leaked the password: %q", out)
	}
}

// TestPrintDiagSummary_RedactsAMSURL pins the OTHER [6] call site (main.go's diag
// config summary) via the io.Writer seam, so a regression that prints the raw AMS URL
// there is caught (the checkAMS test alone would miss it — S73/D-136 review).
func TestPrintDiagSummary_RedactsAMSURL(t *testing.T) {
	var buf bytes.Buffer
	printDiagSummary(&buf, EnvConfig{AMSBaseURL: "http://admin:s3cr3t@ams.internal:5080"})
	out := buf.String()
	if strings.Contains(out, "s3cr3t") {
		t.Errorf("printDiagSummary leaked the AMS password: %q", out)
	}
	if !strings.Contains(out, "xxxxx") {
		t.Errorf("printDiagSummary did not redact the AMS URL: %q", out)
	}
}

// TestCheckAMS_RedactsCredentials pins the [6] call site (not just the helper): the
// diag output for an AMS URL with embedded credentials must not contain the password.
// Uses the existing captureStdout helper (cmd_test.go).
func TestCheckAMS_RedactsCredentials(t *testing.T) {
	readBuf, closePipe := captureStdout(t)
	checkAMS("http://admin:s3cr3t@ams.internal:5080")
	closePipe()
	out := readBuf()
	if strings.Contains(out, "s3cr3t") {
		t.Errorf("checkAMS leaked the AMS password to stdout: %q", out)
	}
	if !strings.Contains(out, "xxxxx") {
		t.Errorf("checkAMS output was not redacted: %q", out)
	}
}
