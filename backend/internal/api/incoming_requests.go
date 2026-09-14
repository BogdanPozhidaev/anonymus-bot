package api

import (
	"net/http"
	"strconv"

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

type incomingRequestListItem struct {
	ID               int64   `json:"id"`
	TelegramID       int64   `json:"telegram_id"`
	Username         *string `json:"username"`
	FirstName        *string `json:"first_name"`
	FirstMessageText *string `json:"first_message_text"`
	LanguageCode     *string `json:"language_code"`
	Status           string  `json:"status"`
	CreatedAt        string  `json:"created_at"`
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

func (s *Server) handleListIncomingRequests(c *gin.Context) {
	status := c.Query("status")
	if status == "" {
		status = models.IncomingRequestStatusNew
	}

	ctx := c.Request.Context()

	requests, err := s.incomingRequestRepo.ListByStatus(ctx, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list incoming requests"})
		return
	}

	items := make([]incomingRequestListItem, 0, len(requests))
	for _, r := range requests {
		items = append(items, incomingRequestListItem{
			ID:               r.ID,
			TelegramID:       r.TelegramID,
			Username:         r.Username,
			FirstName:        r.FirstName,
			FirstMessageText: r.FirstMessageText,
			LanguageCode:     r.LanguageCode,
			Status:           r.Status,
			CreatedAt:        r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	c.JSON(http.StatusOK, gin.H{"requests": items})
}

type updateIncomingRequestStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=new processed"`
}

func (s *Server) handleUpdateIncomingRequestStatus(c *gin.Context) {
	requestID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request id"})
		return
	}

	var req updateIncomingRequestStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	if err := s.incomingRequestRepo.UpdateStatus(ctx, requestID, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
