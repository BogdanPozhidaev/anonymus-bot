package backendclient

import (
	"context"
	"fmt"
)

type BindSessionRequest struct {
	TelegramID int64  `json:"telegram_id"`
	Username   string `json:"username,omitempty"`
	FirstName  string `json:"first_name,omitempty"`
	Role       string `json:"role"`
}

type BindSessionResponse struct {
	SessionID   int64  `json:"session_id"`
	SessionType string `json:"role"`
	Title       string `json:"title"`
}

func (c *Client) BindSession(ctx context.Context, sessionID int64, req BindSessionRequest) (*BindSessionResponse, error) {
	var resp BindSessionResponse

	path := fmt.Sprintf("/internal/sessions/%d/bind", sessionID)
	if err := c.doRequest(ctx, "POST", path, req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
