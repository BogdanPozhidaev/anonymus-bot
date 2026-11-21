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

const MenuPageSize = 10

func (c *Client) ListUserSessions(ctx context.Context, telegramID int64, offset int) ([]UserSessionListItem, bool, error) {
	var resp struct {
		Sessions []UserSessionListItem `json:"sessions"`
		HasMore  bool                  `json:"has_more"`
	}

	path := fmt.Sprintf("/internal/users/by-telegram/%d/sessions?limit=%d&offset=%d", telegramID, MenuPageSize, offset)
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, false, err
	}

	return resp.Sessions, resp.HasMore, nil
}

func (c *Client) SwitchActiveSession(ctx context.Context, telegramID, targetSessionID int64) error {
	path := fmt.Sprintf("/internal/users/by-telegram/%d/active-session", telegramID)
	return c.doRequest(ctx, "PATCH", path, map[string]int64{"target_session_id": targetSessionID}, nil)
}
