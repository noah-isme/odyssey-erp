package shared

import (
	"bytes"
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// MailConfig holds SMTP configuration.
type MailConfig struct {
	Host string
	Port int
	From string
	// Optional authentication
	Username string
	Password string
}

// MailClient sends emails via SMTP.
type MailClient struct {
	config MailConfig
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
	write := func(format string, args ...any) {
		_, _ = fmt.Fprintf(&msg, format, args...)
	}

	// Use multipart if attachment is present
	if attachment != nil {
		boundary := "----=_Part_0_1234567890.1234567890"
		write("From: %s\r\n", c.config.From)
		write("To: %s\r\n", to)
		write("Subject: %s\r\n", subject)
		msg.WriteString("MIME-Version: 1.0\r\n")
		write("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary)
		msg.WriteString("\r\n")

		// Text part
		write("--%s\r\n", boundary)
		msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
		msg.WriteString("\r\n")
		msg.WriteString(body)
		msg.WriteString("\r\n")

		// Attachment part
		write("--%s\r\n", boundary)
		write("Content-Type: %s; name=\"%s\"\r\n", attachment.ContentType, attachment.Filename)
		msg.WriteString("Content-Transfer-Encoding: base64\r\n")
		write("Content-Disposition: attachment; filename=\"%s\"\r\n", attachment.Filename)
		msg.WriteString("\r\n")
		msg.WriteString(encodeBase64(attachment.Data))
		msg.WriteString("\r\n")
		write("--%s--\r\n", boundary)
	} else {
		write("From: %s\r\n", c.config.From)
		write("To: %s\r\n", to)
		write("Subject: %s\r\n", subject)
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

	err := smtp.SendMail(addr, auth, c.config.From, []string{to}, msg.Bytes())
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
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
