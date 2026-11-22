package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/moderation"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/fastcheck/anonymus_bot/backend/internal/service"
)

type relayMessageRequest struct {
	SenderTelegramID int64  `json:"sender_telegram_id" binding:"required"`
	ContentType      string `json:"content_type" binding:"required,oneof=text photo voice"`
	Content          string `json:"content"`
	FileID           string `json:"file_id"`
}

type relayMessageResponse struct {
	Blocked     bool   `json:"blocked"`
	BlockReason string `json:"block_reason,omitempty"`
	MessageID   int64  `json:"message_id,omitempty"`
	SessionID   int64  `json:"session_id,omitempty"`
}

type sendAsOperatorRequest struct {
	ImpersonatedRole string `json:"impersonated_role" binding:"required,oneof=client executor"`
	ContentType      string `json:"content_type" binding:"required,oneof=text"`
	Content          string `json:"content" binding:"required"`
}

func (s *Server) handleRelayMessage(c *gin.Context) {
	var req relayMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	if ((req.ContentType == models.ContentTypePhoto || req.ContentType == models.ContentTypeVoice) && !models.ValidMediaFileID(req.FileID)) || (req.ContentType == models.ContentTypeText && req.FileID != "") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid media file id"})
		return
	}
	resp, err := s.relay.Submit(ctx, service.RelayInput(req), s.notifySessionStatus, s.sendModerationAlert)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "sender or session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, relayMessageResponse(*resp))
}

// handleSendMessageAsOperator godoc
// @Summary      Queue a text message on behalf of a session participant
// @Tags         sessions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "Session ID"
// @Param        request body sendAsOperatorRequest true "Text and sender role"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} map[string]string
// @Failure      403 {object} map[string]string
// @Failure      413 {object} map[string]string
// @Failure      503 {object} map[string]string
// @Router       /api/sessions/{id}/send-as [post]
func (s *Server) handleSendMessageAsOperator(c *gin.Context) {
	sessionID, ok := readSessionID(c)
	if !ok {
		return
	}

	var req sendAsOperatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if !s.canAccessSession(c, session) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if session.Status != models.SessionStatusActive {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session is not active"})
		return
	}

	operatorID, _ := getOperatorID(c)

	// Messages sent on behalf of participants use the same moderation checks.
	modResult, err := s.moderationService.Check(ctx, session.ID, "operator", operatorID, req.Content)
	if err != nil {
		s.logger.Error("moderation check failed for operator message", "error", err, "operator_id", operatorID)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "moderation unavailable; try again"})
		return
	}
	if modResult.Blocked {
		s.logger.Warn("operator message blocked by moderation",
			"operator_id", operatorID,
			"session_id", session.ID,
			"reason", modResult.Reason,
		)
		c.JSON(http.StatusBadRequest, gin.H{"error": "message violates moderation policy", "reason": modResult.Reason})
		return
	}

	var recipientUserID *int64
	if req.ImpersonatedRole == models.SenderRoleClient {
		recipientUserID = session.ExecutorUserID
	} else {
		recipientUserID = session.ClientUserID
	}

	if recipientUserID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "counterparty not connected yet"})
		return
	}

	recipient, err := s.userRepo.GetByID(ctx, *recipientUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	impersonatedRole := req.ImpersonatedRole
	message := &models.Message{
		SessionID:        session.ID,
		SenderRole:       models.SenderRoleOperator,
		ContentType:      req.ContentType,
		Content:          &req.Content,
		SentByOperator:   true,
		ImpersonatedRole: &impersonatedRole,
	}

	if err := s.messageRepo.Enqueue(ctx, message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create message"})
		return
	}

	senderLabel := "Клиент:"
	if req.ImpersonatedRole == models.SenderRoleExecutor {
		senderLabel = "Менеджер:"
	}

	s.writeAuditLog(c, operatorID, "operator", "operator_sent_message_as", "session", session.ID, gin.H{
		"impersonated_role": req.ImpersonatedRole,
	})

	c.JSON(http.StatusOK, gin.H{
		"message_id":            message.ID,
		"recipient_telegram_id": recipient.TelegramID,
		"sender_label":          senderLabel,
	})
}

func (s *Server) sendModerationAlert(ctx context.Context, session *models.Session, modResult moderation.CheckResult) {
	priority := "⚠️"
	if modResult.ShouldPause {
		priority = "🔴 ВЫСОКИЙ ПРИОРИТЕТ"
	}

	text := fmt.Sprintf(
		"%s Сработал фильтр модерации\nСессия: %s (#%d)\nПричина: %s\nСрабатываний: %d",
		priority, session.Title, session.ID, modResult.Reason, modResult.ViolationCount,
	)
	if modResult.ShouldPause {
		text += "\n\n🔴 Сессия автоматически поставлена на паузу"
	}

	err := s.botClient.SendAlert(ctx, botclient.SendAlertRequest{
		Text: text,
		Buttons: []botclient.AlertButton{
			{Label: "Открыть в панели", CallbackData: fmt.Sprintf("open_session:%d", session.ID)},
		},
	})
	if err != nil {
		s.logger.Error("failed to send moderation alert", "error", err, "session_id", session.ID)
	}
}
