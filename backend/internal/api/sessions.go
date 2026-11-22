package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/fastcheck/anonymus_bot/backend/internal/service"
)

type bindSessionRequest struct {
	TelegramID   int64  `json:"telegram_id" binding:"required"`
	Username     string `json:"username"`
	FirstName    string `json:"first_name"`
	Token        string `json:"token" binding:"required"`
	LanguageCode string `json:"language_code,omitempty" binding:"omitempty,oneof=ru en"`
}

type bindSessionResponse struct {
	SessionID   int64  `json:"session_id"`
	SessionType string `json:"role"`
	Title       string `json:"title"`
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
	var req bindSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()
	user := &models.User{TelegramID: req.TelegramID, LanguageCode: "ru"}
	if req.LanguageCode != "" {
		user.LanguageCode = req.LanguageCode
	}
	if req.Username != "" {
		user.Username = &req.Username
	}
	if req.FirstName != "" {
		user.FirstName = &req.FirstName
	}
	session, role, err := s.sessionFlow.Bind(ctx, req.Token, user)
	if errors.Is(err, repository.ErrInviteUnavailable) {
		c.JSON(http.StatusNotFound, gin.H{"error": "invitation unavailable"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusOK, bindSessionResponse{
		SessionID:   session.ID,
		SessionType: role,
		Title:       session.Title,
	})
}

// handleListSessions godoc
// @Summary      List sessions
// @Description  Administrators see all sessions, operators see sessions they own
// @Tags         sessions
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} map[string]interface{}
// @Failure      401 {object} map[string]string
// @Param        limit query int false "Page size (1–200, default 50)"
// @Param        offset query int false "Number of sessions to skip"
// @Param        status query string false "Session status"
// @Param        q query string false "Title substring or session ID"
// @Router       /api/sessions [get]
func (s *Server) handleListSessions(c *gin.Context) {
	page, ok := readPage(c)
	if !ok {
		return
	}
	status := c.Query("status")
	switch status {
	case "", models.SessionStatusActive, models.SessionStatusPaused, models.SessionStatusPendingClose, models.SessionStatusClosed:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session status"})
		return
	}
	query := strings.TrimSpace(c.Query("q"))
	if len(query) > 240 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "search query is too long"})
		return
	}

	operatorID, _ := getOperatorID(c)
	role, _ := getOperatorRole(c)

	var owner *int64
	if role != models.OperatorRoleAdmin {
		owner = &operatorID
	}

	sessions, err := s.sessionRepo.ListPanelPage(c.Request.Context(), owner, status, query, page.Limit+1, page.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}

	hasMore := len(sessions) > page.Limit
	if hasMore {
		sessions = sessions[:page.Limit]
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

	c.JSON(http.StatusOK, gin.H{"sessions": items, "has_more": hasMore})
}

