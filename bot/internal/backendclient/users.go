package backendclient

import (
	"context"
	"fmt"
)

type UserResponse struct {
	ID              int64  `json:"id"`
	TelegramID      int64  `json:"telegram_id"`
	ActiveSessionID *int64 `json:"active_session_id"`
}

type UpdateLanguageRequest struct {
	LanguageCode string `json:"language_code"`
}

func (c *Client) GetUserByTelegramID(ctx context.Context, telegramID int64) (*UserResponse, error) {
	var resp UserResponse

	path := fmt.Sprintf("/internal/users/by-telegram/%d", telegramID)
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *Client) UpdateUserLanguage(ctx context.Context, telegramID int64, languageCode string) error {
	path := fmt.Sprintf("/internal/users/by-telegram/%d/language", telegramID)
	return c.doRequest(ctx, "PATCH", path, UpdateLanguageRequest{LanguageCode: languageCode}, nil)
}
