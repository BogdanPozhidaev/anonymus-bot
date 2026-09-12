package backendclient

import "context"

type CreateIncomingRequestRequest struct {
	TelegramID       int64  `json:"telegram_id"`
	Username         string `json:"username,omitempty"`
	FirstName        string `json:"first_name,omitempty"`
	FirstMessageText string `json:"first_message_text,omitempty"`
	LanguageCode     string `json:"language_code,omitempty"`
}

type CreateIncomingRequestResponse struct {
	ID int64 `json:"id"`
}

func (c *Client) CreateIncomingRequest(ctx context.Context, req CreateIncomingRequestRequest) (*CreateIncomingRequestResponse, error) {
	var resp CreateIncomingRequestResponse

	if err := c.doRequest(ctx, "POST", "/internal/incoming-requests", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
