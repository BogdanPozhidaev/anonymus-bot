package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/gin-gonic/gin"
)

const (
	oidcBrowserCookie = "__Host-anonymus-oidc-browser"
	oidcTicketCookie  = "__Host-anonymus-oidc-ticket"
)

func (s *Server) EnableOIDC(service *auth.OIDCService) {
	s.oidc = service
}

func (s *Server) handleOIDCStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"enabled": s.oidc != nil})
}

func oidcCookie(c *gin.Context, name, value string, ttl time.Duration) {
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: int(ttl.Seconds()),
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func (s *Server) handleOIDCStart(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()
	retry, err := auth.ReserveLoginAttempt(ctx, s.redisClient, "\x00oidc", c.ClientIP())
	if rejectAuthThrottle(c, retry, err) {
		return
	}
	url, browser, err := s.oidc.Begin(ctx)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "single sign-on temporarily unavailable"})
		return
	}
	oidcCookie(c, oidcTicketCookie, "", -time.Second)
	oidcCookie(c, oidcBrowserCookie, browser, auth.OIDCStateTTL)
	c.JSON(http.StatusOK, gin.H{"authorization_url": url})
}

func canUseOIDC(operator *models.Operator) bool {
	return operator.Status == models.OperatorStatusActive && !operator.MustChangePassword && operator.TOTPSecret != nil
}

func (s *Server) handleOIDCCallback(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	failed := func() { c.Redirect(http.StatusSeeOther, "/login?oidc=failed") }
	browser, err := c.Cookie(oidcBrowserCookie)
	if err != nil || c.Query("error") != "" {
		failed()
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	id, err := s.oidc.Verify(ctx, c.Query("state"), browser, c.Query("code"))
	if err != nil {
		failed()
		return
	}
	oidcCookie(c, oidcBrowserCookie, "", -time.Second)
	operator, err := s.operatorRepo.GetByID(ctx, id)
	if err != nil || !canUseOIDC(operator) {
		failed()
		return
	}
	ticket, err := s.oidc.CreateTicket(ctx, operator.ID, operator.AuthVersion)
	if err != nil {
		failed()
		return
	}
	oidcCookie(c, oidcTicketCookie, ticket, auth.OIDCTicketTTL)
	c.Redirect(http.StatusSeeOther, "/login?oidc=complete")
}

func (s *Server) handleOIDCComplete(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ticket, _ := c.Cookie(oidcTicketCookie)
	oidcCookie(c, oidcTicketCookie, "", -time.Second)
	ctx := c.Request.Context()
	value, err := s.oidc.UseTicket(ctx, ticket)
	if errors.Is(err, auth.ErrOIDCAuthentication) {
		invalidPending(c)
		return
	}
	if rejectAuthThrottle(c, 0, err) {
		return
	}
	operator, err := s.operatorRepo.GetByID(ctx, value.OperatorID)
	if err != nil || !canUseOIDC(operator) || operator.AuthVersion != value.AuthVersion {
		invalidPending(c)
		return
	}
	pending, err := s.pendingAuthService.CreatePending(ctx, operator.ID, operator.AuthVersion)
	if rejectAuthThrottle(c, 0, err) {
		return
	}
	c.Set(contextKeyRole, operator.Role)
	s.writeAuditLog(c, operator.ID, "operator", "operator_oidc_verified", "operator", operator.ID, nil)
	c.JSON(http.StatusOK, loginResponse{Status: "totp_required", PendingToken: pending})
}
