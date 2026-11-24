package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/botclient"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
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
	OwnerOperatorID  *int64  `json:"owner_operator_id"`
	SessionID        *int64  `json:"session_id"`
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
		c.JSON(http.StatusCreated, gin.H{"id": ir.ID, "status": ir.Status})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": ir.ID, "status": ir.Status})
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
			{Label: "Открыть обращение", CallbackData: fmt.Sprintf("open_request:%d", ir.ID)},
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
// @Param        status query string false "Request status (new, in_progress for the caller's assignments, or processed)"
// @Param        scope query string false "Use all with in_progress to list all assigned requests as an administrator"
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
	all := c.Query("scope") == "all"
	if all {
		role, _ := getOperatorRole(c)
		if role != models.OperatorRoleAdmin {
			writeIncomingRequestError(c, repository.ErrIncomingRequestForbidden)
			return
		}
		if status != models.IncomingRequestStatusInProgress {
			c.JSON(http.StatusBadRequest, gin.H{"error": "scope all requires in_progress status"})
			return
		}
	}

	ctx := c.Request.Context()

	var requests []*models.IncomingRequest
	var err error
	if status == models.IncomingRequestStatusInProgress && !all {
		operatorID, _ := getOperatorID(c)
		requests, err = s.incomingRequestRepo.ListAssignedPage(ctx, operatorID, page.Limit+1, page.Offset)
	} else {
		requests, err = s.incomingRequestRepo.ListByStatusPage(ctx, status, page.Limit+1, page.Offset)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list incoming requests"})
		return
	}

	requests, hasMore := trimPage(requests, page.Limit)
	items := make([]incomingRequestListItem, 0, len(requests))
	for _, r := range requests {
		items = append(items, incomingRequestItem(r))
	}

	c.JSON(http.StatusOK, gin.H{"requests": items, "has_more": hasMore})
}

func incomingRequestItem(r *models.IncomingRequest) incomingRequestListItem {
	return incomingRequestListItem{
		ID: r.ID, TelegramID: r.TelegramID, Username: r.Username, FirstName: r.FirstName,
		FirstMessageText: r.FirstMessageText, LanguageCode: r.LanguageCode, Status: r.Status,
		OwnerOperatorID: r.OwnerOperatorID, SessionID: r.SessionID,
		CreatedAt: r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// @Summary      Read an incoming request
// @Tags         incoming-requests
// @Security     BearerAuth
// @Param        id path int true "Incoming request ID"
// @Success      200 {object} incomingRequestListItem
// @Failure      403 {object} map[string]string
// @Failure      404 {object} map[string]string
// @Router       /api/incoming-requests/{id} [get]
func (s *Server) handleGetIncomingRequest(c *gin.Context) {
	id, ok := readSessionID(c)
	if !ok {
		return
	}
	operatorID, _ := getOperatorID(c)
	role, _ := getOperatorRole(c)
	request, err := s.incomingRequestRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		writeIncomingRequestError(c, err)
		return
	}
	if request.OwnerOperatorID != nil && *request.OwnerOperatorID != operatorID && role != models.OperatorRoleAdmin {
		writeIncomingRequestError(c, repository.ErrIncomingRequestForbidden)
		return
	}
	c.JSON(http.StatusOK, incomingRequestItem(request))
}

// @Summary      Claim an incoming request
// @Tags         incoming-requests
// @Security     BearerAuth
// @Param        id path int true "Incoming request ID"
// @Success      204
// @Failure      409 {object} map[string]string
// @Router       /api/incoming-requests/{id}/claim [post]
func (s *Server) handleClaimIncomingRequest(c *gin.Context) {
	id, ok := readSessionID(c)
	if !ok {
		return
	}
	operatorID, _ := getOperatorID(c)
	role, _ := getOperatorRole(c)
	if err := s.incomingRequestRepo.Claim(c.Request.Context(), id, operatorID, role); err != nil {
		writeIncomingRequestError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// @Summary      Create or open the session linked to an incoming request
// @Tags         incoming-requests
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path int true "Incoming request ID"
// @Param        request body createSessionRequest true "Session details"
// @Success      200 {object} map[string]int
// @Success      201 {object} map[string]int
// @Failure      403 {object} map[string]string
// @Failure      409 {object} map[string]string
// @Router       /api/incoming-requests/{id}/session [post]
func (s *Server) handleOpenIncomingRequestSession(c *gin.Context) {
	id, ok := readSessionID(c)
	if !ok {
		return
	}
	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	operatorID, _ := getOperatorID(c)
	role, _ := getOperatorRole(c)
	if req.Language == "" {
		req.Language = "ru"
	}
	session := &models.Session{Title: req.Title, Status: models.SessionStatusActive, Language: req.Language}
	sessionID, created, err := s.incomingRequestRepo.OpenSession(c.Request.Context(), id, operatorID, role, session)
	if err != nil {
		writeIncomingRequestError(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"id": sessionID})
}

func writeIncomingRequestError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
	case errors.Is(err, repository.ErrIncomingRequestForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
	case errors.Is(err, repository.ErrIncomingRequestConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "request state changed"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "incoming request operation failed"})
	}
}

type transferIncomingRequestBody struct {
	ExpectedOwnerID int64 `json:"expected_owner_id" binding:"required,gt=0"`
	OperatorID      int64 `json:"operator_id" binding:"required,gt=0"`
}

// @Summary      Transfer an assigned request and its linked session
// @Tags         incoming-requests
// @Accept       json
// @Security     BearerAuth
// @Param        id path int true "Incoming request ID"
// @Param        request body transferIncomingRequestBody true "Expected and new owner"
// @Success      204
// @Failure      403 {object} map[string]string
// @Failure      409 {object} map[string]string
// @Router       /api/incoming-requests/{id}/transfer [post]
func (s *Server) handleTransferIncomingRequest(c *gin.Context) {
	id, ok := readSessionID(c)
	if !ok {
		return
	}
	var req transferIncomingRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	operatorID, _ := getOperatorID(c)
	if err := s.incomingRequestRepo.Transfer(c.Request.Context(), id, operatorID, c.GetInt64(contextKeyAuthVersion), req.ExpectedOwnerID, req.OperatorID); err != nil {
		writeIncomingRequestError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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
		writeIncomingRequestError(c, err)
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
		writeIncomingRequestError(c, err)
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
