package api

import (
	"net/http"

	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/gin-gonic/gin"
)

type recoverTOTPRequest struct {
	PendingToken string `json:"pending_token" binding:"required"`
	RecoveryCode string `json:"recovery_code" binding:"required,max=64"`
}

// handleRecoverTOTP godoc
// @Summary Exchange a one-time recovery code for a new TOTP enrollment
// @Tags auth
// @Accept json
// @Produce json
// @Param request body recoverTOTPRequest true "Password-verified challenge and recovery code"
// @Success 200 {object} loginResponse
// @Failure 401 {object} map[string]string
// @Failure 429 {object} map[string]string
// @Router /api/auth/totp/recover [post]
func (s *Server) handleRecoverTOTP(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req recoverTOTPRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	operator, recovery := s.pendingOperator(c, req.PendingToken)
	if operator == nil {
		return
	}
	if recovery || operatorNeedsTOTPEnrollment(operator) {
		invalidPending(c)
		return
	}
	ctx := c.Request.Context()
	if !s.consumePending(c, req.PendingToken) {
		return
	}
	secret, qrURL, err := auth.GenerateTOTPSecret(operator.Login, "AnonymusBot")
	if rejectAuthThrottle(c, 0, err) {
		return
	}
	token, err := s.pendingAuthService.CreateRecoveryEnrollment(ctx, operator.ID, operator.AuthVersion, secret)
	if rejectAuthThrottle(c, 0, err) {
		return
	}
	consumed, err := s.operatorRepo.ConsumeRecoveryCode(ctx, operator.ID, operator.AuthVersion, auth.RecoveryCodeDigest(req.RecoveryCode))
	if err != nil || !consumed {
		_ = s.pendingAuthService.Consume(ctx, token)
		if !rejectAuthThrottle(c, 0, err) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid recovery code"})
		}
		return
	}
	c.Set(contextKeyRole, operator.Role)
	s.writeAuditLog(c, operator.ID, "operator", "operator_totp_recovery_started", "operator", operator.ID, nil)
	c.JSON(http.StatusOK, loginResponse{
		Status: "totp_setup_required", PendingToken: token, TOTPSecret: secret, TOTPQRUrl: qrURL,
	})
}

type issueRecoveryCodesRequest struct {
	Password string `json:"password" binding:"required"`
	Code     string `json:"code" binding:"required,max=16"`
}

// handleIssueRecoveryCodes godoc
// @Summary Replace recovery codes after confirming the current password and TOTP
// @Tags auth
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body issueRecoveryCodesRequest true "Current password and TOTP code"
// @Success 200 {object} map[string][]string
// @Failure 403 {object} map[string]string
// @Failure 429 {object} map[string]string
// @Router /api/auth/totp/recovery-codes [post]
func (s *Server) handleIssueRecoveryCodes(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req issueRecoveryCodesRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	ctx := c.Request.Context()
	id, version := c.GetInt64(contextKeyOperatorID), c.GetInt64(contextKeyAuthVersion)
	retry, err := auth.ReservePasswordChangeAttempt(ctx, s.redisClient, id)
	if rejectAuthThrottle(c, retry, err) {
		return
	}
	operator, err := s.operatorRepo.GetByID(ctx, id)
	if rejectAuthThrottle(c, 0, err) {
		return
	}
	if operator.AuthVersion != version || operator.PasswordHash == nil || operator.TOTPSecret == nil ||
		len(req.Password) > 72 || !auth.VerifyPassword(*operator.PasswordHash, req.Password) {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid password or 2fa code"})
		return
	}
	secret, _, err := s.pendingAuthService.DecodeTOTP(*operator.TOTPSecret)
	if rejectAuthThrottle(c, 0, err) {
		return
	}
	if !auth.VerifyTOTPCode(secret, req.Code) {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid password or 2fa code"})
		return
	}
	codes, hashes := auth.GenerateRecoveryCodes()
	replaced, err := s.operatorRepo.ReplaceRecoveryCodes(ctx, id, version, hashes)
	if rejectAuthThrottle(c, 0, err) {
		return
	}
	if !replaced {
		c.JSON(http.StatusConflict, gin.H{"error": "operator state changed; sign in again"})
		return
	}
	s.writeAuditLog(c, id, "operator", "operator_recovery_codes_replaced", "operator", id, nil)
	c.JSON(http.StatusOK, gin.H{"codes": codes})
}
