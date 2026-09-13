package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type SessionData struct {
	OperatorID int64  `json:"operator_id"`
	Role       string `json:"role"`
}

type SessionService struct {
	redisClient *redis.Client
	ttl         time.Duration
}

type PendingAuthService struct {
	redisClient *redis.Client
	ttl         time.Duration
}

func NewSessionService(redisClient *redis.Client) *SessionService {
	return &SessionService{
		redisClient: redisClient,
		ttl:         24 * time.Hour,
	}
}

// Create создаёт новую сессию оператора, возвращает токен (случайный UUID),
// который передаётся клиенту и используется как ключ поиска сессии в Redis.
func (s *SessionService) Create(ctx context.Context, data SessionData) (string, error) {
	token := uuid.New().String()

	payload, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session data: %w", err)
	}

	key := sessionKey(token)
	if err := s.redisClient.Set(ctx, key, payload, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("failed to store session: %w", err)
	}

	indexKey := operatorSessionsIndexKey(data.OperatorID)
	if err := s.redisClient.SAdd(ctx, indexKey, token).Err(); err != nil {
		return "", fmt.Errorf("failed to add session to operator index: %w", err)
	}
	// Индекс должен жить не дольше самой длинной возможной сессии,
	// иначе он будет расти бесконечно для активных операторов.
	if err := s.redisClient.Expire(ctx, indexKey, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("failed to set index ttl: %w", err)
	}

	return token, nil
}

// Get возвращает данные сессии по токену. Возвращает redis.Nil (через
// errors.Is), если сессия не найдена или истекла/была отозвана.
func (s *SessionService) Get(ctx context.Context, token string) (*SessionData, error) {
	key := sessionKey(token)

	payload, err := s.redisClient.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var data SessionData
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return &data, nil
}

// Revoke немедленно удаляет сессию — используется при disable оператора
// или логауте.
func (s *SessionService) Revoke(ctx context.Context, token string) error {
	key := sessionKey(token)
	return s.redisClient.Del(ctx, key).Err()
}

// RevokeAllForOperator отзывает все активные сессии конкретного оператора.
// Требует хранения обратного индекса (operator_id -> список токенов),
// который мы поддерживаем отдельно при создании сессии.
func (s *SessionService) RevokeAllForOperator(ctx context.Context, operatorID int64) error {
	indexKey := operatorSessionsIndexKey(operatorID)

	tokens, err := s.redisClient.SMembers(ctx, indexKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get operator sessions index: %w", err)
	}

	for _, token := range tokens {
		if err := s.Revoke(ctx, token); err != nil {
			return fmt.Errorf("failed to revoke session %s: %w", token, err)
		}
	}

	return s.redisClient.Del(ctx, indexKey).Err()
}

func sessionKey(token string) string {
	return fmt.Sprintf("session:%s", token)
}

func operatorSessionsIndexKey(operatorID int64) string {
	return fmt.Sprintf("operator_sessions:%d", operatorID)
}

func NewPendingAuthService(redisClient *redis.Client) *PendingAuthService {
	return &PendingAuthService{
		redisClient: redisClient,
		ttl:         5 * time.Minute,
	}
}

func (s *PendingAuthService) CreatePending(ctx context.Context, operatorID int64) (string, error) {
	token := uuid.New().String()
	key := pendingAuthKey(token)

	if err := s.redisClient.Set(ctx, key, operatorID, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("failed to store pending auth: %w", err)
	}

	return token, nil
}

func (s *PendingAuthService) GetOperatorID(ctx context.Context, token string) (int64, error) {
	key := pendingAuthKey(token)

	operatorID, err := s.redisClient.Get(ctx, key).Int64()
	if err != nil {
		return 0, err
	}

	return operatorID, nil
}

func (s *PendingAuthService) Consume(ctx context.Context, token string) error {
	key := pendingAuthKey(token)
	return s.redisClient.Del(ctx, key).Err()
}

func pendingAuthKey(token string) string {
	return fmt.Sprintf("pending_auth:%s", token)
}
