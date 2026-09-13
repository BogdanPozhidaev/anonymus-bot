package moderation

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type CheckResult struct {
	Blocked        bool
	Reason         string
	ViolationCount int64
	ShouldPause    bool
}

type Service struct {
	redisClient    *redis.Client
	violationTTL   time.Duration
	pauseThreshold int64
}

func NewService(redisClient *redis.Client) *Service {
	return &Service{
		redisClient:    redisClient,
		violationTTL:   24 * time.Hour,
		pauseThreshold: 3,
	}
}

// Check проверяет текст на нарушения модерации. Если нарушение найдено,
// увеличивает счётчик срабатываний пользователя в Redis и сообщает,
// нужно ли поставить сессию на паузу (при достижении порога).
func (s *Service) Check(ctx context.Context, userID int64, text string) (CheckResult, error) {
	if blocked, reason := ContainsContactInfo(text); blocked {
		return s.recordViolation(ctx, userID, reason)
	}

	if blocked, reason := ContainsSuspiciousPhrase(text); blocked {
		return s.recordViolation(ctx, userID, "suspicious_phrase:"+reason)
	}

	return CheckResult{Blocked: false}, nil
}

func (s *Service) recordViolation(ctx context.Context, userID int64, reason string) (CheckResult, error) {
	key := violationKey(userID)

	count, err := s.redisClient.Incr(ctx, key).Result()
	if err != nil {
		return CheckResult{}, fmt.Errorf("failed to increment violation counter: %w", err)
	}

	// TTL устанавливается только при первом нарушении (когда счётчик стал 1),
	// чтобы повторные Incr не продлевали окно бесконечно.
	if count == 1 {
		if err := s.redisClient.Expire(ctx, key, s.violationTTL).Err(); err != nil {
			return CheckResult{}, fmt.Errorf("failed to set violation counter TTL: %w", err)
		}
	}

	return CheckResult{
		Blocked:        true,
		Reason:         reason,
		ViolationCount: count,
		ShouldPause:    count >= s.pauseThreshold,
	}, nil
}

func violationKey(userID int64) string {
	return fmt.Sprintf("moderation:violations:%d", userID)
}