// handleGetSession godoc
// @Summary      Get a session by ID
// @Tags         sessions
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "Session ID"
// @Success      200 {object} models.Session
// @Failure      403 {object} map[string]string
// @Failure      404 {object} map[string]string
// @Router       /api/sessions/{id} [get]
func (s *Server) handleGetSession(c *gin.Context) {
	sessionID, ok := readSessionID(c)
	if !ok {
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

// canAccessSession allows administrators and the operator who owns the session.
func (s *Server) canAccessSession(c *gin.Context, session *models.Session) bool {
	role, _ := getOperatorRole(c)
	if role == models.OperatorRoleAdmin {
		return true
	}

	operatorID, _ := getOperatorID(c)
	return session.OwnerOperatorID != nil && *session.OwnerOperatorID == operatorID
}

// handleCreateSession godoc
// @Summary      Create a session
// @Tags         sessions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body createSessionRequest true "Session details"
// @Success      201 {object} models.Session
// @Router       /api/sessions [post]
func (s *Server) handleCreateSession(c *gin.Context) {
	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
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

	if err := s.sessionRepo.AssignOwner(ctx, session.ID, operatorID); err != nil {
		s.logger.Error("failed to assign creator as owner", "error", err, "session_id", session.ID)
	}

	s.writeAuditLog(c, operatorID, "operator", "session_created", "session", session.ID, nil)
	c.JSON(http.StatusCreated, session)
}

type updateSessionStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active paused closed"`
}

// handleUpdateSessionStatus godoc
// @Summary      Update session status
// @Tags         sessions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "Session ID"
// @Param        request body updateSessionStatusRequest true "New status"
// @Success      200 {object} map[string]string
// @Failure      403 {object} map[string]string
// @Failure      409 {object} map[string]string
// @Failure      413 {object} map[string]string
// @Router       /api/sessions/{id}/status [patch]
func (s *Server) handleUpdateSessionStatus(c *gin.Context) {
	sessionID, ok := readSessionID(c)
	if !ok {
		return
	}

	var req updateSessionStatusRequest
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

	if session.Status == req.Status {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}
	changed, err := s.sessionFlow.ChangeStatus(ctx, session, req.Status)
	if errors.Is(err, service.ErrModerationUnavailable) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "moderation unavailable; try again"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
		return
	}
	if !changed {
		c.JSON(http.StatusConflict, gin.H{"error": "session status changed; refresh and retry"})
		return
	}

	operatorID, _ := getOperatorID(c)
	s.writeAuditLog(c, operatorID, "operator", "session_status_updated", "session", sessionID, gin.H{"new_status": req.Status})

	if req.Status == models.SessionStatusPaused || req.Status == models.SessionStatusClosed {
		s.notifySessionStatus(ctx, session, req.Status)
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type assignOwnerRequest struct {
	OperatorID int64 `json:"operator_id" binding:"required"`
}

func (s *Server) handleAssignSessionOwner(c *gin.Context) {
	sessionID, ok := readSessionID(c)
	if !ok {
		return
	}

	var req assignOwnerRequest
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

	// Only administrators and the current owner can reassign a session.
	if !s.canAccessSession(c, session) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := s.sessionRepo.AssignOwner(ctx, sessionID, req.OperatorID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to assign owner"})
		return
	}

	operatorID, _ := getOperatorID(c)
	s.writeAuditLog(c, operatorID, "operator", "session_owner_reassigned", "session", sessionID, gin.H{"new_owner_id": req.OperatorID})

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleListSessionMessages godoc
// @Summary      Read a page of session history
// @Tags         sessions
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "Session ID"
// @Param        limit query int false "Page size (1–200, default 50)"
// @Param        offset query int false "Number of newest messages to skip"
// @Success      200 {object} map[string]interface{}
// @Router       /api/sessions/{id}/messages [get]
func (s *Server) handleListSessionMessages(c *gin.Context) {
	page, ok := readPage(c)
	if !ok {
		return
	}
	sessionID, ok := readSessionID(c)
	if !ok {
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

	messages, err := s.messageRepo.ListRecentBySessionID(ctx, sessionID, page.Limit+1, page.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list messages"})
		return
	}

	hasMore := len(messages) > page.Limit
	if hasMore {
		messages = messages[:page.Limit]
	}
	slices.Reverse(messages)
	if messages == nil {
		messages = []*models.Message{}
	}
	c.JSON(http.StatusOK, gin.H{"messages": messages, "has_more": hasMore})
}

func (s *Server) handleListUserSessions(c *gin.Context) {
	page, ok := readPage(c)
	if !ok {
		return
	}
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

	sessions, err := s.sessionRepo.ListByParticipantPage(ctx, user.ID, user.ActiveSessionID, page.Limit+1, page.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}

	sessions, hasMore := trimPage(sessions, page.Limit)
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

	c.JSON(http.StatusOK, gin.H{"sessions": items, "has_more": hasMore})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
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

	s.writeAuditLog(c, user.ID, "user", "session_switched", "session", req.TargetSessionID, nil)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleStopSession(c *gin.Context) {
	var req stopSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
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

	s.writeAuditLog(c, user.ID, "user", "session_stop_requested", "session", session.ID, nil)

	// Notify the counterpart that a participant requested closure.
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

	s.sendStopRequestAlert(ctx, session)

	c.JSON(http.StatusOK, gin.H{
		"session_id":              session.ID,
		"counterpart_telegram_id": counterpartTelegramID,
	})
}

func (s *Server) sendStopRequestAlert(ctx context.Context, session *models.Session) {
	text := fmt.Sprintf("🛑 Запрос на завершение сессии\n%s (#%d)", session.Title, session.ID)

	err := s.botClient.SendAlert(ctx, botclient.SendAlertRequest{
		Text: text,
		Buttons: []botclient.AlertButton{
			{Label: "Открыть в панели", CallbackData: fmt.Sprintf("open_session:%d", session.ID)},
			{Label: "Закрыть сессию", CallbackData: fmt.Sprintf("close_session:%d", session.ID)},
		},
	})
	if err != nil {
		s.logger.Error("failed to send stop request alert", "error", err, "session_id", session.ID)
	}
}

func (s *Server) handleListOperatorSessionsInternal(c *gin.Context) {
	page, ok := readPage(c)
	if !ok {
		return
	}
	telegramID, err := strconv.ParseInt(c.Param("telegramID"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid telegram id"})
		return
	}

	ctx := c.Request.Context()

	operator, err := s.operatorRepo.GetByTelegramID(ctx, telegramID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "operator not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if operator.MustChangePassword {
		c.JSON(http.StatusNotFound, gin.H{"error": "operator not found"})
		return
	}
	sessions, err := s.sessionRepo.ListPanelPage(ctx, &operator.ID, "", "", page.Limit+1, page.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}

	type item struct {
		ID     int64  `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
	}

	sessions, hasMore := trimPage(sessions, page.Limit)
	items := make([]item, 0, len(sessions))
	for _, sess := range sessions {
		items = append(items, item{ID: sess.ID, Title: sess.Title, Status: sess.Status})
	}

	c.JSON(http.StatusOK, gin.H{"sessions": items, "has_more": hasMore})
}
