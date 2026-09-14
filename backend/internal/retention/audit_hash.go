package retention

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

// piiFieldsInPayload — ключи в JSON-payload audit_log, которые считаются
// потенциальным PII и подлежат хэшированию при срабатывании retention.
// Список открытый — расширяется по мере появления новых action'ов,
// логирующих персональные данные пользователей.
var piiFieldsInPayload = []string{
	"username", "first_name", "telegram_id", "email", "phone",
}

type AuditLogHasher struct {
	auditLogRepo *repository.AuditLogRepository
	logger       *slog.Logger
}

func NewAuditLogHasher(auditLogRepo *repository.AuditLogRepository, logger *slog.Logger) *AuditLogHasher {
	return &AuditLogHasher{auditLogRepo: auditLogRepo, logger: logger}
}

// HashPIIInPayload заменяет значения полей из piiFieldsInPayload на их
// SHA-256 хэш прямо внутри JSON-структуры payload, не удаляя сам факт
// наличия поля — событие остаётся в истории, но исходное значение
// становится невосстановимым. Возвращает изменённый payload и флаг,
// были ли реально внесены изменения.
func HashPIIInPayload(payload []byte) ([]byte, bool, error) {
	if len(payload) == 0 {
		return payload, false, nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		// Payload не является объектом (например, массив или примитив) —
		// не пытаемся его интерпретировать, оставляем как есть.
		return payload, false, nil
	}

	changed := false
	for _, field := range piiFieldsInPayload {
		if value, exists := data[field]; exists && value != nil {
			data[field] = hashValue(fmt.Sprintf("%v", value))
			changed = true
		}
	}

	if !changed {
		return payload, false, nil
	}

	newPayload, err := json.Marshal(data)
	if err != nil {
		return nil, false, fmt.Errorf("failed to marshal hashed payload: %w", err)
	}

	return newPayload, true, nil
}

func hashValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// HashPIIForUser находит все audit_log записи, где actor_id или target_id
// совпадает с указанным пользователем, и хэширует PII-поля в их payload.
// Вызывается из PIIAnonymizer в момент анонимизации пользователя.
func (h *AuditLogHasher) HashPIIForUser(ctx context.Context, userID int64) error {
	entries, err := h.auditLogRepo.List(ctx, repository.AuditLogFilter{ActorID: &userID})
	if err != nil {
		return fmt.Errorf("failed to list audit log entries for user: %w", err)
	}

	for _, entry := range entries {
		newPayload, changed, err := HashPIIInPayload(entry.Payload)
		if err != nil {
			h.logger.Error("failed to hash payload", "error", err, "audit_log_id", entry.ID)
			continue
		}

		if !changed {
			continue
		}

		if err := h.auditLogRepo.UpdatePayload(ctx, entry.ID, newPayload); err != nil {
			h.logger.Error("failed to update audit log payload", "error", err, "audit_log_id", entry.ID)
		}
	}

	return nil
}
