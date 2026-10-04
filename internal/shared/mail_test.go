package shared

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// smtpStub is a minimal single-connection SMTP server for tests.
type smtpStub struct {
	t         *testing.T
	ln        net.Listener
	tlsConfig *tls.Config // non-nil: advertise STARTTLS (and AUTH after TLS)
	auth      bool        // advertise AUTH PLAIN
	stallData bool        // never answer DATA
	heloOnly  bool        // reject EHLO; answer HELO without extensions
	quit      string      // "": reply 221; "drop": close without replying; "stall": never reply

	mu       sync.Mutex
	commands []string
	data     string
	done     chan struct{}
}

func startSMTPStub(t *testing.T, stub *smtpStub) *smtpStub {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	stub.t = t
	stub.ln = ln
	stub.done = make(chan struct{})
	t.Cleanup(func() { _ = ln.Close() })
	go stub.serve()
	return stub
}

func (s *smtpStub) port() int { return s.ln.Addr().(*net.TCPAddr).Port }

func (s *smtpStub) record(cmd string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commands = append(s.commands, cmd)
}

func (s *smtpStub) transcript() ([]string, string) {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...), s.data
}

func (s *smtpStub) serve() {
	defer close(s.done)
	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	r := bufio.NewReader(conn)
	w := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	secure := false
	w("220 stub ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		s.record(line)
		switch verb {
		case "EHLO", "HELO":
			if s.heloOnly {
				if verb == "EHLO" {
					w("502 command not implemented")
				} else {
					w("250 stub")
				}
				continue
			}
			exts := []string{"stub"}
			if s.tlsConfig != nil && !secure {
				exts = append(exts, "STARTTLS")
			}
			if s.auth && (secure || s.tlsConfig == nil) {
				exts = append(exts, "AUTH PLAIN")
			}
			for i, ext := range exts {
				sep := "-"
				if i == len(exts)-1 {
					sep = " "
				}
				w("250" + sep + ext)
			}
		case "STARTTLS":
			w("220 ready")
			tlsConn := tls.Server(conn, s.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			r = bufio.NewReader(conn)
			secure = true
		case "AUTH":
			w("235 authenticated")
		case "MAIL", "RCPT":
			w("250 ok")
		case "DATA":
			if s.stallData {
				// Never answer; the client must give up on its own deadline.
				_, _ = r.ReadString('\n')
				time.Sleep(5 * time.Second)
				return
			}
			w("354 go ahead")
			var data strings.Builder
			for {
				dl, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if dl == ".\r\n" {
					break
				}
				data.WriteString(dl)
			}
			s.mu.Lock()
			s.data = data.String()
			s.mu.Unlock()
			w("250 queued")
		case "QUIT":
			switch s.quit {
			case "drop":
			case "stall":
				// Never answer; the client gives up on its own deadline and
				// closes the connection.
				_, _ = r.ReadString('\n')
			default:
				w("221 bye")
			}
			return
		default:
			w("502 unknown")
		}
	}
}

func selfSignedTLS(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true,

		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}, pool
}

func verbs(commands []string) []string {
	out := make([]string, 0, len(commands))
	for _, c := range commands {
		out = append(out, strings.ToUpper(strings.SplitN(c, " ", 2)[0]))
	}
	return out
}

func TestSendEmailSTARTTLSAndAuthOrder(t *testing.T) {
	serverTLS, roots := selfSignedTLS(t)
	stub := startSMTPStub(t, &smtpStub{tlsConfig: serverTLS, auth: true})
	client := NewMailClient(MailConfig{Host: "127.0.0.1", Port: stub.port(), From: "payroll@example.com", Username: "user", Password: "secret"})
	client.rootCAs = roots

	err := client.SendEmail(context.Background(), "ayu@example.com", "Payslip 2026-07", "<p>hi</p>", &Attachment{Filename: "payslip.pdf", ContentType: "application/pdf", Data: []byte("%PDF-test")})
	require.NoError(t, err)

	commands, data := stub.transcript()
	require.Equal(t, []string{"EHLO", "STARTTLS", "EHLO", "AUTH", "MAIL", "RCPT", "DATA", "QUIT"}, verbs(commands))
	require.Equal(t, "EHLO localhost", commands[0])
	require.Equal(t, "MAIL FROM:<payroll@example.com>", commands[4])
	require.Equal(t, "RCPT TO:<ayu@example.com>", commands[5])
	require.Contains(t, data, "Content-Disposition: attachment; filename=\"payslip.pdf\"\r\n")
}

