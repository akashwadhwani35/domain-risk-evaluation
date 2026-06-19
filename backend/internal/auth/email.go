package auth

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// OTPSender abstracts delivery of OTP codes so the transport (Resend HTTP API,
// plain SMTP, etc.) can be swapped via configuration.
type OTPSender interface {
	SendOTP(email, code string) error
}

// EmailService handles sending OTP emails via Resend.
type EmailService struct {
	apiKey    string
	fromEmail string
	client    *http.Client
}

// ResendRequest represents the Resend API request body
type ResendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

// ResendResponse represents the Resend API response
type ResendResponse struct {
	ID string `json:"id"`
}

// ResendError represents an error from the Resend API
type ResendError struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
	Name       string `json:"name"`
}

// NewEmailService creates a new email service
func NewEmailService(apiKey, fromEmail string) (*EmailService, error) {
	if apiKey == "" {
		return nil, errors.New("RESEND_API_KEY is required")
	}
	if fromEmail == "" {
		return nil, errors.New("RESEND_FROM_EMAIL is required")
	}
	return &EmailService{
		apiKey:    apiKey,
		fromEmail: fromEmail,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// SendOTP sends an OTP code to the specified email address via Resend.
func (s *EmailService) SendOTP(email, code string) error {
	reqBody := ResendRequest{
		From:    s.fromEmail,
		To:      []string{email},
		Subject: otpSubject(code),
		HTML:    otpEmailHTML(code),
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errResp ResendError
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return fmt.Errorf("resend API error: status %d", resp.StatusCode)
		}
		return fmt.Errorf("resend API error: %s", errResp.Message)
	}

	return nil
}

// SMTPService sends OTP emails over plain SMTP. This works with any standard
// mailbox (e.g. Gmail with an app password) and can deliver to ANY recipient
// without owning/verifying a sending domain.
type SMTPService struct {
	host     string // e.g. smtp.gmail.com
	port     string // e.g. 587 (STARTTLS) or 465 (implicit TLS)
	username string
	password string
	from     string // From address; defaults to username
}

// NewSMTPService creates an SMTP-backed OTP sender.
func NewSMTPService(host, port, username, password, from string) (*SMTPService, error) {
	if host == "" {
		return nil, errors.New("SMTP_HOST is required")
	}
	if username == "" || password == "" {
		return nil, errors.New("SMTP_USERNAME and SMTP_PASSWORD are required")
	}
	if port == "" {
		port = "587"
	}
	if from == "" {
		from = username
	}
	return &SMTPService{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
	}, nil
}

// SendOTP delivers the OTP code over SMTP to any recipient.
func (s *SMTPService) SendOTP(email, code string) error {
	msg := s.buildMessage(email, code)
	addr := s.host + ":" + s.port
	auth := smtp.PlainAuth("", s.username, s.password, s.host)

	if s.port == "465" {
		// Implicit TLS (SMTPS): dial a TLS connection up front.
		return s.sendImplicitTLS(addr, auth, email, msg)
	}

	// STARTTLS path (port 587 and most others): smtp.SendMail upgrades the
	// connection to TLS automatically when the server advertises STARTTLS.
	if err := smtp.SendMail(addr, auth, s.from, []string{email}, msg); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}

func (s *SMTPService) sendImplicitTLS(addr string, auth smtp.Auth, to string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.host})
	if err != nil {
		return fmt.Errorf("smtp tls dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Quit()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return nil
}

func (s *SMTPService) buildMessage(to, code string) []byte {
	var b strings.Builder
	b.WriteString("From: " + s.from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + otpSubject(code) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(otpEmailHTML(code))
	return []byte(b.String())
}

// otpSubject returns the email subject line for an OTP code.
func otpSubject(code string) string {
	return fmt.Sprintf("Your verification code: %s", code)
}

// otpEmailHTML renders the shared HTML body for an OTP email.
func otpEmailHTML(code string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; background-color: #f5f5f5;">
    <table role="presentation" style="width: 100%%; border-collapse: collapse;">
        <tr>
            <td align="center" style="padding: 40px 0;">
                <table role="presentation" style="width: 100%%; max-width: 600px; border-collapse: collapse; background-color: #ffffff; border-radius: 8px; box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);">
                    <tr>
                        <td style="padding: 40px 40px 20px;">
                            <h1 style="margin: 0 0 20px; color: #1a1a1a; font-size: 24px; font-weight: 600;">
                                Domain Risk Evaluation
                            </h1>
                            <p style="margin: 0 0 30px; color: #666666; font-size: 16px; line-height: 24px;">
                                Your verification code is:
                            </p>
                            <div style="background-color: #f8f9fa; border-radius: 8px; padding: 24px; text-align: center; margin-bottom: 30px;">
                                <span style="font-size: 36px; font-weight: 700; letter-spacing: 8px; color: #1a1a1a;">%s</span>
                            </div>
                            <p style="margin: 0 0 20px; color: #666666; font-size: 14px; line-height: 22px;">
                                This code will expire in <strong>5 minutes</strong>.
                            </p>
                            <p style="margin: 0; color: #999999; font-size: 12px; line-height: 20px;">
                                If you didn't request this code, you can safely ignore this email.
                            </p>
                        </td>
                    </tr>
                    <tr>
                        <td style="padding: 20px 40px 40px; border-top: 1px solid #eee;">
                            <p style="margin: 0; color: #999999; font-size: 12px;">
                                Domain Risk Evaluation Tool
                            </p>
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
    </table>
</body>
</html>
`, code)
}
