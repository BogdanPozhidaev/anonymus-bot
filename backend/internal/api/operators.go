package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

func (s *Server) handleGetOperatorByTelegramID(c *gin.Context) {
	telegramIDStr := c.Param("telegramID")
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid telegram id"})
		return
	}

	operator, err := s.operatorRepo.GetByTelegramID(c.Request.Context(), telegramID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "operator not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":     operator.ID,
		"name":   operator.Name,
		"role":   operator.Role,
		"status": operator.Status,
	})
}
