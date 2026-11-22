package backendclient

import (
	"context"
	"fmt"
)

type BindSessionRequest struct {
	TelegramID   int64  `json:"telegram_id"`
	Username     string `json:"username,omitempty"`
	FirstName    string `json:"first_name,omitempty"`
	Token        string `json:"token"`
	LanguageCode string `json:"language_code,omitempty"`
}

type BindSessionResponse struct {
	SessionID   int64  `json:"session_id"`
	SessionType string `json:"role"`
	Title       string `json:"title"`
}

type StopSessionResponse struct {
	SessionID             int64 `json:"session_id"`
	CounterpartTelegramID int64 `json:"counterpart_telegram_id"`
}

type OperatorSessionItem struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

func (c *Client) BindSession(ctx context.Context, req BindSessionRequest) (*BindSessionResponse, error) {
	var resp BindSessionResponse

	path := "/internal/sessions/bind"
	if err := c.doRequest(ctx, "POST", path, req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *Client) StopSession(ctx context.Context, telegramID int64) (*StopSessionResponse, error) {
	var resp StopSessionResponse
	err := c.doRequest(ctx, "POST", "/internal/sessions/stop", map[string]int64{"telegram_id": telegramID}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListOperatorSessions(ctx context.Context, telegramID int64, offset int) ([]OperatorSessionItem, bool, error) {
	var resp struct {
		Sessions []OperatorSessionItem `json:"sessions"`
		HasMore  bool                  `json:"has_more"`
	}
	path := fmt.Sprintf("/internal/operators/by-telegram/%d/sessions?limit=%d&offset=%d", telegramID, MenuPageSize, offset)
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, false, err
	}
	return resp.Sessions, resp.HasMore, nil
}
