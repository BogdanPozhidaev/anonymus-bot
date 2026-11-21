package moderation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const violationTTL = 24 * time.Hour
const pauseThreshold = 3

type Config struct {
	disabled  map[string]bool
	autoPause bool
}

func ParseConfig(disabledRules, autoPause string) (Config, error) {
	cfg := Config{disabled: make(map[string]bool)}
	if disabledRules != "" {
		for _, rule := range strings.Split(disabledRules, ",") {
			switch key := strings.TrimSpace(rule); key {
			case "email", "messenger_link", "username", "phone", "suspicious_phrase":
				cfg.disabled[key] = true
			default:
				return Config{}, fmt.Errorf("MODERATION_DISABLED_RULES contains an unknown rule")
			}
		}
	}
	switch autoPause {
	case "true":
		cfg.autoPause = true
	case "", "false":
	default:
		return Config{}, fmt.Errorf("MODERATION_AUTO_PAUSE must be true or false")
	}
	return cfg, nil
}

type CheckResult struct {
	Blocked        bool
	Reason         string
	ViolationCount int64
	ShouldPause    bool
}

type Service struct {
	redisClient *redis.Client
	config      Config
}

func NewService(redisClient *redis.Client, config Config) *Service {
	return &Service{redisClient: redisClient, config: config}
}

func (s *Service) Check(ctx context.Context, sessionID int64, actorType string, actorID int64, text string) (CheckResult, error) {
	blocked, reason := matchContact(text, s.config.disabled)
	if !blocked && !s.config.disabled["suspicious_phrase"] {
		blocked, _ = ContainsSuspiciousPhrase(text)
		reason = "suspicious_phrase"
	}
	if !blocked {
		return CheckResult{}, nil
	}

	count, err := countViolation.Run(ctx, s.redisClient, []string{violationKey(sessionID)}, fmt.Sprintf("%s:%d", actorType, actorID), violationTTL.Milliseconds()).Int64()
	if err != nil {
		return CheckResult{}, fmt.Errorf("failed to record moderation violation: %w", err)
	}
	return CheckResult{Blocked: true, Reason: reason, ViolationCount: count, ShouldPause: s.config.autoPause && count >= pauseThreshold}, nil
}

// Keep one expiry per session window and reset all actor counters together.
var countViolation = redis.NewScript(`
local fresh = redis.call("EXISTS", KEYS[1]) == 0
local count = redis.call("HINCRBY", KEYS[1], ARGV[1], 1)
if fresh then
    redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return count
`)

func (s *Service) Reset(ctx context.Context, sessionID int64) error {
	return s.redisClient.Del(ctx, violationKey(sessionID)).Err()
}

func violationKey(sessionID int64) string {
	return fmt.Sprintf("moderation:session:%d:violations", sessionID)
}
