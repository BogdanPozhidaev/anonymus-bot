package auth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const authWindow = 5 * time.Minute

// Check every limit before reserving an attempt in any bucket.
const reserveAttempts = `
local retry = 0
for i = 1, #KEYS do
    if tonumber(redis.call('GET', KEYS[i]) or '0') >= tonumber(ARGV[i * 2 - 1]) then
        retry = math.max(retry, redis.call('PTTL', KEYS[i]), 1)
    end
end
if retry > 0 then
    return retry
end
for i = 1, #KEYS do
    if redis.call('INCR', KEYS[i]) == 1 then
        redis.call('PEXPIRE', KEYS[i], ARGV[i * 2])
    end
end
return 0
`

var loginAttempts = redis.NewScript(reserveAttempts)
var totpAttempts = redis.NewScript(`
local pending = table.remove(KEYS, 1)
if redis.call('GET', pending) ~= ARGV[5] then
    return -1
end
local ttl = redis.call('PTTL', pending)
if ttl <= 0 then
    return -1
end
ARGV[2] = ttl
` + reserveAttempts)

func ReserveLoginAttempt(ctx context.Context, client *redis.Client, login, ip string) (time.Duration, error) {
	pair := sha256.Sum256([]byte(login + "\x00" + ip))
	address := sha256.Sum256([]byte(ip))
	keys := []string{fmt.Sprintf("auth_attempt:login_pair:%x", pair), fmt.Sprintf("auth_attempt:login_ip:%x", address)}
	retry, err := loginAttempts.Run(ctx, client, keys, 5, authWindow.Milliseconds(), 30, authWindow.Milliseconds()).Int64()
	return time.Duration(retry) * time.Millisecond, err
}

func (s *PendingAuthService) ReserveAttempt(ctx context.Context, token string, operatorID, authVersion int64) (time.Duration, error) {
	digest := sha256.Sum256([]byte(token))
	keys := []string{pendingAuthKey(token), fmt.Sprintf("auth_attempt:totp_pending:%x", digest), fmt.Sprintf("auth_attempt:totp_operator:%d", operatorID)}
	retry, err := totpAttempts.Run(ctx, s.redisClient, keys, 5, 0, 10, authWindow.Milliseconds(), pendingAuthValue(operatorID, authVersion)).Int64()
	if err == nil && retry < 0 {
		return 0, redis.Nil
	}
	return time.Duration(retry) * time.Millisecond, err
}
