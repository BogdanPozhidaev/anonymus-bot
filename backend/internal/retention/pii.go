package retention

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/models"
	"github.com/fastcheck/anonymus_bot/backend/internal/repository"
)

type PIIAnonymizer struct {
	sessionRepo  *repository.SessionRepository
	userRepo     *repository.UserRepository
	auditLogRepo *repository.AuditLogRepository
	config       Config
	logger       *slog.Logger
}

func NewPIIAnonymizer(
	sessionRepo *repository.SessionRepository,
	userRepo *repository.UserRepository,
	auditLogRepo *repository.AuditLogRepository,
	config Config,
	logger *slog.Logger,
) *PIIAnonymizer {
	return &PIIAnonymizer{
		sessionRepo:  sessionRepo,
		userRepo:     userRepo,
		auditLogRepo: auditLogRepo,
		config:       config,
		logger:       logger,
	}
}

type PIIAnonymizeResult struct {
	SessionsProcessed int
	UsersAnonymized   int
	Errors            []error
}

// Run обрабатывает все сессии, закрытые раньше PIIRetentionPeriod назад,
// и анонимизирует PII участников (клиента и исполнителя), если они ещё
// не были анонимизированы ранее. При dryRun=true никакие изменения
// в БД не производятся — только подсчёт того, что было бы сделано.
func (a *PIIAnonymizer) Run(ctx context.Context, dryRun bool) (*PIIAnonymizeResult, error) {
	threshold := time.Now().Add(-a.config.PIIRetentionPeriod)

	sessions, err := a.sessionRepo.ListClosedBefore(ctx, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to list closed sessions: %w", err)
	}

	result := &PIIAnonymizeResult{}

	for _, session := range sessions {
		result.SessionsProcessed++

		userIDs := collectParticipantIDs(session)

		for _, userID := range userIDs {
			user, err := a.userRepo.GetByID(ctx, userID)
			if err != nil {
				a.logger.Error("failed to get user for anonymization", "error", err, "user_id", userID, "session_id", session.ID)
				result.Errors = append(result.Errors, fmt.Errorf("session %d, user %d: %w", session.ID, userID, err))
				continue
			}

			// Пользователь уже анонимизирован ранее (например, участвовал
			// в другой закрытой сессии, которую уже обработали) — пропускаем.
			if user.TelegramID < 0 {
				continue
			}

			if dryRun {
				a.logger.Info("[DRY RUN] would anonymize user", "user_id", userID, "session_id", session.ID)
				result.UsersAnonymized++
				continue
			}

			if err := a.userRepo.AnonymizeUser(ctx, userID); err != nil {
				a.logger.Error("failed to anonymize user", "error", err, "user_id", userID, "session_id", session.ID)
				result.Errors = append(result.Errors, fmt.Errorf("session %d, user %d: %w", session.ID, userID, err))
				continue
			}

			a.writeAnonymizationAuditLog(ctx, userID, session.ID)
			result.UsersAnonymized++
		}
	}

	return result, nil
}

func collectParticipantIDs(session *models.Session) []int64 {
	var ids []int64
	if session.ClientUserID != nil {
		ids = append(ids, *session.ClientUserID)
	}
	if session.ExecutorUserID != nil {
		ids = append(ids, *session.ExecutorUserID)
	}
	return ids
}

func (a *PIIAnonymizer) writeAnonymizationAuditLog(ctx context.Context, userID, sessionID int64) {
	entry := &models.AuditLog{
		Action:     "pii_anonymized",
		TargetType: strPtr("user"),
		TargetID:   &userID,
		Payload:    []byte(fmt.Sprintf(`{"session_id":%d,"reason":"retention_policy"}`, sessionID)),
	}

	if err := a.auditLogRepo.Create(ctx, entry); err != nil {
		a.logger.Error("failed to write anonymization audit log", "error", err, "user_id", userID)
	}
}

func strPtr(s string) *string {
	return &s
}
