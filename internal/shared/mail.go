package shared

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"sync/atomic"
	"time"
)

// Default SMTP deadlines used when MailConfig leaves them zero.
const (
	DefaultMailDialTimeout = 10 * time.Second
	DefaultMailIOTimeout   = 60 * time.Second
)

// MailConfig holds SMTP configuration.
type MailConfig struct {
	Host string
	Port int
	From string
	// Optional authentication
	Username string
	Password string
	// DialTimeout bounds the TCP connect; zero means DefaultMailDialTimeout.
	DialTimeout time.Duration
	// IOTimeout bounds the whole SMTP conversation after connect; zero means
	// DefaultMailIOTimeout.
	IOTimeout time.Duration
}

// MailClient sends emails via SMTP.
type MailClient struct {
	config MailConfig
	// rootCAs overrides the system roots for STARTTLS verification. Tests only.
	rootCAs *x509.CertPool
}

// NewMailClient creates a new mail client.
func NewMailClient(config MailConfig) *MailClient {
	return &MailClient{config: config}
}

// SendEmail sends an email with optional attachment.
func (c *MailClient) SendEmail(ctx context.Context, to, subject, body string, attachment *Attachment) error {
	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)

	// Build email headers and body
	var msg bytes.Buffer

	// Use multipart if attachment is present
	if attachment != nil {
		boundary := "----=_Part_0_1234567890.1234567890"
		msg.WriteString(fmt.Sprintf("From: %s\r\n", c.config.From))
		msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
		msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
		msg.WriteString("MIME-Version: 1.0\r\n")
		msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary))
		msg.WriteString("\r\n")

		// Text part
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		msg.WriteString("\r\n")
		msg.WriteString(body)
		msg.WriteString("\r\n")

		// Attachment part
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", attachment.ContentType, attachment.Filename))
		msg.WriteString("Content-Transfer-Encoding: base64\r\n")
		msg.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", attachment.Filename))
		msg.WriteString("\r\n")
		msg.WriteString(encodeBase64(attachment.Data))
		msg.WriteString("\r\n")
		msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		msg.WriteString(fmt.Sprintf("From: %s\r\n", c.config.From))
		msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
		msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
		msg.WriteString("MIME-Version: 1.0\r\n")
		msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		msg.WriteString("\r\n")
		msg.WriteString(body)
	}

	// Send email
	var auth smtp.Auth
	if c.config.Username != "" && c.config.Password != "" {
		auth = smtp.PlainAuth("", c.config.Username, c.config.Password, c.config.Host)
	}

	err := c.sendMail(ctx, addr, auth, c.config.From, []string{to}, msg.Bytes())
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// heloTracker records whether the SMTP client fell back from EHLO to HELO.
// net/smtp keeps that state private (Client.ext is nil only after a HELO
// fallback) but net/smtp.SendMail branches on it, so sendMail observes the
// commands the client writes to reproduce the same decision.
type heloTracker struct {
	net.Conn
	sawHELO atomic.Bool
}

func (t *heloTracker) Write(p []byte) (int, error) {
	// net/smtp flushes one command per write, so a HELO command starts a write.
	if len(p) >= 5 && strings.EqualFold(string(p[:5]), "HELO ") {
		t.sawHELO.Store(true)
	}
	return t.Conn.Write(p)
}

// sendMail mirrors net/smtp.SendMail (HELO localhost, opportunistic STARTTLS
// verified against the configured host, PLAIN auth when credentials are set,
// MAIL/RCPT/DATA/QUIT) but bounds the dial and the whole conversation with
// deadlines so a stalled server cannot hang the caller indefinitely.
//
// Two behaviors follow net/smtp.SendMail on purpose:
//   - Credentials are only enforced when the server answered EHLO. A server
//     that only speaks HELO has no extension list (SendMail's c.ext == nil), so
//     SendMail skips AUTH and sends unauthenticated; so does sendMail. An EHLO
//     server that does not offer AUTH is still refused.
//   - Once the server accepted DATA (the final dot), the message is sent. A
//     failing QUIT afterwards is logged and ignored, because reporting it as a
//     failure would make the caller retry and deliver the message twice.
func (c *MailClient) sendMail(ctx context.Context, addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	dialTimeout := c.config.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = DefaultMailDialTimeout
	}
	ioTimeout := c.config.IOTimeout
	if ioTimeout <= 0 {
		ioTimeout = DefaultMailIOTimeout
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}

	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(ioTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err = conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return err
	}

	tracker := &heloTracker{Conn: conn}
	client, err := smtp.NewClient(tracker, host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = client.Close() }()

	if err = client.Hello("localhost"); err != nil {
		return err
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err = client.StartTLS(&tls.Config{ServerName: host, RootCAs: c.rootCAs}); err != nil {
			return err
		}
	}
	if auth != nil && !tracker.sawHELO.Load() {
		// net/smtp.SendMail refuses to send unauthenticated when credentials
		// are configured but the EHLO server does not offer AUTH; keep that.
		// With a HELO-only server it skips AUTH (see heloTracker).
		if ok, _ := client.Extension("AUTH"); !ok {
			return errors.New("smtp: server doesn't support AUTH")
		}
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err = client.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(msg); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	// The server accepted the message. A QUIT failure (timeout, dropped
	// connection) cannot undo that, so it must not be reported as a failed
	// send: the caller would retry and deliver a duplicate.
	if err = client.Quit(); err != nil {
		slog.Warn("smtp: QUIT failed after the message was accepted; treating the send as successful",
			slog.String("host", host), slog.String("error", err.Error()))
	}
	return nil
}

// SendEmailWithPDF sends an email with a PDF attachment.
func (c *MailClient) SendEmailWithPDF(ctx context.Context, to, subject, body string, pdfData []byte, filename string) error {
	attachment := &Attachment{
		Filename:    filename,
		ContentType: "application/pdf",
		Data:        pdfData,
	}
	return c.SendEmail(ctx, to, subject, body, attachment)
}

// Attachment represents an email attachment.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// encodeBase64 encodes data to base64 with line breaks.
func encodeBase64(data []byte) string {
	const lineLength = 76
	encoded := make([]byte, base64EncodedLen(len(data)))
	base64Encode(encoded, data)

	// Add line breaks every 76 characters
	var result strings.Builder
	for i := 0; i < len(encoded); i += lineLength {
		end := i + lineLength
		if end > len(encoded) {
			end = len(encoded)
		}
		result.Write(encoded[i:end])
		result.WriteString("\r\n")
	}
	return result.String()
}

// Base64 encoding (standard library compatible)
const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

func base64EncodedLen(n int) int {
	return (n + 2) / 3 * 4
}

func base64Encode(dst, src []byte) {
	for len(src) > 0 {
		var b0, b1, b2 byte
		switch len(src) {
		default:
			b2 = src[2]
			fallthrough
		case 2:
			b1 = src[1]
			fallthrough
		case 1:
			b0 = src[0]
		}

		dst[0] = base64Chars[b0>>2]
		dst[1] = base64Chars[(b0&0x03)<<4|(b1>>4)]
		dst[2] = base64Chars[(b1&0x0F)<<2|(b2>>6)]
		dst[3] = base64Chars[b2&0x3F]

		if len(src) < 3 {
			dst[3] = '='
			if len(src) < 2 {
				dst[2] = '='
			}
			break
		}

		src = src[3:]
		dst = dst[4:]
	}
}
