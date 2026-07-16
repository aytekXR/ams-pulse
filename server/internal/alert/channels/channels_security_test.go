package channels

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeSMTPServer starts a minimal one-shot SMTP server on 127.0.0.1 that replies
// starttlsReply to STARTTLS and 250 to the rest of the happy path (MAIL/RCPT/DATA/
// QUIT). It returns the listen address. This lets us assert that a STARTTLS failure
// aborts Send (fail-closed) rather than silently continuing on the plaintext socket:
// if the fix is reverted, Send completes the plaintext transaction and returns nil.
func fakeSMTPServer(t *testing.T, starttlsReply string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		fmt.Fprint(conn, "220 fake ESMTP\r\n")
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				fmt.Fprint(conn, "250-fake\r\n250 STARTTLS\r\n")
			case strings.HasPrefix(cmd, "STARTTLS"):
				fmt.Fprintf(conn, "%s\r\n", starttlsReply)
			case strings.HasPrefix(cmd, "DATA"):
				fmt.Fprint(conn, "354 end with .\r\n")
				for {
					l, err := br.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
				}
				fmt.Fprint(conn, "250 ok\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				fmt.Fprint(conn, "221 bye\r\n")
				return
			default: // MAIL, RCPT, ...
				fmt.Fprint(conn, "250 ok\r\n")
			}
		}
	}()
	return ln.Addr().String()
}

// [1] HIGH — STARTTLS failure must fail closed, not fall through to plaintext.
func TestEmailSend_STARTTLSFailure_FailsClosed(t *testing.T) {
	addr := fakeSMTPServer(t, "454 4.7.0 TLS not available")
	ch := NewEmailChannel(EmailConfig{SMTPAddr: addr, From: "from@x", To: "to@y", STARTTLS: true})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := ch.Send(ctx, []byte(`{"title":"t"}`))
	if err == nil {
		t.Fatal("STARTTLS failure must abort Send (fail-closed); got nil — plaintext fallback occurred")
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("expected a STARTTLS error, got: %v", err)
	}
}

// [7] MEDIUM — a publisher-controlled stream_id in the title must not inject SMTP
// headers via CRLF in the Subject.
func TestBuildEmailMessage_SanitizesSubjectCRLF(t *testing.T) {
	n := map[string]any{
		"title": "stream evil\r\nBcc: attacker@example.com",
		"test":  false,
	}
	msg := buildEmailMessage(EmailConfig{From: "from@x", To: "to@y"}, n)
	// The security concern is header injection: a CRLF in the Subject must not spawn
	// a new header. Scope the assertion to the header section (before the blank line);
	// a CRLF appearing later in the rendered body is not header injection.
	sep := strings.Index(msg, "\r\n\r\n")
	if sep < 0 {
		t.Fatalf("no header/body separator in message:\n%q", msg)
	}
	headers := msg[:sep]
	if strings.Contains(headers, "\r\nBcc:") {
		t.Fatalf("CRLF header injection not prevented in Subject header:\n%q", headers)
	}
	// Positive control: the flattened title text is still present in the Subject line.
	if !strings.Contains(headers, "Bcc: attacker@example.com") {
		t.Fatalf("expected flattened title text in Subject header; got:\n%q", headers)
	}
}

// [2] HIGH — the bot token must never appear in a returned (and thus logged) error.
func TestTelegramSend_RedactsTokenInError(t *testing.T) {
	const token = "123456:SECRET-BOT-TOKEN-abcdef"
	// Port 1 → connection refused → client.Do returns a *url.Error embedding the URL.
	ch := NewTelegramChannelWithURL(
		TelegramConfig{BotToken: token, ChatID: "1"},
		"http://127.0.0.1:1/bot%s/sendMessage",
	)
	err := ch.Send(context.Background(), []byte(`{"title":"t"}`))
	if err == nil {
		t.Fatal("expected a send error to the closed port")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("bot token leaked into error: %v", err)
	}
	if !strings.Contains(err.Error(), "REDACTED") {
		t.Fatalf("expected REDACTED marker in error, got: %v", err)
	}
}

// [8] LOW (defense-in-depth) — dashboard_url must be attribute-escaped so it cannot
// break out of the href="…" attribute under Telegram HTML parse mode.
func TestBuildTelegramMessage_EscapesDashboardURLAttr(t *testing.T) {
	n := map[string]any{
		"title":         "t",
		"dashboard_url": `https://x/"><script>alert(1)</script>`,
	}
	msg := buildTelegramMessage(n)
	if strings.Contains(msg, `"><script>`) {
		t.Fatalf("dashboard_url not attribute-escaped — HTML injection possible:\n%s", msg)
	}
	if !strings.Contains(msg, "&quot;") {
		t.Fatalf("expected escaped quote (&quot;) in href, got:\n%s", msg)
	}
}
