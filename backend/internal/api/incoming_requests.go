package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
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
	if strings.TrimSpace(req.FirstMessageText) != "" {
		ir.FirstMessageText = &req.FirstMessageText
	}
	if req.LanguageCode != "" {
		ir.LanguageCode = &req.LanguageCode
	}

	created, err := s.incomingRequestRepo.CreateOrReuse(c.Request.Context(), ir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create incoming request"})
		return
	}

	if created {
		s.sendIncomingRequestAlert(c.Request.Context(), ir)
		c.JSON(http.StatusCreated, gin.H{"id": ir.ID})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": ir.ID})
}

func (s *Server) sendIncomingRequestAlert(ctx context.Context, ir *models.IncomingRequest) {
	name := "Без имени"
	if ir.FirstName != nil {
		name = alertExcerpt(*ir.FirstName, 200)
	}

	text := fmt.Sprintf("📩 Новое обращение от %s", name)
	if ir.FirstMessageText != nil {
		text += fmt.Sprintf("\n\"%s\"", alertExcerpt(*ir.FirstMessageText, 3500))
	}

	err := s.botClient.SendAlert(ctx, botclient.SendAlertRequest{
		Text: text,
		Buttons: []botclient.AlertButton{
			{Label: "Взять в работу", CallbackData: fmt.Sprintf("take_request:%d", ir.ID)},
		},
	})
	if err != nil {
		s.logger.Error("failed to send incoming request alert", "error", err, "request_id", ir.ID)
	}
}

func alertExcerpt(value string, limit int) string {
	used := 0
	for offset, r := range value {
		used += utf16.RuneLen(r)
		if used >= limit {
			return value[:offset] + "…"
		}
	}
	return value
}

// handleListIncomingRequests godoc
// @Summary      List incoming requests
// @Tags         incoming-requests
// @Produce      json
// @Security     BearerAuth
// @Param        status query string false "Request status (new or processed)"
// @Param        limit query int false "Page size (1–200, default 50)"
// @Param        offset query int false "Number of requests to skip"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} map[string]string
// @Router       /api/incoming-requests [get]
func (s *Server) handleListIncomingRequests(c *gin.Context) {
	page, ok := readPage(c)
	if !ok {
		return
	}
	status := c.Query("status")
	if status == "" {
		status = models.IncomingRequestStatusNew
	}

	ctx := c.Request.Context()

	requests, err := s.incomingRequestRepo.ListByStatusPage(ctx, status, page.Limit+1, page.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list incoming requests"})
		return
	}

	requests, hasMore := trimPage(requests, page.Limit)
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

	c.JSON(http.StatusOK, gin.H{"requests": items, "has_more": hasMore})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	if err := s.incomingRequestRepo.UpdateStatus(ctx, requestID, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
		return
	}

	operatorID, _ := getOperatorID(c)
	s.writeAuditLog(c, operatorID, "operator", "incoming_request_status_updated", "incoming_request", requestID, gin.H{"status": req.Status})
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleMarkIncomingRequestProcessedInternal(c *gin.Context) {
	requestID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request id"})
		return
	}

	if err := s.incomingRequestRepo.UpdateStatus(c.Request.Context(), requestID, models.IncomingRequestStatusProcessed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
		return
	}

	s.writeAuditLog(c, 0, "system", "incoming_request_status_updated", "incoming_request", requestID, gin.H{"status": models.IncomingRequestStatusProcessed})
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleListIncomingRequestsInternal(c *gin.Context) {
	page, ok := readPage(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	requests, err := s.incomingRequestRepo.ListByStatusPage(ctx, models.IncomingRequestStatusNew, page.Limit+1, page.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list incoming requests"})
		return
	}

	type item struct {
		ID               int64  `json:"id"`
		FirstName        string `json:"first_name"`
		FirstMessageText string `json:"first_message_text"`
	}

	requests, hasMore := trimPage(requests, page.Limit)
	items := make([]item, 0, len(requests))
	for _, r := range requests {
		i := item{ID: r.ID}
		if r.FirstName != nil {
			i.FirstName = *r.FirstName
		}
		if r.FirstMessageText != nil {
			i.FirstMessageText = *r.FirstMessageText
		}
		items = append(items, i)
	}

	c.JSON(http.StatusOK, gin.H{"requests": items, "has_more": hasMore})
}
