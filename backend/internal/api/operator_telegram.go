package api

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"net/http"
	"strings"

	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type telegramLinkClaim struct {
	Code       string `json:"code" binding:"required,max=64"`
	TelegramID int64  `json:"telegram_id" binding:"required,gt=0"`
}

func telegramLinkDigest(value string) []byte {
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(value))))
	return sum[:]
}

func (s *Server) handleTelegramLinkStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	operator, err := s.operatorRepo.GetByID(c.Request.Context(), c.GetInt64(contextKeyOperatorID))
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "could not load operator"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"linked": operator.TelegramID != nil})
}

func (s *Server) handleBeginTelegramLink(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	code := rand.Text()
	err := s.operatorRepo.BeginTelegramLink(c.Request.Context(), c.GetInt64(contextKeyOperatorID),
		c.GetInt64(contextKeyAuthVersion), telegramLinkDigest(code))
	if err != nil {
		telegramLinkError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": code})
}

func (s *Server) handleClaimTelegramLink(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req telegramLinkClaim
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	confirmation := rand.Text()
	applied, err := s.operatorRepo.ClaimTelegramLink(c.Request.Context(), req.TelegramID,
		telegramLinkDigest(req.Code), telegramLinkDigest(confirmation))
	if err != nil {
		telegramLinkError(c, err)
		return
	}
	if !applied {
		c.JSON(http.StatusNotFound, gin.H{"error": "link challenge not available"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"confirmation_code": confirmation})
}

func (s *Server) handleConfirmTelegramLink(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req struct {
		Code string `json:"code" binding:"required,max=64"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	id := c.GetInt64(contextKeyOperatorID)
	err := s.operatorRepo.ConfirmTelegramLink(c.Request.Context(), id, c.GetInt64(contextKeyAuthVersion), telegramLinkDigest(req.Code))
	if err != nil {
		telegramLinkError(c, err)
		return
	}
	s.writeAuditLog(c, id, "operator", "operator_telegram_linked", "operator", id, nil)
	c.Status(http.StatusNoContent)
}

func telegramLinkError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		c.JSON(http.StatusNotFound, gin.H{"error": "link challenge not available"})
	case errors.Is(err, repository.ErrTelegramAlreadyLinked):
		c.JSON(http.StatusConflict, gin.H{"error": "telegram account is already linked"})
	default:
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "telegram linking temporarily unavailable"})
	}
}
