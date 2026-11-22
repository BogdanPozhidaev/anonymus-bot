package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// Create stores an operator session and returns its random UUID token.
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

	return token, nil
}

// Get returns the session identified by the token, or redis.Nil if it is missing.
func (s *SessionService) Get(ctx context.Context, token string) (*SessionData, error) {
	return s.get(ctx, sessionKey(token))
}

const passwordChangeTTL = 5 * time.Minute

func (s *SessionService) CreatePasswordChange(ctx context.Context, data SessionData) (string, error) {
	token := uuid.New().String()
	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	if err := s.redisClient.Set(ctx, passwordChangeKey(token), payload, passwordChangeTTL).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *SessionService) GetPasswordChange(ctx context.Context, token string) (*SessionData, error) {
	return s.get(ctx, passwordChangeKey(token))
}

func (s *SessionService) get(ctx context.Context, key string) (*SessionData, error) {
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

// Revoke removes both regular and password-change sessions for the token.
func (s *SessionService) Revoke(ctx context.Context, token string) error {
	return s.redisClient.Del(ctx, sessionKey(token), passwordChangeKey(token)).Err()
}

func sessionKey(token string) string {
	return tokenKey("session", token)
}

func passwordChangeKey(token string) string {
	return tokenKey("password_change_session", token)
}

func tokenKey(namespace, token string) string {
	digest := sha256.Sum256([]byte(token))
	return namespace + ":sha256:" + hex.EncodeToString(digest[:])
}

func NewPendingAuthService(redisClient *redis.Client, secrets *crypto.Encryptor) *PendingAuthService {
	return &PendingAuthService{
		redisClient: redisClient,
		ttl:         5 * time.Minute,
		secrets:     secrets,
	}
}

func (s *PendingAuthService) EncodeTOTP(secret string) (string, error) {
	if s.secrets == nil {
		return "", errors.New("TOTP encryption unavailable")
	}
	return s.secrets.EncryptTOTP(secret)
}

func (s *PendingAuthService) DecodeTOTP(encoded string) (secret string, legacy bool, err error) {
	if s.secrets == nil {
		return "", false, errors.New("TOTP encryption unavailable")
	}
	return s.secrets.DecryptTOTP(encoded)
}

func (s *PendingAuthService) CreateEnrollment(ctx context.Context, operatorID, version int64, secret string) (string, error) {
	return s.createEnrollment(ctx, operatorID, version, secret, false)
}

func (s *PendingAuthService) CreateRecoveryEnrollment(ctx context.Context, operatorID, version int64, secret string) (string, error) {
	return s.createEnrollment(ctx, operatorID, version, secret, true)
}

func (s *PendingAuthService) createEnrollment(ctx context.Context, operatorID, version int64, secret string, recovery bool) (string, error) {
	encoded, err := s.EncodeTOTP(secret)
	if err != nil {
		return "", err
	}
	token := uuid.New().String()
	_, err = s.redisClient.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, pendingAuthKey(token), pendingAuthValue(operatorID, version, recovery), s.ttl)
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

	if err := s.redisClient.Set(ctx, key, pendingAuthValue(operatorID, authVersion, false), s.ttl).Err(); err != nil {
		return "", fmt.Errorf("failed to store pending auth: %w", err)
	}

	return token, nil
}

func pendingAuthValue(operatorID, authVersion int64, recovery bool) string {
	value := fmt.Sprintf("%d:%d", operatorID, authVersion)
	if recovery {
		return value + ":recovery"
	}
	return value
}

func (s *PendingAuthService) GetState(ctx context.Context, token string) (int64, int64, bool, error) {
	value, err := s.redisClient.Get(ctx, pendingAuthKey(token)).Result()
	if err != nil {
		return 0, 0, false, err
	}
	value, recovery := strings.CutSuffix(value, ":recovery")
	idPart, versionPart, ok := strings.Cut(value, ":")
	if !ok {
		return 0, 0, false, redis.Nil
	}
	id, idErr := strconv.ParseInt(idPart, 10, 64)
	version, versionErr := strconv.ParseInt(versionPart, 10, 64)
	if idErr != nil || versionErr != nil || id <= 0 || version <= 0 {
		return 0, 0, false, redis.Nil
	}
	return id, version, recovery, nil
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
	return tokenKey("pending_auth", token)
}

func pendingEnrollmentKey(token string) string {
	return tokenKey("pending_totp", token)
}
