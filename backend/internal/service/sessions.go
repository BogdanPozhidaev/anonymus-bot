package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/moderation"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

const InviteTokenBytes = 32

var ErrModerationUnavailable = errors.New("moderation unavailable")

type Sessions struct {
	sessions   *repository.SessionRepository
	users      *repository.UserRepository
	moderation *moderation.Service
}

func NewSessions(sessions *repository.SessionRepository, users *repository.UserRepository, moderation *moderation.Service) *Sessions {
	return &Sessions{sessions: sessions, users: users, moderation: moderation}
}

func (s *Sessions) Bind(ctx context.Context, token string, user *models.User) (models.Session, string, error) {
	if len(token) != base64.RawURLEncoding.EncodedLen(InviteTokenBytes) {
		return models.Session{}, "", repository.ErrInviteUnavailable
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil {
		return models.Session{}, "", repository.ErrInviteUnavailable
	}
	hash := sha256.Sum256(raw)
	return s.sessions.BindInvite(ctx, s.users, hash[:], user)
}

// ChangeStatus resets moderation on resume before attempting the status transition.
func (s *Sessions) ChangeStatus(ctx context.Context, session *models.Session, status string) (bool, error) {
	if status == models.SessionStatusActive {
		if err := s.moderation.Reset(ctx, session.ID); err != nil {
			return false, ErrModerationUnavailable
		}
	}
	return s.sessions.TransitionStatus(ctx, session.ID, session.Status, status)
}
