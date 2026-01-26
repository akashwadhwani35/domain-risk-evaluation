package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// EmailService handles sending OTP emails via Resend
type EmailService struct {
	apiKey    string
	fromEmail string
	client    *http.Client
}

// ResendRequest represents the Resend API request body
type ResendRequest struct {
	From    string `json:"from"`
	To      []string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
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

// SendOTP sends an OTP code to the specified email address
func (s *EmailService) SendOTP(email, code string) error {
	html := fmt.Sprintf(`
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

	reqBody := ResendRequest{
		From:    s.fromEmail,
		To:      []string{email},
		Subject: fmt.Sprintf("Your verification code: %s", code),
		HTML:    html,
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
