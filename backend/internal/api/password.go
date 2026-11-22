package api

import (
	"net/http"

	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/gin-gonic/gin"
)

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

// handleChangePassword godoc
// @Summary Change the signed-in operator's password and revoke existing sessions
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body changePasswordRequest true "Current and new passwords"
// @Success 204
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 429 {object} map[string]string
// @Router /api/auth/password [post]
func (s *Server) handleChangePassword(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req changePasswordRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := auth.ValidateNewPassword(req.NewPassword); err != nil || req.CurrentPassword == req.NewPassword {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new password does not meet the policy or matches the current password"})
		return
	}

	ctx := c.Request.Context()
	id := c.GetInt64(contextKeyOperatorID)
	version := c.GetInt64(contextKeyAuthVersion)
	retry, err := auth.ReservePasswordChangeAttempt(ctx, s.redisClient, id)
	if rejectAuthThrottle(c, retry, err) {
		return
	}
	operator, err := s.operatorRepo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "could not verify operator state"})
		return
	}
	if operator.AuthVersion != version {
		c.JSON(http.StatusConflict, gin.H{"error": "operator state changed; sign in again"})
		return
	}
	if operator.PasswordHash == nil || len(req.CurrentPassword) > 72 || !auth.VerifyPassword(*operator.PasswordHash, req.CurrentPassword) {
		c.JSON(http.StatusForbidden, gin.H{"error": "incorrect current password"})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not change password"})
		return
	}
	changed, err := s.operatorRepo.ChangePassword(ctx, id, version, hash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not change password"})
		return
	}
	if !changed {
		c.JSON(http.StatusConflict, gin.H{"error": "operator state changed; sign in again"})
		return
	}
	s.writeAuditLog(c, id, "operator", "operator_password_changed", "operator", id, nil)
	c.Status(http.StatusNoContent)
}
