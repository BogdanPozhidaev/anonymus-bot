package backendclient

import (
	"context"
	"fmt"
)

type OperatorResponse struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Status   string `json:"status"`
	Language string `json:"language"`
}

func (c *Client) ClaimTelegramLink(ctx context.Context, telegramID int64, code string) (string, error) {
	var response struct {
		ConfirmationCode string `json:"confirmation_code"`
	}
	err := c.doRequest(ctx, "POST", "/internal/operators/telegram-link/claim",
		map[string]any{"telegram_id": telegramID, "code": code}, &response)
	return response.ConfirmationCode, err
}

func (c *Client) GetOperatorByTelegramID(ctx context.Context, telegramID int64) (*OperatorResponse, error) {
	var resp OperatorResponse

	path := fmt.Sprintf("/internal/operators/by-telegram/%d", telegramID)
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
