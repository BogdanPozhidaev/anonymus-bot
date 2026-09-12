package handlers_test

import (
	"testing"

	"github.com/fastcheck/anonymus_bot/bot/internal/handlers"
)

func TestParseDeepLinkPayload_ValidClient(t *testing.T) {
	result, err := handlers.ParseDeepLinkPayload("42-c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionID != 42 {
		t.Errorf("expected session_id 42, got %d", result.SessionID)
	}
	if result.Role != handlers.RoleClient {
		t.Errorf("expected role client, got %s", result.Role)
	}
}

func TestParseDeepLinkPayload_ValidExecutor(t *testing.T) {
	result, err := handlers.ParseDeepLinkPayload("100-e")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionID != 100 {
		t.Errorf("expected session_id 100, got %d", result.SessionID)
	}
	if result.Role != handlers.RoleExecutor {
		t.Errorf("expected role executor, got %s", result.Role)
	}
}

func TestParseDeepLinkPayload_Empty(t *testing.T) {
	_, err := handlers.ParseDeepLinkPayload("")
	if err != handlers.ErrInvalidPayload {
		t.Errorf("expected ErrInvalidPayload, got %v", err)
	}
}

func TestParseDeepLinkPayload_InvalidFormat(t *testing.T) {
	cases := []string{"42", "42-c-extra", "abc-c", "42-x", "-c", "42-"}

	for _, tc := range cases {
		_, err := handlers.ParseDeepLinkPayload(tc)
		if err != handlers.ErrInvalidPayload {
			t.Errorf("payload %q: expected ErrInvalidPayload, got %v", tc, err)
		}
	}
}