func TestSendEmailRefusesUnauthenticatedWhenAuthNotOffered(t *testing.T) {
	stub := startSMTPStub(t, &smtpStub{})
	client := NewMailClient(MailConfig{Host: "127.0.0.1", Port: stub.port(), From: "payroll@example.com", Username: "user", Password: "secret"})
	err := client.SendEmail(context.Background(), "ayu@example.com", "s", "b", nil)
	require.ErrorContains(t, err, "smtp: server doesn't support AUTH")
}

// TestSendEmailPlainPathMatchesNetSMTP proves the plain localhost path (no TLS,
// no AUTH) produces the same SMTP conversation and the same message bytes as
// the rc.8 implementation, which called net/smtp.SendMail.
func TestSendEmailPlainPathMatchesNetSMTP(t *testing.T) {
	cases := []struct {
		name       string
		attachment *Attachment
	}{
		{name: "html body"},
		{name: "pdf attachment", attachment: &Attachment{Filename: "payslip-2026-07.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1.4 test payload with enough bytes to wrap the base64 line length of seventy-six characters")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := startSMTPStub(t, &smtpStub{})
			client := NewMailClient(MailConfig{Host: "127.0.0.1", Port: stub.port(), From: "payroll@example.com"})
			require.NoError(t, client.SendEmail(context.Background(), "ayu@example.com", "Payslip 2026-07", "<p>Your payslip is attached.</p>", tc.attachment))
			gotCommands, gotData := stub.transcript()

			if tc.attachment == nil {
				want := "From: payroll@example.com\r\nTo: ayu@example.com\r\nSubject: Payslip 2026-07\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=\"UTF-8\"\r\n\r\n<p>Your payslip is attached.</p>\r\n"
				require.Equal(t, want, gotData)
			}

			// Replay the received message through net/smtp.SendMail (rc.8 path).
			ref := startSMTPStub(t, &smtpStub{})
			msg := strings.TrimSuffix(gotData, "\r\n")
			if tc.attachment != nil {
				msg = gotData
			}
			require.NoError(t, smtp.SendMail("127.0.0.1:"+strconv.Itoa(ref.port()), nil, "payroll@example.com", []string{"ayu@example.com"}, []byte(msg)))
			wantCommands, wantData := ref.transcript()
			require.Equal(t, wantCommands, gotCommands)
			require.Equal(t, wantData, gotData)
		})
	}
}

func TestSendEmailDialDeadline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())

	client := NewMailClient(MailConfig{Host: "127.0.0.1", Port: port, From: "a@example.com", DialTimeout: 500 * time.Millisecond})
	start := time.Now()
	err = client.SendEmail(context.Background(), "b@example.com", "s", "b", nil)
	require.Error(t, err)
	require.Less(t, time.Since(start), 3*time.Second)
}

func TestSendEmailIODeadlineWhenDataNeverAnswered(t *testing.T) {
	stub := startSMTPStub(t, &smtpStub{stallData: true})
	client := NewMailClient(MailConfig{Host: "127.0.0.1", Port: stub.port(), From: "a@example.com", IOTimeout: 300 * time.Millisecond})
	start := time.Now()
	err := client.SendEmail(context.Background(), "b@example.com", "s", "b", nil)
	require.Error(t, err)
	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	require.True(t, netErr.Timeout())
	require.Less(t, time.Since(start), 3*time.Second)
}

func TestMailClientDefaultDeadlines(t *testing.T) {
	require.Equal(t, 10*time.Second, DefaultMailDialTimeout)
	require.Equal(t, 60*time.Second, DefaultMailIOTimeout)
}

// captureSlog replaces the default logger for the test and returns its output.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// Once the server accepted DATA the message is delivered; a QUIT that fails or
// times out afterwards must not be reported as a failed send, or the payslip
// task would retry and deliver a duplicate.
func TestSendEmailQuitFailureAfterAcceptedDataIsSuccess(t *testing.T) {
	cases := []struct {
		name      string
		quit      string
		ioTimeout time.Duration
	}{
		{name: "connection dropped on QUIT", quit: "drop"},
		{name: "QUIT never answered", quit: "stall", ioTimeout: 300 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureSlog(t)
			stub := startSMTPStub(t, &smtpStub{quit: tc.quit})
			client := NewMailClient(MailConfig{Host: "127.0.0.1", Port: stub.port(), From: "payroll@example.com", IOTimeout: tc.ioTimeout})

			start := time.Now()
			err := client.SendEmail(context.Background(), "ayu@example.com", "Payslip 2026-07", "<p>hi</p>", nil)
			require.NoError(t, err, "a QUIT failure after accepted DATA must not fail the send")
			require.Less(t, time.Since(start), 5*time.Second)

			commands, data := stub.transcript()
			require.Equal(t, []string{"EHLO", "MAIL", "RCPT", "DATA", "QUIT"}, verbs(commands))
			require.Contains(t, data, "<p>hi</p>", "the message was accepted")
			require.Contains(t, logs.String(), "QUIT failed after the message was accepted")
			require.NotContains(t, logs.String(), "ayu@example.com", "the recipient is personal data and must not be logged")
		})
	}
}

