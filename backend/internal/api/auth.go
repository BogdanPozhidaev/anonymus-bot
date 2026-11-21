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
	SessionToken string `json:"session_token"`
	OperatorID   int64  `json:"operator_id"`
	Role         string `json:"role"`
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
// @Summary      Логин оператора
// @Description  Первый шаг аутентификации — проверка логина/пароля
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body loginRequest true "Логин и пароль"
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
			// Одинаковый ответ на "нет такого логина" и "неверный пароль" —
			// не даём подсказки об существовании конкретного аккаунта.
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
// @Summary      Подтверждение TOTP-кода
// @Description  Второй шаг аутентификации — выдаёт session_token при успехе
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body verifyTOTPRequest true "Pending token и код"
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

	ctx := c.Request.Context()

	if err := uuid.Validate(req.PendingToken); err != nil || len(req.PendingToken) != 36 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired pending token"})
		return
	}
	operatorID, authVersion, err := s.pendingAuthService.GetState(ctx, req.PendingToken)
	if errors.Is(err, redis.Nil) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired pending token"})
		return
	}
	if rejectAuthThrottle(c, 0, err) {
		return
	}
	retry, err := s.pendingAuthService.ReserveAttempt(ctx, req.PendingToken, operatorID, authVersion)
	if errors.Is(err, redis.Nil) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired pending token"})
		return
	}
	if rejectAuthThrottle(c, retry, err) {
		return
	}

	operator, err := s.operatorRepo.GetByID(ctx, operatorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if !operatorCanAuthenticate(operator) || operator.AuthVersion != authVersion {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired pending token"})
		return
	}

	enrollment := operatorNeedsTOTPEnrollment(operator)
	var stored string
	if enrollment {
		stored, err = s.pendingAuthService.Enrollment(ctx, req.PendingToken)
		if errors.Is(err, redis.Nil) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired pending token"})
			return
		}
		if rejectAuthThrottle(c, 0, err) {
			return
		}
	} else {
		stored = *operator.TOTPSecret
	}
	secret, legacy, err := s.pendingAuthService.DecodeTOTP(stored)
	if err != nil || (enrollment && legacy) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authentication temporarily unavailable"})
		return
	}

	if !auth.VerifyTOTPCode(secret, req.Code) {
		s.writeAuditLog(c, operator.ID, "operator", "operator_login_failed", "operator", operator.ID, nil)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid 2fa code"})
		return
	}

	err = s.pendingAuthService.Consume(ctx, req.PendingToken)
	if errors.Is(err, redis.Nil) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired pending token"})
		return
	}
	if rejectAuthThrottle(c, 0, err) {
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
	if err := s.operatorRepo.RecordVerifiedLogin(ctx, operator, confirmedSecret, enrollment); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusConflict, gin.H{"error": "operator state changed, sign in again"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authentication temporarily unavailable"})
		return
	}

	sessionToken, err := s.sessionService.Create(ctx, auth.SessionData{
		AuthVersion: operator.AuthVersion,
		OperatorID:  operator.ID,
		Role:        operator.Role,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	c.Set(contextKeyRole, operator.Role)
	s.writeAuditLog(c, operator.ID, "operator", "operator_login", "operator", operator.ID, nil)
	c.JSON(http.StatusOK, verifyTOTPResponse{
		SessionToken: sessionToken,
		OperatorID:   operator.ID,
		Role:         operator.Role,
	})
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
