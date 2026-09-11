package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

type createIncomingRequestRequest struct {
	TelegramID       int64  `json:"telegram_id" binding:"required"`
	Username         string `json:"username"`
	FirstName        string `json:"first_name"`
	FirstMessageText string `json:"first_message_text"`
	LanguageCode     string `json:"language_code"`
}

func (s *Server) handleCreateIncomingRequest(c *gin.Context) {
	var req createIncomingRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ir := &models.IncomingRequest{
		TelegramID: req.TelegramID,
		Status:     models.IncomingRequestStatusNew,
	}
	if req.Username != "" {
		ir.Username = &req.Username
	}
	if req.FirstName != "" {
		ir.FirstName = &req.FirstName
	}
	if req.FirstMessageText != "" {
		ir.FirstMessageText = &req.FirstMessageText
	}
	if req.LanguageCode != "" {
		ir.LanguageCode = &req.LanguageCode
	}

	if err := s.incomingRequestRepo.Create(c.Request.Context(), ir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create incoming request"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": ir.ID})
}
