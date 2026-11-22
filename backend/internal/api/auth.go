package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

type loginRequest struct {
	Login    string `json:"login" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	Status       string `json:"status"` // "totp_required" | "totp_setup_required" | "ok"
	PendingToken string `json:"pending_token,omitempty"`
	SessionToken string `json:"session_token,omitempty"`
	TOTPSecret   string `json:"totp_secret,omitempty"`
	TOTPQRUrl    string `json:"totp_qr_url,omitempty"`
}

type verifyTOTPRequest struct {
	PendingToken string `json:"pending_token" binding:"required"`
	Code         string `json:"code" binding:"required"`
}

type verifyTOTPResponse struct {
	SessionToken           string `json:"session_token"`
	OperatorID             int64  `json:"operator_id"`
	Role                   string `json:"role"`
	PasswordChangeRequired bool   `json:"password_change_required,omitempty"`
}

// This dummy bcrypt hash uses the same cost as operator passwords.
const dummyOperatorPasswordHash = "$2a$12$....................................................."

func rejectAuthThrottle(c *gin.Context, retry time.Duration, err error) bool {
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authentication temporarily unavailable"})
		return true
	}
	if retry > 0 {
		c.Header("Retry-After", strconv.FormatInt(int64((retry+time.Second-1)/time.Second), 10))
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many authentication attempts"})
		return true
	}
	return false
}

func operatorCanAuthenticate(operator *models.Operator) bool {
	return operator.Status == models.OperatorStatusActive || operator.Status == models.OperatorStatusInvited
}

func operatorNeedsTOTPEnrollment(operator *models.Operator) bool {
	return operator.TOTPSecret == nil || operator.Status == models.OperatorStatusInvited
}

// handleLogin godoc
// @Summary      Sign in as an operator
// @Description  The first authentication step verifies the username and password
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body loginRequest true "Username and password"
// @Success      200 {object} loginResponse
// @Failure      401 {object} map[string]string
// @Router       /api/auth/login [post]
func (s *Server) handleLogin(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	retry, err := auth.ReserveLoginAttempt(ctx, s.redisClient, req.Login, c.ClientIP())
	if rejectAuthThrottle(c, retry, err) {
		return
	}

	operator, err := s.operatorRepo.GetByLogin(ctx, req.Login)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			auth.VerifyPassword(dummyOperatorPasswordHash, req.Password)
			// Use the same response for unknown usernames and incorrect passwords.
			s.writeAuditLog(c, 0, "unknown", "operator_login_failed", "", 0, nil)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	hash := dummyOperatorPasswordHash
	if operator.PasswordHash != nil {
		hash = *operator.PasswordHash
	}
	passwordMatches := auth.VerifyPassword(hash, req.Password)
	if !passwordMatches || operator.PasswordHash == nil || !operatorCanAuthenticate(operator) {
		s.writeAuditLog(c, operator.ID, "operator", "operator_login_failed", "operator", operator.ID, nil)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if operatorNeedsTOTPEnrollment(operator) {
		secret, qrURL, err := auth.GenerateTOTPSecret(operator.Login, "AnonymusBot")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate 2fa secret"})
			return
		}

		pendingToken, err := s.pendingAuthService.CreateEnrollment(ctx, operator.ID, operator.AuthVersion, secret)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create pending auth"})
			return
		}

		c.JSON(http.StatusOK, loginResponse{
			Status:       "totp_setup_required",
			PendingToken: pendingToken,
			TOTPSecret:   secret,
			TOTPQRUrl:    qrURL,
		})
		return
	}

	pendingToken, err := s.pendingAuthService.CreatePending(ctx, operator.ID, operator.AuthVersion)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create pending auth"})
		return
	}

	c.JSON(http.StatusOK, loginResponse{
		Status:       "totp_required",
		PendingToken: pendingToken,
	})
}

// handleVerifyTOTP godoc
// @Summary      Verify a TOTP code
// @Description  Successful verification returns a session token for an enrolled operator
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body verifyTOTPRequest true "Pending token and TOTP code"
// @Success      200 {object} verifyTOTPResponse
// @Failure      401 {object} map[string]string
// @Router       /api/auth/totp/verify [post]
func (s *Server) handleVerifyTOTP(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req verifyTOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	operator, recovery := s.pendingOperator(c, req.PendingToken)
	if operator == nil {
		return
	}
	ctx := c.Request.Context()
	enrollment := operatorNeedsTOTPEnrollment(operator)
	if enrollment && recovery {
		invalidPending(c)
		return
	}
	var stored string
	if enrollment || recovery {
		material, err := s.pendingAuthService.Enrollment(ctx, req.PendingToken)
		if errors.Is(err, redis.Nil) {
			invalidPending(c)
			return
		}
		if rejectAuthThrottle(c, 0, err) {
			return
		}
		stored = material
	} else {
		stored = *operator.TOTPSecret
	}
	secret, legacy, err := s.pendingAuthService.DecodeTOTP(stored)
	if err != nil || ((enrollment || recovery) && legacy) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authentication temporarily unavailable"})
		return
	}

	if !auth.VerifyTOTPCode(secret, req.Code) {
		s.writeAuditLog(c, operator.ID, "operator", "operator_login_failed", "operator", operator.ID, nil)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid 2fa code"})
		return
	}

	if !s.consumePending(c, req.PendingToken) {
		return
	}

	var confirmedSecret *string
	if enrollment {
		confirmedSecret = &stored
	} else if legacy {
		encoded, err := s.pendingAuthService.EncodeTOTP(secret)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authentication temporarily unavailable"})
			return
		}
		confirmedSecret = &encoded
	}
	if recovery {
		operator.AuthVersion, err = s.operatorRepo.CompleteTOTPRecovery(ctx, operator.ID, operator.AuthVersion, stored)
	} else {
		err = s.operatorRepo.RecordVerifiedLogin(ctx, operator, confirmedSecret, enrollment)
	}
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusConflict, gin.H{"error": "operator state changed, sign in again"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authentication temporarily unavailable"})
		return
	}

	c.Set(contextKeyRole, operator.Role)
	if recovery {
		s.writeAuditLog(c, operator.ID, "operator", "operator_totp_recovered", "operator", operator.ID, nil)
	}
	data := auth.SessionData{
		AuthVersion: operator.AuthVersion,
		OperatorID:  operator.ID,
		Role:        operator.Role,
	}
	var sessionToken string
	if operator.MustChangePassword {
		sessionToken, err = s.sessionService.CreatePasswordChange(ctx, data)
	} else {
		sessionToken, err = s.sessionService.Create(ctx, data)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	action := "operator_login"
	if operator.MustChangePassword {
		action = "operator_temporary_password_verified"
	}
	s.writeAuditLog(c, operator.ID, "operator", action, "operator", operator.ID, nil)
	c.JSON(http.StatusOK, verifyTOTPResponse{
		SessionToken:           sessionToken,
		OperatorID:             operator.ID,
		Role:                   operator.Role,
		PasswordChangeRequired: operator.MustChangePassword,
	})
}

func invalidPending(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired pending token"})
}

func (s *Server) pendingOperator(c *gin.Context, token string) (*models.Operator, bool) {
	if len(token) != 36 || uuid.Validate(token) != nil {
		invalidPending(c)
		return nil, false
	}
	ctx := c.Request.Context()
	id, version, recovery, err := s.pendingAuthService.GetState(ctx, token)
	if errors.Is(err, redis.Nil) {
		invalidPending(c)
		return nil, false
	}
	if rejectAuthThrottle(c, 0, err) {
		return nil, false
	}
	retry, err := s.pendingAuthService.ReserveAttempt(ctx, token, id, version, recovery)
	if errors.Is(err, redis.Nil) {
		invalidPending(c)
		return nil, false
	}
	if rejectAuthThrottle(c, retry, err) {
		return nil, false
	}
	operator, err := s.operatorRepo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return nil, false
	}
	if !operatorCanAuthenticate(operator) || operator.AuthVersion != version {
		invalidPending(c)
		return nil, false
	}
	return operator, recovery
}

func (s *Server) consumePending(c *gin.Context, token string) bool {
	err := s.pendingAuthService.Consume(c.Request.Context(), token)
	if errors.Is(err, redis.Nil) {
		invalidPending(c)
		return false
	}
	return !rejectAuthThrottle(c, 0, err)
}

func (s *Server) handleLogout(c *gin.Context) {
	token := extractSessionToken(c)
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no session token provided"})
		return
	}

	if err := s.sessionService.Revoke(c.Request.Context(), token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke session"})
		return
	}

	operatorID, _ := getOperatorID(c)
	s.writeAuditLog(c, operatorID, "operator", "operator_logout", "operator", operatorID, nil)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func extractSessionToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if len(header) > len(prefix) && header[:len(prefix)] == prefix {
		return header[len(prefix):]
	}
	return ""
}
