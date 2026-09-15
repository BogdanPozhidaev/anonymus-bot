package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
)

// handleGetMessageMedia отдаёт файл (фото/голос), привязанный к сообщению,
// после проверки, что текущий оператор имеет право видеть эту сессию.
// Файл никогда не отдаётся напрямую по URL с чужим доступом — доступ
// проверяется на уровне сессии, к которой относится сообщение.
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

	if message.FileID == nil {
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

	path := filepath.Join(dir, *message.FileID+ext)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "media file no longer available"})
		return
	}

	c.Header("Content-Type", contentType)
	c.File(path)
}
