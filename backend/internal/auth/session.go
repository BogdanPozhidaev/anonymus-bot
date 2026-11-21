package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/crypto"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type SessionData struct {
	AuthVersion int64  `json:"auth_version"`
	OperatorID  int64  `json:"operator_id"`
	Role        string `json:"role"`
}

type SessionService struct {
	redisClient *redis.Client
	ttl         time.Duration
}

type PendingAuthService struct {
	redisClient *redis.Client
	ttl         time.Duration
	secrets     *crypto.Encryptor
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
			return fmt.Errorf("failed to revoke operator session: %w", err)
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

func NewPendingAuthService(redisClient *redis.Client, secrets *crypto.Encryptor) *PendingAuthService {
	return &PendingAuthService{
		redisClient: redisClient,
		ttl:         5 * time.Minute,
		secrets:     secrets,
	}
}

const totpSecretPrefix = "totp:v1:"

func (s *PendingAuthService) EncodeTOTP(secret string) (string, error) {
	if s.secrets == nil {
		return "", errors.New("TOTP encryption unavailable")
	}
	encoded, err := s.secrets.Encrypt(secret)
	return totpSecretPrefix + encoded, err
}

func (s *PendingAuthService) DecodeTOTP(encoded string) (secret string, legacy bool, err error) {
	if !strings.HasPrefix(encoded, totpSecretPrefix) {
		return encoded, true, nil
	}
	if s.secrets == nil {
		return "", false, errors.New("TOTP encryption unavailable")
	}
	secret, err = s.secrets.Decrypt(strings.TrimPrefix(encoded, totpSecretPrefix))
	return secret, false, err
}

func (s *PendingAuthService) CreateEnrollment(ctx context.Context, operatorID, version int64, secret string) (string, error) {
	encoded, err := s.EncodeTOTP(secret)
	if err != nil {
		return "", err
	}
	token := uuid.New().String()
	_, err = s.redisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, pendingAuthKey(token), pendingAuthValue(operatorID, version), s.ttl)
		pipe.Set(ctx, pendingEnrollmentKey(token), encoded, s.ttl)
		return nil
	})
	return token, err
}

func (s *PendingAuthService) Enrollment(ctx context.Context, token string) (string, error) {
	return s.redisClient.Get(ctx, pendingEnrollmentKey(token)).Result()
}

func (s *PendingAuthService) CreatePending(ctx context.Context, operatorID, authVersion int64) (string, error) {
	token := uuid.New().String()
	key := pendingAuthKey(token)

	if err := s.redisClient.Set(ctx, key, pendingAuthValue(operatorID, authVersion), s.ttl).Err(); err != nil {
		return "", fmt.Errorf("failed to store pending auth: %w", err)
	}

	return token, nil
}

func pendingAuthValue(operatorID, authVersion int64) string {
	return fmt.Sprintf("%d:%d", operatorID, authVersion)
}

func (s *PendingAuthService) GetState(ctx context.Context, token string) (int64, int64, error) {
	value, err := s.redisClient.Get(ctx, pendingAuthKey(token)).Result()
	if err != nil {
		return 0, 0, err
	}
	idPart, versionPart, ok := strings.Cut(value, ":")
	if !ok {
		return 0, 0, redis.Nil
	}
	id, idErr := strconv.ParseInt(idPart, 10, 64)
	version, versionErr := strconv.ParseInt(versionPart, 10, 64)
	if idErr != nil || versionErr != nil || id <= 0 || version <= 0 {
		return 0, 0, redis.Nil
	}
	return id, version, nil
}

func (s *PendingAuthService) Consume(ctx context.Context, token string) error {
	deleted, err := consumePending.Run(ctx, s.redisClient, []string{pendingAuthKey(token), pendingEnrollmentKey(token)}).Int64()
	if err == nil && deleted == 0 {
		return redis.Nil
	}
	return err
}

var consumePending = redis.NewScript(`
if redis.call("DEL", KEYS[1]) == 0 then return 0 end
redis.call("DEL", KEYS[2])
return 1
`)

func pendingAuthKey(token string) string {
	return fmt.Sprintf("pending_auth:%s", token)
}

func pendingEnrollmentKey(token string) string {
	return "pending_totp:" + token
}
