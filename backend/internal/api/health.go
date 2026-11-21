package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// handleHealth godoc
// @Summary      Report PostgreSQL and Redis readiness
// @Tags         health
// @Produce      json
// @Success      200 {object} map[string]string
// @Failure      503 {object} map[string]string
// @Router       /api/health [get]
func (s *Server) handleHealth(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	dbStatus := "ok"
	if err := s.pingDB(ctx); err != nil {
		dbStatus = "error"
	}

	redisStatus := "ok"
	if err := s.pingRedis(ctx); err != nil {
		redisStatus = "error"
	}

	overallStatus := http.StatusOK
	status := "ok"
	if dbStatus != "ok" || redisStatus != "ok" {
		overallStatus = http.StatusServiceUnavailable
		status = "error"
	}

	c.JSON(overallStatus, gin.H{
		"status":   status,
		"postgres": dbStatus,
		"redis":    redisStatus,
	})
}
