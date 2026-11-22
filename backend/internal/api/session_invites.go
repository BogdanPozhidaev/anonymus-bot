package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/fastcheck/anonymus_bot/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) handleIssueInvite(c *gin.Context) {
	var req struct {
		Role string `json:"role" binding:"required,oneof=client executor"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invitation role"})
		return
	}
	raw := make([]byte, service.InviteTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	hash := sha256.Sum256(raw)
	s.manageInvite(c, req.Role, hash[:], base64.RawURLEncoding.EncodeToString(raw))
}

func (s *Server) handleRevokeInvite(c *gin.Context) {
	role := c.Param("role")
	if role != "client" && role != "executor" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invitation role"})
		return
	}
	s.manageInvite(c, role, nil, "")
}

func (s *Server) manageInvite(c *gin.Context, role string, hash []byte, token string) {
	id, ok := readSessionID(c)
	if !ok {
		return
	}
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
		return
	}
	operatorID, ok := getOperatorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	operatorRole, _ := getOperatorRole(c)
	expiry, err := s.sessionRepo.ManageInvite(c.Request.Context(), id, role, operatorID, operatorRole == models.OperatorRoleAdmin, hash)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
	case errors.Is(err, repository.ErrInviteForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	case errors.Is(err, repository.ErrInviteUnavailable):
		c.JSON(http.StatusConflict, gin.H{"error": "invitation unavailable"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	case hash == nil:
		c.Status(http.StatusNoContent)
	default:
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusCreated, gin.H{"token": token, "expires_at": expiry})
	}
}
