package backendclient

import (
	"context"
	"fmt"
)

type UserSessionListItem struct {
	SessionID int64  `json:"session_id"`
	Title     string `json:"title"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	IsCurrent bool   `json:"is_current"`
}

func (c *Client) ListUserSessions(ctx context.Context, telegramID int64) ([]UserSessionListItem, error) {
	var resp struct {
		Sessions []UserSessionListItem `json:"sessions"`
	}

	path := fmt.Sprintf("/internal/users/by-telegram/%d/sessions", telegramID)
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}

	return resp.Sessions, nil
}

func (c *Client) SwitchActiveSession(ctx context.Context, telegramID, targetSessionID int64) error {
	path := fmt.Sprintf("/internal/users/by-telegram/%d/active-session", telegramID)
	return c.doRequest(ctx, "PATCH", path, map[string]int64{"target_session_id": targetSessionID}, nil)
}
