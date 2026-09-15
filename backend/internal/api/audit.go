package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

type listAuditLogQuery struct {
	ActorID    *int64  `form:"actor_id"`
	Action     *string `form:"action"`
	TargetType *string `form:"target_type"`
	DateFrom   *string `form:"date_from"`
	DateTo     *string `form:"date_to"`
	Format     string  `form:"format"` // "json" (default) | "csv"
}

// writeAuditLog — единая точка записи событий аудита. Ошибка записи
// логируется, но не прерывает основной поток запроса — отказ аудита
// не должен блокировать реальное действие пользователя.
func (s *Server) writeAuditLog(ctx context.Context, actorID int64, action, targetType string, targetID int64, payload interface{}) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error("failed to marshal audit log payload", "error", err, "action", action)
		return
	}

	entry := &models.AuditLog{
		ActorID:    &actorID,
		Action:     action,
		TargetType: &targetType,
		TargetID:   &targetID,
		Payload:    payloadJSON,
	}

	if err := s.auditLogRepo.Create(ctx, entry); err != nil {
		s.logger.Error("failed to write audit log", "error", err, "action", action)
	}
}

// handleListAuditLog godoc
// @Summary      Список записей аудита
// @Description  Поддерживает фильтры и экспорт в CSV через ?format=csv
// @Tags         audit
// @Produce      json
// @Security     BearerAuth
// @Param        actor_id query int false "ID актора"
// @Param        action query string false "Тип действия"
// @Param        target_type query string false "Тип цели"
// @Param        format query string false "json или csv"
// @Success      200 {object} map[string]interface{}
// @Router       /api/audit-log [get]
func (s *Server) handleListAuditLog(c *gin.Context) {
	var query listAuditLogQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	filter := repository.AuditLogFilter{
		ActorID:    query.ActorID,
		Action:     query.Action,
		TargetType: query.TargetType,
		DateFrom:   query.DateFrom,
		DateTo:     query.DateTo,
	}

	ctx := c.Request.Context()

	logs, err := s.auditLogRepo.List(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list audit log"})
		return
	}

	if query.Format == "csv" {
		s.writeAuditLogCSV(c, logs)
		return
	}

	c.JSON(http.StatusOK, gin.H{"entries": logs})
}

func (s *Server) writeAuditLogCSV(c *gin.Context, logs []*models.AuditLog) {
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=audit_log.csv")

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	_ = writer.Write([]string{"id", "timestamp", "actor_id", "actor_role", "action", "target_type", "target_id", "ip_address"})

	for _, entry := range logs {
		row := []string{
			strconv.FormatInt(entry.ID, 10),
			entry.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			formatNullableInt64(entry.ActorID),
			formatNullableString(entry.ActorRole),
			entry.Action,
			formatNullableString(entry.TargetType),
			formatNullableInt64(entry.TargetID),
			formatNullableString(entry.IPAddress),
		}
		_ = writer.Write(row)
	}
}

func formatNullableInt64(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func formatNullableString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
