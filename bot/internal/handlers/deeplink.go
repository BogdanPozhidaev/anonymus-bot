package handlers

import (
	"errors"
	"strconv"
	"strings"
)

var ErrInvalidPayload = errors.New("invalid deeplink payload")

type SessionRole string

const (
	RoleClient   SessionRole = "client"
	RoleExecutor SessionRole = "executor"
)

type ParsedPayload struct {
	SessionID int64
	Role      SessionRole
}

// ParseDeepLinkPayload разбирает payload вида "42-c" или "42-e"
// в session_id=42 и роль (client/executor).
func ParseDeepLinkPayload(payload string) (*ParsedPayload, error) {
	if payload == "" {
		return nil, ErrInvalidPayload
	}

	parts := strings.Split(payload, "-")
	if len(parts) != 2 {
		return nil, ErrInvalidPayload
	}

	sessionID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, ErrInvalidPayload
	}

	var role SessionRole
	switch parts[1] {
	case "c":
		role = RoleClient
	case "e":
		role = RoleExecutor
	default:
		return nil, ErrInvalidPayload
	}

	return &ParsedPayload{
		SessionID: sessionID,
		Role:      role,
	}, nil
}
