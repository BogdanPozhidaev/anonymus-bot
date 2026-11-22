package repository_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/fastcheck/anonymus_bot/backend/internal/auth"
	"github.com/redis/go-redis/v9"
)

func TestRedisSessionTokensAreHashed(t *testing.T) {
	ctx := t.Context()
	client := setupThrottleRedis(t)
	sessions := auth.NewSessionService(client)
	pending := auth.NewPendingAuthService(client, testEncryptor(t))
	session := auth.SessionData{OperatorID: 9, AuthVersion: 2, Role: "admin"}
	full, err := sessions.Create(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	limited, err := sessions.CreatePasswordChange(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := pending.CreateEnrollment(ctx, session.OperatorID, session.AuthVersion, "fixture-secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		namespace, token string
		ttl              time.Duration
	}{
		{"session", full, 24 * time.Hour},
		{"password_change_session", limited, 5 * time.Minute},
		{"pending_auth", challenge, 5 * time.Minute},
		{"pending_totp", challenge, 5 * time.Minute},
	} {
		digest := sha256.Sum256([]byte(item.token))
		key := item.namespace + ":sha256:" + hex.EncodeToString(digest[:])
		if ttl, err := client.TTL(ctx, key).Result(); err != nil || ttl <= 0 || ttl > item.ttl {
			t.Fatalf("%s has unexpected TTL: %v, %v", item.namespace, ttl, err)
		}
		if n, err := client.Exists(ctx, item.namespace+":"+item.token).Result(); err != nil || n != 0 {
			t.Fatalf("%s still stores plaintext credentials", item.namespace)
		}
	}
	if got, err := sessions.Get(ctx, full); err != nil || *got != session {
		t.Fatal("full session lookup failed")
	}
	if got, err := sessions.GetPasswordChange(ctx, limited); err != nil || *got != session {
		t.Fatal("password-change session lookup failed")
	}
	legacyKey := "session:" + full
	if err := client.Set(ctx, legacyKey, `{"operator_id":9,"role":"admin","auth_version":2}`, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(full))
	if _, err := sessions.Get(ctx, hex.EncodeToString(digest[:])); !errors.Is(err, redis.Nil) {
		t.Fatal("stored digest was accepted as a bearer token")
	}
	if err := sessions.Revoke(ctx, full); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Get(ctx, full); !errors.Is(err, redis.Nil) {
		t.Fatal("legacy token was accepted after revocation")
	}
	if id, version, recovery, err := pending.GetState(ctx, challenge); err != nil || id != session.OperatorID || version != session.AuthVersion || recovery {
		t.Fatal("pending state lookup failed")
	}
	stored, err := pending.Enrollment(ctx, challenge)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, legacy, err := pending.DecodeTOTP(stored); err != nil || legacy || decoded != "fixture-secret" {
		t.Fatal("pending enrollment lookup failed")
	}
	if err := pending.Consume(ctx, challenge); err != nil {
		t.Fatal(err)
	}
	if err := pending.Consume(ctx, challenge); !errors.Is(err, redis.Nil) {
		t.Fatal("pending token remained reusable")
	}
	if _, err := pending.Enrollment(ctx, challenge); !errors.Is(err, redis.Nil) {
		t.Fatal("pending TOTP material was not removed")
	}
}
