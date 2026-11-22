package api

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

type operatorListItem struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Login       string  `json:"login"`
	Email       *string `json:"email"`
	Role        string  `json:"role"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
	LastLoginAt *string `json:"last_login_at"`
}

type createOperatorRequest struct {
	Name     string `json:"name" binding:"required"`
	Login    string `json:"login" binding:"required"`
	Email    string `json:"email"`
	Role     string `json:"role" binding:"required,oneof=admin operator"`
	Language string `json:"language"`
}

type temporaryPasswordResponse struct {
	ID                int64  `json:"id"`
	Login             string `json:"login"`
	TemporaryPassword string `json:"temporary_password"`
}

type updateOperatorRequest struct {
	Name   *string `json:"name"`
	Role   *string `json:"role" binding:"omitempty,oneof=admin operator"`
	Status *string `json:"status" binding:"omitempty,oneof=active suspended disabled"`
}

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

	if operator.MustChangePassword {
		c.JSON(http.StatusNotFound, gin.H{"error": "operator not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":       operator.ID,
		"name":     operator.Name,
		"role":     operator.Role,
		"status":   operator.Status,
		"language": operator.Language,
	})
}

// handleListOperators godoc
// @Summary      Список операторов
// @Description  Доступно только администраторам
// @Tags         operators
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} map[string]interface{}
// @Failure      403 {object} map[string]string
// @Param        limit query int false "Page size (1–200, default 50)"
// @Param        offset query int false "Number of operators to skip"
// @Router       /api/operators [get]
func (s *Server) handleListOperators(c *gin.Context) {
	page, ok := readPage(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	operators, err := s.operatorRepo.ListPage(ctx, page.Limit+1, page.Offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list operators"})
		return
	}

	operators, hasMore := trimPage(operators, page.Limit)
	items := make([]operatorListItem, 0, len(operators))
	for _, op := range operators {
		item := operatorListItem{
			ID:        op.ID,
			Name:      op.Name,
			Login:     op.Login,
			Email:     op.Email,
			Role:      op.Role,
			Status:    op.Status,
			CreatedAt: op.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if op.LastLoginAt != nil {
			formatted := op.LastLoginAt.Format("2006-01-02T15:04:05Z07:00")
			item.LastLoginAt = &formatted
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{"operators": items, "has_more": hasMore})
}

func (s *Server) handleCreateOperator(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req createOperatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	tempPassword := rand.Text()
	passwordHash, err := auth.HashPassword(tempPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	language := req.Language
	if language == "" {
		language = "ru"
	}

	creatorID, _ := getOperatorID(c)

	newOperator := &models.Operator{
		Name:               req.Name,
		Login:              req.Login,
		Role:               req.Role,
		Status:             models.OperatorStatusInvited,
		MustChangePassword: true,
		PasswordHash:       &passwordHash,
		Language:           language,
		CreatedBy:          &creatorID,
	}
	if req.Email != "" {
		newOperator.Email = &req.Email
	}

	ctx := c.Request.Context()
	if err := s.operatorRepo.Create(ctx, newOperator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create operator"})
		return
	}

	s.writeAuditLog(c, creatorID, "operator", "operator_created", "operator", newOperator.ID, gin.H{"login": req.Login, "role": req.Role})

	c.JSON(http.StatusCreated, temporaryPasswordResponse{
		ID:                newOperator.ID,
		Login:             newOperator.Login,
		TemporaryPassword: tempPassword,
	})
}

// handleResetOperatorPassword godoc
// @Summary Issue a temporary password for another operator
// @Tags operators
// @Produce json
// @Security BearerAuth
// @Param id path int true "Operator ID"
// @Success 200 {object} temporaryPasswordResponse
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Router /api/operators/{id}/password/reset [post]
func (s *Server) handleResetOperatorPassword(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid operator id"})
		return
	}
	adminID := c.GetInt64(contextKeyOperatorID)
	if targetID == adminID {
		c.JSON(http.StatusForbidden, gin.H{"error": "use the self-service password change for your own account"})
		return
	}
	ctx := c.Request.Context()
	target, err := s.operatorRepo.GetByID(ctx, targetID)
	if errors.Is(err, repository.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "operator not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "could not load operator"})
		return
	}
	if target.Status != models.OperatorStatusActive && target.Status != models.OperatorStatusInvited {
		c.JSON(http.StatusConflict, gin.H{"error": "operator is not eligible for password reset"})
		return
	}
	temporary := rand.Text()
	hash, err := auth.HashPassword(temporary)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not reset password"})
		return
	}
	changed, err := s.operatorRepo.ResetPassword(ctx, adminID, c.GetInt64(contextKeyAuthVersion), targetID, target.AuthVersion, hash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not reset password"})
		return
	}
	if !changed {
		c.JSON(http.StatusConflict, gin.H{"error": "operator or administrator state changed"})
		return
	}
	s.writeAuditLog(c, adminID, "operator", "operator_password_reset", "operator", targetID, nil)
	c.JSON(http.StatusOK, temporaryPasswordResponse{ID: targetID, Login: target.Login, TemporaryPassword: temporary})
}

func (s *Server) handleUpdateOperator(c *gin.Context) {
	operatorID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid operator id"})
		return
	}

	var req updateOperatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ctx := c.Request.Context()

	operator, err := s.operatorRepo.GetByID(ctx, operatorID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "operator not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	wasDisabled := operator.Status == models.OperatorStatusDisabled

	if err := s.operatorRepo.UpdateProfile(ctx, operator.ID, operator.AuthVersion, req.Name, req.Role, req.Status); err != nil {
		if errors.Is(err, repository.ErrLastActiveAdmin) {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot remove the last active administrator"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update operator"})
		return
	}

	// При переводе в disabled — немедленный отзыв всех активных сессий
	// и переназначение его сессий на admin (согласно разделу 16 ТЗ).
	if req.Status != nil && *req.Status == models.OperatorStatusDisabled && !wasDisabled {
		if err := s.reassignSessionsToAdmin(ctx, operator.ID); err != nil {
			s.logger.Error("failed to reassign sessions from disabled operator", "error", err, "operator_id", operator.ID)
		}
	}

	adminID, _ := getOperatorID(c)
	s.writeAuditLog(c, adminID, "operator", "operator_updated", "operator", operatorID, gin.H{"changes": req})

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// reassignSessionsToAdmin переназначает все сессии disabled-оператора
// на первого найденного активного admin. Используется как аварийная мера,
// чтобы сессии не остались без владельца.
func (s *Server) reassignSessionsToAdmin(ctx context.Context, disabledOperatorID int64) error {
	sessions, err := s.sessionRepo.ListByOperator(ctx, disabledOperatorID)
	if err != nil {
		return fmt.Errorf("failed to list sessions of disabled operator: %w", err)
	}

	adminID, err := s.findAnyActiveAdminID(ctx)
	if err != nil {
		return fmt.Errorf("failed to find active admin: %w", err)
	}

	for _, sess := range sessions {
		if err := s.sessionRepo.AssignOwner(ctx, sess.ID, adminID); err != nil {
			return fmt.Errorf("failed to reassign session %d: %w", sess.ID, err)
		}
	}

	return nil
}

func (s *Server) findAnyActiveAdminID(ctx context.Context) (int64, error) {
	admins, err := s.operatorRepo.ListAll(ctx)
	if err != nil {
		return 0, err
	}

	for _, op := range admins {
		if op.Role == models.OperatorRoleAdmin && op.Status == models.OperatorStatusActive {
			return op.ID, nil
		}
	}

	return 0, errors.New("no active admin found")
}
