package backendclient

import "context"

type RelayMessageRequest struct {
	SenderTelegramID int64  `json:"sender_telegram_id"`
	ContentType      string `json:"content_type"`
	Content          string `json:"content,omitempty"`
	FileID           string `json:"file_id,omitempty"`
}

type RelayMessageResponse struct {
	Blocked             bool   `json:"blocked"`
	BlockReason         string `json:"block_reason,omitempty"`
	RecipientTelegramID int64  `json:"recipient_telegram_id,omitempty"`
	SenderLabel         string `json:"sender_label,omitempty"`
	MessageID           int64  `json:"message_id,omitempty"`
}

func (c *Client) RelayMessage(ctx context.Context, req RelayMessageRequest) (*RelayMessageResponse, error) {
	var resp RelayMessageResponse

	if err := c.doRequest(ctx, "POST", "/internal/messages/relay", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
