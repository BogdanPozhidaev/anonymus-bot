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
	auditHasher  *AuditLogHasher
	config       Config
	logger       *slog.Logger
}

func NewPIIAnonymizer(
	sessionRepo *repository.SessionRepository,
	userRepo *repository.UserRepository,
	auditLogRepo *repository.AuditLogRepository,
	auditHasher *AuditLogHasher,
	config Config,
	logger *slog.Logger,
) *PIIAnonymizer {
	return &PIIAnonymizer{
		sessionRepo:  sessionRepo,
		userRepo:     userRepo,
		auditLogRepo: auditLogRepo,
		auditHasher:  auditHasher,
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

	seen := make(map[int64]bool)
	for _, session := range sessions {
		result.SessionsProcessed++
		for _, userID := range collectParticipantIDs(session) {
			if seen[userID] {
				continue
			}
			seen[userID] = true
			var changed bool
			var err error
			if dryRun {
				changed, err = a.userRepo.CanAnonymizeUser(ctx, userID, threshold)
			} else {
				changed, err = a.userRepo.AnonymizeUser(ctx, userID, threshold)
			}
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("user %d anonymization: %w", userID, err))
				continue
			}
			if !changed {
				continue
			}
			if dryRun {
				result.UsersAnonymized++
				continue
			}

			if err := a.auditHasher.HashPIIForUser(ctx, userID); err != nil {
				a.logger.Error("failed to hash PII in audit log for user", "error", err, "user_id", userID)
				// Не считаем это фатальной ошибкой всей операции — сам пользователь
				// уже анонимизирован, хэширование audit log — дополнительная мера.
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
