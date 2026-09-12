package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

type bindSessionRequest struct {
	TelegramID int64  `json:"telegram_id" binding:"required"`
	Username   string `json:"username"`
	FirstName  string `json:"first_name"`
	Role       string `json:"role" binding:"required,oneof=client executor"`
}

type bindSessionResponse struct {
	SessionID           int64                `json:"session_id"`
	SessionType         string               `json:"role"`
	Title               string               `json:"title"`
	UndeliveredMessages []undeliveredMessage `json:"undelivered_messages,omitempty"`
}

type undeliveredMessage struct {
	MessageID   int64  `json:"message_id"`
	ContentType string `json:"content_type"`
	Content     string `json:"content,omitempty"`
	FileID      string `json:"file_id,omitempty"`
	SenderLabel string `json:"sender_label"`
}

func (s *Server) handleBindSession(c *gin.Context) {
	sessionIDStr := c.Param("id")
	sessionID, err := strconv.ParseInt(sessionIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	var req bindSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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

	user, err := s.getOrCreateUser(ctx, req.TelegramID, req.Username, req.FirstName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get or create user"})
		return
	}

	if err := s.userRepo.SetActiveSession(ctx, user.ID, session.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set active session"})
		return
	}

	if req.Role == "client" && session.ClientUserID == nil {
		if err := s.sessionRepo.SetClientUser(ctx, session.ID, user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to bind client"})
			return
		}
		session.ClientUserID = &user.ID
	} else if req.Role == "executor" && session.ExecutorUserID == nil {
		if err := s.sessionRepo.SetExecutorUser(ctx, session.ID, user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to bind executor"})
			return
		}
		session.ExecutorUserID = &user.ID
	}

	undelivered, err := s.collectUndeliveredMessagesFor(ctx, session, user.ID)
	if err != nil {
		s.logger.Error("failed to collect undelivered messages", "error", err, "session_id", session.ID)
		// Не блокируем сам bind из-за этой ошибки — привязка важнее, сообщения можно попробовать доставить позже.
	}

	c.JSON(http.StatusOK, bindSessionResponse{
		SessionID:           session.ID,
		SessionType:         req.Role,
		Title:               session.Title,
		UndeliveredMessages: undelivered,
	})
}

// collectUndeliveredMessagesFor находит недоставленные сообщения сессии, адресованные
// только что подключившемуся пользователю (то есть отправленные противоположной стороной),
// помечает их доставленными и возвращает для отправки ботом.
func (s *Server) collectUndeliveredMessagesFor(ctx context.Context, session *models.Session, newlyBoundUserID int64) ([]undeliveredMessage, error) {
	allUndelivered, err := s.messageRepo.ListUndelivered(ctx, session.ID)
	if err != nil {
		return nil, err
	}

	if len(allUndelivered) == 0 {
		return nil, nil
	}

	var result []undeliveredMessage
	var deliveredIDs []int64

	for _, m := range allUndelivered {
		// Сообщение адресовано новому участнику, только если отправитель — НЕ он сам.
		if m.SenderUserID != nil && *m.SenderUserID == newlyBoundUserID {
			continue
		}

		label := "Клиент:"
		if m.SenderRole == models.SenderRoleExecutor {
			label = "Менеджер:"
		}

		item := undeliveredMessage{
			MessageID:   m.ID,
			ContentType: m.ContentType,
			SenderLabel: label,
		}
		if m.Content != nil {
			item.Content = *m.Content
		}
		if m.FileID != nil {
			item.FileID = *m.FileID
		}

		result = append(result, item)
		deliveredIDs = append(deliveredIDs, m.ID)
	}

	if len(deliveredIDs) > 0 {
		if err := s.messageRepo.MarkDelivered(ctx, deliveredIDs); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// getOrCreateUser ищет пользователя по telegram_id; если не найден — создаёт нового.
func (s *Server) getOrCreateUser(ctx context.Context, telegramID int64, username, firstName string) (*models.User, error) {
	user, err := s.userRepo.GetByTelegramID(ctx, telegramID)
	if err == nil {
		return user, nil
	}

	if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	newUser := &models.User{
		TelegramID:   telegramID,
		LanguageCode: "ru",
	}
	if username != "" {
		newUser.Username = &username
	}
	if firstName != "" {
		newUser.FirstName = &firstName
	}

	if err := s.userRepo.Create(ctx, newUser); err != nil {
		return nil, err
	}

	return newUser, nil
}
