package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/fastcheck/anonymus_bot/backend/internal/transcript"
	"github.com/gin-gonic/gin"
)

const (
	maxTranscriptMessages = 2000
	maxTranscriptText     = 1 << 20
)

// handleExportTranscript godoc
// @Summary      Download a session transcript
// @Description  Owner and administrator access. The export includes message text and media captions without attachment files.
// @Tags         sessions
// @Produce      text/csv,application/pdf
// @Security     BearerAuth
// @Param        id path int true "Session ID"
// @Param        format query string true "Export format" Enums(csv,pdf)
// @Success      200 {file} file
// @Failure      400 {object} map[string]string
// @Failure      401 {object} map[string]string
// @Failure      403 {object} map[string]string
// @Failure      404 {object} map[string]string
// @Failure      422 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Failure      503 {object} map[string]string
// @Router       /api/sessions/{id}/transcript [get]
func (s *Server) handleExportTranscript(c *gin.Context) {
	id, ok := readSessionID(c)
	if !ok {
		return
	}
	format := c.Query("format")
	if format != "csv" && format != "pdf" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format must be csv or pdf"})
		return
	}
	ctx := c.Request.Context()
	session, err := s.sessionRepo.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export transcript"})
		return
	}
	if !s.canAccessSession(c, session) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}
	select {
	case s.transcriptSlots <- struct{}{}:
		defer func() { <-s.transcriptSlots }()
	default:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "transcript exporter is busy"})
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	entries, err := s.messageRepo.ListTranscript(readCtx, id, maxTranscriptMessages, maxTranscriptText)
	cancel()
	if errors.Is(err, repository.ErrTranscriptTooLarge) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "transcript exceeds export limits"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export transcript"})
		return
	}

	var data []byte
	contentType := "text/csv; charset=utf-8"
	if format == "pdf" {
		contentType = "application/pdf"
		data, err = transcript.PDF(entries)
	} else {
		data, err = transcript.CSV(entries)
	}
	if errors.Is(err, transcript.ErrTooManyPages) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "transcript exceeds PDF page limit"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export transcript"})
		return
	}
	if ctx.Err() != nil {
		return
	}
	operatorID, _ := getOperatorID(c)
	auditCtx, cancelAudit := context.WithTimeout(ctx, 5*time.Second)
	defer cancelAudit()
	if err := s.recordAuditLog(auditCtx, c, operatorID, "operator", "session_transcript_exported", "session", id,
		gin.H{"format": format, "message_count": len(entries)}); err != nil {
		s.logger.Error("failed to record transcript export")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "transcript audit unavailable"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=session-%d.%s", id, format))
	c.Data(http.StatusOK, contentType, data)
}
