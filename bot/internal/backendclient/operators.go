package backendclient

import (
	"context"
	"fmt"
)

type OperatorResponse struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

func (c *Client) GetOperatorByTelegramID(ctx context.Context, telegramID int64) (*OperatorResponse, error) {
	var resp OperatorResponse

	path := fmt.Sprintf("/internal/operators/by-telegram/%d", telegramID)
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
