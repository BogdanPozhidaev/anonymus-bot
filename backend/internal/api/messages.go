package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

type relayMessageRequest struct {
	SenderTelegramID int64  `json:"sender_telegram_id" binding:"required"`
	ContentType      string `json:"content_type" binding:"required,oneof=text photo voice"`
	Content          string `json:"content"`
	FileID           string `json:"file_id"`
}

type relayMessageResponse struct {
	Blocked             bool   `json:"blocked"`
	BlockReason         string `json:"block_reason,omitempty"`
	RecipientTelegramID int64  `json:"recipient_telegram_id,omitempty"`
	SenderLabel         string `json:"sender_label,omitempty"`
	MessageID           int64  `json:"message_id,omitempty"`
	SessionID           int64  `json:"session_id,omitempty"`
}

func (s *Server) handleRelayMessage(c *gin.Context) {
	var req relayMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	resp, err := s.relayMessage(ctx, req)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "sender or session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// relayMessage — ядро логики маппинга. Определяет сессию и роль отправителя,
// проверяет её статус, находит получателя (вторую сторону), сохраняет сообщение.
func (s *Server) relayMessage(ctx context.Context, req relayMessageRequest) (*relayMessageResponse, error) {
	sender, err := s.userRepo.GetByTelegramID(ctx, req.SenderTelegramID)
	if err != nil {
		return nil, err
	}

	if sender.ActiveSessionID == nil {
		return &relayMessageResponse{
			Blocked:     true,
			BlockReason: "no_active_session",
		}, nil
	}

	session, err := s.sessionRepo.GetByID(ctx, *sender.ActiveSessionID)
	if err != nil {
		return nil, err
	}

	if session.Status != models.SessionStatusActive {
		return &relayMessageResponse{
			Blocked:     true,
			BlockReason: "session_not_active",
		}, nil
	}

	senderRole, recipientUserID, senderLabel := determineRoleAndRecipient(session, sender.ID)
	if senderRole == "" {
		return &relayMessageResponse{
			Blocked:     true,
			BlockReason: "sender_not_in_session",
		}, nil
	}

	// Модерация применяется только к тексту — для фото/голоса контентный
	// анализ не выполняется в MVP (OCR намеренно исключён по решению ТЗ).
	if req.ContentType == models.ContentTypeText && req.Content != "" {
		modResult, err := s.moderationService.Check(ctx, sender.ID, req.Content)
		if err != nil {
			s.logger.Error("moderation check failed", "error", err, "user_id", sender.ID)
			// Не блокируем сообщение из-за сбоя самой проверки — отказ
			// модерации не должен парализовать всю переписку.
		} else if modResult.Blocked {
			s.logger.Warn("message blocked by moderation",
				"user_id", sender.ID,
				"session_id", session.ID,
				"reason", modResult.Reason,
				"violation_count", modResult.ViolationCount,
			)

			if modResult.ShouldPause {
				if err := s.sessionRepo.UpdateStatus(ctx, session.ID, models.SessionStatusPaused); err != nil {
					s.logger.Error("failed to pause session after violations", "error", err, "session_id", session.ID)
				} else {
					s.logger.Warn("session auto-paused due to repeated violations", "session_id", session.ID, "violation_count", modResult.ViolationCount)
				}
			}
			return &relayMessageResponse{
				Blocked:             false,
				RecipientTelegramID: recipient.TelegramID,
				SenderLabel:         senderLabel,
				MessageID:           message.ID,
				SessionID:           session.ID,
			}, nil
		}
	}

	message := &models.Message{
		SessionID:    session.ID,
		SenderUserID: &sender.ID,
		SenderRole:   senderRole,
		ContentType:  req.ContentType,
		Delivered:    recipientUserID != nil,
	}
	if req.Content != "" {
		message.Content = &req.Content
	}
	if req.FileID != "" {
		message.FileID = &req.FileID
	}

	if err := s.messageRepo.Create(ctx, message); err != nil {
		return nil, err
	}

	if recipientUserID == nil {
		return &relayMessageResponse{
			Blocked:     true,
			BlockReason: "counterparty_not_bound_yet",
			MessageID:   message.ID,
		}, nil
	}

	recipient, err := s.userRepo.GetByID(ctx, *recipientUserID)
	if err != nil {
		return nil, err
	}

	return &relayMessageResponse{
		Blocked:             false,
		RecipientTelegramID: recipient.TelegramID,
		SenderLabel:         senderLabel,
		MessageID:           message.ID,
	}, nil
}

// determineRoleAndRecipient возвращает роль отправителя, ID пользователя-получателя
// (вторая сторона сессии) и подпись, с которой сообщение придёт получателю.
func determineRoleAndRecipient(session *models.Session, senderUserID int64) (role string, recipientUserID *int64, label string) {
	if session.ClientUserID != nil && *session.ClientUserID == senderUserID {
		return models.SenderRoleClient, session.ExecutorUserID, "Клиент:"
	}
	if session.ExecutorUserID != nil && *session.ExecutorUserID == senderUserID {
		return models.SenderRoleExecutor, session.ClientUserID, "Менеджер:"
	}
	return "", nil, ""
}
