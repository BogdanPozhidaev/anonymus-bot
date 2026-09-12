package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

type updateLanguageRequest struct {
	LanguageCode string `json:"language_code" binding:"required,oneof=ru en"`
}

func (s *Server) handleGetUserByTelegramID(c *gin.Context) {
	telegramIDStr := c.Param("telegramID")
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid telegram id"})
		return
	}

	user, err := s.userRepo.GetByTelegramID(c.Request.Context(), telegramID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":                user.ID,
		"telegram_id":       user.TelegramID,
		"active_session_id": user.ActiveSessionID,
	})
}

func (s *Server) handleUpdateUserLanguage(c *gin.Context) {
	telegramIDStr := c.Param("telegramID")
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid telegram id"})
		return
	}

	var req updateLanguageRequest
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

	user.LanguageCode = req.LanguageCode

	if err := s.userRepo.Update(ctx, user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update language"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
