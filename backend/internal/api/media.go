package api

import (
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

// handleGetMessageMedia serves a message's media after checking access to its session.
func (s *Server) handleGetMessageMedia(c *gin.Context) {
	messageID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
		return
	}

	ctx := c.Request.Context()

	message, err := s.messageRepo.GetByID(ctx, messageID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
		return
	}

	session, err := s.sessionRepo.GetByID(ctx, message.SessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if !s.canAccessSession(c, session) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if message.FileID == nil || !models.ValidMediaFileID(*message.FileID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "no media attached to this message"})
		return
	}

	var dir, ext, contentType string
	switch message.ContentType {
	case models.ContentTypePhoto:
		dir, ext, contentType = s.photosDir, ".jpg", "image/jpeg"
	case models.ContentTypeVoice:
		dir, ext, contentType = s.voiceDir, ".ogg", "audio/ogg"
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported content type for media"})
		return
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "media file no longer available"})
		return
	}
	defer func() { _ = root.Close() }()
	file, err := root.Open(*message.FileID + ext)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "media file no longer available"})
		return
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		c.JSON(http.StatusNotFound, gin.H{"error": "media file no longer available"})
		return
	}
	c.Header("Content-Type", contentType)
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
	switch c.Writer.Status() {
	case http.StatusOK, http.StatusPartialContent, http.StatusNotModified:
		operatorID, _ := getOperatorID(c)
		s.writeAuditLog(c, operatorID, "operator", "message_media_accessed", "message", messageID, nil)
	}
}
