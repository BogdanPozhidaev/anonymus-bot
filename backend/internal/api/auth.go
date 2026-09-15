package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

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
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	operator, err := s.operatorRepo.GetByLogin(ctx, req.Login)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Одинаковый ответ на "нет такого логина" и "неверный пароль" —
			// не даём подсказки об существовании конкретного аккаунта.
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if operator.Status == models.OperatorStatusDisabled || operator.Status == models.OperatorStatusSuspended {
		c.JSON(http.StatusForbidden, gin.H{"error": "account is not active"})
		return
	}

	if operator.PasswordHash == nil || !auth.VerifyPassword(*operator.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if operator.TOTPSecret == nil {
		secret, qrURL, err := auth.GenerateTOTPSecret(operator.Login, "AnonymusBot")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate 2fa secret"})
			return
		}

		operator.TOTPSecret = &secret
		if err := s.operatorRepo.Update(ctx, operator); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save 2fa secret"})
			return
		}

		pendingToken, err := s.pendingAuthService.CreatePending(ctx, operator.ID)
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

	pendingToken, err := s.pendingAuthService.CreatePending(ctx, operator.ID)
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
	var req verifyTOTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	operatorID, err := s.pendingAuthService.GetOperatorID(ctx, req.PendingToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired pending token"})
		return
	}

	operator, err := s.operatorRepo.GetByID(ctx, operatorID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if operator.TOTPSecret == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "2fa not configured"})
		return
	}

	if !auth.VerifyTOTPCode(*operator.TOTPSecret, req.Code) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid 2fa code"})
		return
	}

	if err := s.pendingAuthService.Consume(ctx, req.PendingToken); err != nil {
		s.logger.Error("failed to consume pending auth token", "error", err)
	}

	sessionToken, err := s.sessionService.Create(ctx, auth.SessionData{
		OperatorID: operator.ID,
		Role:       operator.Role,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	if operator.Status == models.OperatorStatusInvited {
		operator.Status = models.OperatorStatusActive
		if err := s.operatorRepo.Update(ctx, operator); err != nil {
			s.logger.Error("failed to activate operator after first login", "error", err, "operator_id", operator.ID)
		}
	}

	c.JSON(http.StatusOK, verifyTOTPResponse{
		SessionToken: sessionToken,
		OperatorID:   operator.ID,
		Role:         operator.Role,
	})
}

type logoutRequest struct{}

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
