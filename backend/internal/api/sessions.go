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

type sessionListItem struct {
	ID              int64  `json:"id"`
	Title           string `json:"title"`
	Status          string `json:"status"`
	OwnerOperatorID *int64 `json:"owner_operator_id"`
	ClientUserID    *int64 `json:"client_user_id"`
	ExecutorUserID  *int64 `json:"executor_user_id"`
	CreatedAt       string `json:"created_at"`
}

type createSessionRequest struct {
	Title    string `json:"title" binding:"required"`
	Language string `json:"language"`
}

type userSessionListItem struct {
	SessionID int64  `json:"session_id"`
	Title     string `json:"title"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	IsCurrent bool   `json:"is_current"`
}

type stopSessionRequest struct {
	TelegramID int64 `json:"telegram_id" binding:"required"`
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

func (s *Server) handleListSessions(c *gin.Context) {
	ctx := c.Request.Context()

	operatorID, _ := getOperatorID(c)
	role, _ := getOperatorRole(c)

	var sessions []*models.Session
	var err error

	if role == models.OperatorRoleAdmin {
		sessions, err = s.sessionRepo.ListAll(ctx)
	} else {
		sessions, err = s.sessionRepo.ListByOperator(ctx, operatorID)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}

	items := make([]sessionListItem, 0, len(sessions))
	for _, sess := range sessions {
		items = append(items, sessionListItem{
			ID:              sess.ID,
			Title:           sess.Title,
			Status:          sess.Status,
			OwnerOperatorID: sess.OwnerOperatorID,
			ClientUserID:    sess.ClientUserID,
			ExecutorUserID:  sess.ExecutorUserID,
			CreatedAt:       sess.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	c.JSON(http.StatusOK, gin.H{"sessions": items})
}

func (s *Server) handleGetSession(c *gin.Context) {
	sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
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

	c.JSON(http.StatusOK, session)
}

// canAccessSession — единая проверка прав доступа к конкретной сессии:
// admin видит всё, operator — только сессии, где он owner. Вынесена
// отдельно, чтобы использовать во всех хендлерах, работающих с одной
// сессией по ID, и не дублировать эту логику безопасности в каждом месте.
func (s *Server) canAccessSession(c *gin.Context, session *models.Session) bool {
	role, _ := getOperatorRole(c)
	if role == models.OperatorRoleAdmin {
		return true
	}

	operatorID, _ := getOperatorID(c)
	return session.OwnerOperatorID != nil && *session.OwnerOperatorID == operatorID
}

func (s *Server) handleCreateSession(c *gin.Context) {
	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	operatorID, _ := getOperatorID(c)

	language := req.Language
	if language == "" {
		language = "ru"
	}

	session := &models.Session{
		Title:     req.Title,
		Status:    models.SessionStatusActive,
		Language:  language,
		CreatedBy: &operatorID,
	}

	ctx := c.Request.Context()
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	// Создатель сразу становится владельцем сессии.
	if err := s.sessionRepo.AssignOwner(ctx, session.ID, operatorID); err != nil {
		s.logger.Error("failed to assign creator as owner", "error", err, "session_id", session.ID)
	}

	c.JSON(http.StatusCreated, session)
}

type updateSessionStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active paused closed"`
}

func (s *Server) handleUpdateSessionStatus(c *gin.Context) {
	sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	var req updateSessionStatusRequest
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

	if !s.canAccessSession(c, session) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := s.sessionRepo.UpdateStatus(ctx, sessionID, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
		return
	}

	operatorID, _ := getOperatorID(c)
	s.writeAuditLog(ctx, operatorID, "session_status_updated", "session", sessionID, gin.H{"new_status": req.Status})

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type assignOwnerRequest struct {
	OperatorID int64 `json:"operator_id" binding:"required"`
}

func (s *Server) handleAssignSessionOwner(c *gin.Context) {
	sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}

	var req assignOwnerRequest
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

	// Переназначение владельца — только admin, либо текущий владелец сессии.
	if !s.canAccessSession(c, session) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := s.sessionRepo.AssignOwner(ctx, sessionID, req.OperatorID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to assign owner"})
		return
	}

	operatorID, _ := getOperatorID(c)
	s.writeAuditLog(ctx, operatorID, "session_owner_reassigned", "session", sessionID, gin.H{"new_owner_id": req.OperatorID})

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleListSessionMessages(c *gin.Context) {
	sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
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

	messages, err := s.messageRepo.ListBySessionID(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list messages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

func (s *Server) handleListUserSessions(c *gin.Context) {
	telegramID, err := strconv.ParseInt(c.Param("telegramID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid telegram id"})
		return
	}

	ctx := c.Request.Context()

	user, err := s.userRepo.GetByTelegramID(ctx, telegramID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	sessions, err := s.sessionRepo.ListByParticipant(ctx, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}

	items := make([]userSessionListItem, 0, len(sessions))
	for _, sess := range sessions {
		role := models.SenderRoleClient
		if sess.ExecutorUserID != nil && *sess.ExecutorUserID == user.ID {
			role = models.SenderRoleExecutor
		}

		items = append(items, userSessionListItem{
			SessionID: sess.ID,
			Title:     sess.Title,
			Role:      role,
			Status:    sess.Status,
			IsCurrent: user.ActiveSessionID != nil && *user.ActiveSessionID == sess.ID,
		})
	}

	c.JSON(http.StatusOK, gin.H{"sessions": items})
}

type switchActiveSessionRequest struct {
	TargetSessionID int64 `json:"target_session_id" binding:"required"`
}

func (s *Server) handleSwitchActiveSession(c *gin.Context) {
	telegramID, err := strconv.ParseInt(c.Param("telegramID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid telegram id"})
		return
	}

	var req switchActiveSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	user, err := s.userRepo.GetByTelegramID(ctx, telegramID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	err = s.userRepo.SwitchActiveSessionTx(ctx, user.ID, req.TargetSessionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotParticipant) {
			c.JSON(http.StatusForbidden, gin.H{"error": "not a participant of this session"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to switch session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleStopSession(c *gin.Context) {
	var req stopSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	user, err := s.userRepo.GetByTelegramID(ctx, req.TelegramID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if user.ActiveSessionID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no active session"})
		return
	}

	session, err := s.sessionRepo.GetByID(ctx, *user.ActiveSessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if err := s.sessionRepo.UpdateStatus(ctx, session.ID, models.SessionStatusPendingClose); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update session status"})
		return
	}

	if err := s.sessionRepo.SetCloseRequestedBy(ctx, session.ID, user.ID); err != nil {
		s.logger.Error("failed to set close_requested_by", "error", err, "session_id", session.ID)
	}

	s.writeAuditLog(ctx, user.ID, "session_stop_requested", "session", session.ID, nil)

	// Уведомляем вторую сторону нейтральным сообщением.
	var counterpartUserID *int64
	if session.ClientUserID != nil && *session.ClientUserID == user.ID {
		counterpartUserID = session.ExecutorUserID
	} else {
		counterpartUserID = session.ClientUserID
	}

	var counterpartTelegramID int64
	if counterpartUserID != nil {
		counterpart, err := s.userRepo.GetByID(ctx, *counterpartUserID)
		if err == nil {
			counterpartTelegramID = counterpart.TelegramID
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"session_id":              session.ID,
		"counterpart_telegram_id": counterpartTelegramID,
	})
}
