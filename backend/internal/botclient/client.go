package botclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type SendMessageRequest struct {
	ContentType         string `json:"content_type,omitempty"`
	FileID              string `json:"file_id,omitempty"`
	RecipientTelegramID int64  `json:"recipient_telegram_id"`
	SenderLabel         string `json:"sender_label"`
	Text                string `json:"text"`
	SessionID           int64  `json:"session_id"`
}

type Client struct {
	baseURL       string
	internalToken string
	httpClient    *http.Client
}

type SendAlertRequest struct {
	Text    string        `json:"text"`
	Buttons []AlertButton `json:"buttons,omitempty"`
}

type AlertButton struct {
	Label        string `json:"label"`
	CallbackData string `json:"callback_data"`
}

func New(baseURL, internalToken string) *Client {
	return &Client{
		baseURL:       baseURL,
		internalToken: internalToken,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/internal/send-message", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Internal-Token", c.internalToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request to bot: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bot returned status %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) SendAlert(ctx context.Context, req SendAlertRequest) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal alert request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/internal/send-alert", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Internal-Token", c.internalToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send alert request to bot: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bot returned status %d for alert", resp.StatusCode)
	}

	return nil
}