// A failure before the server accepted DATA is still a failed send.
func TestSendEmailFailureBeforeDataAcceptedStillFails(t *testing.T) {
	stub := startSMTPStub(t, &smtpStub{stallData: true, quit: "drop"})
	client := NewMailClient(MailConfig{Host: "127.0.0.1", Port: stub.port(), From: "a@example.com", IOTimeout: 300 * time.Millisecond})
	require.Error(t, client.SendEmail(context.Background(), "b@example.com", "s", "b", nil))
}

// net/smtp.SendMail skips AUTH when the server only speaks HELO (no EHLO
// extension list, Client.ext == nil) and sends unauthenticated, and refuses
// only an EHLO server that does not offer AUTH. sendMail must make the same
// decisions with credentials configured; the reference runs SendMail itself
// against an identical stub.
func TestSendEmailCredentialsFollowNetSMTPForHeloOnlyServer(t *testing.T) {
	const addrFmt = "127.0.0.1:%d"
	run := func(t *testing.T, newStub func() *smtpStub) (gotErr, wantErr error, gotCommands, wantCommands []string, gotData, wantData string) {
		t.Helper()
		got := startSMTPStub(t, newStub())
		ref := startSMTPStub(t, newStub())

		client := NewMailClient(MailConfig{Host: "127.0.0.1", Port: got.port(), From: "payroll@example.com", Username: "user", Password: "secret"})
		gotErr = client.SendEmail(context.Background(), "ayu@example.com", "s", "b", nil)
		msg := "From: payroll@example.com\r\nTo: ayu@example.com\r\nSubject: s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=\"UTF-8\"\r\n\r\nb"
		wantErr = smtp.SendMail(fmt.Sprintf(addrFmt, ref.port()), smtp.PlainAuth("", "user", "secret", "127.0.0.1"), "payroll@example.com", []string{"ayu@example.com"}, []byte(msg))
		gotCommands, gotData = got.transcript()
		wantCommands, wantData = ref.transcript()
		return gotErr, wantErr, gotCommands, wantCommands, gotData, wantData
	}

	t.Run("HELO-only server is sent without AUTH", func(t *testing.T) {
		gotErr, wantErr, gotCommands, wantCommands, gotData, wantData := run(t, func() *smtpStub { return &smtpStub{heloOnly: true} })
		require.NoError(t, wantErr, "net/smtp.SendMail sends without AUTH to a HELO-only server")
		require.NoError(t, gotErr)
		require.Equal(t, []string{"EHLO", "HELO", "MAIL", "RCPT", "DATA", "QUIT"}, verbs(gotCommands))
		require.Equal(t, wantCommands, gotCommands)
		require.Equal(t, wantData, gotData)
	})

	t.Run("EHLO server without AUTH is refused", func(t *testing.T) {
		gotErr, wantErr, gotCommands, wantCommands, _, _ := run(t, func() *smtpStub { return &smtpStub{} })
		require.ErrorContains(t, wantErr, "smtp: server doesn't support AUTH")
		require.ErrorContains(t, gotErr, "smtp: server doesn't support AUTH")
		require.Equal(t, []string{"EHLO"}, verbs(wantCommands), "SendMail closes without QUIT")
		require.Equal(t, wantCommands, gotCommands)
	})

	t.Run("EHLO server with AUTH authenticates", func(t *testing.T) {
		gotErr, wantErr, gotCommands, wantCommands, _, _ := run(t, func() *smtpStub { return &smtpStub{auth: true} })
		require.NoError(t, wantErr)
		require.NoError(t, gotErr)
		require.Equal(t, []string{"EHLO", "AUTH", "MAIL", "RCPT", "DATA", "QUIT"}, verbs(gotCommands))
		require.Equal(t, verbs(wantCommands), verbs(gotCommands))
	})
}
