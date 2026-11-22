package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/moderation"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

type RelayInput struct {
	SenderTelegramID int64
	ContentType      string
	Content          string
	FileID           string
}

type RelayResult struct {
	Blocked     bool
	BlockReason string
	MessageID   int64
	SessionID   int64
}

type Relay struct {
	userRepo          *repository.UserRepository
	sessionRepo       *repository.SessionRepository
	messageRepo       *repository.MessageRepository
	moderationService *moderation.Service
	logger            *slog.Logger
}

func NewRelay(users *repository.UserRepository, sessions *repository.SessionRepository, messages *repository.MessageRepository, moderation *moderation.Service, logger *slog.Logger) *Relay {
	return &Relay{userRepo: users, sessionRepo: sessions, messageRepo: messages, moderationService: moderation, logger: logger}
}

// Submit resolves the sender's session and role, applies moderation,
// and queues accepted messages for delivery.
func (s *Relay) Submit(ctx context.Context, req RelayInput, sessionPaused func(context.Context, *models.Session, string), moderationAlert func(context.Context, *models.Session, moderation.CheckResult)) (*RelayResult, error) {
	sender, err := s.userRepo.GetByTelegramID(ctx, req.SenderTelegramID)
	if errors.Is(err, repository.ErrNotFound) {
		return &RelayResult{Blocked: true, BlockReason: "no_active_session"}, nil
	}
	if err != nil {
		return nil, err
	}

	if sender.ActiveSessionID == nil {
		return &RelayResult{
			Blocked:     true,
			BlockReason: "no_active_session",
		}, nil
	}

	session, err := s.sessionRepo.GetByID(ctx, *sender.ActiveSessionID)
	if err != nil {
		return nil, err
	}

	if session.Status != models.SessionStatusActive {
		return &RelayResult{
			Blocked:     true,
			BlockReason: "session_not_active",
		}, nil
	}

	senderRole, recipientUserID := session.ParticipantRole(sender.ID)
	if senderRole == "" {
		return &RelayResult{
			Blocked:     true,
			BlockReason: "sender_not_in_session",
		}, nil
	}

	// Apply the same checks to media captions and ordinary text.
	if req.Content != "" {
		modResult, err := s.moderationService.Check(ctx, session.ID, "user", sender.ID, req.Content)
		if err != nil {
			s.logger.Error("moderation check failed", "error", err, "user_id", sender.ID)
			return &RelayResult{Blocked: true, BlockReason: "moderation_unavailable"}, nil
		}
		if modResult.Blocked {
			s.logger.Warn("message blocked by moderation",
				"user_id", sender.ID,
				"session_id", session.ID,
				"reason", modResult.Reason,
				"violation_count", modResult.ViolationCount,
			)

			if modResult.ShouldPause {
				changed, err := s.sessionRepo.TransitionStatus(ctx, session.ID, models.SessionStatusActive, models.SessionStatusPaused)
				modResult.ShouldPause = changed
				if err != nil {
					s.logger.Error("failed to pause session after violations", "error", err, "session_id", session.ID)
				} else if changed {
					s.logger.Warn("session auto-paused due to repeated violations", "session_id", session.ID, "violation_count", modResult.ViolationCount)
					sessionPaused(ctx, session, models.SessionStatusPaused)
				}
			}

			moderationAlert(ctx, session, modResult)

			return &RelayResult{
				Blocked:     true,
				BlockReason: "moderation_violation",
			}, nil
		}
	}

	message := &models.Message{
		SessionID:    session.ID,
		SenderUserID: &sender.ID,
		SenderRole:   senderRole,
		ContentType:  req.ContentType,
	}
	if req.Content != "" {
		message.Content = &req.Content
	}
	if req.FileID != "" {
		message.FileID = &req.FileID
	}

	if err := s.messageRepo.Enqueue(ctx, message); err != nil {
		return nil, err
	}

	if recipientUserID == nil {
		return &RelayResult{
			Blocked:     true,
			BlockReason: "counterparty_not_bound_yet",
			MessageID:   message.ID,
			SessionID:   session.ID,
		}, nil
	}

	return &RelayResult{MessageID: message.ID, SessionID: session.ID}, nil
}
