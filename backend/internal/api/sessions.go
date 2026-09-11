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
	SessionID   int64  `json:"session_id"`
	SessionType string `json:"role"`
	Title       string `json:"title"`
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

	// Привязываем пользователя к сессии в правильной роли, только если ещё не привязан
	if req.Role == "client" && session.ClientUserID == nil {
		clientID := user.ID
		session.ClientUserID = &clientID
		if err := s.sessionRepo.SetClientUser(ctx, session.ID, user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to bind client"})
			return
		}
	} else if req.Role == "executor" && session.ExecutorUserID == nil {
		executorID := user.ID
		session.ExecutorUserID = &executorID
		if err := s.sessionRepo.SetExecutorUser(ctx, session.ID, user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to bind executor"})
			return
		}
	}

	c.JSON(http.StatusOK, bindSessionResponse{
		SessionID:   session.ID,
		SessionType: req.Role,
		Title:       session.Title,
	})
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
