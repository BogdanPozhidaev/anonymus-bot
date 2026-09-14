package backendclient

import (
	"context"
	"fmt"
)

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

type IncomingRequestInternalItem struct {
	ID               int64  `json:"id"`
	FirstName        string `json:"first_name"`
	FirstMessageText string `json:"first_message_text"`
}

func (c *Client) CreateIncomingRequest(ctx context.Context, req CreateIncomingRequestRequest) (*CreateIncomingRequestResponse, error) {
	var resp CreateIncomingRequestResponse

	if err := c.doRequest(ctx, "POST", "/internal/incoming-requests", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *Client) ListIncomingRequestsInternal(ctx context.Context) ([]IncomingRequestInternalItem, error) {
	var resp struct {
		Requests []IncomingRequestInternalItem `json:"requests"`
	}
	if err := c.doRequest(ctx, "GET", "/internal/incoming-requests", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Requests, nil
}

func (c *Client) MarkIncomingRequestProcessed(ctx context.Context, requestID int64) error {
	path := fmt.Sprintf("/internal/incoming-requests/%d/mark-processed", requestID)
	return c.doRequest(ctx, "POST", path, nil, nil)
}
