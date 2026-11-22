package service

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

func TestBindRejectsMalformedInvitationBeforeRepository(t *testing.T) {
	sessions := &Sessions{}
	for _, token := range []string{"short", strings.Repeat("!", base64.RawURLEncoding.EncodedLen(InviteTokenBytes))} {
		session, role, err := sessions.Bind(t.Context(), token, &models.User{})
		if !errors.Is(err, repository.ErrInviteUnavailable) || session.ID != 0 || role != "" {
			t.Fatalf("malformed invitation reached the binding flow or returned an unexpected result: %v", err)
		}
	}
}
