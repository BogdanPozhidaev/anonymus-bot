package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleHealth(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	dbStatus := "ok"
	if err := s.pingDB(ctx); err != nil {
		dbStatus = "error: " + err.Error()
	}

	redisStatus := "ok"
	if err := s.pingRedis(ctx); err != nil {
		redisStatus = "error: " + err.Error()
	}

	overallStatus := http.StatusOK
	if dbStatus != "ok" || redisStatus != "ok" {
		overallStatus = http.StatusServiceUnavailable
	}

	c.JSON(overallStatus, gin.H{
		"status":   "ok",
		"postgres": dbStatus,
		"redis":    redisStatus,
	})
}
